# Sessionbus OpenCode plugin

Install this package in exactly one supported OpenCode plugin location. The
same plugin serves lane and peer mode: a lane uses the wrapper's private socket,
while an interactive session publishes one peer from native lifecycle events.

`sessionbus-opencode-install` transactionally adds the exact package specifier
to the `plugin` array in the user OpenCode config. During development, pass a
product-verified local or tarball specifier:

```sh
sessionbus-opencode-install --specifier file:/absolute/path/to/sessionbus-peers/opencode
```

Run `sessionbus-opencode-install --remove` to remove the managed entry. The
installer follows OpenCode's native global-config selection, removes its owned
string or tuple entry from every merged JSON/JSONC file, and preserves comments,
config keys, and unrelated plugin entries. OpenCode resolves the configured
package; the installer does not copy plugin source into its config directory.

The plugin registers the `sessionbus` tool and uses OpenCode's v2 session API
for native delivery. OpenCode must run without `--pure`, which disables external
plugins.

The package pins an immutable `@sessionbus/kit` commit preview in
`package.json` and `package-lock.json`. The initial split publishes
this OpenCode package only through pkg.pr.new previews; it does not claim a
stable `@sessionbus/opencode` registry release. A first registry version must
be published manually before trusted publishing can be configured.

The `list` result includes `self_info` with the bound caller's canonical
`session_id`, optional `name`, `product`, and `groups`. It stays the originating
caller when querying another host or filtering out its row. Use that ID to
recognize self. Older daemons can omit it; names and row order are not an
identity fallback.

The managed Go launcher uses the native authenticated loopback HTTP transport.
Caller topology flags are reserved; the native 1.18.29/1.18.30 network options
(`hostname`, `port`, `mdns`, `mdns-domain`, `cors`) declare no short aliases.
Existing nonempty `OPENCODE_SERVER_*` authentication values are preserved;
otherwise the launcher generates a private per-launch password. These are
native environment variables and can be inherited by native shell-tool children.
The Sessionbus launch binding is snapshotted and scrubbed separately in each
native plugin context. Local-key Sessionbus transport is unsupported by this
build and fails before managed launch.

SIGTERM and SIGHUP cancel managed lifetime, join the direct native child, and
remove the private launch directory. SIGINT remains a native TUI action.
Abrupt launcher SIGKILL can leave the native TUI, its peers, and the unique
runtime directory alive; no extra supervisor or recovery process is installed.
