package qemuctl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// VMConfig is a comprehensive VM configuration.
type VMConfig struct {
	// Name is the VM name.
	Name string

	// Arch is the target architecture (GOARCH-style).
	Arch string

	// QemuPath overrides the QEMU binary path.
	QemuPath string

	// SocketDir overrides the socket directory.
	SocketDir string

	// Machine configures the machine type.
	Machine *MachineConfig

	// CPU configures the virtual CPU.
	CPU *CPUConfig

	// Memory configures the memory.
	Memory *MemoryConfig

	// EFI configures UEFI firmware.
	EFI *EFIConfig

	// Boot configures boot options.
	Boot *BootConfig

	// Disks is the list of disk configurations.
	Disks []*DiskConfig

	// CDROMs is the list of CD-ROM drives.
	CDROMs []*CDROMConfig

	// Networks is the list of network configurations.
	Networks []*NetworkConfig

	// Display configures display output.
	Display *DisplayConfig

	// Audio configures audio device.
	Audio *AudioConfig

	// Serials is the list of serial port configurations.
	Serials []*SerialConfig

	// Chardevs is the list of character devices.
	Chardevs []*ChardevConfig

	// VirtioSerial configures virtio-serial.
	VirtioSerial *VirtioSerialConfig

	// USB configures USB controller.
	USB *USBControllerConfig

	// USBDevices is the list of USB devices.
	USBDevices []*USBDeviceConfig

	// Balloon configures memory balloon.
	Balloon *BalloonConfig

	// RTC configures real-time clock.
	RTC *RTCConfig

	// Secrets is the list of secret objects.
	Secrets []*SecretConfig

	// NoDefaults disables QEMU's default devices.
	NoDefaults bool

	// ExtraArgs are additional command-line arguments.
	ExtraArgs []string
}

// RTCConfig configures the real-time clock.
type RTCConfig struct {
	// Base is the RTC base ("utc", "localtime", datetime).
	Base string

	// Clock is the clock source ("host", "rt", "vm").
	Clock string

	// DriftFix is the drift fix mode ("slew", "none").
	DriftFix string
}

// VMBuilder builds QEMU command-line arguments from VMConfig.
type VMBuilder struct {
	config   *VMConfig
	pciAlloc *pciSlotAllocator
	args     []string
	isQ35    bool
}

// NewVMBuilder creates a new VM builder.
func NewVMBuilder(cfg *VMConfig) *VMBuilder {
	if cfg == nil {
		cfg = &VMConfig{}
	}

	// Determine if Q35
	isQ35 := false
	if cfg.Machine != nil && strings.HasPrefix(cfg.Machine.Type, "q35") {
		isQ35 = true
	} else if cfg.Machine == nil && (cfg.Arch == "" || cfg.Arch == "amd64" || cfg.Arch == "386") {
		// Default to Q35 for x86
		isQ35 = true
	}

	return &VMBuilder{
		config:   cfg,
		pciAlloc: newPCISlotAllocator(isQ35),
		isQ35:    isQ35,
	}
}

// Build builds the complete QEMU command-line arguments.
func (b *VMBuilder) Build(name, socketPath string) []string {
	b.args = nil

	// Name
	if name != "" {
		b.args = append(b.args, "-name", fmt.Sprintf("guest=%s,debug-threads=on", name))
	}

	// No defaults
	if b.config.NoDefaults {
		b.args = append(b.args, "-no-user-config", "-nodefaults")
	}

	// Build in order
	b.buildMachine()
	b.buildEFI()
	b.buildCPU()
	b.buildMemory()
	b.buildRTC()
	b.buildBoot()
	b.buildSecrets()
	b.buildDisplay()
	b.buildAudio()
	b.buildControlSocket(socketPath)
	b.buildSATAController()
	b.buildDisks()
	b.buildCDROMs()
	b.buildNetworks()
	b.buildVirtioSerial()
	b.buildSerials()
	b.buildChardevs()
	b.buildUSB()
	b.buildBalloon()
	b.buildMiscDevices()

	// Extra args
	b.args = append(b.args, b.config.ExtraArgs...)

	return b.args
}

// buildMachine builds machine arguments.
func (b *VMBuilder) buildMachine() {
	cfg := b.config.Machine
	if cfg == nil {
		// Default machine
		machineType := ""
		if b.config.Arch == "" || b.config.Arch == "amd64" || b.config.Arch == "386" {
			machineType = "q35"
		}
		if machineType != "" {
			b.args = append(b.args, "-machine", machineType+",accel=kvm")
		}
		return
	}

	args := buildMachineArgs(cfg)
	b.args = append(b.args, args...)
}

