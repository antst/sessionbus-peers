---
description: Explain the Claude interactive candidate's installed package, native plugin and Sessionbus diagnostics
---

Check the actual candidate installation using the README and native diagnostics.
Do not run an automatic repair, probe loop, login change, installer or service
restart. No diagnostic command is embedded in this document.

Confirm the Node/npm executable paths and that Node supplies `process.execve`.
Confirm `claude-peer` and `sessionbus-claude-mcp` resolve to the intended
`@sessionbus/claude` installation. Inspect native plugin inventory and its
reported status using Claude's own commands. Do not print configuration dumps,
credentials or environment values.

When an acknowledged peer exists, the single `sessionbus` tool's `list` action
can report bus visibility. Before a usable native report, absent presence is
expected; plugin installation or an MCP process alone is not publication.
Report actual native hook/plugin or bus errors without readiness polling.

Connection loss ends this integration instance. A written result proves local
write completion only. The candidate does not supply Claude lane mode; older
bundled lane preflight scripts are not candidate readiness checks. Installed
first-contact acceptance is separate from offline package tests.
