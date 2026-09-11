// SPDX-License-Identifier: MIT
package opencode

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLaneHTTPWritePrecedesTerminalAndCancellationJoins(t *testing.T) {
	received := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Query().Get("directory") != "/work with spaces" || r.Header.Get("x-opencode-directory") != "/work with spaces" {
			t.Errorf("unscoped request %v", r)
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != "u" || p != "p" {
			t.Error("missing native auth")
		}
		_, _ = io.Copy(io.Discard, r.Body)
		close(received)
		<-r.Context().Done()
	}))
	defer s.Close()
	c := newLaneHTTP(s.URL, "/work with spaces", "u", "p")
	defer c.closeIdle()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := c.prepare(ctx, "POST", "/session/ses_test/message", []byte(`{"parts":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	op, err := c.begin(r, 200)
	if err != nil {
		t.Fatal(err)
	}
	<-received
	<-op.written
	if op.writeErr != nil {
		t.Fatal(op.writeErr)
	}
	select {
	case <-op.done:
		t.Fatal("write invented model terminal")
	default:
	}
	cancel()
	if _, err = op.wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("loss = %v", err)
	}
	if len(c.slots) != 0 {
		t.Fatal("HTTP slot retained after completion")
	}
}

func TestLaneHTTPPreCancelAndSizeDoNotSubmit(t *testing.T) {
	var calls atomic.Int32
	c := newLaneHTTP("http://127.0.0.1:1", "/work", "u", "p")
	c.dial = func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("must not submit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.prepare(ctx, "POST", "/session", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := c.prepare(context.Background(), "POST", "/session", make([]byte, maxNativeRequest+1)); err == nil {
		t.Fatal("oversized request accepted")
	}
	r, err := c.prepare(context.Background(), "POST", "/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	r = r.WithContext(ctx)
	if _, err = c.begin(r, 200); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("preflight wrote")
	}
}

type failedNativeWrite struct{ net.Conn }

func (c failedNativeWrite) Write(b []byte) (int, error) { return 0, io.ErrClosedPipe }
func TestLaneHTTPFailedWriteNeverReportsWritten(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	c := newLaneHTTP("http://native.test", "/work", "u", "p")
	c.dial = func(context.Context, string, string) (net.Conn, error) { return failedNativeWrite{left}, nil }
	defer c.closeIdle()
	r, err := c.prepare(context.Background(), "POST", "/session", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	op, err := c.begin(r, 200)
	if err != nil {
		t.Fatal(err)
	}
	_, err = op.wait()
	if err == nil {
		t.Fatal("failed transport accepted")
	}
	<-op.written
	if op.writeErr == nil {
		t.Fatal("failed write classified written")
	}
}

func TestLaneHTTPBoundsResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", maxNativeResponse+1))
	}))
	defer s.Close()
	c := newLaneHTTP(s.URL, "/work", "u", "p")
	r, _ := c.prepare(context.Background(), "POST", "/session", []byte(`{}`))
	op, err := c.begin(r, 200)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = op.wait(); err == nil {
		t.Fatal("unbounded response")
	}
	<-op.written
	if op.writeErr != nil {
		t.Fatal(op.writeErr)
	}
}

type heldNativeWrite struct {
	net.Conn
	returned <-chan struct{}
	written  chan<- struct{}
}

func (c heldNativeWrite) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.written <- struct{}{}
	<-c.returned
	return n, err
}
func TestLaneHTTPHoldsWriteObservationEvenAfterNativeResponse(t *testing.T) {
	answered := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `{}`)
		close(answered)
	}))
	defer s.Close()
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	wrote := make(chan struct{}, 1)
	c := newLaneHTTP(s.URL, "/work", "u", "p")
	c.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		return heldNativeWrite{conn, release, wrote}, err
	}
	r, _ := c.prepare(context.Background(), "POST", "/session", []byte(`{}`))
	op, err := c.begin(r, 200)
	if err != nil {
		t.Fatal(err)
	}
	<-wrote
	<-answered
	select {
	case <-op.written:
		t.Fatal("write returned while actual writer was held")
	default:
	}
	unblock()
	if _, err = op.wait(); err != nil {
		t.Fatal(err)
	}
	if op.writeErr != nil {
		t.Fatal(op.writeErr)
	}
}

func TestLaneHTTPControlCapacityAndCancellationRelease(t *testing.T) {
	arrivals := make(chan struct{}, maxNativeRequests)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		arrivals <- struct{}{}
		<-r.Context().Done()
	}))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newLaneHTTP(s.URL, "/work", "u", "p")
	var ops []*httpOperation
	for i := 0; i < maxNativeRequests-1; i++ {
		r, _ := c.prepare(ctx, "POST", "/session", []byte(`{}`))
		o, err := c.begin(r, 200)
		if err != nil {
			t.Fatal(err)
		}
		ops = append(ops, o)
		<-arrivals
	}
	r, _ := c.prepare(ctx, "POST", "/session", []byte(`{}`))
	if _, err := c.begin(r, 200); err == nil {
		t.Fatal("ordinary capacity exceeded")
	}
	o, err := c.beginControl(r, 200)
	if err != nil {
		t.Fatal("cancel path starved", err)
	}
	ops = append(ops, o)
	<-arrivals
	cancel()
	for _, o := range ops {
		if _, err := o.wait(); err == nil {
			t.Fatal("lost request succeeded")
		}
	}
	if len(c.slots) != 0 || len(c.control) != 0 {
		t.Fatal("capacity leaked")
	}
}
