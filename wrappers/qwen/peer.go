// SPDX-License-Identifier: MIT
package qwen

import (
	"regexp"
	"slices"
	"strings"
)

var qwenSessionID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func environmentValue(environment []string, name string) string {
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found && key == name {
			return value
		}
	}
	return ""
}

func qwenPassthrough(argument string) bool {
	return slices.Contains([]string{"-h", "--help", "-v", "--version", "auth", "channel", "extensions", "hooks", "mcp", "review", "serve", "sessions", "update"}, argument)
}

var qwenRequiredValueOptions = []string{
	"--telemetry-target", "--telemetry-otlp-endpoint", "--telemetry-otlp-protocol", "--telemetry-outfile", "--proxy",
	"-m", "--model", "--fallback-model", "-p", "--prompt", "-i", "--prompt-interactive", "--system-prompt", "--append-system-prompt", "--output-style", "--sandbox-image", "--approval-mode", "--channel", "--allowed-mcp-server-names", "--mcp-config", "--allowed-tools", "-e", "--extensions", "--include-directories", "--add-dir", "--openai-logging-dir", "--openai-api-key", "--openai-base-url", "--input-format", "-o", "--output-format", "--json-fd", "--json-file", "--json-schema", "--input-file", "--session-id", "--max-session-turns", "--max-wall-time", "--max-tool-calls", "--max-subagent-depth", "--core-tools", "--exclude-tools", "--disabled-slash-commands", "--auth-type",
}

var qwenValueOptions = append(slices.Clone(qwenRequiredValueOptions), "--worktree")
