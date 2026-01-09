package qemuctl

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPCISlotAllocator(t *testing.T) {
	t.Run("Q35 machine", func(t *testing.T) {
		alloc := newPCISlotAllocator(true)
		if alloc.Bus() != "pcie.0" {
			t.Errorf("expected bus pcie.0, got %s", alloc.Bus())
		}

		// First allocation should be 0x3 (0, 1, 2 reserved)
		slot := alloc.Alloc()
		if slot != "0x3" {
			t.Errorf("expected first slot 0x3, got %s", slot)
		}

		// Second should be 0x4
		slot = alloc.Alloc()
		if slot != "0x4" {
			t.Errorf("expected second slot 0x4, got %s", slot)
		}
	})

	t.Run("i440FX machine", func(t *testing.T) {
		alloc := newPCISlotAllocator(false)
		if alloc.Bus() != "pci.0" {
			t.Errorf("expected bus pci.0, got %s", alloc.Bus())
		}
	})

	t.Run("Reserve slot", func(t *testing.T) {
		alloc := newPCISlotAllocator(true)
		alloc.Reserve(0x5)

		slot := alloc.Alloc() // 0x3
		slot = alloc.Alloc()  // 0x4
		slot = alloc.Alloc()  // Should skip 0x5 and be 0x6

		if slot != "0x6" {
			t.Errorf("expected slot 0x6 after reserving 0x5, got %s", slot)
		}
	})
}

func TestUserNetBackend(t *testing.T) {
	tests := []struct {
		name     string
		backend  *UserNetBackend
		contains []string
	}{
		{
			name:     "basic",
			backend:  &UserNetBackend{},
			contains: []string{"user", "id=test"},
		},
		{
			name: "with port forwarding",
			backend: &UserNetBackend{
				Hostfwd: []string{"tcp::2222-:22", "tcp::8080-:80"},
			},
			contains: []string{"hostfwd=tcp::2222-:22", "hostfwd=tcp::8080-:80"},
		},
		{
			name: "with custom network",
			backend: &UserNetBackend{
				Net:       "10.0.2.0/24",
				Host:      "10.0.2.2",
				DNS:       "10.0.2.3",
				DHCPStart: "10.0.2.15",
			},
			contains: []string{"net=10.0.2.0/24", "host=10.0.2.2", "dns=10.0.2.3", "dhcpstart=10.0.2.15"},
		},
		{
			name: "restricted",
			backend: &UserNetBackend{
				Restrict: true,
			},
			contains: []string{"restrict=on"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.backend.BuildNetdevArgs("test")
			argsStr := strings.Join(args, " ")

			for _, want := range tt.contains {
				if !strings.Contains(argsStr, want) {
					t.Errorf("expected args to contain %q, got: %s", want, argsStr)
				}
			}
		})
	}
}

func TestTapNetBackend(t *testing.T) {
	tests := []struct {
		name     string
		backend  *TapNetBackend
		contains []string
	}{
		{
			name:     "basic",
			backend:  &TapNetBackend{},
			contains: []string{"tap", "id=test"},
		},
		{
			name: "with interface",
			backend: &TapNetBackend{
				Ifname: "tap0",
				Bridge: "br0",
			},
			contains: []string{"ifname=tap0", "br=br0"},
		},
		{
			name: "with vhost",
			backend: &TapNetBackend{
				VHost:  true,
				Queues: 4,
			},
			contains: []string{"vhost=on", "queues=4"},
		},
		{
			name: "with scripts disabled",
			backend: &TapNetBackend{
				Script:     "no",
				DownScript: "no",
			},
			contains: []string{"script=no", "downscript=no"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.backend.BuildNetdevArgs("test")
			argsStr := strings.Join(args, " ")

			for _, want := range tt.contains {
				if !strings.Contains(argsStr, want) {
					t.Errorf("expected args to contain %q, got: %s", want, argsStr)
				}
			}
		})
	}
}