// buildEFI builds EFI/pflash arguments.
func (b *VMBuilder) buildEFI() {
	cfg := b.config.EFI
	if cfg == nil || cfg.Code == "" {
		return
	}

	// EFI code (pflash0) - read-only
	codeOpts := map[string]any{
		"driver":    "file",
		"filename":  cfg.Code,
		"node-name": "pflash0-file",
		"read-only": true,
	}
	codeJSON, _ := json.Marshal(codeOpts)
	b.args = append(b.args, "-blockdev", string(codeJSON))

	codeFormatOpts := map[string]any{
		"driver":    "raw",
		"file":      "pflash0-file",
		"node-name": "pflash0",
		"read-only": true,
	}
	codeFormatJSON, _ := json.Marshal(codeFormatOpts)
	b.args = append(b.args, "-blockdev", string(codeFormatJSON))

	// EFI vars (pflash1) - read-write
	if cfg.Vars != "" {
		varsOpts := map[string]any{
			"driver":    "file",
			"filename":  cfg.Vars,
			"node-name": "pflash1-file",
		}
		varsJSON, _ := json.Marshal(varsOpts)
		b.args = append(b.args, "-blockdev", string(varsJSON))

		varsFormatOpts := map[string]any{
			"driver":    "raw",
			"file":      "pflash1-file",
			"node-name": "pflash1",
		}
		varsFormatJSON, _ := json.Marshal(varsFormatOpts)
		b.args = append(b.args, "-blockdev", string(varsFormatJSON))
	}

	// Update machine config to reference pflash nodes
	// This is handled in buildMachine via Machine.Pflash0/Pflash1
}

// buildCPU builds CPU arguments.
func (b *VMBuilder) buildCPU() {
	cfg := b.config.CPU
	memSize := 512
	if b.config.Memory != nil && b.config.Memory.Size > 0 {
		memSize = b.config.Memory.Size
	}

	if cfg == nil {
		// Defaults
		b.args = append(b.args, "-cpu", "host")
		b.args = append(b.args, "-m", strconv.Itoa(memSize))
		return
	}

	args := buildCPUArgs(cfg, memSize)
	b.args = append(b.args, args...)
}

// buildMemory builds memory-specific arguments.
func (b *VMBuilder) buildMemory() {
	cfg := b.config.Memory
	if cfg == nil {
		return
	}

	// Memory backend
	if cfg.Backend != nil {
		var parts []string
		switch cfg.Backend.Type {
		case "file":
			parts = append(parts, "memory-backend-file")
			parts = append(parts, "id=mem0")
			parts = append(parts, fmt.Sprintf("size=%dM", cfg.Size))
			if cfg.Backend.Path != "" {
				parts = append(parts, "mem-path="+cfg.Backend.Path)
			}
			if cfg.Backend.Share {
				parts = append(parts, "share=on")
			}
			if cfg.Backend.Prealloc {
				parts = append(parts, "prealloc=on")
			}
		case "memfd":
			parts = append(parts, "memory-backend-memfd")
			parts = append(parts, "id=mem0")
			parts = append(parts, fmt.Sprintf("size=%dM", cfg.Size))
			if cfg.Backend.Share {
				parts = append(parts, "share=on")
			}
		}

		if len(parts) > 0 {
			b.args = append(b.args, "-object", strings.Join(parts, ","))
			b.args = append(b.args, "-numa", "node,memdev=mem0")
		}
	}

	// Memory locking
	if cfg.MemLock == "on" {
		b.args = append(b.args, "-overcommit", "mem-lock=on")
	}
}

// buildRTC builds RTC arguments.
func (b *VMBuilder) buildRTC() {
	cfg := b.config.RTC
	if cfg == nil {
		// Default RTC
		b.args = append(b.args, "-rtc", "base=utc,driftfix=slew")
		return
	}

	var parts []string
	if cfg.Base != "" {
		parts = append(parts, "base="+cfg.Base)
	}
	if cfg.Clock != "" {
		parts = append(parts, "clock="+cfg.Clock)
	}
	if cfg.DriftFix != "" {
		parts = append(parts, "driftfix="+cfg.DriftFix)
	}

	if len(parts) > 0 {
		b.args = append(b.args, "-rtc", strings.Join(parts, ","))
	}
}

