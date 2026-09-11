// SPDX-License-Identifier: MIT
package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