func TestStreamNetBackend(t *testing.T) {
	tests := []struct {
		name     string
		backend  *StreamNetBackend
		contains []string
	}{
		{
			name: "unix socket server",
			backend: &StreamNetBackend{
				Path:   "/tmp/test.sock",
				Server: true,
			},
			contains: []string{"stream", "server=on", "addr.type=unix", "addr.path=/tmp/test.sock"},
		},
		{
			name: "tcp client with reconnect",
			backend: &StreamNetBackend{
				Host:      "192.168.1.1",
				Port:      5000,
				Server:    false,
				Reconnect: 10,
			},
			contains: []string{"server=off", "addr.type=inet", "addr.host=192.168.1.1", "addr.port=5000", "reconnect=10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.backend.BuildNetdevArgs("test")
			argsStr := strings.Join(args, " ")

			for _, want := range tt.contains {
				if !strings.Contains(argsStr, want) {
					t.Errorf("expected args to contain %q, got: %s", want, argsStr)
				}
			}
		})
	}
}

func TestFileDiskBackend(t *testing.T) {
	backend := &FileDiskBackend{
		Path:         "/var/lib/qemu/disk.qcow2",
		Format:       "qcow2",
		AutoReadOnly: true,
	}

	args := backend.BuildBlockdevArgs("drive0")
	argsStr := strings.Join(args, " ")

	// Should have two -blockdev args
	if !strings.Contains(argsStr, "-blockdev") {
		t.Error("expected -blockdev args")
	}

	// First blockdev should be file backend
	if !strings.Contains(argsStr, `"driver":"file"`) {
		t.Error("expected file driver")
	}

	// Should contain filename
	if !strings.Contains(argsStr, `"filename":"/var/lib/qemu/disk.qcow2"`) {
		t.Error("expected filename")
	}

	// Should contain format layer
	if !strings.Contains(argsStr, `"driver":"qcow2"`) {
		t.Error("expected qcow2 format")
	}

	// Should have auto-read-only
	if !strings.Contains(argsStr, `"auto-read-only":true`) {
		t.Error("expected auto-read-only")
	}
}

func TestNBDDiskBackend(t *testing.T) {
	t.Run("unix socket", func(t *testing.T) {
		backend := &NBDDiskBackend{
			SocketPath: "/tmp/nbd.sock",
			Export:     "disk0",
		}

		args := backend.BuildBlockdevArgs("drive0")
		argsStr := strings.Join(args, " ")

		if !strings.Contains(argsStr, `"driver":"nbd"`) {
			t.Error("expected nbd driver")
		}
		if !strings.Contains(argsStr, `"path":"/tmp/nbd.sock"`) {
			t.Error("expected socket path")
		}
		if !strings.Contains(argsStr, `"export":"disk0"`) {
			t.Error("expected export name")
		}
	})

	t.Run("tcp connection", func(t *testing.T) {
		backend := &NBDDiskBackend{
			Host: "192.168.1.100",
			Port: 10809,
		}

		args := backend.BuildBlockdevArgs("drive0")
		argsStr := strings.Join(args, " ")

		if !strings.Contains(argsStr, `"type":"inet"`) {
			t.Error("expected inet type")
		}
		if !strings.Contains(argsStr, `"host":"192.168.1.100"`) {
			t.Error("expected host")
		}
	})
}

func TestRBDDiskBackend(t *testing.T) {
	backend := &RBDDiskBackend{
		Pool:      "rbd",
		Image:     "vm-disk-0",
		User:      "admin",
		KeySecret: "ceph-key",
		Conf:      "/etc/ceph/ceph.conf",
	}

	args := backend.BuildBlockdevArgs("drive0")
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, `"driver":"rbd"`) {
		t.Error("expected rbd driver")
	}
	if !strings.Contains(argsStr, `"pool":"rbd"`) {
		t.Error("expected pool")
	}
	if !strings.Contains(argsStr, `"image":"vm-disk-0"`) {
		t.Error("expected image")
	}
	if !strings.Contains(argsStr, `"user":"admin"`) {
		t.Error("expected user")
	}
	if !strings.Contains(argsStr, `"key-secret":"ceph-key"`) {
		t.Error("expected key-secret")
	}
}

