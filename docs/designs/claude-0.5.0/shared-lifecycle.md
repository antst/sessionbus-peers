# Shared lane lifecycle adaptation

Preparation from peers develop `9c034132d00a3fda630a5200bbb91b8e30594f08`.
The SDK pin `a7c10044f3b4e5dba552fbb50fedfd50e84767fe` is an immutable
**compile-only checkpoint**, not an independently accepted daemon/install pin.
No shared-lifecycle installation or native acceptance is claimed here.

Authority: 2026-09-09 `codex-architecture-20260909/persistence-repair/SELECTION.md`,
`DEV2-BOUNDARY.md`, `CONTRACT.md` and `ACCEPTANCE.md`. This is a shared lifecycle
repair followed by Claude acceptance; Codex design remains paused. Earlier
native evidence and original receipts remain unchanged.

## Product and shared boundaries

The common Worker owns one Run, its stable lane/run reference and the only
ordered output cursor. The daemon owns authenticated owner lifetime, independent
persistent/automatic-close/idle-message/notification policy and the named
post-terminal deadline. Claude adds no output store, scheduler, timer or replay
queue. Unsupported products advertise no waking-run capability and must reject
that policy before usable Open; defensive callback rejection is not the primary
capability gate.

`RunInput.Text` uses the existing native query path. `RunInput.Delivery` renders
the same source/body envelope as stage/active delivery and uses that same path
with `shouldQuery:true`. Its one fresh native UUID remains separate from the
shared run reference. Exact session/UUID replay fixes `injected`; receipt writing
occurs in the Run goroutine, outside the native reader/state mutex. A subsequent
terminal cannot revoke observed admission. Pre-submission failure rejects;
possible submission without native admission is uncertain. Receipt transport
failure retires the native transport. Shared shutdown must settle that write.

Staging remains `shouldQuery:false` and `queued_for_next_turn`. Existing active
append classification and uncertain run-boundary behavior remain unchanged.
No-input Open, PID/title/tool gates, graceful Close and launcher option grammar
are preserved. Claude advertises `supports_message_run:true`; the other wrappers
only adopt the callback signature and retain explicit-text behavior.

## Collection and notification

Public start/status/wait/ack references now contain `session_id` and `run_id`.
Reads are non-consuming and independent of the originating Caller. Ack commits
oldest-first consumption; a submitted successful ack retains its response even
after request cancellation. Cancellation drains the original wire correlation
without invoking its abandoned observer/target. Worker loss/retirement invalidates
unacknowledged output; no recovery journal is introduced.

The generic installed skill/tool explains independent policy defaults and costs,
completion pointers versus answers, explicit collection and acknowledgment. A
real Claude parent receiving a completion pointer is a required installed row;
a controller-only send or resident MCP event is insufficient.

## Controlled regression mapping

- Native replay, foreign replay, query flag, terminal and receipt ordering:
  `wake_test.go`. A blocked reporter transport does not block the native reader.
- Actual public Worker/Claude callback, source envelope, delivery receipt,
  `turn.ready`, repeated status and explicit ack: `wake_worker_test.go`.
  The additional report gate delays the call to actual `Run.ReportDelivery`,
  not an assertion that the kernel Unix socket write was blocked. Separate
  stream tests cover blocked reporter I/O; the actual Worker test closes the bus
  at that call boundary and requires callback/Worker termination.
- MCP cancellation, late valid reply drain, non-consuming reads and a fulfilled
  response after cancellation: `interactive/mcp_test.go`, `owner_test.go`.
- Actual Worker terminal metadata precedes EOF shutdown; output does not survive
  the Worker: `claude_test.go`. Prior native EOF/drain/Close tests stay separate.
- Mechanical other-wrapper tests use real `turn.execute`/`turn.ready`/read APIs.
  Adapter errors without native terminals are unavailable. Oversized output is
  explicit validation failure, replacing the old kit's truncated success.
  These changes do not grant new native capabilities to another product.

Remaining acceptance: exact shared-wire review and final pin, paired permanent
umka daemon/Claude replacement, independent policy combinations, actual parent
notification/collection, collector replacement and normal-close/interactive
regressions on that same changed installation. All earlier seals are retained.
