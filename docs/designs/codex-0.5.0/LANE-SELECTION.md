# Codex lane implementation selection

2026-09-09. Source implementation is released from peers `f5dd343`, using merged public Go SDK `d11fa7433694c077f57b1f0af05a48325b464516`. Authority is REQUIREMENTS.md and the completed shared lifecycle contract. Native citations and retained acceptance are in lane-delta-dev1/REPORT.md and its source binding; this selection does not reinterpret those rows as new runtime proof.

## Selected ownership

One daemon-spawned Go Worker owns one native App Server stdio child and one prebound tool endpoint. The native-kept MCP process is a stateless forwarder to that endpoint: no second bus hello, Caller, scheduler or result cursor. The shared Worker owns seeded/direct run admission and retained results; the daemon owns persistence, parent lifetime and auto-close. No SessionLock, Handoff FIFO or receipt-driven retry survives.

| Element | Required invariant / owner | Earlier implementation |
|---|---|---|
| Native App Server child | Native IDs, thread configuration, run/steer/interrupt and tool protocol stay product-owned | R1 used these APIs through shared supervision; current lane already uses a direct child |
| Shared Worker/Caller | One run slot and one ordered result cursor, including message-started work | Replaces Handoff and old Caller-local consuming handles |
| Tool endpoint + native MCP forwarder | Native tool process reaches the sole Worker; explicit native env allowlist carries endpoint metadata | Reuses the measured Claude lane ownership pattern, not Claude's hooks or stream frames |
| Cancellable native transport | Native replies, server requests and notifications can progress while a public tool or receipt is pending | Retain pipe/drain mechanics; remove blanket request rejection and lock-dependent child helper assumptions |
| EOF close | Preserve native saved history and join the owned product before resource retirement | Deliberately removes current unconditional thread/archive side effect; shared auto-close is lifetime policy, not native history archival |

## Native boundary decisions

- Open starts no model work. Obtain native identity and confirmed name through the native API. Require the exact-thread intended MCP startup event, matching catalog/tool response and actual forwarder readiness; retain early events and fail truthfully if startup fails. Do not poll a catalog or treat catalog presence as connection readiness.
- Direct text and delivery seed enter the same Run callback. Use native turn/start once and correlate its response/events independently of the bus RunRef. Report native admission outside the stdout reader and state mutex. A later terminal cannot undo an acknowledged admission.
- Active delivery uses expectedTurnId with native turn/steer. A matching success remains injected across terminal arrival. Remove the post-ack fallback to queued input.
- Idle stage uses native thread/inject_items, with its exact raw user ResponseItem. Its empty reply does not identify active versus idle admission. Track the existing native/shared run boundary and return uncertainty for unclassified overlap; never create a queue or infer scheduling from a later reply. One installed native staging composition remains required because the retained Handoff tests do not prove it.
- Omitted typed approval policy adds no override. A supplied permission_mode is a native approval-policy string forwarded as approvalPolicy, with accepted forms described by Codex's current native API. Do not translate an explicit unrecognized string such as default into omission or never; preserve native validation. Model and reasoning-effort fields likewise use native API fields. Lane arguments mean ordered native App Server command arguments, as stated by describe, not a wrapper table translating arbitrary TUI options. Interactive CLI aliases remain a separate required boundary.
- Native server requests must be dispatched explicitly. Required MCP/code-mode tool exchanges use their actual native request/response protocol; do not disable code mode to avoid them. A human approval request without a supported recipient returns an explicit unsupported-client-exchange error, never a manufactured user denial or automatic approval. Caller-selected native policy remains the route for unattended work.
- Successful Open transfers native lifetime away from the completed Open context. Pending/failed Open and unexpected bus/forwarder/native failures still abort owned resources. Normal Close ends stdin, drains stdout/reports and waits for direct native exit before cleanup. Do not call thread/archive or delete native history on ordinary close. The daemon's forget flag removes its own resume row; it is not permission to delete product history.

## Source inventory and order

1. Extract only the accepted generic Sessionbus MCP/action/cancellation engine from Claude into `wrappers/mcp/sessionbus.go` and a focused test file (split the action description into `sessionbus_tools.go` if needed). Keep Claude's native report handling in Claude via a small explicit callback boundary. Preserve real Connection/cursor/cancellation/EOF tests. Do not migrate unrelated product engines or revive the older generic server's dropped-notification behavior.
2. Change `wrappers/codex/{codex,app,arguments}.go` and their tests: direct shared Run, native receipt correlation, no Handoff/lock/implicit approval override, cancellable transport and explicit server-request handling. Preserve already proved native settings/terminal/interrupt tests. Add the accepted-after-terminal steer regression and stage/start boundary regressions.
3. Add `wrappers/codex/lane_endpoint.go` and focused tests for the sole-Worker forwarder route and native child lifetime. Wire private forwarder dispatch through `cmd/codex-peer/main.go`; public token mode stays in the same binary.
4. Enable SupportsMessageRun only after the seeded path and actual Worker tests are complete. Test receipt/terminal progress, bus loss, failed Open, completed-Open survival, stdout drain, forwarder EOF, cancellation/later collection and done/unavailable acknowledgement with controlled events.
5. Package both modes through one eventual native plugin/private-binary layout. The exact native plugin registration and interactive activation contract are the remaining shared packaging decision; do not create a lane-only skill, installer, private config or alternate product home to get ahead of it. Implement source steps now while that bounded decision is completed.

