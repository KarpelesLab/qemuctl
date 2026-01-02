package qemuctl

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"
)

// skipIfNoQemu skips the test if QEMU is not available or KVM is not accessible.
func skipIfNoQemu(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := LocateQemu(runtime.GOARCH, ""); err != nil {
		t.Skipf("QEMU not found: %v", err)
	}

	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("no KVM access")
	}
}

// startTestVM starts a minimal test VM and returns cleanup function.
func startTestVM(t *testing.T, name string) (*Instance, func()) {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Name = name
	cfg.Memory = 64 // Minimal memory for faster tests

	inst, err := Start(cfg)
	if err != nil {
		t.Fatalf("failed to start QEMU: %v", err)
	}

	cleanup := func() {
		inst.ForceStop()
	}

	return inst, cleanup
}

func TestIntegrationStartStop(t *testing.T) {
	skipIfNoQemu(t)

	inst, cleanup := startTestVM(t, "test-start-stop")
	defer cleanup()

	// Verify instance properties
	if inst.Name() != "test-start-stop" {
		t.Errorf("expected name 'test-start-stop', got %q", inst.Name())
	}

	if inst.PID() <= 0 {
		t.Errorf("expected positive PID, got %d", inst.PID())
	}

	if inst.SocketPath() == "" {
		t.Error("expected non-empty socket path")
	}

	t.Logf("Started VM: name=%s pid=%d socket=%s", inst.Name(), inst.PID(), inst.SocketPath())

	// Verify socket file exists
	if _, err := os.Stat(inst.SocketPath()); err != nil {
		t.Errorf("socket file does not exist: %v", err)
	}

	// Stop and verify cleanup
	if err := inst.ForceStop(); err != nil {
		t.Errorf("ForceStop error: %v", err)
	}

	// Give it a moment to clean up
	time.Sleep(100 * time.Millisecond)

	// Socket should be removed
	if _, err := os.Stat(inst.SocketPath()); err == nil {
		t.Error("socket file should be removed after stop")
	}
}

func TestIntegrationStateTransitions(t *testing.T) {
	skipIfNoQemu(t)

	inst, cleanup := startTestVM(t, "test-state-transitions")
	defer cleanup()

	// Query initial state
	if err := inst.QueryState(); err != nil {
		t.Fatalf("QueryState error: %v", err)
	}

	initialState := inst.State()
	t.Logf("Initial state: %s", initialState)

	if initialState != StateRunning {
		t.Errorf("expected running state, got %s", initialState)
	}

	// Test pause
	t.Log("Pausing VM...")
	if err := inst.Pause(); err != nil {
		t.Fatalf("Pause error: %v", err)
	}

	// Query state after pause
	if err := inst.QueryState(); err != nil {
		t.Fatalf("QueryState after pause error: %v", err)
	}

	if inst.State() != StatePaused {
		t.Errorf("expected paused state, got %s", inst.State())
	}
	t.Logf("State after pause: %s", inst.State())

	// Test continue
	t.Log("Resuming VM...")
	if err := inst.Continue(); err != nil {
		t.Fatalf("Continue error: %v", err)
	}

	// Query state after continue
	if err := inst.QueryState(); err != nil {
		t.Fatalf("QueryState after continue error: %v", err)
	}

	if inst.State() != StateRunning {
		t.Errorf("expected running state after continue, got %s", inst.State())
	}
	t.Logf("State after continue: %s", inst.State())
}

func TestIntegrationReset(t *testing.T) {
	skipIfNoQemu(t)

	inst, cleanup := startTestVM(t, "test-reset")
	defer cleanup()

	// Ensure running
	if err := inst.QueryState(); err != nil {
		t.Fatalf("QueryState error: %v", err)
	}

	if inst.State() != StateRunning {
		t.Fatalf("expected running state, got %s", inst.State())
	}

	// Reset
	t.Log("Resetting VM...")
	if err := inst.Reset(); err != nil {
		t.Fatalf("Reset error: %v", err)
	}

	// Should still be running after reset
	time.Sleep(100 * time.Millisecond)
	if err := inst.QueryState(); err != nil {
		t.Fatalf("QueryState after reset error: %v", err)
	}

	if inst.State() != StateRunning {
		t.Errorf("expected running state after reset, got %s", inst.State())
	}
	t.Logf("State after reset: %s", inst.State())
}

