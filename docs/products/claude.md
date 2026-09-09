# Claude Code product facts

> Historical source note: citations to pre-split Sessionbus paths resolve in
> the Forgejo `ai/sessionbus` repository through its `legacy-*` branches.
> Citations to product source resolve in the external repository and full
> commit recorded by the split archive manifest. Host evidence paths are
> immutable external artifacts, not repository paths.

## Current interactive candidate — 2026-09-09

Implementation steps 1–6 are released by delivery-6edd9c4b9118c9fbef0bdb212d8012c4. The candidate is `@sessionbus/claude` 0.5.0-interactive.0, with the exact kit preview at signed 5f93fbb2532b0119edbd427aba21507989dddbfb (compatible daemon b1d7adbc922c24734abc729a49503eff4bcd3484). [Inventory](../designs/claude-0.5.0/INTERACTIVE-IMPLEMENTATION-INVENTORY.md) and [dependency binding](../designs/claude-0.5.0/KIT-BINDING.json) specify the changed implementation. Candidate native installation/FC01–FC08 have **not run**; lane L02/OB03 and lane implementation remain held. Offline tests are implementation checks, not native acceptance.

Current interactive publication follows the first usable native report, with no launch/prompt/turn deadline and no generated identity or name. A usable Stop can establish unnamed presence; a later title report supplies the name. Native rename refreshes at a subsequent report. `written` acknowledges only the local native-carrier write. Missing native EOF evidence stays a limitation; implementation stdio cleanup is tested separately. The older inherited lock, injected-on-write, option-parser and PID-lookup prescriptions below are historical; they are removed from this interactive implementation.

All following raw paths resolve on umka-dev1 under `/home/antst/sessionbus-evidence/claude-phase-a-20260908`. The exact ledgers and source are retained at the stated full commits in the separate evidence branch and sealed bundles. Review reports reside under `/home/antst/claude-architecture-20260908/evidence/`. Product probes used the real login/home/config/service; this implementation has made no product or service change.

| Fact | Native version | Exact ledger commit / raw evidence and limit |
|---|---|---|
| First prompt can miss MCP hook during plugin startup; later hidden Stop succeeded | 2.1.265 | d16a3caea7a868e1385c22144110c53fdddd0a3a, `raw/fp01-runtime`; operator submission precedes initialize response, native hook processing time unobserved. No deadline guarantee. |
| Named fresh, next-prompt rename, clear, resume and fork publish native identities | 2.1.265 | 9dec0e74b6278a3d014d71266c60eff14f203a1e plus wording amendment 1df7ae7f1720ef17c9ac34c1c8810eb90aecac3d, `raw/fp03-runtime`; only clear has recorded SessionEnd/withdraw. Quit has empty rosters/exits, no hook cause attribution. |
| Unnamed hello then same-ID named rehello | 2.1.265 | f30c5757a158aba83ad78f0edbc6ec2ea4f6d38f, `raw/fp03u-runtime`; idle rename alone did not name the roster; next prompt did. No natural Stop bootstrap in this row. |
| Idle and held-tool-boundary real delivery with written receipt and exact native consumption | 2.1.265 | c7c05fb20e38995b61495e47b934b6d44efe72b4, `raw/fp03d2-runtime`, `raw/fp03d2-post-exit`; active consumption after release, routing-only unknown-target rejection, no uncertain-write/native acknowledgment credit. |
| Native quit/kill, owner kill, local Connection destruction remove observed presence | 2.1.265 | a41d1af1827fa737371d74fb3771952c90e8c95f, `raw/fp04-final`; native remains alive after owner kill or local bus loss. No published-owner EOF, remote failure, pending-call or Peer-policy proof. One final native zombie retained as an accounting limit. |
| Native updates can occur during a parent session | 2.1.265 root / 2.1.266 child | 4bf03abfa74620c0f8fd009287b5f134d0e2eb38, `FP05-PARTIAL-EVIDENCE.json`; evidence hash guard stopped before child initialize/user writes. Integration has no version guard, selection, pin or downgrade. |
| Distinct nested peers share inherited explicit groups; same-ID resume supersedes old bus owner | 2.1.266 | d65ffaa21a05490ababc13da19c2b4d97c7d8ce5, `FP05-266-EVIDENCE.json`, `raw/fp05r266-roster-both-live`, `raw/fp05r266-observed-superseded`; three exact replies, three native exits0/reaped, nine identified PIDs absent. Old-owner exit cause unknown; cleanup-helper EOF was unpublished. No writer lock, merge, general lifecycle or lane claim. |
| Zero-turn native named session not resumable in tested options | 2.1.263 | e5350e7af5b4e8181632ac1eb481c2d626c66135 and later B06/NP ledgers in evidence root; actual native missing-conversation diagnostics, not a universal storage claim. Complete a real turn before quitting a session intended for resume. |

## Historical split archive (unchanged citations)

