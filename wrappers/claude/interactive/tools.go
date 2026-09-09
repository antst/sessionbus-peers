// SPDX-License-Identifier: MIT

package interactive

import (
	"context"
	"encoding/json"
	"errors"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"slices"
)

const toolDescription = `Call Sessionbus using the current published Claude identity. Written means local write completion only.
Arguments by action (wire validation remains in the public kit):
list: {} or {session_id:string} or {host:string}; session_id and host are exclusive.
send: {message:nonempty string, target:string} or {message, targets:unique nonempty string[]} or {message, group:string, host?:string}; choose exactly one addressing form; host applies only to group. Use returned session IDs or unambiguous names.
describe: {product:string, host?:string}; returns supported open fields and native extra arguments.
spawn fresh: {product:string, name:string, open:object, host?:string, extra_groups?:unique string[], persistent?:boolean, auto_close_ms?:nonnegative integer, idle_message?:"stage"|"run", notify?:boolean, notify_target?:string}. open accepts cwd, permission_mode, model, reasoning_effort (strings), arguments (string[]); use describe to check product support. spawn resume: {resume_session_id:string, persistent?:boolean, auto_close_ms?:nonnegative integer, idle_message?:"stage"|"run", notify?:boolean, notify_target?:string}. Use product "claude-peer" for a Claude lane; use returned session_id, never a shell substitute.
Policies are independent: persistent=false ends the lane when its owner leaves; true survives owner exit. auto_close_ms defaults to 60000 after a terminal; 0 disables. idle_message defaults to stage; run wakes on idle messages. Open itself starts no work or retirement deadline. Resume preserves persistence and omitted idle_message, but omitted auto_close_ms resets to 60000; persistence can be promoted, not demoted. Parent-owned lanes notify their owner unless notify=false; persistent lanes need an explicit notify_target (or one retained on resume/promotion). A completion pointer is an ordinary peer message from the lane, not the answer. It follows the recipient's normal admission policy: an active lane admits it normally; an idle stage lane stages it, while an idle run lane can wake. Interactive wake follows its native carrier; delivery success does not prove collection.
run: {session_id:string, input:nonempty string}; starts and waits, returning session_id/run_id/state and either result (done) or reason (unavailable), without consuming the record.
start: same fields as run; returns {session_id,run_id} for collection by an authorized caller while that worker lives.
status: {session_id:string, run_id?:string}; non-consuming read, omitted run_id selects oldest unacknowledged record.
wait: {session_id:string, run_id?:string, timeout_ms?:nonnegative integer}; non-consuming read or running at the requested bound. Cancellation stops this wait, not the native turn or result retention.
ack: {session_id:string, run_id:string}; acknowledge the oldest terminal record: for state=done, first receive and use its result/outcome/native reason; for state=unavailable, first record/report its reason (there is no result). Both states must be acknowledged to advance the cursor. Never acknowledge running or infer an acknowledgeable record from an RPC error. Repeating an acknowledgment is idempotent, without returning the answer again. No skipping older records; unavailable does not claim a native terminal. Closing, auto-close or worker loss makes unacknowledged output unavailable; collection does not reset the retirement deadline.
interrupt: {session_id:string}; acknowledgment is not terminal completion.
close: {session_id:string, forget?:boolean}; forget: {session_id:string} closes with forget=true.
No unlisted argument fields. Message and input strings have a 262144-character wire limit. Native policy can deny any public call.`

func Tool() any {
	return map[string]any{"name": "sessionbus", "description": toolDescription, "inputSchema": map[string]any{
		"type": "object", "required": []string{"action", "arguments"}, "additionalProperties": false,
		"properties": map[string]any{"action": map[string]any{"type": "string", "enum": kit.Actions}, "arguments": map[string]any{"type": "object"}},
	}}
}

func CallTool(ctx context.Context, owner interface {
	Action(context.Context, string, json.RawMessage) (json.RawMessage, error)
}, raw json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 2 {
		return nil, errors.New("expected a Sessionbus action and arguments object")
	}
	var action string
	var args map[string]json.RawMessage
	if json.Unmarshal(fields["action"], &action) != nil || !slices.Contains(kit.Actions, action) || json.Unmarshal(fields["arguments"], &args) != nil || args == nil {
		return nil, errors.New("expected a Sessionbus action and arguments object")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return owner.Action(ctx, action, fields["arguments"])
}
