// SPDX-License-Identifier: MIT

package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type muxPipe struct {
	net.Conn
	dec *json.Decoder
	mu  sync.Mutex
}

func (p *muxPipe) Read(v any) error { return p.dec.Decode(v) }
func (p *muxPipe) Write(v any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return json.NewEncoder(p.Conn).Encode(v)
}
func muxFixture(t *testing.T, observe func(string, json.RawMessage, json.RawMessage) error) (*brokerMux, *muxPipe) {
	t.Helper()
	a, b := net.Pipe()
	m := newBrokerMux(context.Background(), &muxPipe{Conn: a, dec: json.NewDecoder(a)}, observe)
	p := &muxPipe{Conn: b, dec: json.NewDecoder(b)}
	t.Cleanup(func() { m.fail(io.EOF); _ = p.Close(); <-m.done })
	return m, p
}
func frame(method string, id any, p any) brokerFrame {
	f := brokerFrame{"jsonrpc": brokerRaw("2.0")}
	if method != "" {
		f["method"] = brokerRaw(method)
		f["params"] = brokerRaw(p)
	}
	if id != nil {
		f["id"] = brokerRaw(id)
	}
	return f
}
func receiveNative(t *testing.T, p *muxPipe) brokerFrame {
	t.Helper()
	_ = p.SetReadDeadline(time.Now().Add(3 * time.Second))
	var f brokerFrame
	if err := p.Read(&f); err != nil {
		t.Fatal(err)
	}
	return f
}
func receiveTUI(t *testing.T, m *brokerMux) brokerFrame {
	t.Helper()
	select {
	case packet := <-m.tuiOut:
		m.release(len(packet.body))
		var f brokerFrame
		if err := json.Unmarshal(packet.body, &f); err != nil {
			t.Fatal(err)
		}
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("no TUI frame")
	}
	return nil
}
func initializeMux(t *testing.T, m *brokerMux, p *muxPipe) {
	t.Helper()
	if err := m.fromTUI(frame("initialize", "initialize", map[string]any{"clientInfo": map[string]string{"name": "native"}})); err != nil {
		t.Fatal(err)
	}
	f := receiveNative(t, p)
	if err := p.Write(brokerFrame{"id": f["id"], "result": brokerRaw(map[string]bool{"ok": true})}); err != nil {
		t.Fatal(err)
	}
	response := receiveTUI(t, m)
	if string(response["id"]) != `"initialize"` {
		t.Fatal(response)
	}
	if err := m.fromTUI(frame("initialized", nil, map[string]any{})); err != nil {
		t.Fatal(err)
	}
	if f = receiveNative(t, p); string(f["method"]) != `"initialized"` {
		t.Fatal(f)
	}
}
func TestBrokerMuxInitializeAndOpaqueIDs(t *testing.T) {
	m, p := muxFixture(t, nil)
	initializeMux(t, m, p)
	for _, id := range []any{"7", int64(7), int64(-2), "unicode/λ"} {
		f := frame("thread/read", id, map[string]any{"threadId": "native"})
		f["extension"] = brokerRaw(map[string]any{"unchanged": "value"})
		if err := m.fromTUI(f); err != nil {
			t.Fatal(err)
		}
		n := receiveNative(t, p)
		if string(n["extension"]) != `{"unchanged":"value"}` {
			t.Fatal(n)
		}
		if err := p.Write(brokerFrame{"id": n["id"], "result": brokerRaw(map[string]string{"opaque": "result"}), "extra": brokerRaw(17)}); err != nil {
			t.Fatal(err)
		}
		got := receiveTUI(t, m)
		if string(got["id"]) != string(brokerRaw(id)) || string(got["extra"]) != "17" {
			t.Fatal(got)
		}
	}
}
func TestBrokerMuxServerRequestAndResolvedTranslation(t *testing.T) {
	m, p := muxFixture(t, nil)
	initializeMux(t, m, p)
	var lateID json.RawMessage
	// Native resolution dismisses the TUI handler without any response. More than
	// one full live-map capacity must remain usable with no retained tombstones.
	for id := int64(1); id <= brokerRouteLimit+1; id++ {
		if err := p.Write(frame("item/commandExecution/requestApproval", id, map[string]string{"threadId": "t"})); err != nil {
			t.Fatal(err)
		}
		request := receiveTUI(t, m)
		if id == 1 {
			lateID = append(json.RawMessage(nil), request["id"]...)
		}
		if err := p.Write(frame("serverRequest/resolved", nil, map[string]any{"threadId": "t", "requestId": id})); err != nil {
			t.Fatal(err)
		}
		resolved := receiveTUI(t, m)
		var params map[string]json.RawMessage
		if err := json.Unmarshal(resolved["params"], &params); err != nil {
			t.Fatal(err)
		}
		if string(params["requestId"]) != string(request["id"]) {
			t.Fatal(params)
		}
		m.mu.Lock()
		live := len(m.servers)
		m.mu.Unlock()
		if live != 0 {
			t.Fatalf("resolved request retained live capacity: %d", live)
		}
	}
	// A genuinely racing mapped response is harmless after final dismissal.
	if err := m.fromTUI(brokerFrame{"id": lateID, "result": brokerRaw(map[string]string{"decision": "decline"})}); err != nil {
		t.Fatal(err)
	}
	if err := m.fromTUI(frame("thread/read", int64(1), map[string]any{})); err != nil {
		t.Fatal(err)
	}
	if got := receiveNative(t, p); string(got["method"]) != `"thread/read"` {
		t.Fatal(got)
	}
	for _, id := range []string{"foreign", "sessionbus/server/!", "sessionbus/server/bnVsbA"} {
		if err := m.fromTUI(brokerFrame{"id": brokerRaw(id), "result": brokerRaw(nil)}); err == nil {
			t.Fatalf("accepted foreign/malformed response %q", id)
		}
	}
}
func TestBrokerMuxCancelledCallDrainsAndReaderProgresses(t *testing.T) {
	observed := make(chan string, 4)
	m, p := muxFixture(t, func(method string, _, _ json.RawMessage) error { observed <- method; return nil })
	initializeMux(t, m, p)
	<-observed
	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan error, 1)
	go func() {
		var out json.RawMessage
		called <- m.call(ctx, "turn/start", map[string]string{"threadId": "t"}, &out)
	}()
	call := receiveNative(t, p)
	cancel()
	if err := <-called; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := p.Write(frame("turn/completed", nil, map[string]string{"threadId": "t"})); err != nil {
		t.Fatal(err)
	}
	receiveTUI(t, m)
	if <-observed != "turn/completed" {
		t.Fatal("reader stalled")
	}
	if err := p.Write(brokerFrame{"id": call["id"], "result": brokerRaw(map[string]any{"turn": map[string]string{"id": "x"}})}); err != nil {
		t.Fatal(err)
	}
	if <-observed != "turn/start" {
		t.Fatal("late reply did not drain")
	}
	m.mu.Lock()
	remaining := len(m.calls)
	m.mu.Unlock()
	if remaining != 0 {
		t.Fatal(remaining)
	}
}
func TestBrokerMuxInternalWaitsForNativeInitialize(t *testing.T) {
	m, p := muxFixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan error, 1)
	go func() { called <- m.call(ctx, "thread/read", map[string]any{}, nil) }()
	cancel()
	if !errors.Is(<-called, context.Canceled) {
		t.Fatal("cancel")
	}
	// First native frame must be the TUI's initialize, not an internal call.
	initializeMux(t, m, p)
}
func TestBrokerMuxResponseObservationPrecedesNextFrame(t *testing.T) {
	events := make(chan string, 4)
	m, p := muxFixture(t, func(method string, _, _ json.RawMessage) error { events <- method; return nil })
	initializeMux(t, m, p)
	<-events
	if err := m.fromTUI(frame("thread/start", 9, map[string]any{})); err != nil {
		t.Fatal(err)
	}
	f := receiveNative(t, p)
	if err := p.Write(brokerFrame{"id": f["id"], "result": brokerRaw(map[string]any{"thread": map[string]string{"id": "new"}})}); err != nil {
		t.Fatal(err)
	}
	if err := p.Write(frame("thread/closed", nil, map[string]string{"threadId": "new"})); err != nil {
		t.Fatal(err)
	}
	if <-events != "thread/start" || <-events != "thread/closed" {
		t.Fatal("observation order")
	}
}

