// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"strings"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func newKiloWorkerFixture(t *testing.T) *workerFixture {
	t.Helper()
	f := newWorkerKindFixture(t, kiloNative, nil)
	// Assertion failures must not strand a deliberately held native disposal.
	// Successful tests close normally before this fallback is reached.
	t.Cleanup(func() { f.p.stopKiloChild(true) })
	return f
}

func closeKiloFixture(t *testing.T, f *workerFixture) {
	t.Helper()
	f.call(t, "session.close", kit.SessionCloseRequest{SessionID: "ses_native@local"}, nil)
	select {
	case <-f.p.childDone:
	default:
		t.Fatal("close replied before native child was reaped")
	}
	if f.p.childErr != nil {
		t.Fatal(f.p.childErr)
	}
}

func TestKiloLanePolicyAndNativeDefaults(t *testing.T) {
	p := NewKilo("", "unused", "/native/kilo")
	hello, err := p.Hello(context.Background())
	if err != nil || hello.Product != "kilo-peer" || !hello.SupportsMessageRun {
		t.Fatalf("hello=%+v/%v", hello, err)
	}
	for _, mode := range []string{"bypassPermissions", "plan"} {
		if _, _, _, err := launchArgumentsFor(kiloNative, kit.OpenOptions{PermissionMode: mode}); err == nil {
			t.Fatalf("unsupported Kilo policy accepted: %s", mode)
		}
	}
	if _, _, _, err := launchArgumentsFor(openCodeNative, kit.OpenOptions{PermissionMode: "bypassPermissions"}); err != nil {
		t.Fatalf("OpenCode explicit bypass changed: %v", err)
	}
	f := newKiloWorkerFixture(t)
	raw, err := f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
	if err != nil {
		t.Fatal(err)
	}
	var state struct{ Permission any }
	if json.Unmarshal(raw, &state) != nil || state.Permission != nil {
		t.Fatalf("native default policy replaced: %s", raw)
	}
	closeKiloFixture(t, f)
}

func TestKiloActiveStageDoesNotWriteUntilNextOwnedRun(t *testing.T) {
	f := newKiloWorkerFixture(t)
	f.start(t, 1, "hold")
	if _, err := f.p.client.call(f.ctx, "GET", "/fixture/started", nil, 200); err != nil {
		t.Fatal(err)
	}
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", fixtureDelivery(), &receipt)
	if receipt.Disposition != "queued_for_next_turn" {
		t.Fatal(receipt)
	}
	raw, err := f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Messages []withParts
		Aborts   int
	}
	if json.Unmarshal(raw, &state) != nil || len(state.Messages) != 1 || strings.Contains(string(raw), "marker") {
		t.Fatalf("active stage changed native input: %s", raw)
	}
	f.call(t, "turn.interrupt", map[string]any{"session_id": "ses_native@local"}, nil)
	result := f.wait(t, 1)
	if result.Result == nil || result.Result.Outcome != "interrupted" {
		t.Fatalf("interrupt=%+v", result)
	}
	f.start(t, 2, "healthy")
	next := f.wait(t, 2)
	if next.Result == nil || next.Result.Outcome != "completed" || strings.Count(next.Result.Result, "marker") != 1 || !strings.Contains(next.Result.Result, "healthy") {
		t.Fatalf("next Run lost/duplicated staged input: %+v", next)
	}
	f.start(t, 3, "last")
	if next := f.wait(t, 3); next.Result == nil || next.Result.Result != "answer:last" {
		t.Fatalf("stage replayed: %+v", next)
	}
	raw, err = f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
	if err != nil || json.Unmarshal(raw, &state) != nil || state.Aborts != 1 {
		t.Fatalf("abort ownership=%s/%v", raw, err)
	}
	closeKiloFixture(t, f)
}

func TestKiloIdleStageSeedWrittenAndRepeatedCollection(t *testing.T) {
	f := newKiloWorkerFixture(t)
	d := fixtureDelivery()
	d.Body = "idle-staged"
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", d, &receipt)
	if receipt.Disposition != "queued_for_next_turn" {
		t.Fatal(receipt)
	}
	d.Body, d.MessageID, d.RunID = "seeded", "seed", "g/1"
	f.call(t, "message.deliver", d, &receipt)
	if receipt.Disposition != "written" {
		t.Fatal(receipt)
	}
	r := f.wait(t, 1)
	if r.Result == nil || r.Result.Outcome != "completed" || strings.Count(r.Result.Result, "idle-staged") != 1 || strings.Count(r.Result.Result, "seeded") != 1 {
		t.Fatalf("seed=%+v", r)
	}
	var status kit.RunStatus
	f.call(t, "turn.status", kit.ReadRequest{SessionID: "ses_native@local", RunID: "g/1"}, &status)
	if status.Result == nil || status.Result.Result != r.Result.Result {
		t.Fatal("collection consumed output")
	}
	f.call(t, "turn.ack", kit.RunRef{SessionID: "ses_native@local", RunID: "g/1"}, nil)
	f.start(t, 2, "next")
	if r := f.wait(t, 2); r.Result == nil || r.Result.Result != "answer:next" {
		t.Fatalf("seed replayed=%+v", r)
	}
	closeKiloFixture(t, f)
}

