package qemuctl

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the configuration for launching a QEMU instance.
type Config struct {
	// Name is a unique identifier for this instance.
	// If empty, a random name will be generated.
	Name string

	// Arch is the target architecture (GOARCH-style, e.g., "amd64", "arm64").
	// Defaults to runtime.GOARCH if empty.
	Arch string

	// QemuPath is the path to the QEMU binary.
	// If empty, it will be located automatically.
	QemuPath string

	// SocketDir is the directory for control sockets.
	// If empty, defaults to os.UserCacheDir()/qemuctl for users
	// or /var/run/qemu for root.
	SocketDir string

	// Memory is the amount of memory in megabytes.
	// Defaults to 512 if not set.
	Memory int

	// CPUs is the number of virtual CPUs.
	// Defaults to 1 if not set.
	CPUs int

	// Machine is the machine type (e.g., "q35", "pc", "virt").
	// If empty, QEMU's default for the architecture is used.
	Machine string

	// CPU is the CPU model (e.g., "host", "qemu64").
	// Defaults to "host" if KVM is available.
	CPU string

	// KVM enables KVM acceleration if available.
	// Defaults to true.
	KVM *bool

	// Drives is a list of drive configurations.
	Drives []DriveConfig

	// NetworkDevices is a list of network device configurations.
	NetworkDevices []NetDevConfig

	// VNC enables VNC display.
	// Use "none" for no listening socket (clients via add_client).
	VNC string

	// Spice enables SPICE display.
	Spice *SpiceConfig

	// ExtraArgs are additional command-line arguments.
	ExtraArgs []string

	// NoDefaults disables QEMU's default devices.
	// Defaults to true.
	NoDefaults *bool

	// Daemonize runs QEMU in the background.
	// Note: This is handled by the library, not QEMU's -daemonize.
	Daemonize bool
}

// DriveConfig configures a disk drive.
type DriveConfig struct {
	// File is the path to the disk image.
	File string

	// Format is the disk format (e.g., "qcow2", "raw").
	Format string

	// Interface is the disk interface (e.g., "virtio", "ide", "scsi").
	// Defaults to "virtio".
	Interface string

	// ReadOnly makes the drive read-only.
	ReadOnly bool

	// Cache is the caching mode (e.g., "none", "writeback").
	Cache string

	// Serial is the drive's serial number.
	Serial string
}

// NetDevConfig configures a network device.
type NetDevConfig struct {
	// Type is the network type (e.g., "user", "tap", "socket").
	Type string

	// ID is the unique identifier for this network device.
	ID string

	// Model is the NIC model (e.g., "virtio", "e1000", "rtl8139").
	// Defaults to "virtio".
	Model string

	// MACAddr is the MAC address.
	MACAddr string

	// Bridge is the bridge to connect to (for tap devices).
	Bridge string

	// Script is the interface up script path (for tap devices).
	// Use "no" to disable.
	Script string

	// DownScript is the interface down script path.
	// Use "no" to disable.
	DownScript string

	// SocketPath is the Unix socket path (for socket devices).
	SocketPath string
}

// SpiceConfig configures SPICE display.
type SpiceConfig struct {
	// UnixSocket uses a Unix socket instead of TCP.
	UnixSocket bool

	// Password is the SPICE password.
	Password string

	// DisableTicketing disables password authentication.
	DisableTicketing bool

	// ImageCompression sets the image compression mode.
	ImageCompression string

	// PlaybackCompression enables audio playback compression.
	PlaybackCompression bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	kvm := true
	noDefaults := true

	return &Config{
		Memory:     512,
		CPUs:       1,
		KVM:        &kvm,
		NoDefaults: &noDefaults,
	}
}

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() error {
	if c.Memory <= 0 {
		return fmt.Errorf("memory must be positive")
	}
	if c.CPUs <= 0 {
		return fmt.Errorf("CPUs must be positive")
	}
	return nil
}

// socketDir returns the directory for control sockets.
func (c *Config) socketDir() (string, error) {
	if c.SocketDir != "" {
		return c.SocketDir, nil
	}
	return defaultSocketDir()
}

// defaultSocketDir returns the default socket directory based on UID.
func defaultSocketDir() (string, error) {
	if os.Getuid() == 0 {
		return "/var/run/qemu", nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user cache dir: %w", err)
	}
	return filepath.Join(cacheDir, "qemuctl"), nil
}

// ensureSocketDir creates the socket directory if it doesn't exist.
func (c *Config) ensureSocketDir() (string, error) {
	dir, err := c.socketDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create socket directory: %w", err)
	}

	return dir, nil
}
