# OpenCode implementation and resource accounting

This inventory describes the completed rewrite at its accepted limits.
[ACCEPTANCE.md](ACCEPTANCE.md) records exact installed revisions and evidence.
Released native OpenCode remains unmodified. The implementation starts from
peers 9b2327a and preserves the historical product evidence separately.

| Source | Responsibility |
| --- | --- |
| cmd/opencode-peer/main.go; wrappers/opencode/interactive_launch.go, arguments.go | Public Go launcher, native selectors, direct-child lifetime, unique launch directory, authenticated native loopback topology, token Worker dispatch. |
| wrappers/opencode/opencode.go, http.go, client.go, run.go | Native lane new/resume, legacy synchronous prompt ownership, bounded HTTP/SSE/history, staging, native cancellation and terminal result materialization. |
| wrappers/opencode/lane_endpoint.go; wrappers/mcp | Private resident endpoint and existing shared MCP engine, authentic native session/message metadata, sole Worker Caller and bounded native ancestry checks. |
| opencode/server.mjs, forward.mjs, readiness.mjs | Native server hook, one resident connection, complete generic declaration, actual native call identity, startup readiness and joined close/cancellation. |
| opencode/tui.mjs, activation.mjs, owners.mjs, peer.mjs, delivery.mjs, gate.mjs, endpoint.mjs | Native TUI claim, host Solid selection, per-native-session Peer/Caller, title authority, unsent FIFO, bounded owner/connection/work lifetimes. |
| wrappers/opencode/install*.go; scripts/package-product; scripts/release/install-product | Go native config maintenance, complete permanent archive, owned-entry reconciliation and rollback. |
| internal/cmd/gen-opencode-tool; opencode/skills/sessionbus | Generic declaration generated from the shared protocol and one generic skill. |

## Native ownership

Go owns launcher/installer/lane functionality. JavaScript is required only for
OpenCode's in-process native plugin hooks, actual tool invocation context and
reactive TUI route API. Those hooks run in the product's existing Bun runtime;
there is no separate Node/Bun target installation or Go interactive broker.
The server and TUI run in separate native runtime contexts, so they communicate
over one bounded resident Unix endpoint rather than a shared module map.

The plugin uses native JSON-schema declarations and the supplied native client.
It ships only the pinned Sessionbus JS kit, with no plugin SDK, Zod, Effect,
Solid or other runtime dependency tree. Native provides the shared Solid
module. The same global specifier resolves separately through native
exports["./server"] and exports["./tui"]. Ordinary launches may load these tiny
entries and the globally visible skill, but create no Sessionbus tool, owner,
queue or listener. Installation uses Go to edit only owned native JSON/JSONC
entries; npm is a build prerequisite, not a target installer.

The TUI owns independent exact-native-ID Peers until native deletion or TUI
disposal, including previously selected sessions. An actual tool/subagent
context can establish its native session before route selection. Only route
selection elects the initial requested name, and publication awaits native
confirmation. Invalidated or retiring owners remain counted until joined;
late GET/hello completions cannot resurrect deleted owners. Each Peer uses
the kit's single Caller and reconnect policy. A native child tool in an
interactive session creates an addressable child Peer; a lane child uses the
verified ancestor's Worker capability instead.

A retained bus lane ID cannot also register as an interactive Peer, even after
close. Native resume may open its history while bus hello is rejected. Neither
native history nor the bus row is automatically reclassified. The installed
native-only quiet-resume case is separate and passed without aliasing or
forgetting a lane.

Managed TUI requests use native authenticated loopback HTTP because the native
embedded-worker RPC path drops AbortSignal. The launcher preserves nonempty
native auth or generates a password, rejects conflicting topology/pure flags,
and adds no process for this HTTP listener. HTTP abort joins the owned request;
it does not assert model cancellation. Native auth may reach native shell
children. Sessionbus activation variables are captured and scrubbed in both
native contexts. An empty exclusive claim and empty readiness marker live only
in the unique launch directory; a failed claim is never transferred.

The shared OpenCode/Kilo readiness wait subscribes to a process-local
BroadcastChannel before checking the marker. The TUI publishes a wake only
after its endpoint listens and the empty marker has been created and closed.
Every wake rechecks the marker; its payload grants no authority. Synchronous
subscription closes the startup window left by asynchronous macOS filesystem
watch registration. Late subscribers find the marker directly. Disposal closes
the subscription and joins the current check. This uses the native TUI/Worker
process boundary, at most eight existing server-instance subscriptions and one
transient publisher; it adds no dependency, process, polling or timer. Ordinary
inactive launches and lane mode do not create these channels.

