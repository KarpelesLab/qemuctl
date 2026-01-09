package qemuctl

import (
	"fmt"
	"strings"
)

// NetworkConfig configures a network device.
type NetworkConfig struct {
	// ID is the netdev/device ID.
	ID string

	// Backend configures the network backend.
	Backend NetworkBackend

	// Model is the NIC model ("virtio-net-pci", "e1000", "rtl8139").
	Model string

	// MACAddr is the MAC address.
	MACAddr string

	// BootIndex sets the boot priority for network boot.
	BootIndex int
}

// NetworkBackend is the interface for network backends.
type NetworkBackend interface {
	// Type returns the backend type name.
	Type() string

	// BuildNetdevArgs builds the netdev arguments.
	BuildNetdevArgs(id string) []string
}

// UserNetBackend provides user-mode networking (NAT).
type UserNetBackend struct {
	// Hostfwd configures port forwarding (e.g., "tcp::2222-:22").
	Hostfwd []string

	// Net is the guest network (e.g., "10.0.2.0/24").
	Net string

	// Host is the host address in guest network.
	Host string

	// DNS is the DNS server address.
	DNS string

	// DHCPStart is the first DHCP address.
	DHCPStart string

	// Restrict isolates guest from host.
	Restrict bool
}

func (u *UserNetBackend) Type() string { return "user" }

func (u *UserNetBackend) BuildNetdevArgs(id string) []string {
	var parts []string
	parts = append(parts, "user")
	parts = append(parts, "id="+id)

	if u.Net != "" {
		parts = append(parts, "net="+u.Net)
	}
	if u.Host != "" {
		parts = append(parts, "host="+u.Host)
	}
	if u.DNS != "" {
		parts = append(parts, "dns="+u.DNS)
	}
	if u.DHCPStart != "" {
		parts = append(parts, "dhcpstart="+u.DHCPStart)
	}
	if u.Restrict {
		parts = append(parts, "restrict=on")
	}
	for _, fwd := range u.Hostfwd {
		parts = append(parts, "hostfwd="+fwd)
	}

	return []string{"-netdev", strings.Join(parts, ",")}
}

// TapNetBackend provides TAP device networking.
type TapNetBackend struct {
	// Ifname is the TAP interface name.
	Ifname string

	// Bridge is the bridge to attach to.
	Bridge string

	// Script is the interface up script ("no" to disable).
	Script string

	// DownScript is the interface down script ("no" to disable).
	DownScript string

	// VHost enables vhost-net acceleration.
	VHost bool

	// Queues is the number of queues (for multiqueue).
	Queues int

	// FD is a pre-opened TAP file descriptor.
	FD int
}

func (t *TapNetBackend) Type() string { return "tap" }

func (t *TapNetBackend) BuildNetdevArgs(id string) []string {
	var parts []string
	parts = append(parts, "tap")
	parts = append(parts, "id="+id)

	if t.Ifname != "" {
		parts = append(parts, "ifname="+t.Ifname)
	}
	if t.Bridge != "" {
		parts = append(parts, "br="+t.Bridge)
	}
	if t.Script != "" {
		parts = append(parts, "script="+t.Script)
	}
	if t.DownScript != "" {
		parts = append(parts, "downscript="+t.DownScript)
	}
	if t.VHost {
		parts = append(parts, "vhost=on")
	}
	if t.Queues > 1 {
		parts = append(parts, fmt.Sprintf("queues=%d", t.Queues))
	}
	if t.FD > 0 {
		parts = append(parts, fmt.Sprintf("fd=%d", t.FD))
	}

	return []string{"-netdev", strings.Join(parts, ",")}
}

// SocketNetBackend provides socket-based networking.
type SocketNetBackend struct {
	// Path is the Unix socket path.
	Path string

	// Server makes this end the server.
	Server bool

	// Reconnect interval in seconds (client mode).
	Reconnect int
}

func (s *SocketNetBackend) Type() string { return "socket" }

func (s *SocketNetBackend) BuildNetdevArgs(id string) []string {
	var parts []string
	parts = append(parts, "socket")
	parts = append(parts, "id="+id)

	if s.Path != "" {
		if s.Server {
			parts = append(parts, "listen="+s.Path)
		} else {
			parts = append(parts, "connect="+s.Path)
		}
	}

	return []string{"-netdev", strings.Join(parts, ",")}
}

