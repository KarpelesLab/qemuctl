package qemuctl

import (
	"fmt"
	"net"
	"os"
	"time"
)

// AddVNCClient passes a client connection to QEMU for VNC.
// The connection will be taken over by QEMU for VNC protocol.
// The skipAuth parameter controls whether VNC authentication is skipped.
func (i *Instance) AddVNCClient(conn net.Conn, skipAuth bool) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	fd, err := connToFd(conn)
	if err != nil {
		return err
	}
	defer closeFd(fd)

	return i.addClientFd(qmp, "vnc", fd, skipAuth)
}

// AddSpiceClient passes a client connection to QEMU for SPICE.
// The connection will be taken over by QEMU for SPICE protocol.
// The skipAuth parameter controls whether SPICE authentication is skipped.
func (i *Instance) AddSpiceClient(conn net.Conn, skipAuth bool) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	fd, err := connToFd(conn)
	if err != nil {
		return err
	}
	defer closeFd(fd)

	return i.addClientFd(qmp, "spice", fd, skipAuth)
}

// addClientFd sends a file descriptor to QEMU and registers it as a client.
func (i *Instance) addClientFd(qmp *QMP, protocol string, fd int, skipAuth bool) error {
	// Generate unique fd name
	fdName := fmt.Sprintf("%s-client-%d", protocol, time.Now().UnixNano())

	// First, pass the fd to QEMU with getfd command
	_, err := qmp.ExecuteWithFd("getfd", map[string]any{
		"fdname": fdName,
	}, fd)
	if err != nil {
		return fmt.Errorf("failed to pass fd: %w", err)
	}

	// Then add the client
	_, err = qmp.Execute("add_client", map[string]any{
		"protocol": protocol,
		"fdname":   fdName,
		"skipauth": skipAuth,
		"tls":      false,
	})
	if err != nil {
		return fmt.Errorf("failed to add client: %w", err)
	}

	return nil
}

// SetVNCPassword sets the VNC password.
func (i *Instance) SetVNCPassword(password string) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	_, err := qmp.Execute("set_password", map[string]any{
		"protocol": "vnc",
		"password": password,
	})
	return err
}

// SetSpicePassword sets the SPICE password.
func (i *Instance) SetSpicePassword(password string) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	_, err := qmp.Execute("set_password", map[string]any{
		"protocol": "spice",
		"password": password,
	})
	return err
}

// ExpireVNCPassword expires the VNC password at a given time.
// Use "now" to expire immediately, "never" for no expiration.
func (i *Instance) ExpireVNCPassword(expireTime string) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	_, err := qmp.Execute("expire_password", map[string]any{
		"protocol": "vnc",
		"time":     expireTime,
	})
	return err
}

// ExpireSpicePassword expires the SPICE password at a given time.
func (i *Instance) ExpireSpicePassword(expireTime string) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return fmt.Errorf("not connected")
	}

	_, err := qmp.Execute("expire_password", map[string]any{
		"protocol": "spice",
		"time":     expireTime,
	})
	return err
}

// connToFd extracts the file descriptor from a net.Conn.
func connToFd(conn net.Conn) (int, error) {
	fileConn, ok := conn.(interface{ File() (*os.File, error) })
	if !ok {
		return 0, fmt.Errorf("connection does not support File() method")
	}

	f, err := fileConn.File()
	if err != nil {
		return 0, fmt.Errorf("failed to get file descriptor: %w", err)
	}

	// Dup the fd so we own it
	fd, err := dupFd(int(f.Fd()))
	f.Close()
	if err != nil {
		return 0, fmt.Errorf("failed to dup fd: %w", err)
	}

	return fd, nil
}
