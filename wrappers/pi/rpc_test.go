// SPDX-License-Identifier: MIT
package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type nativeRPCTestPeer struct {
	commands *bufio.Reader
	events   *io.PipeWriter
}

func nativeRPCTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func newNativeRPCTest(t *testing.T, observe func(json.RawMessage) error, limits nativeRPCLimits) (*nativeRPC, *nativeRPCTestPeer) {
	t.Helper()
	commands, input := io.Pipe()
	output, events := io.Pipe()
	rpc, err := newNativeRPC(input, output, observe, limits)
	if err != nil {
		t.Fatal(err)
	}
	peer := &nativeRPCTestPeer{commands: bufio.NewReader(commands), events: events}
	t.Cleanup(func() {
		_ = commands.Close()
		_ = events.Close()
		_ = rpc.Close()
	})
	return rpc, peer
}

func (peer *nativeRPCTestPeer) read(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	type readResult struct {
		line string
		err  error
	}
	result := make(chan readResult, 1)
	go func() {
		line, err := peer.commands.ReadString('\n')
		result <- readResult{line, err}
	}()
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatal(got.err)
		}
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(got.line), &object) != nil || object == nil {
			t.Fatalf("command = %q", got.line)
		}
		return object
	case <-nativeRPCTestContext(t).Done():
		t.Fatal("reading Pi native RPC command timed out")
	}
	return nil
}

func (peer *nativeRPCTestPeer) write(t *testing.T, frame string) {
	t.Helper()
	if _, err := peer.events.Write([]byte(frame + "\n")); err != nil {
		t.Fatal(err)
	}
}

func nativeRPCField(t *testing.T, object map[string]json.RawMessage, key string) string {
	t.Helper()
	var value string
	if json.Unmarshal(object[key], &value) != nil {
		t.Fatalf("%s = %s", key, object[key])
	}
	return value
}

func waitNativeRPCDone(t *testing.T, rpc *nativeRPC) {
	t.Helper()
	select {
	case <-rpc.Done():
	case <-nativeRPCTestContext(t).Done():
		t.Fatal("waiting for Pi native RPC shutdown timed out")
	}
}

func waitNativeRPCStats(t *testing.T, rpc *nativeRPC, want nativeRPCStats) {
	t.Helper()
	ctx := nativeRPCTestContext(t)
	for rpc.Stats() != want {
		select {
		case <-ctx.Done():
			t.Fatalf("Pi native RPC stats = %#v, want %#v", rpc.Stats(), want)
		default:
			runtime.Gosched()
		}
	}
}

func TestNativeRPCOutOfOrderResponsesAndNativeError(t *testing.T) {
	rpc, peer := newNativeRPCTest(t, nil, nativeRPCLimits{})
	type value struct {
		Value string `json:"value"`
	}
	type outcome struct {
		value value
		err   error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			var result value
			err := rpc.Call(nativeRPCTestContext(t), "get_state", nil, &result)
			results <- outcome{result, err}
		}()
	}
	first, second := peer.read(t), peer.read(t)
	firstID, secondID := nativeRPCField(t, first, "id"), nativeRPCField(t, second, "id")
	if firstID != "pi:1" || secondID != "pi:2" {
		t.Fatalf("request IDs = %q, %q", firstID, secondID)
	}
	peer.write(t, `{"id":"`+secondID+`","type":"response","command":"get_state","success":true,"data":{"value":"second"}}`)
	peer.write(t, `{"id":"`+firstID+`","type":"response","command":"get_state","success":true,"data":{"value":"first"}}`)
	seen := map[string]bool{}
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatal(got.err)
		}
		seen[got.value.Value] = true
	}
	if !seen["first"] || !seen["second"] {
		t.Fatalf("results = %#v", seen)
	}

	nativeError := make(chan error, 1)
	go func() {
		var history json.RawMessage
		nativeError <- rpc.Call(nativeRPCTestContext(t), "get_entries", map[string]any{"since": "gone"}, &history)
	}()
	request := peer.read(t)
	peer.write(t, `{"id":"`+nativeRPCField(t, request, "id")+`","type":"response","command":"get_entries","success":false,"error":"Entry not found: gone"}`)
	err := <-nativeError
	var rejected *nativeRPCError
	if !errors.As(err, &rejected) || rejected.Command != "get_entries" || rejected.Message != "Entry not found: gone" {
		t.Fatalf("native error = %#v", err)
	}
	waitNativeRPCStats(t, rpc, nativeRPCStats{})
}

