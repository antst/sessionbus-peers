// SPDX-License-Identifier: MIT
package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/antst/sessionbus-peers/internal/testsocket"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

func TestNativeArgumentsPreserveCallerSuffix(t *testing.T) {
	var request kit.OpenRequest
	if err := json.Unmarshal([]byte(`{"name":"parent/child@local","resume_session_id":"native-id","open":{"model":"native-model","permission_mode":"default","reasoning_effort":"high","arguments":["--model","last-model","--","literal"]}}`), &request); err != nil {
		t.Fatal(err)
	}
	actual := launchArguments(request, "/installed plugin", "/owned settings")
	expected := []string{"--allowedTools", interactive.PublicTool, "--plugin-dir", "/installed plugin", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--replay-user-messages", "--settings", "/owned settings", "--name", "parent/child", "--resume", "native-id", "--permission-mode", "default", "--model", "native-model", "--effort", "high", "--model", "last-model", "--", "literal"}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("argv %#v", actual)
	}
}
func TestRequiredToolsNeedsConnectedPresence(t *testing.T) {
	for _, tc := range []struct {
		value string
		ready bool
	}{
		{`{"mcpServers":[{"name":"plugin:sessionbus:sessionbus","status":"connected","tools":[{"name":"sessionbus"}]}]}`, true},
		{`{"mcpServers":[{"name":"plugin:sessionbus:sessionbus","status":"pending","tools":[{"name":"sessionbus"}]}]}`, false},
		{`{"mcpServers":[{"name":"plugin:sessionbus:sessionbus","status":"connected"}]}`, false},
		{`{"mcpServers":[{"name":"other","status":"connected","tools":[{"name":"sessionbus"}]}]}`, false},
	} {
		if got := requiredTools(json.RawMessage(tc.value)); (got == nil) != tc.ready {
			t.Fatalf("%s: %v", tc.value, got)
		}
	}
}
func TestFailedOpenRemovesItsEndpoint(t *testing.T) {
	// Native lookup fails before spawn; Open itself must release its allocated listener.
	t.Setenv("PATH", t.TempDir())
	p := New(t.TempDir())
	_, err := p.Open(context.Background(), kit.OpenRequest{})
	if err == nil {
		t.Fatal("missing native executable accepted")
	}
	if p.endpoint == nil {
		t.Fatal("test did not allocate endpoint")
	}
	if _, err := os.Stat(p.endpoint.dir); !os.IsNotExist(err) {
		t.Fatalf("endpoint retained: %v", err)
	}
	if !p.closing {
		t.Fatal("unsuccessful Open did not close")
	}
}

