# Remove older Agent Sessions installations

The migration tool recognizes the v0.3/v0.4 host layout, older marketplace
payloads, standalone Go development binaries, and the direct MCP registration
that invoked `codex-peer mcp`. It preserves the current Sessionbus installation.
The previous `remove-host` script assumes all peer aliases still belong to the
old host and must not be used after some products have migrated.

From this checkout, inspect first, then apply:

```sh
go run ./scripts/cleanup-legacy
go run ./scripts/cleanup-legacy --apply
```

For a development host without Go, build the standalone maintenance executable
on your build machine and copy it to the host. It has no runtime dependencies:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o sessionbus-cleanup-legacy ./scripts/cleanup-legacy
./sessionbus-cleanup-legacy --apply
```

Run as the normal login user. The tool uses the real home and native product
commands; it refuses alternate `CODEX_HOME`/`CLAUDE_CONFIG_DIR`. Linux service
and executable retirement is supported. On macOS an existing legacy service
or executable tree must be retired separately; the tool reports this before
performing any cleanup. Native product CLIs must support the installed plugin
management commands. This is a migration to the current native versions, not
an uninstaller for every historical vendor CLI.

The preview lists the exact actions. Apply removes the old Codex MCP/plugin/
marketplace registrations, old Claude plugin/marketplace registrations and
orphaned user enablement key, old Qwen/Grok registrations, and the recognized
auto-loaded OpenCode plugin. Native uninstall commands own their asset removal;
their user registration files are snapshotted where listed in the plan. The
tool then quarantines inactive legacy service definitions, aliases that still
target the old host, Go binaries with the old module identity, old plugin-cache
namespaces, and the old host payload. The retired Node Claude module link can
also be removed without running npm's bin unlink against the new Go launcher.

Directly managed files move to
`~/.local/state/sessionbus/legacy-cleanup/<timestamp>/files/`; config snapshots,
the action plan and completed action count accompany them. Config snapshots
are private (0600) and may contain secrets. No automatic whole-config rollback
is attempted: restoring an old config after later edits could undo user work.
Native uninstalls can remove cached assets; these can be reinstalled from the
old release if needed. Rerunning after successful cleanup is a no-op.

The tool does not stop active legacy sessions, purge transcripts/state/config,
search arbitrary evidence directories, or delete binaries on filename alone.
It refuses active old services/executables and indirect installation parents.
Finish those sessions first. An unrecognized executable is printed as `KEEP`
for review. Current `sessionbus*` services, product binaries, native vendor
installs, current plugins and unrelated config keys remain intact.

UMKA IS A DISPOSABLE DEVELOPMENT HOST. Run installation and migration checks
against its real permanent installation, login home and configuration. Temporary
unit-test fixtures verify destructive boundaries; they do not replace the real
host check.