func TestNativeRPCRejectedCommandDoesNotConsumeID(t *testing.T) {
	limits := defaultNativeRPCLimits
	limits.maxInputFrame = 256
	rpc, peer := newNativeRPCTest(t, nil, limits)
	if err := rpc.Call(nativeRPCTestContext(t), "prompt", map[string]any{"message": strings.Repeat("x", 300)}, nil); !errors.Is(err, errNativeRPCProtocol) {
		t.Fatalf("oversized call error = %v", err)
	}
	if err := rpc.Call(nativeRPCTestContext(t), "prompt", map[string]any{"id": "foreign"}, nil); !errors.Is(err, errNativeRPCProtocol) {
		t.Fatalf("reserved field error = %v", err)
	}
	result := make(chan error, 1)
	go func() { result <- rpc.Call(nativeRPCTestContext(t), "get_state", nil, nil) }()
	request := peer.read(t)
	if id := nativeRPCField(t, request, "id"); id != "pi:1" {
		t.Fatalf("request ID after local rejection = %q", id)
	}
	peer.write(t, `{"id":"pi:1","type":"response","command":"get_state","success":true,"data":{}}`)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestNativeRPCCompletedWriteWinsSimultaneousCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	write := newNativeRPCWrite([]byte("owned"))
	write.complete(nil)
	cancel()
	if err := waitNativeRPCWrite(ctx, write); err != nil {
		t.Fatalf("completed write disposition = %v", err)
	}
	if write.body != nil {
		t.Fatalf("completed write retained %d bytes", len(write.body))
	}
}

func TestNativeRPCAcceptedResponseWinsSimultaneousCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	write := newNativeRPCWrite([]byte("held"))
	pending := &nativeRPCPending{result: make(chan nativeRPCResult, 1)}
	pending.result <- nativeRPCResult{data: json.RawMessage(`{"accepted":true}`)}
	completed, responded, err := waitNativeRPCAdmission(ctx, write, pending)
	if err != nil || !responded || string(completed.data) != `{"accepted":true}` {
		t.Fatalf("accepted response disposition = %s, %v, %v", completed.data, responded, err)
	}
}

func TestNativeRPCResponseViolationsRetireAndReleaseCaller(t *testing.T) {
	tests := map[string]string{
		"unknown":          `{"id":"pi:99","type":"response","command":"get_state","success":true,"data":{}}`,
		"command mismatch": `{"id":"pi:1","type":"response","command":"abort","success":true}`,
		"invalid shape":    `{"id":"pi:1","type":"response","command":"get_state","success":true,"error":"bad"}`,
	}
	for name, frame := range tests {
		t.Run(name, func(t *testing.T) {
			rpc, peer := newNativeRPCTest(t, nil, nativeRPCLimits{})
			if name == "unknown" {
				peer.write(t, frame)
				waitNativeRPCDone(t, rpc)
			} else {
				result := make(chan error, 1)
				go func() { result <- rpc.Call(nativeRPCTestContext(t), "get_state", nil, nil) }()
				_ = peer.read(t)
				peer.write(t, frame)
				waitNativeRPCDone(t, rpc)
				if err := <-result; !errors.Is(err, errNativeRPCProtocol) {
					t.Fatalf("call error = %v", err)
				}
			}
			if !errors.Is(rpc.Err(), errNativeRPCProtocol) {
				t.Fatalf("RPC error = %v", rpc.Err())
			}
		})
	}
}

func TestNativeRPCDuplicateResponseRetiresAfterAcceptedResult(t *testing.T) {
	rpc, peer := newNativeRPCTest(t, nil, nativeRPCLimits{})
	result := make(chan error, 1)
	go func() { result <- rpc.Call(nativeRPCTestContext(t), "abort", nil, nil) }()
	request := peer.read(t)
	id := nativeRPCField(t, request, "id")
	frame := `{"id":"` + id + `","type":"response","command":"abort","success":true}`
	peer.write(t, frame)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	peer.write(t, frame)
	waitNativeRPCDone(t, rpc)
	if !errors.Is(rpc.Err(), errNativeRPCProtocol) {
		t.Fatalf("RPC error = %v", rpc.Err())
	}
}

func TestNativeRPCPostWriteCancellationDrainsLateResponse(t *testing.T) {
	rpc, peer := newNativeRPCTest(t, nil, nativeRPCLimits{})
	callCtx, cancel := context.WithCancel(nativeRPCTestContext(t))
	result := make(chan error, 1)
	go func() { result <- rpc.Call(callCtx, "get_state", nil, nil) }()
	first := peer.read(t)
	ctx := nativeRPCTestContext(t)
	for rpc.Stats().pendingWrites != 0 {
		select {
		case <-ctx.Done():
			t.Fatal("Pi command write did not settle")
		default:
			runtime.Gosched()
		}
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled call error = %v", err)
	}
	peer.write(t, `{"id":"`+nativeRPCField(t, first, "id")+`","type":"response","command":"get_state","success":true,"data":{}}`)

	healthy := make(chan error, 1)
	go func() { healthy <- rpc.Call(nativeRPCTestContext(t), "abort", nil, nil) }()
	second := peer.read(t)
	peer.write(t, `{"id":"`+nativeRPCField(t, second, "id")+`","type":"response","command":"abort","success":true}`)
	if err := <-healthy; err != nil {
		t.Fatal(err)
	}
	waitNativeRPCStats(t, rpc, nativeRPCStats{})
}

