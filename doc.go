// Package qemuctl provides a Go library for managing QEMU virtual machines.
//
// The library supports:
//   - Launching QEMU instances with a simplified configuration
//   - Attaching to existing QEMU processes
//   - Controlling VMs via QMP (QEMU Monitor Protocol)
//   - VNC and SPICE client connection passthrough
//   - State tracking and event handling
//
// # Quick Start
//
// Start a new QEMU instance:
//
//	cfg := qemuctl.DefaultConfig()
//	cfg.Memory = 2048
//	cfg.CPUs = 4
//	cfg.Drives = []qemuctl.DriveConfig{
//		{File: "/path/to/disk.qcow2", Format: "qcow2"},
//	}
//
//	inst, err := qemuctl.Start(cfg)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer inst.Stop(30 * time.Second)
//
// Attach to an existing QEMU instance by socket:
//
//	inst, err := qemuctl.Attach("/var/run/qemu/myvm.sock")
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Attach to an existing QEMU instance by PID:
//
//	inst, err := qemuctl.AttachByPID(12345)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// # Control Socket Location
//
// When starting a new instance, the QMP control socket is created in:
//   - For root: /var/run/qemu/<name>.sock
//   - For users: <os.UserCacheDir()>/qemuctl/<name>.sock
//
// You can override this by setting Config.SocketDir.
//
// # QEMU Binary Location
//
// The library searches for QEMU binaries in this order:
//  1. Config.QemuPath if provided
//  2. PATH environment variable
//  3. /pkg/main/app-emulation.qemu.core/bin/
//  4. Common system paths (/usr/bin, /usr/local/bin)
//
// # Architecture Support
//
// The library uses GOARCH-style architecture names (amd64, arm64, 386, etc.)
// and maps them to QEMU binary names (qemu-system-x86_64, qemu-system-aarch64, etc.).
//
// # VNC and SPICE
//
// The library supports passing client connections to QEMU for VNC and SPICE:
//
//	// Accept a client connection
//	clientConn, _ := listener.Accept()
//
//	// Pass it to QEMU for VNC
//	err := inst.AddVNCClient(clientConn, true)
//
// This uses QEMU's add_client command with SCM_RIGHTS to pass the file descriptor.
package qemuctl