func TestThrottleConfig(t *testing.T) {
	cfg := &ThrottleConfig{
		Group:   "throttle0",
		BPS:     100 * 1024 * 1024, // 100 MB/s
		IOPS:    1000,
		BPSMax:  200 * 1024 * 1024,
		IOPSMax: 2000,
	}

	// Test throttle group args
	groupArgs := cfg.BuildThrottleGroupArgs()
	argsStr := strings.Join(groupArgs, " ")

	if !strings.Contains(argsStr, "throttle-group") {
		t.Error("expected throttle-group")
	}
	if !strings.Contains(argsStr, "id=throttle0") {
		t.Error("expected id")
	}
	if !strings.Contains(argsStr, "x-bps-total=104857600") {
		t.Error("expected x-bps-total")
	}
	if !strings.Contains(argsStr, "x-iops-total=1000") {
		t.Error("expected x-iops-total")
	}

	// Test throttle blockdev args
	blockdevArgs := cfg.BuildThrottleBlockdevArgs("drive0-throttle", "drive0-format")
	blockdevStr := strings.Join(blockdevArgs, " ")

	if !strings.Contains(blockdevStr, `"driver":"throttle"`) {
		t.Error("expected throttle driver")
	}
	if !strings.Contains(blockdevStr, `"throttle-group":"throttle0"`) {
		t.Error("expected throttle-group reference")
	}
}

func TestVMBuilderBasic(t *testing.T) {
	cfg := &VMConfig{
		Name: "test-vm",
		Machine: &MachineConfig{
			Type:  "q35",
			Accel: "kvm",
		},
		CPU: &CPUConfig{
			Model:   "host",
			Sockets: 1,
			Cores:   2,
			Threads: 2,
		},
		Memory: &MemoryConfig{
			Size: 2048,
		},
		NoDefaults: true,
	}

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Check essential args
	if !strings.Contains(argsStr, "-name guest=test-vm") {
		t.Error("expected name arg")
	}
	if !strings.Contains(argsStr, "-no-user-config") {
		t.Error("expected -no-user-config")
	}
	if !strings.Contains(argsStr, "-nodefaults") {
		t.Error("expected -nodefaults")
	}
	if !strings.Contains(argsStr, "-cpu host") {
		t.Error("expected cpu host")
	}
	if !strings.Contains(argsStr, "-m 2048") {
		t.Error("expected memory 2048")
	}
	if !strings.Contains(argsStr, "path=/tmp/test.sock") {
		t.Error("expected socket path")
	}
}

func TestVMBuilderWithDisk(t *testing.T) {
	cfg := &VMConfig{
		Name: "test-vm",
		Disks: []*DiskConfig{
			{
				ID: "disk0",
				Backend: &FileDiskBackend{
					Path:   "/var/lib/qemu/test.qcow2",
					Format: "qcow2",
				},
				Interface: "virtio",
				BootIndex: 1,
			},
		},
		NoDefaults: true,
	}

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Should have blockdev args
	if !strings.Contains(argsStr, "-blockdev") {
		t.Error("expected -blockdev args")
	}

	// Should have virtio-blk-pci device
	if !strings.Contains(argsStr, "virtio-blk-pci") {
		t.Error("expected virtio-blk-pci device")
	}

	// Should have boot index
	if !strings.Contains(argsStr, "bootindex=1") {
		t.Error("expected bootindex=1")
	}
}

func TestVMBuilderWithNetwork(t *testing.T) {
	cfg := &VMConfig{
		Name: "test-vm",
		Networks: []*NetworkConfig{
			{
				ID: "net0",
				Backend: &UserNetBackend{
					Hostfwd: []string{"tcp::2222-:22"},
				},
				Model:   "virtio-net-pci",
				MACAddr: "52:54:00:12:34:56",
			},
		},
		NoDefaults: true,
	}

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Should have netdev
	if !strings.Contains(argsStr, "-netdev") {
		t.Error("expected -netdev")
	}
	if !strings.Contains(argsStr, "hostfwd=tcp::2222-:22") {
		t.Error("expected port forwarding")
	}

	// Should have virtio-net-pci device
	if !strings.Contains(argsStr, "virtio-net-pci") {
		t.Error("expected virtio-net-pci device")
	}

	// Should have MAC address
	if !strings.Contains(argsStr, "mac=52:54:00:12:34:56") {
		t.Error("expected MAC address")
	}
}

