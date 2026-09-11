// SPDX-License-Identifier: MIT

package pi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/host"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestPiOwnedRunJoinsNativeWitnessesAndCurrentHistory(t *testing.T) {
	wrapper, cwd := newPiTestWrapper(t, "run")
	if _, err := wrapper.Open(context.Background(), sessionkit.OpenRequest{Name: "managed@local", Open: sessionkit.OpenOptions{Cwd: cwd}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wrapper.Close(context.Background(), sessionkit.SessionCloseRequest{}) })
	turn, err := wrapper.startNativeTurn(context.Background(), nil, "owned prompt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := turn.Wait(context.Background())
	if err != nil || result.Outcome != "completed" || result.Result != "native answer" || result.NativeStopReason != "stop" {
		t.Fatalf("Run result = %+v, %v", result, err)
	}
	if wrapper.active != nil {
		t.Fatal("completed native interval remained active")
	}
	if err = wrapper.Close(context.Background(), sessionkit.SessionCloseRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestPiHandledPromptConsumesSubmittedHandoff(t *testing.T) {
	wrapper, cwd := newPiTestWrapper(t, "handled")
	if _, err := wrapper.Open(context.Background(), sessionkit.OpenRequest{Name: "managed@local", Open: sessionkit.OpenOptions{Cwd: cwd}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wrapper.Close(context.Background(), sessionkit.SessionCloseRequest{}) })
	receipt, err := wrapper.handoff.Deliver(context.Background(), sessionkit.DeliveryRequest{
		MessageID: "queued", Body: "queued once",
		From: sessionkit.DeliverySource{SessionID: "sender@local", Product: "fixture"},
	}, nil)
	if err != nil || receipt.Disposition != "queued_for_next_turn" {
		t.Fatalf("stage delivery = %+v, %v", receipt, err)
	}
	_, err = wrapper.handoff.Run(context.Background(), &sessionkit.Run{}, "/handled", func(ctx context.Context, prompt string) (host.Turn, error) {
		return wrapper.startNativeTurn(ctx, nil, prompt)
	})
	if err == nil || !strings.Contains(err.Error(), "without the managed Run preflight") {
		t.Fatalf("handled prompt terminal = %v", err)
	}
	if queued := wrapper.handoff.Claim(); len(queued) != 0 {
		t.Fatalf("submitted delivery was replayable: %#v", queued)
	}
	if wrapper.active != nil {
		t.Fatal("handled prompt retained an active native interval")
	}
	if closeErr := wrapper.Close(context.Background(), sessionkit.SessionCloseRequest{}); closeErr == nil ||
		!strings.Contains(closeErr.Error(), "without the managed Run preflight") {
		t.Fatalf("handled prompt close = %v", closeErr)
	}
}

func TestPiOwnedInterruptJoinsAbortAndNativeTerminal(t *testing.T) {
	wrapper, cwd := newPiTestWrapper(t, "run-abort")
	if _, err := wrapper.Open(context.Background(), sessionkit.OpenRequest{Name: "managed@local", Open: sessionkit.OpenOptions{Cwd: cwd}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wrapper.Close(context.Background(), sessionkit.SessionCloseRequest{}) })
	turn, err := wrapper.startNativeTurn(context.Background(), nil, "owned prompt")
	if err != nil {
		t.Fatal(err)
	}
	if err = turn.Interrupt(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := turn.Wait(context.Background())
	if err != nil || result.Outcome != "interrupted" || result.Result != "" || result.NativeStopReason != "aborted" {
		t.Fatalf("interrupted result = %+v, %v", result, err)
	}
	if err = wrapper.Close(context.Background(), sessionkit.SessionCloseRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestPiRunAllowsLaterNativeUsersAndFiltersExtensionErrors(t *testing.T) {
	wrapper := &Wrapper{extension: "/managed/extension.mjs"}
	turn := newPiNativeTurn(wrapper, nil, "", "original")
	if err := turn.recordInput("rpc", "original", false); err != nil {
		t.Fatal(err)
	}
	if err := turn.recordPreflight("expanded", false); err != nil {
		t.Fatal(err)
	}
	if err := turn.recordStart(false); err != nil {
		t.Fatal(err)
	}
	if err := turn.recordEvent("agent_start", json.RawMessage(`{"type":"agent_start"}`)); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"expanded", "owned continuation"} {
		raw, _ := json.Marshal(map[string]any{
			"type": "message_start", "message": map[string]any{"role": "user", "content": text},
		})
		if err := turn.recordEvent("message_start", raw); err != nil {
			t.Fatalf("later user %q: %v", text, err)
		}
	}
	if err := turn.recordEvent("extension_error", json.RawMessage(`{"type":"extension_error","extensionPath":"/ambient.mjs","event":"agent_start","error":"ambient failed"}`)); err != nil {
		t.Fatalf("unrelated extension failure became owned failure: %v", err)
	}
	err := turn.recordEvent("extension_error", json.RawMessage(`{"type":"extension_error","extensionPath":"/managed/extension.mjs","event":"agent_start","error":"managed failed"}`))
	if err == nil || !strings.Contains(err.Error(), "managed extension") {
		t.Fatalf("managed extension failure = %v", err)
	}
}

func TestPiRuntimeDialogCancellationUsesOneWayNativeInput(t *testing.T) {
	rpc, peer := newNativeRPCTest(t, nil, nativeRPCLimits{})
	ctx, cancel := context.WithCancelCause(context.Background())
	t.Cleanup(func() { cancel(errors.New("test complete")) })
	wrapper := &Wrapper{ctx: ctx, cancel: cancel, rpc: rpc, opened: true, id: "native", extension: "/managed.mjs"}
	turn := newPiNativeTurn(wrapper, nil, "", "prompt")
	wrapper.active = turn
	if err := wrapper.observeNativeUI(json.RawMessage(`{"type":"extension_ui_request","id":"dialog opaque","method":"confirm","title":"Proceed?","message":"Continue"}`)); err != nil {
		t.Fatal(err)
	}
	request := peer.read(t)
	if len(request) != 3 || nativeRPCField(t, request, "type") != "extension_ui_response" ||
		nativeRPCField(t, request, "id") != "dialog opaque" || string(request["cancelled"]) != "true" {
		t.Fatalf("native UI cancellation = %#v", request)
	}
	ctxWait := nativeRPCTestContext(t)
	for {
		turn.mu.Lock()
		pending, failure := turn.pendingUI, turn.failure
		turn.mu.Unlock()
		if failure != nil {
			t.Fatal(failure)
		}
		if pending == 0 {
			break
		}
		select {
		case <-ctxWait.Done():
			t.Fatal("one-way UI write waited for a response")
		default:
		}
	}
	if stats := rpc.Stats(); stats.pendingCalls != 0 || stats.pendingWrites != 0 || stats.retainedBytes != 0 {
		t.Fatalf("one-way UI cancellation retained work: %+v", stats)
	}
}
