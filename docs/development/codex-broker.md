# Codex interactive broker checkpoint

The selected per-launch broker keeps native Codex responsible for TUI thread
selection and the only initialize exchange. It multiplexes one native App Server
stdio connection with internal calls, and forwards native server requests back
to the TUI. No private home, global server or native thread precreation is added.

The transport dependency is github.com/coder/websocket v1.8.15 (ISC). Its module
has no third-party runtime dependencies and supports server-side fragmentation,
control frames and contextual I/O. Source: https://github.com/coder/websocket and
https://pkg.go.dev/github.com/coder/websocket@v1.8.15. Native-compatible message
bound: 128 MiB. Compression is disabled. The package remains one Go artifact;
final linked binary delta and total installed files must be recorded when main
is wired, before installed acceptance.

Both request domains have 256 pending correlations. Internal cancellation leaves
the correlation until response or transport close; no retries or replay. Native
server request IDs are encoded into a separate TUI ID domain, including resolved
notification references. Native resolution releases its live slot immediately; a late response in the
valid mapped namespace is discarded without a tombstone. Native request IDs
use the server-lifetime monotonic counter. This drain does not prove an ID
was previously issued.
Outbound queues also have a 256-frame bound. Queued and in-write serialized
payloads plus retained original ID/key bytes share one 256 MiB byte budget
across both directions; exhaustion closes the transport. Response correlations
retain method and ID only, never arbitrary request params. Incoming decoding
and the currently serialized frame are transient allocations bounded by the
native message limit; the byte budget is not a whole-process RSS promise.
These are admission/resource bounds, not answer storage.

Supported config-mirror forms before native `--`: `-c KEY=VALUE`,
`--config KEY=VALUE`, `-cKEY=VALUE`, `-c=KEY=VALUE`, `--config=KEY=VALUE`.
Values stay byte-for-byte argv strings; TOML is never parsed by the wrapper.
A separate option-looking token is not consumed as a config value. Native
validation failures are preserved. A value's RHS may contain literal `-c`,
`-g` or `--remote` without being scanned for flags. Every other native flag,
including profile selection, remains TUI-only. Native remote projection omits
some server settings; this does not promise full standalone profile/plugin/MCP
transport equivalence. Explicit caller remote selection conflicts with managed
broker ownership. Repeated group forms and --resume/--yolo aliases are wrapper
syntax; native `--` stops wrapper interpretation.

The launcher consumes initial `-n`/`--name` and names only the first correlated
TUI selection through native name/set. The broker owns one Connection/Caller
per native loaded thread; public MCP calls use native per-request metadata.
Startup-ready plus the exact installed plugin tool catalog gates publication.
A later native MCP failure withdraws tool/delivery admission; native thread/closed
or server loss closes the row. Forwarder catalog EOF has no identity to withdraw.

Current source checkpoint: mux, stdio/WebSocket transport, OS parent watches,
launcher, resident owners, common metadata endpoint and controlled tests. Linux parent watch is exercised across an
actual Go test helper exec and exit. Darwin is compile-checked here; its same
controlled process test must execute on Darwin CI. The common main still needs these entry functions wired by the package owner.
This source checkpoint is not an installed compatibility claim.
