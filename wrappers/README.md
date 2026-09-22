# Codex wrapper and retained common support

`codex/` is the native adapter. `host/` and `mcp/` are unchanged copies of the
common support used by the pre-separation implementation. Their extraction to a
pinned shared module is a separate change; this migration does not redesign them.
The Bus dependency remains the released public Go SDK, never daemon internals.
Local imports use `github.com/sessionbus/codex-peer`.
