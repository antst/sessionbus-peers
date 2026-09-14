# Claude Code product facts

> Historical source note: citations to pre-split Sessionbus paths resolve in
> the Forgejo `ai/sessionbus` repository through its `legacy-*` branches.
> Citations to product source resolve in the external repository and full
> commit recorded by the split archive manifest. Host evidence paths are
> immutable external artifacts, not repository paths.

## Current shared lifecycle — installed 2026-09-09

The [shared lifecycle mapping](../designs/claude-0.5.0/shared-lifecycle.md) and
[installed acceptance ledger](../designs/claude-0.5.0/shared-lifecycle-acceptance.json)
bind the merged PR48 SDK/daemon `d11fa7433694` and permanent Go Claude installation.
Actual parent-tool calls establish the independent persistence/archive policies,
idle staging versus message-triggered work, lane-originated completion pointer,
non-consuming reads, explicit ordered ack and collection after parent exit.
Same-install ordinary/mixed-group/idle-wake/quit regressions are included.
Tests ran on `7d5fb9f`; `c138131` changes only tool/skill/README deadline wording.
Automatic close follows native completed/failed/interrupted terminals; an
unavailable record alone does not establish that native event.

Exact scope and accounting limits are in the ledger. The older installation,
SDK bindings and native results below remain historical evidence at their own
revisions; they are not the current dependency or installation instructions.

## Historical interactive acceptance — 2026-09-09

The interactive candidate is now one Go binary plus native plugin assets; it
preserves reviewed Node behavior at `9644348` and pins the public Go kit to
merged `f8d409e98218`. The accepted interactive artifact on umka was `a056303`,
SHA256 `dc5d8b745a393e32721a5f2c14d59dc2c455f59419f34a88881657bb2083d805`.
It requires no Node/npm integration runtime. The
[Go migration](../designs/claude-0.5.0/GO-MIGRATION.md),
[Go dependency binding](../designs/claude-0.5.0/GO-KIT-BINDING.json) and
[installed ledger](../designs/claude-0.5.0/interactive-candidate.md) supersede the
archived Node installation instructions. Actual installed interactive checks
ran in the real umka environment. Lane development now replaces that same
permanent installation; the scoped lane observations below do not establish
complete lane acceptance. Offline fixtures establish implementation behavior separately.

Current interactive publication follows the first usable native report, with no launch/prompt/turn deadline and no generated identity or name. A usable Stop can establish unnamed presence; a later title report supplies the name. Native rename refreshes at a subsequent report. `written` acknowledges only the local native-carrier write. Missing native EOF evidence stays a limitation; implementation stdio cleanup is tested separately. The older inherited lock, injected-on-write, option-parser and PID-lookup prescriptions below are historical; they are removed from this interactive implementation.

The historical table below uses raw paths on umka-dev1 under `/home/antst/sessionbus-evidence/claude-phase-a-20260908`. The exact ledgers and source are retained at the stated full commits in the separate evidence branch and sealed bundles. Review reports reside under `/home/antst/claude-architecture-20260908/evidence/`. Historical product probes used the real login/home/config/service. The Go candidate was subsequently installed, removed and reinstalled as recorded below; the executor did not alter Claude credentials or restart the daemon.

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

## Go installed observations — Claude 2.1.266

Raw root on umka: `/home/antst/sessionbus-evidence/claude-go-fc-20260909/raw/`.
Immutable row manifest hashes and counts are in
[first-contact.json](../designs/claude-0.5.0/first-contact.json); detailed comparisons
and limitations are in the [installed ledger](../designs/claude-0.5.0/interactive-candidate.md).

