# Installed Go interactive candidate — 2026-09-09

The installed Go adapter preserves the reviewed Node behavior at `9644348`,
including C01 cancellation retention, C02 actual-wire argument validation and
C03 the single active skill. This is installed interactive acceptance at the
recorded strength, not a new native capability survey or Claude lane acceptance.
At that accepted historical snapshot, token-selected lane mode was unavailable.
The current combined development section below supersedes that mode status.

Evidence host: **umka-dev1**, real `/home/antst`, login shell/PATH, ordinary
Claude configuration and the existing `sessionbus.service`. Raw rows are under
`/home/antst/sessionbus-evidence/claude-go-fc-20260909/raw/`. Each row has an
immutable `FINAL-SHA256SUMS`; exact manifest hashes and entry counts are in
[first-contact.json](first-contact.json). Pdev holds review copies and offline
fixture tests only. Native runtime was never run on pdev.

## Artifact and installation binding

The initial `efcd86b` install was a runnable checkpoint, not acceptance: review
found G01 blocked stdin during Go dial/write. The corrected `76b921b` binary
passed the fresh row. Subsequent rows use `a0563035c0255c4d2fa6c3562e7a28c2506d1e5b`
with merged public Go SDK `f8d409e98218`; later PR commits change tests, CI and
documentation only. G01 is explicitly described in
[GO-MIGRATION.md](GO-MIGRATION.md).

Historical accepted interactive binary SHA256:
`dc5d8b745a393e32721a5f2c14d59dc2c455f59419f34a88881657bb2083d805`.
The private plugin alias resolves to that same binary. Public path:
`/home/antst/.local/bin/claude-peer`. Permanent package:
`/home/antst/.local/libexec/sessionbus/claude/`.

Actual native processes were Claude **2.1.266**, executable SHA256
`19842705e989393fce936804df6d2ab034860e24b8f8880357981d87ffd83fac`.
The existing daemon stayed PID1688425, binary SHA256
`e33415994997ea2991119ad6978fb1b97a7273dc59dcfd7784009bdc9ef82c24`
(commit `b1d7adbc922c24734abc729a49503eff4bcd3484`). The integration has no native
version guard, pin or downgrade. Observed versions qualify evidence only.

## Installed checklist and comparison

