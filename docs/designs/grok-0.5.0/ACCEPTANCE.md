# Grok installed acceptance, 2026-09-10

Runtime/package checkpoint: e24f9ca40ef08e73a27a1624423e39d4bf335a2a.
One permanent umka installation, actual user home/config/login/service; no
isolated product prefix or protected baseline. Grok native1.0.25 executable
SHA256 a46d17bcd602c46135b5be2da69a081447f8c514c5ea733c5be1265bcfb62b80;
installed Go binary1534033e5fc19ccd48fa6cbdebe259e52fbee5d28b04a320e994c7a61078f650.

Evidence root (external immutable artifacts):
`/home/antst/grok-architecture-20260910/dev1-runtime-e24f9ca`.
P1=`installed-phase1`, P2=`installed-phase2`, P3=`installed-phase3`.
The [manifest index](EVIDENCE.json) binds each seal. Raw native history is
observational evidence only; no wrapper code reads it for readiness or recovery.
The original source packet is `/home/antst/grok-architecture-20260910`;
relative archaeology references in the copied contract refer to that packet.

| Requirement | Actual evidence and scope |
| --- | --- |
| Permanent package, one binary/private alias/one generic skill | BUILD.json, P1 install/plugin records, P3 shared-smoke/grok-final-install.json. Literal installer replaced owned payload; package fixture also proves stale-skill removal. Earlier c8 zero-input Open/list/close is separately reviewed historical startup evidence. |
| Managed fresh name and mixed groups | P1 initial launch, process/env bindings, native-initial-parent/01a08a32-1832-7441-9623-77885877e8db and list records. Actual initial native ID/title, all three requested groups, public list and exact reply. |
| Interactive idle wake | P1 idle send/response, native initial transcript: injected message-7pha2u6k8nwr, exact GROK_E24_IDLE_RECEIVED. Receipt is admission; reply independently proves consumption. |
| Native /new and rename | P1 list-after-new: new01a08a34-928d-7ac0-9cea-ab2265d9a411, old owner remains. New name initially empty, no inherited initial title. Later native rename appears under the same ID. Immediate pre-completion list was still blank; no stronger installed event-timing claim. Exact event-without-delivery publication is controlled-tested. |
| Offline title resume and native fork | P1 list-resume/resume-selected; P2 resumed/fork process and cleanup records plus tui-commands. Resume selects original01a08a32... with new title and fresh launch directory. Fork selects distinct01a08a42-4b93-7753-8292-6c48981719e7 with requested title. These are zero-model native selection rows; inherited transcript is not new execution credit. |
| Actual parent-to-lane lifecycle | P1 native-parent-final/01a08a34...: native use_tool wraps sessionbus__sessionbus spawn/start/wait/ack/close. Child01a08a35-53cb-7232-aac4-e86f2e874efc, run-v1o99nklrizh/1, exact GROK_E24_CHILD_RESULT and completed/end_turn, matched close result{}. Parent also reads installed skill and discovers schema natively; lifecycle is not a shell/controller substitute. |
| Seeded wake and actual completion pointer | Same parent transcript plus child01a08a36-90c0-72f0-a350-f381ab887b41: message-me82n1wqr9e1 injected, run-a1vp7yevredg/1 exact GROK_E24_WAKE_RESULT. Authenticated completion pointer wakes actual parent, which calls wait/ack/close and replies GROK_E24_POINTER_COLLECTED. |
| Local unsent stage and nonconsuming cursor | P1 stage-unsent, spawn-stage/send-stage/run/status/ack/close records and native-parent-final/01a08a36-c69d-7512-8ceb-70cf6a6eff7d. Queued marker absent from pre-run native snapshot; one explicit input without literal marker yields exact staged marker. Repeated status agrees before ack. This is local worker-memory retention, not native idle staging. |
| Terminal-crossing native continuation | P1 crossing-r2 and native-crossing-r2: one imposed pause of exact owned primary after original submission, independently recorded original terminal, one interject, native fallback, same shared run held, exact original+preamble/newline/fallback aggregate, repeated cursor, then /2 without replay. Source-bound executed composition; direct primary actor-ack/frame order was not captured. Failed ptrace setup in P1 crossing is preserved with zero model/SIGSTOP/interject scope. |
| Active delivery and interrupt | P3 remaining-lane/interrupt and native-interrupt. Actual held tool, injected active receipt before interrupt, same run interrupted/cancelled with real preamble, repeated cursor/ack/normal close. Seven identified PIDs absent after close. Interrupted marker consumption is not claimed. |
| Helper loss | P3 remaining-lane/helper-loss: exact bound MCP killed during held tool; wait returns not_connected, five integration PIDs absent. Native tool/parent survived until separately recorded operator fixture release. This is the hard-failure descendant limit, not a successful containment result or a native terminal. |
| Independent persistence/auto-close policies | P2 policies and completion-supplement. All four effective policies returned and native turns completed/acked. Early false/1500 disconnected; true/0 survives owner exit then explicit close succeeds; false/0 cleanup reports busy during inferred owner close; true/1500 disconnects between roster and later not_connected cleanup. The fixture's fixed2s full-retirement assertion failed and remains preserved. 1500ms is close-initiation grace; no exact four-policy deadline PASS. Reviewed shared daemon/SDK controlled matrix is separate evidence. |
| Ordinary Grok | P2 ordinary-helper/native-events/private-group-list/process/cleanup. Real native session01a08a43... starts same installed inert helper, native tool_count0, no managed marker/observer child, no row in the exact session-private group. Global plugin/skill discovery remains visible. Zero model turns. |
| Cleanup | P1/P2 interactive/resume/fork records: all identified processes absent after only dedicated dead-pane removal; no missing native exit codes invented. P2 completion-supplement closes C scope: seven PIDs and native leader socket absent later; fixture socket unlink separate. P3 records remaining lane/shared-native cleanup and controller exit. |
| Shared-engine Claude regression | P3 shared-smoke: same-e24 archive/member/installed hash binding, native2.1.267 actual public list toolu_014NmjwBDkzxjA5kSF4jgvkf and matched result with actual Claude/Codex IDs, exact GROK_SHARED_CLAUDE_OK. Native ToolSearch discovery is separate. Ordinary zero-model native startup has no direct Sessionbus MCP in captured process set; versions pass. |
| Shared-engine Codex regression | P3 shared-smoke: same-e24 binary/archive binding, native0.153.4 startup/name and catalog sessionbus connected1; ordinary catalog only codex_apps. The single model request hit native account quota, so no new public-tool/result PASS. Refusal preserved; no reset/purchase/account/model substitution. Earlier installed Codex public-tool acceptance at docs/designs/codex-0.5.0 remains historical; exact-e24 controlled engine/Codex regressions pass separately. |

