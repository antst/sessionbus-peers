// SPDX-License-Identifier: MIT
package qwen

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

type reconnectDial struct {
	result chan net.Conn
	done   chan struct{}
}
type reconnectFixture struct {
	b             *interactiveOwner
	history       string
	dial          chan reconnectDial
	retry, resume chan struct{}
}

func reconnectOwner(t *testing.T) *reconnectFixture {
	t.Helper()
	b, history, registry := ownerFixture(t, "")
	f := &reconnectFixture{b: b, history: history, dial: make(chan reconnectDial), retry: make(chan struct{}), resume: make(chan struct{})}
	b.dial = func(ctx context.Context, network, socket string) (net.Conn, error) {
		if network != "unix" || socket != b.launch.Socket {
			return nil, errors.New("dial changed frozen launch endpoint")
		}
		request := reconnectDial{result: make(chan net.Conn), done: make(chan struct{})}
		defer close(request.done)
		select {
		case f.dial <- request:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case c := <-request.result:
			if c == nil {
				return nil, os.ErrNotExist
			}
			return c, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	b.retry = func(ctx context.Context) bool {
		select {
		case f.retry <- struct{}{}:
		case <-ctx.Done():
			return false
		}
		select {
		case <-f.resume:
			return true
		case <-ctx.Done():
			return false
		}
	}
	registry()
	return f
}

func receiveReconnect[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect fixture barrier timed out")
		var zero T
		return zero
	}
}
func sendReconnect[T any](t *testing.T, ch chan<- T, value T) {
	t.Helper()
	select {
	case ch <- value:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect fixture handoff timed out")
	}
}

type reconnectWire struct {
	net.Conn
	decoder *json.Decoder
	encoder *json.Encoder
}
type reconnectFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
}

