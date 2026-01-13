# qemuctl

A Go library for managing QEMU virtual machines via QMP (QEMU Monitor Protocol).

## Features

- **Launch QEMU instances** with comprehensive configuration
- **Attach to existing QEMU processes** by socket path or PID
- **Full QMP support** for VM control (start, stop, pause, reset, etc.)
- **VNC/SPICE client passthrough** via SCM_RIGHTS file descriptor passing
- **Event handling** with callbacks for state changes
- **Automatic QEMU discovery** in PATH or standard locations
- **Architecture support** using GOARCH-style names (amd64, arm64, etc.)
- **Multiple disk backends** (file, NBD, RBD/Ceph, iSCSI)
- **Multiple network backends** (user NAT, TAP, socket, stream, VDE, bridge)
- **I/O throttling** with configurable BPS/IOPS limits
- **EFI/UEFI support** with OVMF firmware
- **Guest agent support** via virtio-serial
- **Automatic PCI slot allocation** for Q35 and i440FX machines

## Installation

```bash
go get github.com/KarpelesLab/qemuctl
```

## Quick Start

### Simple VM (Basic Config)

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

### Advanced VM (VMConfig)

For full control over VM configuration, use `VMConfig` and `StartVM`:

```go
package main

import (
    "log"
    "time"

    "github.com/KarpelesLab/qemuctl"
)

func main() {
    cfg := &qemuctl.VMConfig{
        Name: "production-vm",
        Machine: &qemuctl.MachineConfig{
            Type:  "q35",
            Accel: "kvm",
        },
        CPU: &qemuctl.CPUConfig{
            Model:   "host",
            Sockets: 1,
            Cores:   4,
            Threads: 2,
        },
        Memory: &qemuctl.MemoryConfig{
            Size: 8192, // 8GB
        },
        EFI: &qemuctl.EFIConfig{
            Code: "/usr/share/OVMF/OVMF_CODE.fd",
            Vars: "/var/lib/qemu/my-vm_VARS.fd",
        },
        Disks: []*qemuctl.DiskConfig{
            {
                ID: "system",
                Backend: &qemuctl.FileDiskBackend{
                    Path:   "/var/lib/qemu/system.qcow2",
                    Format: "qcow2",
                },
                Interface: "virtio",
                BootIndex: 1,
            },
        },
        Networks: []*qemuctl.NetworkConfig{
            {
                ID: "net0",
                Backend: &qemuctl.UserNetBackend{
                    Hostfwd: []string{"tcp::2222-:22"},
                },
                Model:   "virtio-net-pci",
                MACAddr: "52:54:00:12:34:56",
            },
        },
        Display: &qemuctl.DisplayConfig{
            Type: "vnc",
            VNC: &qemuctl.VNCConfig{
                Listen: "none", // Use add_client mode
            },
        },
        NoDefaults: true,
    }

    // Add guest agent support
    cfg.WithGuestAgent("/var/run/qemu/my-vm-qga.sock")

    // Add USB tablet for better mouse handling
    cfg.WithUSBTablet()

    inst, err := qemuctl.StartVM(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer inst.Stop(30 * time.Second)

    log.Printf("VM started with PID %d", inst.PID())
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

## Disk Backends

### File Backend

```go
backend := &qemuctl.FileDiskBackend{
    Path:         "/var/lib/qemu/disk.qcow2",
    Format:       "qcow2", // or "raw", "vmdk"
    AutoReadOnly: true,
}
```

### NBD Backend

```go
// Unix socket
backend := &qemuctl.NBDDiskBackend{
    SocketPath: "/tmp/nbd.sock",
    Export:     "disk0",
}

// TCP
backend := &qemuctl.NBDDiskBackend{
    Host:   "192.168.1.100",
    Port:   10809,
    Export: "disk0",
    TLS:    true,
}
```

### Ceph RBD Backend

```go
backend := &qemuctl.RBDDiskBackend{
    Pool:      "rbd",
    Image:     "vm-disk-0",
    User:      "admin",
    KeySecret: "ceph-key", // Reference to a secret object
    Conf:      "/etc/ceph/ceph.conf",
}
```

### iSCSI Backend

```go
backend := &qemuctl.ISCSIDiskBackend{
    Portal:         "192.168.1.100:3260",
    Target:         "iqn.2023-01.com.example:storage",
    Lun:            0,
    User:           "admin",
    PasswordSecret: "iscsi-password",
    InitiatorName:  "iqn.2023-01.com.example:client",
}
```

### I/O Throttling

```go
disk := &qemuctl.DiskConfig{
    ID:      "drive0",
    Backend: backend,
    Throttle: &qemuctl.ThrottleConfig{
        Group:    "tg0",
        BPS:      100 * 1024 * 1024, // 100 MB/s
        IOPS:     1000,
        BPSMax:   200 * 1024 * 1024, // Burst: 200 MB/s
        IOPSMax:  2000,
    },
}
```

## Network Backends

### User Mode (NAT)

```go
backend := &qemuctl.UserNetBackend{
    Hostfwd: []string{
        "tcp::2222-:22",  // SSH
        "tcp::8080-:80",  // HTTP
    },
    Net:      "10.0.2.0/24",
    DNS:      "10.0.2.3",
    Restrict: false,
}
```

### TAP Device

```go
backend := &qemuctl.TapNetBackend{
    Ifname:     "tap0",
    Bridge:     "br0",
    Script:     "no",     // Or path to up script
    DownScript: "no",
    VHost:      true,     // Enable vhost-net
    Queues:     4,        // Multiqueue
}
```

### Socket Backend

```go
backend := &qemuctl.SocketNetBackend{
    Path:   "/tmp/vm-network.sock",
    Server: true,
}
```

### Stream Backend (QEMU 7.2+)

```go
// Unix socket
backend := &qemuctl.StreamNetBackend{
    Path:   "/tmp/stream.sock",
    Server: true,
}