func TestKiloRejectedMessageIDPreservesStageBeforeSubmission(t *testing.T) {
	f := newKiloWorkerFixture(t)
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", fixtureDelivery(), &receipt)
	if receipt.Disposition != "queued_for_next_turn" {
		t.Fatal(receipt)
	}
	f.p.messageIDs = messageIDs{now: func() int64 { return 99 }, lastTime: 100, lastPrefix: 100*4096 + 1, counter: 1, initialized: true}
	f.start(t, 1, "rejected")
	if r := f.wait(t, 1); r.Result != nil {
		t.Fatalf("clock rollback reached native: %+v", r)
	}
	raw, err := f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
	var state struct{ Messages []withParts }
	if err != nil || json.Unmarshal(raw, &state) != nil || len(state.Messages) != 0 {
		t.Fatalf("preflight submitted: %s/%v", raw, err)
	}
	f.p.messageIDs.now = func() int64 { return 101 }
	f.start(t, 2, "accepted")
	if r := f.wait(t, 2); r.Result == nil || strings.Count(r.Result.Result, "marker") != 1 {
		t.Fatalf("stage lost: %+v", r)
	}
	closeKiloFixture(t, f)
}

func TestKiloNativeUnknownAndInterruptedIntermediateTerminals(t *testing.T) {
	for _, tc := range []struct{ input, outcome, reason string }{
		{"hold-terminal-unknown", "completed", "unknown"},
		{"hold-terminal-stop", "completed", "stop"},
		{"hold-terminal-ordinary", "interrupted", ""},
		{"hold-toolcalls", "interrupted", ""},
		{"hold-summary", "interrupted", ""},
		{"hold", "interrupted", "MessageAbortedError"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			f := newKiloWorkerFixture(t)
			f.start(t, 1, tc.input)
			f.call(t, "turn.interrupt", map[string]any{"session_id": "ses_native@local"}, nil)
			r := f.wait(t, 1)
			if r.Result == nil || r.Result.Outcome != tc.outcome || r.Result.NativeStopReason != tc.reason {
				t.Fatalf("native terminal=%+v", r)
			}
			closeKiloFixture(t, f)
		})
	}
}

func TestKiloCloseJoinsNativeDisposalAndOnlyCallerCancellationForces(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "native cleanup", true: "caller cancellation"}[force], func(t *testing.T) {
			t.Setenv("KILO_TEST_HOLD_CLOSE", "1")
			f := newKiloWorkerFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- f.p.Close(ctx, kit.SessionCloseRequest{}) }()
			// Released by the native fixture's actual SIGTERM handler.
			if _, err := f.p.client.call(f.ctx, "GET", "/fixture/term", nil, 200); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				t.Fatalf("Close skipped held native disposal: %v", err)
			default:
			}
			select {
			case <-f.worker.Closed():
				t.Fatal("intentional teardown shut Worker before Close settled")
			default:
			}
			if force {
				cancel()
			} else {
				if _, err := f.p.client.call(f.ctx, "POST", "/fixture/close-release", nil, 200); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if force {
					var child *exec.ExitError
					if !errors.Is(err, context.Canceled) || !errors.As(err, &child) {
						t.Fatalf("forced cleanup lost causes: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			case <-f.ctx.Done():
				t.Fatal("Close failed to join cleanup")
			}
			select {
			case <-f.p.childDone:
			default:
				t.Fatal("Close before reaping")
			}
		})
	}
}

func TestKiloNativeToolChildCapabilityAndMetadataSeparation(t *testing.T) {
	f := newKiloWorkerFixture(t)
	c, err := net.Dial("unix", f.p.endpoint.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	reader := bufio.NewReader(c)
	send := func(id int, method string, params any) map[string]json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if _, err := c.Write(append(raw, '\n')); err != nil {
			t.Fatal(err)
		}
		raw, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var response map[string]json.RawMessage
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	send(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "native-kilo", "version": "1"}})
	for i, tc := range []struct {
		key, id string
		allowed bool
	}{
		{"sessionbus.kilo", "ses_child", true},
		{"sessionbus.kilo", "ses_other", false},
		{"sessionbus.kilo", "", false},
		{"sessionbus.opencode", "ses_child", false},
	} {
		reply := send(i+2, "tools/call", map[string]any{"name": "sessionbus", "arguments": map[string]any{"action": "list", "arguments": map[string]any{}}, "_meta": map[string]any{tc.key: map[string]string{"session_id": tc.id, "message_id": "msg_native_tool"}}})
		failed := len(reply["error"]) > 0 || strings.Contains(string(reply["result"]), "\"isError\":true")
		if failed == tc.allowed {
			t.Fatalf("native identity %s/%s result=%s", tc.key, tc.id, reply)
		}
	}
	closeKiloFixture(t, f)
}

