# Claude 0.5.0 — consolidated design requirements

Updated 2026-09-09. **Current requirements register for architect, implementer and reviewer.** Read this before the implementation inventory. Direct owner instructions take precedence over earlier designs/stamps. A proposed mechanism is not a requirement or a proven product fact. Keep changes here, with their disposition, before issuing dependent implementation instructions.

Current work: interactive Phase C steps1–6 released by fable59b631e8; activation/default-global-plugin behavior is on HOLD after the owner's ordinary-Claude question. Independent implementation/tests may continue. No product runtime on pdev, no installation/runtime on umka without its separate release. Lane L02/OB03 and its implementation inventory remain open. The latest activation investigation is source/evidence-only, not a new native execution release.

## Product and integration boundaries

| ID | Requirement |
|---|---|
| R01 | Complete one product end to end before starting the next: Claude, then Codex, Grok, Qwen, OpenCode, DSH, Kilo. Interactive and lane behavior both belong to the Claude product scope. |
| R02 | The daemon remains product-independent. It routes the universal protocol and owns generic worker/process-group lifecycle. Claude-specific SDK, hooks, naming, native socket, turn and permission logic lives only in the Claude adapter. No Claude manager daemon is added. |
| R03 | The same public `claude-peer` launcher serves both modes. Interactive execs Claude; lane mode is selected by the daemon launch token/environment and runs a resident worker. Never accidentally start interactive mode from a lane token. |
| R04 | Interactive: Claude owns its lifetime and keeps the MCP process. That MCP process owns the sole bus connection/Caller and public tools. No retained interactive parent, private identity endpoint or tool forwarder. |
| R05 | Lane: the daemon spawns the adapter worker; that worker owns the bus connection and controls its native child. Its MCP helper forwards tools/reports to that one worker. L02/OB03 must prove this composition and generic process-group cleanup. Interactive tests do not prove it. |
| R06 | Ordinary `claude` must remain ordinary, without implicit Sessionbus communication/publication. Installing the integration must not silently turn every ordinary session into a peer. **Activation mechanism undecided; this overrides the old inventory's default direct-native publication.** |
| R07 | Activation must account for the whole integration: MCP loading, hidden native report hooks, public tools, skill/command discovery and actual native permissions. Do not select per-launch plugin loading from flag existence alone, or assume always-loaded/inert MCP was pointless. Compare earlier releases and prove the chosen route. |
| R08 | A skill is part of the integration. It must describe the actual usable public tool and mode/activation contract; it must not cause fabricated identity, permission bypass or calls to tools represented as available when inactive. Skill visibility for an ordinary unintegrated session is not yet specified by an activation selection. |
| R09 | Native configuration, authentication, trust, approvals and permissions remain native. No isolated home, credential copying, permissive replacement configuration, blanket allow/bypass mode or silent policy weakening to make integration work. |
| R10 | All product-owned features stay in the product: no session listing/selector/picker, resume matching, title inference, native option/error reimplementation, substitute history, extra native writer lock or invented native identity. |

## Launch, identity and presence

| ID | Requirement |
|---|---|
| R11 | `claude-peer -n NAME -g GROUP` names the actual native Claude session; native `-n` is passed to Claude, not separately implemented. No ID/title is minted by the interactive adapter. |
| R12 | The stamped outer argument grammar removes only the final `-g VALUE` pair. Preceding native argv remains byte-for-byte and ordered, including --, empty values, repeats, variadic options and native errors. Any added native activation argument must be separately documented/tested after selection; do not silently change this contract. |
| R13 | Group-list VALUE uses the bus grammar: empty string→[]; otherwise comma splitting with no trimming/deduplication/empty-element dropping. Existing bus schema validates it. No suffix means this launch's empty groups, not an install-time/inherited default. |
| R14 | Name and groups come from launch/native reports, never installation. No name fallback to session ID/cwd, truncation, trimming or automatic /rename repair. Unnamed native sessions are legitimate. |
| R15 | Interactive identity authority is a usable native report through the selected product hook route, never tool arguments invented by the model, session-file searches, startup environment guesses or native PID registry matching. Hidden handler stays out of the public tool catalog. Do not claim hidden means cryptographically authenticated. |
| R16 | First-report contract: presence begins only after a usable native report reaches the resident owner and bus hello succeeds. Before that, the session is absent/unaddressable. No launch, first-prompt or first-turn publication deadline. Missed startup events are not replayed. |
| R17 | UserPromptSubmit can publish/update native ID/title. Usable Stop can establish identity with absent name; absent/template-empty title does not erase a known title for that same ID. Matching SessionEnd withdraws and never creates. Interactive subscribes to no SessionStart hook. |
| R18 | Native rename is reflected when a subsequent native report supplies it. Fresh/clear/resume/fork follow the same first-report rule. No idle polling, guessed name or repair prompt. |
| R19 | Native resume by name uses Claude's own mechanism and any native picker. After at least one real turn, successful resume must publish the selected old native ID. Zero-turn sessions are not made resumable by dummy prompts or manufactured history. |
| R20 | Nested native sessions use actual native IDs. Explicit integrated launches may inherit launch groups; same-ID resume follows daemon supersession. No suffixes or external-native-writer locks. **Earlier globally installed nested-plugin evidence does not decide whether a plain child `claude` activates under the revised R06 requirement.** |
| R21 | Late acknowledgment/cancellation cannot revive withdrawn identity or retarget a pending message. Keep only necessary current connection/identity/cancellation state; no identity registry/history/queue or resume-matching logic. |