// StreamNetBackend provides stream socket networking (QEMU 7.2+).
type StreamNetBackend struct {
	// Path is the Unix socket path.
	Path string

	// Host is the TCP host.
	Host string

	// Port is the TCP port.
	Port int

	// Server makes this end the server.
	Server bool

	// Reconnect interval in seconds (client mode).
	Reconnect int
}

func (s *StreamNetBackend) Type() string { return "stream" }

func (s *StreamNetBackend) BuildNetdevArgs(id string) []string {
	var parts []string
	parts = append(parts, "stream")
	parts = append(parts, "id="+id)

	if s.Server {
		parts = append(parts, "server=on")
	} else {
		parts = append(parts, "server=off")
	}

	if s.Path != "" {
		parts = append(parts, "addr.type=unix")
		parts = append(parts, "addr.path="+s.Path)
	} else if s.Host != "" {
		parts = append(parts, "addr.type=inet")
		parts = append(parts, "addr.host="+s.Host)
		if s.Port > 0 {
			parts = append(parts, fmt.Sprintf("addr.port=%d", s.Port))
		}
	}

	if s.Reconnect > 0 && !s.Server {
		parts = append(parts, fmt.Sprintf("reconnect=%d", s.Reconnect))
	}

	return []string{"-netdev", strings.Join(parts, ",")}
}

// VDENetBackend provides VDE networking.
type VDENetBackend struct {
	// Sock is the VDE socket path.
	Sock string

	// Port is the VDE port number.
	Port int

	// Group is the VDE group.
	Group string

	// Mode is the VDE socket mode.
	Mode string
}

func (v *VDENetBackend) Type() string { return "vde" }

func (v *VDENetBackend) BuildNetdevArgs(id string) []string {
	var parts []string
	parts = append(parts, "vde")
	parts = append(parts, "id="+id)

	if v.Sock != "" {
		parts = append(parts, "sock="+v.Sock)
	}
	if v.Port > 0 {
		parts = append(parts, fmt.Sprintf("port=%d", v.Port))
	}
	if v.Group != "" {
		parts = append(parts, "group="+v.Group)
	}
	if v.Mode != "" {
		parts = append(parts, "mode="+v.Mode)
	}

	return []string{"-netdev", strings.Join(parts, ",")}
}

// BridgeNetBackend provides bridge helper networking.
type BridgeNetBackend struct {
	// Bridge is the bridge name.
	Bridge string

	// Helper is the bridge helper path.
	Helper string
}

func (b *BridgeNetBackend) Type() string { return "bridge" }

func (b *BridgeNetBackend) BuildNetdevArgs(id string) []string {
	var parts []string
	parts = append(parts, "bridge")
	parts = append(parts, "id="+id)

	if b.Bridge != "" {
		parts = append(parts, "br="+b.Bridge)
	}
	if b.Helper != "" {
		parts = append(parts, "helper="+b.Helper)
	}

	return []string{"-netdev", strings.Join(parts, ",")}
}

// buildNetworkArgs builds all arguments for a network configuration.
func buildNetworkArgs(cfg *NetworkConfig, pciAlloc *pciSlotAllocator) []string {
	if cfg == nil || cfg.Backend == nil {
		return nil
	}

	var args []string

	id := cfg.ID
	if id == "" {
		id = "net0"
	}

	// Build netdev
	args = append(args, cfg.Backend.BuildNetdevArgs(id)...)

	// Build device
	model := cfg.Model
	if model == "" {
		model = "virtio-net-pci"
	}

	deviceParts := []string{model}
	deviceParts = append(deviceParts, "netdev="+id)
	deviceParts = append(deviceParts, "id="+id+"-device")

	if cfg.MACAddr != "" {
		deviceParts = append(deviceParts, "mac="+cfg.MACAddr)
	}

	if pciAlloc != nil && (model == "virtio-net-pci" || model == "e1000" || model == "e1000e" || model == "rtl8139") {
		deviceParts = append(deviceParts, "bus="+pciAlloc.Bus())
		deviceParts = append(deviceParts, "addr="+pciAlloc.Alloc())
	}

	if cfg.BootIndex > 0 {
		deviceParts = append(deviceParts, fmt.Sprintf("bootindex=%d", cfg.BootIndex))
	}

	args = append(args, "-device", strings.Join(deviceParts, ","))

	return args
}
