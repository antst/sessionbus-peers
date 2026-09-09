# Codex Go composition acceptance — 2026-09-09

The source selections in this directory are historical design decisions. This
ledger records the implemented boundary and the strength of each check. Native
Codex is 0.153.4, executable SHA256
`56ef98ab4032d317ab26e9b5e5a175650717351edb16ed9cde0cb6d1734d62da`.
The real umka daemon remained d11fa743, SHA256
`295fb3d6ff0efd66f313d912daac168b5cc06a57fabc86868e300b86694946ff`.
The wrapper pins public Go SDK f415cf720584, including the independently reviewed
late-successful-Open cleanup correction. No wire/daemon reinstall was required.

All installed work used real login/home/config/service and the permanent public
commands. Native folder-trust prompts were answered through native UI. Those
native trust edits are recorded; there is no claim that config never changed.
Unrelated configured MCP errors and processes were preserved. No private native
home, alternate product binary, fixture MCP replacement or global approval
change supplied a pass.

## Installed composition

Evidence directories below are relative to the immutable packet
`/home/antst/codex-architecture-20260909/`. `first-contact.json` binds the exact
manifest paths and hashes. The parent projection contains inherited history;
only each row's later native input/tool/result is credited to that execution.

| Requirement | Actual installed observation | Evidence |
|---|---|---|
| Failed Open and activation | Wrong quoted CLI key failed required-server readiness before input. Correct unquoted CLI key opened; config/value/write retains its separate quote-aware key. No relaxed gate. | dev1-lane-open-ebd5c0a; dev1-lane-open-aaea430 |
| No-input lane readiness | Actual native identity/title, bound worker/AppServer/MCP and OpenResult; no model prompt. Gate code/event correlation is independently source-tested; not every native readiness frame is separately captured. | dev1-lane-open-aaea430 |
| Idle staging | Native inject_items staged user marker before assistant output; queued receipt, one later explicit run consumed marker. Initial visibility rejection is preserved as routing-only. | dev1-lane-stage-aaea430 |
| Seeded wake | Idle message started one shared run; injected receipt, terminal collection/ack/close. No adapter scheduler. | dev1-lane-wake-aaea430; dev1-lane-grant-aaea430 |
| Native MCP policy | Inherited policy rejected the call; explicit never still required approval. Caller per-plugin/tool approve allowed actual list. It grants all actions of that tool, not only list; no default grant. | same wake/grant rows |
| Parent operates child | Actual native parent public list/describe/spawn/start/wait/ack/close, child exact result and parent exact marker. No external controller substitutes for lifecycle calls. | dev1-interactive-9b833c3/PARENT-FINAL-SHA256SUMS |
| Completion pointer/idle wake | Child completed after parent returned; authenticated ordinary pointer message caused next native parent turn with actual wait/ack/close and exact reply. | same parent seal; transcript-parent-pointer.json |
| Active interactive admission | Matched native delivery receipt preceded independent held-command release; exact later marker followed zero-marker tool return. Native one-time command approval passed through TUI. Receipt is admission, not while-held consumption. | dev1-interactive-active-9b833c3 |
| Groups/name/fork/clear | Repeated mixed -g/--group appear on actual rows. Native rename preserves ID; fork gets new ID and its public spawn reports fork owner. Clear creates an unnamed row while earlier native threads remain loaded/addressable. | parent seal, roster0–4 and fork projection |
| Native resume | Saved parent ID/name restored and new native public list/exact reply succeeded. Saved child ID resumed and performed its new native public list/exact reply. | DEATH-FINAL-SHA256SUMS; dev1-lane-resume-loss-9b833c3 |
| Ordinary Codex | Zero prompts, actual plain native argv, native MCP inventory excludes Sessionbus and no Go broker/helper/peer row. Existing native config warnings retained. | parent seal, ordinary directory |
| Normal interactive quit | Native exit0/reaped; later all8 identified processes, three peer rows, directory/two endpoints absent. No individual child exit-status/raw EOF attribution. | parent seal |
| Abrupt native TUI death | No-input exact TUI SIGKILL; observer records signal9/reaping, all5 identified processes and owned endpoints absent later. No active-tool containment credit. | DEATH-FINAL-SHA256SUMS |
| Abrupt broker death | Broker zombie under live TUI, native server/MCP/code-mode host absent, peer row absent. TUI's native reconnect screen and stale owned sockets remained. Later Ctrl-C/absence and exact operator socket removal are separate cleanup. | death seal |
| Lane forwarder loss | After completed public call, exact bound helper kill retired lane; all3 identified processes absent, row disconnected, unacked result query not_connected. Not a pending-turn or endpoint cleanup observation. | dev1-lane-resume-loss-9b833c3 |
| Shared Claude regression | Actual rebuilt permanent Claude archive/native2.1.266 loaded generic skill, ToolSearch and real public list with own ID/name/two groups/exact reply. Ordinary zero prompts had no Go MCP child. Both native exits0/reaped, all7 identified processes absent. | dev1-claude-shared-regression |

