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

Claude's interactive candidate is one Go binary with its bundled native plugin
in [`claude/`](claude/README.md). Build its archive with `scripts/package-claude`
and follow the literal permanent user installation in that README. No Node/npm
runtime or global native plugin registration is required. `claude-peer` loads
its skill, hooks and tools only for that launch; ordinary `claude` stays ordinary.
This candidate remains interactive-only: token-selected Claude lane mode is
explicitly unavailable until the held lane work is completed. The reviewed
Node implementation and regressions remain a behavioral reference under
`docs/designs/claude-0.5.0/node-reference`, outside the installed plugin.

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
`docs/products/grok.md`, and `docs/products/qwen.md`. Held Claude lane references are outside the activated plugin under
`docs/designs/claude-0.5.0/held-lane-skills/` for later lane work;
this README is the installation authority until then.

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
- Claude interactive uses one Go artifact, public Connection/Caller, native exec and a private MCP alias; no Node runtime.
- `wrappers/<product>` and `cmd/<product>-peer` contain the Go adapters and
  commands.
- `claude/`, `grok/`, `qwen/`, `opencode/`, and the retained plugin manifests
  contain product integration assets.
- `docs/products/` preserves verified facts and immutable historical
  citations.

The module has no filesystem replacement or workspace file. Cross-repository
development uses an untracked workspace outside both repositories; CI always
runs with `GOWORK=off`.