func TestWorkerHelloUsesActualProtocolSchema(t *testing.T) {
	h, err := New("unused").Hello(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(protocol.WorkerHello{Protocol: 1, LaunchToken: "offline-test-token", HelloDescription: h})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.DecodeParams("session.hello", raw); err != nil {
		t.Fatal(err)
	}
	if !h.SupportsMessageRun {
		t.Fatal("Claude omitted waking-run capability")
	}
	description, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var result kit.LaneDescribeResult
	if err := protocol.UnmarshalResult("lane.describe", description, &result); err != nil {
		t.Fatal(err)
	}
}

func TestOpenTitleRequiresNativeConfirmation(t *testing.T) {
	for _, title := range []string{"", "${session_title}", "other"} {
		if confirmTitle("parent/child@local", title) == nil {
			t.Fatalf("unconfirmed %q admitted", title)
		}
	}
	if err := confirmTitle("parent/child@local", "parent/child"); err != nil {
		t.Fatal(err)
	}
}

// Only the native boundary is controlled here. Public hello/open/run and result
// serialization use the installed kit Worker and protocol over an actual socket.
type workerStreamProduct struct{ *Wrapper }

func (p *workerStreamProduct) Open(context.Context, kit.OpenRequest) (kit.OpenResult, error) {
	p.mu.Lock()
	p.opened = true
	p.identity = "native-id"
	p.mu.Unlock()
	return kit.OpenResult{SessionID: "native-id"}, nil
}
func TestWorkerPublishesTerminalReadyBeforeNativeEOFShutdown(t *testing.T) {
	path := filepath.Join(testsocket.Directory(t), "bus.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("SESSIONBUS_SOCKET", path)
	t.Setenv("SESSIONBUS_LAUNCH_TOKEN", "controlled-token")
	t.Setenv("SESSIONBUS_LOCAL_KEY", "")
	p := New(t.TempDir())
	p.ctx, p.cancel = context.WithCancel(context.Background())
	nativeInput, workerInput := io.Pipe()
	workerOutput, nativeOutput := io.Pipe()
	p.stream = newStream(workerInput, workerOutput, p.fail)
	worker := kit.NewWorker(&workerStreamProduct{p})
	p.SetCaller(worker.Caller())
	p.SetShutdown(worker.Shutdown)
	done := make(chan error, 1)
	go func() { done <- worker.Serve(context.Background()) }()
	c, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	reader := bufio.NewReader(c)
	receive := func() protocol.Frame {
		t.Helper()
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		f, err := protocol.DecodeFrame(line[:len(line)-1])
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	send := func(body []byte, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	hello := receive()
	if _, err = protocol.DecodeParams("session.hello", hello.Params); err != nil {
		t.Fatal(err)
	}
	send(protocol.ResultBytes(hello.ID, "session.hello", struct{}{}))
	send(protocol.RequestBytes(1, "session.open", kit.OpenRequest{Name: "parent/child@local", Groups: []string{}}))
	opened := receive()
	var openResult kit.OpenResult
	if err = protocol.UnmarshalResult("session.open", opened.Result, &openResult); err != nil {
		t.Fatal(err)
	}
	send(protocol.RequestBytes(2, "turn.execute", protocol.ExecuteRequest{SessionID: "native-id@local", RunID: "g/1", Input: "prompt"}))
	admission := receive()
	if admission.ID != 2 || string(admission.Result) != `{"session_id":"native-id@local","run_id":"g/1"}` {
		t.Fatal(admission)
	}
	var input map[string]json.RawMessage
	if err = json.NewDecoder(nativeInput).Decode(&input); err != nil {
		t.Fatal(err)
	}
	id := rawString(t, input["uuid"])
	encoder := json.NewEncoder(nativeOutput)
	if err = encoder.Encode(map[string]any{"type": "user", "session_id": "native-id", "uuid": id, "isReplay": true}); err != nil {
		t.Fatal(err)
	}
	if err = encoder.Encode(map[string]any{"type": "result", "session_id": "native-id", "subtype": "success", "result": "retained terminal", "user_message_uuid": id}); err != nil {
		t.Fatal(err)
	}
	_ = nativeOutput.Close()
	ready := receive()
	var terminal protocol.TurnReady
	if ready.Method != "turn.ready" || json.Unmarshal(ready.Params, &terminal) != nil || terminal.RunID != "g/1" || terminal.State != "done" || terminal.Outcome != "completed" {
		t.Fatalf("terminal metadata did not precede shutdown: %+v", ready)
	}
	// Native EOF still ends this worker; detached output does not survive its
	// retirement. The shared ready event, not a terminal-body RPC, precedes it.
	send(protocol.ResultBytes(ready.ID, "turn.ready", struct{}{}))
	if _, err = reader.ReadByte(); err != io.EOF {
		t.Fatalf("worker did not retire after terminal: %v", err)
	}
	<-done
	_ = nativeInput.Close()
}

func TestNativeLifetimeStartupCancellationAndCommit(t *testing.T) {
	for _, commitFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel-pending", true: "cancel-after-commit"}[commitFirst], func(t *testing.T) {
			type key struct{}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "kept"))
			defer cancel()
			p := New("unused")
			stop, err := p.startLifetime(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			defer p.cancel()
			// A compiled fixture process, never native Claude on the offline host.
			child := exec.CommandContext(p.ctx, os.Args[0], "-test.run=^TestNativeLifetimeFixture$")
			child.Env = append(os.Environ(), "CLAUDE_GO_LIFETIME_FIXTURE=1")
			stdin, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = child.Start(); err != nil {
				t.Fatal(err)
			}
			reader := bufio.NewReader(stdout)
			if line, err := reader.ReadString('\n'); err != nil || line != "ready\n" {
				t.Fatal(line, err)
			}
			if p.ctx.Value(key{}) != "kept" {
				t.Fatal("Open values lost")
			}
			p.identity, p.title = "native-id", "parent/child"
			request := kit.OpenRequest{Name: "parent/child@local"}
			if !commitFirst {
				cancel()
				<-p.ctx.Done()
			}
			_, err = p.commitOpen(ctx, request, stop)
			if commitFirst {
				if err != nil {
					t.Fatal(err)
				}
				cancel()
				if p.ctx.Err() != nil {
					t.Fatal("completed Open cancellation killed native lifetime")
				}
				if _, err := io.WriteString(stdin, "still-alive\n"); err != nil {
					t.Fatal(err)
				}
				if line, err := reader.ReadString('\n'); err != nil || line != "still-alive\n" {
					t.Fatal(line, err)
				}
				_ = stdin.Close()
				if err := child.Wait(); err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, context.Canceled) || p.opened {
				t.Fatal("cancelled Open committed", err)
			} else {
				if err := child.Wait(); err == nil {
					t.Fatal("startup cancellation did not abort child")
				}
				_ = stdin.Close()
			}
		})
	}
}
func TestNativeLifetimeFixture(t *testing.T) {
	if os.Getenv("CLAUDE_GO_LIFETIME_FIXTURE") != "1" {
		return
	}
	_, _ = io.WriteString(os.Stdout, "ready\n")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		_, _ = io.WriteString(os.Stdout, scanner.Text()+"\n")
	}
	os.Exit(0)
}