// buildBoot builds boot arguments.
func (b *VMBuilder) buildBoot() {
	cfg := b.config.Boot
	if cfg == nil {
		return
	}

	// Boot order/menu
	if cfg.Order != "" || cfg.Menu != nil || cfg.Strict {
		var parts []string
		if cfg.Order != "" {
			parts = append(parts, "order="+cfg.Order)
		}
		if cfg.Menu != nil {
			if *cfg.Menu {
				parts = append(parts, "menu=on")
			} else {
				parts = append(parts, "menu=off")
			}
		}
		if cfg.Strict {
			parts = append(parts, "strict=on")
		}
		if len(parts) > 0 {
			b.args = append(b.args, "-boot", strings.Join(parts, ","))
		}
	}

	// Direct kernel boot
	if cfg.Kernel != "" {
		b.args = append(b.args, "-kernel", cfg.Kernel)
	}
	if cfg.Initrd != "" {
		b.args = append(b.args, "-initrd", cfg.Initrd)
	}
	if cfg.Append != "" {
		b.args = append(b.args, "-append", cfg.Append)
	}
}

// buildSecrets builds secret object arguments.
func (b *VMBuilder) buildSecrets() {
	for _, secret := range b.config.Secrets {
		args := buildSecretArgs(secret)
		b.args = append(b.args, args...)
	}
}

// buildDisplay builds display arguments.
func (b *VMBuilder) buildDisplay() {
	cfg := b.config.Display
	if cfg == nil {
		// Default: no display
		b.args = append(b.args, "-display", "none")
		return
	}

	// Display type
	switch cfg.Type {
	case "none":
		b.args = append(b.args, "-display", "none")
	case "gtk":
		b.args = append(b.args, "-display", "gtk")
	case "sdl":
		b.args = append(b.args, "-display", "sdl")
	case "vnc":
		if cfg.VNC != nil {
			args := buildVNCArgs(cfg.VNC)
			b.args = append(b.args, args...)
		}
		b.args = append(b.args, "-display", "none")
	case "spice":
		if cfg.Spice != nil {
			args := buildSpiceArgs(cfg.Spice)
			b.args = append(b.args, args...)
		}
		b.args = append(b.args, "-display", "none")
	default:
		b.args = append(b.args, "-display", "none")
	}

	// Video device
	if cfg.Video != nil {
		b.buildVideoDevice(cfg.Video)
	}
}

// buildVideoDevice builds video device arguments.
func (b *VMBuilder) buildVideoDevice(cfg *VideoConfig) {
	if cfg == nil || cfg.Type == "" {
		return
	}

	var parts []string
	parts = append(parts, cfg.Type)
	parts = append(parts, "id=video0")

	if cfg.VgaMem > 0 {
		parts = append(parts, fmt.Sprintf("vgamem_mb=%d", cfg.VgaMem))
	}
	if cfg.Ram > 0 {
		parts = append(parts, fmt.Sprintf("ram_size=%d", cfg.Ram))
	}
	if cfg.Vram > 0 {
		parts = append(parts, fmt.Sprintf("vram_size=%d", cfg.Vram))
	}
	if cfg.MaxOutputs > 0 {
		parts = append(parts, fmt.Sprintf("max_outputs=%d", cfg.MaxOutputs))
	}

	// Add PCI bus/addr for PCI video devices
	if cfg.Type == "qxl-vga" || cfg.Type == "virtio-vga" || cfg.Type == "vga" {
		parts = append(parts, "bus="+b.pciAlloc.Bus())
		parts = append(parts, "addr="+b.pciAlloc.Alloc())
	}

	b.args = append(b.args, "-device", strings.Join(parts, ","))
}

// buildAudio builds audio device arguments.
func (b *VMBuilder) buildAudio() {
	cfg := b.config.Audio
	if cfg == nil {
		return
	}

	// Audio backend
	if cfg.Backend != "" && cfg.Backend != "none" {
		var audioParts []string
		audioParts = append(audioParts, fmt.Sprintf("%s,id=audio0", cfg.Backend))
		b.args = append(b.args, "-audiodev", strings.Join(audioParts, ","))
	}

	// Audio device
	if cfg.Device != "" {
		var deviceParts []string
		deviceParts = append(deviceParts, cfg.Device)
		deviceParts = append(deviceParts, "id=sound0")
		deviceParts = append(deviceParts, "bus="+b.pciAlloc.Bus())
		deviceParts = append(deviceParts, "addr="+b.pciAlloc.Alloc())

		b.args = append(b.args, "-device", strings.Join(deviceParts, ","))

		// Codec for HDA devices
		if cfg.Codec != "" && (cfg.Device == "intel-hda" || cfg.Device == "ich9-intel-hda") {
			var codecParts []string
			codecParts = append(codecParts, cfg.Codec)
			codecParts = append(codecParts, "bus=sound0.0")
			if cfg.Backend != "" && cfg.Backend != "none" {
				codecParts = append(codecParts, "audiodev=audio0")
			}
			b.args = append(b.args, "-device", strings.Join(codecParts, ","))
		}
	}
}

