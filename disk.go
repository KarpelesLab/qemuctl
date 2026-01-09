package qemuctl

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DiskConfig configures a disk drive.
type DiskConfig struct {
	// ID is the drive/device ID.
	ID string

	// Backend configures the storage backend.
	Backend DiskBackend

	// Interface is the disk interface ("virtio", "ide", "scsi", "nvme").
	Interface string

	// Cache is the caching mode ("none", "writeback", "writethrough").
	Cache string

	// Discard enables discard/TRIM ("unmap", "ignore").
	Discard string

	// ReadOnly makes the drive read-only.
	ReadOnly bool

	// BootIndex sets the boot priority (lower = higher priority).
	BootIndex int

	// Throttle configures I/O throttling.
	Throttle *ThrottleConfig

	// Serial is the drive's serial number.
	Serial string
}

// DiskBackend is the interface for disk backends.
type DiskBackend interface {
	// Type returns the backend type name.
	Type() string

	// BuildBlockdevArgs builds the blockdev arguments.
	BuildBlockdevArgs(id string) []string
}

// FileDiskBackend represents a file-based disk.
type FileDiskBackend struct {
	// Path is the file path.
	Path string

	// Format is the disk format ("raw", "qcow2", "vmdk").
	Format string

	// AutoReadOnly enables auto read-only mode.
	AutoReadOnly bool
}

func (f *FileDiskBackend) Type() string { return "file" }

func (f *FileDiskBackend) BuildBlockdevArgs(id string) []string {
	fileNode := id + "-file"
	formatNode := id + "-format"

	// Build file backend
	fileOpts := map[string]any{
		"driver":    "file",
		"filename":  f.Path,
		"node-name": fileNode,
	}
	if f.AutoReadOnly {
		fileOpts["auto-read-only"] = true
	}

	fileJSON, _ := json.Marshal(fileOpts)

	// Build format layer
	format := f.Format
	if format == "" {
		format = "raw"
	}
	formatOpts := map[string]any{
		"driver":    format,
		"file":      fileNode,
		"node-name": formatNode,
	}

	formatJSON, _ := json.Marshal(formatOpts)

	return []string{
		"-blockdev", string(fileJSON),
		"-blockdev", string(formatJSON),
	}
}

// NBDDiskBackend represents an NBD-connected disk.
type NBDDiskBackend struct {
	// SocketPath is the Unix socket path.
	SocketPath string

	// Host is the TCP host (alternative to socket).
	Host string

	// Port is the TCP port.
	Port int

	// Export is the NBD export name.
	Export string

	// TLS enables TLS.
	TLS bool

	// TLSCreds is the TLS credentials ID.
	TLSCreds string
}

func (n *NBDDiskBackend) Type() string { return "nbd" }

func (n *NBDDiskBackend) BuildBlockdevArgs(id string) []string {
	nbdNode := id + "-nbd"
	formatNode := id + "-format"

	// Build NBD backend
	nbdOpts := map[string]any{
		"driver":    "nbd",
		"node-name": nbdNode,
	}

	if n.SocketPath != "" {
		nbdOpts["server"] = map[string]any{
			"type": "unix",
			"path": n.SocketPath,
		}
	} else if n.Host != "" {
		server := map[string]any{
			"type": "inet",
			"host": n.Host,
		}
		if n.Port > 0 {
			server["port"] = fmt.Sprintf("%d", n.Port)
		}
		nbdOpts["server"] = server
	}

	if n.Export != "" {
		nbdOpts["export"] = n.Export
	}

	if n.TLS {
		nbdOpts["tls-creds"] = n.TLSCreds
	}

	nbdOpts["cache"] = map[string]any{
		"direct":   true,
		"no-flush": false,
	}

	nbdJSON, _ := json.Marshal(nbdOpts)

	// Build format layer (raw on top of NBD)
	formatOpts := map[string]any{
		"driver":    "raw",
		"file":      nbdNode,
		"node-name": formatNode,
		"read-only": false,
	}

	formatJSON, _ := json.Marshal(formatOpts)

	return []string{
		"-blockdev", string(nbdJSON),
		"-blockdev", string(formatJSON),
	}
}

// RBDDiskBackend represents a Ceph RBD disk.
type RBDDiskBackend struct {
	// Pool is the RBD pool name.
	Pool string

	// Image is the RBD image name.
	Image string

	// Snapshot is the snapshot name (optional).
	Snapshot string

	// Conf is the path to ceph.conf.
	Conf string

	// User is the Ceph user name.
	User string

	// KeySecret is the secret ID for the key.
	KeySecret string

	// AuthClientRequired is the auth method list.
	AuthClientRequired []string
}

func (r *RBDDiskBackend) Type() string { return "rbd" }