// TCP with reconnect
backend := &qemuctl.StreamNetBackend{
    Host:      "192.168.1.1",
    Port:      5000,
    Server:    false,
    Reconnect: 10, // Reconnect every 10 seconds
}
```

### Bridge Helper

```go
backend := &qemuctl.BridgeNetBackend{
    Bridge: "br0",
    Helper: "/usr/lib/qemu/qemu-bridge-helper",
}
```

## Display Configuration

### VNC

```go
display := &qemuctl.DisplayConfig{
    Type: "vnc",
    VNC: &qemuctl.VNCConfig{
        Listen:         "none",  // Use add_client mode
        PasswordSecret: "vnc-pw",
        Lossy:          true,
        Websocket:      5901,
    },
    Video: &qemuctl.VideoConfig{
        Type:   "virtio-vga",
        VgaMem: 64,
    },
}
```

### SPICE

```go
display := &qemuctl.DisplayConfig{
    Type: "spice",
    Spice: &qemuctl.SpiceDisplayConfig{
        Unix:                true,
        DisableTicketing:    false,
        PasswordSecret:      "spice-pw",
        ImageCompression:    "auto_glz",
        PlaybackCompression: true,
        SeamlessMigration:   true,
    },
    Video: &qemuctl.VideoConfig{
        Type:   "qxl-vga",
        VgaMem: 64,
        Ram:    128 * 1024 * 1024,
        Vram:   128 * 1024 * 1024,
    },
}

// Add SPICE agent for copy/paste
cfg.WithSpiceAgent()
```

## Configuration Reference

### Basic Config Options

| Field | Type | Description |
|-------|------|-------------|
| `Name` | string | Instance name (auto-generated if empty) |
| `Arch` | string | Target architecture (GOARCH-style) |
| `QemuPath` | string | Path to QEMU binary (auto-detected if empty) |
| `SocketDir` | string | Directory for control sockets |
| `Memory` | int | Memory in MB (default: 512) |
| `CPUs` | int | Number of vCPUs (default: 1) |
| `Machine` | string | Machine type (e.g., "q35", "pc", "virt") |
| `CPU` | string | CPU model (default: "host" with KVM) |
| `KVM` | *bool | Enable KVM acceleration (default: true) |
| `NoDefaults` | *bool | Disable QEMU default devices (default: true) |

### VMConfig Options

| Field | Type | Description |
|-------|------|-------------|
| `Machine` | *MachineConfig | Machine type, accelerator, pflash |
| `CPU` | *CPUConfig | CPU model, features, topology |
| `Memory` | *MemoryConfig | Size, backend, memory locking |
| `EFI` | *EFIConfig | UEFI firmware (OVMF) configuration |
| `Boot` | *BootConfig | Boot order, kernel, initrd |
| `Disks` | []*DiskConfig | Disk configurations with backends |
| `CDROMs` | []*CDROMConfig | CD-ROM drives |
| `Networks` | []*NetworkConfig | Network configurations |
| `Display` | *DisplayConfig | VNC, SPICE, video device |
| `Audio` | *AudioConfig | Sound device and backend |
| `Serials` | []*SerialConfig | Serial ports |
| `Chardevs` | []*ChardevConfig | Character devices |
| `VirtioSerial` | *VirtioSerialConfig | Virtio-serial controller |
| `USB` | *USBControllerConfig | USB controller |
| `USBDevices` | []*USBDeviceConfig | USB devices |
| `Balloon` | *BalloonConfig | Memory balloon |
| `RTC` | *RTCConfig | Real-time clock |
| `Secrets` | []*SecretConfig | Secret objects |

### Socket Locations

Control sockets are created in:
- **Root**: `/var/run/qemu/<name>.sock`
- **User**: `<os.UserCacheDir()>/qemuctl/<name>.sock`

Override with `Config.SocketDir` or `VMConfig.SocketDir`.

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
