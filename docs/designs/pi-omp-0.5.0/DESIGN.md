# Pi and OMP integration

Status: implementation in progress. Native prerequisites are installed and
version/help checked. Pi's first permanent build passed zero-input Worker
Open/Close and persisted interactive resume/quit. Run implementation review and
model acceptance remain in progress. OMP's native executable resolver is
implemented; its wrapper lifecycle remains to be built. The source base is peers
`75866839b7f024edf2c7f9d2d3af647a343f4987`.

## Native versions and implementation cost

Pi is `@earendil-works/pi-coding-agent` 0.85.1, upstream
`d981de1229ef899957bbe968bc8dcda02a21f477`. OMP is
`@oh-my-pi/pi-coding-agent` 18.1.17, upstream
`3b3a6dc9bbd85102ce19d0b1c11bf6870915f6ec`. These are fixed campaign
inputs, not a moving claim about the newest release.

The launcher, daemon Peer/Worker, caller, process ownership, lane queues and
native RPC controller will be Go. Each product requires an in-process JavaScript
extension to register its native Sessionbus tool and access its session context;
those APIs cannot be called from Go. That extension runs in the product's
existing Node (Pi) or Bun (OMP) process. No Node sidecar, new JavaScript SDK or
additional interpreter installation is selected. The extension uses platform
APIs and a private connection to the Go owner. The existing Go Sessionbus SDK
remains the daemon protocol implementation.

OMP's interactive FIFO is the necessary exception to Go queue ownership: its
timed hooks expose no cancellation signal, so injection must take a batch from
bounded local factory state without awaiting the Go owner. The native JavaScript
extension owns that queue and its confirmation state; Go still owns the bus
connection, delivery validation and process lifetime. No separate JavaScript
runtime or dependency is introduced by this state.

The native prerequisites are separate costs. Before the update, umka's complete
installed package trees occupied 111,289,752 logical regular-file bytes for Pi
and 1,864,727,343 for OMP, including dependencies and optional native packages.
These are neither minimum runtime costs nor RSS measurements. The selected
package tarballs alone unpack to 21,935,887 and 49,070,164 bytes respectively;
their resolved trees contain 118,599,634 and 1,220,404,166 regular-file
bytes respectively. The permanent installations now match those complete trees
after actual version/help commands under the real login environment. These are
logical file lengths, not RSS measurements. OMP retains its optional Transformers, CPU ONNX and Sharp
dependencies. Ignoring lifecycle scripts omits optional CUDA setup; required
CPU payloads are present in the staged packages. CUDA acceptance is not claimed.
At the architecture checkpoint `ccfa25d`, the private transport had 991 Go and 722 JavaScript
physical source lines, including comments and blank lines. Pi's separate native
RPC transport had another 754 Go lines. These are checkpoint counts, not final
implementation totals. The historical 650-line shared estimate
is not met. Ordered bounded writes, retained responses, cancellation and joined
closure remain required behavior; the estimate cannot be met by deleting those
semantics. Final accounting will count shared source once, each shipped copy
separately, and native prerequisites separately from wrapper binaries and
extensions. These source counts do not measure linked bytes or runtime memory.

## Ownership

`pi-peer` and `omp-peer` are the public products in every hello. The native
executable names remain `pi` and `omp`. The real native session ID is the
identity authority; a display name, wrapper request ID or bus run ID cannot
replace it. Open/resume must verify the returned native ID and directory.

`Close` joins the native lifetime. `Forget` additionally asks the daemon to
remove its retained lane row after the Worker stops; it does not delete the
native transcript. Pi has no session-deletion RPC. Its interactive picker is
the native deletion surface, and the wrapper does not imitate that operation
with filesystem removal.

One managed launch has a Go owner, its direct native child and a private
0700 launch directory. The native extension is supplied per launch. Ordinary
native launches do not acquire a Peer or Sessionbus tool. Managed metadata is
validated against the launcher and removed from the environment inherited by
native tools and nested ordinary launches. Pi uses the fixed private descriptor
`SESSIONBUS_PI_LAUNCH` with directory, owner_pid, socket and topology; native
mode must agree with topology (rpc/lane or tui/interactive). One launch retains
one private connection across extension reloads. Extension reload and native session
replacement must preserve the launch binding while replacing the old session
context. No stale captured context may execute a tool or receive a delivery.