The normal-close rows preserve native history. Daemon result cursors disappear
on retirement; saved history is not a retained answer cache. Shared persistence,
auto-close, wake independence and cursor policy use the prior merged PR48/peers
PR10 acceptance; this work does not repeat their entire native matrix.

## Controlled tests and retained facts, not new native rows

- Lane active steer/interrupt use the retained native API and captured terminal
  shapes. `TestLaneRunSteerAndTerminal`, `TestLaneInterruptAfterNativeStart` and
  `TestLaneTerminalBeforeSteerResponseRetainsAdmission` test the changed adapter,
  including successful admission after terminal ordering. The new installed
  held active row is interactive; it is not relabelled as a lane interrupt test.
- `TestNativeStageRequiresAckAndPreservesRunBoundary` covers empty-ack overlap;
  `TestCodexActualWorkerSeedReceiptTerminalAndCollection` uses the real Worker
  for seeded admission, receipt/terminal progress, collection and loss.
- Cancellation, bounded abandoned drains, incoming request resolution and
  completion-wins races use actual transport/MCP/Caller tests. There is no claim
  that an installed native MCP cancellation notification was forced. No native
  undo or replay is implied by cancellation.
- Failed-start cleanup, completed-Open context ownership, malformed drain,
  actual process exit and Close errors have controlled compiled-child tests.
  The installed failed-activation row is not proof of every failure schedule.
- Native config spelling/order and settings projection use source plus exact
  compiled production-entry tests. Profiles remain TUI-only; settings omitted
  by native remote projection do not gain standalone equivalence. Interactive
  caller --remote conflicts explicitly. No native option whitelist is added.
- Darwin watch execution is a controlled macOS CI check, not an umka native
  observation. Linux native death observations do not establish Darwin cleanup.
- Arbitrary tool-descendant containment is outside this product's accepted
  ownership scope. No ancestry registry, sweeper or grace timer was added.

## Source, payload and cost

The tested combined Codex runtime is9b833c3, binary SHA256
`44e476739cdada2a411e3db857855d3c480d03fa5e7395b2e12854bd9d1af2d2`.
Later commits amend documentation/CI/measurement only. The tested shared Claude
archive is9be97a8, binary SHA256
`49b32c7a3f7915ba0f15d71e39c235818d8c45b006e529cbcb190087d17a4838`.
Final packaging rebinds unchanged runtime bytes and separately changed README/
source-version metadata. Both permanent installations remain usable.

`SIZE.json` counts the linked artifact, archive members, common/local dependency
source, all retained Codex files/tests and the whole tracked tree. The broker
adds a process, two endpoints, ID routing and an OS watch; it is not a byte-only
relay. The WebSocket library is coder/websocket1.8.15 with its ISC notice and no
transitive runtime module dependencies. Go standard-library cost is included in
the linked executable; native Codex and its configured helpers are external.
