// SPDX-License-Identifier: MIT
package qwen

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
)

func duplexFixture(t *testing.T, notify func(string, json.RawMessage), answer acpAnswer) (*acpClient, net.Conn, *bufio.Reader) {
	t.Helper()
	local, remote := net.Pipe()
	c := newDuplexACP(local, local, notify, answer)
	t.Cleanup(func() { c.close(); remote.Close(); <-c.done })
	return c, remote, bufio.NewReader(remote)
}
func acpRead(t *testing.T, r *bufio.Reader) acpFrame {
	t.Helper()
	body, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var f acpFrame
	if err = json.Unmarshal(body, &f); err != nil {
		t.Fatal(err)
	}
	return f
}
func acpWrite(t *testing.T, w io.Writer, s string) {
	t.Helper()
	if _, err := io.WriteString(w, s+"\n"); err != nil {
		t.Fatal(err)
	}
}
func TestACPPrecancelDoesNotWrite(t *testing.T) {
	c, remote, r := duplexFixture(t, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.call(ctx, "never", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.call(context.Background(), "real", nil, nil) }()
	f := acpRead(t, r)
	if f.Method != "real" {
		t.Fatalf("unexpected write: %s", f.Method)
	}
	acpWrite(t, remote, `{"jsonrpc":"2.0","id":`+string(f.ID)+`,"result":{}}`)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestACPCancelDrainsLateReplyBeforeNextCall(t *testing.T) {
	c, remote, r := duplexFixture(t, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	written := make(chan error, 1)
	go func() { done <- c.request(ctx, "old", nil, nil, written, nil, nil) }()
	old := acpRead(t, r)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	go func() { done <- c.call(context.Background(), "next", nil, nil) }()
	next := acpRead(t, r)
	acpWrite(t, remote, `{"jsonrpc":"2.0","id":`+string(old.ID)+`,"result":{"old":true}}`)
	acpWrite(t, remote, `{"jsonrpc":"2.0","id":`+string(next.ID)+`,"result":{}}`)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) != 0 || c.retained != 0 {
		t.Fatalf("pending=%d bytes=%d", len(c.pending), c.retained)
	}
}
func TestACPObserverPrecedesBufferedNotification(t *testing.T) {
	seen := make(chan bool, 1)
	observed := false
	c, remote, r := duplexFixture(t, func(string, json.RawMessage) { seen <- observed }, nil)
	done := make(chan error, 1)
	go func() {
		done <- c.request(context.Background(), "prompt", nil, nil, nil, nil, func(json.RawMessage, error) error { observed = true; return nil })
	}()
	f := acpRead(t, r)
	acpWrite(t, remote, `{"jsonrpc":"2.0","id":`+string(f.ID)+`,"result":{}}`+"\n"+`{"jsonrpc":"2.0","method":"session/update","params":{}}`)
	if !<-seen {
		t.Fatal("notification dispatched before terminal observer")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestACPBlockedNativeResponseDoesNotBlockEOF(t *testing.T) {
	selected := make(chan struct{})
	settled := make(chan error, 1)
	c, remote, _ := duplexFixture(t, nil, func(string, json.RawMessage) (*acpResponse, error) {
		close(selected)
		return &acpResponse{Result: map[string]bool{"ok": true}, Finish: func(err error) { settled <- err }}, nil
	})
	acpWrite(t, remote, `{"jsonrpc":"2.0","id":"","method":"native","params":{}}`)
	<-selected
	// Do not read its response. Closing the actual socket releases the writer.
	remote.Close()
	<-c.done
	if err := <-settled; err == nil {
		t.Fatal("abandoned response was reported written")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.requests) != 0 || c.retained != 0 {
		t.Fatalf("work=%d bytes=%d", len(c.requests), c.retained)
	}
}
func TestACPNullRequestIDFailsWithoutDispatch(t *testing.T) {
	dispatched := false
	c, remote, _ := duplexFixture(t, nil, func(string, json.RawMessage) (*acpResponse, error) { dispatched = true; return nil, nil })
	acpWrite(t, remote, `{"jsonrpc":"2.0","id":null,"method":"native","params":{}}`)
	<-c.done
	if dispatched {
		t.Fatal("null aliased empty-string request ID")
	}
}
func TestACPInputFrameBound(t *testing.T) {
	c, remote, _ := duplexFixture(t, nil, nil)
	_, _ = io.WriteString(remote, strings.Repeat("x", maxACPFrame)+"\n")
	<-c.done
	if c.failure() == nil {
		t.Fatal("oversized input accepted")
	}
}
func TestACPCancelledCorrelationsRetainCapacity(t *testing.T) {
	c, _, r := duplexFixture(t, nil, nil)
	for i := 0; i < maxACPPending; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		written := make(chan error, 1)
		go func() { done <- c.request(ctx, "pending", nil, nil, written, nil, nil) }()
		acpRead(t, r)
		if err := <-written; err != nil {
			t.Fatal(err)
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if err := c.call(context.Background(), "overflow", nil, nil); !errors.Is(err, errACPCapacity) {
		t.Fatal(err)
	}
}

type acpHeldWriter struct {
	io.WriteCloser
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *acpHeldWriter) Write(b []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.WriteCloser.Write(b)
}
func (w *acpHeldWriter) Close() error {
	w.once.Do(func() { close(w.entered) })
	select {
	case <-w.release:
	default:
		close(w.release)
	}
	return w.WriteCloser.Close()
}
func TestACPAttemptedWriteCancellationClosesAndJoins(t *testing.T) {
	local, remote := net.Pipe()
	w := &acpHeldWriter{WriteCloser: local, entered: make(chan struct{}), release: make(chan struct{})}
	c := newDuplexACP(w, local, nil, nil)
	defer remote.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.call(ctx, "prompt", nil, nil) }()
	<-w.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-c.done
}

func TestACPNativeRequestBytesBoundBeforeWorkSlots(t *testing.T) {
	c, remote, _ := duplexFixture(t, nil, func(string, json.RawMessage) (*acpResponse, error) {
		return &acpResponse{Result: map[string]bool{"ok": true}}, nil
	})
	// Keep responses unread, allowing admitted requests to queue behind the one
	// blocked writer. Large but individually valid requests hit the byte limit
	// before the 256-request count ceiling, then real EOF joins all writers.
	padding := strings.Repeat("x", 200000)
	var admitted int
	for i := 0; i < maxACPPending; i++ {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": i, "method": "held", "params": map[string]string{"padding": padding}})
		if _, err := remote.Write(append(body, '\n')); err != nil {
			break
		}
		admitted++
	}
	<-c.done
	if !errors.Is(c.failure(), errACPCapacity) || admitted >= maxACPPending {
		t.Fatalf("admitted=%d failure=%v", admitted, c.failure())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.retained != 0 || len(c.requests) != 0 {
		t.Fatalf("retained=%d work=%d", c.retained, len(c.requests))
	}
}
