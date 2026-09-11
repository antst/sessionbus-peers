// SPDX-License-Identifier: MIT
package opencode

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestNativeMessageSummaryDiscriminatesRole(t *testing.T) {
	for _, tc := range []struct {
		role, summary   string
		valid, internal bool
	}{
		{"assistant", `true`, true, true},
		{"assistant", `false`, true, false},
		{"assistant", `{ "diffs": [] }`, false, false},
		{"assistant", `null`, false, false},
		{"assistant", `"true"`, false, false},
		{"user", `{"diffs":[]}`, true, false},
		{"user", `{"title":"title","body":"body","diffs":[{"file":"a","patch":"patch","additions":1,"deletions":0,"status":"modified"}]}`, true, false},
		{"user", `true`, false, false},
		{"user", `null`, false, false},
		{"user", `{}`, false, false},
		{"user", `{"diffs":null}`, false, false},
		{"user", `{"title":3,"diffs":[]}`, false, false},
		{"user", `{"diffs":[{"additions":"1","deletions":0}]}`, false, false},
		{"user", `{"diffs":[{"additions":1,"deletions":0,"status":"unknown"}]}`, false, false},
	} {
		t.Run(tc.role+"/"+tc.summary, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{"info":{"id":"msg_input","sessionID":"ses_test","parentID":"msg_parent","role":%q,"summary":%s},"parts":[]}`, tc.role, tc.summary))
			got, err := decodeParts(raw, "ses_test")
			if (err == nil) != tc.valid || err == nil && got.Info.Summary != tc.internal {
				t.Fatalf("message=%+v error=%v", got, err)
			}
		})
	}
	// Reusing a decoder destination must not leak the previous assistant marker.
	var info nativeInfo
	if err := json.Unmarshal([]byte(`{"role":"assistant","summary":true}`), &info); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"role":"user","summary":{"diffs":[]}}`), &info); err != nil || info.Summary {
		t.Fatalf("retained internal marker: %+v %v", info, err)
	}
}

func TestNativeSummaryEventDispatchAndValidation(t *testing.T) {
	p := &Wrapper{}
	for _, raw := range []string{
		`{"type":"session.updated","properties":{"sessionID":"ses_test","info":{"id":"ses_test","summary":{"additions":0,"deletions":0,"files":0}}}}`,
		`{"type":"message.updated","properties":{"sessionID":"ses_test","info":{"id":"msg_test","sessionID":"ses_test","role":"user","summary":{"diffs":[]}}}}`,
		`{"type":"unowned.event","properties":{"info":[1,2],"status":"opaque"}}`,
	} {
		if err := p.observe([]byte(raw)); err != nil {
			t.Fatalf("valid event: %v", err)
		}
	}
	for _, raw := range []string{
		`{"type":"session.updated","properties":{"info":{"id":"ses_test","summary":true}}}`,
		`{"type":"session.updated","properties":{"info":{"id":"ses_test","summary":{"additions":0,"deletions":0}}}}`,
		`{"type":"session.updated","properties":{"sessionID":"ses_wrong","info":{"id":"ses_test"}}}`,
		`{"type":"message.updated","properties":{"sessionID":"ses_wrong","info":{"id":"msg_test","sessionID":"ses_test","role":"user"}}}`,
		`{"type":"message.updated","properties":{"info":{"id":"msg_test","sessionID":"ses_test","role":"assistant","summary":{}}}}`,
		`{"type":"message.updated","properties":{"info":{"id":"msg_test","sessionID":"ses_test","role":"unknown"}}}`,
		`{"type":"message.updated","properties":{"info":[]}}`,
		`{"type":"session.updated",`,
	} {
		if err := p.observe([]byte(raw)); err == nil {
			t.Fatalf("invalid event accepted: %s", raw)
		}
	}
}
