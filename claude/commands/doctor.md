---
description: Explain the installed Claude peer, lane and Sessionbus diagnostics
---

Check the actual installation using the README and native diagnostics.
Do not run an automatic repair, probe loop, login change, installer or service
restart. No diagnostic command is embedded in this document.

Confirm `claude-peer` resolves to the installed Go binary under
`~/.local/libexec/sessionbus/claude`. Its adjacent plugin invokes
`${CLAUDE_PLUGIN_ROOT}/bin/sessionbus-mcp`, a private alias to the same binary.
No Node/npm runtime or global marketplace installation is needed. Inspect the current session's plugin/MCP
status with Claude's native commands. Ordinary `claude` does not activate this
package by installation alone; existing global plugins remain separate. Do not print configuration dumps,
credentials or environment values.

When an acknowledged peer exists, the single `sessionbus` tool's `list` action
can report bus visibility. Before a usable native report, absent presence is
expected; plugin installation or an MCP process alone is not publication.
Report actual native hook/plugin or bus errors without readiness polling.

Connection loss ends this integration instance. A written result proves local
write completion only. The same installed binary supplies Claude lanes. Use the public tool’s
`describe` action with `{"product":"claude-peer"}` to inspect supported open
fields; `list` alone does not prove a lane lifecycle. Follow the bundled
Sessionbus skill for authorized spawn, run or start/wait, close and resume.
Held scripts outside this plugin are historical references, not readiness checks. Installed
first-contact acceptance is separate from offline package tests.