## Controlled correctness and remaining limits

Actual ACP/Worker/socket/compiled-child tests cover response-before-EOF,
cancelled drain and capacity, replay rejection, seed refusal without transport
death, ReportDelivery/cursor, startup/helper-loss adoption, cleanup errors,
unsent FIFO preservation before submission, attempted-write accounting, actor
ack after terminal, continuation mismatch/overflow retirement and submission
gate races. Reviewer counterexamples GKR01–07 are retained as regressions.
`TestResidentForwarderEOFCancelsCallerWaitWithoutAck` is an actual forwarding
socket/Caller test; it is not an installed native cancellation-notification row.

The common engine has actual blocked OS pipe/socket EOF tests, response-work
and payload bounds, initialized-after-success, and no response after EOF. Final
full Linux race tests, vet and lint are recorded in the author handoff; CI binds
the final exact head. A documentation header-placement failure is preserved
and corrected without weakening the architecture test. macOS CI is required;
no actual native Grok macOS run is claimed.

Failed Open/native policy/error races not exercised in current native rows
retain their controlled and historical scope. No automatic permission grant
or native approval broker is added. Hard integration failure can leave native
tool descendants; no PID registry, sweeper, timer workaround, or durable state
is added. The Codex current-model quota gap and policy timing assertion remain
visible, rather than being relabelled green.
