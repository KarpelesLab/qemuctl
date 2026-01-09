package qemuctl

import (
	"fmt"
	"strconv"
	"strings"
)

// MachineConfig configures the virtual machine type.
type MachineConfig struct {
	// Type is the machine type (e.g., "q35", "pc", "virt").
	Type string

	// Accel is the accelerator (e.g., "kvm", "tcg", "hvf").
	Accel string

	// USB enables/disables USB controller creation by machine.
	USB *bool

	// DumpGuestCore enables/disables guest core dumps.
	DumpGuestCore *bool

	// Pflash0 is the node name for pflash0 (EFI code).
	Pflash0 string

	// Pflash1 is the node name for pflash1 (EFI vars).
	Pflash1 string
}

// CPUConfig configures the virtual CPU.
type CPUConfig struct {
	// Model is the CPU model (e.g., "host", "qemu64", "max").
	Model string

	// Features are CPU feature flags (e.g., "+ssse3", "-svm").
	Features []string

	// Sockets is the number of CPU sockets.
	Sockets int

	// Cores is the number of cores per socket.
	Cores int

	// Threads is the number of threads per core.
	Threads int
}

// MemoryConfig configures the virtual machine memory.
type MemoryConfig struct {
	// Size is the memory size in megabytes.
	Size int

	// Backend configures memory backend (optional).
	Backend *MemoryBackendConfig

	// MemLock controls memory locking ("on", "off").
	MemLock string
}

// MemoryBackendConfig configures a memory backend.
type MemoryBackendConfig struct {
	// Type is the backend type ("file", "memfd", "ram").
	Type string

	// Path is the file path for file-backed memory.
	Path string

	// Share enables memory sharing with other processes.
	Share bool

	// Prealloc preallocates memory.
	Prealloc bool
}

// EFIConfig configures UEFI firmware.
type EFIConfig struct {
	// Code is the path to the EFI code (pflash0).
	Code string

	// Vars is the path to the EFI variables file (pflash1).
	Vars string

	// VarsTemplate is the template for creating new vars file.
	VarsTemplate string
}

// BootConfig configures boot options.
type BootConfig struct {
	// Order is the boot order (e.g., "cdn" for cdrom, disk, network).
	Order string

	// Menu enables/disables boot menu.
	Menu *bool

	// Strict enforces boot order.
	Strict bool

	// Kernel is the path to a kernel image for direct boot.
	Kernel string

	// Initrd is the path to an initrd image.
	Initrd string

	// Append is the kernel command line.
	Append string
}

// DisplayConfig configures display output.
type DisplayConfig struct {
	// Type is the display type ("none", "vnc", "spice", "gtk", "sdl").
	Type string

	// VNC configures VNC display.
	VNC *VNCConfig

	// Spice configures SPICE display.
	Spice *SpiceDisplayConfig

	// Video configures video device.
	Video *VideoConfig
}

// VNCConfig configures VNC display.
type VNCConfig struct {
	// Listen is the listen address (e.g., ":0", "127.0.0.1:5900", "none").
	// Use "none" for add_client mode.
	Listen string

	// Password is the VNC password (use PasswordSecret for security).
	Password string

	// PasswordSecret is the secret ID for password.
	PasswordSecret string

	// Lossy enables lossy compression.
	Lossy bool

	// AudioDev is the audio device ID for VNC audio.
	AudioDev string

	// Websocket enables websocket support on given port.
	Websocket int
}

// SpiceDisplayConfig configures SPICE display.
type SpiceDisplayConfig struct {
	// Unix uses Unix socket instead of TCP.
	Unix bool

	// Port is the TCP port (0 for none).
	Port int

	// Password is the SPICE password.
	Password string

	// PasswordSecret is the secret ID for password.
	PasswordSecret string

	// DisableTicketing disables password authentication.
	DisableTicketing bool

	// ImageCompression sets image compression mode.
	ImageCompression string

	// JpegWanCompression sets JPEG WAN compression.
	JpegWanCompression string

	// ZlibGlzWanCompression sets zlib-glz WAN compression.
	ZlibGlzWanCompression string

	// PlaybackCompression enables audio playback compression.
	PlaybackCompression bool

	// SeamlessMigration enables seamless migration.
	SeamlessMigration bool

	// DisableCopyPaste disables copy-paste.
	DisableCopyPaste bool
}

// VideoConfig configures video device.
type VideoConfig struct {
	// Type is the video device type ("qxl-vga", "virtio-vga", "vga", "cirrus").
	Type string

	// VgaMem is VGA memory size in MB.
	VgaMem int

	// Ram is video RAM size in bytes (for QXL).
	Ram int

	// Vram is video VRAM size in bytes (for QXL).
	Vram int

	// MaxOutputs is maximum number of outputs.
	MaxOutputs int
}

// AudioConfig configures audio device.
type AudioConfig struct {
	// Backend is the audio backend ("spice", "pa", "alsa", "none").
	Backend string

	// Device is the sound device type ("intel-hda", "ich9-intel-hda", "ac97").
	Device string

	// Codec is the codec for HDA devices ("hda-micro", "hda-duplex", "hda-output").
	Codec string
}