func (f *reconnectFixture) wire(t *testing.T) *reconnectWire {
	t.Helper()
	attempt := receiveReconnect(t, f.dial)
	client, server := net.Pipe()
	t.Cleanup(func() { server.Close() })
	must(t, server.SetDeadline(time.Now().Add(5*time.Second)))
	sendReconnect(t, attempt.result, net.Conn(client))
	receiveReconnect(t, attempt.done)
	return &reconnectWire{server, json.NewDecoder(server), json.NewEncoder(server)}
}
func (w *reconnectWire) read(t *testing.T) reconnectFrame {
	t.Helper()
	var frame reconnectFrame
	must(t, w.decoder.Decode(&frame))
	return frame
}
func (w *reconnectWire) reply(t *testing.T, request reconnectFrame, value any) {
	t.Helper()
	must(t, w.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": value}))
}
func (w *reconnectWire) hello(t *testing.T, f *reconnectFixture, title string) reconnectFrame {
	t.Helper()
	frame := w.read(t)
	check(t, frame.Method == "session.hello", "first method=%s", frame.Method)
	var identity kit.PeerIdentity
	must(t, json.Unmarshal(frame.Params, &identity))
	want := kit.PeerIdentity{Protocol: 1, Product: Product, SessionID: fixtureID, Name: title, Groups: []string{"a", "b"}, Info: map[string]any{"cwd": filepath.Join(f.b.home, "project😀")}}
	check(t, reflect.DeepEqual(identity, want), "identity=%+v want=%+v", identity, want)
	return frame
}
func reconnectCall(b *interactiveOwner, method string, args any) <-chan error {
	done := make(chan error, 1)
	go func() { _, e := b.Call(context.Background(), method, args); done <- e }()
	return done
}
func reconnectListed(t *testing.T, f *reconnectFixture, w *reconnectWire) {
	t.Helper()
	done := reconnectCall(f.b, "session.list", map[string]any{})
	frame := w.read(t)
	check(t, frame.Method == "session.list", "unexpected/replayed method %s: %s", frame.Method, frame.Params)
	w.reply(t, frame, map[string]any{"sessions": []any{}})
	must(t, receiveReconnect(t, done))
}
func disconnectedReconnect(t *testing.T, b *interactiveOwner) {
	t.Helper()
	_, e := b.Call(context.Background(), "session.list", map[string]any{})
	var protocolError *kit.ProtocolError
	check(t, errors.As(e, &protocolError) && protocolError.Code == protocol.NotConnected, "outage call=%v", e)
}

func TestInteractiveReconnectInitialDaemonAbsent(t *testing.T) {
	f := reconnectOwner(t)
	b := f.b
	b.Initialized()
	first := receiveReconnect(t, f.dial)
	sendReconnect(t, first.result, net.Conn(nil))
	receiveReconnect(t, first.done)
	receiveReconnect(t, f.retry)
	claim, e := os.Stat(filepath.Join(b.launch.Directory, "owner.claim"))
	must(t, e)
	check(t, b.ctx.Err() == nil, "initial absence canceled owner")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = b.Call(canceled, "session.list", map[string]any{})
	check(t, errors.Is(e, context.Canceled), "initial gated call=%v", e)
	sendReconnect(t, f.resume, struct{}{})
	w := f.wire(t)
	hello := w.hello(t, f, "")
	w.reply(t, hello, struct{}{})
	receiveReconnect(t, b.ready)
	reconnectListed(t, f, w)
	again, e := os.Stat(filepath.Join(b.launch.Directory, "owner.claim"))
	must(t, e)
	check(t, os.SameFile(claim, again), "claim was replaced")
	input, e := os.ReadFile(filepath.Join(b.launch.Directory, "input.jsonl"))
	must(t, e)
	check(t, len(input) == 0, "retry wrote native input")
}

func TestInteractiveReconnectUsesLatestNativeTitle(t *testing.T) {
	f := reconnectOwner(t)
	f.b.Initialized()
	w := f.wire(t)
	hello := w.hello(t, f, "")
	w.reply(t, hello, struct{}{})
	receiveReconnect(t, f.b.ready)
	must(t, w.Close())
	receiveReconnect(t, f.retry)
	// During retry no supervisor consumes changed, so this channel is an exact
	// observer handoff, not a sleep used as evidence that the history was read.
	awaitTitle := func(want string) {
		t.Helper()
		for {
			f.b.mu.Lock()
			got := f.b.identity.Name
			f.b.mu.Unlock()
			if got == want {
				return
			}
			receiveReconnect(t, f.b.changed)
		}
	}
	appendFixtureJSON(t, f.history, titleRecord("intermediate"))
	awaitTitle("intermediate")
	appendFixtureJSON(t, f.history, titleRecord(""))
	awaitTitle("")
	disconnectedReconnect(t, f.b)
	sendReconnect(t, f.resume, struct{}{})
	next := f.wire(t)
	held := next.hello(t, f, "")
	appendFixtureJSON(t, f.history, titleRecord("latest"))
	awaitTitle("latest")
	next.reply(t, held, struct{}{})
	fresh := next.hello(t, f, "latest")
	disconnectedReconnect(t, f.b) // Old ACK cannot admit stale identity.
	next.reply(t, fresh, struct{}{})
	// An inbound frame after ACK is a reader-order barrier for admission.
	must(t, next.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 900, "method": "message.deliver", "params": reconnectDelivery("current")}))
	receipt := next.read(t)
	check(t, string(receipt.ID) == "900" && strings.Contains(string(receipt.Result), "written"), "fresh admission=%+v", receipt)
	reconnectListed(t, f, next)
}

func TestInteractiveReconnectDoesNotReplayLostCall(t *testing.T) {
	f := reconnectOwner(t)
	f.b.Initialized()
	w := f.wire(t)
	hello := w.hello(t, f, "")
	w.reply(t, hello, struct{}{})
	receiveReconnect(t, f.b.ready)
	done := reconnectCall(f.b, "message.send", map[string]any{"target": "target", "message": "old-marker"})
	sent := w.read(t)
	check(t, sent.Method == "message.send" && strings.Contains(string(sent.Params), "old-marker"), "old request missing")
	must(t, w.Close())
	check(t, receiveReconnect(t, done) != nil, "lost call succeeded")
	receiveReconnect(t, f.retry)
	disconnectedReconnect(t, f.b)
	sendReconnect(t, f.resume, struct{}{})
	next := f.wire(t)
	hello = next.hello(t, f, "")
	next.reply(t, hello, struct{}{})
	// Receiving a new list response on the ordered replacement wire proves that
	// no old send was emitted ahead of it. The caller has no retry goroutine.
	must(t, next.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 901, "method": "message.deliver", "params": reconnectDelivery("barrier")}))
	next.read(t)
	reconnectListed(t, f, next)
}

