# Kilo and the shared native family

The implementation uses shared source with fixed OpenCode and Kilo policies.
[ACCEPTANCE.md](ACCEPTANCE.md) records installed observations, retained first
outcomes and their limits. [SIZE.md](SIZE.md) measures source, dependencies and
the two physical installations at the named b71 snapshot.

| Source | Responsibility |
| --- | --- |
| `cmd/kilo-peer`, `wrappers/kilo` | Product entry, direct native executable/resource resolution, argument policy and thin fixed-family facades. |
| `wrappers/opencodefamily/interactive_*` | Shared direct-child launcher, unique private directory, native loopback topology, signals and selector translation. |
| `wrappers/opencodefamily/lane*`, `run.go`, `native_policy.go`, `native_summary.go`, `kilo_close.go` | One Worker-owned Run, synchronous native request, history/result materialization, staged input and fixed product-specific interruption/close behavior. |
| `wrappers/opencodefamily/plugin` | Native server/TUI hooks, actual tool identity, per-session owners, inbound delivery and endpoint readiness. |
| `wrappers/opencodefamily/install*` | Fixed-product native config reconciliation, bounded local skill registration and rollback. |
| `internal/pluginstage`, `scripts/package-product`, `scripts/release/install-product` | Stage canonical source into complete permanent packages and install through Go maintenance. |

## Native integration and cost

Go supplies the command, maintenance and lane implementation. JavaScript is
required for native in-process server/TUI hooks, actual tool invocation context
and the supplied reactive route API. It executes inside the already required
product's Bun runtime. Installation adds no Node or independent Bun runtime,
interactive broker process, plugin SDK, Effect or Solid dependency. Build-time
npm stages the sole Sessionbus kit dependency from the `pkg.pr.new` preview pin
`0b35c99`.

Both installed packages contain the common JS and kit. Shared source reduces
implementation duplication; it does not eliminate those physical copies or
reachable fixed-policy code from the linked OpenCode binary. SIZE records these
costs, including the increase from OpenCode's previously accepted snapshot.
Native package size and runtime memory are separate from wrapper payload;
unmeasured costs are explicitly listed there.

Managed Kilo starts the resolved native executable directly, with loopback HTTP,
per-launch auth when none was supplied, `KILO_NO_DAEMON=1` and its launcher's PID.
The native shell hook clears the two managed topology variables for ordinary
descendants. Native credential projection remains the product's responsibility.
Unsupported executable layouts and incompatible managed topology flags fail
before launch. A bare executable override and its resource directory retain
their distinct native resolution rules.

## Ownership and delivery

Ordinary plugin loading exposes the registered generic skill but creates no
Sessionbus tool, Peer or endpoint. Managed activation gives the TUI ownership of
the private action endpoint and separate exact-native-session Peers. Previously
selected sessions remain addressable. An actual tool call, including a native
child, establishes identity from its own native context rather than current UI
selection. Deletion/disposal retires the affected owners and joins their work.

Server instances subscribe to a process-local BroadcastChannel before checking
the empty readiness marker. The TUI creates and closes the marker after listen,
then broadcasts a wake. Each wake rechecks the marker; message content conveys
no authority. Cancellation closes the subscription and joins the current check.
This removes the asynchronous macOS filesystem-watcher startup window without
a polling timer, extra process or dependency. Native TUI and server Workers must
share the process; ordinary inactive launches and lane mode create no channel.

Interactive delivery checks exact-session status and pending native questions
and permissions before native submission, including a final live check. Blocked
input remains unsent; the wrapper never answers an interactive blocker. Idle
handoff uses native `prompt_async` without synthesizing a native message ID.
The native API has no atomic check-and-submit operation, so a new blocker can
still cross the final check. `written` means API acceptance, not consumption.

Kilo lanes stage ordinary idle and active input as `queued_for_next_turn` until
an explicit Run. The existing Worker handles seeded input. One synchronous
native message request owns the Run, with no wrapper silence deadline. Native
history and terminal predicates distinguish completion, interruption, errors,
internal summaries and the bounded native plan-stop case. No queue or failed
write is replayed after restart.

The lane accepts the selected default permission policy and rejects unattended
native questions/permissions. It does not synthesize a wildcard permission
grant. Explicit native agent/model selection belongs to the Open request;
provider availability depends on that native process's actual config and
environment, not another TUI's displayed model. An omitted public resume Open
uses the daemon's retained initial Open recipe, including agent, model and cwd;
the new Worker explicitly supplies that selection to native prompts.

Kilo Close sends one TERM to an adopted native child, drains output and joins
owned operations. Only the supplied close context can trigger force escalation;
there is no grace timer. Active Run cancellation and native TERM drainage are
distinct events. A public close reply alone does not attest raw child wait status.
Abrupt launcher death receives no wrapper cleanup credit; stale directories may
require operator removal after owned processes have disappeared.

## Installation

The installer edits native JSON/JSONC documents in the actual permanent home.
It parses and validates the affected documents before the existing transaction,
preserves unrelated settings, and removes the historical Kilo plugin only at
the recognized exact-byte identity. Local package directories register their
single validated rendered skill through native `skills.paths`; native array
replacement order selects the effective target document. Remote/npm/tarball
specifier maintenance remains plugin-only without a local skill root.

No skill is copied into a native skill directory. A fresh native instance loads
the registration; maintenance does not restart existing native processes. Remove
owned registrations before deleting their package directory, whose manifest
proves ownership. There is no journal, cache resolver or alternate acceptance
installation.