| Observation | Raw row | Limit |
|---|---|---|
| Installed skill/public list reports actual name, native ID and explicit group | fresh | No sole permission-grant attribution or publication deadline. |
| Native name resume selects original ID; rename becomes visible next prompt | resume | Older inherited transcript work distinguished by UUID. |
| Idle and held-tool active messages return written and exact assistant markers | fresh, resume | Active consumption follows release; written is not native acknowledgment. |
| Unnamed publication becomes named at next prompt on the same ID | unnamed | Projected name cannot distinguish omitted and empty wire fields; no Stop-bootstrap trace. |
| Native fork produces new ID; same-ID resumer survives old quit; clear publishes another new ID at next prompt | fork, supersede | No explicit superseded wire trace, writer lock or history-merge claim. |
| Killing the bound Go MCP removes roster while native remains alive | supersede | No End/EOF cause attribution; later native quit is separate cleanup. |
| Plain Claude has no observed direct Go MCP or candidate row; native disallow omits public tool and no public call occurs | ordinary, denied | Not exhaustive skill inventory or explicit denial-frame proof. Ordinary observer remains zombie at its final checkpoint. |
| Exact invalid option has identical native/wrapper stderr hash and exit1 | invalid | Exact tested option only; wider argv semantics covered by controlled exec tests. |
| Real removal and reinstall restore exact payload; candidate remains installed | remove-reinstall | Only listed configuration/native/service hashes compared; no universal cache/history claim. |


## Go lane development observations — Claude 2.1.266

These rows use the same permanent umka installation and real daemon `b1d7adb`.
Raw evidence root: `/home/antst/sessionbus-evidence/claude-go-lane-runtime-20260909/raw/`.
Each named row has an immutable SHA256 manifest; `FINAL-SHA256SUMS` is the final
manifest where present. This section supplements the accepted interactive
observations; it does not relabel older receipts or extend cleanup credit.

| Observation | Installed source / raw row | Limit |
|---|---|---|
| Zero-input Open returns a native ID with the bound initialize/title/tool gates; worker/native/MCP share the worker group | fd1e89f / ob03-first | OpenResult plus bound gate code; native control response bodies not independently retained. |
| Killing the idle worker causes four identified pidfd exits | fd1e89f / l02-worker-death, first-open-cleanup | Immediate child zombies, later identified PIDs absent; endpoint removal was operator cleanup. No active-tool guarantee. |
| Idle replay stages a marker without starting an assistant turn; one explicit run invokes the real public list and consumes it | fd1e89f / lfc-core | Exact projected result; no following-run duplicate check in this row. |
| Default native permissions refuse the requested Bash command with “requires approval”; the model still completes its turn | fd1e89f / lfc-active | Tool refusal is not automatically a failed native terminal. No adapter permission substitution. |
| Interrupt produces native aborted_streaming; following explicit run succeeds | fd1e89f / lfc-interrupt | This sealed build incorrectly mapped the terminal to failed; the mapping correction and regression are separate source changes. |
| Caller-supplied exact Bash allow rule admits the held tool; replay and exact marker occur in the current run, following run returns only NEXT | 7f40b24 / lfc-active-allowed | Recorded queued_for_next_turn was a receipt defect, preserved as observed. No sole permission-classifier attribution. |
| Bash tool PID/pgrp 1730226 is outside worker pgrp 1729921 | 7f40b24 / lfc-active-allowed | A worker-group signal does not directly reach this tool; normal cleanup does not prove death cleanup. |
| Hard worker kill leaves a native tool and its parent alive outside the worker group | 145cf92 / lfc-active-death | Pending run gets daemon -32002, no native terminal; operator release and exact endpoint removal are separate cleanup. |
| Interrupt ends the held tool; following session.close returns success while its held tool/parent survive | 145cf92 / lfc-graceful | Interrupted terminal is not descendant-cleanup acknowledgment. lfc-graceful-cleanup records operator release; later tool release record is supplementary. |
| Invalid argv/cwd fail Open; concurrent distinct lanes and exact saved-ID resume succeed; connected-ID resume returns AlreadyConnected | 7f40b24 / lfc-config | Zero model input. Same repeated model values establish argv order only; failed-start diagnostics/reaping not captured. Closing one leaves the others connected. |
| Corrected EOF close: separate held interrupt and held close each end the observed tool and parent | 653b41e / lfc-graceful-r3 | Both exact PIDs absent at checkpoints after the operation; no independent before-close-result cleanup or native exit/signal ordering capture. No operator release. The preceding r2 socket-length harness failure remains separate. |
| Cancelling Caller wait leaves its handle collectible after explicit interruption | 653b41e / lfc-cancellation | Cancelled wait on t-1, later collection yields interrupted/aborted_streaming; status after collection fails. Caller API cancellation, not a native MCP cancellation notification. |
| Killing the installed MCP forwarder during the following run disconnects the lane and settles the result | 653b41e / lfc-cancellation | Public Worker returns failed EOF, not a native terminal; five identified PIDs and endpoint absent afterward. No native exit-status/reaping credit. |
| Ordinary Claude stays unintegrated; same-build interactive public list and idle delivery still work | 653b41e / ordinary653, integrated653 in claude-go-lane-interactive-20260909/raw | Actual list and exact replies; idle receipt written only; native quit0/reaped and identified PID absence. No quit-hook/EOF causal attribution. |

