# Pi and OMP integration

Status: implementation in progress. Native prerequisites are installed and
version/help checked. Pi's first permanent build passed zero-input Worker
Open/Close and persisted interactive resume/quit. Its first installed normal
Run/list/result/close also passed. With the UI-admission correction installed,
idle lane delivery, one explicit Run and Forget preserving native history
passed. Interactive idle delivery and native session replacement also passed
on the subsequent permanent build. The remaining native acceptance rows are
still pending.
OMP's native executable resolver, argument routing, process/RPC components,
owner registry, native extension and joined process owner are integrated as
components. The process owner separates terminal and RPC transport and joins
startup, cancellation and shutdown cleanup in local subprocess tests. Its public
Wrapper, Run controller, managed payload resolver and thin interactive launcher
are integrated with the command, installer and release package. Local tests
cover fresh Open, admitted interruption followed by a healthy Run, and submitted
staged delivery interrupted before preflight without replay. The first permanent
OMP installation passed exact artifact, alias and captured service checks; native
Worker and interactive startup acceptance remain pending. No native OMP acceptance is claimed. The source base is peers
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

The first OMP wrapper artifact at `1c7fef76f1977d988d12d3b443ccddac8faf5e6e`
contains eight regular files totaling 4,188,047 bytes. Its stripped Linux amd64
Go binary is 4,124,834 bytes; the managed OMP extension and private bridge total
56,596 bytes. The compressed archive is 1,739,536 bytes. These figures exclude
the existing native OMP/Bun installation and do not measure memory use. Its Go
dependency set is unchanged from the reviewed Pi artifact, and it adds no
JavaScript package or sidecar. Test-only follow-ups preserve these shipped bytes.

## Ownership

`pi-peer` and `omp-peer` are the public products in every hello. The native
executable names remain `pi` and `omp`. The real native session ID is the
identity authority; a display name, wrapper request ID or bus run ID cannot
replace it. Open/resume must verify the returned native ID and directory.

OMP distinguishes the direct child's launch directory from its live native
working directory. Native startup may apply `--cwd`, choose a directory when
started from HOME, or restore a resumed session's project. The managed extension
and its describe response supply that live directory. A lane with an explicit
`Open.Cwd` must match it for both fresh and resumed sessions. With an omitted
resume cwd, the lane adopts the validated persisted directory while still
requiring the exact requested native session ID. A fresh lane must match its
chosen launch directory. Interactive argv retains native cwd behavior.

`Close` joins the native lifetime. `Forget` additionally asks the daemon to
remove its retained lane row after the Worker stops; it does not delete the
native transcript. Pi has no session-deletion RPC. Its interactive picker is
the native deletion surface, and the wrapper does not imitate that operation
with filesystem removal.

OMP likewise exposes no session-deletion RPC or extension operation. Its ordinary
fresh RPC startup persists lazily: without an assistant entry, zero-input Close
leaves no resumable file. Explicit native `new_session` instead ensures an empty
session header is on disk. After a reply, native Close retains the conversation.
The wrapper does not delete or rewrite transcripts; native resume can still
migrate their contents or relocate a session whose original directory is missing.

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
name. Existing native titles win over a wrapper's initial name. Whenever the
current native title is empty, including after replacement or title clearing,
the public Peer uses the wrapper's initial name. This is the wrapper's display
policy; the daemon also accepts an empty Peer name.

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

The transport pins the advertised limits to this native version: 1 MiB per
physical frame and 64 MiB per assembled frame. A native `rpc_frame_error`
reports a dropped oversized event; retiring on it preserves attribution rather
than treating missing event content as success. An oversized response remains
a native failed response. An elided `agent_end` with empty messages is not
evidence that no assistant ran and cannot replace the required terminal flag.
OMP does not redirect unrelated stdout writes as Pi does; an ambient extension
writing non-protocol text can therefore retire the managed transport.

One-way UI cancellation enters the ordered writer queue atomically, then
releases the admission lock before awaiting its write. A blocked UI write must
not prevent a separately canceled call from reaching its cancellation check.
The same ownership rule applies to Pi's native RPC writer.
The OMP turn joins admitted UI cancellation work before finalizing its result.
If caller cancellation or transport retirement interrupts that join, the turn
cancels its owned UI context and reports unsettled writes as failure. A native
terminal cannot conceal a pending cancellation-write failure.

OMP has an explicit same-request `prompt_result`, including
`agentInvoked:false`. Its immediate RPC success acknowledges the command,
not admission or completion; a later error with the same request ID can follow.
Skill and consumed builtin commands may also return an exact `agentInvoked`
boolean in that initial response. Preserve this native fact separately from
the ordinary response with omitted data; it does not replace run attribution.
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

