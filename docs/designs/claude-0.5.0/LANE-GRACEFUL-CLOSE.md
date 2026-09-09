# Graceful close: native lifetime boundary

2026-09-09. Architect selection for stamp, responding to fable delivery30b4b9309e204df0b0a8550c03e85a56. No generic containment guarantee is granted by this correction.

## Evidence and source finding

Installed145cf92 lfc-graceful has 19 verified manifest entries. Explicit interrupt yields aborted_tools/tool_use and the first tool/parent are absent at the later checkpoint. The following held-tool session.close yields the same interrupted terminal at08:41:28.608 and close success at08:41:28.632, yet the second tool1734911/parent1734909 survive at08:41:47.566. Interrupted terminal is not a descendant-cleanup acknowledgment. The difference is consistent with premature native termination, but the seal does not capture the internal causal ordering of Claude's tool cleanup.

There are three premature-termination routes to fix together:

1. Public Go Worker.close cancels w.context before product.Close. Open receives w.context, and Claude derives exec.CommandContext from it. Go's default CommandContext cancellation kills the native process; this can happen before product.Close is entered.
2. Claude.Close explicitly cancels that context, stops both pipes and closes the endpoint before waiting.
3. Reader EOF, native SessionEnd and forwarder End can call fail(), which cancels native even when normal close is underway.

Therefore replacing only the body of Close with stdin EOF is insufficient. This is an adapter process-lifetime ownership correction; the SDK's operation cancellation ordering need not change.

## Selected correction

The adapter owns native lifetime independently of the completed Open operation. Preserve Open context values but do not make successful native lifetime depend on future cancellation of that operation context. Startup cancellation must still abort the native process immediately while Open is pending, and failed Open must use unsuccessful-start cleanup rather than the new graceful-close path. Stop the startup cancellation binding when Open finishes; no background retry/watch loop, timer or extra process.

On normal product.Close with a live close context: mark closing, close native stdin once, keep the native stdout reader and report endpoint available, drain until EOF and wait for the directly owned process to exit. Expected SessionEnd/forwarder EOF/stdout EOF during this path must not cancel or kill native or prematurely close the remaining shutdown transport. After native exit, finish stream/endpoint cleanup and release native lifetime resources. Do not report success merely on stdout EOF or a run terminal.

On failed startup, unexpected integration failure, or cancelled close context/bus loss, retain explicit abort and truthful settlement. While waiting for graceful native exit, cancelled close context must unblock the wait and take the abort path. No new grace interval: existing daemon closeBound remains its existing outer limit. No fallback retry of Close, no delayed signal, no fabricated terminal. Scope normal-close exemptions to expected shutdown events; do not allow a later native report to reopen a closing lane.

The required implementation tests use controlled events and real public Worker sequencing: kit operation-context cancellation before Close must not kill a successfully opened native child; startup cancellation still aborts; stdin EOF precedes native exit; an interrupted terminal or stdout EOF alone cannot finish Close; expected report/forwarder shutdown cannot force a kill; close-context loss aborts and settles pending work; reader drain/Run.Done ordering remains intact. This is the same process/endpoint ownership, not a second supervisor or copied Worker.

## Reused facts and remaining installed proof

NP02 direct native initialization followed by stdin EOF ends native without a model turn. FP05-266 closes only the real child stdin after its completed turn and records native exit0/reaping. These existing rows justify EOF as a concrete native exit route. The older SDK EOF-active row includes a subsequent SDK close and is not stronger evidence. R4 ff81565 persistent native stream/native interrupt is precedent; its exact child cleanup did not prove interrupted-tool completion before forced termination. Do not reopen the historical discovery campaign.

Implement this correction, replace the same umka installation and repeat the specific held-tool normal close acceptance on the corrected build, preserving the original counterexample. Record whether native exits without an adapter signal and whether the identified tool and parent die before the close result. The correction is consistent with LANE's truthful close/EOF requirement and kit sequencing, but sufficiency remains an installed acceptance question.

## Generic containment position unchanged in direction, stronger in scope

LANE-PROCESS-OWNERSHIP.md remains the proposed generic bus approach. Native graceful exit is the preferred normal lifecycle; kernel-owned containment addresses forced termination and any native cleanup omission. Neither a terminal nor a clean parent exit proves every descendant stopped. The cgroup decision therefore covers failed ordinary close as well as hard worker death. No adapter PID registry, process scan, kill list or timer is introduced; the generic host/platform decision remains separate and must not silently weaken the advertised cleanup guarantee.