The [stamped receipt correction](../designs/claude-0.5.0/LANE-DELIVERY-AMENDMENT.md)
requires exact message UUID/session replay: `injected` during the same confirmed
active run, `queued_for_next_turn` for demonstrated idle staging, and uncertainty
across an unclassified run boundary. These receipts acknowledge admission or
staging, never consumption. The corrected source has controlled boundary tests;
the original active row remains evidence of the old incorrect label. The corrected
installed `lfc-active-admission` row returns injected before its exact same-run
marker and is independently verified.

The [graceful-close correction](../designs/claude-0.5.0/LANE-GRACEFUL-CLOSE.md)
separates native lifetime from the completed Open context. Normal close ends
stdin, drains native output/reports and waits for actual exit; cancellation or
unexpected loss aborts. Controlled tests include the real Worker cancellation
order and an actual compiled child fixture. The corrected installed row above
observes tool/parent absence after each completed operation. Generic containment
after forced death remains a bus/platform ownership gap, not a wrapper PID registry.
Startup cancellation is covered by controlled actual-Worker/compiled-child tests;
no separate installed native startup-cancellation row is claimed.

## Claude-originated lane lifecycle — Claude 2.1.266

The corrected live tool, doctor and generic Sessionbus skill describe lane
operations. The unused repository marketplace was removed; the native manifest
remains at `claude/.claude-plugin/plugin.json`. Plugin and MCP metadata now use
`0.5.0`. Historical Node and held-reference metadata is unchanged.

On installed source `3ee62d6`, native parent `ec74d626-aeb0-4da0-b28b-026b34d6cecd`
called the public Sessionbus tool to spawn child
`8ec6b75a-36e1-49b8-9744-70ce817264f1@local`, start a turn, wait on returned handle
`t-1`, and close that child. The collected result was completed with the exact
child marker; the separate child transcript has the same reply and input hash.
The close tool result decodes to `{}`. This is the parent's real tool lifecycle,
not an external controller's lane operations. Source:
`/home/antst/sessionbus-evidence/claude-parent-lane-20260909/raw/parent/FINAL-SHA256SUMS`
(43 entries). Parent quit0/reaped and three identified PIDs absent are recorded;
child PID/exit-status accounting is not. The parent final response contained extra
prose rather than the requested exact marker. Read-only observer executable-mode
and group corrections are harness notes, not product failures or absence proof.

Installed `9fd7aa5` subsequently makes the single skill's wording product-generic,
using Claude as its concrete example. Archive comparison proves the binary is
byte-equal and only that skill asset changed; no native rerun is attributed to
the wording amendment. Installation evidence:
`/home/antst/sessionbus-evidence/claude-parent-lane-install-9fd7aa5/`.

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

The stamped [activation amendment](../designs/claude-0.5.0/ACTIVATION-AMENDMENT.md) historically selected npm-only per-launch whole-plugin loading. The Go migration preserves that native activation behavior and replaces npm with the bundled archive. The launcher prefixes exact allow then plugin-dir before verbatim native argv; the owner correction consumes repeated `-g VALUE`, `--group VALUE` and `--group=VALUE` anywhere before native `--`, and translates `--yolo` to `--dangerously-skip-permissions`. Other native arguments remain in order; after `--` they are literal. See the [installed correction ledger](../designs/claude-0.5.0/launch-flags-fix.md). One public bin, native-root MCP path, three hidden hooks and the bundled skill are included. Production multi-action tool has no read-only annotation. Plain unconfigured nested Claude remains ordinary; earlier globally installed FP05 evidence does not prove inherited flags.

Offline package/transport tests establish implementation behavior separately from the installed Go observations above. Exact native EOF/exit ordering remains source/test evidence where not separately captured at runtime; generic forced-death descendant containment remains open.
