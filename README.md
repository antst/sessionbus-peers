# Sessionbus product peers

This repository contains the Sessionbus wrappers, peer commands, plugins,
skills, installers, and product facts. The daemon and public SDKs live in
[`antst/sessionbus`](https://github.com/antst/sessionbus); this repository uses
the public Go SDK and the pinned public JavaScript kit.

`list` reports the bound originating caller in `self_info` alongside the visible
`sessions`. Compare its `session_id` with row IDs to recognize self; a filter or
remote host query does not change the caller identity. Older daemons may omit
this field, which must not be guessed from names or row order.

When updating an existing installation for `self_info`, update every product
peer first and restart managed sessions so their helpers load the updated SDK.
Then update the host daemon. Older SDKs reject the new response field; updated
SDKs also accept older daemon responses. Coordinate federated host upgrades as
well, since daemons validate forwarded responses with their embedded SDK.

## Install a product from a binary release

Install [Sessionbus v0.5.3 or later](https://github.com/antst/sessionbus/tree/main#install-binaries)
and the native product first. The daemon and peers have independent release
versions. Then run the command for each product you want:

```sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-claude.sh | sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-codex.sh | sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-grok.sh | sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-qwen.sh | sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-opencode.sh | sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-kilo.sh | sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-pi.sh | sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-omp.sh | sh
```

Pi and OMP remain preview integrations, including when distributed in a stable
peers release. See the [v0.5.0 release scope](docs/releases/v0.5.0.md) and
[development notes](docs/releases/development.md) for included fixes and remaining
preview acceptance work.

Each command installs only that peer and its plugin into your normal user
installation under `~/.local`; it does not install the native vendor product,
daemon or hub. Use your normal login shell with `~/.local/bin` on PATH. Linux
and macOS, amd64 and arm64 archives are provided. No Go or npm is needed on the
target. OpenCode and Kilo use their Go installer and small plugin in the native product's
existing Bun runtime; no separate Node or Bun installation is required. Pi's
managed extension runs inside its native product's existing Node process; OMP's
runs inside its existing Bun process. Neither adds a sidecar or global extension
registration. The OpenCode-family plugins'
only JavaScript runtime dependency is the bundled pinned Sessionbus kit.
Grok/Qwen install their native plugin globally; Claude/Codex retain their
documented managed-launch activation. OpenCode and Kilo register tiny server/TUI hooks
that stay inert in ordinary launches, plus one globally discoverable skill.

Downloads are verified against `SHA256SUMS`. By default, installers select
GitHub's **latest stable release**, or the published **development prerelease**
while no stable release exists. Rerunning the same command updates the product;
no version edit is needed. `SESSIONBUS_VERSION=latest` explicitly selects this
behavior. `SESSIONBUS_VERSION=development` always selects the rolling prerelease
from tested builds of merged `develop` pushes. A failed release lookup or asset
verification stops installation; only a confirmed missing stable release selects
development. Mirrors and explicit release tags skip this lookup.

Pin an actual release tag by setting the variable on **sh**:

```sh
curl -fsSL https://raw.githubusercontent.com/antst/sessionbus-peers/main/scripts/install-codex.sh | SESSIONBUS_VERSION=vX.Y.Z sh
```

Replace `vX.Y.Z` with a published [release](https://github.com/antst/sessionbus-peers/releases).
`SESSIONBUS_DOWNLOAD_ROOT` optionally selects a mirror with the same files.
Missing releases/checksum failures stop before installation. Inspect the
downloaded shell script first if preferred. Rerun to update that product.
Native login, permissions and history stay native. These installers package
the current adapters; the per-product facts still define their accepted scope
(including the documented activation and lifecycle limits).

Maintainers build Claude/Codex using `scripts/package-claude` and
`scripts/package-codex`. For other products use
`scripts/package-product PRODUCT OUTPUT_DIRECTORY`. The `Binary releases`
workflow builds all eight for four platforms, publishes development after its tests and builds pass, and publishes a stable release when a new `vX.Y.Z` tag is pushed. Release
archives record their exact source in the accompanying `SOURCE.txt`.

## Delivery and presence

A delivery reported as `rejected` with reason `no_receipt` means that no usable
receipt was obtained. It does not prove the message was never submitted or
consumed, including when the recipient disconnects. Preserve the delivery ID,
reason and any run reference; report the uncertainty without automatically
resending. A later connected or idle-looking row does not make replay safe.

In a `list` row, `connected` describes the Sessionbus attachment and `running`
describes a daemon-managed Run. An interactive peer's `running:false` does not
prove its native model is idle. Do not use these flags to predict delivery
admission; follow the actual receipt.

The orchestrator's installed tool declaration governs the tool identifier and
`{action, arguments}` envelope. After selecting a lane product, that product's
`describe` response and product skill/README govern its open fields, native
permission values, delivery receipts and lifecycle. `describe` lists available
fields; product documentation supplies their meaning and allowed native values.
Do not apply the orchestrator product's native options to a different lane product.

## Interactive CLI aliases

Use `--yolo` to select the native permission-bypass flag and `--resume` to
select native history. These aliases apply to interactive launches; they do
not change the typed lane `permission_mode` API.

| Peer | `--yolo` passes as | `--resume VALUE` passes as | Bare `--resume` |
| --- | --- | --- | --- |
| `claude-peer` | `--dangerously-skip-permissions` | `--resume VALUE` | Native picker |
| `codex-peer` | `--dangerously-bypass-approvals-and-sandbox` | `resume VALUE` | Native picker |
| `qwen-peer` | `--yolo` | `--resume VALUE` | Native picker |
| `grok-peer` | `--always-approve` | `--resume VALUE` (ID or title) | Most recent native session |
| `opencode-peer` | `--yolo` | `-s ID` | Requires an ID |
| `kilo-peer` | `--yolo` | `-s ID` | Requires an ID |

Native `--continue` remains separate. Tokens after literal `--`, attached
native values such as `--model=--yolo`, and the first required value of recognized native
options are preserved. Codex retains its documented special `-c` handling;
optional native selectors keep their native behavior.

## Install 0.5.0 from source

Upgrading from Agent Sessions v0.3/v0.4 or development installations? Use the
[legacy cleanup tool](docs/development/legacy-cleanup.md) to preview and remove
old integrations without unlinking the new Sessionbus peer commands.

Install the daemon and public SDK commands from their repository:

```sh
git clone https://github.com/antst/sessionbus.git && cd sessionbus && GOBIN="$HOME/.local/bin" go install ./bus/cmd/...
```

`go install <pkg>@version` is not available for that module because its
`go.mod` carries a `replace` directive. Clone it and install from the checkout
instead.

Clone and check the product sources:

```sh
git clone https://github.com/antst/sessionbus-peers.git && cd sessionbus-peers && go test -race ./...
```

Build and install each product's archive using the recipes below. An isolated
`go install ./cmd/opencode-peer` installs only a command; it does not install the
required native server/TUI hooks, pinned kit or generic skill. OpenCode uses
`scripts/package-product opencode ./dist` and the archive's Go maintenance
installer, as documented in [`opencode/README.md`](opencode/README.md). Its public
`opencode-peer` front door and lane worker share that permanent installation.

Claude's candidate is one Go binary with its bundled native plugin
in [`claude/`](claude/README.md). Build its archive with `scripts/package-claude`
and follow the literal permanent user installation in that README. No Node/npm
runtime or global native plugin registration is required. `claude-peer` loads
its skill, hooks and tools only for that launch; ordinary `claude` stays ordinary.
The same Go binary now includes the token-selected lane worker; installed
lane rows cover open, tools, delivery, interruption, normal close and same-build
interactive regression. Forced-death descendant containment remains open;
individual results and limits are in the product facts. The reviewed
Node implementation and regressions remain a behavioral reference under
`docs/designs/claude-0.5.0/node-reference`, outside the installed plugin.

`codex-peer` uses `scripts/package-codex` and the permanent installer in
[`codex/README.md`](codex/README.md). One Go binary supplies the launcher,
interactive broker, lane worker and private MCP entry. Native plugin
registration is disabled for ordinary Codex and activated per managed launch.
The earlier `scripts/codex-mcp` hookup is retained historical source and is not
the current installation route. Grok uses `scripts/package-product grok ./dist`
and the archive installer in [`grok/README.md`](grok/README.md). Its private
`grok-peer-mcp` alias shares the permanent binary with interactive and lane modes.
The global native plugin starts an inert, zero-tool helper for ordinary Grok;
managed launches activate it using the exact private native leader.
Qwen uses `scripts/package-product qwen ./dist` and the same permanent archive
installer in [`qwen/README.md`](qwen/README.md). Its lane private alias resolves
the installed sibling `qwen-peer-mcp` binary. The extension contains one generic
skill and no ordinary MCP server. Managed interactive launches supply native MCP
configuration; lanes supply it per session. The accepted interactive session-switch
limitation is documented there. OpenCode uses one Go launcher/lane worker and
small native Bun server/TUI hooks. A retained lane ID cannot register as an
interactive Peer, even after lane close; native history is not automatically
reclassified. Native-only interactive resume is a separate supported path.

Complete each product hookup using only the retained integration:

- Claude: use the bundled candidate recipe and scoped status in [`claude/README.md`](claude/README.md).
- Codex: build with `scripts/package-codex ./dist` and use the archive's installer as documented in [`codex/README.md`](codex/README.md).
- OpenCode: build with `scripts/package-product opencode ./dist` and run the archive installer in [`opencode/README.md`](opencode/README.md). The npm preview contains native hooks, not an installer executable.
- Kilo: `kilo-peer` supplies managed native sessions and daemon-owned lanes through the permanent Go installer and native-hook package in [`kilo/README.md`](kilo/README.md). See the [Kilo 7.6.2 installed acceptance and limits](docs/designs/kilo-0.5.0/ACCEPTANCE.md).
- Grok: build with `scripts/package-product grok ./dist` and run the archive installer in [`grok/README.md`](grok/README.md).
- Qwen: build the archive with `scripts/package-product qwen ./dist` and follow [`qwen/README.md`](qwen/README.md); registering only the plugin does not install the required private sibling alias.
- Pi: build the archive with `scripts/package-product pi ./dist` and follow [`pi/README.md`](pi/README.md); its fixed extension is activated only by managed launches.
- OMP: build the archive with `scripts/package-product omp ./dist` and follow [`omp/README.md`](omp/README.md); its managed extension is activated only for wrapper-owned launches.

Product evidence and constraints are in `docs/products/claude.md`,
`docs/products/codex.md`, `docs/products/opencode.md`,
`docs/products/grok.md`, and `docs/products/qwen.md`. Pi and OMP's in-progress contract and installed acceptance status
are in [`docs/designs/pi-omp-0.5.0/DESIGN.md`](docs/designs/pi-omp-0.5.0/DESIGN.md).
Held Claude lane references are outside the activated plugin under
`docs/designs/claude-0.5.0/held-lane-skills/` as historical references;
this README is the installation authority until then.

## Build and test

Go 1.24 is required. Build the peer binaries from source:

```sh
GOWORK=off go mod download
GOWORK=off go build ./cmd/...
GOWORK=off go test ./...
```

Standalone command builds are useful for development checks:

```sh
GOWORK=off go build ./cmd/grok-peer ./cmd/qwen-peer ./cmd/omp-peer ./cmd/opencode-peer ./cmd/pi-peer
```

Use the archive installers for a runnable product integration. The binary
release workflow publishes those complete archives; building a command alone
does not perform the native plugin hookup.

The OpenCode native plugin is rooted at [`opencode/`](opencode/). Its JavaScript
package is published as a [pkg.pr.new](https://pkg.pr.new/) preview;
`@sessionbus/opencode` is not claimed as a stable registry release. Its first
registry version must be published manually before npm trusted publishing can
be configured in a later reviewed change.

Development checks also require Node.js 24 for the Node packages and
`golangci-lint` v2.12.2 for the repository lint gate.

## Repository boundaries

- `wrappers/host` and `wrappers/mcp` remain shared by the retained Go sources.
- Claude interactive uses one Go artifact, public Connection/Caller, native exec and a private MCP alias; no Node runtime.
- `wrappers/<product>` and `cmd/<product>-peer` contain the Go adapters and
  commands.
- `claude/`, `codex/`, `grok/`, `qwen/`, `omp/`, `opencode/`, `pi/`, and the retained plugin manifests
  contain product integration assets.
- `docs/products/` preserves verified facts and immutable historical
  citations.

The module has no filesystem replacement or workspace file. Cross-repository
development uses an untracked workspace outside both repositories; CI always
runs with `GOWORK=off`.