Pi has one current native session per process. New/resume/fork tears down the
old AgentSession and extension runner; the old Peer is withdrawn, then the new
native ID is reported. Rename updates the same identity, including clearing a
name. Existing native titles win over a wrapper's initial name.

OMP also has one selected main session, but replaces it in place. Its
`session_switch` event covers new, fork and resume; reload switches to the
current session again. The AgentSession, runner and captured context survive,
and the same context accessor now returns the new native ID. The extension
must withdraw/re-report at that event and compare the live ID on every tool
call and injection. It cannot rely on Pi's native stale-context rejection.
OMP Task children have distinct AgentSessions and factory instances under the
same process launch binding. Each enabled child instance adopts its own native
ID and owns separate Peer/queue state. A restricted child with no extensions
does not acquire a Sessionbus tool. Shared module state must not merge these
instances or substitute the parent's identity.

OMP uses one owner registry and private bridge per launch. Every factory/session
binding has a bounded opaque owner token, rotated synchronously on a switch,
including a switch back to the same native ID. Requests carry both that token
and the live native session ID so delayed reports cannot affect a replacement
binding. The token is not a public identity. Each child has its own daemon
Peer and Caller; it never borrows the main Worker's Caller. Task factories run
in native print mode, while the main binding uses RPC or TUI mode. Adoption
must not infer the main owner from whichever readiness report arrives first.

The Go owner holds the bus Caller. Each native tool request carries its actual
session context and native tool call ID; the private connection binds them to
the current owner. Tool cancellation cancels only that call. Disconnect or
session replacement cancels and joins all work belonging to the old context.
Private bridge framing, pending calls, retained bytes and queue sizes are
bounded. Readers must continue while handlers await other responses.
The private hello's role field checks protocol consistency; it does not
authenticate a native process. Authentication remains the validated launch
binding and owned private socket. Cancellation signals exposed by the native
API must be passed into bridge calls. OMP does not expose its generic hook
timeout signal; that limitation requires a separate delivery boundary below.

## Delivery

The selected lane policy stages inbound messages in an owned bounded FIFO.
Only the next explicit Run submits them with its input. It does not silently
start a model turn, claim native consumption from a queue acknowledgement, or
replay an uncertain submission. Active and idle staging report
`queued_for_next_turn`; the queue is not durable across owner loss.

For Pi interactive delivery, an idle `sendMessage` custom message with
`triggerTurn:false` can append native model context without starting a Run.
The extension must immediately read the native leaf entry and match its type,
custom type, message ID and content before reporting `written`. The extension
API returns void, and its message event handlers do **not** receive this append:
`_appendCustomMessage` emits only to AgentSession subscribers. A busy delivery
stays in the owned FIFO until `agent_settled` or `before_agent_start` observes an
idle context; the append and leaf check then occur synchronously without an
intervening await. The next prompt's preflight also covers messages staged
during a separate compaction. A normal `session_shutdown` may drain into the
still-bound old context before withdrawing it. It never transfers the queue to
the replacement identity. Remaining queued messages are lost when their owner
ends; the sender was told `queued_for_next_turn`, not durable delivery.

Pi's native busy custom-message queue is not selected because the void API
provides no immediate acknowledgement and offers no withdrawal once queued.
The wrapper keeps ownership until it can prove the native append. A stale
context's guarded session-manager access throws; that cancels the old operation
instead of retrying it against a newly selected session.

`written` means accepted into native session context, not consumed by a model
or necessarily flushed to disk. Pi holds entries in memory until its first
assistant message; later appends use its native persistence path. OMP uses the
separate queued-injection policy below and does not inherit this immediate
acknowledgement.

## Pi Run completion

The native correlated `prompt` success is admission, not completion. Pi's
`agent_settled` event has no messages. `get_last_assistant_text` can skip an empty
aborted assistant and return an earlier turn, so it is not used for collection.