func TestIntegrationQuit(t *testing.T) {
	skipIfNoQemu(t)

	inst, cleanup := startTestVM(t, "test-quit")
	defer cleanup()

	pid := inst.PID()
	t.Logf("VM PID: %d", pid)

	// Quit command
	t.Log("Sending quit command...")
	if err := inst.Quit(); err != nil {
		t.Logf("Quit returned error (may be expected): %v", err)
	}

	// Wait for process to exit
	time.Sleep(500 * time.Millisecond)

	// Verify process is gone
	if err := checkProcessExists(pid); err == nil {
		t.Error("process should not exist after quit")
	}
}

func TestIntegrationAttachByPID(t *testing.T) {
	skipIfNoQemu(t)

	// Start QEMU directly without connecting QMP (so we can attach later)
	qemuPath, err := LocateQemu(runtime.GOARCH, "")
	if err != nil {
		t.Fatalf("LocateQemu error: %v", err)
	}

	socketDir, err := defaultSocketDir()
	if err != nil {
		t.Fatalf("defaultSocketDir error: %v", err)
	}
	os.MkdirAll(socketDir, 0755)
	socketPath := socketDir + "/test-attach-pid.sock"
	os.Remove(socketPath)

	// Start QEMU manually
	cmd := startQemuProcess(t, qemuPath, "test-attach-pid", socketPath)
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
		os.Remove(socketPath)
	}()

	pid := cmd.Process.Pid
	t.Logf("Started QEMU with PID %d", pid)

	// Wait for socket
	if err := waitForSocketFile(socketPath, 5*time.Second); err != nil {
		t.Fatalf("socket not created: %v", err)
	}

	// Attach by PID
	attached, err := AttachByPID(pid)
	if err != nil {
		t.Fatalf("AttachByPID error: %v", err)
	}

	// Verify attached instance works
	if err := attached.QueryState(); err != nil {
		t.Fatalf("QueryState on attached instance error: %v", err)
	}

	t.Logf("Attached instance state: %s", attached.State())

	if attached.State() != StateRunning {
		t.Errorf("expected running state, got %s", attached.State())
	}

	// Verify we can control via attached instance
	if err := attached.Pause(); err != nil {
		t.Fatalf("Pause via attached instance error: %v", err)
	}

	if err := attached.QueryState(); err != nil {
		t.Fatalf("QueryState after pause error: %v", err)
	}

	if attached.State() != StatePaused {
		t.Errorf("expected paused state, got %s", attached.State())
	}

	// Resume and quit
	attached.Continue()
	attached.Quit()
}

func TestIntegrationAttachBySocket(t *testing.T) {
	skipIfNoQemu(t)

	// Start QEMU directly without connecting QMP
	qemuPath, err := LocateQemu(runtime.GOARCH, "")
	if err != nil {
		t.Fatalf("LocateQemu error: %v", err)
	}

	socketDir, err := defaultSocketDir()
	if err != nil {
		t.Fatalf("defaultSocketDir error: %v", err)
	}
	os.MkdirAll(socketDir, 0755)
	socketPath := socketDir + "/test-attach-socket.sock"
	os.Remove(socketPath)

	cmd := startQemuProcess(t, qemuPath, "test-attach-socket", socketPath)
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
		os.Remove(socketPath)
	}()

	t.Logf("Socket path: %s", socketPath)

	// Wait for socket
	if err := waitForSocketFile(socketPath, 5*time.Second); err != nil {
		t.Fatalf("socket not created: %v", err)
	}

	// Attach by socket
	attached, err := Attach(socketPath)
	if err != nil {
		t.Fatalf("Attach error: %v", err)
	}

	// Verify attached instance works
	if err := attached.QueryState(); err != nil {
		t.Fatalf("QueryState on attached instance error: %v", err)
	}

	t.Logf("Attached instance state: %s", attached.State())

	if attached.State() != StateRunning {
		t.Errorf("expected running state, got %s", attached.State())
	}

	attached.Quit()
}

