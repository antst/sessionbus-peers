# Codex extraction record

Source baseline: `710e5d33369cba4fb9468cd24fea0fe844a0219d`.
This repository retains the original GitHub identity, history, issues, tags and
release assets. Other products were copied in full to the organisation before
pruning. Deleted product paths remain in those copies and this Git history.

All 74 Codex/common files and 160 original Go test functions are preserved.
Runtime Go changes are only local module import relocation plus gofmt import
ordering. Native plugin identity, permission strings, protocol handling, SDK
version, install locations, aliases and skill bytes are unchanged. Codex README
changes only its bootstrap URL. Root documentation now points to sibling repos.

Product-specific root tests, version guard and CI were scoped to Codex. The
version guard no longer requires Claude's manifest. Codex's tag/base-version
checks, native plugin staged version, checksum/installer tests, all public-version
cases and four-platform release archives remain. Shared host/MCP/version/socket
helpers stay local unchanged until a separate pinned common-module extraction.

The old Codex lane reference formerly under Claude's held docs is copied
byte-for-byte to `docs/designs/codex-0.5.0/legacy-lane-skill`. Cross-product
historical release and common design notes remain historical. Live implementation,
commands and plugin payloads contain Codex only. The legacy cleanup utility is
retained unchanged as an explicitly invoked migration tool, not product runtime.

Local validation before artifact creation: all Go tests and race tests PASS;
vet, golangci-lint, actionlint, historical installer fixture and diff check PASS.
A separate preservation comparison verified every protected file against the
baseline after only the permitted import/README URL normalization.

Installed validation is pending and must identify the actual new commit/binary.
Do not treat these local checks or earlier acceptance as an installed pass.
