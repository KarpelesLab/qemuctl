package qemuctl

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KarpelesLab/runutil"
)

// Instance represents a running or attached QEMU instance.
type Instance struct {
	name       string
	pid        int
	process    *os.Process
	config     *Config
	socketPath string

	qmp     *QMP
	qmpMu   sync.Mutex
	state   State
	stateMu sync.RWMutex

	onStateChange func(State)
	onEvent       func(*Event)
}

// Name returns the instance name.
func (i *Instance) Name() string {
	return i.name
}

// PID returns the process ID of the QEMU process.
func (i *Instance) PID() int {
	if i.process != nil {
		return i.process.Pid
	}
	return i.pid
}

// SocketPath returns the path to the QMP control socket.
func (i *Instance) SocketPath() string {
	return i.socketPath
}

// State returns the current state of the instance.
func (i *Instance) State() State {
	i.stateMu.RLock()
	defer i.stateMu.RUnlock()
	return i.state
}

// setState updates the instance state and calls the callback if set.
func (i *Instance) setState(s State) {
	i.stateMu.Lock()
	old := i.state
	i.state = s
	i.stateMu.Unlock()

	if old != s && i.onStateChange != nil {
		i.onStateChange(s)
	}
}

// SetStateChangeCallback sets a callback for state changes.
func (i *Instance) SetStateChangeCallback(cb func(State)) {
	i.onStateChange = cb
	if i.qmp != nil {
		i.qmp.SetStateChangeCallback(func(s State) {
			i.setState(s)
		})
	}
}

// SetEventCallback sets a callback for all events.
func (i *Instance) SetEventCallback(cb func(*Event)) {
	i.onEvent = cb
	if i.qmp != nil {
		i.qmp.SetEventCallback(cb)
	}
}

// Events returns the event channel.
func (i *Instance) Events() <-chan *Event {
	if i.qmp == nil {
		return nil
	}
	return i.qmp.Events()
}

// QMP returns the QMP connection for direct command execution.
func (i *Instance) QMP() *QMP {
	i.qmpMu.Lock()
	defer i.qmpMu.Unlock()
	return i.qmp
}

// Start launches a new QEMU instance with the given configuration.
func Start(cfg *Config) (*Instance, error) {
	return StartContext(context.Background(), cfg)
}

// StartContext launches a new QEMU instance with context support.
func StartContext(ctx context.Context, cfg *Config) (*Instance, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
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
	socketDir, err := cfg.ensureSocketDir()
	if err != nil {
		return nil, err
	}

	socketPath := filepath.Join(socketDir, name+".sock")

	// Remove stale socket
	os.Remove(socketPath)

	// Build command line
	args := buildArgs(cfg, name, socketPath)

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
		config:     cfg,
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

// Attach connects to an existing QEMU instance by its control socket.
func Attach(socketPath string) (*Instance, error) {
	return AttachContext(context.Background(), socketPath)
}

// AttachContext connects to an existing QEMU instance with context support.
func AttachContext(ctx context.Context, socketPath string) (*Instance, error) {
	// Verify socket exists
	if _, err := os.Stat(socketPath); err != nil {
		return nil, fmt.Errorf("socket not found: %w", err)
	}

	// Connect QMP
	qmp, err := newQMP(socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect QMP: %w", err)
	}

	name := filepath.Base(socketPath)
	name = strings.TrimSuffix(name, ".sock")

	inst := &Instance{
		name:       name,
		socketPath: socketPath,
		qmp:        qmp,
		state:      StateUnknown,
	}

	qmp.SetStateChangeCallback(func(s State) {
		inst.setState(s)
	})

	// Query initial state
	if err := inst.QueryState(); err != nil {
		// Non-fatal
	}

	return inst, nil
}

// AttachByPID connects to an existing QEMU instance by its process ID.
// It uses runutil.ArgsOf to find the control socket from the process arguments.
func AttachByPID(pid int) (*Instance, error) {
	return AttachByPIDContext(context.Background(), pid)
}

// AttachByPIDContext connects to an existing QEMU instance by PID with context.
func AttachByPIDContext(ctx context.Context, pid int) (*Instance, error) {
	args, err := runutil.ArgsOf(pid)
	if err != nil {
		return nil, fmt.Errorf("failed to get process arguments: %w", err)
	}

	// Find the QMP socket path from arguments
	socketPath := findSocketFromArgs(args)
	if socketPath == "" {
		return nil, fmt.Errorf("could not find QMP socket in process arguments")
	}

	inst, err := AttachContext(ctx, socketPath)
	if err != nil {
		return nil, err
	}

	// Find process
	proc, err := os.FindProcess(pid)
	if err == nil {
		inst.process = proc
	}
	inst.pid = pid

	// Try to find name from -name argument
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-name" {
			nameArg := args[i+1]
			if strings.HasPrefix(nameArg, "guest=") {
				parts := strings.Split(nameArg[6:], ",")
				if len(parts) > 0 {
					inst.name = parts[0]
				}
			} else if !strings.Contains(nameArg, "=") {
				inst.name = nameArg
			}
			break
		}
	}

	return inst, nil
}

