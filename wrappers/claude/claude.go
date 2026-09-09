// SPDX-License-Identifier: MIT
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

const Product = "claude-peer"
const LaneEndpointEnv = interactive.LaneEndpointEnv
const HookAlias = "sessionbus-hook"

type Wrapper struct {
	mu              sync.Mutex
	caller          *kit.Caller
	root            string
	cancel          context.CancelFunc
	ctx             context.Context
	endpoint        *laneEndpoint
	native          *exec.Cmd
	stream          *stream
	processDone     chan struct{}
	spawnReady      chan struct{}
	reportReady     chan struct{}
	reportOnce      sync.Once
	identity, title string
	opened, closing bool
	failure         error
	shutdown        func()
	activeDone      <-chan struct{}
}

func New(root string) *Wrapper {
	return &Wrapper{root: root, spawnReady: make(chan struct{}), reportReady: make(chan struct{})}
}
func (p *Wrapper) SetCaller(c *kit.Caller) { p.caller = c }
func (p *Wrapper) SetShutdown(f func())    { p.shutdown = f }
func (*Wrapper) Hello(context.Context) (kit.HelloDescription, error) {
	return kit.HelloDescription{Product: Product, ExtraArguments: []kit.ExtraArgument{}, SupportedOpenFields: []string{"cwd", "permission_mode", "model", "reasoning_effort", "arguments"}}, nil
}
func (p *Wrapper) Open(ctx context.Context, r kit.OpenRequest) (result kit.OpenResult, err error) {
	p.mu.Lock()
	if p.ctx != nil || p.closing {
		p.mu.Unlock()
		return result, errors.New("Claude worker already opened or closed")
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	nativeCtx := p.ctx
	p.mu.Unlock()
	defer func() {
		if err != nil {
			_ = p.Close(context.Background(), kit.SessionCloseRequest{})
		}
	}()
	endpoint, err := newLaneEndpoint(p)
	if err != nil {
		return result, err
	}
	p.mu.Lock()
	p.endpoint = endpoint
	p.mu.Unlock()
	settings := filepath.Join(endpoint.dir, "session-start.json")
	command := shellQuote(filepath.Join(p.root, "bin", HookAlias))
	config := map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"hooks": []any{map[string]string{"type": "command", "command": command}}}}}}
	body, err := json.Marshal(config)
	if err != nil {
		return result, err
	}
	if err = os.WriteFile(settings, body, 0600); err != nil {
		return result, err
	}
	args := launchArguments(r, p.root, settings)
	cwd := r.Open.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return result, err
		}
	}
	env := interactive.Environment(os.Environ())
	path, err := interactive.NativePath(env, cwd)
	if err != nil {
		return result, err
	}
	child := exec.CommandContext(nativeCtx, path, args...)
	child.Dir = cwd
	child.Stderr = os.Stderr
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, LaneEndpointEnv+"=") && !strings.HasPrefix(value, "SESSIONBUS_LAUNCH_TOKEN=") {
			child.Env = append(child.Env, value)
		}
	}
	child.Env = append(child.Env, LaneEndpointEnv+"="+endpoint.path)
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		return result, err
	}
	defer inputRead.Close()
	outputRead, outputWrite, err := os.Pipe()
	if err != nil {
		_ = inputWrite.Close()
		return result, err
	}
	defer outputWrite.Close()
	child.Stdin = inputRead
	child.Stdout = outputWrite
	if err = child.Start(); err != nil {
		_ = inputWrite.Close()
		_ = outputRead.Close()
		return result, err
	}
	p.mu.Lock()
	p.native = child
	p.processDone = make(chan struct{})
	p.mu.Unlock()
	close(p.spawnReady)
	_ = inputRead.Close()
	_ = outputWrite.Close()
	s := newStream(inputWrite, outputRead, p.fail)
	p.mu.Lock()
	p.stream = s
	processDone := p.processDone
	p.mu.Unlock()
	go waitNative(child, processDone)
	init, err := s.control(nativeCtx, map[string]string{"subtype": "initialize"})
	if err != nil {
		return result, err
	}
	var initialized struct {
		PID int `json:"pid"`
	}
	if err = json.Unmarshal(init, &initialized); err != nil {
		return result, err
	}
	if initialized.PID != child.Process.Pid {
		return result, errors.New("native initialize did not confirm the spawned process")
	}
	select {
	case <-p.reportReady:
	case <-nativeCtx.Done():
		return result, nativeCtx.Err()
	}
	status, err := s.control(nativeCtx, map[string]string{"subtype": "mcp_status"})
	if err != nil {
		return result, err
	}
	if err = requiredTools(status); err != nil {
		return result, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing || p.failure != nil {
		return result, errors.New("native integration ended during open")
	}
	if r.ResumeSessionID != "" && r.ResumeSessionID != p.identity {
		return result, errors.New("native report did not confirm the requested resume identity")
	}
	if err := confirmTitle(r.Name, p.title); err != nil {
		return result, err
	}
	p.opened = true
	return kit.OpenResult{SessionID: p.identity}, nil
}
func launchArguments(r kit.OpenRequest, root, settings string) []string {
	args := []string{"--allowedTools", interactive.PublicTool, "--plugin-dir", root, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--replay-user-messages", "--settings", settings}
	if r.Name != "" {
		args = append(args, "--name", nativeName(r.Name))
	}
	if r.ResumeSessionID != "" {
		args = append(args, "--resume", r.ResumeSessionID)
	}
	if r.Open.PermissionMode != "" {
		args = append(args, "--permission-mode", r.Open.PermissionMode)
	}
	if r.Open.Model != "" {
		args = append(args, "--model", r.Open.Model)
	}
	if r.Open.ReasoningEffort != "" {
		args = append(args, "--effort", r.Open.ReasoningEffort)
	}
	return append(args, r.Open.Arguments...)
}
func nativeName(name string) string {
	if at := strings.LastIndexByte(name, '@'); at >= 0 {
		return name[:at]
	}
	return name
}
func confirmTitle(requested, reported string) error {
	if interactive.MissingNativeField(reported) || reported != nativeName(requested) {
		return errors.New("integration open unavailable: native title did not confirm requested name")
	}
	return nil
}
func requiredTools(raw json.RawMessage) error {
	var status struct {
		Servers []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Tools  []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return err
	}
	for _, server := range status.Servers {
		if server.Name != "plugin:sessionbus:sessionbus" {
			continue
		}
		if server.Status != "connected" {
			return fmt.Errorf("integration open unavailable: required MCP status is %s", server.Status)
		}
		for _, tool := range server.Tools {
			if tool.Name == "sessionbus" || tool.Name == interactive.PublicTool {
				return nil
			}
		}
		return errors.New("integration open unavailable: required Sessionbus tool not reported")
	}
	return errors.New("integration open unavailable: required Sessionbus MCP not reported")
}
func (p *Wrapper) current() (*stream, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.opened || p.closing || p.failure != nil || p.stream == nil {
		return nil, "", errors.New("native lane is unavailable")
	}
	return p.stream, p.identity, nil
}
func (p *Wrapper) Run(ctx context.Context, r *kit.Run, input string) (kit.TurnResult, error) {
	p.mu.Lock()
	p.activeDone = r.Done()
	p.mu.Unlock()
	s, id, err := p.current()
	if err != nil {
		return kit.TurnResult{}, err
	}
	return s.run(ctx, id, input, r.Admitted)
}
func (p *Wrapper) Interrupt(ctx context.Context, _ *kit.Run) error {
	s, _, err := p.current()
	if err != nil {
		return err
	}
	_, err = s.control(ctx, map[string]string{"subtype": "interrupt"})
	if err != nil && ctx.Err() == nil {
		p.fail(err)
	}
	return err
}
func (p *Wrapper) Deliver(ctx context.Context, r kit.DeliveryRequest, _ *kit.Run) (kit.DeliveryReceipt, error) {
	s, id, err := p.current()
	if err != nil {
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "native_unavailable"}, nil
	}
	body, err := json.Marshal(map[string]any{"from": r.From, "message": r.Body})
	if err != nil {
		return kit.DeliveryReceipt{}, err
	}
	return s.append(ctx, id, string(body))
}
func (p *Wrapper) fail(err error) {
	p.mu.Lock()
	first := p.failure == nil
	if first {
		p.failure = err
	}
	cancel, shutdown, closing, opened, done := p.cancel, p.shutdown, p.closing, p.opened, p.activeDone
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if first && shutdown != nil && !closing && opened {
		// Run.Done is the public kit's serialized-result boundary. Native EOF can
		// settle the run but must not overtake its response on the bus.
		go func() {
			if done != nil {
				<-done
			}
			shutdown()
		}()
	}
}
func (p *Wrapper) Close(context.Context, kit.SessionCloseRequest) error {
	p.mu.Lock()
	p.closing = true
	cancel, s, endpoint, done := p.cancel, p.stream, p.endpoint, p.processDone
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if s != nil {
		s.stop(errors.New("native lane closed"))
	}
	if endpoint != nil {
		endpoint.close()
	}
	if done != nil {
		<-done
	}
	return nil
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// The stream reader owns EOF; process exit must not discard buffered output.
func waitNative(child *exec.Cmd, done chan struct{}) {
	_ = child.Wait()
	close(done)
}