## Communication, tools and lifecycle

| ID | Requirement |
|---|---|
| R22 | Both integrated modes support public bus communication, including peer/group addressing and caller operations. Lane communication must not disappear as a side effect of simplifying interactive ownership. |
| R23 | Interactive uses Claude's native messaging carrier. Each delivery captures the acknowledged recipient native ID, bus source/body/message ID and native endpoint before async work. Identity changes never retarget the captured frame. No Channels dependency, second route or terminal typing fallback. |
| R24 | `written` means local native-carrier write completion only. `accepted` remains reserved for retention responsibility; `injected` requires native admission acknowledgment. EOF/zero response bytes do not prove either. Destination consumption is separate runtime evidence. |
| R25 | Pre-submission unavailability or observed refusal can be rejected with its boundary stated. Possible/partial submission followed by failure is uncertainty, not native rejection/non-consumption. Preserve Internal -32603 with valid string data→daemon no_receipt. Never relabel uncertainty as closing/rejected or retry. |
| R26 | Lane idle delivery must obey the native staging/no-unrequested-turn contract. `queued_for_next_turn` needs the actual demonstrated replay/staging boundary. Interactive idle wake-up is not lane acceptance. No adapter FIFO or fabricated admission acknowledgment. |
| R27 | Native terminal results bind to the correct input/run. Interrupt acknowledgment records/coalesces a request; only the native terminal establishes the outcome. No fabricated successful interrupt, dropped spawn options, SDK-imposed native flag translation or hidden close fallback. |
| R28 | Public tools run in the current owner using one Caller. Interactive tools cannot publish a substitute identity, open another connection, wait for a future prompt to become ready or expose the hidden identity handler. Actual tool permissions and skill discoverability are acceptance obligations. |
| R29 | Use the pinned public kit Connection/Caller exports directly for interactive, as stamped. Its Peer still retries and is not selected. No SDK fork, monkey patch, schedule override or copied wire/schema implementation. Native SDK choice remains optional and must meet wire/native facts. |
| R30 | No automatic integration timers, retries, fallback routes, pollers, report replay, native queue, service or private endpoint. Existing explicit user-requested Caller wait behavior is not a readiness/reconnect mechanism. Every process/connection/state/listener/cleanup action needs a concrete invariant and owner. |
| R31 | Unexpected bus loss/supersession ends this resident integration and settles pending calls. No reconnect or automatic re-publication. Deliberate SessionEnd withdrawal may allow a later actual report after native clear in the same healthy resident. Record terminal intent before async close/reply; handle rejected promises. |
| R32 | Interactive MCP death must not strand Claude behind a retained launcher; Claude remains native-owned. Adapter cleans its bus/native-transfer resources on EOF/output failure/shutdown, without killing/scanning unrelated processes. Published-owner EOF was not observed in the native ledger and stays a facts limit; test the implemented EOF path deterministically. |
| R33 | Lane worker death/process-group cleanup is a generic daemon ownership obligation to prove (L02), not permission to add product-specific daemon logic. EOF/close/interrupt/terminal behavior is scoped to actual measured native boundaries. |
| R34 | Claude may update in place while a parent is alive; root and child can run different versions. Production does not pin/select/downgrade or hash-gate Claude. Probe hashes qualify observations only. Keep every fact version-qualified. |

## Packaging, migration, size and acceptance

