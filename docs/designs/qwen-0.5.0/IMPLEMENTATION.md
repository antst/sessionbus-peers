# Qwen implementation and resource inventory

The rewrite starts from peers a9a0f42 and reuses retained v0.1–v0.4/pre-split
native knowledge. Installed lane correction b047ce0 and combined interactive
68e0407 are bound by [EVIDENCE.json](EVIDENCE.json). Released native Qwen remains
unmodified. The exploratory upstream metadata patch was not adopted.

| Files | Responsibility |
| --- | --- |
| cmd/qwen-peer/main.go | Public native launcher, token Worker, same-binary private basename dispatch; native SIGINT stays native, TERM owns cleanup. |
| wrappers/qwen/qwen.go, acp.go, run.go, delivery.go | Native ACP new/load, one shared Run, strict relevant output decoding, unsent FIFO, native pull, seeded write receipt, joined cancellation and close. |
| wrappers/qwen/lane_endpoint.go, forward_stdio.go | One endpoint supplied to one native ACP session, sole Worker Caller; helper initialize/EOF, bounded full-duplex forwarding and pollable owned stdio. |
| wrappers/qwen/interactive_launch.go, interactive_config.go, interactive_name.go | Native selectors/argv, unique launch resources, bounded one-argument native MCP config composition, literal native initial naming. |
| wrappers/qwen/interactive_owner.go, interactive_events.go, interactive_records.go | Helper-owned Caller, empty exclusive claim, initial native identity/registry gate, FIFO drain, confirmed native titles, append-only inbound delivery. |
| wrappers/qwen/interactive_process_*.go, interactive_watch_*.go | Live process/start/ancestry and OS event watches, parent/launcher loss and joined cancellation. |
| wrappers/qwen/package.go, qwen/, scripts/package-product, scripts/release/install-product | Permanent absolute sibling helper, one binary/alias/skill, skill-only ordinary extension, native install and owned-payload reconciliation. |

## Native ownership and receipts

Interactive uses one Go launcher and one native MCP Go helper. Native Qwen owns
its normal runtime/processes; no extra wrapper relay/supervisor or Node adapter
is added. The helper has one shared Caller/connection. A lane has one Go Worker,
one native ACP child tree and its stateless MCP helper; no second Caller exists.

A lane adopts native new/load identity. Only its configured session receives
its private endpoint; helper bootstrap environment is not lane identity.
Optional native invocation metadata is checked when present, not fabricated
or required on ordinary untrusted ACP prompts. Helper initialize response
proves startup, not completed native discovery/model exposure. Native ToolSearch
and existing permission/config filtering remain active.

One shared Run owns one native session/prompt request and its admitted cancel
operation. Seed ReportDelivery(written) occurs after the complete request write,
outside reader/mutex. Active craft/drainMidTurnQueue serves at most ten messages
per native pull. A completed response write is written, not injected/consumed.
Native late recovery may consume handed-off input during a later explicitly
requested turn; the wrapper never replays it. Truly unsent input remains staged
in RAM at terminal. Pre-write refusal retains staging; any attempted handoff
is not replayed. craft/cancelPendingPrompt and the original prompt response are
both joined before ownership ends. Native outcome/reason remains distinct from
an interrupt acknowledgment. Run.Done governs subsequent admission.

Close retires the worker and native process. It does not archive/move/delete
native history, or retain a result after Worker retirement. Daemon policy owns
persistence, owner loss, grace and idle stage/run independently. There is no
wrapper database, receipt journal, restart recovery, session lock or Handoff
replay. Native history is native-owned. The interactive title observer reads
native manual custom_title records only for title authority, not result storage.

## Initial interactive binding and limits

The first validated helper reserves an empty O_EXCL claim in its unique launch
directory, then matches native env ID, first native session_start and a live
same-launch registry/process identity. The registry orders normal watcher
construction; caught native watcher-init failures prevent a universal health
claim. Linux uses native PID/start/namespace tokens. Darwin native null tokens
are qualified by live sysctl identity and registry timestamp; no native Darwin
execution is claimed.

Initial names use native literal `/rename --`, ECMAScript whitespace
normalization and the native 200 UTF-16-unit limit. A new matching native manual
title is required before publication; prior history does not confirm a new
rename. Empty title remains empty. One helper/claim prevents rename replay or
ownership transfer after failure/restart. The native updater-relaunch case was
observed and requires a fresh wrapper launch.

Interactive integration remains bound to the initial session. After native
/new, /clear, /resume or another in-process switch, outbound MCP identity can
remain original while input-file messages reach the displayed session. The
wrapper does not enforce a switch ban or claim race-free withdrawal. Exit and
launch/resume a new qwen-peer process for a supported Sessionbus session change.
This limitation is accepted for the inspected released interfaces; ACP lanes
remain isolated per native session.

Ordinary Qwen sees one generic skill and starts no Sessionbus MCP helper.
Managed interactive adds its server through one --mcp-config, preserving one
caller config/server fields and argument position. Files use native comment
rules; inline JSON is strict. Duplicate flags, explicit sessionbus collisions,
invalid input and over-limit input fail. Native camel aliases share those
boundaries. Existing settings/extensions and explicit permission argv remain
native-owned; no global server toggle, implicit grant or alwaysLoad override.

## Bounded resources

ACP frames/output are 1 MiB, pending/request work 256 and aggregate retained
payload 32 MiB. Unsent lane staging is at most 256 entries/128 KiB. Native pull
selects at most 10. Shared MCP engine bounds remain frame 2 MiB, work 256,
retained payload 32 MiB and encoded result 8 MiB.

Interactive caller/combined MCP config is at most 64 KiB UTF-8. Native event or
history records and each submitted input record are at most 8 MiB; incoming
bus handler work 32, watched paths 8. FIFO has one joined reader: initial identity
then output drain/discard, no duplicate answer file. Its RDWR/nonblocking
descriptor is explicitly registered with Go through NewFile, because Darwin
OpenFile excludes FIFOs from its poller. The held writer prevents dependence
on external writer EOF; process watches and owned Close govern lifetime. The native output bridge's
own 1 MiB buffered-stream self-disable is not a wrapper bound.

The native input file is append-only for the live launch, with no aggregate
byte ceiling. It is never truncated or rotated without native consumed-offset
proof. The empty claim contains no session/result data and is never recovered
by a future launch. Native-owned title history can be scanned with bounded
memory; total history length is not a wrapper storage budget.

Owned stdin/stdout normalization adds two pollable duplicate descriptors. Linux
adds inotify, two pidfds and a cancellation pipe; Darwin uses kqueue, a
cancellation pipe and bounded vnode descriptors. FIFO and bus connection add
one each. Native history/input file descriptors close after each bounded read
or append. Capacity limits are not measured idle RSS. SIZE.json records source,
linked/archive/installed payload and dependencies; no new runtime module was
added by Qwen.

Normal exit/TERM joins launcher-owned native work and removes resources.
Helper EOF or parent/launcher loss cancels and joins its Caller/actions/watches.
A killed launcher cannot promise reaping or file cleanup: the installed SIGKILL
row withdrew presence and left a stale runtime directory, removed separately
by the operator. Process absence in that terminal setup does not prove a killed
launcher performed cleanup.
