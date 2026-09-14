// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
)

type blockedAppTransport struct {
	entered, release, closed chan struct{}
	once                     sync.Once
	err                      error
}

func (t *blockedAppTransport) Read(any) error { <-t.closed; return io.EOF }
func (t *blockedAppTransport) Write(any) error {
	close(t.entered)
	select {
	case <-t.closed:
		return io.ErrClosedPipe
	case <-t.release:
		return t.err
	}
}
func (t *blockedAppTransport) Close() error { t.once.Do(func() { close(t.closed) }); return nil }
func TestNativeWriteCancellationAndFailure(t *testing.T) {
	for _, mode := range []string{"blocked-cancel", "error", "short", "success", "waiting-cancel"} {
		t.Run(mode, func(t *testing.T) {
			tr := &blockedAppTransport{entered: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
			if mode == "error" {
				tr.err = io.ErrUnexpectedEOF
			}
			if mode == "short" {
				tr.err = io.ErrShortWrite
			}
			c := newTransportClient(tr, nil, nil)
			defer c.fail(io.EOF)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type result struct {
				attempted bool
				err       error
			}
			done := make(chan result, 1)
			go func() { a, e := c.writeContext(ctx, struct{}{}); done <- result{a, e} }()
			<-tr.entered
			if mode == "waiting-cancel" {
				waiting, stop := context.WithCancel(context.Background())
				stop()
				attempted, err := c.writeContext(waiting, struct{}{})
				if attempted || !errors.Is(err, context.Canceled) || c.isFailed() {
					t.Fatalf("pre-submit=%v %v failed=%v", attempted, err, c.isFailed())
				}
			}
			if mode == "blocked-cancel" {
				cancel()
			} else {
				close(tr.release)
			}
			got := <-done
			if !got.attempted {
				t.Fatal("write was attempted")
			}
			if mode == "success" || mode == "waiting-cancel" {
				if got.err != nil || c.isFailed() {
					t.Fatalf("%+v", got)
				}
			} else {
				if got.err == nil || !c.isFailed() {
					t.Fatalf("failed transport survived: %+v", got)
				}
			}
		})
	}
}