// buildControlSocket builds QMP control socket arguments.
func (b *VMBuilder) buildControlSocket(socketPath string) {
	if socketPath == "" {
		return
	}

	b.args = append(b.args, "-chardev",
		fmt.Sprintf("socket,id=qmp,path=%s,server=on,wait=off", socketPath))
	b.args = append(b.args, "-mon", "chardev=qmp,id=monitor,mode=control")
}

// buildSATAController builds SATA controller for CD-ROMs.
func (b *VMBuilder) buildSATAController() {
	// Only add SATA controller if we have CD-ROMs
	if len(b.config.CDROMs) == 0 {
		return
	}

	if b.isQ35 {
		// Q35 has ICH9 AHCI built-in, but we add it explicitly for control
		b.args = append(b.args, "-device",
			fmt.Sprintf("ich9-ahci,id=sata0,bus=%s,addr=%s",
				b.pciAlloc.Bus(), b.pciAlloc.Alloc()))
	} else {
		// For i440FX, use PIIX4 IDE or ahci
		b.args = append(b.args, "-device",
			fmt.Sprintf("ahci,id=sata0,bus=%s,addr=%s",
				b.pciAlloc.Bus(), b.pciAlloc.Alloc()))
	}
}

// buildDisks builds disk device arguments.
func (b *VMBuilder) buildDisks() {
	for _, disk := range b.config.Disks {
		args := buildDiskArgs(disk, b.pciAlloc)
		b.args = append(b.args, args...)
	}
}

// buildCDROMs builds CD-ROM drive arguments.
func (b *VMBuilder) buildCDROMs() {
	for i, cdrom := range b.config.CDROMs {
		args := buildCDROMArgs(cdrom, i, "sata0")
		b.args = append(b.args, args...)
	}
}

// buildNetworks builds network device arguments.
func (b *VMBuilder) buildNetworks() {
	for _, net := range b.config.Networks {
		args := buildNetworkArgs(net, b.pciAlloc)
		b.args = append(b.args, args...)
	}
}

// buildVirtioSerial builds virtio-serial controller and ports.
func (b *VMBuilder) buildVirtioSerial() {
	cfg := b.config.VirtioSerial
	if cfg == nil && len(b.config.Chardevs) == 0 {
		return
	}

	// Add virtio-serial controller
	var controllerParts []string
	controllerParts = append(controllerParts, "virtio-serial-pci")
	controllerParts = append(controllerParts, "id=virtio-serial0")
	controllerParts = append(controllerParts, "bus="+b.pciAlloc.Bus())
	controllerParts = append(controllerParts, "addr="+b.pciAlloc.Alloc())

	if cfg != nil && cfg.MaxPorts > 0 {
		controllerParts = append(controllerParts, fmt.Sprintf("max_ports=%d", cfg.MaxPorts))
	}

	b.args = append(b.args, "-device", strings.Join(controllerParts, ","))

	// Add ports
	if cfg != nil {
		for _, port := range cfg.Ports {
			portType := port.Type
			if portType == "" {
				portType = "virtserialport"
			}

			var portParts []string
			portParts = append(portParts, portType)
			portParts = append(portParts, "bus=virtio-serial0.0")
			if port.Chardev != "" {
				portParts = append(portParts, "chardev="+port.Chardev)
			}
			if port.Name != "" {
				portParts = append(portParts, "name="+port.Name)
			}

			b.args = append(b.args, "-device", strings.Join(portParts, ","))
		}
	}
}

