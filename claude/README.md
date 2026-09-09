# Claude peer — Go candidate

`claude-peer` runs your installed Claude with Sessionbus communication. The
integration is one Go binary and a small native plugin; it requires no Node or
npm. Claude, its login, permissions, configuration and history remain native.
The interactive path is accepted. The direct Go lane path is under development;
its installed Open, delivery and lifecycle checks are not yet complete.

## Install

Use the archive for your operating system and architecture. Development builds
are produced from the pinned repository with Go 1.24 or newer:

```sh
./scripts/package-claude "$PWD/dist"
```

Cross-build on a development host with `GOOS=linux GOARCH=amd64`; the target host
does not need Go. Transfer the resulting archive to the target host. From the
directory containing `claude-peer-linux-amd64.tar.gz`, install in your normal
login shell:

```sh
mkdir -p "$HOME/.local/libexec/sessionbus/claude" "$HOME/.local/bin"
tar -xzf claude-peer-linux-amd64.tar.gz -C "$HOME/.local/libexec/sessionbus/claude"
ln -sfn "$HOME/.local/libexec/sessionbus/claude/claude-peer" "$HOME/.local/bin/claude-peer"
command -v claude-peer
claude-peer --version
```

Use the matching archive filename on macOS. `$HOME/.local/bin` must be on your
normal login PATH; add it through your shell's normal startup configuration if
needed, then open a new login shell. No names or groups are installed. The
Sessionbus user service must already be installed and running through its own
installation procedure.

If replacing the earlier npm candidate, first run
`npm uninstall --global @sessionbus/claude` using that installation's actual
prefix, then install the archive above. npm is required only to remove that
old npm installation, never by the Go candidate. If the old `claude-peer` is a
regular Go binary rather than a symlink, move that exact file aside before the
link command; do not remove unrelated binaries or packages. Reinstall by
extracting the new archive into the same permanent directory and refreshing
the same public symlink. Quit integrated sessions before replacing an executing
binary. The package adds no service or global Claude plugin registration.

The installed layout is:

```text
~/.local/bin/claude-peer -> ~/.local/libexec/sessionbus/claude/claude-peer
~/.local/libexec/sessionbus/claude/claude-peer
~/.local/libexec/sessionbus/claude/plugin/.mcp.json
~/.local/libexec/sessionbus/claude/plugin/bin/sessionbus-mcp -> ../../claude-peer
~/.local/libexec/sessionbus/claude/plugin/bin/sessionbus-hook -> ../../claude-peer
```

The private MCP alias invokes the same binary as the native MCP child. Lane
startup uses the private hook alias for its initial native identity report. There is
one compiled artifact, one public command and no Node/npm dependency.

## Use

```sh
claude-peer -n NAME -g GROUP
claude-peer --resume NAME -g GROUP
```

Only a final `-g VALUE` pair belongs to the wrapper. Groups are comma-separated;
no suffix means this launch's empty groups. Native arguments stay ordered and
unchanged after the fixed activation prefix:
`--allowedTools mcp__plugin_sessionbus_sessionbus__sessionbus --plugin-dir ROOT`.
Native `-n`, resume, `--`, repeated flags and errors remain Claude's own.

Ordinary `claude` stays ordinary. The wrapper loads the whole plugin only for
this launch. An unconfigured plain nested `claude` stays ordinary; use an
explicit `claude-peer` invocation with its own `-g` for an integrated child.
The one Sessionbus skill describes the actual public `sessionbus` tool. Native
permissions can deny that tool; the allow rule does not bypass native policy.

Presence starts only after a usable native report reaches the resident owner
and the bus acknowledges it. There is no launch, first-prompt or first-turn
deadline. A prompt entered during plugin startup may be reported at that
turn's end or at a later prompt; missed reports are not replayed. A Stop report
can publish an unnamed peer; a later title report supplies its name. Before
publication, the session is absent and unaddressable. Rename is reflected at
the next report that carries its title.

`written` means local native socket write completion only, not native admission
or model consumption. A failure after possible submission remains uncertain.
There are no retries, reconnects, queues, polling or alternative delivery paths.
Unexpected bus loss ends this resident integration. Cancelled public waits
leave their result handles collectible; cancellation does not interrupt a
native turn or retract a message.

## Lane development status

The daemon launches this same binary as a token-selected lane worker. Use the
public Sessionbus tool: `spawn` with `product: "claude-peer"`, `name` and `open`,
then `run` or `start` with the returned `session_id` and `input`. `status`/`wait`
collect a started turn; `interrupt` requests native interruption; `close`
releases the lane. Resume passes that exact ID as `resume_session_id` to spawn.
No lane-specific public launcher is installed.

Open waits for native initialize, the initial root ID/title report and the
required Sessionbus tool. An idle message does not start work. A matching
native replay during the same confirmed active run returns `injected`; idle
native staging returns `queued_for_next_turn` for a later explicit run. Neither
receipt promises model consumption. An unclassified run boundary leaves
admission uncertain; write completion alone earns no receipt. Explicit runs
return the native terminal result and reason.

With native default permissions, a lane has nobody to approve an interactive
tool request. A tool can return “requires approval” and the model can still
finish its turn successfully. The adapter does not substitute `dontAsk` or a
bypass. Callers may explicitly choose `open.permission_mode` or native
`open.arguments`; those values are passed to Claude. Prefer a rule for the
specific command needed over a broad grant. Native policy remains authoritative.

Installed Open, public list, staging, active consumption, interruption,
configuration and independent lanes have been checked at their recorded scope.
A native Bash tool can have its own process group outside the worker group;
the daemon’s group kill does not directly reach that tool. The idle-worker
death check does not establish active-tool cleanup. Remaining lifetime and
combined regression rows are in progress; this is not yet full lane acceptance.

## Remove

After quitting integrated sessions, remove the exact owned launcher symlink
and package directory:

```sh
rm "$HOME/.local/bin/claude-peer"
rm -r "$HOME/.local/libexec/sessionbus/claude"
```

Check that these paths still name this installation before removing them.
Keep ordinary Claude, its history/configuration, other plugins and the
Sessionbus service. The integration creates no separate persistent data store.

The reviewed Node implementation at `9644348`, including C01–C03 regressions,
is retained under `docs/designs/claude-0.5.0/node-reference` as a behavioral
reference. It is not included in this installed archive. Existing native facts
remain version-qualified evidence; the Go translation requires its own actual
installed acceptance. Source tests alone do not establish that acceptance.
