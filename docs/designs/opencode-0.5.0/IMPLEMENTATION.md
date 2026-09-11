# OpenCode implementation and resource accounting

This is the rewrite's current source inventory, not final acceptance clearance.
[ACCEPTANCE.md](ACCEPTANCE.md) records installed revisions and outstanding rows.
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
Shared Go MCP/Worker budgets continue to apply. The final source measurement
must include selected non-standard-library dependency files, tests, retained
source and the complete checkout, not just new runtime files.

The sealed installed ee7381a Linux archive is a historical measurement:
2,269,367 compressed bytes, 31 regular members totaling 5,373,533 bytes, of which
the Go executable is 5,222,562 bytes. Its plugin directory has 25 regular files,
140,656 bytes; the ten own native JavaScript modules total 54,071 bytes and the
nine kit files total 68,326 bytes. Overlapping categories are not additive.
The Go executable includes linked runtime/standard-library cost. Source and
JS bytes do not measure native Bun's shared code or live RSS.

Those values bind implementation/dev1-kit-0b35c99/BUILD.json under the external
OpenCode architecture packet. They are not the pending corrected archive's
size. Final SIZE.json must be generated from the corrected archive and exact
checkout, with its own byte size recorded separately from hashes, and compared
with the real permanent file inventory. No final total or runtime-equivalence
claim is stamped while correction/acceptance work remains open.

Normal exit, TERM and HUP join the launcher's direct child and remove owned
resources. Abrupt launcher SIGKILL can leave the native TUI, live Peers and
stale directory; no extra supervisor is installed. Native plugin descendants
and explicit operator cleanup require separate identified evidence. Capacity
limits are not measurements of idle/peak memory or blanket process containment.