func TestNativeRPCPendingCapacityRejectsBeforeWrite(t *testing.T) {
	limits := defaultNativeRPCLimits
	limits.maxPendingCalls = 1
	rpc, peer := newNativeRPCTest(t, nil, limits)
	first := make(chan error, 1)
	go func() { first <- rpc.Call(nativeRPCTestContext(t), "get_state", nil, nil) }()
	request := peer.read(t)
	if err := rpc.Call(nativeRPCTestContext(t), "abort", nil, nil); !errors.Is(err, errNativeRPCBusy) {
		t.Fatalf("second call error = %v", err)
	}
	if stats := rpc.Stats(); stats.pendingCalls != 1 {
		t.Fatalf("stats after rejected call = %#v", stats)
	}
	peer.write(t, `{"id":"`+nativeRPCField(t, request, "id")+`","type":"response","command":"get_state","success":true,"data":{}}`)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	waitNativeRPCStats(t, rpc, nativeRPCStats{})
}

type heldNativeRPCWriter struct {
	io.WriteCloser
	enabled atomic.Bool
	held    atomic.Bool
	wrote   chan struct{}
	release chan struct{}
}

func (writer *heldNativeRPCWriter) Write(body []byte) (int, error) {
	n, err := writer.WriteCloser.Write(body)
	if writer.enabled.Load() && writer.held.CompareAndSwap(false, true) {
		close(writer.wrote)
		<-writer.release
	}
	return n, err
}

func newHeldNativeRPCTest(t *testing.T, limits nativeRPCLimits) (*nativeRPC, *nativeRPCTestPeer, *heldNativeRPCWriter, func()) {
	t.Helper()
	commands, input := io.Pipe()
	output, events := io.Pipe()
	held := &heldNativeRPCWriter{WriteCloser: input, wrote: make(chan struct{}), release: make(chan struct{})}
	rpc, err := newNativeRPC(held, output, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	peer := &nativeRPCTestPeer{commands: bufio.NewReader(commands), events: events}
	var once sync.Once
	release := func() { once.Do(func() { close(held.release) }) }
	t.Cleanup(func() {
		release()
		_ = commands.Close()
		_ = events.Close()
		_ = rpc.Close()
	})
	return rpc, peer, held, release
}

func TestNativeRPCAcceptedResponseCompletesBeforeWriteCallback(t *testing.T) {
	rpc, peer, held, release := newHeldNativeRPCTest(t, nativeRPCLimits{})
	held.enabled.Store(true)
	callCtx := nativeRPCTestContext(t)
	type state struct {
		SessionID string `json:"sessionId"`
	}
	result := make(chan struct {
		state state
		err   error
	}, 1)
	go func() {
		var value state
		err := rpc.Call(callCtx, "get_state", nil, &value)
		result <- struct {
			state state
			err   error
		}{value, err}
	}()
	request := peer.read(t)
	<-held.wrote
	peer.write(t, `{"id":"`+nativeRPCField(t, request, "id")+`","type":"response","command":"get_state","success":true,"data":{"sessionId":"owned"}}`)
	got := <-result
	if got.err != nil || got.state.SessionID != "owned" {
		t.Fatalf("accepted result = %#v, %v", got.state, got.err)
	}
	if stats := rpc.Stats(); stats.pendingWrites != 1 || stats.retainedBytes == 0 {
		t.Fatalf("held write ownership after accepted response = %#v", stats)
	}
	release()
	waitNativeRPCStats(t, rpc, nativeRPCStats{})
}

func TestNativeRPCCancellationDuringPartialWriteRetiresAndJoins(t *testing.T) {
	rpc, peer := newNativeRPCTest(t, nil, nativeRPCLimits{})
	callCtx, cancel := context.WithCancel(nativeRPCTestContext(t))
	result := make(chan error, 1)
	go func() {
		result <- rpc.Call(callCtx, "prompt", map[string]any{"message": strings.Repeat("x", 900_000)}, nil)
	}()
	read := make(chan struct{})
	go func() {
		buffer := make([]byte, 128)
		_, _ = peer.commands.Read(buffer)
		close(read)
	}()
	<-read
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("partial-write cancellation error = %v", err)
	}
	waitNativeRPCDone(t, rpc)
	waitNativeRPCStats(t, rpc, nativeRPCStats{})
}

