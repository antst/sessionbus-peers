// SPDX-License-Identifier: MIT
package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// The registry is a lagging native witness, not an ordered hook stream. Use it
// only until the first valid hook; it must never revive a SessionEnd identity.
const startupPollInterval = 250 * time.Millisecond
const nativeRegistryLimit = 64 << 10

var errStartupParentEnded = errors.New("native Claude parent generation ended")

type startupIdentity struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Name       string `json:"name"`
	NameSource string `json:"nameSource"`
	StartedAt  int64  `json:"startedAt"`
	ProcStart  string `json:"procStart"`
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
	Socket     string `json:"messagingSocketPath"`
}

type startupProcess struct {
	start, generation string
	startedMillis     int64
}

type startupReader struct {
	path, socket string
	pid          int
	process      startupProcess
	inspect      func(int) (startupProcess, error)
}

func newStartupReader(env map[string]string) (*startupReader, error) {
	// These are native MCP-child markers, not identity authority. The identity
	// comes exclusively from the exact live parent's registry record.
	if env["CLAUDECODE"] != "1" || env["CLAUDE_CODE_SESSION_ID"] == "" {
		return nil, errors.New("not a native Claude MCP child")
	}
	pid := os.Getppid()
	if pid <= 1 {
		return nil, errors.New("native Claude parent is absent")
	}
	process, err := inspectStartupProcess(pid)
	if err != nil {
		return nil, err
	}
	root := env["CLAUDE_CONFIG_DIR"]
	if root == "" {
		home := env["HOME"]
		if home == "" {
			home, err = os.UserHomeDir()
		}
		if err != nil || home == "" {
			return nil, errors.New("native configuration directory unavailable")
		}
		root = filepath.Join(home, ".claude")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &startupReader{path: filepath.Join(root, "sessions", fmt.Sprintf("%d.json", pid)), socket: env["CLAUDE_CODE_MESSAGING_SOCKET"], pid: pid, process: process, inspect: inspectStartupProcess}, nil
}

func (r *startupReader) read() (startupIdentity, error) {
	var row startupIdentity
	before, err := r.inspect(r.pid)
	if err != nil || before != r.process {
		return row, errStartupParentEnded
	}
	info, err := os.Lstat(r.path)
	if err != nil {
		return row, err
	}
	if !info.Mode().IsRegular() || info.Size() > nativeRegistryLimit {
		return row, errors.New("invalid native registry file")
	}
	file, err := os.Open(r.path)
	if err != nil {
		return row, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return row, errors.New("native registry file changed during open")
	}
	body, err := io.ReadAll(io.LimitReader(file, nativeRegistryLimit+1))
	if err != nil {
		return row, err
	}
	if len(body) > nativeRegistryLimit || json.Unmarshal(body, &row) != nil {
		return row, errors.New("native registry is incomplete")
	}
	if row.PID != r.pid || row.ProcStart != before.start || row.SessionID == "" || row.Kind != "interactive" || row.Entrypoint != "cli" {
		return row, errors.New("native registry does not identify the live Claude parent")
	}
	if before.startedMillis > 0 && row.StartedAt < before.startedMillis {
		return row, errors.New("native registry predates the current parent generation")
	}
	// Claude's cwd-derived PID label is not a persisted session title. Preserve
	// unnamed publication until the native name write replaces that label.
	if row.NameSource != "" && row.NameSource != "user" {
		row.Name = ""
	}
	if !filepath.IsAbs(row.Socket) || r.socket != "" && row.Socket != r.socket {
		return row, errors.New("native registry socket differs")
	}
	socket, err := os.Lstat(row.Socket)
	if err != nil || socket.Mode()&os.ModeSocket == 0 {
		return row, errors.New("native messaging socket is not ready")
	}
	after, err := r.inspect(r.pid)
	if err != nil || after != before {
		return row, errStartupParentEnded
	}
	return row, nil
}

// StartStartupObservation adds no process or endpoint. If the native registry
// backend is unavailable, the existing native hooks remain the sole authority.
func (o *Owner) StartStartupObservation(env map[string]string) {
	r, err := newStartupReader(env)
	if err != nil {
		return
	}
	o.startStartupObservation(r.read, startupPollInterval)
}

func (o *Owner) startStartupObservation(read func() (startupIdentity, error), interval time.Duration) {
	o.mu.Lock()
	if o.ended || o.hookObserved || o.startupCancel != nil {
		o.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	o.startupCancel = cancel
	o.startupWork.Add(1)
	o.mu.Unlock()
	go func() {
		defer o.startupWork.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var last startupIdentity
		for {
			if ctx.Err() != nil {
				return
			}
			row, err := read()
			if errors.Is(err, errStartupParentEnded) {
				o.mu.Lock()
				if !o.hookObserved && !o.ended {
					o.ended = true
					o.withdrawLocked(unavailable())
				}
				o.mu.Unlock()
				return
			}
			if err == nil && row != last {
				o.mu.Lock()
				if o.ended || o.hookObserved || ctx.Err() != nil {
					o.mu.Unlock()
					return
				}
				o.nativeSocket = row.Socket
				// No fabricated hook is emitted. Both native sources share the
				// same publication transition under the owner mutex.
				_, err = o.beginIdentityLocked(NativeReport{ID: row.SessionID, Title: row.Name})
				if err == nil {
					last = row
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
