# Claude lane delivery amendment — LGO06

2026-09-09. Fable delivery `af4900f8e96aeec537b785361449c51f` accepts the receipt correction and separate active-tool cleanup scope. This amends the delivery paragraph of LANE.md; the original stamped file remains unchanged. No wire or daemon change.

## Receipt boundary

An exact native append replay, matching both the pending message UUID and captured native session ID, is necessary for either successful lane receipt. A completed write alone proves neither.

- Return `injected` for native admission observed during the same confirmed active run. This acknowledges admission only: it does not promise consumption, terminal membership, or a particular assistant reply.
- Return `queued_for_next_turn` for the demonstrated idle native staging path, for a later explicit run. Idle at one later observation is insufficient to classify a message submitted during work that has since ended.
- If a run starts, ends or changes across the pending admission and the scheduling classification is not established, use the existing `ProtocolError(-32603)` uncertainty path with diagnostic string data. Do not manufacture rejection or default to a future-run receipt. No retry or replay.

Evaluate classification at the native report in the serialized reader, using pending message/session correlation and existing run events. Retain that result for the waiting delivery; do not recalculate it from later worker state. Terminal UUID membership is not a prerequisite for an admission receipt, and no terminal wait is added. Cover replay before/after a terminal, idle submission crossed by run start, replacement-run ordering, wrong UUID/session, and write-only failure with controlled events. No queue, timer, new native run, history lookup or replay counter.

The signed UNIVERSAL-SESSION-PROTOCOL.md message.deliver contract already reserves `injected` for identity-bound native admission and `queued_for_next_turn` for demonstrated staging for a later explicit run. This corrects the Claude adapter's classification to that contract.

## Evidence and historical comparison

Installed 7f40b24, `claude-go-lane-runtime-20260909/review-copy/lfc-active-allowed`: the message was sent while a Bash tool was held; an independent release preceded its replay-based receipt. The current run returned the exact message marker, and the following explicit run returned only its own marker. The recorded `queued_for_next_turn` receipt is the defect; preserve the seal and that label as observed.

Earlier R4 (ff81565a252151575409efad2fccfd6d5e544383) supplies persistent native stream precedent, but not this shouldQuery/UUID receipt boundary. Retained SDK idle-append evidence supports idle staging; the old circular active-append harness proved no active admission classification. LANE.md generalized that staging wording too far, and 7f40b24 applied it to every append. The new installed active evidence closes the distinction without reinstating R3/R5 queues or modifying the daemon.

## Active tool lifetime scope

The same row places Bash tool PID/pgrp 1730226 outside worker pgrp 1729921. A signal to the worker process group does not directly signal that tool group. The prior L02 result remains an idle-worker result. Normal completion and cleanup here do not prove cleanup of active tools after worker death.

The remaining installed active-worker-death row must record what native Claude does to its own tool children under that stimulus, including survivors. No adapter ancestry registry, PID scanning, kill list, timer or additional daemon is authorized by this finding. Keep the result and its disposition separate from L02, and continue on the same real umka installation.
