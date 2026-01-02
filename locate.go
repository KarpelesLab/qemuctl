package qemuctl

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrQemuNotFound is returned when QEMU cannot be located.
var ErrQemuNotFound = errors.New("QEMU binary not found")

// qemuSearchPaths are additional paths to search for QEMU binaries.
var qemuSearchPaths = []string{
	"/pkg/main/app-emulation.qemu.core/bin",
	"/usr/bin",
	"/usr/local/bin",
}

// archToQemu maps GOARCH values to QEMU binary suffixes.
var archToQemu = map[string]string{
	"amd64":   "x86_64",
	"386":     "i386",
	"arm64":   "aarch64",
	"arm":     "arm",
	"riscv64": "riscv64",
	"ppc64":   "ppc64",
	"ppc64le": "ppc64",
	"mips":    "mips",
	"mips64":  "mips64",
	"s390x":   "s390x",
}

// LocateQemu finds the QEMU system emulator for the given architecture.
// It searches in the following order:
// 1. If customPath is provided and valid, use it
// 2. Search in PATH
// 3. Search in /pkg/main/app-emulation.qemu.core/bin/
// 4. Search in common system paths
//
// The arch parameter should be a GOARCH-style value (e.g., "amd64", "arm64").
// If arch is empty, it defaults to runtime.GOARCH.
func LocateQemu(arch string, customPath string) (string, error) {
	if arch == "" {
		arch = runtime.GOARCH
	}

	qemuArch, ok := archToQemu[arch]
	if !ok {
		return "", &UnsupportedArchError{Arch: arch}
	}

	binaryName := "qemu-system-" + qemuArch

	// 1. Check custom path
	if customPath != "" {
		if info, err := os.Stat(customPath); err == nil && !info.IsDir() {
			return customPath, nil
		}
		// Check if it's a directory containing the binary
		fullPath := filepath.Join(customPath, binaryName)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return fullPath, nil
		}
	}

	// 2. Search in PATH
	if path, err := exec.LookPath(binaryName); err == nil {
		return path, nil
	}

	// 3. Search in known paths
	for _, dir := range qemuSearchPaths {
		fullPath := filepath.Join(dir, binaryName)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return fullPath, nil
		}
	}

	return "", ErrQemuNotFound
}

// UnsupportedArchError is returned when the architecture is not supported.
type UnsupportedArchError struct {
	Arch string
}

func (e *UnsupportedArchError) Error() string {
	return "unsupported architecture: " + e.Arch
}

// SupportedArches returns a list of supported GOARCH values.
func SupportedArches() []string {
	arches := make([]string, 0, len(archToQemu))
	for arch := range archToQemu {
		arches = append(arches, arch)
	}
	return arches
}

// QemuArchName converts a GOARCH value to the QEMU architecture name.
func QemuArchName(goarch string) (string, bool) {
	name, ok := archToQemu[goarch]
	return name, ok
}