func TestVMBuilderWithVNC(t *testing.T) {
	cfg := &VMConfig{
		Name: "test-vm",
		Display: &DisplayConfig{
			Type: "vnc",
			VNC: &VNCConfig{
				Listen:    "none",
				Websocket: 5901,
			},
		},
		NoDefaults: true,
	}

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Should have VNC args
	if !strings.Contains(argsStr, "-vnc") {
		t.Error("expected -vnc")
	}
	if !strings.Contains(argsStr, "none") {
		t.Error("expected vnc listen=none")
	}
	if !strings.Contains(argsStr, "websocket=5901") {
		t.Error("expected websocket port")
	}
}

func TestVMBuilderWithSPICE(t *testing.T) {
	cfg := &VMConfig{
		Name: "test-vm",
		Display: &DisplayConfig{
			Type: "spice",
			Spice: &SpiceDisplayConfig{
				Unix:             true,
				DisableTicketing: true,
				DisableCopyPaste: false,
			},
			Video: &VideoConfig{
				Type:   "qxl-vga",
				VgaMem: 64,
			},
		},
		NoDefaults: true,
	}

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Should have SPICE args
	if !strings.Contains(argsStr, "-spice") {
		t.Error("expected -spice")
	}
	if !strings.Contains(argsStr, "unix=on") {
		t.Error("expected unix=on")
	}
	if !strings.Contains(argsStr, "disable-ticketing=on") {
		t.Error("expected disable-ticketing=on")
	}

	// Should have QXL device
	if !strings.Contains(argsStr, "qxl-vga") {
		t.Error("expected qxl-vga device")
	}
}

func TestVMBuilderWithGuestAgent(t *testing.T) {
	cfg := &VMConfig{
		Name:       "test-vm",
		NoDefaults: true,
	}
	cfg.WithGuestAgent("/tmp/qga.sock")

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Should have chardev for guest agent
	if !strings.Contains(argsStr, "-chardev") {
		t.Error("expected -chardev")
	}
	if !strings.Contains(argsStr, "path=/tmp/qga.sock") {
		t.Error("expected guest agent socket path")
	}

	// Should have virtio-serial
	if !strings.Contains(argsStr, "virtio-serial-pci") {
		t.Error("expected virtio-serial-pci")
	}

	// Should have virtserialport with guest agent name
	if !strings.Contains(argsStr, "org.qemu.guest_agent.0") {
		t.Error("expected guest agent port name")
	}
}

func TestVMBuilderWithUSB(t *testing.T) {
	cfg := &VMConfig{
		Name:       "test-vm",
		NoDefaults: true,
	}
	cfg.WithUSBTablet()

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Should have USB controller
	if !strings.Contains(argsStr, "qemu-xhci") {
		t.Error("expected qemu-xhci controller")
	}

	// Should have USB tablet
	if !strings.Contains(argsStr, "usb-tablet") {
		t.Error("expected usb-tablet device")
	}
}

func TestVMBuilderWithCDROM(t *testing.T) {
	cfg := &VMConfig{
		Name: "test-vm",
		CDROMs: []*CDROMConfig{
			{
				Path:      "/path/to/install.iso",
				BootIndex: 0,
			},
		},
		NoDefaults: true,
	}

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Should have SATA controller
	if !strings.Contains(argsStr, "ich9-ahci") {
		t.Error("expected ich9-ahci controller")
	}

	// Should have drive with CD-ROM
	if !strings.Contains(argsStr, "media=cdrom") {
		t.Error("expected media=cdrom")
	}

	// Should have ide-cd device
	if !strings.Contains(argsStr, "ide-cd") {
		t.Error("expected ide-cd device")
	}
}

