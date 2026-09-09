// SPDX-License-Identifier: MIT

package interactive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicActionsUseActualConnectionAndSchema(t *testing.T) {
	cases := []struct{ action, args, method, result string }{
		{"describe", `{"product":"claude-peer"}`, "lane.describe", `{"product":"claude-peer","supported_open_fields":["cwd","arguments"],"extra_arguments":[]}`},
		{"list", `{}`, "session.list", `{"sessions":[]}`},
		{"send", `{"target":"peer","message":"body"}`, "message.send", `{"message_id":"m","deliveries":[]}`},
		{"send", `{"group":"g","message":"body"}`, "message.send", `{"message_id":"m","deliveries":[]}`},
		{"send", `{"targets":["a","b"],"message":"body"}`, "message.send", `{"message_id":"m","deliveries":[]}`},
		{"spawn", `{"product":"claude-peer","name":"lane","open":{"arguments":["--name","native"]}}`, "lane.spawn", `{"session_id":"lane"}`},
		{"spawn", `{"resume_session_id":"lane"}`, "lane.spawn", `{"session_id":"lane"}`},
		{"run", `{"session_id":"lane","input":"one"}`, "turn.run", `{"outcome":"completed","result":"done"}`},
		{"interrupt", `{"session_id":"lane"}`, "turn.interrupt", `{}`},
		{"close", `{"session_id":"lane"}`, "session.close", `{}`},
		{"forget", `{"session_id":"lane"}`, "session.close", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.action+tc.args, func(t *testing.T) {
			o, wires := testOwner(t)
			w := published(t, o, wires)
			done := make(chan error, 1)
			go func() {
				_, err := CallTool(context.Background(), o, json.RawMessage(`{"action":"`+tc.action+`","arguments":`+tc.args+`}`))
				done <- err
			}()
			f := w.next(t)
			if f.Method != tc.method {
				t.Fatal(f.Method)
			}
			if tc.action == "forget" && !strings.Contains(string(f.Params), `"forget":true`) {
				t.Fatal(string(f.Params))
			}
			w.reply(t, f, json.RawMessage(tc.result))
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInvalidSendAndSpawnNeverReachWire(t *testing.T) {
	o, wires := testOwner(t)
	w := published(t, o, wires)
	for _, raw := range []string{`{"action":"send","arguments":{"to":"x","body":"wrong"}}`, `{"action":"spawn","arguments":{"product":"claude-peer","name":"x"}}`, `{"action":"native_identity_event","arguments":{}}`, `{"action":"list","arguments":{},"extra":true}`} {
		if _, err := CallTool(context.Background(), o, json.RawMessage(raw)); err == nil {
			t.Fatal(raw)
		}
	}
	// Ordered valid request is a barrier: no earlier invalid frame may precede it.
	done := make(chan error, 1)
	go func() { _, err := o.Action(context.Background(), "list", json.RawMessage(`{}`)); done <- err }()
	f := w.next(t)
	if f.Method != "session.list" {
		t.Fatal(f.Method)
	}
	w.reply(t, f, json.RawMessage(`{"sessions":[]}`))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