func reconnectDelivery(body string) map[string]any {
	return map[string]any{"message_id": "m-" + body, "from": map[string]any{"session_id": "sender", "product": "test", "groups": []string{}}, "body": body}
}

func TestInteractiveReconnectEndJoinsAttempts(t *testing.T) {
	for _, phase := range []string{"retry", "dial", "hello"} {
		t.Run(phase, func(t *testing.T) {
			f := reconnectOwner(t)
			f.b.Initialized()
			var attempt reconnectDial
			switch phase {
			case "retry":
				attempt = receiveReconnect(t, f.dial)
				sendReconnect(t, attempt.result, net.Conn(nil))
				receiveReconnect(t, attempt.done)
				receiveReconnect(t, f.retry)
			case "dial":
				attempt = receiveReconnect(t, f.dial)
			case "hello":
				w := f.wire(t)
				w.hello(t, f, "")
			}
			ended := make(chan struct{})
			go func() { f.b.End(); close(ended) }()
			receiveReconnect(t, ended)
			if phase == "dial" {
				receiveReconnect(t, attempt.done)
			}
			select {
			case <-f.dial:
				t.Fatal("attempt survived End")
			default:
			}
			select {
			case <-f.retry:
				t.Fatal("retry survived End")
			default:
			}
		})
	}
}

func TestInteractiveReconnectTerminalSuperseded(t *testing.T) {
	for _, refused := range []bool{false, true} {
		t.Run(map[bool]string{false: "superseded", true: "invalid-hello"}[refused], func(t *testing.T) {
			f := reconnectOwner(t)
			f.b.Initialized()
			w := f.wire(t)
			hello := w.hello(t, f, "")
			if refused {
				must(t, w.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": hello.ID, "error": map[string]any{"code": protocol.InvalidHello, "message": "invalid_hello"}}))
			} else {
				w.reply(t, hello, struct{}{})
				receiveReconnect(t, f.b.ready)
				must(t, w.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 902, "method": "session.superseded", "params": map[string]any{}}))
			}
			receiveReconnect(t, f.b.done)
			f.b.End()
			var failure *kit.ProtocolError
			check(t, errors.As(f.b.failure(), &failure), "missing terminal protocol error: %v", f.b.failure())
			select {
			case <-f.retry:
				t.Fatal("terminal disposition retried")
			default:
			}
			binding, e := json.Marshal(f.b.launch)
			must(t, e)
			other, e := newInteractiveOwner(context.Background(), []string{InteractiveEnv + "=" + string(binding), nativeSessionEnv + "=" + fixtureID, "QWEN_HOME=" + f.b.home}, os.Getpid())
			must(t, e)
			defer other.End()
			other.Initialized()
			receiveReconnect(t, other.done)
			check(t, strings.Contains(other.failure().Error(), "already had an integration owner"), "terminal claim was reusable")
		})
	}
}

func TestInteractiveReconnectIdleEOFKeepsMCP(t *testing.T) {
	f := reconnectOwner(t)
	client, server := net.Pipe()
	defer client.Close()
	must(t, client.SetDeadline(time.Now().Add(5*time.Second)))
	served := make(chan error, 1)
	go func() { served <- serveInteractiveOwner(context.Background(), f.b, server, server) }()
	encoder, decoder := json.NewEncoder(client), json.NewDecoder(client)
	must(t, encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}}))
	var result map[string]any
	must(t, decoder.Decode(&result))
	must(t, encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}))
	w := f.wire(t)
	hello := w.hello(t, f, "")
	w.reply(t, hello, struct{}{})
	receiveReconnect(t, f.b.ready)
	must(t, w.Close())
	receiveReconnect(t, f.retry)
	must(t, encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}))
	must(t, decoder.Decode(&result))
	check(t, result["error"] == nil, "MCP list failed during outage: %v", result)
	must(t, encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "sessionbus", "arguments": map[string]any{"action": "list", "arguments": map[string]any{}}}}))
	must(t, decoder.Decode(&result))
	check(t, result["result"].(map[string]any)["isError"] == true, "outage action did not return local error: %v", result)
	sendReconnect(t, f.resume, struct{}{})
	next := f.wire(t)
	hello = next.hello(t, f, "")
	next.reply(t, hello, struct{}{})
	must(t, next.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 903, "method": "message.deliver", "params": reconnectDelivery("mcp-barrier")}))
	next.read(t)
	must(t, encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "sessionbus", "arguments": map[string]any{"action": "list", "arguments": map[string]any{}}}}))
	call := next.read(t)
	check(t, call.Method == "session.list", "recovered MCP action=%s", call.Method)
	next.reply(t, call, map[string]any{"sessions": []any{}})
	must(t, decoder.Decode(&result))
	check(t, result["result"].(map[string]any)["isError"] != true, "recovered MCP action failed: %v", result)
	must(t, client.Close())
	err := receiveReconnect(t, served)
	check(t, err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed), "MCP close=%v", err)
}