Before submission the controller records the last entry in native file order
as its cursor and separately records the current tree leaf as its parent.
These IDs can differ after tree navigation. After an owned completion it reads
the new entries since that cursor and verifies their
ancestry and current leaf. The answer and stop reason come from the current
interval's actual terminal assistant. No new assistant, an unknown stop reason,
an unrelated branch or failed collection is an error, never empty success.
An actual empty assistant with a recognized native stop reason remains valid.
Native retries and automatic compaction belong to the same Run. Intermediate
`agent_end` or additional `agent_start` frames do not by themselves end it.

The Pi native RPC reader caps each output frame at 8 MiB. Native message
events can contain the full user input or tool output, so overflow is an
explicit unavailable/retirement boundary, not truncation or partial success.
Lane input in this implementation is text-only; native image/attachment input
is not claimed. Native error text may be empty and still represents a valid
failed command.

Pi's public RPC has no finite no-agent result for commands or input handlers
that handle a prompt locally. The selected guard therefore uses the required
extension's awaited input/preflight observations, together with native stream
ordering and the actual first user message. A success without the required
owned preflight is unavailable and retires the child; it is not classified as
successful local-only work. A nested extension-origin prompt cannot stand in
for the submitted RPC prompt. Plain native input transformations and skill
expansion must be bound to their observed preflight text, not mistaken for an
unmodified echo.

The managed extension is first in the native CLI extension list. Pi preserves
that order ahead of global extensions, including after reload. Its first
`agent_settled` handler synchronously marks the owned interval as settling before
any await. This runs immediately after Pi clears its active flag and before any
global settled handler can start another run. Native starts before that marker
may be continuations; starts after it are foreign work and retire the owned
child with an unavailable result. Global extensions remain enabled.

The controller requires both that owned marker and the native settled frame
before collecting the result. The private bridge and stdout have independent
read schedules: an earlier stdout start may be read after the private marker.
Their observations must be reconciled without assuming cross-channel arrival
order. A failure or foreign start during collection invalidates the pending
result; once a result is finalized, later owner loss does not rewrite it.
A missing or disabled managed extension fails
the required handshake/witness checks. Ownership state survives extension
reload in the process and is bound to the current native session. This is a
capability and ordering guard, not an invented native turn ID. State-idle plus
absent events is not a completion authority. Installed acceptance must bind the
ordering with global handlers present; source review alone is not runtime proof.

Unexpected native work outside an owned Run retires the child. Startup dialogs
also fail Open: Pi installs its stdin RPC reader only after awaiting initial
session-start hooks, so writing a cancellation response cannot unblock a dialog
held inside those hooks. Runtime dialogs use the native one-way cancellation
response and must remain bounded and joined by their owning Run.

Cancellation owns the operation from before the first write. Before native
admission, cancellation must prevent a delayed preflight from escaping cleanup;
an idle `abort` response alone does not establish that. After admission, native
abort and actual outcome/terminal collection are joined. Canceling a public
waiter does not consume or erase the Worker's cached terminal result.
Writer-queue submission is distinct from full native admission. A command that
crossed that queue boundary may have reached Pi even without an admission
witness. Its staged input is consumed before retirement and cannot be replayed;
its delivery receipt must not claim `not_submitted`. That reason is reserved
for rejection before queue admission.

The pinned SDK has a retirement limit: acknowledging `turn.ready` completes
the Run before the SDK necessarily writes an in-flight interrupt reply.
Retiring the Worker at that point may therefore give the interrupt caller
`no_receipt`, while the acknowledged terminal status remains authoritative.
A controlled test binds this ordering after the actual delivery receipt and
terminal were received. It does not treat an earlier EOF as success or promise
an interrupt receipt after retirement. No SDK change is selected for this
milestone.

## OMP protocol

OMP 18.1.17 advertises protocols 1 and 2. Its initial `ready` precedes extension
initialization. Adoption requires protocol-2 negotiation, a correlated
`get_state`, and the managed extension's bridge readiness, with exact identity
and cwd checks. Physical and assembled frames need separate bounds because
protocol 2 supports chunked messages.

