package qemuctl

import "fmt"

// State represents the current state of a QEMU instance.
type State int

const (
	StateUnknown State = iota
	StateRunning
	StatePaused
	StateShutdown
	StateCrashed
	StateSuspended
	StatePrelaunch // VM is being initialized but not yet running
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	case StateShutdown:
		return "shutdown"
	case StateCrashed:
		return "crashed"
	case StateSuspended:
		return "suspended"
	case StatePrelaunch:
		return "prelaunch"
	default:
		return fmt.Sprintf("State(%d)", s)
	}
}

// IsAlive returns true if the state indicates the VM is alive (running or paused).
func (s State) IsAlive() bool {
	return s == StateRunning || s == StatePaused || s == StateSuspended || s == StatePrelaunch
}

// parseQMPStatus converts a QMP status string to State.
func parseQMPStatus(status string) State {
	switch status {
	case "running":
		return StateRunning
	case "paused":
		return StatePaused
	case "shutdown":
		return StateShutdown
	case "suspended":
		return StateSuspended
	case "prelaunch":
		return StatePrelaunch
	case "inmigrate":
		return StatePrelaunch
	case "internal-error", "io-error":
		return StateCrashed
	default:
		return StateUnknown
	}
}
