// SPDX-License-Identifier: MIT
package qwen

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkerNativeToolUpdatesPreservePromptResult(t *testing.T) {
	// Qwen v0.23.0 transcript-replay.ts emits array-valued content for both
	// tool variants. They are tool UI events, not the lane's answer text.
	for name, update := range map[string]string{
		"tool_call":        `{"sessionUpdate":"tool_call","toolCallId":"native-call","status":"pending","title":"Search","kind":"search","locations":[],"content":[]}`,
		"tool_call_update": `{"sessionUpdate":"tool_call_update","toolCallId":"native-call","status":"completed","content":[{"type":"content","content":{"type":"text","text":"tool result must not enter answer"}}]}`,
		"plan":             `{"sessionUpdate":"plan","entries":[]}`,
		"mode":             `{"sessionUpdate":"current_mode_update","currentModeId":"default"}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newLaneFixture(t, false)
			prompt := f.execute(t, 1, "controlled prompt")
			// One write supplies a valid tool/status event, original answer, and
			// matching terminal. The old parser closes during this write; inspect
			// the actual Worker cursor rather than hiding that behind Write failure.
			wire := `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"` + fixtureID + `","update":` + update + `}}` + "\n" +
				`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"answer"}}}}` + "\n" +
				`{"jsonrpc":"2.0","id":` + string(prompt.ID) + `,"result":{"stopReason":"end_turn"}}` + "\n"
			_, _ = f.native.Write([]byte(wire))
			<-f.ready
			status := f.status(t, 1)
			if status.State != "done" || status.Result == nil || status.Result.Result != "answer" || status.Result.NativeStopReason != "end_turn" {
				body, _ := json.Marshal(status)
				t.Fatalf("native update corrupted original prompt result: %s", body)
			}
		})
	}
}

func TestWorkerMalformedAnswerUpdateFails(t *testing.T) {
	for name, params := range map[string]string{
		"invalid_json":    `{"sessionId":`,
		"session_type":    `{"sessionId":7,"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"wrong"}}}`,
		"missing_session": `{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"wrong"}}}`,
		"missing_kind":    `{"sessionId":"` + fixtureID + `","update":{"content":{"type":"text","text":"wrong"}}}`,
		"array_answer":    `{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_message_chunk","content":[]}}`,
		"null_answer":     `{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_message_chunk","content":null}}`,
		"missing_type":    `{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_message_chunk","content":{"text":"wrong"}}}`,
		"missing_text":    `{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text"}}}`,
		"null_text":       `{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":null}}}`,
		"numeric_text":    `{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":1}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newLaneFixture(t, false)
			f.execute(t, 1, "prompt")
			acpWrite(t, f.native, `{"jsonrpc":"2.0","method":"session/update","params":`+params+`}`)
			<-f.ready
			if status := f.status(t, 1); status.State != "unavailable" {
				t.Fatalf("malformed update accepted: %+v", status)
			}
		})
	}
}

func TestWorkerAnswerUpdatesStayWithinOwnedPromptAndBound(t *testing.T) {
	t.Run("session_and_variant", func(t *testing.T) {
		f := newLaneFixture(t, false)
		prompt := f.execute(t, 1, "prompt")
		for _, params := range []string{
			`{"sessionId":"foreign","update":{"sessionUpdate":"agent_message_chunk","content":[]}}`,
			`{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"thought"}}}`,
			`{"sessionId":"` + fixtureID + `","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"image","data":"AA==","mimeType":"image/png"}}}`,
		} {
			acpWrite(t, f.native, `{"jsonrpc":"2.0","method":"session/update","params":`+params+`}`)
		}
		f.chunk(t, fixtureID, "") // Empty native text is valid.
		f.chunk(t, fixtureID, "answer")
		f.terminal(t, prompt, "end_turn")
		<-f.ready
		if status := f.status(t, 1); status.State != "done" || status.Result.Result != "answer" {
			t.Fatalf("non-answer update entered result: %+v", status)
		}
	})
	t.Run("aggregate_output", func(t *testing.T) {
		f := newLaneFixture(t, false)
		f.execute(t, 1, "prompt")
		// Each native frame is below its bound; accumulated output crosses it.
		for range 3 {
			f.chunk(t, fixtureID, strings.Repeat("a", maxACPFrame/2))
		}
		<-f.ready
		status := f.status(t, 1)
		if status.State != "unavailable" || !strings.Contains(status.Reason, "output exceeds") {
			t.Fatalf("aggregate output bound lost: %+v", status)
		}
	})
}