type lifetimeWorkerProduct struct{ *Wrapper }

func (p *lifetimeWorkerProduct) Open(ctx context.Context, r kit.OpenRequest) (kit.OpenResult, error) {
	stop, err := p.startLifetime(ctx)
	if err != nil {
		return kit.OpenResult{}, err
	}
	defer stop()
	p.mu.Lock()
	p.identity = "native-id"
	p.title = nativeName(r.Name)
	p.mu.Unlock()
	return p.commitOpen(ctx, r, stop)
}
func TestRealWorkerNormalCloseKeepsNativeLifetimeUntilExit(t *testing.T) {
	for _, scenario := range []string{"stdout-first", "process-first", "close-context-loss"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(testsocket.Directory(t), "bus.sock")
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			t.Setenv("SESSIONBUS_SOCKET", path)
			t.Setenv("SESSIONBUS_LAUNCH_TOKEN", "controlled-close-token")
			t.Setenv("SESSIONBUS_LOCAL_KEY", "")
			p := New("unused")
			nativeInput, workerInput := io.Pipe()
			workerOutput, nativeOutput := io.Pipe()
			defer nativeInput.Close()
			defer nativeOutput.Close()
			p.stream = newStream(workerInput, workerOutput, func(err error) { p.end(err, errors.Is(err, io.EOF)) })
			p.processDone = make(chan struct{})
			worker := kit.NewWorker(&lifetimeWorkerProduct{p})
			p.SetCaller(worker.Caller())
			p.SetShutdown(worker.Shutdown)
			served := make(chan error, 1)
			go func() { served <- worker.Serve(context.Background()) }()
			c, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			reader := bufio.NewReader(c)
			receive := func() protocol.Frame {
				t.Helper()
				line, e := reader.ReadBytes('\n')
				if e != nil {
					t.Fatal(e)
				}
				f, e := protocol.DecodeFrame(line[:len(line)-1])
				if e != nil {
					t.Fatal(e)
				}
				return f
			}
			send := func(body []byte, e error) {
				t.Helper()
				if e != nil {
					t.Fatal(e)
				}
				if _, e = c.Write(body); e != nil {
					t.Fatal(e)
				}
			}
			hello := receive()
			send(protocol.ResultBytes(hello.ID, "session.hello", struct{}{}))
			send(protocol.RequestBytes(1, "session.open", kit.OpenRequest{Name: "parent/child@local", Groups: []string{}}))
			if f := receive(); f.ID != 1 || len(f.Result) == 0 {
				t.Fatal("Open failed", f)
			}
			send(protocol.RequestBytes(2, "session.close", kit.SessionCloseRequest{SessionID: "native-id"}))
			var one [1]byte
			if _, e := nativeInput.Read(one[:]); e != io.EOF {
				t.Fatal("normal close did not end stdin", e)
			}
			// The actual Worker has cancelled its Open-operation context before this
			// EOF. Its native lifetime must still be live and reports must still work.
			if p.ctx.Err() != nil {
				t.Fatal("Worker cancellation killed native before Close")
			}
			backendCtx, backendCancel := context.WithCancel(p.ctx)
			defer backendCancel()
			backend := &laneBackend{owner: p, ctx: backendCtx, cancel: backendCancel}
			if _, e := backend.BeginReport(json.RawMessage(`{"hook_event_name":"SessionEnd","session_id":"native-id","agent_id":"${agent_id}"}`)); e != nil {
				t.Fatal(e)
			}
			for _, raw := range []string{`{"hook_event_name":"SessionEnd","session_id":"foreign"}`, `{"hook_event_name":"SessionEnd","session_id":null}`, `{"hook_event_name":"UserPromptSubmit","session_id":"native-id","session_title":"reopen"}`} {
				if _, e := backend.BeginReport(json.RawMessage(raw)); e == nil {
					t.Fatal("closing accepted invalid/reopening report", raw)
				}
			}
			backend.End()
			if p.ctx.Err() != nil || p.title != "parent/child" {
				t.Fatal("expected report shutdown killed/reopened native")
			}
			if scenario == "close-context-loss" {
				c.Close()
				<-p.ctx.Done()
				close(p.processDone)
				<-served
				return
			}
			if scenario == "process-first" {
				close(p.processDone)
				if p.ctx.Err() != nil {
					t.Fatal("process exit discarded unread stdout")
				}
				nativeOutput.Close()
				<-p.stream.done
			} else {
				nativeOutput.Close()
				<-p.stream.done
				if p.ctx.Err() != nil {
					t.Fatal("stdout EOF killed native before exit")
				}
				select {
				case <-served:
					t.Fatal("stdout EOF completed close without child exit")
				default:
				}
				close(p.processDone)
			}
			f := receive()
			if f.ID != 2 || string(f.Result) != "{}" {
				t.Fatal("missing close result", f)
			}
			if _, e := reader.ReadByte(); e != io.EOF {
				t.Fatal("worker did not close", e)
			}
			<-served
		})
	}
}