func TestVMBuilderWithEFI(t *testing.T) {
	cfg := &VMConfig{
		Name: "test-vm",
		EFI: &EFIConfig{
			Code: "/usr/share/OVMF/OVMF_CODE.fd",
			Vars: "/var/lib/qemu/test_VARS.fd",
		},
		NoDefaults: true,
	}

	builder := NewVMBuilder(cfg)
	args := builder.Build("test-vm", "/tmp/test.sock")
	argsStr := strings.Join(args, " ")

	// Should have pflash blockdev args
	if !strings.Contains(argsStr, "pflash0-file") {
		t.Error("expected pflash0-file node")
	}
	if !strings.Contains(argsStr, "pflash1-file") {
		t.Error("expected pflash1-file node")
	}
	if !strings.Contains(argsStr, `"read-only":true`) {
		t.Error("expected read-only pflash0")
	}
}

func TestBuildMachineArgs(t *testing.T) {
	usb := false
	dumpCore := true

	cfg := &MachineConfig{
		Type:          "q35",
		Accel:         "kvm",
		USB:           &usb,
		DumpGuestCore: &dumpCore,
		Pflash0:       "pflash0",
		Pflash1:       "pflash1",
	}

	args := buildMachineArgs(cfg)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "-machine") {
		t.Error("expected -machine")
	}
	if !strings.Contains(argsStr, "q35") {
		t.Error("expected q35")
	}
	if !strings.Contains(argsStr, "accel=kvm") {
		t.Error("expected accel=kvm")
	}
	if !strings.Contains(argsStr, "usb=off") {
		t.Error("expected usb=off")
	}
	if !strings.Contains(argsStr, "dump-guest-core=on") {
		t.Error("expected dump-guest-core=on")
	}
	if !strings.Contains(argsStr, "pflash0=pflash0") {
		t.Error("expected pflash0")
	}
}

func TestBuildCPUArgs(t *testing.T) {
	cfg := &CPUConfig{
		Model:    "host",
		Features: []string{"+aes", "-sse4.2"},
		Sockets:  2,
		Cores:    4,
		Threads:  2,
	}

	args := buildCPUArgs(cfg, 4096)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "-cpu host,+aes,-sse4.2") {
		t.Errorf("expected cpu with features, got: %s", argsStr)
	}
	if !strings.Contains(argsStr, "-m 4096") {
		t.Error("expected memory")
	}
	if !strings.Contains(argsStr, "-smp 16,sockets=2,cores=4,threads=2") {
		t.Errorf("expected smp, got: %s", argsStr)
	}
}

func TestBuildVNCArgs(t *testing.T) {
	cfg := &VNCConfig{
		Listen:         ":0",
		PasswordSecret: "vnc-password",
		Lossy:          true,
		AudioDev:       "audio0",
		Websocket:      5901,
	}

	args := buildVNCArgs(cfg)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "-vnc") {
		t.Error("expected -vnc")
	}
	if !strings.Contains(argsStr, ":0") {
		t.Error("expected listen address")
	}
	if !strings.Contains(argsStr, "password-secret=vnc-password") {
		t.Error("expected password-secret")
	}
	if !strings.Contains(argsStr, "lossy=on") {
		t.Error("expected lossy=on")
	}
	if !strings.Contains(argsStr, "audiodev=audio0") {
		t.Error("expected audiodev")
	}
	if !strings.Contains(argsStr, "websocket=5901") {
		t.Error("expected websocket")
	}
}

func TestBuildSpiceArgs(t *testing.T) {
	cfg := &SpiceDisplayConfig{
		Port:                  5900,
		PasswordSecret:        "spice-password",
		ImageCompression:      "auto_glz",
		JpegWanCompression:    "auto",
		ZlibGlzWanCompression: "auto",
		PlaybackCompression:   true,
		SeamlessMigration:     true,
		DisableCopyPaste:      true,
	}

	args := buildSpiceArgs(cfg)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "-spice") {
		t.Error("expected -spice")
	}
	if !strings.Contains(argsStr, "port=5900") {
		t.Error("expected port")
	}
	if !strings.Contains(argsStr, "password-secret=spice-password") {
		t.Error("expected password-secret")
	}
	if !strings.Contains(argsStr, "image-compression=auto_glz") {
		t.Error("expected image-compression")
	}
	if !strings.Contains(argsStr, "playback-compression=on") {
		t.Error("expected playback-compression")
	}
	if !strings.Contains(argsStr, "seamless-migration=on") {
		t.Error("expected seamless-migration")
	}
	if !strings.Contains(argsStr, "disable-copy-paste=on") {
		t.Error("expected disable-copy-paste")
	}
}

