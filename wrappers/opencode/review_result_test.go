// SPDX-License-Identifier: MIT
package opencode

import (
	"context"
	"encoding/json"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReviewHistoryRequiresNativeUserParentInOwnedRange(t *testing.T) {
	for _, parent := range []string{"msg_absent", "msg_intermediate_assistant"} {
		t.Run(parent, func(t *testing.T) {
			initial := historyMessage("msg_initial", "user", "", "input", false)
			intermediate := historyMessage("msg_intermediate_assistant", "assistant", initial.Info.ID, "first", false)
			final := historyMessage("msg_final", "assistant", parent, "final", false)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode([]withParts{initial, intermediate, final})
			}))
			defer s.Close()
			c := newLaneHTTP(s.URL, "/work", "u", "p")
			out, err := c.historyResult(context.Background(), "ses_test", initial.Info.ID, final)
			if err == nil {
				t.Fatalf("invalid native user ancestry accepted: parent=%s output=%q", parent, out)
			}
		})
	}
}

func TestReviewCancelledRunDoesNotCompleteAtToolCallStep(t *testing.T) {
	f := newWorkerFixture(t)
	f.start(t, 1, "hold-toolcalls")
	f.call(t, "turn.interrupt", map[string]any{"session_id": "ses_native@local"}, nil)
	r := f.wait(t, 1)
	if r.Result == nil || r.Result.Outcome != "interrupted" {
		t.Fatalf("cancelled execution promoted tool-call step to completion: %+v", r.Result)
	}
}

func TestLegacyCancelUsesNativeCompletedUserStopPredicate(t *testing.T) {
	for _, tc := range []struct{ variant, outcome, reason string }{
		{"ordinary", "interrupted", ""}, {"unknown", "interrupted", ""},
		{"provider", "completed", "stop"}, {"orphan", "completed", "stop"}, {"stop", "completed", "stop"},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			f := newWorkerFixture(t)
			f.start(t, 1, "hold-terminal-"+tc.variant)
			f.call(t, "turn.interrupt", map[string]any{"session_id": "ses_native@local"}, nil)
			r := f.wait(t, 1)
			if r.Result == nil || r.Result.Outcome != tc.outcome || r.Result.NativeStopReason != tc.reason {
				t.Fatalf("native final classification: %+v", r.Result)
			}
		})
	}
}

func TestReviewStageCannotAcceptAnUnsubmittablePrefix(t *testing.T) {
	f := newWorkerFixture(t)
	d := fixtureDelivery()
	d.Body = strings.Repeat("<", 100000)
	d.MessageID = "first"
	var first kit.DeliveryReceipt
	f.call(t, "message.deliver", d, &first)
	if first.Disposition != "queued_for_next_turn" {
		t.Fatal(first)
	}
	d.MessageID = "second"
	var second kit.DeliveryReceipt
	f.call(t, "message.deliver", d, &second)
	if second.Disposition == "rejected" {
		return
	}
	if second.Disposition != "queued_for_next_turn" {
		t.Fatal(second)
	}
	f.start(t, 1, "x")
	r := f.wait(t, 1)
	if r.Result == nil {
		t.Fatalf("accepted staged prefix cannot fit even minimal explicit input: %+v", r)
	}
}

func TestLegacyStageOverflowRejectsNewPrefixAndKeepsUnsentFIFO(t *testing.T) {
	f := newWorkerFixture(t)
	d := fixtureDelivery()
	d.Body = strings.Repeat("<", 100000)
	d.MessageID = "first"
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", d, &receipt)
	if receipt.Disposition != "queued_for_next_turn" {
		t.Fatal(receipt)
	}
	d.MessageID = "second"
	f.call(t, "message.deliver", d, &receipt)
	if receipt.Disposition != "rejected" {
		t.Fatal(receipt)
	}
	// The accepted prefix still fits with a minimal explicit input.
	f.start(t, 1, "x")
	r := f.wait(t, 1)
	if r.Result == nil || r.Result.Outcome != "completed" || strings.Count(r.Result.Result, strings.Repeat("<", 100000)) != 1 {
		t.Fatalf("retained FIFO not consumed once: state=%s", r.State)
	}
}
func TestLegacyOversizedCombinedInputKeepsStageForSmallerRun(t *testing.T) {
	f := newWorkerFixture(t)
	d := fixtureDelivery()
	d.Body = strings.Repeat("<", 100000)
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", d, &receipt)
	if receipt.Disposition != "queued_for_next_turn" {
		t.Fatal(receipt)
	}
	// This bus request fits, but native JSON escaping exceeds the native cap.
	f.start(t, 1, strings.Repeat("<", 100000))
	r := f.wait(t, 1)
	if r.Result != nil {
		t.Fatal("oversized native request unexpectedly submitted")
	}
	f.start(t, 2, "x")
	r = f.wait(t, 2)
	if r.Result == nil || r.Result.Outcome != "completed" || strings.Count(r.Result.Result, d.Body) != 1 {
		t.Fatalf("stage lost after preflight failure: %+v", r)
	}
}
