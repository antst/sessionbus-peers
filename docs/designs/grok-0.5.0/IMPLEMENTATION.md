# Grok implementation inventory

Baseline eb727701; runtime/package checkpoint e24f9ca. The accepted native
facts remain in the historical product ledger and external archaeology packet.
CONTRACT plus the three amendments select the new behavior. They are copied
unchanged; later evidence qualifies installed claims without rewriting history.

| Files | Responsibility |
| --- | --- |
| cmd/grok-peer/main.go | Public launcher, token worker and private basename dispatch; no public MCP subcommand. |
| wrappers/grok/acp.go | Bounded duplex ACP, registered correlations, cancelled response drains and actual submission hook. |
| wrappers/grok/grok.go, continuation.go | Native lane open/run/seed/stage/interrupt/close, primary ordered events, original Run ownership through native continuation, bounded output, failure retirement. |
| wrappers/grok/lane_endpoint.go | Resident native MCP forwarder over owned Unix socket, actual helper readiness/identity/loss, sole Worker Caller, joined EOF cleanup. |
| wrappers/grok/peer.go, peer_owner.go | Native argument projection, private leader/TUI/quiet hold, per-session native helper/observer, exact leader activation, empty one-launch name claim, same-connection title publication. |
| wrappers/mcp/sessionbus.go | Existing generic engine plus inert capability, response/frame/work bounds and independent EOF shutdown. Claude/Codex native identity policy remains in their existing adapters. |
| grok/ and scripts/package-product, scripts/release/install-product | One binary/private alias, one generic skill, permanent native plugin inventory installation and owned stale-payload replacement. |

Interactive topology retains one Go launcher, native private leader, native
quiet hold and native TUI; each native session adds one Go MCP owner and native
observer. /new keeps earlier resident owners. Lane topology has one Go Worker,
native leader/primary/observer and one Go MCP forwarder. Other native-config
MCPs are visible in process captures but are not this package's dependencies.
Ordinary native Grok starts the inert Go helper because global registration is
the available native leader route; it creates no Sessionbus owner or observer.

Stage retains at most64 never-submitted messages/1MiB, in worker memory only.
ACP frames/output are bounded1MiB, pending correlations and continuation
segments256. The common MCP engine bounds frames2MiB, response work256,
retained payload32MiB and encoded result8MiB. Actual process observations and
linked/source/dependency/installed costs are recorded in SIZE.json; maxima are
capacity limits, not measured idle RSS.

No native ID resolver, generated session identity, session lock, Handoff
replay, reconnect loop, wrapper database or receipt/result journal remains in
the Grok path. The empty name claim is a temporary resource in a fresh launch
directory, never session metadata or recovery input. Native history is owned
by Grok. Native permission switches stay explicit and ordered; omitted policy
stays omitted, with no blanket MCP allow inserted.
