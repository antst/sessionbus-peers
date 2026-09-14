# Codex interactive implementation selection

2026-09-09. This selects implementation from peers `f5dd343` and the accepted
shared kit `d11fa743`, using the retained native `3d2ee51` source and earlier
release evidence. It does not claim an installed result. LANE-SELECTION.md owns
the common package and lane; REQUIREMENTS.md remains the requirement register.

## Decision and stated cost

Use a per-launch Go broker between the native TUI's Unix WebSocket connection
and a native App Server's single-client stdio connection. The launcher starts
the broker and then execs native Codex; native Codex still owns fresh selection,
picker, resume and fork. The broker never precreates or selects a thread to make
attachment easier. It owns one bus Connection/Caller for each settled, loaded
native thread on its own server. Native-kept MCP children are stateless tool
forwarders, as in the selected lane, not second bus owners.

This costs one broker process, a Unix WebSocket listener, one MCP control
endpoint, bounded bidirectional RPC routing, an OS parent-exit watch and a native
stdout/stderr drainer. It is not a byte-only relay. Both private entries belong
to the same compiled Go artifact and common plugin. No extra daemon service,
timer, poller, replay queue, native session lock or result store is added.

| Alternative | Earlier evidence / reason not selected |
|---|---|
| Shared App Server + precreated thread | R1 used it and exact legacy zero-turn attachment is already proved. It does not preserve native picker/fork/arbitrary selection without implementing selection ourselves. |
| Ordinary embedded native TUI + hooks | The scoped native audit found boundary context delivery but no external idle wake ingress. Next-prompt-only delivery fails the owner's idle-wake requirement. |
| Native per-launch Unix listener | Native supports it, but has no unconditional readiness signal or client-bound exit. Moving its scope alone does not solve those lifetime edges. |
| Native dynamic tools | Native start can add the tool; arbitrary existing resume/fork cannot. Do not split tool behavior by how a thread was originally created. |
| Broker + MCP-owned bus identities | MCP startup receives no fresh native thread ID. Broker-owned native thread events remove child attribution and reverse native-call forwarding; no PID/name/cwd matching is needed. |

## Native CLI boundary: explicit requirement amendment

The wrapper consumes repeated `-g`/`--group` forms, the established initial-name
`-n`/`--name` option, and supported aliases before native `--`.
It translates `--resume` to the native resume
subcommand without resolving the selector, and `--yolo` to the native option.
Native remaining argv retain order and bytes. The documented managed activation
and broker `--remote` arguments are prefixed, never appended after a caller's
`--`. A caller-supplied `--remote` conflicts with this owned transport and gets
an explicit integration error explaining that conflict; it is never ignored.

Mirror native config override occurrences (`-c`, `--config`, including attached
value spellings) in order to App Server's native CLI while retaining them on the
TUI. Recognizing this one native option's value is a bounded exception to the
no-native-arity-table rule. Do not parse/reorder TOML, consume a separate
option-looking token as a value, scan past native `--`, or mistake literal RHS
text inside a value for another option. Preserve native validation failures.
Define and test the supported spellings rather than claiming an arbitrary
native argv extractor; do not add a table of other product flags.

Every other native flag goes to the TUI. Its remote thread API forwards the
settings native Codex chooses to project; the wrapper does not recreate that
projection. In particular, **profile flags cannot be mirrored to App Server**:
native `cli/src/main.rs:1825–1853` rejects profile for that command, whose startup
at `:1288` uses default LoaderOverrides. Preserve `--profile`/`-p` on the TUI and
document that server-level profile settings, arbitrary plugin/MCP transport
changes, and other settings omitted by native remote projection do not gain full
standalone equivalence. The earlier suggestion to pass profile to both processes
is withdrawn on this source fact. Do not read/merge profile files ourselves or
invent a native override to conceal this limit.

## Process and protocol ownership