| ID | Requirement |
|---|---|
| R35 | Install from a literal README in the real home/config/login shell and real user-service environment. Bind actual npm/bin/plugin/skill/hook payload, runtime prerequisite, daemon and kit. No names/groups at install time. Installation recipe awaits activation selection under R06/R07. |
| R36 | Activation must actually permit the intended public MCP call and hidden hook report under native policy. Loading/listing alone is not callability; callability is not permission; skill existence is not discovery. Record native denial truthfully; do not add a bypass to get a pass. |
| R37 | Interactive package has five runtime modules in the stamped inventory, no custom installer/service/forwarder executable. Pin kit preview `https://pkg.pr.new/@sessionbus/kit@5f93fbb` until a compatible release; lock integrity and count the dependency footprint. Activation amendment may adjust existing modules/manifests/recipe, not silently add a subsystem. |
| R38 | Remove Go interactive backend/tests and interactive-only functions. Preserve shared Go lane source until lane selection; no old-Go compatibility bridge. Partial candidate token mode is explicitly unavailable and is not installed/advertised as a complete Claude lane provider. |
| R39 | Native/plugin/npm removal must remove exactly owned integration assets and preserve ordinary Claude, history, other plugins and services. Record retained cache/data and baseline limits; never claim global rollback from partial inventories. |
| R40 | Count whole implementation: runtime, tests, helpers/scripts, manifests/lock, guidance, shared modified files, package/dependency bytes. Moving code outside a file/package is not a reduction. Current baseline and final INTERACTIVE-SIZE.json are required; no claim of unmeasured after-size. |
| R41 | FC01 installed hookup; FC02 correct named identity/groups at first usable report; FC03 quit/resource accounting; FC04 native name resume after a real turn; FC05 correct resumed identity; FC06 active/idle communication with truthful receipt; FC07 removal; FC08 argv/native errors, rename/clear/fork/supersession/lifetime. Add ordinary-Claude/activation/skill/permission checks to FC01/FC08 before acceptance. |
| R42 | Lane LFC01–LFC10 remain separate: install/spawn readiness, actual native open identity, resume, tools, controlled runs/terminals, idle/active delivery, interrupt/close, session isolation/same-ID daemon rule, and removal. FP06 common-hook forwarding/initial command report requires OB03; no fixture or interactive credit substitutes. |
| R43 | Review R1 first, then every R2/R3/R4/R5 delta. Compare every new mechanism with earlier working behavior, including why an always-loaded/inert MCP might have existed. Cite release+commit, separate code/tests/runtime evidence and missing rationale. Do not declare unsupported/impossible from one failed probe. |
| R44 | Product-dependent behavior needs real umka runtime evidence or cited prior sealed evidence. Source inspection identifies an exact probe and bounds claims; it does not manufacture a pass. No source/fixture/projection credit beyond what it records. |
| R45 | Implementation tests use production code, controlled events and relevant assertions; no sleeps or copies of fixtures masquerading as implementation tests. Product/runtime counterexamples trigger design disposition, not an undocumented workaround. Preserve failed evidence separately. |
| R47 | MCP cancellation may keep one AbortController per currently in-flight public request, keyed by its MCP request ID. This is request lifetime state, not a session registry/queue/timer. Cancel only the matching pending Caller operation; unknown/completed IDs do nothing. Cancellation never asserts remote turn interruption, message withdrawal or non-consumption. Remove request state on completion/EOF/output failure. Test the actual pinned kit's late-response behavior; if it closes the connection, apply terminal integration loss, never reconnect or claim remote cancellation. |
| R46 | Dev1 is sole Claude implementer; astra reviews non-author; fable stamps/sequences. Current implementation is a signed PR into peers develop, steps1–6 offline only. Real step7 installation/product probes require their own release. No new native action follows merely from this requirements document. |

## Current decisions versus open work

| Topic | Current disposition |
|---|---|
| Generic daemon / two adapter modes | Fixed requirement, R02–R05. No Claude logic moves into daemon. |
| Interactive exec + product-kept MCP owner / native report / native carrier | Selected, scoped evidence reviewed. See B04 and FP03/FP04/FP05 reviews. |
| First-report/no deadline / optional name / written / no Peer retry | Stamped. BN01/BC01/BD01 merged and installed; C8 remains separate bus release work. |
| Ordinary Claude / skill+MCP+hooks+permissions activation | New owner requirement; old default-global-publication clause superseded. **Mechanism undecided.** Candidates include native per-launch whole-plugin loading and correctly gated installed MCP; source/history/real call/permission evidence must decide. No dummy server or injected activation flag selected yet. |
| Prior FP05 nested rows | Prove their installed-global-plugin topology and inherited explicit group, not the new activation contract. Preserve evidence and account the changed premise. |
| Lane / common hook composition | Shape specified, L02/OB03/activation interaction still unproved. Full Claude design/acceptance is incomplete until these close. |
| Implementation handoff | Stamped inventory hash154a594b preserved as history; activation section held. Independent authorized implementation continues. |

## Sources and precedence

Owner's direct requirements across this session govern, including the 2026-09-09 ordinary-Claude, activation-permission and skill corrections. Base protocol: `antst/sessionbus@9f366be908a4719f514f7383ca5210e2d0208b37`, with explicitly recorded later amendments. Release analysis and exact historical hashes: [ANALYSIS](full-product-review/ANALYSIS.md). Current architecture: [B04](full-product-review/phase-b/B04-DESIGN.md), [B05](full-product-review/phase-b/B05-EDGE-AND-CONTRADICTION-ANALYSIS.md), [B06](full-product-review/phase-b/B06-PLAN.md), [FIRST-REPORT](full-product-review/phase-b/FIRST-REPORT-CONTRACT.md). The [stamped inventory](full-product-review/phase-b/INTERACTIVE-IMPLEMENTATION-INVENTORY.md) is preserved byte-for-byte; its conflicting default native activation is superseded by R06/R07 here, pending a concrete amended mechanism. [STAMP](STAMP.md) records approval history, not permission to contradict later direct owner requirements.
