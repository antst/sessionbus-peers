# Codex wrapper

`codex` retains the native Codex implementation and its regression tests.
Shared host/MCP/version helpers are imported from the exact `peer-common`
version pinned in `go.mod` and `go.sum`; there is no local copy or workspace
replacement. See [the preservation record](../docs/migration/PRESERVED-FILES.json)
for the original source paths and their current locations.
