# Qwen Sessionbus package

One Go binary supplies `qwen-peer` and its private sibling alias
`qwen-peer-mcp`. The native Qwen installation supplies its own Node runtime;
this package adds no Node adapter or npm dependencies. Use the archive so the
lane worker can resolve its exact private sibling executable.

```sh
scripts/package-product qwen ./dist
mkdir -p ./dist/qwen-install
tar -xzf ./dist/qwen-peer-linux-amd64.tar.gz -C ./dist/qwen-install
sh ./dist/qwen-install/install
```

Select the archive for the target platform. The literal installer replaces
`~/.local/libexec/sessionbus/qwen`, links `~/.local/bin/qwen-peer`, and uses
native `qwen extensions uninstall/install` for the owned `sessionbus`
extension. Reinstall replaces the owned plugin payload, including removed
skills. It preserves unrelated extensions and native history. The archive
contains one generic `skills/sessionbus/SKILL.md`, covering every product
through the same public tool; there are no separate product lane skills.

## Current lane interface

The daemon starts the token-selected worker with no model input. It owns one
native `qwen --acp` session and one shared Worker/Caller. Fresh Open adopts the
native `session/new` ID. Resume uses the retained native ID and rejects a
conflicting returned ID. The per-session MCP configuration uses the absolute
private sibling executable and a unique endpoint for that native session.
The helper forwards the single public tool to that existing Caller.

Call `mcp__sessionbus__sessionbus` with `{action, arguments}`. Qwen may defer
the tool behind native `tool_search`; use `select:mcp__sessionbus__sessionbus`
to discover it. Successful helper initialization proves helper startup only,
not completed native tool discovery or permission to call a tool. Preserve
native refusals; do not force a grant or change deferral to obtain success.

Use `describe` with `product:"qwen-peer"` for supported Open fields, and
`spawn` with a product, child name and explicit `open` object. The current
fields are cwd, permission_mode, model, reasoning_effort and arguments.
Omitted/default permission uses native policy; bypassPermissions is an
explicit caller choice. Model and supported effort values are passed through
their native configuration paths. Unsupported values and arguments conflicting
with owned lane controls are rejected. No default permission grant is added.

`start` returns session_id/run_id; `run`, `status` and `wait` read without
consuming. Receive a done result or report an unavailable reason before `ack`;
never acknowledge running. Interrupt acknowledgment is not a terminal result.
The one shared run remains owned until its native prompt and admitted delivery
and cancel operations settle. Closing or losing the worker invalidates its
unacknowledged results. See the [generic skill](skills/sessionbus/SKILL.md) for
the action examples, completion pointers and independent lifetime policies.

Idle stage retains only never-submitted messages in a bounded memory FIFO;
queued_for_next_turn is not native admission. A seeded wake returns written
after the full native prompt request write. Active delivery waits for native
craft/drainMidTurnQueue and writes at most ten messages per response. Written
does not mean injected or consumed. Native late recovery owns handed-off
messages; the wrapper never replays them. An unsent message remains staged if
the current run ends without a native pull.

Close and automatic close retire the lane and leave native Qwen history in
place. Forget removes the daemon's resume recipe, not native history. There is
no wrapper database, result journal or restart recovery. The shared result
cursor and staging are bounded worker memory.

## Checkpoint limits

This source checkpoint updates guidance and payload reconciliation while
retaining the current MCP activation mechanism: the globally installed
extension's `mcp.json` still invokes `qwen-peer mcp` from PATH. The lane's
per-session same-name server selects the private entry under the inspected
native precedence; unrelated native servers and settings retain their normal
behavior. No global enable/disable toggle or per-launch MCP-config merger is
introduced here.

The generic skill is therefore globally discoverable. Ordinary isolation and
the remaining interactive implementation are unfinished in this checkpoint.
The owner has accepted the released native interfaces' session-switch limit:
Sessionbus binds to the launch's initial native session. After an in-process
/new, /clear, /resume or other switch, outbound MCP calls can retain that old
identity while input-file deliveries reach the displayed session. Presence,
delivery and completion-pointer attribution across that switch are outside
supported guarantees. Exit and start a fresh `qwen-peer` using the native
selector to use Sessionbus with another session. This is guidance, not an
enforced switch ban or a race-free withdrawal mechanism. ACP lanes each own a
dedicated session and are unaffected. No native patch is a prerequisite.

The lane runtime was exercised on unmodified Qwen 0.23.0: new/resume startup,
native ToolSearch plus public list, staged delivery, seeded wake, active pull,
interrupt, repeated collection/acknowledgment and healthy following runs.
Receipt limits and later process-absence evidence are retained separately;
these observations are not an interactive or all-policy acceptance claim.
This skill migration itself has archive/literal-reinstall tests; it has not
yet replaced the real installation or received native skill-discovery credit.