| Check | Actual installed evidence | Relation to retained Node/native evidence and limits |
|---|---|---|
| FC01 package, activation, skill and public tool | `fresh`: real installed launcher/native/MCP executable binding; `/sessionbus:sessionbus` expands and native public `list` returns the actual peer ID/name/groups. Installation seals also live under `claude-go-install-{efcd86b,76b921b,a056303}`. | Replaces npm with one Go binary/native assets. Same per-launch whole-plugin activation; no fixture owner/plugin substitution. Public list success does not isolate the sole native permission grant. |
| FC02 named and unnamed publication | `fresh`: ID `dd132cac-7b32-4a01-bed9-a1c3b0ed6a4c`, name GoFC01-20260909, explicit group go-fc-20260909. `unnamed`: ID `2a83f7a8-a789-456a-9cc0-1d1f6279e70f` initially has an unpopulated name; idle rename leaves it unchanged, next prompt supplies GoFC-Unnamed-Renamed-20260909 with the same ID. | Matches FP03/FP03u first-report behavior. Control's Go struct projects empty name for omitted or empty wire fields, so this row does not distinguish them. No direct hello trace or natural Stop-bootstrap credit. |
| FC03 quit and cleanup | Fresh/resume/fork/supersede/unnamed/denied native exits are 0 and reaped by the recording observer. Exact identified PIDs are absent in those final checkpoints. Ordinary native also exits0/reaped, but its observer1719890 is Z at its final checkpoint. | No general descendant/reaping guarantee. Pane query exit0 with blank output is not proof of pane absence. No quit SessionEnd or native stdin-EOF causal inference. |
| FC04/FC05 native resume by name and selected identity | `resume`: argv contains `--resume GoFC01-20260909`; native selected the original dd132cac ID, and a new public list/result and exact reply establish new work. | Matches saved native resume evidence. Retained older transcript records are not credited as new work. No selector or session listing on the integration side. |
| FC06 idle and active delivery | `fresh`: message-a3tqzksooao7 returns written and exact idle reply follows. `resume`: message-pz0fcbt5g3bz returns written while actual native Bash toolu_0127FVYhRw1aJdtgzuHKZ2mA is pending; one explicit controller release follows the receipt. Tool return contains zero active markers; exact assistant marker follows release. | Matches FP03d2/native msgV1 carrier. Active consumption is after release, not while held or arbitrary mid-token. `written` is local write completion, not native acknowledgment. No universal duplicate/race guarantee. |
| FC06 negative and uncertain boundaries | `after-quit-rejection`: withdrawn target receives rejected/unknown_session. Actual Go Connection/schema tests cover valid Internal/no_receipt for attempted short/failed writes. | Native row is routing-only, not a native-connect failure or uncertain-write runtime proof. Production has no retry or invented native refusal. |
| FC07 remove/reinstall | `remove-reinstall`: literal README rm of exact checked symlink/package, command lookup absent, ordinary Claude version available, then mkdir/tar/ln reinstalls exact archive. All archive member hashes/private alias match; candidate left installed. | Real permanent installation, no temporary home/prefix. Listed native/config/npmrc/unit/live-daemon hashes match before/removed/after; no daemon restart. This is not universal cache/history/config byte equality. |
| FC08 native flags and ordinary use | `invalid`: native and wrapper have identical stderr hashes/exit1 for the exact invalid flag. `ordinary`: plain native Claude, zero prompts, no direct Go MCP child and no candidate bus row. Compiled-fixture tests prove exact argv/PID/cwd/stdio/exit37/SIGTERM on Linux/macOS. | Native behavior reused; no adapter flags/errors/identity matcher. Ordinary observation is scoped to the recorded children/roster, not exhaustive native skill inventory. |
| FC08 explicit native disallow | `denied`: caller's native disallow targets the public tool. Actual ToolSearch results omit it; no public Sessionbus invocation appears, and the requested unavailable reply appears. Hidden publication still works. | Matches AP01 measured omission route. No explicit permission-error frame, handler-side instrumentation or sole classifier attribution. Other real installed tools remain visible; they were not substituted. |
| FC08 rename, fork and same-ID handover | `resume` next-prompt rename; `fork` new ID `b3fc3115-7e62-4efe-b33f-78622c2dbee6`. `supersede` resumes that ID while the original native is alive, returns a new public result/reply, retains one roster row after the old session quits. | Matches retained native fork/supersession behavior. No explicit session.superseded wire trace, writer exclusion or history-merge guarantee. Fork's later transcript snapshot includes subsequent resumer records and is qualified accordingly. |
| FC08 clear and owner death | `supersede`: /clear removes old row, next prompt publishes new ID `2affcdc0-08da-447a-9a55-0210eabac7df`. Exact hash-bound SIGKILL of Go MCP1716794 removes roster while native1716641 remains alive; its later /quit is separate cleanup. | Same first-report/lifetime boundary. No End/EOF causal trace or whole-product death assertion. Controlled actual-wire tests separately establish implemented EOF/output-failure/bus-loss cleanup and no reconnect. |

## Exact replies and accounting

Fresh `ac51404d-7187-4964-8f96-8929f2c98876`; idle
`b2125f78-cdc9-45f4-b188-2287c56f567a`; resumed
`2f5873d9-f949-4b75-98bb-0586609d6c2d`; active
`62e67939-8d6f-4228-9ca7-0e08b3168c44`; fork
`97fa0c16-fd50-4d8b-874c-04f69103c6e4`; same-ID resumer
`ee02c9ce-e60d-4a90-a7f8-d8f9f3f14a49`; unnamed
`e60bb774-e5ef-406e-b545-39f16059c330`; named-later
`45827b83-34ef-4bf6-89f8-54674616d7de`; denied
`bc429e5d-1a45-4881-96eb-754f09461589`. Clear's exact reply UUID is in
`supersede/transcript-clear-complete.json`. The transcript projections retain
source hashes, native UUIDs, selected tool/result fields and exact correlation
markers; they never serialize arbitrary config or response dumps.

