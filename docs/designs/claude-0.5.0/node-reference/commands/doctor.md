---
description: Explain the Claude interactive candidate's installed package, native plugin and Sessionbus diagnostics
---

Check the actual candidate installation using the README and native diagnostics.
Do not run an automatic repair, probe loop, login change, installer or service
restart. No diagnostic command is embedded in this document.

Confirm the Node/npm executable paths and that Node supplies `process.execve`.
Confirm `claude-peer` resolves to the intended `@sessionbus/claude` npm
installation. The native per-launch plugin uses `node` and its own
`${CLAUDE_PLUGIN_ROOT}/mcp.mjs`; there is no second public bin or required
global marketplace installation. Inspect the current session's plugin/MCP
status with Claude's native commands. Ordinary `claude` does not activate this
package by installation alone; existing global plugins remain separate. Do not print configuration dumps,
credentials or environment values.

When an acknowledged peer exists, the single `sessionbus` tool's `list` action
can report bus visibility. Before a usable native report, absent presence is
expected; plugin installation or an MCP process alone is not publication.
Report actual native hook/plugin or bus errors without readiness polling.

Connection loss ends this integration instance. A written result proves local
write completion only. The candidate does not supply Claude lane mode; held lane scripts outside this plugin are not candidate readiness checks. Installed
first-contact acceptance is separate from offline package tests.