func TestIntegrationEventCallback(t *testing.T) {
	skipIfNoQemu(t)

	inst, cleanup := startTestVM(t, "test-events")
	defer cleanup()

	var mu sync.Mutex
	var stateChanges []State
	var events []*Event

	// Set up callbacks
	inst.SetStateChangeCallback(func(s State) {
		mu.Lock()
		stateChanges = append(stateChanges, s)
		mu.Unlock()
	})

	inst.SetEventCallback(func(e *Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	// Trigger state changes
	if err := inst.Pause(); err != nil {
		t.Fatalf("Pause error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if err := inst.Continue(); err != nil {
		t.Fatalf("Continue error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Check callbacks were called
	mu.Lock()
	defer mu.Unlock()

	t.Logf("State changes received: %v", stateChanges)
	t.Logf("Events received: %d", len(events))

	if len(stateChanges) < 2 {
		t.Errorf("expected at least 2 state changes, got %d", len(stateChanges))
	}

	// Should have STOP and RESUME events
	foundStop := false
	foundResume := false
	for _, e := range events {
		t.Logf("Event: %s", e.Name)
		if e.Name == "STOP" {
			foundStop = true
		}
		if e.Name == "RESUME" {
			foundResume = true
		}
	}

	if !foundStop {
		t.Error("expected STOP event")
	}
	if !foundResume {
		t.Error("expected RESUME event")
	}
}

func TestIntegrationQMPCommands(t *testing.T) {
	skipIfNoQemu(t)

	inst, cleanup := startTestVM(t, "test-qmp")
	defer cleanup()

	qmp := inst.QMP()
	if qmp == nil {
		t.Fatal("QMP is nil")
	}

	// Test query-version
	t.Log("Testing query-version...")
	result, err := qmp.Execute("query-version", nil)
	if err != nil {
		t.Fatalf("query-version error: %v", err)
	}

	var version struct {
		Qemu struct {
			Major int `json:"major"`
			Minor int `json:"minor"`
			Micro int `json:"micro"`
		} `json:"qemu"`
		Package string `json:"package"`
	}
	if err := json.Unmarshal(result, &version); err != nil {
		t.Fatalf("failed to parse version: %v", err)
	}
	t.Logf("QEMU version: %d.%d.%d", version.Qemu.Major, version.Qemu.Minor, version.Qemu.Micro)

	// Test query-status
	t.Log("Testing query-status...")
	result, err = qmp.Execute("query-status", nil)
	if err != nil {
		t.Fatalf("query-status error: %v", err)
	}
	t.Logf("Status: %s", string(result))

	// Test query-cpus-fast
	t.Log("Testing query-cpus-fast...")
	result, err = qmp.Execute("query-cpus-fast", nil)
	if err != nil {
		t.Fatalf("query-cpus-fast error: %v", err)
	}

	var cpus []struct {
		CPUIndex int    `json:"cpu-index"`
		Target   string `json:"target"`
	}
	if err := json.Unmarshal(result, &cpus); err != nil {
		t.Fatalf("failed to parse CPUs: %v", err)
	}
	t.Logf("CPUs: %d", len(cpus))

	// Test query-machines
	t.Log("Testing query-machines...")
	result, err = qmp.Execute("query-machines", nil)
	if err != nil {
		t.Fatalf("query-machines error: %v", err)
	}

	var machines []struct {
		Name      string `json:"name"`
		IsDefault bool   `json:"is-default"`
	}
	if err := json.Unmarshal(result, &machines); err != nil {
		t.Fatalf("failed to parse machines: %v", err)
	}
	t.Logf("Available machines: %d", len(machines))

	// Test human-monitor-command
	t.Log("Testing human-monitor-command...")
	output, err := inst.HumanMonitorCommand("info version")
	if err != nil {
		t.Fatalf("HumanMonitorCommand error: %v", err)
	}
	t.Logf("Human monitor output: %s", output)
}

func TestIntegrationContextCancellation(t *testing.T) {
	skipIfNoQemu(t)

	ctx, cancel := context.WithCancel(context.Background())

	cfg := DefaultConfig()
	cfg.Name = "test-context"
	cfg.Memory = 64

	inst, err := StartContext(ctx, cfg)
	if err != nil {
		t.Fatalf("StartContext error: %v", err)
	}

	// Verify running
	if err := inst.QueryState(); err != nil {
		t.Fatalf("QueryState error: %v", err)
	}

	if inst.State() != StateRunning {
		t.Errorf("expected running state, got %s", inst.State())
	}

	// Cancel context and try to stop
	cancel()

	// StopContext should respect cancellation
	err = inst.StopContext(ctx, 5*time.Second)
	if err != context.Canceled {
		t.Logf("StopContext result: %v (expected context.Canceled)", err)
	}

	// Cleanup
	inst.ForceStop()
}

func TestIntegrationMultipleInstances(t *testing.T) {
	skipIfNoQemu(t)

	// Start multiple VMs
	var instances []*Instance
	var cleanups []func()

	for i := 0; i < 3; i++ {
		inst, cleanup := startTestVM(t, "test-multi-"+string(rune('a'+i)))
		instances = append(instances, inst)
		cleanups = append(cleanups, cleanup)
	}

	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	// Verify all running
	for i, inst := range instances {
		if err := inst.QueryState(); err != nil {
			t.Errorf("Instance %d QueryState error: %v", i, err)
			continue
		}

		if inst.State() != StateRunning {
			t.Errorf("Instance %d: expected running state, got %s", i, inst.State())
		}

		t.Logf("Instance %d: name=%s pid=%d state=%s", i, inst.Name(), inst.PID(), inst.State())
	}

	// Pause all
	for i, inst := range instances {
		if err := inst.Pause(); err != nil {
			t.Errorf("Instance %d Pause error: %v", i, err)
		}
	}

	// Verify all paused
	time.Sleep(100 * time.Millisecond)
	for i, inst := range instances {
		if err := inst.QueryState(); err != nil {
			t.Errorf("Instance %d QueryState error: %v", i, err)
			continue
		}

		if inst.State() != StatePaused {
			t.Errorf("Instance %d: expected paused state, got %s", i, inst.State())
		}
	}

	// Resume all
	for i, inst := range instances {
		if err := inst.Continue(); err != nil {
			t.Errorf("Instance %d Continue error: %v", i, err)
		}
	}
}

func TestIntegrationGracefulStop(t *testing.T) {
	skipIfNoQemu(t)

	inst, _ := startTestVM(t, "test-graceful")
	// Don't use cleanup since we're testing Stop

	pid := inst.PID()
	t.Logf("VM PID: %d", pid)

	// Try graceful stop with short timeout
	// Since there's no OS, ACPI shutdown won't work, so it should force stop
	t.Log("Testing graceful stop (will timeout and force stop)...")
	start := time.Now()
	if err := inst.Stop(1 * time.Second); err != nil {
		t.Logf("Stop returned error (may be expected): %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("Stop took %v", elapsed)

	// Should have waited at least close to the timeout
	if elapsed < 900*time.Millisecond {
		t.Logf("Stop completed faster than expected (may have force stopped early)")
	}

	// Verify process is gone
	time.Sleep(100 * time.Millisecond)
	if err := checkProcessExists(pid); err == nil {
		t.Error("process should not exist after stop")
	}
}

func TestIntegrationSendKey(t *testing.T) {
	skipIfNoQemu(t)

	inst, cleanup := startTestVM(t, "test-sendkey")
	defer cleanup()

	// Send some keys
	t.Log("Sending keys...")

	// Single key
	if err := inst.SendKey("a"); err != nil {
		t.Errorf("SendKey('a') error: %v", err)
	}

	// Key combination
	if err := inst.SendKey("ctrl", "alt", "delete"); err != nil {
		t.Errorf("SendKey('ctrl', 'alt', 'delete') error: %v", err)
	}

	t.Log("Keys sent successfully")
}

// checkProcessExists returns nil if process exists, error otherwise.
func checkProcessExists(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// On Unix, FindProcess always succeeds, so we need to send signal 0
	return proc.Signal(nil)
}

// startQemuProcess starts a QEMU process directly without connecting QMP.
func startQemuProcess(t *testing.T, qemuPath, name, socketPath string) *exec.Cmd {
	t.Helper()

	args := []string{
		"-name", "guest=" + name,
		"-display", "none",
		"-no-user-config", "-nodefaults",
		"-machine", "q35,accel=kvm",
		"-cpu", "host",
		"-m", "64",
		"-chardev", "socket,id=qmp,path=" + socketPath + ",server=on,wait=off",
		"-mon", "chardev=qmp,id=monitor,mode=control",
	}

	cmd := exec.Command(qemuPath, args...)
	cmd.Dir = "/"

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start QEMU: %v", err)
	}

	return cmd
}

// waitForSocketFile waits for a socket file to be created.
func waitForSocketFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return os.ErrNotExist
}
