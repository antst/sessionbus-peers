// SPDX-License-Identifier: MIT

package interactive

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
func (w *wire) fail(t *testing.T, f protocol.Frame, code int) {
	t.Helper()
	b, err := protocol.ErrorBytes(f.ID, code, nil)
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
func gateRetries(o *Owner) (<-chan struct{}, chan<- struct{}) {
	waiting := make(chan struct{})
	release := make(chan struct{})
	o.retry = func(ctx context.Context) bool {
		select {
		case waiting <- struct{}{}:
		case <-ctx.Done():
			return false
		}
		select {
		case <-release:
			return true
		case <-ctx.Done():
			return false
		}
	}
	return waiting, release
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
func TestUnexpectedLossReconnectsLatestExactIdentity(t *testing.T) {
	o, wires := testOwner(t)
	waiting, retry := gateRetries(o)
	w := published(t, o, wires)
	o.mu.Lock()
	first := o.connection
	o.mu.Unlock()
	_ = w.fd.Close()
	<-first.Done()
	<-waiting
	_, err := o.Action(context.Background(), "list", json.RawMessage(`{}`))
	var failure *kit.ProtocolError
	if !errors.As(err, &failure) || failure.Code != protocol.NotConnected {
		t.Fatalf("outage action = %v", err)
	}
	done := report(t, o, "Stop", "native-id", "renamed while down")
	retry <- struct{}{}
	next := <-wires
	hello := next.next(t)
	var identity kit.Identity
	if err := json.Unmarshal(hello.Params, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.SessionID != "native-id" || identity.Name != "renamed while down" || len(identity.Groups) != 1 || identity.Groups[0] != "group" {
		t.Fatal(identity)
	}
	next.reply(t, hello, map[string]any{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestStartupDaemonAbsentRecoversWithoutAnotherNativeReport(t *testing.T) {
	o, err := NewOwner(map[string]string{"SESSIONBUS_GROUPS": `["group"]`})
	if err != nil {
		t.Fatal(err)
	}
	wires := make(chan *wire, 1)
	first := true
	o.dial = func(context.Context, string, string) (net.Conn, error) {
		if first {
			first = false
			return nil, errors.New("daemon absent")
		}
		a, b := net.Pipe()
		wires <- newWire(t, b)
		return a, nil
	}
	waiting, retry := gateRetries(o)
	t.Cleanup(o.End)
	done := report(t, o, "Stop", "native-id", "name")
	<-waiting
	retry <- struct{}{}
	w := <-wires
	w.reply(t, w.next(t), map[string]any{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestLostInFlightCallIsNotReplayedAfterReconnect(t *testing.T) {
	o, wires := testOwner(t)
	o.deliver = func(Recipient, kit.DeliveryRequest) (kit.DeliveryReceipt, error) {
		return kit.DeliveryReceipt{Disposition: "written"}, nil
	}
	waiting, retry := gateRetries(o)
	w := published(t, o, wires)
	returned := make(chan error, 1)
	go func() { _, err := o.Action(context.Background(), "list", json.RawMessage(`{}`)); returned <- err }()
	request := w.next(t)
	if request.Method != "session.list" {
		t.Fatal(request.Method)
	}
	_ = w.fd.Close()
	if err := <-returned; err == nil {
		t.Fatal("lost admitted call succeeded")
	}
	<-waiting
	retry <- struct{}{}
	next := <-wires
	next.reply(t, next.next(t), map[string]any{})
	next.request(t, "message.deliver", kit.DeliveryRequest{MessageID: "barrier", From: kit.DeliverySource{SessionID: "source", Product: "fixture", Groups: []string{}}, Body: "body"})
	barrier := next.next(t)
	if barrier.Request || barrier.ID != 1 {
		t.Fatalf("old call replayed before admission barrier: %#v", barrier)
	}
	var receipt kit.DeliveryReceipt
	if err := protocol.UnmarshalResult("message.deliver", barrier.Result, &receipt); err != nil || receipt.Disposition != "written" {
		t.Fatal(receipt, err)
	}
	go func() { _, err := o.Action(context.Background(), "list", json.RawMessage(`{}`)); returned <- err }()
	fresh := next.next(t)
	if fresh.Method != "session.list" {
		t.Fatal(fresh.Method)
	}
	next.reply(t, fresh, json.RawMessage(`{"sessions":[]}`))
	if err := <-returned; err != nil {
		t.Fatal(err)
	}
}

func TestOldDeliveryCompletionCannotRetireReplacementConnection(t *testing.T) {
	o, wires := testOwner(t)
	waiting, retry := gateRetries(o)
	entered, release := make(chan struct{}), make(chan struct{})
	o.deliver = func(_ Recipient, request kit.DeliveryRequest) (kit.DeliveryReceipt, error) {
		if request.MessageID == "old" {
			close(entered)
			<-release
		}
		return kit.DeliveryReceipt{Disposition: "written"}, nil
	}
	old := published(t, o, wires)
	old.request(t, "message.deliver", kit.DeliveryRequest{MessageID: "old", From: kit.DeliverySource{SessionID: "source", Product: "fixture", Groups: []string{}}, Body: "body"})
	<-entered
	_ = old.fd.Close()
	<-waiting
	retry <- struct{}{}
	next := <-wires
	next.reply(t, next.next(t), map[string]any{})
	next.request(t, "message.deliver", kit.DeliveryRequest{MessageID: "new", From: kit.DeliverySource{SessionID: "source", Product: "fixture", Groups: []string{}}, Body: "body"})
	barrier := next.next(t)
	var receipt kit.DeliveryReceipt
	if err := protocol.UnmarshalResult("message.deliver", barrier.Result, &receipt); err != nil || receipt.Disposition != "written" {
		t.Fatal(receipt, err)
	}
	close(release)
	o.handlers.Wait()
	returned := make(chan error, 1)
	go func() { _, err := o.Action(context.Background(), "list", json.RawMessage(`{}`)); returned <- err }()
	fresh := next.next(t)
	if fresh.Method != "session.list" {
		t.Fatal(fresh.Method)
	}
	next.reply(t, fresh, json.RawMessage(`{"sessions":[]}`))
	if err := <-returned; err != nil {
		t.Fatal(err)
	}
}

func TestSupersessionIsTerminalAndJoinsPublisher(t *testing.T) {
	o, wires := testOwner(t)
	waiting, _ := gateRetries(o)
	w := published(t, o, wires)
	w.request(t, "session.superseded", map[string]any{})
	if ack := w.next(t); ack.Error != nil {
		t.Fatal(ack.Error)
	}
	o.End()
	if _, err := o.BeginReport(json.RawMessage(`{"hook_event_name":"Stop","session_id":"native-id"}`)); err == nil {
		t.Fatal("superseded owner accepted report")
	}
	select {
	case <-waiting:
		t.Fatal("superseded owner entered reconnect backoff")
	default:
	}
}

func TestInvalidHelloIsTerminalAndDoesNotRetry(t *testing.T) {
	o, wires := testOwner(t)
	waiting, _ := gateRetries(o)
	done := report(t, o, "Stop", "native-id", "name")
	w := <-wires
	w.fail(t, w.next(t), protocol.InvalidHello)
	var failure *kit.ProtocolError
	if err := <-done; !errors.As(err, &failure) || failure.Code != protocol.InvalidHello {
		t.Fatalf("hello error = %v", err)
	}
	o.End()
	if _, err := o.BeginReport(json.RawMessage(`{"hook_event_name":"Stop","session_id":"native-id"}`)); err == nil {
		t.Fatal("invalid hello did not end owner")
	}
	select {
	case <-waiting:
		t.Fatal("invalid hello entered reconnect backoff")
	default:
	}
}

func TestPendingNativeReportObserversAreBounded(t *testing.T) {
	o, wires := testOwner(t)
	waits := []<-chan error{report(t, o, "Stop", "native-id", "name")}
	w := <-wires
	hello := w.next(t)
	for len(waits) < protocol.MaxOperations {
		waits = append(waits, report(t, o, "Stop", "native-id", "name"))
	}
	if _, err := o.BeginReport(json.RawMessage(`{"hook_event_name":"Stop","session_id":"native-id","session_title":"name"}`)); err == nil {
		t.Fatal("unbounded report observers accepted")
	}
	w.reply(t, hello, map[string]any{})
	for _, done := range waits {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	o.mu.Lock()
	pending := o.pendingReports
	o.mu.Unlock()
	if pending != 0 {
		t.Fatal("report observers retained", pending)
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
