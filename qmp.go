package qemuctl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// QMP handles the QEMU Monitor Protocol connection.
type QMP struct {
	conn       net.Conn
	connMu     sync.Mutex
	reader     *bufio.Reader
	cmdCounter atomic.Uint64

	// Command response routing
	pending   map[string]chan *qmpResponse
	pendingMu sync.Mutex

	// Event handling
	eventCh chan *Event
	closeCh chan struct{}

	// Callbacks
	onStateChange func(State)
	onEvent       func(*Event)
}

// Event represents a QMP event from QEMU.
type Event struct {
	Name      string
	Data      map[string]any
	Timestamp time.Time
}

// QMP message types
type qmpCommand struct {
	Execute   string         `json:"execute"`
	Arguments map[string]any `json:"arguments,omitempty"`
	ID        string         `json:"id,omitempty"`
}

type qmpResponse struct {
	Return    json.RawMessage `json:"return,omitempty"`
	Error     *qmpError       `json:"error,omitempty"`
	Event     string          `json:"event,omitempty"`
	Data      map[string]any  `json:"data,omitempty"`
	Timestamp *qmpTimestamp   `json:"timestamp,omitempty"`
	ID        string          `json:"id,omitempty"`
}

type qmpError struct {
	Class string `json:"class"`
	Desc  string `json:"desc"`
}

type qmpTimestamp struct {
	Seconds      int64 `json:"seconds"`
	Microseconds int64 `json:"microseconds"`
}

// QMPError is returned when QEMU reports an error.
type QMPError struct {
	Class       string
	Description string
}

func (e *QMPError) Error() string {
	return fmt.Sprintf("QMP error [%s]: %s", e.Class, e.Description)
}

// newQMP creates a new QMP connection to the given socket path.
func newQMP(socketPath string) (*QMP, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to QMP socket: %w", err)
	}

	q := &QMP{
		conn:    conn,
		pending: make(map[string]chan *qmpResponse),
		eventCh: make(chan *Event, 100),
		closeCh: make(chan struct{}),
	}

	// Read QMP greeting (must be done before starting event loop)
	if err := q.readGreeting(); err != nil {
		conn.Close()
		return nil, err
	}

	// Start event loop
	q.reader = bufio.NewReader(conn)
	go q.eventLoop()

	// Send qmp_capabilities to enter command mode
	if err := q.negotiate(); err != nil {
		q.Close()
		return nil, err
	}

	return q, nil
}

// readGreeting reads the initial QMP greeting message.
func (q *QMP) readGreeting() error {
	// Read one byte at a time to avoid buffering data the event loop needs
	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := q.conn.Read(b)
		if err != nil {
			return fmt.Errorf("failed to read QMP greeting: %w", err)
		}
		if n > 0 {
			buf = append(buf, b[0])
			if b[0] == '\n' {
				break
			}
		}
	}

	var greeting struct {
		QMP struct {
			Version struct {
				Qemu struct {
					Major int `json:"major"`
					Minor int `json:"minor"`
					Micro int `json:"micro"`
				} `json:"qemu"`
			} `json:"version"`
			Capabilities []string `json:"capabilities"`
		} `json:"QMP"`
	}

	if err := json.Unmarshal(buf, &greeting); err != nil {
		return fmt.Errorf("failed to parse QMP greeting: %w", err)
	}

	return nil
}

// negotiate sends qmp_capabilities to enter command mode.
func (q *QMP) negotiate() error {
	_, err := q.Execute("qmp_capabilities", nil)
	return err
}

// Execute sends a QMP command and waits for the response.
func (q *QMP) Execute(command string, args map[string]any) (json.RawMessage, error) {
	return q.ExecuteWithTimeout(command, args, 30*time.Second)
}

// ExecuteWithTimeout sends a QMP command with a custom timeout.
func (q *QMP) ExecuteWithTimeout(command string, args map[string]any, timeout time.Duration) (json.RawMessage, error) {
	cmdID := fmt.Sprintf("cmd-%d", q.cmdCounter.Add(1))

	cmd := qmpCommand{
		Execute:   command,
		Arguments: args,
		ID:        cmdID,
	}

	// Create response channel
	respCh := make(chan *qmpResponse, 1)

	// Register pending command
	q.pendingMu.Lock()
	if q.pending == nil {
		q.pendingMu.Unlock()
		return nil, fmt.Errorf("QMP connection closed")
	}
	q.pending[cmdID] = respCh
	q.pendingMu.Unlock()

	// Cleanup on exit
	defer func() {
		q.pendingMu.Lock()
		delete(q.pending, cmdID)
		q.pendingMu.Unlock()
	}()

	// Send command
	q.connMu.Lock()
	if q.conn == nil {
		q.connMu.Unlock()
		return nil, fmt.Errorf("QMP connection closed")
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		q.connMu.Unlock()
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	if _, err := q.conn.Write(append(data, '\n')); err != nil {
		q.connMu.Unlock()
		return nil, fmt.Errorf("failed to write command: %w", err)
	}
	q.connMu.Unlock()

	// Wait for response
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, &QMPError{
				Class:       resp.Error.Class,
				Description: resp.Error.Desc,
			}
		}
		return resp.Return, nil
	case <-timer.C:
		return nil, fmt.Errorf("command %q timeout after %v", command, timeout)
	case <-q.closeCh:
		return nil, fmt.Errorf("QMP connection closed")
	}
}