Recording parents are evidence harnesses, not retained production launchers.
Source copies in final rows are byte-exact retained sources; launch records bind
actual controller/plan/observer hashes. Final-source copying alone is not a
trace of every file opened at execution. SOURCE-V2's projector filename error
is retained alongside its corrected manifest and note; no product rerun or
retroactive seal rewrite occurred. Earlier rows remain immutable.

The current binary is 3,162,274 bytes plus 12,348 asset/README/license bytes;
archive1,349,048 bytes. One Go MCP checkpoint measured5,356KiB RSS, not peak or
a measured Node comparison. Full source, tests, held assets and Node reference
are counted in [INTERACTIVE-SIZE.json](INTERACTIVE-SIZE.json).
Lane L02/OB03 and end-to-end lane implementation remain unfinished.

## Current combined lane development

The ledger filename is retained for existing references. The same permanent
installation now contains both interactive exec and token-selected Go lane
worker modes; the Node reference and all historical seals remain unchanged.
Current installed source: `653b41ea6e73653ee167d18ce48bec20e7bbb271`.
Its artifact/install records are under
`/home/antst/sessionbus-evidence/claude-go-lane-install-653b41e/` on umka.

The [lane handoff](GO-LANE.md) owns the implementation boundary. The runtime
ledger below reuses the earlier native facts rather than re-discovering them.
Raw row root: `/home/antst/sessionbus-evidence/claude-go-lane-runtime-20260909/raw/`.
Exact per-row manifest hashes are appended to [first-contact.json](first-contact.json).

| Scope | Installed rows | Observed result / remaining limit |
|---|---|---|
| L02 idle worker group | ob03-first, l02-worker-death, first-open-cleanup | Native/MCP group bound; exact worker kill causes group pidfd exits. Later identified PIDs absent. Stale endpoint removal operator-owned. |
| OB03 native open and common forwarder | ob03-first, lfc-core | Native-generated ID, bound title/initialize/status gates, actual public list through installed forwarder; no separate raw init/status bodies. |
| Idle staging | lfc-core | One native marker user row/no assistant before explicit run; exact next-run marker, own public list; queued receipt. |
| Native caller permission policy | lfc-active, lfc-active-allowed | Default tool refusal preserved; caller exact allow rule admits held command. No adapter default or broad bypass. |
| Active admission | lfc-active-allowed, lfc-active-admission | Original queued label is wrong; corrected145cf92 returns injected after replay and before exact same-run marker. Following-run nonduplication observed in original row only. |
| Interruption | lfc-interrupt, lfc-graceful | Original aborted_streaming mapping defect fixed; later installed aborted_tools maps interrupted. Interrupted terminal alone is not tool-cleanup acknowledgment. |
| Config/resume/isolation | lfc-config | Invalid argv/cwd fail Open; simultaneous distinct lanes and real saved-ID resume; connected same-ID AlreadyConnected; closing one leaves others connected. Identical repeated model flags prove order, not precedence. |
| Forced active death | lfc-active-death | Worker/native/MCP group dies; tool and parent survive. Pending call gets no-terminal error. Operator release is separate. Generic containment remains open. |
| Normal close counterexample | lfc-graceful and cleanup supplements | Interrupt alone ends tool; subsequent close returns success but leaves tool/parent. Preserved source/seal; operator release ends survivors. |
| Corrected graceful lifetime | lfc-graceful-r2 | One prompt reaches exact Bash command but fixture socket110 bytes fails AF_UNIX path too long; no held-tool or interrupt credit. Idle EOF close succeeds. Corrected short-path repetition is in progress. |

LGO01–LGO06 source findings are closed by independently reviewed commits: shared
report placeholder semantics, actual Worker hello schema, drain/Run.Done
ordering, title gate, cancellation/write retirement and replay-time receipt
classification. The graceful correction separates native lifetime from completed
Open cancellation, keeps stdout/reports until native exit, and aborts cancelled
close or unexpected failure. Real Worker and compiled-child regressions pass.
Installed graceful tool cleanup and the remaining cancellation/forwarder and
combined interactive rows must still finish before full lane acceptance.

The whole combined footprint remains in [INTERACTIVE-SIZE.json](INTERACTIVE-SIZE.json);
its name is historical. It counts both modes, tests, active assets, linked
dependencies, and held/reference sources without treating moves as deletion.
