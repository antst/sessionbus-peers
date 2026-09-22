# Codex extraction record

Source baseline: `710e5d33369cba4fb9468cd24fea0fe844a0219d`.
This repository retains the original GitHub identity, history, issues, tags and
release assets. Other products were copied in full to the organisation before
pruning. Deleted product paths remain in those copies and this Git history.

All 74 protected Codex/common files and 160 original Go test functions are
preserved across this repository and the pinned common module. Each manifest
entry records its current repository/path.
Runtime Go changes are only local module import relocation plus gofmt import
ordering. Native plugin identity, permission strings, protocol handling, SDK
version, install locations, aliases and skill bytes are unchanged. Codex README
changes only its bootstrap URL. Root documentation now points to sibling repos.

Product-specific root tests, version guard and CI were scoped to Codex. The
version guard no longer requires Claude's manifest. Codex's tag/base-version
checks, native plugin staged version, checksum/installer tests, all public-version
cases and four-platform release archives remain. Shared host/MCP/version/socket
helpers are now imported from `github.com/sessionbus/peer-common` at
`v0.0.0-20260922143100-eb655f686e44`. Their original bodies/tests are
preserved at commit `eb655f686e4456a4c1121054763318e3d27e89b0`.
The linker stamps the Codex release/revision into the shared version helper,
so `codex-peer --version` identifies this product build, not the library commit.

The old Codex lane reference formerly under Claude's held docs is copied
byte-for-byte to `docs/designs/codex-0.5.0/legacy-lane-skill`. Cross-product
historical release and common design notes remain historical. Live implementation,
commands and plugin payloads contain Codex only. The legacy cleanup utility is
retained unchanged in peer-common as an explicitly invoked migration tool, not
product runtime; its local documentation links to the pinned source.

Local validation before artifact creation: all Go tests and race tests PASS;
vet, golangci-lint, actionlint, historical installer fixture and diff check PASS.
A separate preservation comparison verified every protected file against the
baseline after only the permitted import/README URL normalization.

The earlier a97 extraction was installed and reinstalled successfully. That
installation did not include this common pin and ran no native model tests.
Installed consumer validation is pending and must identify the new commit/binary.
Do not treat these local checks or earlier acceptance as an installed pass.

Compatibility installer scripts for the other seven products are retained solely
for previously published raw links. Default/latest resolves to the immutable
combined v0.5.3 assets retained here; explicit historical tags/mirrors keep their
meaning. Checksum, archive-role and installation checks are unchanged and run
for all eight entrypoints. No other-product runtime, plugin or packaging code
is restored. Do not enable publication that overwrites the inherited combined
development checksums before migrating those compatibility users.
