# Go migration of the reviewed interactive candidate

Behavior reference: sessionbus-peers `964434895d93232a71883cb55785c1605014a5e8`
(merged as `833885a`). The byte-exact Node implementation/tests/assets are
retained in `node-reference/` and excluded from the installed Go archive.
Native facts and prior counterexamples remain valid at their recorded versions;
this migration does not reset discovery or claim unfinished lane acceptance.

| Reference | Go implementation |
|---|---|
| main.mjs | cmd/claude-peer/main.go + wrappers/claude/interactive/launch.go |
| owner.mjs | wrappers/claude/interactive/owner.go |
| mcp.mjs | wrappers/claude/interactive/mcp.go |
| tools.mjs | wrappers/claude/interactive/tools.go |
| delivery.mjs | wrappers/claude/interactive/delivery.go |
| npm payload | scripts/package-claude + claude native assets + literal README archive recipe |

The shared Go kit is pinned to merge `f8d409e98218` (module revision
`v0.1.0-pre.2.0.20260909061716-f8d409e98218`), with the reviewed contents of
`fc18c7e4a54efaf8869dda2e7c058ba372b2bb8f`.
Its public Connection/Request/NewConnection exposes the existing validated
RPC, including acknowledgment-before-next-frame ordering. Caller.Action uses
the reviewed WaitContext cancellation; the adapter has no result cache or
private Caller state. The Go binary links no Node runtime or held lane backend.
The held lane source remains in wrappers/claude for the subsequent lane step.

## Explicit translation difference: overlapping report initiation (G01)

Go DialContext and connection Begin can block, unlike Node socket initiation.
The initial efcd86b checkpoint held the input reader across these operations;
review found that this prevented EOF/SessionEnd handling during a stall.
The corrected implementation applies validation, identity generation and
SessionEnd in input order, then performs cancellable transport I/O separately.
End never takes the transport submission mutex or waits for dial/write.

A report superseded before its hello reaches submission returns a local
not_connected/unavailable result and cannot terminally close the current owner.
The latest live report still initiates its hello without another prompt.
A transport submission mutex protects the generation check and ordering of
actual Begin calls. An older submitted hello can complete before a newer one;
its acknowledgment cannot admit an obsolete generation. It cannot be written
after the newer hello. No report replay, adapter message queue or retry is added.
Intermediate native title publication is not guaranteed. This narrow change
was accepted in astra delivery-3956a9ad9ef00d60a668ed22780d4f20.

Controlled tests cover blocked dial and hello-write with real Serve EOF,
SessionEnd and external End; only cancellation/connection closure releases the
transport. Separate overlap tests cover obsolete work during dial and an
already-submitted old hello preceding the latest hello. The asynchronous port
also checks connection Context directly before accepting a later report, so a
closed connection cannot be revived before its watcher goroutine runs.

C01 tests use the actual public Caller/Connection: cancel then status or wait,
result retained, and fulfilled consuming wait retains its MCP response despite
later cancellation. A status arriving before the Caller's settlement goroutine
may truthfully return running, followed by one explicit wait; no polling or
private completion state is used. C02 tests validate public send/spawn fields
against the real wire and preserve model-visible guidance. C03 archive tests
assert one active skill and no Node or held-lane payload.

The Go native carrier treats any failure after attempting Write as uncertain,
including a short write. It does not infer a pre-write EOF from an error returned
by that Write. Successful full local Write is written only, never native ack.

## Installed accounting

The first runnable efcd86b archive was installed on real umka and version
passthrough was checked. Evidence is under
`/home/antst/sessionbus-evidence/claude-go-install-efcd86b/` on umka. That checkpoint
includes G01 and is not final acceptance. Corrected76b921b and a056303 have separate installed artifact bindings. The
[installed ledger](../../../probes/claude/interactive-candidate.md) records
FC01–FC08 observations and retained limits; the a056303 candidate remains
installed for owner manual testing. The Go kit source/module sums are bound in
[GO-KIT-BINDING.json](GO-KIT-BINDING.json); KIT-BINDING.json remains the historical
Node dependency record. Lane remains a separate unfinished step.