- Claude Code 2.1.260 accepts `-p --input-format stream-json --output-format stream-json --verbose --replay-user-messages` for one resident headless session. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/native-private-mcp.processes.txt`
- A fresh lane uses a bare UUID with `--session-id`; its native session ID is that exact UUID. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/lane-spawn.stdout`
- Claude Code emits no `system/init` frame before the first stream-json user frame. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/init.result.json`
- The first turn emits `system/init`, and its `session_id` is available for checking against the wrapper-minted UUID. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/claude.stdout`
- A replayed user frame has `type:"user"` and `isReplay:true`; it is Claude's acknowledgement that the corresponding input entered the native turn. — verified: 0.4.0 — source: `internal/products/claude/lane.go:438`
- A result is a turn terminal only after every accepted user write for that turn has a matching replay. — verified: 0.4.0 — source: `internal/products/claude/lane.go:489`
- A successful stream-json result has subtype `success`, `is_error:false`, the exact session ID, and a string result. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/claude.stdout`
- Native terminal reasons `interrupted` and `aborted_streaming` map to an interrupted turn. — verified: 0.4.0 — source: `internal/products/claude/lane.go:510`
- A native interrupt is a `control_request` whose request object has subtype `interrupt`. — verified: 0.4.0 — source: `internal/products/claude/lane.go:277`
- `control_response.response` is an object with subtype and request ID; a successful interrupt response may contain a nested response object. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/claude.stdout`
- The lane's completed turn returns the final Claude result and native stop reason `completed`. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/lane-first-run.stdout`
- The lane's same-name `sessionbus` stdio MCP entry invokes `claude-peer mcp` through the private Unix socket and completes a caller action. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/native-private-mcp.processes.txt`
- Active lane delivery is acknowledged as `injected` only after its native replay, and the delivered text affects the same turn's final result. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/native-active-inject.run.stdout`
- Idle lane delivery is acknowledged as `queued_for_next_turn`, then prepended to the caller's next run. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/native-idle-queue.run.stdout`
- An exact native interrupt returns `{}` on the bus after Claude accepts the control request. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/native-interrupt-fixed.stdout`
- A healthy lane close returns `{}` and releases the row. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/native-close-healthy.stdout`
- The inherited flock keeps a native session busy after the wrapper dies and releases it when the native child exits. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/native-abrupt-lock.busy.stdout`
- Claude Code accepts an HTTP MCP entry shaped as `{"type":"http","url":"http://127.0.0.1:<port>/mcp"}`. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/p0-http.config.json`
- Claude Code's HTTP MCP client first sends `server/discover`, then initialize, initialized, GET for an event stream, tools/list, and tools/call. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/p0-http.events.jsonl`
- Claude Code's HTTP MCP client does not require or send an MCP session header when the server returns none. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/p0-http.events.jsonl`
- The 0.4.0 interactive launcher preserves Claude argv, projects wrapper name and groups, and exports launcher identity before replacing itself with Claude. — verified: 0.4.0 — source: `internal/launcher/claude_peer.go:15`
- Without launcher identity, the 0.4.0 connector runs `claude agents --json` and accepts exactly one `interactive` row whose PID equals the connector's parent PID. — verified: 0.4.0 — source: `ff81565:cmd/agent-sessions/connector.go:449`
- Interactive inbound delivery writes one newline-delimited msgV1 user frame with `priority:"next"` to `CLAUDE_CODE_MESSAGING_SOCKET`. — verified: 0.4.0 — source: `ff81565:cmd/agent-sessions/connector.go:383`
- Interactive title changes update the same peer row without changing groups. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/peer-title.list.stdout`
- `/clear` replaces the native Claude session ID on the same bus connection, and later delivery targets the replacement ID. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/peer-clear.after.stdout`
- Interactive delivery through the Claude messaging socket is reported as `injected`. — verified: 2.1.260 — source: `/home/antst/agentbus-evidence/claude-20260906T004825Z/peer-delivery.stdout`
- `UNVERIFIED:` The exact stream-json result fields and error text emitted by Claude Code 2.1.260 for every failed terminal subtype have not been captured.

## Per-launch activation amendment and implementation scope

AP01 on Claude2.1.266/bus b1d7adb, signed evidence293ebf3 at `/home/antst/sessionbus-evidence/claude-phase-a-20260908/AP01-ledger.md`, observes native per-launch skill expansion, hidden identity reports, named/grouped hello and one public diagnostic list call. Exact native disallow omits the public tool from observed ToolSearch results; the handler is never called. Ordinary control supplies no fixture MCP/report/row, with incomplete skill-inventory coverage. The granting rule was not isolated; the diagnostic was read-only. No production-action approval follows.

The stamped [activation amendment](../designs/claude-0.5.0/ACTIVATION-AMENDMENT.md) replaces global publication with npm-only per-launch whole-plugin loading. The launcher prefixes exact allow then plugin-dir before verbatim native argv; only its final group suffix is consumed. One public bin, native-root MCP path, three hidden hooks and the bundled skill are included. Production multi-action tool has no read-only annotation. Plain unconfigured nested Claude remains ordinary; earlier globally installed FP05 evidence does not prove inherited flags.

Offline package/transport tests establish implementation behavior only. Actual installed payload, exact production prefix, native permissions, FC01–FC08 and lane acceptance remain separately gated; no current production native run is claimed.
