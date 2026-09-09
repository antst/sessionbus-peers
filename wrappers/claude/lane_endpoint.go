// SPDX-License-Identifier: MIT
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
)

type laneEndpoint struct {
	dir, path   string
	listener    net.Listener
	mu          sync.Mutex
	connections map[net.Conn]struct{}
	closed      bool
}

func newLaneEndpoint(p *Wrapper) (*laneEndpoint, error) {
	dir, err := os.MkdirTemp("", "sessionbus-claude-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "owner.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	e := &laneEndpoint{dir: dir, path: path, listener: listener, connections: make(map[net.Conn]struct{})}
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			e.mu.Lock()
			if e.closed {
				e.mu.Unlock()
				_ = c.Close()
				return
			}
			e.connections[c] = struct{}{}
			e.mu.Unlock()
			go func() {
				ctx, cancel := context.WithCancel(p.ctx)
				backend := &laneBackend{owner: p, ctx: ctx, cancel: cancel}
				_ = interactive.Serve(backend, c, c)
				_ = c.Close()
				e.mu.Lock()
				delete(e.connections, c)
				e.mu.Unlock()
			}()
		}
	}()
	return e, nil
}
func (e *laneEndpoint) close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	_ = e.listener.Close()
	for c := range e.connections {
		_ = c.Close()
	}
	e.mu.Unlock()
	_ = os.RemoveAll(e.dir)
}

type laneBackend struct {
	owner   *Wrapper
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	initial bool
}
type nativeReport struct {
	Event   string          `json:"hook_event_name"`
	ID      string          `json:"session_id"`
	Title   *string         `json:"session_title"`
	Agent   json.RawMessage `json:"agent_id,omitempty"`
	Emitter int             `json:"emitter_pid"`
}

func (b *laneBackend) BeginReport(raw json.RawMessage) (<-chan error, error) {
	var report nativeReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, err
	}
	if report.Event == "SessionStart" {
		b.mu.Lock()
		b.initial = true
		b.mu.Unlock()
	}
	if report.ID == "" || len(report.Agent) > 0 {
		return nil, errors.New("native root report required")
	}
	p := b.owner
	if report.Event == "SessionStart" {
		result := make(chan error, 1)
		go func() {
			select {
			case <-p.spawnReady:
			case <-b.ctx.Done():
				result <- b.ctx.Err()
				return
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.closing {
				result <- errors.New("lane closing")
				return
			}
			if p.native == nil || report.Emitter != p.native.Process.Pid {
				result <- errors.New("foreign native report emitter")
				return
			}
			if p.identity != "" && p.identity != report.ID {
				result <- errors.New("native identity changed during lane lifetime")
				go p.fail(errors.New("native identity changed"))
				return
			}
			p.identity = report.ID
			if report.Title != nil {
				p.title = *report.Title
			}
			p.reportOnce.Do(func() { close(p.reportReady) })
			result <- nil
		}()
		return result, nil
	}
	if report.Event != "UserPromptSubmit" && report.Event != "Stop" && report.Event != "SessionEnd" {
		return nil, errors.New("unsupported native report event")
	}
	p.mu.Lock()
	if p.closing || p.identity == "" || report.ID != p.identity {
		p.mu.Unlock()
		return nil, errors.New("native report does not match this lane")
	}
	if report.Title != nil && *report.Title != "" {
		p.title = *report.Title
	}
	ended := report.Event == "SessionEnd"
	p.mu.Unlock()
	if ended {
		p.fail(errors.New("native session ended"))
	}
	return nil, nil
}
func (b *laneBackend) Action(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	if b.ctx.Err() != nil {
		return nil, b.ctx.Err()
	}
	if _, _, err := b.owner.current(); err != nil {
		return nil, err
	}
	return b.owner.caller.Action(ctx, action, args)
}
func (b *laneBackend) End() {
	b.cancel()
	b.mu.Lock()
	initial := b.initial
	b.mu.Unlock()
	if !initial {
		b.owner.fail(errors.New("native MCP forwarder disconnected"))
	}
}

// Forward uses the native MCP byte stream unchanged. The only Caller and report
// authority remain inside the worker; the native child never holds its token.
func Forward(ctx context.Context, path string, input io.ReadCloser, output io.Writer) error {
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return err
	}
	defer c.Close()
	sent := make(chan error, 1)
	received := make(chan error, 1)
	go func() {
		_, e := io.Copy(c, input)
		if unix, ok := c.(*net.UnixConn); ok {
			_ = unix.CloseWrite()
		}
		sent <- e
	}()
	go func() { _, e := io.Copy(output, c); received <- e }()
	select {
	case err = <-received:
	case <-ctx.Done():
		err = ctx.Err()
	}
	_ = c.Close()
	_ = input.Close()
	<-sent
	return err
}
func InitialReport(ctx context.Context, path, pid string, input io.Reader) error {
	var report nativeReport
	if err := json.NewDecoder(input).Decode(&report); err != nil {
		return err
	}
	emitter, err := strconv.Atoi(pid)
	if err != nil || emitter <= 0 {
		return errors.New("native CLAUDE_PID is required")
	}
	report.Emitter = emitter
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return err
	}
	defer c.Close()
	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": interactive.HiddenTool, "arguments": report}}
	if err = json.NewEncoder(c).Encode(request); err != nil {
		return err
	}
	var reply struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err = json.NewDecoder(c).Decode(&reply); err != nil {
		return err
	}
	if len(reply.Error) > 0 || reply.Result.IsError {
		return errors.New("initial native report was not admitted")
	}
	return nil
}