func TestBuildChardevArgs(t *testing.T) {
	cfg := &ChardevConfig{
		ID:        "qga0",
		Backend:   "socket",
		Path:      "/tmp/qga.sock",
		Server:    true,
		Wait:      false,
		Reconnect: 10,
	}

	args := buildChardevArgs(cfg)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "-chardev") {
		t.Error("expected -chardev")
	}
	if !strings.Contains(argsStr, "socket") {
		t.Error("expected socket backend")
	}
	if !strings.Contains(argsStr, "id=qga0") {
		t.Error("expected id")
	}
	if !strings.Contains(argsStr, "path=/tmp/qga.sock") {
		t.Error("expected path")
	}
	if !strings.Contains(argsStr, "server=on") {
		t.Error("expected server=on")
	}
	if !strings.Contains(argsStr, "wait=off") {
		t.Error("expected wait=off")
	}
	if !strings.Contains(argsStr, "reconnect=10") {
		t.Error("expected reconnect")
	}
}

func TestBuildSecretArgs(t *testing.T) {
	cfg := &SecretConfig{
		ID:     "vnc-password",
		Data:   "secret123",
		Format: "raw",
	}

	args := buildSecretArgs(cfg)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "-object") {
		t.Error("expected -object")
	}
	if !strings.Contains(argsStr, "secret") {
		t.Error("expected secret")
	}
	if !strings.Contains(argsStr, "id=vnc-password") {
		t.Error("expected id")
	}
	if !strings.Contains(argsStr, "data=secret123") {
		t.Error("expected data")
	}
	if !strings.Contains(argsStr, "format=raw") {
		t.Error("expected format")
	}
}

func TestDefaultVMConfig(t *testing.T) {
	cfg := DefaultVMConfig()

	if cfg.Machine == nil || cfg.Machine.Type != "q35" {
		t.Error("expected q35 machine")
	}
	if cfg.Machine.Accel != "kvm" {
		t.Error("expected kvm accel")
	}
	if cfg.CPU == nil || cfg.CPU.Model != "host" {
		t.Error("expected host cpu")
	}
	if cfg.Memory == nil || cfg.Memory.Size != 1024 {
		t.Error("expected 1024MB memory")
	}
	if !cfg.NoDefaults {
		t.Error("expected nodefaults")
	}
}

func TestVMConfigWithHelpers(t *testing.T) {
	cfg := &VMConfig{}

	// Test WithGuestAgent
	cfg.WithGuestAgent("/tmp/qga.sock")
	if len(cfg.Chardevs) != 1 {
		t.Error("expected 1 chardev")
	}
	if cfg.Chardevs[0].ID != "qga0" {
		t.Error("expected qga0 chardev")
	}
	if cfg.VirtioSerial == nil || len(cfg.VirtioSerial.Ports) != 1 {
		t.Error("expected virtio-serial port")
	}

	// Test WithUSBTablet
	cfg.WithUSBTablet()
	if cfg.USB == nil {
		t.Error("expected USB controller")
	}
	if len(cfg.USBDevices) != 1 {
		t.Error("expected 1 USB device")
	}
	if cfg.USBDevices[0].Type != "usb-tablet" {
		t.Error("expected usb-tablet")
	}

	// Test WithSpiceAgent
	cfg.WithSpiceAgent()
	if len(cfg.Chardevs) != 2 {
		t.Error("expected 2 chardevs")
	}
	if cfg.Chardevs[1].ID != "vdagent0" {
		t.Error("expected vdagent0 chardev")
	}
}