func (r *RBDDiskBackend) BuildBlockdevArgs(id string) []string {
	rbdNode := id + "-rbd"
	formatNode := id + "-format"

	// Build RBD backend
	rbdOpts := map[string]any{
		"driver":    "rbd",
		"pool":      r.Pool,
		"image":     r.Image,
		"node-name": rbdNode,
	}

	if r.Snapshot != "" {
		rbdOpts["snapshot"] = r.Snapshot
	}
	if r.Conf != "" {
		rbdOpts["conf"] = r.Conf
	}
	if r.User != "" {
		rbdOpts["user"] = r.User
	}
	if r.KeySecret != "" {
		rbdOpts["key-secret"] = r.KeySecret
	}
	if len(r.AuthClientRequired) > 0 {
		rbdOpts["auth-client-required"] = r.AuthClientRequired
	}

	rbdOpts["cache"] = map[string]any{
		"direct":   true,
		"no-flush": false,
	}
	rbdOpts["discard"] = "unmap"

	rbdJSON, _ := json.Marshal(rbdOpts)

	// Build format layer
	formatOpts := map[string]any{
		"driver":    "raw",
		"file":      rbdNode,
		"node-name": formatNode,
		"read-only": false,
	}

	formatJSON, _ := json.Marshal(formatOpts)

	return []string{
		"-blockdev", string(rbdJSON),
		"-blockdev", string(formatJSON),
	}
}

// ISCSIDiskBackend represents an iSCSI disk.
type ISCSIDiskBackend struct {
	// Portal is the iSCSI portal address (host:port).
	Portal string

	// Target is the iSCSI target name.
	Target string

	// Lun is the LUN number.
	Lun int

	// User is the CHAP user name.
	User string

	// PasswordSecret is the secret ID for CHAP password.
	PasswordSecret string

	// InitiatorName is the initiator IQN.
	InitiatorName string
}

func (i *ISCSIDiskBackend) Type() string { return "iscsi" }

func (i *ISCSIDiskBackend) BuildBlockdevArgs(id string) []string {
	iscsiNode := id + "-iscsi"

	// Build iSCSI backend
	iscsiOpts := map[string]any{
		"driver":    "iscsi",
		"transport": "tcp",
		"portal":    i.Portal,
		"target":    i.Target,
		"lun":       i.Lun,
		"node-name": iscsiNode,
	}

	if i.User != "" {
		iscsiOpts["user"] = i.User
	}
	if i.PasswordSecret != "" {
		iscsiOpts["password-secret"] = i.PasswordSecret
	}
	if i.InitiatorName != "" {
		iscsiOpts["initiator-name"] = i.InitiatorName
	}

	iscsiOpts["cache"] = map[string]any{
		"direct":   true,
		"no-flush": false,
	}
	iscsiOpts["discard"] = "unmap"

	iscsiJSON, _ := json.Marshal(iscsiOpts)

	// For iSCSI, we typically use scsi-block which needs direct access
	return []string{
		"-blockdev", string(iscsiJSON),
	}
}

// ThrottleConfig configures I/O throttling.
type ThrottleConfig struct {
	// Group is the throttle group name.
	Group string

	// BPS is the total bytes per second limit.
	BPS uint64

	// BPSRead is the read bytes per second limit.
	BPSRead uint64

	// BPSWrite is the write bytes per second limit.
	BPSWrite uint64

	// IOPS is the total I/O operations per second limit.
	IOPS uint64

	// IOPSRead is the read IOPS limit.
	IOPSRead uint64

	// IOPSWrite is the write IOPS limit.
	IOPSWrite uint64

	// BPSMax is the burst BPS limit.
	BPSMax uint64

	// IOPSMax is the burst IOPS limit.
	IOPSMax uint64

	// BurstLength is the burst duration in seconds.
	BurstLength int
}

// BuildThrottleGroupArgs builds throttle-group object arguments.
func (t *ThrottleConfig) BuildThrottleGroupArgs() []string {
	if t == nil || t.Group == "" {
		return nil
	}

	var parts []string
	parts = append(parts, "throttle-group")
	parts = append(parts, "id="+t.Group)

	if t.BPS > 0 {
		parts = append(parts, fmt.Sprintf("x-bps-total=%d", t.BPS))
	}
	if t.BPSRead > 0 {
		parts = append(parts, fmt.Sprintf("x-bps-read=%d", t.BPSRead))
	}
	if t.BPSWrite > 0 {
		parts = append(parts, fmt.Sprintf("x-bps-write=%d", t.BPSWrite))
	}
	if t.IOPS > 0 {
		parts = append(parts, fmt.Sprintf("x-iops-total=%d", t.IOPS))
	}
	if t.IOPSRead > 0 {
		parts = append(parts, fmt.Sprintf("x-iops-read=%d", t.IOPSRead))
	}
	if t.IOPSWrite > 0 {
		parts = append(parts, fmt.Sprintf("x-iops-write=%d", t.IOPSWrite))
	}
	if t.BPSMax > 0 {
		parts = append(parts, fmt.Sprintf("x-bps-total-max=%d", t.BPSMax))
	}
	if t.IOPSMax > 0 {
		parts = append(parts, fmt.Sprintf("x-iops-total-max=%d", t.IOPSMax))
	}
	if t.BurstLength > 0 {
		parts = append(parts, fmt.Sprintf("x-bps-total-max-length=%d", t.BurstLength))
		parts = append(parts, fmt.Sprintf("x-iops-total-max-length=%d", t.BurstLength))
	}

	return []string{"-object", strings.Join(parts, ",")}
}