func TestInteractiveReconnectCancelsOldDelivery(t *testing.T) {
	for _, written := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting-append", true: "written-receipt-lost"}[written], func(t *testing.T) {
			f := reconnectOwner(t)
			b := f.b
			b.Initialized()
			w := f.wire(t)
			hello := w.hello(t, f, "")
			w.reply(t, hello, struct{}{})
			receiveReconnect(t, b.ready)
			path := filepath.Join(b.launch.Directory, "input.jsonl")
			watch, e := newInteractiveWatch(b.parent)
			must(t, e)
			defer watch.close()
			must(t, watch.add(path))
			if !written {
				<-b.appendGate
			}
			// A response following delivery on the same reader establishes handler
			// admission before disconnect, independently of goroutine scheduling.
			probe := reconnectCall(b, "session.list", map[string]any{})
			request := w.read(t)
			must(t, w.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 904, "method": "message.deliver", "params": reconnectDelivery("old-delivery")}))
			w.reply(t, request, map[string]any{"sessions": []any{}})
			must(t, receiveReconnect(t, probe))
			if written {
				waitFileCondition(t, watch, func() bool {
					data, err := os.ReadFile(path)
					return err == nil && strings.Contains(string(data), "old-delivery")
				})
			}
			must(t, w.Close())
			receiveReconnect(t, f.retry)
			joined := make(chan struct{})
			go func() { b.work.Wait(); close(joined) }()
			receiveReconnect(t, joined)
			if !written {
				b.appendGate <- struct{}{}
			}
			sendReconnect(t, f.resume, struct{}{})
			next := f.wire(t)
			hello = next.hello(t, f, "")
			next.reply(t, hello, struct{}{})
			must(t, next.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 905, "method": "message.deliver", "params": reconnectDelivery("new-delivery")}))
			receipt := next.read(t)
			check(t, strings.Contains(string(receipt.Result), "written"), "new delivery failed: %+v", receipt)
			data, e := os.ReadFile(path)
			must(t, e)
			wantOld := 0
			if written {
				wantOld = 1
			}
			// Bodies appear once in the rendered native message; count decoded records,
			// rather than SDK framing or timing an absence on the wire.
			records := strings.Split(strings.TrimSpace(string(data)), "\n")
			old, newCount := 0, 0
			for _, record := range records {
				if strings.Contains(record, "old-delivery") {
					old++
				}
				if strings.Contains(record, "new-delivery") {
					newCount++
				}
			}
			check(t, old == wantOld && newCount == 1, "native records old=%d new=%d: %s", old, newCount, data)
			reconnectListed(t, f, next)
		})
	}
}

func TestInteractiveReconnectCanceledHelloCannotPublish(t *testing.T) {
	f := reconnectOwner(t)
	f.b.Initialized()
	w := f.wire(t)
	held := w.hello(t, f, "")
	f.b.End()
	err := w.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": held.ID, "result": map[string]any{}})
	check(t, err != nil, "canceled hello transport remained writable")
	f.b.mu.Lock()
	admitted, conn := f.b.admitted, f.b.conn
	f.b.mu.Unlock()
	check(t, !admitted && conn == nil, "late hello revived ended owner")
	select {
	case <-f.b.ready:
		t.Fatal("canceled hello published initial readiness")
	default:
	}
	select {
	case <-f.retry:
		t.Fatal("canceled hello retried")
	default:
	}
}