// findSocketFromArgs extracts the QMP socket path from QEMU arguments.
func findSocketFromArgs(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-chardev" {
			chardev := args[i+1]
			// Look for socket,id=XXX,path=YYY or socket,path=YYY,id=XXX
			if strings.HasPrefix(chardev, "socket,") {
				parts := strings.Split(chardev, ",")
				var isMonitor bool
				var socketPath string

				for _, part := range parts[1:] {
					if strings.HasPrefix(part, "path=") {
						socketPath = part[5:]
					}
					if strings.HasPrefix(part, "id=") {
						id := part[3:]
						if strings.Contains(id, "monitor") || strings.Contains(id, "qmp") {
							isMonitor = true
						}
					}
				}

				if socketPath != "" && isMonitor {
					return socketPath
				}
			}
		}
	}
	return ""
}

// QueryState queries and updates the current state from QEMU.
func (i *Instance) QueryState() error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	result, err := qmp.Execute("query-status", nil)
	if err != nil {
		return err
	}

	var status struct {
		Status  string `json:"status"`
		Running bool   `json:"running"`
	}

	if err := unmarshalJSON(result, &status); err != nil {
		return err
	}

	i.setState(parseQMPStatus(status.Status))
	return nil
}

// Continue resumes a paused VM.
func (i *Instance) Continue() error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	_, err := qmp.Execute("cont", nil)
	return err
}

// Pause pauses a running VM.
func (i *Instance) Pause() error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	_, err := qmp.Execute("stop", nil)
	return err
}

// Reset performs a hard reset of the VM.
// This is equivalent to pressing the reset button - it's immediate and does not
// give the guest OS a chance to shut down gracefully.
func (i *Instance) Reset() error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return ErrNotConnected
	}

	_, err := qmp.Execute("system_reset", nil)
	if err != nil {
		return fmt.Errorf("system_reset failed: %w", err)
	}

	return nil
}

// Stop performs a graceful shutdown, waiting for the guest to respond.
// If the guest doesn't shut down within the timeout, it is forcefully killed.
// Returns nil on successful shutdown, or an error if force stop was required.
func (i *Instance) Stop(timeout time.Duration) error {
	return i.StopContext(context.Background(), timeout)
}

// StopContext performs a graceful shutdown with context support.
// It sends an ACPI power button event (system_powerdown) and waits for the
// guest to shut down. If the guest doesn't respond within the timeout,
// it forcefully terminates the QEMU process.
func (i *Instance) StopContext(ctx context.Context, timeout time.Duration) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		// No QMP connection, just force stop
		return i.ForceStop()
	}

	// Send ACPI power button event
	_, err := qmp.Execute("system_powerdown", nil)
	if err != nil {
		// QMP command failed, force stop
		i.ForceStop()
		return fmt.Errorf("system_powerdown failed: %w", err)
	}

	// Wait for graceful shutdown
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			i.ForceStop()
			return ctx.Err()
		case <-ticker.C:
			// Check if process has exited
			if !i.isProcessAlive() {
				i.cleanup()
				return nil
			}

			// Check if we received SHUTDOWN event
			if i.State() == StateShutdown {
				// Guest has shut down, wait briefly for process to exit
				time.Sleep(500 * time.Millisecond)
				if !i.isProcessAlive() {
					i.cleanup()
					return nil
				}
			}
		}
	}

	// Timeout reached, force stop
	i.ForceStop()
	return fmt.Errorf("graceful shutdown timed out after %v", timeout)
}

// Shutdown sends a powerdown request to the guest (ACPI power button).
// Unlike Stop, this does not wait or force kill.
func (i *Instance) Shutdown() error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	_, err := qmp.Execute("system_powerdown", nil)
	return err
}

// ForceStop immediately terminates the QEMU process.
func (i *Instance) ForceStop() error {
	if i.process != nil {
		// Kill the process group
		syscall.Kill(-i.process.Pid, syscall.SIGKILL)
		i.process.Kill()
	} else if i.pid > 0 {
		syscall.Kill(i.pid, syscall.SIGKILL)
	}

	i.cleanup()
	return nil
}

// Quit sends the quit command to QEMU (immediate exit).
func (i *Instance) Quit() error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	_, err := qmp.Execute("quit", nil)
	if err != nil {
		// Quit may not return a response before disconnecting
		return nil
	}

	i.cleanup()
	return nil
}

// isProcessAlive checks if the QEMU process is still running.
func (i *Instance) isProcessAlive() bool {
	var pid int
	if i.process != nil {
		pid = i.process.Pid
	} else if i.pid > 0 {
		pid = i.pid
	} else {
		return false
	}

	return syscall.Kill(pid, 0) == nil
}

// cleanup releases resources associated with this instance.
func (i *Instance) cleanup() {
	i.qmpMu.Lock()
	if i.qmp != nil {
		i.qmp.Close()
		i.qmp = nil
	}
	i.qmpMu.Unlock()

	i.setState(StateShutdown)

	// Clean up socket if we created it
	if i.socketPath != "" {
		os.Remove(i.socketPath)
	}
}

