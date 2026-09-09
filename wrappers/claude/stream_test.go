// SPDX-License-Identifier: MIT
package claude

import (
	"context"
	"encoding/json"
	"errors"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"io"
	"os"
	"os/exec"
	"testing"
)

type nativeFixture struct {
	s      *stream
	input  *json.Decoder
	output *json.Encoder
}

func streamFixture(t *testing.T) nativeFixture {
	t.Helper()
	incoming, writer := io.Pipe()
	reader, outgoing := io.Pipe()
	s := newStream(writer, reader, nil)
	t.Cleanup(func() { s.stop(io.EOF); _ = incoming.Close(); _ = outgoing.Close() })
	return nativeFixture{s, json.NewDecoder(incoming), json.NewEncoder(outgoing)}
}
func (f nativeFixture) next(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var value map[string]json.RawMessage
	if err := f.input.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
func (f nativeFixture) send(t *testing.T, v any) {
	t.Helper()
	if err := f.output.Encode(v); err != nil {
		t.Fatal(err)
	}
}
func rawString(t *testing.T, v json.RawMessage) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// A correlated control response is a deterministic read-loop barrier, without sleeps.
func (f nativeFixture) barrier(t *testing.T) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := f.s.control(context.Background(), map[string]string{"subtype": "mcp_status"})
		done <- err
	}()
	request := f.next(t)
	f.send(t, map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": rawString(t, request["request_id"]), "response": map[string]any{}}})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestRunRequiresMatchingReplayAndTerminal(t *testing.T) {
	for _, terminalFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "replay-first", true: "terminal-first"}[terminalFirst], func(t *testing.T) {
			f := streamFixture(t)
			done := make(chan runResult, 1)
			admitted := make(chan struct{}, 1)
			go func() {
				v, e := f.s.run(context.Background(), "native-id", "exact input", func() { admitted <- struct{}{} })
				done <- runResult{v, e}
			}()
			request := f.next(t)
			id := rawString(t, request["uuid"])
			if string(request["shouldQuery"]) != "true" || rawString(t, request["session_id"]) != "native-id" {
				t.Fatal("incorrect native frame")
			}
			replay := map[string]any{"type": "user", "session_id": "native-id", "uuid": id, "isReplay": true}
			terminal := map[string]any{"type": "result", "session_id": "native-id", "subtype": "success", "result": "exact result", "user_message_uuids": []string{id}}
			f.send(t, map[string]any{"type": "result", "session_id": "native-id", "subtype": "success", "num_turns": 0})
			f.send(t, map[string]any{"type": "user", "session_id": "other-id", "uuid": id, "isReplay": true})
			if terminalFirst {
				f.send(t, terminal)
			} else {
				f.send(t, replay)
			}
			f.barrier(t)
			select {
			case <-done:
				t.Fatal("uncorrelated/incomplete turn completed")
			default:
			}
			if terminalFirst {
				f.send(t, replay)
			} else {
				f.send(t, terminal)
			}
			got := <-done
			if got.err != nil || got.value.Result != "exact result" || got.value.Outcome != "completed" {
				t.Fatalf("%+v", got)
			}
			<-admitted
		})
	}
}
func TestAppendWaitsForNativeReplay(t *testing.T) {
	f := streamFixture(t)
	done := make(chan string, 1)
	go func() {
		receipt, err := f.s.append(context.Background(), "native-id", "context only")
		if err != nil {
			done <- err.Error()
		} else {
			done <- receipt.Disposition
		}
	}()
	request := f.next(t)
	id := rawString(t, request["uuid"])
	if string(request["shouldQuery"]) != "false" {
		t.Fatal("delivery started work")
	}
	f.send(t, map[string]any{"type": "user", "session_id": "wrong", "uuid": id, "isReplay": true})
	f.barrier(t)
	select {
	case v := <-done:
		t.Fatalf("write/wrong-id credited: %s", v)
	default:
	}
	f.send(t, map[string]any{"type": "user", "session_id": "native-id", "uuid": id, "isReplay": true})
	if got := <-done; got != "queued_for_next_turn" {
		t.Fatal(got)
	}
}
func TestAttemptedAppendWithoutAdmissionIsUncertain(t *testing.T) {
	f := streamFixture(t)
	done := make(chan error, 1)
	go func() { _, err := f.s.append(context.Background(), "native-id", "context"); done <- err }()
	f.next(t)
	f.s.stop(io.EOF)
	err := <-done
	var protocol *kit.ProtocolError
	if !errors.As(err, &protocol) || protocol.Code != -32603 || string(protocol.Data) != `"uncertain_native_admission"` {
		t.Fatalf("missing uncertain failure: %#v", err)
	}
}
func TestNativeErrorAndInterruptReasons(t *testing.T) {
	for _, tc := range []struct {
		reason, stop, outcome string
		isError               bool
	}{
		{"api_error", "refusal", "failed", true}, {"aborted_tools", "tool_use", "interrupted", true}, {"aborted_streaming", "", "interrupted", true},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			f := streamFixture(t)
			done := make(chan runResult, 1)
			go func() { v, e := f.s.run(context.Background(), "id", "input", func() {}); done <- runResult{v, e} }()
			id := rawString(t, f.next(t)["uuid"])
			f.send(t, map[string]any{"type": "user", "session_id": "id", "uuid": id, "isReplay": true})
			f.send(t, map[string]any{"type": "result", "session_id": "id", "subtype": "success", "is_error": tc.isError, "terminal_reason": tc.reason, "stop_reason": tc.stop, "user_message_uuid": id})
			got := <-done
			if got.err != nil || got.value.Outcome != tc.outcome || got.value.NativeStopReason != func() string {
				if tc.stop == "" {
					return tc.reason
				}
				return tc.reason + "; " + tc.stop
			}() {
				t.Fatalf("%+v", got)
			}
		})
	}
}
func TestCancellationClosesBlockedNativeWrite(t *testing.T) {
	incoming, writer := io.Pipe()
	reader, outgoing := io.Pipe()
	defer incoming.Close()
	defer outgoing.Close()
	s := newStream(writer, reader, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := s.control(ctx, map[string]string{"subtype": "initialize"}); done <- err }()
	// Reading one byte proves entry into the pipe write while leaving it blocked.
	var b [1]byte
	if _, err := incoming.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-s.done
}

