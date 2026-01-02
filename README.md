# qemuctl

A Go library for managing QEMU virtual machines via QMP (QEMU Monitor Protocol).

## Features

- **Launch QEMU instances** with simplified configuration
- **Attach to existing QEMU processes** by socket path or PID
- **Full QMP support** for VM control (start, stop, pause, reset, etc.)
- **VNC/SPICE client passthrough** via SCM_RIGHTS file descriptor passing
- **Event handling** with callbacks for state changes
- **Automatic QEMU discovery** in PATH or standard locations
- **Architecture support** using GOARCH-style names (amd64, arm64, etc.)

## Installation

```bash
go get github.com/KarpelesLab/qemuctl
```

## Quick Start

### Start a New VM

```go
package main

import (
    "log"
    "time"

    "github.com/KarpelesLab/qemuctl"
)

func main() {
    cfg := qemuctl.DefaultConfig()
    cfg.Name = "my-vm"
    cfg.Memory = 2048
    cfg.CPUs = 4
    cfg.Drives = []qemuctl.DriveConfig{
        {File: "/path/to/disk.qcow2", Format: "qcow2"},
    }

    inst, err := qemuctl.Start(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer inst.Stop(30 * time.Second)

    log.Printf("VM started with PID %d", inst.PID())
    log.Printf("State: %s", inst.State())

    // Wait for VM to exit
    inst.Wait()
}
```

### Attach to Existing VM

```go
// By socket path
inst, err := qemuctl.Attach("/var/run/qemu/my-vm.sock")

// By PID (automatically finds the QMP socket from process arguments)
inst, err := qemuctl.AttachByPID(12345)
```

### VM Control

```go
// Pause and resume
inst.Pause()
inst.Continue()

// Reset (hard reboot)
inst.Reset()

// Graceful shutdown (sends ACPI power button)
inst.Shutdown()

// Graceful shutdown with timeout, then force kill
inst.Stop(30 * time.Second)

// Force kill immediately
inst.ForceStop()

// Send quit command to QEMU
inst.Quit()
```

### VNC/SPICE Client Passthrough

Pass incoming client connections directly to QEMU:

```go
// Accept a client connection from your proxy/server
clientConn, _ := listener.Accept()

// Pass it to QEMU for VNC (skipAuth=true to skip VNC auth)
err := inst.AddVNCClient(clientConn, true)

// Or for SPICE
err := inst.AddSpiceClient(clientConn, false)
```

Set display passwords:

```go
inst.SetVNCPassword("secret123")
inst.SetSpicePassword("secret123")
```

### Event Handling

```go
// Set callback for state changes
inst.SetStateChangeCallback(func(state qemuctl.State) {
    log.Printf("State changed to: %s", state)
})

// Set callback for all events
inst.SetEventCallback(func(event *qemuctl.Event) {
    log.Printf("Event: %s, Data: %v", event.Name, event.Data)
})

// Or read from event channel
go func() {
    for event := range inst.Events() {
        log.Printf("Event: %s", event.Name)
    }
}()
```

### Direct QMP Commands

```go
// Execute any QMP command
result, err := inst.QMP().Execute("query-version", nil)

// Human monitor command (for commands not in QMP)
output, err := inst.HumanMonitorCommand("info registers")
```

### Utility Functions

```go
// Query block devices
blocks, err := inst.QueryBlockDevices()

// Set I/O throttling
err := inst.SetIOThrottle("drive0", 100*1024*1024, 1000) // 100MB/s, 1000 IOPS

// Capture screenshot
err := inst.Screendump("/tmp/screen.ppm")

// Send key combination
err := inst.SendKey("ctrl", "alt", "delete")
```

## Configuration

### Config Options

| Field | Type | Description |
|-------|------|-------------|
| `Name` | string | Instance name (auto-generated if empty) |
| `Arch` | string | Target architecture (GOARCH-style, defaults to runtime.GOARCH) |
| `QemuPath` | string | Path to QEMU binary (auto-detected if empty) |
| `SocketDir` | string | Directory for control sockets |
| `Memory` | int | Memory in MB (default: 512) |
| `CPUs` | int | Number of vCPUs (default: 1) |
| `Machine` | string | Machine type (e.g., "q35", "pc", "virt") |
| `CPU` | string | CPU model (default: "host" with KVM) |
| `KVM` | *bool | Enable KVM acceleration (default: true) |
| `Drives` | []DriveConfig | Disk configurations |
| `NetworkDevices` | []NetDevConfig | Network configurations |
| `VNC` | string | VNC display config (e.g., "none", ":0") |
| `Spice` | *SpiceConfig | SPICE display config |
| `ExtraArgs` | []string | Additional QEMU arguments |
| `NoDefaults` | *bool | Disable QEMU default devices (default: true) |

### Socket Locations

Control sockets are created in:
- **Root**: `/var/run/qemu/<name>.sock`
- **User**: `<os.UserCacheDir()>/qemuctl/<name>.sock`

Override with `Config.SocketDir`.

### QEMU Binary Discovery

The library searches for QEMU in this order:
1. `Config.QemuPath` if provided
2. `PATH` environment variable
3. `/pkg/main/app-emulation.qemu.core/bin/`
4. `/usr/bin`, `/usr/local/bin`

## States

| State | Description |
|-------|-------------|
| `StateUnknown` | Unknown state |
| `StateRunning` | VM is running |
| `StatePaused` | VM is paused |
| `StateShutdown` | VM has shut down |
| `StateCrashed` | VM has crashed |
| `StateSuspended` | VM is suspended |
| `StatePrelaunch` | VM is initializing |

## Architecture Mapping

| GOARCH | QEMU Binary |
|--------|-------------|
| amd64 | qemu-system-x86_64 |
| 386 | qemu-system-i386 |
| arm64 | qemu-system-aarch64 |
| arm | qemu-system-arm |
| riscv64 | qemu-system-riscv64 |
| ppc64/ppc64le | qemu-system-ppc64 |
| mips | qemu-system-mips |
| mips64 | qemu-system-mips64 |
| s390x | qemu-system-s390x |

## License

See LICENSE file.