// Wait waits for the QEMU process to exit.
func (i *Instance) Wait() error {
	if i.process != nil {
		_, err := i.process.Wait()
		i.cleanup()
		return err
	}

	// For attached instances, poll until dead
	for i.isProcessAlive() {
		time.Sleep(100 * time.Millisecond)
	}
	i.cleanup()
	return nil
}

// WaitContext waits for the QEMU process with context support.
func (i *Instance) WaitContext(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		done <- i.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// buildArgs builds the QEMU command line arguments.
func buildArgs(cfg *Config, name, socketPath string) []string {
	var args []string

	// Name
	args = append(args, "-name", fmt.Sprintf("guest=%s,debug-threads=on", name))

	// Disable GUI
	args = append(args, "-display", "none")

	// No defaults
	if cfg.NoDefaults == nil || *cfg.NoDefaults {
		args = append(args, "-no-user-config", "-nodefaults")
	}

	// Machine
	machine := cfg.Machine
	if machine == "" {
		if cfg.Arch == "" || cfg.Arch == "amd64" || cfg.Arch == "386" {
			machine = "q35"
		}
	}

	if machine != "" {
		machineArg := machine
		if cfg.KVM == nil || *cfg.KVM {
			machineArg += ",accel=kvm"
		}
		args = append(args, "-machine", machineArg)
	}

	// CPU
	cpu := cfg.CPU
	if cpu == "" && (cfg.KVM == nil || *cfg.KVM) {
		cpu = "host"
	}
	if cpu != "" {
		args = append(args, "-cpu", cpu)
	}

	// Memory
	args = append(args, "-m", strconv.Itoa(cfg.Memory))

	// CPUs
	if cfg.CPUs > 1 {
		args = append(args, "-smp", strconv.Itoa(cfg.CPUs))
	}

	// QMP monitor socket
	args = append(args, "-chardev", fmt.Sprintf("socket,id=qmp,path=%s,server=on,wait=off", socketPath))
	args = append(args, "-mon", "chardev=qmp,id=monitor,mode=control")

	// Drives
	for idx, drive := range cfg.Drives {
		driveArg := fmt.Sprintf("file=%s", drive.File)
		if drive.Format != "" {
			driveArg += fmt.Sprintf(",format=%s", drive.Format)
		}
		if drive.Cache != "" {
			driveArg += fmt.Sprintf(",cache=%s", drive.Cache)
		}
		if drive.ReadOnly {
			driveArg += ",readonly=on"
		}

		iface := drive.Interface
		if iface == "" {
			iface = "virtio"
		}
		driveArg += fmt.Sprintf(",if=%s,id=drive%d", iface, idx)

		args = append(args, "-drive", driveArg)
	}

	// Network devices
	for idx, net := range cfg.NetworkDevices {
		netType := net.Type
		if netType == "" {
			netType = "user"
		}

		id := net.ID
		if id == "" {
			id = fmt.Sprintf("net%d", idx)
		}

		// Netdev
		netdevArg := fmt.Sprintf("%s,id=%s", netType, id)
		if net.SocketPath != "" {
			netdevArg += fmt.Sprintf(",path=%s", net.SocketPath)
		}
		if net.Bridge != "" {
			netdevArg += fmt.Sprintf(",br=%s", net.Bridge)
		}
		if net.Script != "" {
			netdevArg += fmt.Sprintf(",script=%s", net.Script)
		}
		if net.DownScript != "" {
			netdevArg += fmt.Sprintf(",downscript=%s", net.DownScript)
		}
		args = append(args, "-netdev", netdevArg)

		// Device
		model := net.Model
		if model == "" {
			model = "virtio-net-pci"
		}
		deviceArg := fmt.Sprintf("%s,netdev=%s", model, id)
		if net.MACAddr != "" {
			deviceArg += fmt.Sprintf(",mac=%s", net.MACAddr)
		}
		args = append(args, "-device", deviceArg)
	}

	// VNC
	if cfg.VNC != "" {
		args = append(args, "-vnc", cfg.VNC)
	}

	// SPICE
	if cfg.Spice != nil {
		spiceArg := ""
		if cfg.Spice.UnixSocket {
			spiceArg = "unix=on"
		}
		if cfg.Spice.Password != "" {
			spiceArg += fmt.Sprintf(",password=%s", cfg.Spice.Password)
		}
		if cfg.Spice.DisableTicketing {
			spiceArg += ",disable-ticketing=on"
		}
		if cfg.Spice.ImageCompression != "" {
			spiceArg += fmt.Sprintf(",image-compression=%s", cfg.Spice.ImageCompression)
		}
		if cfg.Spice.PlaybackCompression {
			spiceArg += ",playback-compression=on"
		}
		if spiceArg != "" {
			args = append(args, "-spice", spiceArg)
		}
	}

	// Extra args
	args = append(args, cfg.ExtraArgs...)

	return args
}

// waitForSocket waits for the QMP socket to become available.
func waitForSocket(ctx context.Context, path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if conn, err := net.Dial("unix", path); err == nil {
			conn.Close()
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for socket %s", path)
}

// generateName generates a random instance name.
func generateName() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "qemu-" + hex.EncodeToString(b)
}