// buildSerials builds serial port arguments.
func (b *VMBuilder) buildSerials() {
	for i, serial := range b.config.Serials {
		chardevID := fmt.Sprintf("serial%d", i)

		// Build chardev
		var chardevParts []string
		chardevParts = append(chardevParts, serial.Type)
		chardevParts = append(chardevParts, "id="+chardevID)

		if serial.Path != "" {
			chardevParts = append(chardevParts, "path="+serial.Path)
		}
		if serial.Server {
			chardevParts = append(chardevParts, "server=on")
		}
		if !serial.Wait {
			chardevParts = append(chardevParts, "wait=off")
		}

		b.args = append(b.args, "-chardev", strings.Join(chardevParts, ","))

		// Build device
		device := serial.Device
		if device == "" {
			device = "isa-serial"
		}

		b.args = append(b.args, "-device",
			fmt.Sprintf("%s,chardev=%s,id=%s-device", device, chardevID, chardevID))
	}
}

// buildChardevs builds character device arguments.
func (b *VMBuilder) buildChardevs() {
	for _, chardev := range b.config.Chardevs {
		args := buildChardevArgs(chardev)
		b.args = append(b.args, args...)
	}
}

// buildUSB builds USB controller and device arguments.
func (b *VMBuilder) buildUSB() {
	cfg := b.config.USB
	if cfg == nil && len(b.config.USBDevices) == 0 {
		return
	}

	// Add USB controller
	controllerType := "qemu-xhci"
	if cfg != nil && cfg.Type != "" {
		controllerType = cfg.Type
	}

	b.args = append(b.args, "-device",
		fmt.Sprintf("%s,id=usb0,bus=%s,addr=%s",
			controllerType, b.pciAlloc.Bus(), b.pciAlloc.Alloc()))

	// Add USB devices
	for i, device := range b.config.USBDevices {
		var parts []string
		parts = append(parts, device.Type)
		parts = append(parts, fmt.Sprintf("id=usb-dev%d", i))
		parts = append(parts, "bus=usb0.0")

		if device.Chardev != "" {
			parts = append(parts, "chardev="+device.Chardev)
		}

		b.args = append(b.args, "-device", strings.Join(parts, ","))
	}
}

// buildBalloon builds memory balloon device arguments.
func (b *VMBuilder) buildBalloon() {
	cfg := b.config.Balloon
	if cfg == nil || !cfg.Enabled {
		return
	}

	b.args = append(b.args, "-device",
		fmt.Sprintf("virtio-balloon-pci,id=balloon0,bus=%s,addr=%s",
			b.pciAlloc.Bus(), b.pciAlloc.Alloc()))
}

// buildMiscDevices builds miscellaneous devices (RNG, etc.).
func (b *VMBuilder) buildMiscDevices() {
	// Always add virtio-rng for entropy
	b.args = append(b.args, "-object", "rng-random,id=rng0,filename=/dev/urandom")
	b.args = append(b.args, "-device",
		fmt.Sprintf("virtio-rng-pci,rng=rng0,id=rng-dev0,bus=%s,addr=%s",
			b.pciAlloc.Bus(), b.pciAlloc.Alloc()))
}

// ToConfig converts VMConfig to the simpler Config for Start().
func (cfg *VMConfig) ToConfig() *Config {
	c := &Config{
		Name:      cfg.Name,
		Arch:      cfg.Arch,
		QemuPath:  cfg.QemuPath,
		SocketDir: cfg.SocketDir,
		ExtraArgs: cfg.ExtraArgs,
	}

	if cfg.Memory != nil {
		c.Memory = cfg.Memory.Size
	} else {
		c.Memory = 512
	}

	if cfg.CPU != nil {
		c.CPUs = cfg.CPU.Sockets * cfg.CPU.Cores * cfg.CPU.Threads
		if c.CPUs == 0 {
			c.CPUs = 1
		}
		c.CPU = cfg.CPU.Model
	} else {
		c.CPUs = 1
	}

	if cfg.Machine != nil {
		c.Machine = cfg.Machine.Type
		if cfg.Machine.Accel == "kvm" {
			kvm := true
			c.KVM = &kvm
		}
	}

	noDefaults := cfg.NoDefaults
	c.NoDefaults = &noDefaults

	return c
}

// StartVM launches a QEMU instance using VMConfig.
func StartVM(cfg *VMConfig) (*Instance, error) {
	return StartVMContext(context.Background(), cfg)
}

