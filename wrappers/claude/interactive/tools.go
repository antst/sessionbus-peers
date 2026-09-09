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
spawn fresh: {product:string, name:string, open:object, host?:string, extra_groups?:unique string[]}. open accepts cwd, permission_mode, model, reasoning_effort (strings), arguments (string[]); use describe to check product support. spawn resume: {resume_session_id:string} alone. This candidate cannot provide Claude lanes.
run: {session_id:string, input:nonempty string}; waits for the terminal result.
start: same fields as run; returns a local turn_id for collection.
status: {turn_id:string}; running is non-consuming; a completed/unavailable result is consumed once.
wait: {turn_id:string, timeout_ms?:nonnegative integer}; collects the result or returns running at the explicit bound. Cancelling a pending wait leaves its run/result collectible; it does not interrupt the remote turn. Handles belong to this MCP owner, not daemon session IDs.
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
