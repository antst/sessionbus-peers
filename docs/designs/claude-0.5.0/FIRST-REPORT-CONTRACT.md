# Interactive first-report publication — current stamped contract

Authority: fable delivery `delivery-5ee640db9140e1d13a3f2dc9ebec402e`, 2026-09-08. This replaces FP-C01. The owner has been told and may veto; the relayed stamp stands without a veto. No implementation release follows.

> Interactive presence begins only when the resident owner receives a usable native identity report through an approved native report event. Before that report, the session is absent and unaddressable. No launch, first-prompt or first-turn publication deadline is guaranteed. A successful Stop report may establish identity with no name under BN01; a subsequent native title report may supply the name. SessionEnd withdraws matching presence and never creates it. No timer, retry or replay is introduced.

## Event and mode boundaries

Interactive installs only native mcp_tool hooks UserPromptSubmit, Stop and SessionEnd; no SessionStart. UserPromptSubmit supplies native identity/title and may publish or update. Stop may publish a usable native identity without a name, or update the corresponding identity using only actual report fields. Omitted/template-empty title does not erase a known name. SessionEnd withdraws matching presence and never bootstraps. Ordered identity transitions, delayed reports and context provenance remain FP03/FP05 obligations; allowing Stop does not make arbitrary tool arguments authoritative native reports.

Fresh/resumed/forked/cleared sessions follow the same event requirement. An unpublished session is unaddressable. Name is optional under BN01; no generated title or readiness prompt. After publication, active and idle communication and BD01 target/write truth remain required.

Lane still uses the same public launcher, selected by SESSIONBUS_LAUNCH_TOKEN, and retains its daemon-owned worker, SDK child, initial command-hook report, endpoint and stdio tool forwarder. The lane helper must not publish an extra peer. [FP06 composition](FP06-COMPOSITION-PROPOSAL.md) is stamped by delivery-0e63eaf3447786179b67c9b44425cc9b: one common installed mcp_tool UserPromptSubmit/Stop/SessionEnd set, handled in the interactive owner or forwarded through the existing lane endpoint to the worker. Lane adds only its SessionStart command report. The four runtime preconditions are open under OB03; caller options remain native-owned and interactive has no command hook.

## Plain README/facts wording

> A Claude peer appears when its MCP owner receives a native session report. This can be the first prompt or a later report, such as the turn's Stop event or a subsequent prompt. During plugin startup a report can fail to reach the owner. Until a usable report arrives, the session is absent from the roster and cannot receive peer messages. Publication is not guaranteed by the first prompt or the end of its turn. A peer may initially have no name; a later native title report supplies it.

This preserves the stamped no-deadline contract. The relay's “at that turn's end or at the next prompt” must not be written as guaranteed recovery timing. “Ordinarily first prompt” is a usage expectation supported by connected prompt examples, not a measured frequency or universal fact. No promise that every hook after callability arrives is supported.

## Evidence and historical comparison

FP01 final d16a3ca/tree13f7889 verifies the failed first-prompt report and successful later hidden-handler Stop in that run. [Independent review](../../evidence/fp01-final-review/REVIEW.md) preserves exact timing/projection limits. FP02 remains inconclusive for its different empty-catalog SessionEnd row; its source trace plus FP01's Stop call supports the narrower hidden-control candidate. Native hook/context and production-server behavior must still match the chosen implementation.

R3 `679fe9d3068b6362df867f8d78ce6708c4ce1342` and R4 `ff81565a252151575409efad2fccfd6d5e544383` Claude explicitly used no hooks; native exec/MCP topology is precedent, not proof of this report contract. OB02 on2.1.265 withdrew the retained interactive parent after SIGKILL left Claude alive. I12's initial-listener alternative was never released/run and remains unproved. The current design removes its endpoint rather than claiming that alternative impossible.

## Remaining work and upstream item

The later FP06 boundary stamp releases continued FP03 lifecycle → FP04 MCP/native/bus lifetimes → FP05 nested identities/groups, each sealed and reported under existing stop conditions. FP06 runtime preconditions are folded into OB03 when lane rows run; this adds no OB03 or L02 release. FP03 now tests valid Stop bootstrap with absent name, next-prompt naming, identity changes and active/idle communication under this contract. L02/revised OB03 and production implementation are not newly released.

After release, file the requested Claude Code upstream issue with this exact counterexample: an early submitted prompt failed its configured MCP hook without demonstrated MCP readiness gating; later Stop reached the hidden handler. Do not claim that plugins cannot receive any first events or that all hooks are always independent of readiness. The current projection does not timestamp native hook execution relative to barrier release. No upstream issue is filed by this document.
