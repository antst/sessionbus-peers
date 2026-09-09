// SPDX-License-Identifier: MIT

package interactive

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

// The remote side uses the pinned production codec; malformed fixture requests
// cannot silently pass the model-tool tests as the former JS stubs did.
type wire struct {
	fd     net.Conn
	frames chan protocol.Frame
	writes sync.Mutex
	nextID int64
}

func newWire(t *testing.T, fd net.Conn) *wire {
	t.Helper()
	w := &wire{fd: fd, frames: make(chan protocol.Frame, 32)}
	t.Cleanup(func() { _ = fd.Close() })
	go func() {
		defer close(w.frames)
		r := bufio.NewReader(fd)
		for {
			b, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			f, err := protocol.DecodeFrame(b[:len(b)-1])
			if err != nil {
				t.Error(err)
				return
			}
			if f.Request {
				if _, err := protocol.DecodeParams(f.Method, f.Params); err != nil {
					t.Error(err)
					return
				}
			}
			w.frames <- f
		}
	}()
	return w
}
func (w *wire) next(t *testing.T) protocol.Frame {
	t.Helper()
	f, ok := <-w.frames
	if !ok {
		t.Fatal("wire closed")
	}
	return f
}
func (w *wire) reply(t *testing.T, f protocol.Frame, value any) {
	t.Helper()
	b, err := protocol.ResultBytes(f.ID, f.Method, value)
	if err != nil {
		t.Fatal(err)
	}
	w.write(t, b)
}
func (w *wire) write(t *testing.T, b []byte) {
	t.Helper()
	w.writes.Lock()
	defer w.writes.Unlock()
	if _, err := w.fd.Write(b); err != nil {
		t.Error(err)
	}
}
func (w *wire) request(t *testing.T, method string, params any) {
	t.Helper()
	w.nextID++
	raw, err := protocol.EncodeParams(method, params)
	if err != nil {
		t.Fatal(err)
	}
	b, err := protocol.EncodeRequest(w.nextID, method, raw)
	if err != nil {
		t.Fatal(err)
	}
	w.write(t, b)
}
func testOwner(t *testing.T) (*Owner, <-chan *wire) {
	t.Helper()
	o, err := NewOwner(map[string]string{"SESSIONBUS_GROUPS": `["group"]`})
	if err != nil {
		t.Fatal(err)
	}
	wires := make(chan *wire, 8)
	o.dial = func(context.Context, string, string) (net.Conn, error) {
		a, b := net.Pipe()
		wires <- newWire(t, b)
		return a, nil
	}
	t.Cleanup(o.End)
	return o, wires
}
func report(t *testing.T, o *Owner, event, id, title string) <-chan error {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"hook_event_name": event, "session_id": id, "session_title": title})
	done, err := o.BeginReport(raw)
	if err != nil {
		t.Fatal(err)
	}
	return done
}
func published(t *testing.T, o *Owner, wires <-chan *wire) *wire {
	t.Helper()
	done := report(t, o, "UserPromptSubmit", "native-id", "name")
	w := <-wires
	f := w.next(t)
	w.reply(t, f, map[string]any{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	return w
}
func TestNativeReportPublicationRenameAndWithdrawal(t *testing.T) {
	o, wires := testOwner(t)
	if _, err := o.Action(context.Background(), "list", json.RawMessage(`{}`)); err == nil {
		t.Fatal("public call bootstrapped")
	}
	done := report(t, o, "Stop", "native-id", "")
	w := <-wires
	hello := w.next(t)
	var id kit.Identity
	if err := json.Unmarshal(hello.Params, &id); err != nil {
		t.Fatal(err)
	}
	if id.Name != "" || id.SessionID != "native-id" || len(id.Groups) != 1 {
		t.Fatalf("identity %#v", id)
	}
	w.reply(t, hello, map[string]any{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	done = report(t, o, "UserPromptSubmit", "native-id", "renamed")
	hello = w.next(t)
	w.reply(t, hello, map[string]any{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if done := report(t, o, "Stop", "native-id", ""); done != nil {
		t.Fatal("empty title erased name")
	}
	report(t, o, "SessionEnd", "foreign", "")
	o.mu.Lock()
	name := o.admitted.Name
	o.mu.Unlock()
	if name != "renamed" {
		t.Fatal(name)
	}
	report(t, o, "SessionEnd", "native-id", "")
	if _, err := o.Action(context.Background(), "list", json.RawMessage(`{}`)); err == nil {
		t.Fatal("withdrawal not applied")
	}
	done = report(t, o, "UserPromptSubmit", "after-clear", "clear")
	next := <-wires
	next.reply(t, next.next(t), map[string]any{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestHelloObserverCapturesIdentityBeforeImmediateDelivery(t *testing.T) {
	o, wires := testOwner(t)
	captured := make(chan Recipient, 1)
	o.deliver = func(r Recipient, _ kit.DeliveryRequest) (kit.DeliveryReceipt, error) {
		captured <- r
		return kit.DeliveryReceipt{Disposition: "written"}, nil
	}
	done := report(t, o, "Stop", "native-id", "")
	w := <-wires
	hello := w.next(t)
	w.reply(t, hello, map[string]any{})
	w.request(t, "message.deliver", kit.DeliveryRequest{MessageID: "message", From: kit.DeliverySource{SessionID: "sender", Product: "claude-peer", Groups: []string{}}, Body: "body"})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if r := <-captured; r.SessionID != "native-id" {
		t.Fatal(r)
	}
	f := w.next(t)
	var receipt kit.DeliveryReceipt
	if err := protocol.UnmarshalResult("message.deliver", f.Result, &receipt); err != nil || receipt.Disposition != "written" {
		t.Fatalf("%s %v", f.Result, err)
	}
}
func TestLateHelloCannotReviveWithdrawnOwner(t *testing.T) {
	o, wires := testOwner(t)
	done := report(t, o, "Stop", "id", "")
	w := <-wires
	_ = w.next(t)
	report(t, o, "SessionEnd", "id", "")
	if err := <-done; err == nil {
		t.Fatal("closed pending hello succeeded")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.admitted != nil || o.ended {
		t.Fatal("withdrawn hello revived or poisoned next clear")
	}
}
func TestSupersessionAndUnexpectedLossAreTerminal(t *testing.T) {
	for _, kind := range []string{"loss", "superseded"} {
		t.Run(kind, func(t *testing.T) {
			o, wires := testOwner(t)
			w := published(t, o, wires)
			o.mu.Lock()
			c := o.connection
			o.mu.Unlock()
			if kind == "superseded" {
				w.request(t, "session.superseded", map[string]any{})
				_ = w.next(t)
			} else {
				_ = w.fd.Close()
			}
			<-c.Done()
			// End is idempotent, but wait for the connection watcher through a real
			// report: a dead connection can never admit another hello.
			done, err := o.BeginReport(json.RawMessage(`{"hook_event_name":"Stop","session_id":"id"}`))
			if err == nil && done != nil {
				err = <-done
			}
			if err == nil {
				t.Fatal("lost owner accepted report")
			}
		})
	}
}

func TestCancelledWireCallDrainsLateReplyAndKeepsIntegration(t *testing.T) {
	o, wires := testOwner(t)
	w := published(t, o, wires)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := o.Action(ctx, "list", json.RawMessage(`{}`)); done <- err }()
	f := w.next(t)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled call succeeded")
	}
	o.mu.Lock()
	c := o.connection
	o.mu.Unlock()
	w.reply(t, f, json.RawMessage(`{"sessions":[]}`))
	// An ordered second request proves the canceled response drained without
	// closing the published connection or writing into the canceled result.
	go func() { _, err := o.Action(context.Background(), "list", json.RawMessage(`{}`)); done <- err }()
	second := w.next(t)
	if second.Method != "session.list" {
		t.Fatal(second.Method)
	}
	w.reply(t, second, json.RawMessage(`{"sessions":[]}`))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Done():
		t.Fatal("valid late response closed integration")
	default:
	}
}