OMP has an explicit same-request `prompt_result`, including
`agentInvoked:false`. Its immediate RPC success acknowledges the command,
not admission or completion; a later error with the same request ID can follow.
Its RPC path does not emit Pi's `input` event. The OMP admission witness must
use hooks actually reached by RPC, together with the native start event.
Only `agent_end.isTerminal:true` ends the owned interval; maintenance can emit
intermediate events. Collect bounded native assistant messages across that
whole interval. Whole-session last-assistant text cannot substitute for missing
current output. Native no-agent results are handled explicitly and never borrow
an earlier answer. Exact projection and event ordering require installed probes.

The selected default preserves native configuration and does not inject
`--yolo` or grant tools. OMP's unconfigured native default is itself permissive;
preserving it must not be described as a restrictive wrapper policy. Unattended
interactive requests are canceled through the native response surface. Where
the frame cannot identify an approval, this is cancellation of the dialog, not
an invented tool-specific permission decision. Installed OMP help confirms `rpc-ui`; its actual tool UI mediation and
cancellation still require installed acceptance.

OMP delivery uses a separate mechanism. `appendEntry` records extension state
that is explicitly not sent to the model, so its leaf is not a delivery
acknowledgement. `sendMessage` returns void and its implementation awaits image
normalization before choosing an idle or busy path. Pi's synchronous append and
leaf confirmation cannot be copied. Its idle custom append emits no receipt
event, and replacement can occur during the normalization await.

Both idle and busy OMP interactive deliveries therefore stay in a bounded
per-factory FIFO and report `queued_for_next_turn`. The Go owner calls the
extension's stage operation; the extension checks the live session identity
and enqueues synchronously before acknowledging staging. The next naturally
started native run synchronously claims a batch in `before_agent_start` and
returns one custom message through that hook, without a bridge await. This
message lands after the native user prompt, queued next-turn messages and any
earlier hook messages.

Returning the batch alone does not prove injection. Exact native
`message_start`/`message_end` events confirm that it entered the run's prompt
messages; the subsequent `context` hook proves presence at that hook. Later
extension handlers and provider transformations can still change context, so
neither observation proves model consumption. Confirmation state is updated
synchronously, with exact message IDs and session ownership. Subsequent owner
reports are bounded, tracked and joined or canceled with their lifetime.
Native persistence follows the message-end hook and has its own generation
guard; a hook observation is not a disk-flush receipt.
Each claimed batch has a per-factory token and an exact ordered message-ID
list. Reports bind that batch and owner generation; claiming alone gives no
injection credit. Lane preflight witnesses likewise capture synchronously and
report through bounded owned work, reconciled with the separate RPC stream.
The wrapper starts no run to flush it. A batch submitted without confirmation
is not replayed; replacement never transfers it to a different identity.
OMP does not promise Pi's immediate idle `written` receipt. Installed tests must
prove this explicit difference, including replacement and cancellation.

Shutdown is a narrow exception to nonawaited generic hook reporting. The
extension closes its binding synchronously, then awaits its end-report
acknowledgement under OMP's existing dedicated two-second shutdown-hook bound.
No wrapper timer is added. Missing acknowledgement or native timeout is not a
graceful-end receipt; Go still joins the actual process and private connection.
The private native-shutdown method is main-owner-only: Task children bind that
native method to a no-op. A main shutdown response means only that shutdown
was requested, never that the process has exited.

OMP loads its explicit CLI extension after ambient extensions, so Pi's first
settled-handler guard does not transfer. OMP marks a prompt in flight before
calling preflight hooks, preventing an independent run inside those hooks.
Its wire terminal waits for that prompt to unwind, including awaited
`session_stop` continuations. The separate extension `agent_end` notification
is detached: its handlers are not joined by the terminal. Work they start later
is outside the owned event interval. Materialize the recorded current result at
the terminal; later foreign work retires the native owner before reuse but
does not revoke an already materialized result. OMP has no Pi-style
`get_entries` API, so it does not use Pi's history parser.

