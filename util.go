package qemuctl

import (
	"encoding/json"
)

// unmarshalJSON is a helper to unmarshal JSON data into a struct.
func unmarshalJSON(data json.RawMessage, v any) error {
	return json.Unmarshal(data, v)
}

// BlockInfo represents information about a block device.
type BlockInfo struct {
	Device   string `json:"device"`
	NodeName string `json:"node-name"`
	QDev     string `json:"qdev,omitempty"`
	Type     string `json:"type"`
	Inserted *struct {
		File             string `json:"file"`
		NodeName         string `json:"node-name"`
		Ro               bool   `json:"ro"`
		Drv              string `json:"drv"`
		BackingFile      string `json:"backing_file,omitempty"`
		BackingFileDepth int    `json:"backing_file_depth"`
		Encrypted        bool   `json:"encrypted"`
		DetectZeroes     string `json:"detect_zeroes"`
		Bps              int64  `json:"bps"`
		BpsRd            int64  `json:"bps_rd"`
		BpsWr            int64  `json:"bps_wr"`
		Iops             int64  `json:"iops"`
		IopsRd           int64  `json:"iops_rd"`
		IopsWr           int64  `json:"iops_wr"`
	} `json:"inserted,omitempty"`
}

// QueryBlockDevices returns information about all block devices.
func (i *Instance) QueryBlockDevices() ([]BlockInfo, error) {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return nil, ErrNotConnected
	}

	result, err := qmp.Execute("query-block", nil)
	if err != nil {
		return nil, err
	}

	var blocks []BlockInfo
	if err := unmarshalJSON(result, &blocks); err != nil {
		return nil, err
	}

	return blocks, nil
}

// SetIOThrottle sets I/O throttling for a block device.
func (i *Instance) SetIOThrottle(device string, bps, iops uint64) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return ErrNotConnected
	}

	_, err := qmp.Execute("block_set_io_throttle", map[string]any{
		"id":              device,
		"bps":             bps,
		"bps_rd":          0,
		"bps_wr":          0,
		"iops":            iops,
		"iops_rd":         0,
		"iops_wr":         0,
		"bps_max":         bps * 8,
		"iops_max":        iops * 8,
		"bps_max_length":  60,
		"iops_max_length": 60,
	})
	return err
}

// Screendump captures the screen to a file.
func (i *Instance) Screendump(filename string) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return ErrNotConnected
	}

	_, err := qmp.Execute("screendump", map[string]any{
		"filename": filename,
	})
	return err
}

// SendKey sends a key event to the guest.
// Keys should be in QEMU key format (e.g., "ctrl-alt-delete").
func (i *Instance) SendKey(keys ...string) error {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return ErrNotConnected
	}

	keyList := make([]map[string]any, len(keys))
	for idx, key := range keys {
		keyList[idx] = map[string]any{
			"type": "qcode",
			"data": key,
		}
	}

	_, err := qmp.Execute("send-key", map[string]any{
		"keys": keyList,
	})
	return err
}

// HumanMonitorCommand executes a human monitor command.
// This is useful for commands not exposed via QMP.
func (i *Instance) HumanMonitorCommand(cmd string) (string, error) {
	i.qmpMu.Lock()
	qmp := i.qmp
	i.qmpMu.Unlock()

	if qmp == nil {
		return "", ErrNotConnected
	}

	result, err := qmp.Execute("human-monitor-command", map[string]any{
		"command-line": cmd,
	})
	if err != nil {
		return "", err
	}

	var output string
	if err := unmarshalJSON(result, &output); err != nil {
		return "", err
	}

	return output, nil
}
