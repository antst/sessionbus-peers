// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"encoding/json"
	"testing"
)

// These native-shaped returns exercise the actual Worker, HTTP child and
// history projection. They deliberately need no question event to break.
func kiloPlanFixture(mode, session, initial string, history []withParts, final *withParts) []withParts {
	tool := func(message, name, status string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"type": "tool", "sessionID": session, "messageID": message, "tool": name, "callID": "call_plan", "state": map[string]any{"status": status, "input": map[string]any{}, "output": "plan", "title": "plan", "time": map[string]int{"start": 1, "end": 2}}})
		return b
	}
	final.Info.Finish = "tool-calls"
	final.Info.Error = nil // An acknowledged abort may return error-free lastAssistant.
	name, status := "plan_exit", "completed"
	switch mode {
	case "plan-pending":
		status = "pending"
	case "plan-error-tool":
		status = "error"
	case "plan-ordinary":
		name = "bash"
	case "plan-earlier", "plan-reset", "plan-parent-mismatch", "plan-no-tool":
		prior := withParts{Info: final.Info}
		prior.Info.ID = "msg_prior_plan"
		prior.Parts = []json.RawMessage{tool(prior.Info.ID, "plan_exit", "completed")}
		history = append(history, prior)
		name = "bash"
		if mode == "plan-reset" || mode == "plan-parent-mismatch" {
			user := withParts{Info: nativeInfo{ID: "msg_native_continuation", SessionID: session, Role: "user"}, Parts: []json.RawMessage{fakePart(session, "msg_native_continuation", "text", "native continuation")}}
			history = append(history, user)
			final.Info.ParentID = user.Info.ID
			if mode == "plan-parent-mismatch" {
				prior.Info.ID = "msg_new_plan"
				prior.Info.ParentID = user.Info.ID
				prior.Parts = []json.RawMessage{tool(prior.Info.ID, "plan_exit", "completed")}
				history = append(history, prior)
				final.Info.ParentID = initial
			}
		}
	}
	if mode != "plan-no-tool" {
		final.Parts = append(final.Parts, tool(final.Info.ID, name, status))
	}
	switch mode {
	case "plan-no-question":
		final.Info.Finish = "stop"
	case "plan-error":
		final.Info.Error = json.RawMessage(`{"name":"ProviderError"}`)
	case "plan-summary":
		final.Info.Summary = true
	case "hold-plan-completed":
		final.Info.Finish = "stop"
		final.Parts = final.Parts[:1] // A genuine ordinary completed stop still wins.
	}
	return history
}

func TestKiloPlanHardStopUsesLatestNativeUserAndOriginalReturn(t *testing.T) {
	for _, tc := range []struct {
		input, outcome, reason string
		interrupt              bool
	}{
		{"plan-no-question", "completed", "stop", false},
		{"plan-earlier", "completed", "tool-calls", false},
		{"plan-reset", "", "", false},
		{"plan-parent-mismatch", "", "", false},
		{"plan-no-tool", "", "", false},
		{"plan-ordinary", "", "", false},
		{"plan-pending", "", "", false},
		{"plan-error-tool", "", "", false},
		{"plan-summary", "", "", false},
		{"plan-error", "failed", "ProviderError", false},
		{"hold-plan", "interrupted", "", true},
		{"hold-plan-completed", "completed", "stop", true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			f := newKiloWorkerFixture(t)
			f.start(t, 1, tc.input)
			if tc.interrupt {
				f.call(t, "turn.interrupt", map[string]any{"session_id": "ses_native@local"}, nil)
			}
			got := f.wait(t, 1)
			if tc.outcome == "" {
				if got.State != "unavailable" || got.Result != nil {
					t.Fatalf("nonterminal became a completion: %+v", got)
				}
				return
			}
			if got.Result == nil || got.Result.Outcome != tc.outcome || got.Result.NativeStopReason != tc.reason {
				t.Fatalf("result=%+v", got)
			}
			raw, err := f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
			var state struct{ Rejections, Aborts int }
			wantAborts := 0
			if tc.interrupt {
				wantAborts = 1
			}
			if err != nil || json.Unmarshal(raw, &state) != nil || state.Rejections != 0 || state.Aborts != wantAborts {
				t.Fatalf("question/abort=%s/%v", raw, err)
			}
			closeKiloFixture(t, f)
		})
	}
}

func TestKiloPlanFollowupUsesCapturedNativeClientMode(t *testing.T) {
	if !supportsKiloPlanFollowup(nil) {
		t.Fatal("unset native client must default to cli")
	}
	for _, mode := range []string{"cli", "vscode", "jetbrains", "", "other"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("KILO_CLIENT", mode)
			f := newKiloWorkerFixture(t)
			supported := mode == "cli" || mode == "vscode" || mode == "jetbrains"
			if f.p.planFollowup != supported {
				t.Fatalf("captured mode=%q support=%v", mode, f.p.planFollowup)
			}
			// Changing the parent's environment cannot change this child's mode.
			t.Setenv("KILO_CLIENT", "changed-after-launch")
			f.start(t, 1, "plan-no-question")
			got := f.wait(t, 1)
			if !supported {
				if got.State != "unavailable" || got.Result != nil {
					t.Fatalf("unsupported native client completed: %+v", got)
				}
				return
			}
			if got.Result == nil || got.Result.Outcome != "completed" {
				t.Fatalf("native plan completion=%+v", got)
			}
			closeKiloFixture(t, f)
		})
	}
}
