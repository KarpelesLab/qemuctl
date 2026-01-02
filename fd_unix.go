//go:build unix

package qemuctl

import "syscall"

// dupFd duplicates a file descriptor.
func dupFd(fd int) (int, error) {
	return syscall.Dup(fd)
}

// closeFd closes a file descriptor.
func closeFd(fd int) error {
	return syscall.Close(fd)
}