## Lane and delivery boundaries

One shared Run owns a legacy synchronous POST /session/id/message, its active
deliveries, and one coalesced native abort operation. Native input persistence,
idle status and cancel acknowledgment are distinct from the final response and
joined Run lifetime. Active noReply:true is a handoff to native storage; written
does not establish model consumption. Idle staging remains unsent RAM until an
explicit Run; seeded wake uses the shared Worker policy. Failed attempted
writes are never restored/replayed. Default native permissions are preserved;
unattended permission/question requests are rejected through native APIs.

Terminal output is collected only within the admitted-user/returned-assistant
history interval, excluding native internal assistant summaries and later
answers. Native title/model/session summaries are not interchangeable with
the assistant's boolean summary marker. Close retires the integration and
retains native history and the daemon's resume recipe. There is no wrapper
history database, receipt journal or restart replay.

## Bounded resources and measured payload

Interactive ownership: 128 owners including retiring work; 16 pending identity
establishments; 256 owned HTTP requests. Each owner retains at most 64 unsent
messages/1 MiB, within a 16 MiB aggregate FIFO. The kit adapter shares a
256-task bound between delivery and tools, so a delivery burst can reject new
tool work. Resident MCP transport permits eight connections, 2 MiB ingress,
8 MiB response frames, 256 operations and 32 MiB retained payload. Native SDK
response parsing allocations are outside these wrapper bounds.

Lane HTTP has eight owned slots, including one reserved control slot, 64 KiB
headers, 1 MiB request/output and 8 MiB response limits. History reads use pages
of at most 64, a 4096-message/16 MiB aggregate limit and bounded cursor tracking.
Shared Go MCP/Worker budgets continue to apply.

[SIZE.json](SIZE.json), generated by [measure.py](measure.py), records every
checkout file's bytes, lines and hash, with a separate fixed-point self-size.
It separates Go runtime/tests, required native JS runtime/tests, installer and
skill assets, all selected main-module Go dependency files, selected external
Go dependency sources, the complete shipped JS kit, archive members and the
actual permanent inventory. Categories overlap and must not be summed.

The native-acceptance 502 Linux archive measured 2,268,830 compressed bytes and
31 regular members totaling 5,384,199 bytes. Its Go executable was 5,230,754 bytes.
The final documentation archive is measured separately in SIZE.json; runtime,
plugin, kit and pins remain byte-identical to 502. The executable includes linked
Go runtime/standard-library cost. Both permanent command symlinks and the native
OpenCode executable are counted once by resolved target. Native OpenCode 1.18.30
was 184,825,984 bytes at capture, plus its 825-byte package manifest; these are
existing product costs, not new wrapper dependencies or a full native package
installation measurement. Native MCP descendants from unrelated existing
configuration are not Sessionbus runtime additions.

Build-only npm selects the 14-file own plugin package and installs its sole
immutable JS kit dependency. The complete nine-file kit is counted, including
its schema, types and license. Go SDK remains 8cc6a59; JS kit is 0b35c99. Dependency
source counts select Linux go list files rather than whole module caches.
The measurement records compiler/target, archive hash and observed native
version. It does not estimate native Bun's shared memory or claim measured
idle/peak RSS. One native loopback listener plus the resident Unix bridge adds
no process to the existing native TUI/worker topology; one Go launcher or lane
Worker owns its direct native child.

Normal exit, TERM and HUP join the launcher's direct child and remove owned
resources. Abrupt launcher SIGKILL can leave the native TUI, live Peers and
stale directory; no extra supervisor is installed. Native plugin descendants
and explicit operator cleanup require separate identified evidence. Capacity
limits are not measurements of idle/peak memory or blanket process containment.

Actual acceptance observed normal quit and TERM/HUP resource removal. In the
KILL PTY row, native processes were also later absent, but the unique directory
survived and required explicit operator removal. That row does not prove the
killed launcher joined native or that native always exits with it. Original
native history survived all quiet resumes; only the selected empty fork was
deleted. Default-model failure and successful explicit native model requests
remain distinct in the acceptance index.
