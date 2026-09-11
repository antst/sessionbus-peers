// SPDX-License-Identifier: MIT
package pi

import (
	"encoding/json"
	"strings"
	"testing"
)

// Shapes follow Pi d981de1 SessionEntryBase/SessionMessageEntry and the
// get_entries response. These are parser tests, not native runtime evidence.
func history(t *testing.T, entries string, leaf string) nativeHistory {
	t.Helper()
	value, err := decodeHistory([]byte(`{"entries":[` + entries + `],"leafId":` + leaf + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func entry(id, parent, role, text, reason string) string {
	value := map[string]any{"id": id, "parentId": parent, "type": "message", "message": map[string]any{
		"role": role, "content": []any{map[string]any{"type": "text", "text": text}}, "stopReason": reason,
	}}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestHistoryDoesNotBorrowAnEarlierAnswer(t *testing.T) {
	old := entry("first", "base", "user", "prompt", "") + "," + entry("reply", "first", "assistant", "prior answer", "stop")
	for _, suffix := range []string{
		"," + entry("next", "reply", "user", "follow up", ""),
		`,` + `{"id":"next","parentId":"reply","type":"message","message":{"role":"assistant","content":[],"stopReason":"aborted"}}`,
	} {
		result, err := historyResult("base", history(t, old+suffix, `"next"`), "prompt")
		if strings.Contains(suffix, "follow up") {
			if err == nil {
				t.Fatal("accepted prior answer after a new user with no assistant")
			}
		} else if err != nil || result.Outcome != "interrupted" || result.Result != "" {
			t.Fatalf("empty native aborted assistant: %+v, %v", result, err)
		}
	}
}

func TestHistoryUsesNativeTerminalAndPreservesText(t *testing.T) {
	for reason, outcome := range map[string]string{"stop": "completed", "aborted": "interrupted", "error": "failed", "length": "failed", "toolUse": "failed"} {
		t.Run(reason, func(t *testing.T) {
			raw := entry("user", "selected", "user", "expanded prompt", "") + "," + entry("answer", "user", "assistant", "  answer\n", reason)
			result, err := historyResult("selected", history(t, raw, `"answer"`), "expanded prompt")
			if err != nil || result.Outcome != outcome || result.NativeStopReason != reason || result.Result != "  answer\n" {
				t.Fatalf("%+v, %v", result, err)
			}
		})
	}
}

func TestHistoryIncludesRetryAndCompactionInCurrentBranch(t *testing.T) {
	raw := entry("user", "selected", "user", "prompt", "") + "," + entry("failed", "user", "assistant", "", "error") +
		`,{"id":"compact","parentId":"failed","type":"compaction","summary":"native summary","firstKeptEntryId":"user"},` +
		entry("answer", "compact", "assistant", "recovered", "stop") +
		`,{"id":"title","parentId":"answer","type":"session_info","name":"title"}`
	result, err := historyResult("selected", history(t, raw, `"title"`), "prompt")
	if err != nil || result.Outcome != "completed" || result.Result != "recovered" {
		t.Fatalf("%+v, %v", result, err)
	}
}

func TestHistoryNativeCustomDeliveryAndToolIntervals(t *testing.T) {
	user := entry("user", "base", "user", "prompt", "")
	tool := user + "," + entry("tool", "user", "assistant", "", "toolUse") +
		`,{"id":"result","parentId":"tool","type":"message","message":{"role":"toolResult","content":[{"type":"text","text":"native result"}]}}`
	for name, row := range map[string]struct{ body, leaf, outcome, text string }{
		"custom before and after": {
			`{"id":"mail","parentId":"base","type":"custom_message","customType":"sessionbus","content":"mail"},` +
				entry("user", "mail", "user", "prompt", "") + "," + entry("answer", "user", "assistant", "answer", "stop") +
				`,{"id":"tail","parentId":"answer","type":"custom_message","customType":"sessionbus","content":"later mail"}`,
			"tail", "completed", "answer",
		},
		"native steering": {
			tool + "," + entry("steer", "result", "user", "different steering input", "") + "," + entry("answer", "steer", "assistant", "final", "stop"),
			"answer", "completed", "final",
		},
		"aborted after tool": {
			tool + "," + entry("answer", "result", "assistant", "", "aborted"), "answer", "interrupted", "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := historyResult("base", history(t, row.body, `"`+row.leaf+`"`), "prompt")
			if err != nil || result.Outcome != row.outcome || result.Result != row.text {
				t.Fatalf("%+v, %v", result, err)
			}
		})
	}
}

func TestHistoryEmptyNativeRoot(t *testing.T) {
	raw := strings.Replace(entry("user", "base", "user", "prompt", ""), `"parentId":"base"`, `"parentId":null`, 1) +
		"," + entry("answer", "user", "assistant", "", "stop")
	result, err := historyResult("", history(t, raw, `"answer"`), "prompt")
	if err != nil || result.Outcome != "completed" || result.Result != "" {
		t.Fatalf("%+v, %v", result, err)
	}
	if _, err := historyResult("", history(t, "", "null"), "prompt"); err == nil {
		t.Fatal("empty interval became an empty success")
	}
}

func TestHistoryRejectsMissingOrForeignOutcome(t *testing.T) {
	user := entry("user", "base", "user", "prompt", "")
	for name, row := range map[string]struct{ entries, leaf, prompt string }{
		"no assistant":      {user, `"user"`, "prompt"},
		"wrong preflight":   {user + "," + entry("answer", "user", "assistant", "answer", "stop"), `"answer"`, "different"},
		"foreign branch":    {user + "," + entry("answer", "other", "assistant", "answer", "stop"), `"answer"`, "prompt"},
		"moved leaf":        {user + "," + entry("answer", "user", "assistant", "answer", "stop"), `"user"`, "prompt"},
		"unknown reason":    {user + "," + entry("answer", "user", "assistant", "answer", "settled"), `"answer"`, "prompt"},
		"pending assistant": {user + "," + entry("answer", "user", "assistant", "answer", "pending"), `"answer"`, "prompt"},
		"deferred provider": {user + "," + entry("answer", "user", "assistant", "answer", "deferred"), `"answer"`, "prompt"},
		"assistant first":   {entry("answer", "base", "assistant", "old", "stop"), `"answer"`, "prompt"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := historyResult("base", history(t, row.entries, row.leaf), row.prompt); err == nil {
				t.Fatal("accepted missing or unrelated native result")
			}
		})
	}
}

func TestHistoryMalformedReferences(t *testing.T) {
	for _, raw := range []string{
		`{"entries":[],"leafId":""}`, `{"entries":[]}`, `{"leafId":null}`,
		`{"entries":[{"id":"x","type":"custom"}],"leafId":"x"}`,
		`{"entries":[{"id":"x","parentId":"x","type":"custom"}],"leafId":"x"}`,
		`{"entries":[{"id":"x","parentId":null,"type":"custom"},{"id":"x","parentId":null,"type":"custom"}],"leafId":"x"}`,
	} {
		if _, err := decodeHistory([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := decodeHistory([]byte(`{"entries":[],"leafId":null}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeHistory(make([]byte, maxHistoryFrame+1)); err == nil {
		t.Fatal("accepted oversized response")
	}
}
