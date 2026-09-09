# Sessionbus product peers

This repository contains the Sessionbus wrappers, peer commands, plugins,
skills, installers, and product facts. The daemon and public SDKs live in
[`antst/sessionbus`](https://github.com/antst/sessionbus); this repository uses
the public Go SDK and the pinned public JavaScript kit.

## Install 0.5.0 from source

Install the daemon and public SDK commands from their repository:

```sh
git clone https://github.com/antst/sessionbus.git && cd sessionbus && GOBIN="$HOME/.local/bin" go install ./bus/cmd/...
```

`go install <pkg>@version` is not available for that module because its
`go.mod` carries a `replace` directive. Clone it and install from the checkout
instead.

Install the product peers from this repository:

```sh
git clone https://github.com/antst/sessionbus-peers.git && cd sessionbus-peers && go test -race ./... && GOBIN="$HOME/.local/bin" go install ./cmd/codex-peer ./cmd/grok-peer ./cmd/qwen-peer ./cmd/opencode-peer
```

That command installs `codex-peer`, `grok-peer`, `qwen-peer` and
`opencode-peer` in `~/.local/bin`. The retained Go Claude lane source is held
and is not installed under the competing `claude-peer` name.

Claude's interactive candidate is the separate `@sessionbus/claude` Node
package in [`claude/`](claude/README.md), version 0.5.0-interactive.0. It owns
`claude-peer` and `sessionbus-claude-mcp`. Follow its README for the separately
sequenced native installation and removal. This is an interactive-only
implementation branch; installed first contact has not run and token mode is
explicitly unavailable. Do not register it as the daemon's Claude lane command.

Codex's `scripts/codex-mcp` receives `$HOME/.local/bin/codex-peer` as its
exact path; Grok's native entry defaults to `$HOME/.local/bin/grok-peer`;
Qwen's MCP manifest invokes `qwen-peer` from PATH; Sessionbus uses
`opencode-peer` for the retained OpenCode lane source.

Complete each product hookup using only the retained integration:

- Claude: use the interactive candidate recipe and scoped status in [`claude/README.md`](claude/README.md).
- Codex: source `scripts/codex-mcp`, then run `install_codex_mcp "$(command -v codex)" "$HOME/.local/bin/codex-peer"`.
- OpenCode: install the pkg.pr.new preview rooted at `opencode/`, then run its `sessionbus-opencode-install` executable from `bin.mjs`.
- Grok: install or register the `grok/` plugin directory.
- Qwen: install or register the `qwen/` plugin directory.

Product evidence and constraints are in `docs/products/claude.md`,
`docs/products/codex.md`, `docs/products/opencode.md`,
`docs/products/grok.md`, and `docs/products/qwen.md`. The Claude plugin's
bundled skill references are being brought up to date in the first post-split
documentation unit; this README is the installation authority until then.

## Build and test

Go 1.24 is required. Build the peer binaries from source:

```sh
GOWORK=off go mod download
GOWORK=off go build ./cmd/...
GOWORK=off go test ./...
```

To install the commands into a private prefix:

```sh
GOBIN="$PWD/bin" GOWORK=off go install ./cmd/codex-peer ./cmd/grok-peer ./cmd/qwen-peer ./cmd/opencode-peer
```

No binary release workflow is part of the initial split. The commands are
built from source until a separately reviewed release workflow exists.

The OpenCode integration is rooted at [`opencode/`](opencode/). It is published
only as a [pkg.pr.new](https://pkg.pr.new/) preview in the initial repository;
`@sessionbus/opencode` is not claimed as a stable registry release. Its first
registry version must be published manually before npm trusted publishing can
be configured in a later reviewed change.

Development checks also require Node.js 24 for the Node packages and
`golangci-lint` v2.12.2 for the repository lint gate.

## Repository boundaries

- `wrappers/host` and `wrappers/mcp` remain shared by the retained Go sources.
- Claude interactive uses five Node modules and public Connection/Caller; it has no Go interactive backend.
- `wrappers/<product>` and `cmd/<product>-peer` contain the Go adapters and
  commands.
- `claude/`, `grok/`, `qwen/`, `opencode/`, and the retained plugin manifests
  contain product integration assets.
- `docs/products/` preserves verified facts and immutable historical
  citations.

The module has no filesystem replacement or workspace file. Cross-repository
development uses an untracked workspace outside both repositories; CI always
runs with `GOWORK=off`.
