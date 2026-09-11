// SPDX-License-Identifier: MIT
package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func historyMessage(id, role, parent, text string, summary bool) withParts {
	m := withParts{Info: nativeInfo{ID: id, Role: role, SessionID: "ses_test", ParentID: parent, Summary: summary}, Parts: []json.RawMessage{fakePart("ses_test", id, "text", text)}}
	if role == "assistant" {
		m.Info.Finish = "stop"
		n := float64(1)
		m.Info.Time.Completed = &n
	}
	return m
}
func TestLegacyHistoryCompactionAndPagingStopsAtNativeFinal(t *testing.T) {
	user := historyMessage("msg_z_initial", "user", "", "input", false)
	first := historyMessage("msg_a_first", "assistant", user.Info.ID, "first", false)
	summary := historyMessage("msg_summary", "assistant", user.Info.ID, "internal", true)
	replay := historyMessage("msg_replay", "user", "", "native replay", false)
	final := historyMessage("msg_final", "assistant", replay.Info.ID, "final", false)
	later := historyMessage("msg_later", "assistant", replay.Info.ID, "not-owned", false)
	pages := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		switch r.URL.Query().Get("before") {
		case "":
			w.Header().Set("X-Next-Cursor", "older")
			_ = json.NewEncoder(w).Encode([]withParts{replay, final, later})
		case "older":
			_ = json.NewEncoder(w).Encode([]withParts{user, first, summary})
		default:
			t.Error("invented cursor")
		}
	}))
	defer s.Close()
	c := newLaneHTTP(s.URL, "/work", "u", "p")
	out, err := c.historyResult(context.Background(), "ses_test", user.Info.ID, final)
	if err != nil || out != "firstfinal" || pages != 2 {
		t.Fatalf("projection=%q/%v pages=%d", out, err, pages)
	}
}
func TestLegacyHistoryRejectsPriorMissingRepeatedAndOversized(t *testing.T) {
	user := historyMessage("msg_input", "user", "", "input", false)
	final := historyMessage("msg_final", "assistant", user.Info.ID, "answer", false)
	for _, kind := range []string{"prior", "missing", "repeat", "oversize", "bad-part"} {
		t.Run(kind, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := []withParts{user, final}
				switch kind {
				case "prior":
					page = []withParts{final, user}
				case "missing":
					page = []withParts{user}
				case "repeat":
					w.Header().Set("X-Next-Cursor", "same")
					page = []withParts{}
				case "oversize":
					page[1] = historyMessage("msg_final", "assistant", user.Info.ID, strings.Repeat("x", maxNativeRequest+1), false)
				case "bad-part":
					page[1].Parts = []json.RawMessage{json.RawMessage(`{"type":"text","text":3}`)}
				}
				_ = json.NewEncoder(w).Encode(page)
			}))
			defer s.Close()
			c := newLaneHTTP(s.URL, "/work", "u", "p")
			if _, err := c.historyResult(context.Background(), "ses_test", user.Info.ID, final); err == nil {
				t.Fatal("invalid interval accepted")
			}
		})
	}
}
