# Sessionbus peers development prerelease

This development/beta build publishes all eight peer archives for Linux and macOS, on amd64 and arm64. It adds Pi and OMP to Claude, Codex, Grok, Qwen, OpenCode, and Kilo. DSH is packaged separately.

## Changes

- Claude interactive messages restore the structured envelope used by its compact sender display, instead of displaying native peer-origin prose as an ordinary user message. Native model-input guidance remains Claude's own.
- Codex, Claude, Grok, Pi, and OMP interactive owners reconnect after public daemon loss or initial absence while preserving the native session. Old calls and deliveries are not replayed. Supersession and native exit remain terminal. OMP Task child peers reconnect independently; lane Workers retain their single connection lifetime.
- Shared MCP and Pi/OMP native tool declarations expose the protocol's closed argument fields. In particular, send accepts message and target/targets, not summary. The SDK retains action-specific validation.
- Worker startup and service honor cancellation through the updated Go SDK. A Worker connection remains a single lifetime and does not reconnect.
- Installers select latest stable, falling back to the published development prerelease only while no stable release exists. Explicit versions and mirrors remain supported; checksums are verified before installation.
- Pi and OMP include managed native identity, interactive launch, lane Run, staged delivery, and joined process/private bridge ownership.

## Preview status

Pi and OMP remain preview integrations. Installed evidence establishes Pi normal Run, delivery/Forget, interactive replacement, and startup/resume; OMP zero-input Worker/interactive and core normal Run. Some original fixture runs failed on verifier assumptions and were assessed from preserved evidence; those are not reported as full fixture passes. The detailed record is in [the design and acceptance record](../designs/pi-omp-0.5.0/DESIGN.md).

The newly integrated reconnect and schema fixes have deterministic and process-fixture coverage, but fresh installed-product validation is still in progress. Pi admitted interrupt/healthy recovery and OMP staged delivery/Forget remain selected work before removing their preview designation; they do not block a stable peers release that explicitly retains that designation. Qwen interactive daemon reconnection remains outstanding: after daemon loss that integration can still require a fresh launch/resume. An OMP native extension writing arbitrary stdout can also invalidate the RPC connection.

This prerelease is available now for testing; publication does not mark the remaining stable acceptance work complete. No interrupted action is retried automatically. Native vendor products, credentials, and the Sessionbus daemon must already be installed separately.