An explicit, correlated `queue_full` rejection removes only an unclaimed Go
reservation and leaves the owner usable. A missing staging boolean, malformed
reply or rejection after a native claim is a protocol failure. A failed bridge
call cannot establish rejection: the submission remains uncertain and is not
replayed.

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
Per-binding report sequence numbers preserve native ordering across concurrent
bridge handlers. The Go registry accounts for at most 32 MiB of retained
payloads across bindings, queued deliveries and reports; consumed evidence is
evicted. This is separate from transport accounting and is not a peak heap or
RSS bound.
The wrapper starts no run to flush it. A batch submitted without confirmation
is not replayed; replacement never transfers it to a different identity.
OMP does not promise Pi's immediate idle `written` receipt. Installed tests must
prove this explicit difference, including replacement and cancellation.

Shutdown is a narrow exception to nonawaited generic hook reporting. The
extension closes its binding synchronously, then awaits its end-report
acknowledgement under OMP's existing dedicated two-second shutdown-hook bound.
No wrapper timer is added. Missing acknowledgement or native timeout is not a
graceful-end receipt; Go still joins the actual process and private connection.
An earlier factory failure must not cancel or discard its ending report.
Cleanup remains bound to the process connection, joins prior report work and
preserves the original failure after ending the public binding. In particular,
a failed Task child must not leave its Peer registered while the parent lives.
The private native-shutdown method is main-owner-only: Task children bind that
native method to a no-op. A main shutdown response means only that shutdown
was requested, never that the process has exited.

OMP loads its explicit CLI extension after ambient extensions, so Pi's first
settled-handler guard does not transfer. OMP marks a prompt in flight before
calling preflight hooks, preventing an independent run inside those hooks.
The private bridge and stdout can deliver their observations in either order.
The controller waits for the next unconsumed preflight of the exact owner token
and session ID, in native report order. It rejects a leading foreign prompt
instead of searching for later matching text. Cancellation or recorded owner
failure before consumption leaves that witness and its byte accounting intact.
The Run controller must join this wait with native no-agent results, terminal
events and owner loss; the waiter alone is not run admission.
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

The installed `e946fb1` checkpoint passed one normal Run using the model Pi
reported at native startup, `deepseek/deepseek-v4-pro`. Copied native history
and public controller records bind one actual Sessionbus list call, its tool
result and self identity, the final assistant JSON, and equal wait/status
results followed by acknowledgement and close. The subsequent separate
persisted interactive resume/Ctrl-D row passed with unchanged history.
Root review is in `review-pi-normal-run-installed-root` under the evidence
directory. Private config equality and private startup-probe output remain
host-side observations; native startup fallback is not independently exposed
by the model snapshot. This row does not establish delivery, interrupt,
Forget, replacement or the later UI-admission correction on the native host.

The installed `399de2f` checkpoint passed one idle lane delivery followed by
one explicit Run and `session.close` with `forget:true`. Explicit fixture
handshakes captured the queued, idle public state before Run and the native
transcript before Forget. The sole native user message contained the exact
staged sender envelope before the prompt; one Sessionbus list call and final
JSON matched the full public identity. Wait/status agreed. Forget removed the
bus row while the same native file retained all eight entries and identical
6,822 bytes. Owned processes and the private directory were gone after close.
Root review is in `review-pi-delivery-forget-installed-root`. Pre-Run disk
absence remains corroboration only because fresh Pi entries can be held in
memory. This row does not establish interactive delivery or interrupt recovery.

The installed `aa096e8` checkpoint passed the separate zero-model interactive
delivery/replacement row. A resumed session received one exact custom message
with a `written` receipt. Native `/new` withdrew that Peer and reported a new
native ID with the explicit wrapper fallback name in the same wrapper and
Node process generations. A second delivery returned `written`; Ctrl-D exited
normally, both Peers disappeared, and owned processes/private resources were
gone. The old transcript retained its custom entry and identical 7,416 bytes
through replacement and quit. The new session's receipt proves the native leaf
append, not persistence or model consumption. Root review is in
`review-pi-interactive-installed-root`. The preceding observer hello failure
was a fixture product label over the daemon's 32-character limit, before any
native launch; it remains a separate retained outcome.

The subsequent interactive busy-delivery attempt on `aa096e8` demonstrated a
queued receipt while a real Sessionbus tool reply was held. The native
transcript remained byte-identical until the reply was released. After the
assistant completed, the delivered custom message appeared exactly once as
its next child entry. Independent raw replay confirms that behavior in
`review-pi-interactive-busy-installed-root`. The fixture itself exited with a
verifier error: it expected protocol 1 on the sink's local identity before the
SDK normalized its copy for admission. The later idle-write step was not
attempted. Cleanup withdrew the Peers and joined owned processes; it used
fixture TERM, not normal Ctrl-D. This is evidence for busy delivery and settled
draining, not a full fixture pass or an interrupt-recovery claim.

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