Its native hook timeout is 30 seconds in general and 2 seconds for shutdown;
after timeout native processing continues. The generic ExtensionContext has
no signal for this timeout: before-agent-start, message and context handlers
cannot propagate an unavailable AbortSignal into bridge calls. The injection
path therefore needs a boundary that does not await the bridge inside those
timed hooks. The per-factory staging contract above supplies that boundary.
A missing confirmation cannot become success. No additional
wrapper timer is selected to compensate for this native limitation.

Only transport, process ownership and other demonstrably identical mechanics
may be shared. Admission, completion and delivery remain product-specific.

## Installation and acceptance

Use the actual permanent umka installation, real home/config/login/PATH and
service environment. Native package preparation may be staged, but tests run
the installed commands. Native prerequisites and wrapper installs are separate
bound artifacts. No automatic model fallback, permission grant or replacement
of user extensions is part of this integration.

Native help and zero-model tests first bind CLI options, identity/resume/title,
extension loading, tool schema, delivery append, context replacement and joined
cleanup. Then single, source-bound model rows cover tool self identity,
completion/results, interrupt and next healthy Run, resume, staged delivery and
interactive context. Error and local-only paths are required acceptance cases.
Existing product regression and final archive/installation correspondence are
required before merge. Darwin native acceptance must be reported separately.

Historical `ff81565` tests and fixes remain regression inputs; fabricated
`agent_settled.messages`, earlier-answer collection and superseded permission
defaults are not copied into the new implementation. Evidence is retained under
`/home/antst/pi-omp-architecture-20260911`, with first failures kept immutable.

## Implementation checkpoints

Pi's Open/Close, interactive launch and package components form the first
installed checkpoint; its Run implementation is reviewed separately. Pi's
public Worker constructor takes the daemon socket, provisional launch-token
digest, absolute native executable and absolute extension path. Caller and
shutdown callbacks are supplied by the command. Product-specific process/RPC
state remains private to that implementation. The command, native resolver and
interactive owner are developed separately against this boundary.

On the installed `30192cb` checkpoint, the actual Worker advertised `pi-peer`
with its daemon-owned identity and closed with its captured wrapper/native
generations and private directory gone. Interactive Pi resumed an existing
persisted native session, advertised its exact ID and native name, and exited
with Ctrl-D; the fixture verified unchanged session history. These rows made
no Run or model request and do not establish delivery or terminal-result
behavior. The original process-detector timeout and retained-name collision
remain separate failed fixture attempts in the evidence record.

The first installed Pi checkpoint must distinguish these authorities:

| Boundary | Required evidence | Insufficient evidence |
| --- | --- | --- |
| Open | Managed extension readiness plus native state with exact ID/cwd | Process spawn or RPC ready alone |
| Admission | Owned RPC input, transformed preflight and native first-user correlation | Prompt success alone |
| Completion | Synchronous owned settling marker, native settled frame and current native entry interval | Idle state, intermediate agent_end or previous assistant text |
| Lane delivery | Both idle/busy receipts queued; no native submission before explicit Run | Interactive idle append behavior |
| Interactive delivery | Actual custom entry/leaf and exact message ID, or bounded owned queue while busy | Void send return or extension message hook |
| Replacement | Withdraw old native identity; no stale tool/delivery or queue transfer | Displayed title change alone |
| Cleanup | Actual owned child and bridge tasks joined, private resources accounted | Close request or missing roster row alone |

Installed ordering probes must preserve ordinary global extensions while
exercising continuation before the owned settling marker and foreign work
after it. They must distinguish current empty/error/aborted assistant results
from a prior successful answer. A local-only handled prompt is unavailable in
Pi. OMP reports an explicit no-agent result only when no extension work was
scheduled under that request ID; nested extension work still needs the owned
preflight/terminal checks. Neither product may borrow earlier output.
Model-driven scenarios follow only after their relevant zero-model mechanism
has been observed. Unexpected outcomes are preserved before correcting the
fixture or product cause; a corrected run uses a new evidence packet.
