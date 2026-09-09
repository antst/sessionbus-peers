# Codex with Sessionbus

This Go candidate gives a Codex lane one native App Server session and one
Sessionbus worker. The common package also supplies the private MCP entry point
used by the interactive broker. Interactive broker integration and installed
Codex acceptance are still being completed at this checkpoint.

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

## Remove

```sh
"$HOME/.local/libexec/sessionbus/codex/uninstall"
```

This runs native `plugin remove codex@sessionbus-peers` and removes only the
owned package, marketplace files and matching public symlink. The native
marketplace registration may remain; remove that registration with the native
marketplace command if desired. Other plugins and native history remain intact.