// BuildThrottleBlockdevArgs builds throttle filter blockdev arguments.
func (t *ThrottleConfig) BuildThrottleBlockdevArgs(id, fileNode string) []string {
	if t == nil || t.Group == "" {
		return nil
	}

	throttleOpts := map[string]any{
		"driver":         "throttle",
		"node-name":      id,
		"file":           fileNode,
		"throttle-group": t.Group,
	}

	throttleJSON, _ := json.Marshal(throttleOpts)

	return []string{"-blockdev", string(throttleJSON)}
}

// CDROMConfig configures a CD-ROM drive.
type CDROMConfig struct {
	// Path is the ISO file path.
	Path string

	// BootIndex sets the boot priority.
	BootIndex int
}

// buildDiskArgs builds all arguments for a disk configuration.
func buildDiskArgs(cfg *DiskConfig, pciAlloc *pciSlotAllocator) []string {
	if cfg == nil || cfg.Backend == nil {
		return nil
	}

	var args []string

	id := cfg.ID
	if id == "" {
		id = "drive0"
	}

	// Build throttle group if configured
	if cfg.Throttle != nil && cfg.Throttle.Group != "" {
		args = append(args, cfg.Throttle.BuildThrottleGroupArgs()...)
	}

	// Build backend blockdev args
	backendArgs := cfg.Backend.BuildBlockdevArgs(id)
	args = append(args, backendArgs...)

	// Determine the final node name
	var finalNode string
	switch cfg.Backend.Type() {
	case "iscsi":
		finalNode = id + "-iscsi"
	default:
		finalNode = id + "-format"
	}

	// Add throttle layer if configured
	if cfg.Throttle != nil && cfg.Throttle.Group != "" {
		throttleNode := id + "-throttle"
		args = append(args, cfg.Throttle.BuildThrottleBlockdevArgs(throttleNode, finalNode)...)
		finalNode = throttleNode
	}

	// Build device
	iface := cfg.Interface
	if iface == "" {
		iface = "virtio"
	}

	var deviceType string
	switch iface {
	case "virtio":
		deviceType = "virtio-blk-pci"
	case "scsi":
		// For SCSI, we need to add a controller first (handled separately)
		deviceType = "scsi-hd"
	case "ide":
		deviceType = "ide-hd"
	case "nvme":
		deviceType = "nvme"
	default:
		deviceType = "virtio-blk-pci"
	}

	deviceArgs := fmt.Sprintf("%s,drive=%s,id=%s-device", deviceType, finalNode, id)

	if pciAlloc != nil && (iface == "virtio" || iface == "nvme") {
		deviceArgs += fmt.Sprintf(",bus=%s,addr=%s", pciAlloc.Bus(), pciAlloc.Alloc())
	}

	if cfg.BootIndex > 0 {
		deviceArgs += fmt.Sprintf(",bootindex=%d", cfg.BootIndex)
	}

	if cfg.Serial != "" {
		deviceArgs += fmt.Sprintf(",serial=%s", cfg.Serial)
	}

	args = append(args, "-device", deviceArgs)

	return args
}

// buildCDROMArgs builds CD-ROM drive arguments.
func buildCDROMArgs(cfg *CDROMConfig, index int, sataController string) []string {
	if cfg == nil || cfg.Path == "" {
		return nil
	}

	id := fmt.Sprintf("cdrom%d", index)

	// Use simple -drive for CD-ROM
	driveArg := fmt.Sprintf("file=%s,format=raw,if=none,id=%s,media=cdrom,readonly=on", cfg.Path, id)

	var args []string
	args = append(args, "-drive", driveArg)

	// Add IDE-CD device on SATA controller
	deviceArg := fmt.Sprintf("ide-cd,bus=%s.%d,drive=%s,id=%s-device", sataController, index, id, id)
	if cfg.BootIndex > 0 {
		deviceArg += fmt.Sprintf(",bootindex=%d", cfg.BootIndex)
	}
	args = append(args, "-device", deviceArg)

	return args
}
