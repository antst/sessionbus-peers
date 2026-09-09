# Codex with Sessionbus

This Go package supports interactive Codex and daemon-owned Codex lanes through
one executable and one native plugin. Installed checks cover native identity,
public tools, parent-to-lane work, delivery, resume and process cleanup. The
acceptance ledger distinguishes those observations from controlled race tests.

## Install from an archive

Codex and the Sessionbus service must already be available in the real login
environment. Build an archive on a development host:

```sh
scripts/package-codex ./dist
```

Copy the matching OS/architecture archive to the target host, extract it, and
run its installer as your ordinary user:

```sh
mkdir codex-peer-package
tar -xzf codex-peer-linux-amd64.tar.gz -C codex-peer-package
./codex-peer-package/install
```

The installer replaces `~/.local/libexec/sessionbus/codex/bin/codex-peer` and
links `~/.local/bin/codex-peer` to it. Private MCP, broker and installer aliases
refer to that same compiled artifact. Put `~/.local/bin` on your login PATH if it
is not already there. The target host needs no Node.js, npm or Go runtime.

The owned marketplace lives at `~/.local/share/sessionbus/codex/marketplace`.
The installer uses native commands:

```sh
codex plugin marketplace add "$HOME/.local/share/sessionbus/codex/marketplace"
codex plugin add codex@sessionbus-peers --json
```

The marketplace command registers its own `marketplaces.sessionbus-peers`
source entry in native user configuration. The plugin add records native
`installedPath`, then the installer uses a zero-input native App Server
connection for the single `config/value/write` request:

```json
{"keyPath":"plugins.\"codex@sessionbus-peers\".enabled","value":false,"mergeStrategy":"replace"}
```

No `filePath` override is supplied: Codex edits its real user configuration,
following native symlink handling and preserving unrelated settings. The native
add initially enables the plugin; installation is complete only after the
following disable returns `ok`. A failure is reported as installation failure.
The package record is `~/.local/libexec/sessionbus/codex/installed.json`.
The rendered MCP manifest uses the absolute private binary path and explicitly
passes only the launch metadata it needs through native `env_vars`.

Plain `codex` uses ordinary native configuration with this plugin disabled.
A managed launch prefixes `-c features.plugins=true` and
`-c plugins.codex@sessionbus-peers.enabled=true` to enable the plugin. CLI keys
split on dots without unquoting segments; the quotes used by the separate
config-write API must not be embedded in this CLI key. The launcher
keeps the caller's native arguments in order. Caller overrides retain native
precedence; disabling the required tool can make lane Open unavailable.
There is one generic `sessionbus` skill and public tool, shared across products.

Repeat the same archive install to update the permanent installation. Reinstall
replaces the owned marketplace payload, so removed skills do not remain active. The
archive includes a source-revision plugin version so native add sees the new
payload. Installation does not change the Sessionbus service or other plugins.

## Lane behavior

Use the generic Sessionbus tool's `describe` action for `codex-peer`, then
`spawn`, `start` or `run`, `status`/`wait`, `ack` and `close`. Use returned IDs.
Open confirms native identity, title and required tool readiness without a model
prompt. Resume uses native saved history; close and forget do not delete it.

Active sends require a matching native steer acknowledgment for `injected`.
Idle `stage` sends require native history insertion for `queued_for_next_turn`.
A crossed run boundary can make admission uncertain; the adapter does not replay
it. With explicit `idle_message:"run"`, an idle delivery starts one shared run.
Receipts describe admission, not model consumption. Results are read without
consuming and acknowledged separately. Persistence, automatic close and idle
message policy are independent; see the installed generic skill for their exact
fields and collection rules.

Omitted `permission_mode` inherits Codex policy. An explicit value is passed as
the native approval-policy string, without substituting `never` or a sandbox
policy. A headless lane has no human approval recipient; unsupported native
approval requests fail truthfully. Normal close ends stdin, drains native output
and waits for the direct native process. Hard failure uses forced cleanup; this
does not promise containment of arbitrary native tool descendants.

`never` prevents approval prompts; it does not grant MCP tool access. On the
installed candidate, an inherited-policy call was rejected, and a call with
`permission_mode:"never"` reported that the tool still required approval.
A caller can explicitly grant the generic Sessionbus tool through native open
arguments, without changing the installation or activation defaults:

```json
{"permission_mode":"never","arguments":["-c","plugins.codex@sessionbus-peers.mcp_servers.sessionbus.tools.sessionbus.approval_mode=\"approve\""]}
```

This grants that tool's actions, not only its read-only `list` action. Use it
only when that access is intended. The installed acceptance exercised `list`;
the grant is a native policy choice, not an approval added by the adapter.
It does not enable a disabled plugin, server or tool, or override managed
requirements. Native `strict_auto_review` has a separate policy path.

## Interactive launch

```sh
codex-peer --group team -n review
codex-peer --resume "native session selector" -g team --group review
```

Repeated `-g`/`--group` values accumulate before native `--`, wherever supplied.
`--group=value` and comma-separated values are supported. `-n`/`--name` names
the initial native selection; native Codex owns resume/picker/fork selection.
`--resume` becomes native `resume`; `--yolo` becomes
`--dangerously-bypass-approvals-and-sandbox`. Other arguments keep their bytes
and order. After `--`, all arguments remain native operands.

The launcher starts one Go broker and execs the native TUI. That broker owns
one native App Server, two local endpoints and a Connection/Caller for each
loaded native thread. MCP helpers forward native thread metadata to those
owners. The launcher prefixes its native activation and `--remote` transport;
a caller's own `--remote` conflicts and is reported as an integration error.
The broker uses an OS parent-exit watch and closes native stdin on shutdown,
drains output and joins its owned child. It adds no shared server service,
polling loop, replay queue or native history lock.

Loaded native threads have separate peer rows. Rename updates the same row;
fork adds a new one. Clear can leave earlier threads loaded and addressable;
native thread closure or server loss withdraws their rows. Normal TUI quit and
abrupt TUI death removed the owned processes and endpoints in installed checks.
Killing the broker itself left stale private sockets and the TUI's native
reconnect screen; the bus rows and native server/MCP processes disappeared.
No individual child exit status or arbitrary-tool containment follows from
those absence checkpoints.

Native `-c`/`--config` occurrences are mirrored to App Server in order and kept
on the TUI. Other native options go to the TUI. In particular, profile options
remain on the TUI: native App Server rejects that CLI option. Server-level
profile settings and settings omitted by native remote projection do not gain
full standalone equivalence. The wrapper does not read or merge profiles.

The package includes `THIRD-PARTY-NOTICES.txt`, including the ISC notice for
the Go WebSocket implementation. No Node.js process or runtime is required.

## Remove

```sh
"$HOME/.local/libexec/sessionbus/codex/uninstall"
```

This runs native `plugin remove codex@sessionbus-peers` and removes only the
owned package, marketplace files and matching public symlink. The native
marketplace registration may remain; remove that registration with the native
marketplace command if desired. Other plugins and native history remain intact.
