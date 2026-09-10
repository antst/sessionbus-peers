# Grok native continuation ownership

Selected 2026-09-10. Extends contract C without changing the daemon, wire, or Go SDK. The existing shared Worker run is the sole reservation and result cursor. A product-created continuation caused by an already-submitted delivery remains part of that reservation; the adapter neither submits another prompt nor creates another run.

## Native authority

Use one ordered PRIMARY ACP notification stream for identity, admission, output and native completion. Retained P1-P7-acp-raw-v3.txt has primary queue entry41, running prompt43, user chunk45, active actor acknowledgement57, native terminal280, prompt_complete281, correlated RPC result283. Idle actor acknowledgement295 is followed by actual fallback running event297. Observer40/42/44 and58 are duplicates, not a second ordering authority. Terminal notification method is exactly `_x.ai/session_notification`, update.sessionUpdate=`turn_completed`, update.prompt_id and update.stop_reason. Drop frames carrying `_meta.isReplay:true`; never infer replay from prompt-ID naming. Observer remains the sender of interject RPC, not a second output reader.

These captures support the selected composition; installed acceptance must bind the current native version and demonstrate its continuation behavior. Never infer fallback merely because an observer acknowledgement arrived after the primary RPC response.

## Submission and admission

Register the explicit/seed prompt's exact combined rendered text before attempting its write. A live same-session runningKind=prompt, exact runningText and nonempty runningPromptId binds that native prompt. A user chunk without a prompt ID adds no necessary identity. The correlated session/prompt terminal must agree with the bound ID. Seed ReportDelivery runs outside the primary reader/mutex after native binding, without waiting for terminal.

Serialize interject submission through a cancellable gate until the previous submitted interjection has a scheduling classification. This is one in-flight correlation, not a text queue or replay scheduler. Acquire the gate and register message ID/text against the currently owned run before submission. Under the same owner mutex, reject submission once that run has entered terminal retirement; do not deliberately wake a known-idle stage lane through interject.

The exact message/session actor acknowledgement is permanent admission and produces the existing injected receipt, never an invented 'injected,new turn' enum. Native primary ordering determines scheduling: an acknowledgement within a still-open native prompt attaches to it. After that prompt's native terminal, keep the scheduling classification pending until an actual running event binds a new native prompt ID to the exact submitted text. That is a continuation, not a guessed new run. No decision from callback/observer wall-clock ordering, a queue RPC reply, socket write, or a prompt-ID naming convention.

Register before write and distinguish known pre-submission refusal from attempted delivery. A pre-submission cancellation/refusal removes the reservation without changing native work. A definite native refusal likewise settles it without replay. After attempted submission, caller cancellation may end the caller wait but must not discard native accounting: retain the correlation until native acknowledgement/classification or connection failure. Never hold the primary reader or owner mutex on a bus write or a waiting caller.

## Completion and output

Do not return from the existing Run callback while an admitted submission's scheduling is unresolved or a bound native continuation is running. Original session/prompt result, primary terminal state, pending submission classification and continuation completion must all settle before publishing the shared terminal. The same primary stream captures the continuation's output and actual terminal even though it has no new session/prompt RPC of ours.

Concatenate owned native answer segments in native execution order, separated by a newline. Use bounded output retention; overflow becomes the existing unavailable result, not truncation. Ignore thought, replay and foreign-ID chunks. Overall outcome is completed only if every owned segment completed; preserve the first non-completed native outcome and its actual stop reason, otherwise the final actual completion reason. No fabricated terminal or discarded earlier failure. Documentation states that a delivery crossing a native boundary may append a native continuation to the same shared result.

Interrupt continues to target the lane's actual current native work while this reservation is held. EOF/bus loss settles pending accounting and returns unavailable where native completion is missing. No result-body journal or survival after worker retirement is introduced.

## Required controlled schedules

Use actual ACP framing and the real Worker where applicable: primary actor ack before terminal while observer reply is late; primary terminal before actor ack followed by a held native fallback and terminal; exact fallback output retained with one shared run/cursor; cancellation before submission; cancellation after write while native classification is held; terminal retirement versus a waiting delivery; primary/observer order inversion; foreign/replayed IDs; interrupt and EOF during continuation. No new prompt submission may occur in the continuation test, and no following explicit run may replay any submitted message.

## Installed observation scope

Selected 2026-09-10 after the setup-only ptrace attempt failed: current umka ptrace_scope=1 denies non-ancestor attach and passwordless sudo is unavailable. No model work, SIGSTOP or interject occurred in that setup row; it is retained separately, not a native behavior failure.

The bounded crossing may instead establish source-bound executed composition. Bind exact installed binary/source and primary bridge PID/role; pause only after the original prompt is submitted. Native updates/history must independently record original terminal before the single interject, a distinct fallback prompt ID with the exact submitted text, and each prompt's answer/terminal. Keep the same shared run running through held fallback, resume the bridge in guaranteed cleanup, and collect the exact aggregate of both independently retained answers without another session/prompt or replay. The reviewed primary receive/classification/completion gates explain that resulting aggregate; this is an execution inference from bound source and native records, not direct primary actor-ack/frame-order capture. Plain wall-clock timestamps or an ordinary in-turn interject alone are insufficient. Record the imposed pause and all missing observation fields honestly; no further trace-discovery campaign is required for these conditions.