func TestNativeRPCAggregateRetainedCapacityReleasesCaller(t *testing.T) {
	limits := defaultNativeRPCLimits
	limits.maxInputFrame = 512
	limits.maxOutputFrame = 512
	limits.maxRetainedBytes = 512
	rpc, peer, held, release := newHeldNativeRPCTest(t, limits)
	held.enabled.Store(true)
	result := make(chan error, 1)
	go func() {
		result <- rpc.Call(nativeRPCTestContext(t), "prompt", map[string]any{"message": strings.Repeat("x", 180)}, nil)
	}()
	request := peer.read(t)
	<-held.wrote
	peer.write(t, `{"id":"`+nativeRPCField(t, request, "id")+`","type":"response","command":"prompt","success":true,"data":{"padding":"`+strings.Repeat("y", 340)+`"}}`)
	if err := <-result; !errors.Is(err, errNativeRPCBusy) {
		t.Fatalf("aggregate capacity error = %v", err)
	}
	release()
	waitNativeRPCDone(t, rpc)
	waitNativeRPCStats(t, rpc, nativeRPCStats{})
}

func TestNativeRPCEventsAreObservedAndObserverFailureRetires(t *testing.T) {
	events := make(chan string, 2)
	rpc, peer := newNativeRPCTest(t, func(raw json.RawMessage) error {
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &event) != nil {
			return errors.New("bad event")
		}
		events <- event.Type
		if event.Type == "extension_ui_request" {
			return errors.New("unattended dialog")
		}
		return nil
	}, nativeRPCLimits{})
	peer.write(t, `{"type":"agent_start"}`)
	if event := <-events; event != "agent_start" {
		t.Fatalf("event = %q", event)
	}
	peer.write(t, `{"type":"extension_ui_request","id":"dialog","method":"confirm"}`)
	waitNativeRPCDone(t, rpc)
	if rpc.Err() == nil || rpc.Err().Error() != "unattended dialog" {
		t.Fatalf("observer failure = %v", rpc.Err())
	}
}

func TestNativeRPCEndInputRequiresSettledCallsAndAcceptsCleanEOF(t *testing.T) {
	rpc, peer := newNativeRPCTest(t, nil, nativeRPCLimits{})
	result := make(chan error, 1)
	go func() { result <- rpc.Call(nativeRPCTestContext(t), "get_state", nil, nil) }()
	request := peer.read(t)
	if err := rpc.EndInput(nativeRPCTestContext(t)); !errors.Is(err, errNativeRPCBusy) {
		t.Fatalf("EndInput with pending call = %v", err)
	}
	peer.write(t, `{"id":"`+nativeRPCField(t, request, "id")+`","type":"response","command":"get_state","success":true,"data":{}}`)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := rpc.EndInput(nativeRPCTestContext(t)); err != nil {
		t.Fatal(err)
	}
	eof := make(chan error, 1)
	go func() {
		_, err := peer.commands.ReadByte()
		eof <- err
	}()
	if err := <-eof; !errors.Is(err, io.EOF) {
		t.Fatalf("native stdin end = %v", err)
	}
	if err := peer.events.Close(); err != nil {
		t.Fatal(err)
	}
	waitNativeRPCDone(t, rpc)
	if err := rpc.Err(); err != nil {
		t.Fatalf("clean RPC shutdown = %v", err)
	}
}

func TestNativeRPCRejectsPartialCRLFAndOversizedOutput(t *testing.T) {
	tests := map[string]struct {
		body   string
		close  bool
		limits nativeRPCLimits
	}{
		"partial EOF":   {body: `{"type":"agent_start"}`, close: true},
		"CRLF":          {body: "{\"type\":\"agent_start\"}\r\n"},
		"trailing JSON": {body: "{\"type\":\"agent_start\"}{}\n"},
		"oversized": {
			body: `{"type":"event","padding":"` + strings.Repeat("x", 300) + "\"}\n",
			limits: nativeRPCLimits{
				maxInputFrame: 256, maxOutputFrame: 256, maxPendingCalls: 1,
				maxPendingWrites: 1, maxRetainedBytes: 256,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rpc, peer := newNativeRPCTest(t, nil, test.limits)
			if _, err := peer.events.Write([]byte(test.body)); err != nil {
				t.Fatal(err)
			}
			if test.close {
				_ = peer.events.Close()
			}
			waitNativeRPCDone(t, rpc)
			if !errors.Is(rpc.Err(), errNativeRPCProtocol) {
				t.Fatalf("RPC error = %v", rpc.Err())
			}
		})
	}
}