type gatedNativeReader struct {
	io.ReadCloser
	release <-chan struct{}
}

func (r gatedNativeReader) Read(p []byte) (int, error) { <-r.release; return r.ReadCloser.Read(p) }
func TestProcessExitBeforeReaderPreservesBufferedTerminal(t *testing.T) {
	input, write := io.Pipe()
	output, childOutput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	s := newStream(write, gatedNativeReader{output, release}, nil)
	t.Cleanup(func() { s.stop(io.EOF); _ = input.Close(); _ = childOutput.Close() })
	done := make(chan runResult, 1)
	go func() { v, e := s.run(context.Background(), "id", "input", func() {}); done <- runResult{v, e} }()
	var user map[string]json.RawMessage
	if err = json.NewDecoder(input).Decode(&user); err != nil {
		t.Fatal(err)
	}
	id := rawString(t, user["uuid"])
	replay, _ := json.Marshal(map[string]any{"type": "user", "session_id": "id", "uuid": id, "isReplay": true})
	terminal, _ := json.Marshal(map[string]any{"type": "result", "session_id": "id", "user_message_uuid": id, "subtype": "success", "result": "buffered terminal"})
	// A controlled process writes the native-shaped frames. No product runs here.
	child := exec.Command("sh", "-c", `printf '%s\n' "$1" "$2"`, "fixture", string(replay), string(terminal))
	child.Stdout = childOutput
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	_ = childOutput.Close()
	exited := make(chan struct{})
	go waitNative(child, exited)
	<-exited
	close(release)
	got := <-done
	if got.err != nil || got.value.Result != "buffered terminal" {
		t.Fatalf("%+v", got)
	}
}

func TestCanceledInterruptResponseWaitKeepsLaneTransport(t *testing.T) {
	f := streamFixture(t)
	p := New(t.TempDir())
	p.opened = true
	p.identity = "id"
	p.stream = f.s
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Interrupt(ctx, nil) }()
	request := f.next(t)
	// Later complete control exchange proves the first serialized write returned.
	f.barrier(t)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	p.mu.Lock()
	failure := p.failure
	p.mu.Unlock()
	if failure != nil {
		t.Fatalf("request cancellation retired lane: %v", failure)
	}
	f.send(t, map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": rawString(t, request["request_id"]), "response": map[string]any{}}})
	f.barrier(t)
}
