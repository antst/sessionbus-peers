// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
)

type boundedTransport struct {
	reads  chan appFrame
	writes chan json.RawMessage
	closed chan struct{}
	once   sync.Once
}

func newBoundedTransport() *boundedTransport {
	return &boundedTransport{reads: make(chan appFrame), writes: make(chan json.RawMessage, 1), closed: make(chan struct{})}
}
func (t *boundedTransport) Read(v any) error {
	select {
	case f := <-t.reads:
		*(v.(*appFrame)) = f
		return nil
	case <-t.closed:
		return io.EOF
	}
}
func (t *boundedTransport) Write(v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	select {
	case t.writes <- b:
		return nil
	case <-t.closed:
		return io.ErrClosedPipe
	}
}
func (t *boundedTransport) Close() error { t.once.Do(func() { close(t.closed) }); return nil }
func completeNativeWrite(c *appClient)   { <-c.writeGate; c.writeGate <- struct{}{} }
func TestNativeCancelledDrainBoundAndLateResponses(t *testing.T) {
	tr := newBoundedTransport()
	barrier := make(chan struct{})
	c := newTransportClient(tr, func(string, json.RawMessage) { barrier <- struct{}{} }, nil)
	defer c.fail(io.EOF)
	var observed atomic.Int32
	for i := 0; i < nativeRequestLimit; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- c.callObserved(ctx, "test", nil, nil, func(json.RawMessage) error { observed.Add(1); return nil })
		}()
		<-tr.writes
		completeNativeWrite(c)
		cancel()
		if e := <-done; !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
	if e := c.call(context.Background(), "overflow", nil, nil); !errors.Is(e, errNativeRequestLimit) {
		t.Fatal(e)
	}
	c.mu.Lock()
	next := c.next
	c.mu.Unlock()
	if next != nativeRequestLimit {
		t.Fatalf("refusal allocated id %d", next)
	}
	select {
	case b := <-tr.writes:
		t.Fatalf("overflow wrote %s", b)
	default:
	}
	tr.reads <- appFrame{ID: json.RawMessage(`1`), Result: json.RawMessage(`{}`)}
	tr.reads <- appFrame{Method: "barrier"}
	<-barrier
	if observed.Load() != 0 {
		t.Fatal("abandoned observer invoked")
	}
	done := make(chan error, 1)
	go func() { done <- c.call(context.Background(), "after-drain", nil, nil) }()
	var request appFrame
	json.Unmarshal(<-tr.writes, &request)
	tr.reads <- appFrame{ID: request.ID, Result: json.RawMessage(`{}`)}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	// A cancelled slot still validates its late response before discarding it.
	tr.reads <- appFrame{ID: json.RawMessage(`2`), Result: json.RawMessage(`{}`), Error: &appError{Code: -32603, Message: "both"}}
	<-c.done
	if !c.isFailed() {
		t.Fatal("malformed late response survived")
	}
}
func TestNativeClaimedObserverCompletionWinsCancellation(t *testing.T) {
	tr := newBoundedTransport()
	c := newTransportClient(tr, nil, nil)
	defer c.fail(io.EOF)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.callObserved(ctx, "test", nil, nil, func(json.RawMessage) error { close(entered); <-release; return nil })
	}()
	var req appFrame
	json.Unmarshal(<-tr.writes, &req)
	completeNativeWrite(c)
	tr.reads <- appFrame{ID: req.ID, Result: json.RawMessage(`{}`)}
	<-entered
	cancel()
	close(release)
	if e := <-done; e != nil {
		t.Fatalf("claimed response lost: %v", e)
	}
}
func TestNativeIncomingRequestBound(t *testing.T) {
	tr := newBoundedTransport()
	entered, exited := make(chan struct{}, nativeRequestLimit), make(chan struct{}, nativeRequestLimit)
	c := newTransportClient(tr, nil, nil, func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
		entered <- struct{}{}
		<-ctx.Done()
		exited <- struct{}{}
		return nil, ctx.Err()
	})
	defer c.fail(io.EOF)
	for i := 1; i <= nativeRequestLimit; i++ {
		id, _ := json.Marshal(i)
		tr.reads <- appFrame{ID: id, Method: "held"}
		<-entered
	}
	tr.reads <- appFrame{ID: json.RawMessage(`257`), Method: "overflow"}
	<-c.done
	for i := 0; i < nativeRequestLimit; i++ {
		<-exited
	}
	c.mu.Lock()
	e := c.failed
	c.mu.Unlock()
	if !errors.Is(e, errNativeRequestLimit) {
		t.Fatal(e)
	}
	select {
	case <-entered:
		t.Fatal("overflow handler started")
	default:
	}
}

func TestNativeResolutionCancelsMatchingNestedRequest(t *testing.T) {
	tr := newBoundedTransport()
	var c *appClient
	completed := make(chan error, 1)
	barrier := make(chan struct{})
	c = newTransportClient(tr, func(string, json.RawMessage) { barrier <- struct{}{} }, nil, func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
		err := c.call(ctx, "mcpServer/tool/call", map[string]string{"threadId": "owned"}, nil)
		completed <- err
		return map[string]bool{"success": true}, err
	})
	tr.reads <- appFrame{ID: json.RawMessage(`"native-tool"`), Method: "item/tool/call", Params: json.RawMessage(`{"threadId":"owned"}`)}
	var nested appFrame
	json.Unmarshal(<-tr.writes, &nested)
	completeNativeWrite(c)
	// Same request ID with a foreign thread must not cancel the owned operation.
	tr.reads <- appFrame{Method: "serverRequest/resolved", Params: json.RawMessage(`{"requestId":"native-tool","threadId":"foreign"}`)}
	tr.reads <- appFrame{Method: "barrier"}
	<-barrier
	select {
	case err := <-completed:
		t.Fatalf("foreign resolution cancelled: %v", err)
	default:
	}
	tr.reads <- appFrame{Method: "serverRequest/resolved", Params: json.RawMessage(`{"requestId":"native-tool","threadId":"owned"}`)}
	if err := <-completed; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Already-written nested work is drained, not undone or replayed.
	tr.reads <- appFrame{ID: nested.ID, Result: json.RawMessage(`{}`)}
	tr.reads <- appFrame{Method: "barrier"}
	<-barrier
	c.fail(io.EOF)
	<-c.done
	select {
	case raw := <-tr.writes:
		t.Fatalf("resolved native request got stale reply: %s", raw)
	default:
	}
}
