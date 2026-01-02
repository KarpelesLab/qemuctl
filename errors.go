package qemuctl

import "errors"

// Common errors.
var (
	ErrNotConnected = errors.New("not connected to QEMU")
	ErrNotRunning   = errors.New("QEMU process not running")
)