func TestKiloUnattendedQuestionsAndPermissionsRejectWithoutPolicyChange(t *testing.T) {
	for _, input := range []string{"hold-permission", "hold-question"} {
		t.Run(input, func(t *testing.T) {
			f := newKiloWorkerFixture(t)
			f.start(t, 1, input)
			if r := f.wait(t, 1); r.Result == nil || r.Result.Outcome != "completed" {
				t.Fatalf("native rejection did not settle prompt: %+v", r)
			}
			raw, err := f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
			var state struct {
				Rejections int
				Permission any
			}
			if err != nil || json.Unmarshal(raw, &state) != nil || state.Rejections != 1 || state.Permission != nil {
				t.Fatalf("permission state=%s/%v", raw, err)
			}
			closeKiloFixture(t, f)
		})
	}
}

func TestKiloUnexpectedEventLossRetiresAndJoinsNativeRun(t *testing.T) {
	f := newKiloWorkerFixture(t)
	f.start(t, 1, "hold")
	if _, err := f.p.client.call(f.ctx, "GET", "/fixture/started", nil, 200); err != nil {
		t.Fatal(err)
	}
	_, _ = f.p.client.call(f.ctx, "POST", "/fixture/end-events", nil, 200)
	select {
	case <-f.worker.Closed():
	case <-f.ctx.Done():
		t.Fatal("unexpected loss did not retire Worker")
	}
	select {
	case <-f.p.childDone:
	default:
		t.Fatal("Worker closed before direct child reaping")
	}
	if f.p.childErr != nil {
		t.Fatalf("unexpected owner loss bypassed native TERM cleanup: %v", f.p.childErr)
	}
}

func TestKiloStageEncodedBoundAndOversizedRunPreserveAcceptedPrefix(t *testing.T) {
	f := newKiloWorkerFixture(t)
	d := fixtureDelivery()
	d.Body = strings.Repeat("<", 100000)
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", d, &receipt)
	if receipt.Disposition != "queued_for_next_turn" {
		t.Fatal(receipt)
	}
	d.MessageID = "second"
	f.call(t, "message.deliver", d, &receipt)
	if receipt.Disposition != "rejected" {
		t.Fatalf("irreversibly oversized stage accepted: %+v", receipt)
	}
	f.start(t, 1, strings.Repeat("<", 100000))
	if r := f.wait(t, 1); r.Result != nil {
		t.Fatalf("oversized combined request submitted: %+v", r)
	}
	f.start(t, 2, "x")
	if r := f.wait(t, 2); r.Result == nil || r.Result.Outcome != "completed" || strings.Count(r.Result.Result, d.Body) != 1 {
		t.Fatal("accepted prefix lost or replayed")
	}
	closeKiloFixture(t, f)
}

func TestKiloResumeAndRequestedForgetRemainNativeOperations(t *testing.T) {
	for _, forget := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserve history", true: "forget while live"}[forget], func(t *testing.T) {
			expected := "0"
			if forget {
				expected = "1"
			}
			t.Setenv("KILO_TEST_EXPECT_DELETE", expected)
			f := newWorkerRequestFixture(t, kiloNative, nil, "ses_native")
			t.Cleanup(func() { f.p.stopKiloChild(true) })
			raw, err := f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
			var state struct{ Creates, Loads, Patches, Deletes int }
			if err != nil || json.Unmarshal(raw, &state) != nil || state.Creates != 0 || state.Loads != 1 || state.Patches != 1 || state.Deletes != 0 {
				t.Fatalf("native resume path=%s/%v", raw, err)
			}
			d := fixtureDelivery()
			d.RunID = "g/1"
			var receipt kit.DeliveryReceipt
			f.call(t, "message.deliver", d, &receipt)
			if receipt.Disposition != "written" {
				t.Fatal(receipt)
			}
			if r := f.wait(t, 1); r.Result == nil || r.Result.Outcome != "completed" {
				t.Fatalf("resumed seed=%+v", r)
			}
			f.call(t, "session.close", kit.SessionCloseRequest{SessionID: "ses_native@local", Forget: forget}, nil)
			if f.p.childErr != nil {
				t.Fatalf("native deletion/termination ordering=%v", f.p.childErr)
			}
		})
	}
}