// StartVMContext launches a QEMU instance with context support.
func StartVMContext(ctx context.Context, cfg *VMConfig) (*Instance, error) {
	if cfg == nil {
		cfg = DefaultVMConfig()
	}

	// Generate name if not provided
	name := cfg.Name
	if name == "" {
		name = generateName()
	}

	// Locate QEMU binary
	qemuPath, err := LocateQemu(cfg.Arch, cfg.QemuPath)
	if err != nil {
		return nil, err
	}

	// Ensure socket directory exists
	socketDir, err := ensureSocketDirFromCfg(cfg)
	if err != nil {
		return nil, err
	}

	socketPath := filepath.Join(socketDir, name+".sock")

	// Remove stale socket
	os.Remove(socketPath)

	// Build command line using VMBuilder
	builder := NewVMBuilder(cfg)
	args := builder.Build(name, socketPath)

	// Create and start process
	cmd := exec.CommandContext(ctx, qemuPath, args...)
	cmd.Dir = "/"
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Set process group so we can kill all children
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start QEMU: %w", err)
	}

	inst := &Instance{
		name:       name,
		process:    cmd.Process,
		socketPath: socketPath,
		state:      StatePrelaunch,
	}

	// Wait for socket to be available
	if err := waitForSocket(ctx, socketPath, 10*time.Second); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("QEMU failed to create socket: %w", err)
	}

	// Connect QMP
	qmp, err := newQMP(socketPath)
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("failed to connect QMP: %w", err)
	}

	inst.qmp = qmp
	qmp.SetStateChangeCallback(func(s State) {
		inst.setState(s)
	})

	// Query initial state
	if err := inst.QueryState(); err != nil {
		// Non-fatal, state will be updated via events
	}

	return inst, nil
}

// ensureSocketDirFromCfg creates the socket directory from VMConfig.
func ensureSocketDirFromCfg(cfg *VMConfig) (string, error) {
	dir := cfg.SocketDir
	if dir == "" {
		var err error
		dir, err = defaultSocketDir()
		if err != nil {
			return "", err
		}
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create socket directory: %w", err)
	}

	return dir, nil
}

// WithGuestAgent adds a guest agent chardev and virtio-serial port.
func (cfg *VMConfig) WithGuestAgent(socketPath string) *VMConfig {
	// Add chardev for guest agent
	cfg.Chardevs = append(cfg.Chardevs, &ChardevConfig{
		ID:      "qga0",
		Backend: "socket",
		Path:    socketPath,
		Server:  true,
		Wait:    false,
	})

	// Add virtio-serial port for guest agent
	if cfg.VirtioSerial == nil {
		cfg.VirtioSerial = &VirtioSerialConfig{}
	}
	cfg.VirtioSerial.Ports = append(cfg.VirtioSerial.Ports, VirtioSerialPortConfig{
		Chardev: "qga0",
		Name:    "org.qemu.guest_agent.0",
		Type:    "virtserialport",
	})

	return cfg
}

// WithSpiceAgent adds a SPICE agent chardev and virtio-serial port.
func (cfg *VMConfig) WithSpiceAgent() *VMConfig {
	// Add chardev for SPICE agent
	cfg.Chardevs = append(cfg.Chardevs, &ChardevConfig{
		ID:      "vdagent0",
		Backend: "spicevmc",
		Name:    "vdagent",
	})

	// Add virtio-serial port for SPICE agent
	if cfg.VirtioSerial == nil {
		cfg.VirtioSerial = &VirtioSerialConfig{}
	}
	cfg.VirtioSerial.Ports = append(cfg.VirtioSerial.Ports, VirtioSerialPortConfig{
		Chardev: "vdagent0",
		Name:    "com.redhat.spice.0",
		Type:    "virtserialport",
	})

	return cfg
}

// WithUSBTablet adds a USB tablet device for better mouse handling.
func (cfg *VMConfig) WithUSBTablet() *VMConfig {
	if cfg.USB == nil {
		cfg.USB = &USBControllerConfig{Type: "qemu-xhci"}
	}
	cfg.USBDevices = append(cfg.USBDevices, &USBDeviceConfig{
		Type: "usb-tablet",
	})
	return cfg
}

// DefaultVMConfig returns a VMConfig with sensible defaults.
func DefaultVMConfig() *VMConfig {
	return &VMConfig{
		Machine: &MachineConfig{
			Type:  "q35",
			Accel: "kvm",
		},
		CPU: &CPUConfig{
			Model:   "host",
			Sockets: 1,
			Cores:   1,
			Threads: 1,
		},
		Memory: &MemoryConfig{
			Size: 1024,
		},
		Display: &DisplayConfig{
			Type: "none",
		},
		NoDefaults: true,
	}
}