func TestBrokerRejectsNullIDWithoutAliasingEmptyString(t *testing.T) {
	if _, err := brokerID(json.RawMessage(`null`)); err == nil {
		t.Fatal("accepted null")
	}
	if _, err := brokerID(json.RawMessage(`""`)); err != nil {
		t.Fatal(err)
	}
	m, p := muxFixture(t, nil)
	initializeMux(t, m, p)
	if err := m.fromTUI(frame("thread/read", "", map[string]any{})); err != nil {
		t.Fatal(err)
	}
	f := receiveNative(t, p)
	if err := m.fromTUI(brokerFrame{"id": json.RawMessage(`null`), "method": brokerRaw("thread/read"), "params": brokerRaw(map[string]any{})}); err == nil {
		t.Fatal("null request aliased empty string")
	}
	if err := p.Write(brokerFrame{"id": f["id"], "result": brokerRaw(map[string]any{})}); err != nil {
		t.Fatal(err)
	}
	if got := receiveTUI(t, m); string(got["id"]) != `""` {
		t.Fatal(got)
	}
}
func TestBrokerAggregateByteBudgetAndRelease(t *testing.T) {
	// Drive queue/transport completion explicitly so there is no scheduler gap
	// between a peer read and the native writer releasing its accounted bytes.
	m := &brokerMux{ctx: context.Background(), byteLimit: 240, nativeOut: make(chan brokerPacket, 2), tuiOut: make(chan brokerPacket, 2)}
	f := frame("notice", nil, map[string]string{"body": "one"})
	if err := m.enqueue(m.tuiOut, f); err != nil {
		t.Fatal(err)
	}
	packet := <-m.tuiOut // Dequeue is not completion: an in-write frame stays charged.
	held := m.byteLimit - len(packet.body)
	if err := m.reserve(held); err != nil {
		t.Fatal(err)
	}
	if err := m.enqueue(m.nativeOut, f); err == nil {
		t.Fatal("other queue escaped aggregate budget")
	}
	m.release(held)
	m.release(len(packet.body))
	if err := m.enqueue(m.nativeOut, f); err != nil {
		t.Fatal(err)
	}
	next := <-m.nativeOut
	m.release(len(next.body))
	if m.buffered != 0 {
		t.Fatal(m.buffered)
	}
}