// SerialConfig configures a serial port.
type SerialConfig struct {
	// Type is the chardev type ("socket", "pty", "file", "pipe").
	Type string

	// Path is the socket/file/pipe path.
	Path string

	// Server makes the socket a server.
	Server bool

	// Wait waits for client connection.
	Wait bool

	// Device is the serial device type ("isa-serial", "usb-serial", "virtio-serial").
	Device string
}

// ChardevConfig configures a character device.
type ChardevConfig struct {
	// ID is the chardev ID.
	ID string

	// Backend is the chardev backend type.
	Backend string

	// Path is the socket/file path.
	Path string

	// Server makes socket a server.
	Server bool

	// Wait waits for connection.
	Wait bool

	// Host is the TCP host.
	Host string

	// Port is the TCP port.
	Port int

	// Reconnect is the reconnect interval in seconds.
	Reconnect int

	// Name is the spicevmc channel name.
	Name string
}

// VirtioSerialConfig configures virtio-serial device.
type VirtioSerialConfig struct {
	// MaxPorts is maximum number of ports.
	MaxPorts int

	// Ports is the list of serial ports.
	Ports []VirtioSerialPortConfig
}

// VirtioSerialPortConfig configures a virtio-serial port.
type VirtioSerialPortConfig struct {
	// Chardev is the chardev ID.
	Chardev string

	// Name is the port name (e.g., "org.qemu.guest_agent.0").
	Name string

	// Type is the port type ("virtserialport", "virtconsole").
	Type string
}

// USBControllerConfig configures USB controller.
type USBControllerConfig struct {
	// Type is the controller type ("qemu-xhci", "nec-usb-xhci", "ich9-usb-uhci1").
	Type string
}

// USBDeviceConfig configures a USB device.
type USBDeviceConfig struct {
	// Type is the device type ("usb-tablet", "usb-mouse", "usb-kbd", "usb-redir").
	Type string

	// Chardev is the chardev ID (for usb-redir, usb-serial).
	Chardev string
}

// BalloonConfig configures memory balloon device.
type BalloonConfig struct {
	// Enabled enables/disables balloon device.
	Enabled bool
}

// SecretConfig configures a secret object.
type SecretConfig struct {
	// ID is the secret ID.
	ID string

	// Data is the raw secret data.
	Data string

	// File is the path to secret file.
	File string

	// Format is the secret format ("raw", "base64").
	Format string
}

// pciSlotAllocator manages PCI slot allocation.
type pciSlotAllocator struct {
	nextSlot int
	used     map[int]bool
	bus      string
}

// newPCISlotAllocator creates a new PCI slot allocator.
func newPCISlotAllocator(isQ35 bool) *pciSlotAllocator {
	a := &pciSlotAllocator{
		nextSlot: 0x3,
		used:     make(map[int]bool),
	}

	// Reserve slot 0 for root complex
	a.used[0] = true

	if isQ35 {
		// For Q35, reserve slots 1 and 2 for pcie-root-ports
		a.used[0x1] = true
		a.used[0x2] = true
		a.bus = "pcie.0"
	} else {
		a.bus = "pci.0"
	}

	return a
}

// Alloc returns the next available PCI slot address.
func (a *pciSlotAllocator) Alloc() string {
	for a.used[a.nextSlot] {
		a.nextSlot++
		if a.nextSlot > 0x1f {
			panic("ran out of PCI slots")
		}
	}
	slot := a.nextSlot
	a.used[slot] = true
	a.nextSlot++
	return fmt.Sprintf("0x%x", slot)
}

// Reserve marks a specific slot as used.
func (a *pciSlotAllocator) Reserve(slot int) {
	a.used[slot] = true
}

// Bus returns the PCI bus name.
func (a *pciSlotAllocator) Bus() string {
	return a.bus
}

// buildMachineArgs builds machine-related arguments.
func buildMachineArgs(cfg *MachineConfig) []string {
	if cfg == nil {
		return nil
	}

	var parts []string
	if cfg.Type != "" {
		parts = append(parts, cfg.Type)
	}
	if cfg.Accel != "" {
		parts = append(parts, "accel="+cfg.Accel)
	}
	if cfg.USB != nil {
		if *cfg.USB {
			parts = append(parts, "usb=on")
		} else {
			parts = append(parts, "usb=off")
		}
	}
	if cfg.DumpGuestCore != nil {
		if *cfg.DumpGuestCore {
			parts = append(parts, "dump-guest-core=on")
		} else {
			parts = append(parts, "dump-guest-core=off")
		}
	}
	if cfg.Pflash0 != "" {
		parts = append(parts, "pflash0="+cfg.Pflash0)
	}
	if cfg.Pflash1 != "" {
		parts = append(parts, "pflash1="+cfg.Pflash1)
	}

	if len(parts) == 0 {
		return nil
	}

	return []string{"-machine", strings.Join(parts, ",")}
}

