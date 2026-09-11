// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"encoding/json"
	"testing"
)

// The native Kilo plan-followup branch breaks normally after its question is
// rejected, returning the completed plan_exit assistant even with tool-calls.
func TestReviewKiloPlanFollowupReturnIsNotLost(t *testing.T) {
	for _, finish := range []string{"stop", "tool-calls"} {
		t.Run(finish, func(t *testing.T) {
			f := newKiloWorkerFixture(t)
			f.start(t, 1, "review-plan-"+finish)
			got := f.wait(t, 1)
			raw, err := f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
			var state struct {
				Rejections int
				Aborts     int
			}
			if err != nil || json.Unmarshal(raw, &state) != nil || state.Rejections != 1 || state.Aborts != 0 {
				t.Fatalf("native plan question/abort=%s/%v", raw, err)
			}
			t.Logf("actual Worker result: %+v", got)
			if got.Result == nil || got.Result.Outcome != "completed" || got.Result.NativeStopReason != finish {
				t.Fatalf("native normal plan-exit response lost: %+v", got)
			}
			closeKiloFixture(t, f)
		})
	}
}
