# Claude interactive peer candidate

`@sessionbus/claude` 0.5.0-interactive.0 adds Sessionbus communication to an
explicit `claude-peer` session. This is an interactive candidate; installed
first-contact acceptance is pending. Token-selected Claude lane mode is
unavailable. Do not register this package as the daemon's Claude lane command.

## Install

Use Node.js >=24.0.0 with `process.execve`, npm, Claude installed on the login
PATH with working native authentication, and a separately installed compatible
Sessionbus user service. The service must support optional names and `written`
receipts (bus b1d7adb or a compatible release). This package pins the reviewed
public kit preview at `https://pkg.pr.new/@sessionbus/kit@fdff89c` in its lockfile.
Go is not needed to install this Node package; the daemon is a separate install.

From this candidate repository checkout:

```sh
npm ci --prefix claude
npm install --global ./claude
```

Put npm's global bin directory on the real login PATH. Keep the source checkout
if npm links the local package to it; `npm ls --global @sessionbus/claude` shows
the installed package. The only public bin is `claude-peer`. The launcher
resolves its own package root and uses native `--plugin-dir` to load the
manifests, hooks, skill and `node ${CLAUDE_PLUGIN_ROOT}/mcp.mjs` together.
No native marketplace add or plugin install is part of this recipe.

Installing this package does not enable it globally in Claude. Run claude-peer to load its skill, hooks and communication tools for that session. Ordinary claude remains ordinary unless you explicitly configure a plugin yourself. Claude's own policy can deny a Sessionbus tool; the integration does not bypass it.

Preexisting globally enabled integrations are separate installations. This
recipe does not disable them or claim to make their hooks inactive. Establish
their exact ownership before any native migration/removal.

## Use and native permissions

```sh
claude-peer -n NAME -g GROUP
claude-peer --resume NAME -g GROUP
```

Only a final `-g VALUE` pair belongs to the launcher. Empty VALUE means no
groups; otherwise commas separate values without trimming or deduplication.
Without that suffix, this launch uses no explicit groups, even if a parent
exported groups. Claude owns `-n`, resume matching, any picker, native errors,
all other options and their ordering.

The launcher prefixes `--allowedTools mcp__plugin_sessionbus_sessionbus__sessionbus`
and `--plugin-dir <absolute package root>` before those native arguments. This
keeps activation before a caller's literal `--`; the fixed-arity plugin flag
terminates the prefix's variadic allow flag. Explicit caller disallow and
native policy still apply. No wildcard, bypass mode or permission configuration
is added. The production tool includes mutations and is not marked read-only.

Use `/sessionbus:sessionbus` for the bundled skill and `/sessionbus:doctor` for
manual diagnostic guidance. The public tool is
`mcp__plugin_sessionbus_sessionbus__sessionbus`, accepting `{action, arguments}`.
Its schema lists the public kit's actions, including list/send and supported
daemon caller operations. The hidden native identity handler is not a public
tool. A failed or denied call is reported without an alternative route.

A plain nested claude process does not receive the launcher's plugin flag and is not automatically a peer. Use claude-peer explicitly for a child that should participate, with that launch's groups. Sessionbus does not intercept or rewrite Bash commands.

## Observable costs

A Claude peer appears when its MCP owner receives a usable native session report. Until then it is absent from the roster and cannot receive peer messages. Plugin startup can miss a report; publication is not guaranteed at launch, at the first prompt or by the end of that turn. A peer can initially have no name. Native rename becomes visible at the next report that carries the title. A named session quit before any real turn is not resumable by Claude. Sessionbus does not create a turn or write history to make it resumable.

A written delivery means the local write to Claude's native carrier completed. It does not promise retention, admission or consumption. Connection loss ends this integration instance; it does not restart it or terminate the interactive Claude process. Resuming the same native ID in another integration supersedes the old bus connection; Claude can also run external concurrent writers, which Sessionbus does not lock.

Claude can update its installed executable while a session is running, so a parent and a subsequently launched child may run different versions. Sessionbus uses the installed product and does not pin or select a Claude version. The interactive candidate is not a completed Claude lane release.

These costs are qualified by [native facts](../docs/products/claude.md): first
report/startup and delivery on 2.1.265, zero-turn resume on 2.1.263, nested
identities/supersession and per-launch activation on 2.1.266. The AP01 diagnostic
proved one read-only list call under its tested configuration, not the sole
granting permission rule or all production actions. FP05 nested evidence used
a global fixture; it does not prove plugin flags are inherited. Final installed
launcher ordering, production tool permissions and FC01–FC08 remain to be run.

## Remove

```sh
npm uninstall --global @sessionbus/claude
```

The per-launch plugin directory is not registered with Claude, so no native
plugin uninstall applies to this recipe. Preserve ordinary Claude, its history
and configuration, other plugins and the Sessionbus service. Account separately
for any preexisting global integration and any native cache/data left behind;
this command is not a general configuration rollback. An already running native
session has its own lifetime; package removal is not a process-kill operation.

## Development

Run `npm ci --prefix claude`, `npm test --prefix claude`, `npm pack ./claude`,
and the repository Go checks. The package tests use local controlled transports
and a tiny executable fixture, not a real Claude or daemon. Five runtime modules
own launch, stdio dispatch, presence, native delivery and tool forwarding. Legacy lane guidance is retained outside the activated plugin at
`docs/designs/claude-0.5.0/held-lane-skills/`; it is not candidate runtime
acceptance or permission to use an old Go bridge.
