// SPDX-License-Identifier: MIT

package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

type mcpHarness struct {
	input  *io.PipeWriter
	frames chan map[string]json.RawMessage
	done   chan struct{}
}

func newMCP(t *testing.T, o MCPOwner) *mcpHarness {
	t.Helper()
	in, w := io.Pipe()
	out, r := io.Pipe()
	h := &mcpHarness{input: w, frames: make(chan map[string]json.RawMessage, 32), done: make(chan struct{})}
	go func() { defer close(h.done); _ = Serve(o, in, r); _ = r.Close() }()
	go func() {
		defer close(h.frames)
		d := json.NewDecoder(out)
		for {
			var f map[string]json.RawMessage
			if d.Decode(&f) != nil {
				return
			}
			h.frames <- f
		}
	}()
	t.Cleanup(func() { _ = w.Close(); <-h.done; _ = out.Close() })
	return h
}
func (h *mcpHarness) send(t *testing.T, frame any) {
	t.Helper()
	b, _ := json.Marshal(frame)
	if _, err := h.input.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}
func (h *mcpHarness) call(t *testing.T, id int, action string, args any) {
	t.Helper()
	h.send(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": "sessionbus", "arguments": map[string]any{"action": action, "arguments": args}}})
}
func (h *mcpHarness) next(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	f, ok := <-h.frames
	if !ok {
		t.Fatal("MCP closed")
	}
	return f
}
func (h *mcpHarness) cancel(t *testing.T, id int) {
	h.send(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": id}})
}
func value(t *testing.T, f map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	var result struct {
		Content []struct{ Text string }
		IsError bool
	}
	if err := json.Unmarshal(f["result"], &result); err != nil || len(result.Content) != 1 {
		t.Fatalf("bad result %s", f["result"])
	}
	return json.RawMessage(result.Content[0].Text)
}

type observedOwner struct {
	*Owner
	waitEntered  chan struct{}
	waitReturned chan error
	listReturned chan error
	collected    chan struct{}
	release      chan struct{}
	once         sync.Once
}

func (o *observedOwner) Action(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	if action == "wait" && o.waitEntered != nil {
		o.once.Do(func() { close(o.waitEntered) })
	}
	raw, err := o.Owner.Action(ctx, action, args)
	if action == "list" && o.listReturned != nil {
		o.listReturned <- err
	}
	if action == "wait" && o.waitReturned != nil {
		o.waitReturned <- err
	}
	if action == "wait" && err == nil && o.collected != nil {
		close(o.collected)
		<-o.release
	}
	return raw, err
}

func startMCPRun(t *testing.T, h *mcpHarness, w *wire) kit.RunRef {
	t.Helper()
	h.call(t, 1, "start", map[string]string{"session_id": "lane", "input": "one"})
	request := w.next(t)
	if request.Method != "turn.start" {
		t.Fatal(request.Method)
	}
	ref := kit.RunRef{SessionID: "lane", RunID: "g/1"}
	w.reply(t, request, ref)
	var returned kit.RunRef
	if json.Unmarshal(value(t, h.next(t)), &returned) != nil || returned != ref {
		t.Fatal(returned)
	}
	return ref
}

func TestCancelledMCPWaitPreservesActualCallerResult(t *testing.T) {
	for _, collector := range []string{"status", "wait"} {
		t.Run(collector, func(t *testing.T) {
			o, wires := testOwner(t)
			w := published(t, o, wires)
			observed := &observedOwner{Owner: o, waitReturned: make(chan error, 2)}
			h := newMCP(t, observed)
			ref := startMCPRun(t, h, w)
			h.call(t, 2, "wait", ref)
			abandoned := w.next(t)
			if abandoned.Method != "turn.wait" {
				t.Fatal(abandoned.Method)
			}
			h.cancel(t, 2)
			if err := <-observed.waitReturned; !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			terminal := kit.RunStatus{SessionID: ref.SessionID, RunID: ref.RunID, State: "done", Result: &kit.TurnResult{Outcome: "completed", Result: "retained"}}
			w.reply(t, abandoned, terminal)
			// Collection now routes to the worker; no local handle is consumed.
			for _, callID := range []int{3, 4} {
				h.call(t, callID, collector, ref)
				read := w.next(t)
				if read.Method != "turn."+collector {
					t.Fatal(read.Method)
				}
				w.reply(t, read, terminal)
				f := h.next(t)
				var status kit.RunStatus
				if string(f["id"]) != fmt.Sprint(callID) || json.Unmarshal(value(t, f), &status) != nil || status.Result == nil || status.Result.Result != "retained" {
					t.Fatal(f)
				}
			}
			h.call(t, 5, "ack", ref)
			ack := w.next(t)
			if ack.Method != "turn.ack" {
				t.Fatal(ack.Method)
			}
			w.reply(t, ack, struct{}{})
			if string(h.next(t)["id"]) != "5" {
				t.Fatal("missing explicit ack result")
			}
		})
	}
}

func TestFulfilledWaitKeepsResponseAfterLateCancellation(t *testing.T) {
	o, wires := testOwner(t)
	w := published(t, o, wires)
	observed := &observedOwner{Owner: o, collected: make(chan struct{}), release: make(chan struct{})}
	h := newMCP(t, observed)
	ref := startMCPRun(t, h, w)
	h.call(t, 2, "wait", ref)
	read := w.next(t)
	w.reply(t, read, kit.RunStatus{SessionID: ref.SessionID, RunID: ref.RunID, State: "done", Result: &kit.TurnResult{Outcome: "completed", Result: "won"}})
	<-observed.collected
	h.cancel(t, 2)
	h.send(t, map[string]any{"jsonrpc": "2.0", "id": 9, "method": "ping"})
	if string(h.next(t)["id"]) != "9" {
		t.Fatal("cancellation barrier")
	}
	close(observed.release)
	f := h.next(t)
	var result kit.RunStatus
	_ = json.Unmarshal(value(t, f), &result)
	if string(f["id"]) != "2" || result.Result == nil || result.Result.Result != "won" {
		t.Fatal("fulfilled result dropped")
	}
}

func TestMCPProtocolHiddenReportAndEOF(t *testing.T) {
	o, wires := testOwner(t)
	h := newMCP(t, o)
	h.send(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	f := h.next(t)
	var list struct{ Tools []struct{ Name string } }
	_ = json.Unmarshal(f["result"], &list)
	if len(list.Tools) != 1 || list.Tools[0].Name != "sessionbus" {
		t.Fatal(list)
	}
	h.send(t, map[string]any{"jsonrpc": "2.0", "method": "native_identity_event", "params": map[string]string{"session_id": "fake"}})
	h.send(t, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": HiddenTool, "arguments": map[string]string{"hook_event_name": "Stop", "session_id": "native"}}})
	w := <-wires
	hello := w.next(t)
	w.reply(t, hello, map[string]any{})
	_ = h.next(t)
	h.send(t, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "unknown"})
	if string(h.next(t)["error"]) == "" {
		t.Fatal("missing protocol error")
	}
	_ = h.input.Close()
	<-h.done
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.ended || o.connection != nil {
		t.Fatal("EOF left integration alive")
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestOutputFailureEndsOwnerAndUnblocksInput(t *testing.T) {
	o, _ := testOwner(t)
	r, w := io.Pipe()
	defer w.Close()
	done := make(chan struct{})
	go func() { _ = Serve(o, r, failedOutput{}); close(done) }()
	_, _ = io.WriteString(w, "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n")
	<-done
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.ended {
		t.Fatal("output failure left owner active")
	}
}

func TestMCPRejectsNullVersionAndMalformedFrames(t *testing.T) {
	o, _ := testOwner(t)
	h := newMCP(t, o)
	for _, raw := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":null}}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":4}}`,
		`{"jsonrpc":"2.0","id":1,"method":"ping","params":[]}`,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
		`null`, `{`,
	} {
		if _, err := h.input.Write([]byte(raw + "\n")); err != nil {
			t.Fatal(err)
		}
		if f := h.next(t); len(f["error"]) == 0 {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{
		`{"hook_event_name":"Stop","session_id":"id","agent_id":null}`,
		`{"hook_event_name":"Stop","session_id":"id","session_title":null}`,
		`{"hook_event_name":"Stop","session_id":"id","agent_id":"agent"}`,
		`{"hook_event_name":"SessionStart","session_id":"id"}`,
	} {
		if _, err := o.BeginReport(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted report %s", raw)
		}
	}
}

func TestPublishedMCPWithPendingCallerSettlesOnEOF(t *testing.T) {
	o, wires := testOwner(t)
	observed := &observedOwner{Owner: o, listReturned: make(chan error, 1)}
	h := newMCP(t, observed)
	h.send(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": HiddenTool, "arguments": map[string]string{"hook_event_name": "UserPromptSubmit", "session_id": "native-id", "session_title": "name"}}})
	w := <-wires
	hello := w.next(t)
	if hello.Method != "session.hello" {
		t.Fatal(hello.Method)
	}
	w.reply(t, hello, map[string]any{})
	if string(h.next(t)["id"]) != "1" {
		t.Fatal("publication response")
	}
	h.call(t, 2, "list", map[string]any{})
	request := w.next(t)
	if request.Method != "session.list" {
		t.Fatal(request.Method)
	}
	// This real Caller request remains unanswered; EOF alone must settle it.
	if err := h.input.Close(); err != nil {
		t.Fatal(err)
	}
	<-h.done
	if err := <-observed.listReturned; err == nil {
		t.Fatal("pending request falsely succeeded")
	}
	for f := range h.frames {
		t.Fatalf("response after EOF: %s", f["id"])
	}
	for f := range w.frames {
		t.Fatalf("new wire frame after EOF: %s", f.Method)
	}
	late, err := protocol.ResultBytes(request.ID, request.Method, map[string]any{"sessions": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.fd.Write(late); err == nil {
		t.Fatal("EOF left public Connection open")
	}
	if _, err = o.BeginReport(json.RawMessage(`{"hook_event_name":"Stop","session_id":"later"}`)); err == nil {
		t.Fatal("report revived ended owner")
	}
	select {
	case <-wires:
		t.Fatal("reconnected after EOF")
	default:
	}
}

func TestSubmittedMCPAckKeepsResponseAfterCancellation(t *testing.T) {
	o, wires := testOwner(t)
	w := published(t, o, wires)
	h := newMCP(t, o)
	h.call(t, 1, "ack", kit.RunRef{SessionID: "lane", RunID: "g/1"})
	ack := w.next(t)
	if ack.Method != "turn.ack" {
		t.Fatal(ack.Method)
	}
	h.cancel(t, 1)
	h.send(t, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "ping"})
	if string(h.next(t)["id"]) != "2" {
		t.Fatal("ack returned before its wire result")
	}
	w.reply(t, ack, struct{}{})
	result := h.next(t)
	if string(result["id"]) != "1" || string(value(t, result)) != "{}" {
		t.Fatal("submitted acknowledgement lost its successful response")
	}
}
