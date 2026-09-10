// SPDX-License-Identifier: MIT
package grok

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

func acpPipe(t *testing.T) (*acpClient, *json.Decoder, *json.Encoder, io.Closer) {
	t.Helper()
	requestRead, requestWrite := io.Pipe()
	responseRead, responseWrite := io.Pipe()
	c := newACPClient(requestWrite, responseRead, nil)
	t.Cleanup(func() { c.close(); _ = requestRead.Close(); _ = responseWrite.Close() })
	return c, json.NewDecoder(requestRead), json.NewEncoder(responseWrite), responseWrite
}
func readACP(t *testing.T, d *json.Decoder) acpFrame {
	t.Helper()
	var f acpFrame
	if err := d.Decode(&f); err != nil {
		t.Fatal(err)
	}
	return f
}
func replyACP(t *testing.T, e *json.Encoder, f acpFrame, value any) {
	t.Helper()
	if err := e.Encode(map[string]any{"jsonrpc": "2.0", "id": f.ID, "result": value}); err != nil {
		t.Fatal(err)
	}
}
func TestResponseBeforeEOFWins(t *testing.T) {
	c, d, e, end := acpPipe(t)
	done := make(chan error, 1)
	var result map[string]string
	go func() { done <- c.request(context.Background(), "example", nil, &result) }()
	f := readACP(t, d)
	replyACP(t, e, f, map[string]string{"value": "read"})
	_ = end.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if result["value"] != "read" {
		t.Fatal(result)
	}
}
func TestACPConcurrentCallsAndCancelledDrain(t *testing.T) {
	c, d, e, _ := acpPipe(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	started := make(chan error, 1)
	go func() { done <- c.requestStarted(ctx, "first", nil, nil, started) }()
	first := readACP(t, d)
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	cancel()
	if !errors.Is(<-done, context.Canceled) {
		t.Fatal("cancellation lost")
	}
	go func() { done <- c.request(context.Background(), "second", nil, nil) }()
	second := readACP(t, d)
	replyACP(t, e, second, map[string]bool{"ok": true})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	replyACP(t, e, first, map[string]bool{"late": true})
	go func() { done <- c.request(context.Background(), "barrier", nil, nil) }()
	barrier := readACP(t, d)
	replyACP(t, e, barrier, map[string]bool{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) != 0 {
		t.Fatal(c.pending)
	}
}
func TestACPActorAcknowledgementOrdering(t *testing.T) {
	for _, early := range []bool{false, true} {
		t.Run(map[bool]string{false: "reply-first", true: "echo-first"}[early], func(t *testing.T) {
			c, d, e, _ := acpPipe(t)
			done := make(chan error, 1)
			go func() { done <- c.interject(context.Background(), "session", "message", "text") }()
			f := readACP(t, d)
			notice := func() {
				if err := e.Encode(map[string]any{"jsonrpc": "2.0", "method": "_x.ai/session/interjection", "params": map[string]string{"sessionId": "session", "interjectionId": "message"}}); err != nil {
					t.Fatal(err)
				}
			}
			if early {
				notice()
			}
			replyACP(t, e, f, map[string]any{"result": map[string]string{"status": "queued"}})
			if !early {
				notice()
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestACPQueuedWithoutActorAckFailsOnEOF(t *testing.T) {
	c, d, e, end := acpPipe(t)
	done := make(chan error, 1)
	go func() { done <- c.interject(context.Background(), "session", "message", "text") }()
	replyACP(t, e, readACP(t, d), map[string]any{"result": map[string]string{"status": "queued"}})
	_ = end.Close()
	if err := <-done; err == nil {
		t.Fatal("queued reply was counted as admission")
	}
}
func TestACPBlockedWriteCancellationClosesTransport(t *testing.T) {
	requestRead, requestWrite := io.Pipe()
	responseRead, responseWrite := io.Pipe()
	c := newACPClient(requestWrite, responseRead, nil)
	defer c.close()
	defer requestRead.Close()
	defer responseWrite.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.request(ctx, "blocked", nil, nil) }()
	if _, err := io.ReadFull(requestRead, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-c.done
}

func TestACPCancelledRequestsRemainBounded(t *testing.T) {
	c, d, e, _ := acpPipe(t)
	var first acpFrame
	for i := 0; i < maxACPPending; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan error, 1)
		done := make(chan error, 1)
		go func() { done <- c.requestStarted(ctx, "held", nil, nil, started) }()
		f := readACP(t, d)
		if i == 0 {
			first = f
		}
		if err := <-started; err != nil {
			t.Fatal(err)
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if err := c.request(context.Background(), "overflow", nil, nil); !errors.Is(err, errACPCapacity) {
		t.Fatal(err)
	}
	// Reader-ordered notification is a barrier after the late reply releases capacity.
	barrier := make(chan struct{})
	c.notify = func(acpFrame) { close(barrier) }
	replyACP(t, e, first, map[string]bool{})
	if err := e.Encode(map[string]any{"jsonrpc": "2.0", "method": "barrier"}); err != nil {
		t.Fatal(err)
	}
	<-barrier
	done := make(chan error, 1)
	go func() { done <- c.request(context.Background(), "after-drain", nil, nil) }()
	replyACP(t, e, readACP(t, d), map[string]bool{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestACPMalformedCancelledResponseRetiresConnection(t *testing.T) {
	c, d, e, _ := acpPipe(t)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	done := make(chan error, 1)
	go func() { done <- c.requestStarted(ctx, "held", nil, nil, started) }()
	f := readACP(t, d)
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
	if err := e.Encode(map[string]any{"jsonrpc": "2.0", "id": f.ID}); err != nil {
		t.Fatal(err)
	}
	<-c.done
}