// ExecuteWithFd sends a QMP command with a file descriptor via SCM_RIGHTS.
func (q *QMP) ExecuteWithFd(command string, args map[string]any, fd int) (json.RawMessage, error) {
	cmdID := fmt.Sprintf("cmd-%d", q.cmdCounter.Add(1))

	cmd := qmpCommand{
		Execute:   command,
		Arguments: args,
		ID:        cmdID,
	}

	// Create response channel
	respCh := make(chan *qmpResponse, 1)

	// Register pending command
	q.pendingMu.Lock()
	if q.pending == nil {
		q.pendingMu.Unlock()
		return nil, fmt.Errorf("QMP connection closed")
	}
	q.pending[cmdID] = respCh
	q.pendingMu.Unlock()

	// Cleanup on exit
	defer func() {
		q.pendingMu.Lock()
		delete(q.pending, cmdID)
		q.pendingMu.Unlock()
	}()

	// Send command with SCM_RIGHTS
	q.connMu.Lock()
	if q.conn == nil {
		q.connMu.Unlock()
		return nil, fmt.Errorf("QMP connection closed")
	}

	unixConn, ok := q.conn.(*net.UnixConn)
	if !ok {
		q.connMu.Unlock()
		return nil, fmt.Errorf("QMP connection is not a Unix socket")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		q.connMu.Unlock()
		return nil, fmt.Errorf("failed to get raw connection: %w", err)
	}

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		q.connMu.Unlock()
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	var sendErr error
	err = rawConn.Control(func(sockfd uintptr) {
		rights := syscall.UnixRights(fd)
		sendErr = syscall.Sendmsg(int(sockfd), append(cmdData, '\n'), rights, nil, 0)
	})
	q.connMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to control raw connection: %w", err)
	}
	if sendErr != nil {
		return nil, fmt.Errorf("failed to send fd via SCM_RIGHTS: %w", sendErr)
	}

	// Wait for response
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, &QMPError{
				Class:       resp.Error.Class,
				Description: resp.Error.Desc,
			}
		}
		return resp.Return, nil
	case <-timer.C:
		return nil, fmt.Errorf("command %q timeout", command)
	case <-q.closeCh:
		return nil, fmt.Errorf("QMP connection closed")
	}
}

// eventLoop reads and dispatches QMP events and command responses.
func (q *QMP) eventLoop() {
	defer func() {
		close(q.closeCh)
		close(q.eventCh)

		// Clear pending commands
		q.pendingMu.Lock()
		q.pending = nil
		q.pendingMu.Unlock()
	}()

	for {
		line, err := q.reader.ReadBytes('\n')
		if err != nil {
			return
		}

		var resp qmpResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		if resp.ID != "" {
			// Command response
			q.pendingMu.Lock()
			if ch, ok := q.pending[resp.ID]; ok {
				select {
				case ch <- &resp:
				default:
				}
			}
			q.pendingMu.Unlock()
		} else if resp.Event != "" {
			// Event
			event := &Event{
				Name: resp.Event,
				Data: resp.Data,
			}
			if resp.Timestamp != nil {
				event.Timestamp = time.Unix(resp.Timestamp.Seconds, resp.Timestamp.Microseconds*1000)
			} else {
				event.Timestamp = time.Now()
			}

			// Handle state change events
			q.handleEvent(event)

			// Send to event channel
			select {
			case q.eventCh <- event:
			default:
			}

			// Call event callback if set
			if q.onEvent != nil {
				q.onEvent(event)
			}
		}
	}
}

// handleEvent processes QMP events for state tracking.
func (q *QMP) handleEvent(event *Event) {
	if q.onStateChange == nil {
		return
	}

	var newState State
	switch event.Name {
	case "SHUTDOWN":
		newState = StateShutdown
	case "RESET":
		newState = StateRunning
	case "STOP":
		newState = StatePaused
	case "RESUME":
		newState = StateRunning
	case "SUSPEND":
		newState = StateSuspended
	case "WAKEUP":
		newState = StateRunning
	default:
		return
	}

	q.onStateChange(newState)
}

// Events returns the event channel.
func (q *QMP) Events() <-chan *Event {
	return q.eventCh
}

// SetEventCallback sets a callback for all events.
func (q *QMP) SetEventCallback(cb func(*Event)) {
	q.onEvent = cb
}

// SetStateChangeCallback sets a callback for state changes.
func (q *QMP) SetStateChangeCallback(cb func(State)) {
	q.onStateChange = cb
}

// Close closes the QMP connection.
func (q *QMP) Close() error {
	q.connMu.Lock()
	defer q.connMu.Unlock()

	if q.conn != nil {
		err := q.conn.Close()
		q.conn = nil
		return err
	}
	return nil
}
