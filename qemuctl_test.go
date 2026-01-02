package qemuctl

import (
	"os"
	"runtime"
	"testing"
)

func TestLocateQemu(t *testing.T) {
	// Test with default arch
	path, err := LocateQemu("", "")
	if err != nil {
		t.Skipf("QEMU not found (this is OK if QEMU is not installed): %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
	t.Logf("Found QEMU at: %s", path)
}

func TestLocateQemuCustomPath(t *testing.T) {
	// Test with custom path that doesn't exist
	_, err := LocateQemu("amd64", "/nonexistent/qemu")
	if err != ErrQemuNotFound {
		t.Skipf("Expected ErrQemuNotFound for invalid custom path, got: %v", err)
	}
}

func TestLocateQemuInvalidArch(t *testing.T) {
	_, err := LocateQemu("invalid-arch", "")
	if err == nil {
		t.Error("expected error for invalid architecture")
	}

	var unsupportedErr *UnsupportedArchError
	if !matchError(err, &unsupportedErr) {
		t.Errorf("expected UnsupportedArchError, got %T", err)
	}
}

func TestQemuArchName(t *testing.T) {
	tests := []struct {
		goarch   string
		expected string
		ok       bool
	}{
		{"amd64", "x86_64", true},
		{"arm64", "aarch64", true},
		{"386", "i386", true},
		{"invalid", "", false},
	}

	for _, tt := range tests {
		name, ok := QemuArchName(tt.goarch)
		if ok != tt.ok {
			t.Errorf("QemuArchName(%q): expected ok=%v, got %v", tt.goarch, tt.ok, ok)
		}
		if name != tt.expected {
			t.Errorf("QemuArchName(%q): expected %q, got %q", tt.goarch, tt.expected, name)
		}
	}
}

func TestSupportedArches(t *testing.T) {
	arches := SupportedArches()
	if len(arches) == 0 {
		t.Error("expected at least one supported architecture")
	}

	// Check that common architectures are supported
	found := make(map[string]bool)
	for _, arch := range arches {
		found[arch] = true
	}

	expected := []string{"amd64", "arm64", "386"}
	for _, arch := range expected {
		if !found[arch] {
			t.Errorf("expected %q in supported architectures", arch)
		}
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateUnknown, "unknown"},
		{StateRunning, "running"},
		{StatePaused, "paused"},
		{StateShutdown, "shutdown"},
		{StateCrashed, "crashed"},
		{StateSuspended, "suspended"},
		{StatePrelaunch, "prelaunch"},
		{State(999), "State(999)"},
	}

	for _, tt := range tests {
		s := tt.state.String()
		if s != tt.expected {
			t.Errorf("State(%d).String(): expected %q, got %q", tt.state, tt.expected, s)
		}
	}
}

func TestStateIsAlive(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateUnknown, false},
		{StateRunning, true},
		{StatePaused, true},
		{StateShutdown, false},
		{StateCrashed, false},
		{StateSuspended, true},
		{StatePrelaunch, true},
	}

	for _, tt := range tests {
		alive := tt.state.IsAlive()
		if alive != tt.expected {
			t.Errorf("State(%s).IsAlive(): expected %v, got %v", tt.state, tt.expected, alive)
		}
	}
}

func TestParseQMPStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected State
	}{
		{"running", StateRunning},
		{"paused", StatePaused},
		{"shutdown", StateShutdown},
		{"suspended", StateSuspended},
		{"prelaunch", StatePrelaunch},
		{"inmigrate", StatePrelaunch},
		{"internal-error", StateCrashed},
		{"io-error", StateCrashed},
		{"unknown-status", StateUnknown},
	}

	for _, tt := range tests {
		state := parseQMPStatus(tt.status)
		if state != tt.expected {
			t.Errorf("parseQMPStatus(%q): expected %v, got %v", tt.status, tt.expected, state)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Memory != 512 {
		t.Errorf("expected default memory 512, got %d", cfg.Memory)
	}
	if cfg.CPUs != 1 {
		t.Errorf("expected default CPUs 1, got %d", cfg.CPUs)
	}
	if cfg.KVM == nil || !*cfg.KVM {
		t.Error("expected KVM enabled by default")
	}
	if cfg.NoDefaults == nil || !*cfg.NoDefaults {
		t.Error("expected NoDefaults enabled by default")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     &Config{Memory: 512, CPUs: 1},
			wantErr: false,
		},
		{
			name:    "zero memory",
			cfg:     &Config{Memory: 0, CPUs: 1},
			wantErr: true,
		},
		{
			name:    "negative memory",
			cfg:     &Config{Memory: -1, CPUs: 1},
			wantErr: true,
		},
		{
			name:    "zero CPUs",
			cfg:     &Config{Memory: 512, CPUs: 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultSocketDir(t *testing.T) {
	dir, err := defaultSocketDir()
	if err != nil {
		t.Fatalf("defaultSocketDir() error: %v", err)
	}

	if os.Getuid() == 0 {
		if dir != "/var/run/qemu" {
			t.Errorf("expected /var/run/qemu for root, got %s", dir)
		}
	} else {
		if dir == "" {
			t.Error("expected non-empty socket dir for user")
		}
	}
}

func TestBuildArgs(t *testing.T) {
	cfg := &Config{
		Name:   "test-vm",
		Memory: 1024,
		CPUs:   2,
	}

	args := buildArgs(cfg, "test-vm", "/tmp/test.sock")

	// Check for expected arguments
	expectedArgs := map[string]bool{
		"-name":    false,
		"-display": false,
		"-m":       false,
		"-smp":     false,
		"-chardev": false,
		"-mon":     false,
	}

	for _, arg := range args {
		if _, ok := expectedArgs[arg]; ok {
			expectedArgs[arg] = true
		}
	}

	for arg, found := range expectedArgs {
		if !found {
			t.Errorf("expected argument %s not found in args", arg)
		}
	}
}

func TestBuildArgsWithDrives(t *testing.T) {
	cfg := &Config{
		Memory: 512,
		CPUs:   1,
		Drives: []DriveConfig{
			{
				File:   "/path/to/disk.qcow2",
				Format: "qcow2",
			},
		},
	}

	args := buildArgs(cfg, "test", "/tmp/test.sock")

	foundDrive := false
	for i, arg := range args {
		if arg == "-drive" && i+1 < len(args) {
			foundDrive = true
			driveArg := args[i+1]
			if driveArg == "" {
				t.Error("drive argument is empty")
			}
		}
	}

	if !foundDrive {
		t.Error("expected -drive argument")
	}
}

func TestGenerateName(t *testing.T) {
	name1 := generateName()
	name2 := generateName()

	if name1 == "" || name2 == "" {
		t.Error("expected non-empty names")
	}

	if name1 == name2 {
		t.Error("expected different names for each call")
	}

	// Check format
	if len(name1) < 10 {
		t.Errorf("expected longer name, got %s", name1)
	}
}

func TestFindSocketFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name: "standard monitor socket",
			args: []string{
				"qemu-system-x86_64",
				"-chardev", "socket,id=qmp,path=/tmp/qemu.sock,server=on,wait=off",
				"-mon", "chardev=qmp,mode=control",
			},
			expected: "/tmp/qemu.sock",
		},
		{
			name: "chardev with monitor id",
			args: []string{
				"qemu-system-x86_64",
				"-chardev", "socket,id=charmonitor,path=/run/qemu/mon.sock,server=on,wait=off",
				"-mon", "chardev=charmonitor,mode=control",
			},
			expected: "/run/qemu/mon.sock",
		},
		{
			name: "no monitor socket",
			args: []string{
				"qemu-system-x86_64",
				"-m", "512",
			},
			expected: "",
		},
		{
			name: "non-monitor chardev",
			args: []string{
				"qemu-system-x86_64",
				"-chardev", "socket,id=serial0,path=/tmp/serial.sock,server=on,wait=off",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findSocketFromArgs(tt.args)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// matchError checks if err matches the target error type.
func matchError(err error, target any) bool {
	switch target.(type) {
	case **UnsupportedArchError:
		_, ok := err.(*UnsupportedArchError)
		return ok
	default:
		return false
	}
}

// Integration test - only runs if QEMU is installed
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if QEMU is available
	_, err := LocateQemu(runtime.GOARCH, "")
	if err != nil {
		t.Skipf("QEMU not found, skipping integration test: %v", err)
	}

	// Check if we're running as root or have KVM access
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("no KVM access, skipping integration test")
	}

	cfg := DefaultConfig()
	cfg.Name = "qemuctl-test"
	cfg.Memory = 128

	// Start without any drives - will fail to boot but tests QMP
	t.Log("Starting QEMU...")
	inst, err := Start(cfg)
	if err != nil {
		t.Fatalf("failed to start QEMU: %v", err)
	}

	defer func() {
		t.Log("Stopping QEMU...")
		if err := inst.ForceStop(); err != nil {
			t.Logf("ForceStop error (may be OK): %v", err)
		}
	}()

	// Test state query
	if err := inst.QueryState(); err != nil {
		t.Errorf("QueryState error: %v", err)
	}

	state := inst.State()
	t.Logf("VM state: %s", state)

	// Test pause/continue
	if state == StateRunning {
		if err := inst.Pause(); err != nil {
			t.Errorf("Pause error: %v", err)
		}

		if err := inst.QueryState(); err != nil {
			t.Errorf("QueryState after pause error: %v", err)
		}

		if inst.State() != StatePaused {
			t.Errorf("expected paused state, got %s", inst.State())
		}

		if err := inst.Continue(); err != nil {
			t.Errorf("Continue error: %v", err)
		}
	}

	// Test human monitor command
	output, err := inst.HumanMonitorCommand("info version")
	if err != nil {
		t.Errorf("HumanMonitorCommand error: %v", err)
	}
	t.Logf("QEMU version: %s", output)
}
