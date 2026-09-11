// SPDX-License-Identifier: MIT
package opencode

import (
	"context"
	"encoding/json"
	"testing"
)

// Native .30 User.summary is an object; Assistant.summary is a boolean.
// These fixture maps intentionally avoid nativeInfo's production summary encoding.
func reviewUserSummary(info nativeInfo) map[string]any {
	b, _ := json.Marshal(info)
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	v["summary"] = map[string]any{"diffs": []any{}}
	return v
}
func reviewSummaryHistory(messages []withParts) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		if m.Info.Role == "user" {
			out = append(out, map[string]any{"info": reviewUserSummary(m.Info), "parts": m.Parts})
		} else {
			out = append(out, m)
		}
	}
	return out
}
func TestReviewNativeObjectSummariesThroughWorker(t *testing.T) {
	for _, mode := range []string{"event-user", "event-session", "history"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENCODE_TEST_SUMMARY", mode)
			f := newWorkerFixture(t)
			f.start(t, 1, "summary")
			select {
			case <-f.p.ctx.Done():
				t.Fatalf("valid native summary retired owner: %v", context.Cause(f.p.ctx))
			case <-f.ready:
			case <-f.ctx.Done():
				t.Fatal(f.ctx.Err())
			}
			result := f.wait(t, 1)
			if result.Result == nil || result.Result.Outcome != "completed" || result.Result.Result != "answer:summary" {
				t.Fatalf("valid native summary lost output: %+v", result)
			}
		})
	}
}