func TestBuildNetworkArgs(t *testing.T) {
	alloc := newPCISlotAllocator(true)

	cfg := &NetworkConfig{
		ID: "net0",
		Backend: &UserNetBackend{
			Hostfwd: []string{"tcp::22-:22"},
		},
		Model:     "virtio-net-pci",
		MACAddr:   "52:54:00:12:34:56",
		BootIndex: 2,
	}

	args := buildNetworkArgs(cfg, alloc)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "-netdev") {
		t.Error("expected -netdev")
	}
	if !strings.Contains(argsStr, "-device") {
		t.Error("expected -device")
	}
	if !strings.Contains(argsStr, "virtio-net-pci") {
		t.Error("expected virtio-net-pci")
	}
	if !strings.Contains(argsStr, "mac=52:54:00:12:34:56") {
		t.Error("expected mac address")
	}
	if !strings.Contains(argsStr, "bootindex=2") {
		t.Error("expected bootindex")
	}
	if !strings.Contains(argsStr, "bus=pcie.0") {
		t.Error("expected pcie bus")
	}
}

func TestBuildDiskArgs(t *testing.T) {
	alloc := newPCISlotAllocator(true)

	cfg := &DiskConfig{
		ID: "drive0",
		Backend: &FileDiskBackend{
			Path:   "/var/lib/qemu/disk.qcow2",
			Format: "qcow2",
		},
		Interface: "virtio",
		BootIndex: 1,
		Serial:    "DISK001",
	}

	args := buildDiskArgs(cfg, alloc)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "-blockdev") {
		t.Error("expected -blockdev")
	}
	if !strings.Contains(argsStr, "-device") {
		t.Error("expected -device")
	}
	if !strings.Contains(argsStr, "virtio-blk-pci") {
		t.Error("expected virtio-blk-pci")
	}
	if !strings.Contains(argsStr, "bootindex=1") {
		t.Error("expected bootindex")
	}
	if !strings.Contains(argsStr, "serial=DISK001") {
		t.Error("expected serial")
	}
}

func TestBuildDiskArgsWithThrottle(t *testing.T) {
	alloc := newPCISlotAllocator(true)

	cfg := &DiskConfig{
		ID: "drive0",
		Backend: &FileDiskBackend{
			Path:   "/var/lib/qemu/disk.qcow2",
			Format: "qcow2",
		},
		Interface: "virtio",
		Throttle: &ThrottleConfig{
			Group: "tg0",
			BPS:   100 * 1024 * 1024,
			IOPS:  1000,
		},
	}

	args := buildDiskArgs(cfg, alloc)
	argsStr := strings.Join(args, " ")

	// Should have throttle-group object
	if !strings.Contains(argsStr, "throttle-group") {
		t.Error("expected throttle-group object")
	}

	// Should have throttle blockdev layer
	if !strings.Contains(argsStr, `"driver":"throttle"`) {
		t.Error("expected throttle driver")
	}
}

func TestISCSIDiskBackend(t *testing.T) {
	backend := &ISCSIDiskBackend{
		Portal:         "192.168.1.100:3260",
		Target:         "iqn.2023-01.com.example:storage",
		Lun:            0,
		User:           "admin",
		PasswordSecret: "iscsi-password",
		InitiatorName:  "iqn.2023-01.com.example:client",
	}

	args := backend.BuildBlockdevArgs("drive0")

	// Parse the JSON to verify
	if len(args) < 2 {
		t.Fatal("expected at least 2 args")
	}

	var opts map[string]any
	if err := json.Unmarshal([]byte(args[1]), &opts); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if opts["driver"] != "iscsi" {
		t.Error("expected iscsi driver")
	}
	if opts["portal"] != "192.168.1.100:3260" {
		t.Error("expected portal")
	}
	if opts["target"] != "iqn.2023-01.com.example:storage" {
		t.Error("expected target")
	}
	if opts["user"] != "admin" {
		t.Error("expected user")
	}
	if opts["initiator-name"] != "iqn.2023-01.com.example:client" {
		t.Error("expected initiator-name")
	}
}