// buildCPUArgs builds CPU-related arguments.
func buildCPUArgs(cfg *CPUConfig, memory int) []string {
	if cfg == nil {
		return nil
	}

	var args []string

	// CPU model and features
	if cfg.Model != "" {
		cpuArg := cfg.Model
		for _, f := range cfg.Features {
			cpuArg += "," + f
		}
		args = append(args, "-cpu", cpuArg)
	}

	// Memory
	if memory > 0 {
		args = append(args, "-m", strconv.Itoa(memory))
	}

	// SMP
	sockets := cfg.Sockets
	if sockets == 0 {
		sockets = 1
	}
	cores := cfg.Cores
	if cores == 0 {
		cores = 1
	}
	threads := cfg.Threads
	if threads == 0 {
		threads = 1
	}
	total := sockets * cores * threads
	if total > 1 {
		args = append(args, "-smp", fmt.Sprintf("%d,sockets=%d,cores=%d,threads=%d",
			total, sockets, cores, threads))
	}

	return args
}

// buildVNCArgs builds VNC display arguments.
func buildVNCArgs(cfg *VNCConfig) []string {
	if cfg == nil {
		return nil
	}

	var parts []string

	listen := cfg.Listen
	if listen == "" {
		listen = "none"
	}
	parts = append(parts, listen)

	if cfg.PasswordSecret != "" {
		parts = append(parts, "password-secret="+cfg.PasswordSecret)
	} else if cfg.Password != "" {
		parts = append(parts, "password=on")
	}

	if cfg.Lossy {
		parts = append(parts, "lossy=on")
	}

	if cfg.AudioDev != "" {
		parts = append(parts, "audiodev="+cfg.AudioDev)
	}

	if cfg.Websocket > 0 {
		parts = append(parts, fmt.Sprintf("websocket=%d", cfg.Websocket))
	}

	return []string{"-vnc", strings.Join(parts, ",")}
}

// buildSpiceArgs builds SPICE display arguments.
func buildSpiceArgs(cfg *SpiceDisplayConfig) []string {
	if cfg == nil {
		return nil
	}

	var parts []string

	if cfg.Unix {
		parts = append(parts, "unix=on")
	} else if cfg.Port > 0 {
		parts = append(parts, fmt.Sprintf("port=%d", cfg.Port))
	}

	if cfg.PasswordSecret != "" {
		parts = append(parts, "password-secret="+cfg.PasswordSecret)
	}

	if cfg.DisableTicketing {
		parts = append(parts, "disable-ticketing=on")
	}

	if cfg.ImageCompression != "" {
		parts = append(parts, "image-compression="+cfg.ImageCompression)
	}

	if cfg.JpegWanCompression != "" {
		parts = append(parts, "jpeg-wan-compression="+cfg.JpegWanCompression)
	}

	if cfg.ZlibGlzWanCompression != "" {
		parts = append(parts, "zlib-glz-wan-compression="+cfg.ZlibGlzWanCompression)
	}

	if cfg.PlaybackCompression {
		parts = append(parts, "playback-compression=on")
	}

	if cfg.SeamlessMigration {
		parts = append(parts, "seamless-migration=on")
	}

	if cfg.DisableCopyPaste {
		parts = append(parts, "disable-copy-paste=on")
	} else {
		parts = append(parts, "disable-copy-paste=off")
	}

	if len(parts) == 0 {
		return nil
	}

	return []string{"-spice", strings.Join(parts, ",")}
}

// buildChardevArgs builds chardev arguments.
func buildChardevArgs(cfg *ChardevConfig) []string {
	if cfg == nil || cfg.ID == "" {
		return nil
	}

	var parts []string
	parts = append(parts, cfg.Backend)
	parts = append(parts, "id="+cfg.ID)

	if cfg.Path != "" {
		parts = append(parts, "path="+cfg.Path)
	}
	if cfg.Host != "" {
		parts = append(parts, "host="+cfg.Host)
	}
	if cfg.Port > 0 {
		parts = append(parts, fmt.Sprintf("port=%d", cfg.Port))
	}
	if cfg.Server {
		parts = append(parts, "server=on")
	}
	if !cfg.Wait {
		parts = append(parts, "wait=off")
	}
	if cfg.Reconnect > 0 {
		parts = append(parts, fmt.Sprintf("reconnect=%d", cfg.Reconnect))
	}
	if cfg.Name != "" {
		parts = append(parts, "name="+cfg.Name)
	}

	return []string{"-chardev", strings.Join(parts, ",")}
}

// buildSecretArgs builds secret object arguments.
func buildSecretArgs(cfg *SecretConfig) []string {
	if cfg == nil || cfg.ID == "" {
		return nil
	}

	var parts []string
	parts = append(parts, "secret")
	parts = append(parts, "id="+cfg.ID)

	if cfg.Data != "" {
		parts = append(parts, "data="+cfg.Data)
	}
	if cfg.File != "" {
		parts = append(parts, "file="+cfg.File)
	}
	if cfg.Format != "" {
		parts = append(parts, "format="+cfg.Format)
	}

	return []string{"-object", strings.Join(parts, ",")}
}