As soon as the shared package/open/endpoint is runnable, install it in the real umka installation and iterate there. Reuse prior shared-policy acceptance rather than repeating every permutation. Required new composition includes actual no-input Codex open/tool readiness, native staging, seeded wake, native parent generic-tool lane lifecycle and pointer collection, native policy/error behavior, normal close/loss and same-install interactive regression. Claude also gets a same-install regression because the MCP engine extraction is shared. No native baseline rediscovery is authorized by this plan.

## Front-end comparison and package selection

The bounded `interactive-input-dev2/DYNAMIC-TOOLS.md` comparison closes the optional
native dynamic-tool alternative. Native thread/start can register Sessionbus as a
dynamic tool without MCP; resume/fork cannot add that definition to an arbitrary
existing thread. Definitions inherited from a compatible saved thread do not
satisfy ordinary resume coverage. Therefore retain the selected MCP route for
both modes; do not introduce a second dynamic-tool route or manufacture a new
thread to make an existing resume work. The valid generic extraction at
`9270b69e9072cfb845fcf93cc9a0eab1a080146f` remains subject to independent review.

Select one native plugin `codex@sessionbus-peers`, with one generic Sessionbus
skill and one MCP server. The permanent executable is
`~/.local/libexec/sessionbus/codex/bin/codex-peer`, public symlink
`~/.local/bin/codex-peer`, and private basename alias `codex-peer-mcp` beside the
executable. Both modes use this same artifact. The permanent marketplace payload
lives at `~/.local/share/sessionbus/codex/marketplace`; it contains a product-local
plugin manifest, `.mcp.json`, and generic skill, with marketplace metadata under
`.agents/plugins/marketplace.json`. No top-level Claude marketplace or product-
specific lane skill is introduced.

Use the exact native registration recipe and source constraints in
`interactive-input-dev2/SUPPLEMENT.md`: native `plugin marketplace add`, then
`plugin add codex@sessionbus-peers --json`, then persistently set only that
plugin's enabled key to false in the real user config, preserving other values
and config symlink behavior. Add enables the plugin, so every re-add/update must
restore this documented disabled setting. Capture the returned `installedPath`
instead of guessing a cache location. Native plugin removal and removal of the
owned permanent binary/assets are the documented uninstall route. Do not remove
other configured plugins or rewrite unrelated settings.

The native write mechanism is selected from the exact retained source binding
in `dev1-package-config-source/REPORT.md`: a same-binary private installer entry
initializes a direct native App Server, sends one `config/value/write` with
`keyPath: plugins."codex@sessionbus-peers".enabled`, `value: false`,
`mergeStrategy: replace`, and no filePath. Native code owns real user-layer
selection, quoted-key parsing, symlink handling and document preservation. No
TOML rewrite/dependency is needed. Require returned status `ok`; `okOverridden`
does not establish ordinary inactivity. No thread/model input is involved.

Render the actual absolute private alias into `.mcp.json`; do not assume an
unsupported plugin-root command substitution. Explicit `env_vars` carries only
the selected launch groups, actual bus socket and prebound control endpoint.
Native MCP startup does not inject a thread ID: `_meta.threadId` on calls and
exact native App Server events provide identity, never helper PID/order or an
inherited ancestor variable. The lane endpoint accepts only its settled native
thread's calls. A forwarder connection alone is not a native identity report.

Managed native App Server activation prefixes
`-c features.plugins=true -c 'plugins.codex@sessionbus-peers.enabled=true'`
before caller-supplied native arguments; preserve those arguments and native
precedence. Caller configuration that disables the required tool must produce a
truthful failed Open, not a second hidden override. Interactive activation must
also account for the TUI's config view; its transport/argv boundary remains the
separate unresolved interactive selection, not something this packaging choice
silently settles.

Implement this common package and real installer now alongside lane source work.
It does not require waiting for a second installer or a private testing route.
Install/reinstall on umka as soon as the selected no-input lane Open is runnable;
document its measured status without claiming the pending interactive rewrite
complete. No additional permission or per-install approval step is introduced.

### Installed activation correction (2026-09-09)

The first no-input installed Open at ebd5c0a reported the required plugin server absent. Retained native 3d2ee51 utils/cli/src/config_override.rs:48–89 preserves CLI key quotes, and config/src/overrides.rs:18–22 splits keys literally on dots. The CLI activation key therefore has no embedded quotes: `plugins.codex@sessionbus-peers.enabled=true`; the caller-disable example is `plugins.codex@sessionbus-peers.enabled=false`. Shell quoting the whole argument is fine. The installer config/value/write API has a different, quote-aware key parser and retains `plugins."codex@sessionbus-peers".enabled`. This corrects the earlier CLI literal; no user-argv rewriting or readiness relaxation. Installed confirmation follows the corrected build.