The broker is the launcher's direct child in its own process group. Before
starting App Server or announcing ready, bracket Linux pidfd / Darwin kqueue
registration with actual-parent checks, then arm event handling. The launcher's
exec keeps that process identity. Parent exit begins shutdown; no numeric PID
polling or retained-parent TUI supervisor is needed. Darwin parent-watch behavior
requires the controlled OS-process test on Darwin; do not claim it from Linux.

The broker owns the only App Server stdin writer and the TUI listener; neither
TUI nor product children inherit duplicates. Bind/start accepting before ready.
Forward the TUI's one initialize/initialized exchange; internal calls wait for
its success without a second initialize. On parent/transport exit stop new
admission, close native stdin, continue drain, wait/reap native, and remove owned
endpoints. Native EOF supplies thread/MCP shutdown without history archival.
No outer adapter timeout is selected by this document. Kernel closure on broker
death also closes the sole native input; actual installed death acceptance must
state what exits rather than infer reaping or graceful completion.

Route JSON-RPC string/integer IDs in both directions with bounded live maps,
preserving opaque payloads except required ID references. Forward native
approval/elicitation/tool requests to the TUI and restore their IDs, including
`serverRequest/resolved.requestId`. No automatic approval or synthetic generic
cancel RPC. Locally cancelled internal requests retain their correlation for
drain; successful native admission never gets replayed. A blocked public call
must not occupy the native stdout reader. Use a complete Go WebSocket transport
with native-compatible message bounds, control frames and fragmentation; the old
client-only 64MiB helper is not a server implementation. Account any dependency
and all runtime files in the whole package size.

## Native identities and public tool

Create a resident owner only from exact settled native start/resume/fork results
or thread events on this owned server. Native IDs/names are authority. Match
required MCP startup status before presenting tool-ready integration; keep early
status against the exact thread, not helper spawn order. Native confirmed rename
updates the same owner. The integration initial-name option names
only the TUI's correlated initial selection through native name/set, outside an
in-band tool callback; do not name every subthread or invent fallback names.

The broker holds the resident owner/Caller. MCP forwards native `_meta.threadId`
and complete call metadata through the one launch endpoint; the broker resolves
only an already known loaded native ID. Unknown/missing identity fails without
creating/loading/resuming a thread. Initialize/tools-list need no identity.
Extend the generic tool boundary explicitly to preserve this native metadata;
do not put model-supplied identity in action arguments. Reuse action validation,
cancellation and shared non-consuming status/wait plus ack semantics.

Presence can begin before a prompt from native events; it does not wait for the
first public tool call. A cleared thread that remains loaded retains its row.
Exact native thread/closed withdraws its row; whole server loss withdraws all.
Each row owns one connection, so daemon same-ID supersession still applies. MCP
forwarder EOF has no bus connection of its own to reopen or retry; record how
native startup/runtime failure affects tool availability and test its installed
behavior. Arbitrary native tool descendants remain outside the requested scope.

Idle delivery uses native turn/start; active delivery uses expected-turn steer.
These are interactive product turns, not daemon lane cursor entries. Admission
receipts follow the same exact native acknowledgments and no-replay correction
as the lane. Shared persistence/auto-archive applies to daemon lanes only.

## Implementation and acceptance

Dev1 continues lane/native transport/common package. Dev2 implements broker,
launcher option handling and OS watch in a separate branch from the common
checkpoint; coordinate exported native transport interfaces instead of editing
the same files concurrently. Suggested ownership: new broker/mux/watch/launch
files and their tests for dev2; current app/codex/lane endpoint and common MCP
boundary for dev1. Integrate into one runnable common installed package; no
parallel private installation. Root reviews immutable checkpoints and combines
the final PR scope.

Controlled tests first cover ID/reference translation, concurrent tool/receipt
progress, config spelling and `--`, parent exec/death, sole-writer EOF and
startup failure. Then use real umka installation for native no-input readiness,
ordinary isolation, skill/public tool, groups, resume/picker/fork, idle/active
delivery, native permissions, normal/death cleanup and an actual native Codex
parent operating a Codex lane. Reuse the already sealed native facts and shared
policy tests; these are composition/installed acceptance, not rediscovery.
