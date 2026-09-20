// SPDX-License-Identifier: MIT
package qwen

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	managedQwenServer = "sessionbus"
	managedQwenTool   = "mcp__sessionbus__sessionbus"
)

var qwenNegativeNumber = regexp.MustCompile(`^-([0-9]+(\.[0-9]+)?|\.[0-9]+)$`)

func managedQwenGrant() []string {
	return []string{"--allowed-tools", managedQwenTool}
}

// appendManagedQwenGrant puts the native array option after every caller
// argument and immediately before the native literal boundary. Qwen greedily
// consumes consecutive non-option tokens into array flags, so prefixing this
// grant would silently absorb a leading positional prompt.
func appendManagedQwenGrant(arguments []string) []string {
	position := len(arguments)
	for index, argument := range arguments {
		if argument == "--" {
			position = index
			break
		}
	}
	result := make([]string, 0, len(arguments)+2)
	result = append(result, arguments[:position]...)
	result = append(result, managedQwenGrant()...)
	return append(result, arguments[position:]...)
}

// validateManagedQwenArguments rejects only caller controls that would make
// the managed server or its one public tool unavailable. All other native
// policy stays caller-owned. Array and scalar options stop at native option
// boundaries, so a flag-looking token is never reinterpreted as another
// option's value.
func validateManagedQwenArguments(arguments []string) error {
	serverBound := false
	serverAllowed := false
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			break
		}
		name, value, attached := strings.Cut(argument, "=")
		managedArray := qwenManagedArrayOption(name)
		if managedArray != "" {
			values := []string{}
			if attached {
				values = append(values, qwenArrayValue(value)...)
			}
			for index+1 < len(arguments) && qwenArrayArgument(arguments[index+1]) {
				index++
				values = append(values, qwenArrayValue(arguments[index])...)
			}
			switch managedArray {
			case "--allowed-mcp-server-names":
				serverBound = true
				for _, candidate := range values {
					serverAllowed = serverAllowed || candidate == managedQwenServer
				}
			case "--exclude-tools":
				for _, pattern := range values {
					if qwenMCPPatternMatches(pattern, managedQwenTool) {
						return errors.New("--exclude-tools cannot disable the managed Sessionbus tool")
					}
				}
			}
			continue
		}
		if !attached && containsQwenOption(qwenRequiredValueOptions, name) {
			if index+1 < len(arguments) && qwenScalarArgument(arguments[index+1]) {
				index++
			}
		}
	}
	if serverBound && !serverAllowed {
		return errors.New("--allowed-mcp-server-names must include sessionbus for a managed launch")
	}
	return nil
}

func qwenManagedArrayOption(name string) string {
	switch name {
	case "--allowed-mcp-server-names", "--allowedMcpServerNames":
		return "--allowed-mcp-server-names"
	case "--allowed-tools", "--allowedTools":
		return "--allowed-tools"
	case "--exclude-tools", "--excludeTools":
		return "--exclude-tools"
	default:
		return ""
	}
}

func qwenArrayArgument(argument string) bool {
	return argument != "--" && (!strings.HasPrefix(argument, "-") || qwenNegativeNumber.MatchString(argument))
}

func qwenScalarArgument(argument string) bool {
	return !strings.HasPrefix(argument, "-") || qwenNegativeNumber.MatchString(argument)
}

func containsQwenOption(options []string, wanted string) bool {
	for _, option := range options {
		if option == wanted {
			return true
		}
	}
	return false
}

func qwenArrayValue(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

// qwenMCPPatternMatches mirrors the native MCP branches relevant to the fixed
// public tool: exact names, a two-part server rule, and sanitized trailing
// wildcards. A non-empty MCP specifier never matches natively.
func qwenMCPPatternMatches(raw, tool string) bool {
	pattern := strings.TrimSpace(raw)
	if open := strings.IndexByte(pattern, '('); open >= 0 {
		if !strings.HasSuffix(pattern, ")") || pattern[open+1:len(pattern)-1] != "" {
			return false
		}
		pattern = strings.TrimSpace(pattern[:open])
	}
	if pattern == tool {
		return true
	}
	if !strings.HasSuffix(pattern, "*") && len(strings.Split(pattern, "__")) >= 3 && qwenNormalizedMCPToolName(pattern) == qwenNormalizedMCPToolName(tool) {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := qwenProviderToolName(pattern[:len(pattern)-1])
		return strings.HasPrefix(qwenProviderToolName(tool), prefix)
	}
	patternParts, toolParts := strings.Split(pattern, "__"), strings.Split(tool, "__")
	return len(patternParts) == 2 && len(toolParts) >= 3 && patternParts[0] == toolParts[0] &&
		qwenProviderToolName(patternParts[1]) == qwenProviderToolName(toolParts[1])
}

func qwenNormalizedMCPToolName(name string) string {
	if !strings.HasPrefix(name, "mcp__") || qwenProviderSafeToolName(name) {
		return name
	}
	suffix := "_" + strconv.FormatUint(uint64(qwenToolNameHash(name)), 36)
	if padding := 8 - len(suffix); padding > 0 {
		suffix = "_" + strings.Repeat("0", padding) + suffix[1:]
	}
	value := qwenProviderToolName(name)
	return value[:min(len(value), 63-len(suffix))] + suffix
}

func qwenProviderSafeToolName(name string) bool {
	if len(name) == 0 || len(name) > 63 || !((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) {
		return false
	}
	for index := 1; index < len(name); index++ {
		character := name[index]
		if !((character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func qwenToolNameHash(name string) uint32 {
	hash := uint32(2166136261)
	for _, character := range utf16.Encode([]rune(name)) {
		hash = (hash ^ uint32(character)) * 16777619
	}
	return hash
}

func qwenProviderToolName(name string) string {
	var result strings.Builder
	// Native JavaScript's non-Unicode regular expression replaces each UTF-16
	// code unit independently, including both halves of a surrogate pair.
	for _, character := range utf16.Encode([]rune(name)) {
		if character <= 0x7f && (character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '_' || character == '-') {
			result.WriteByte(byte(character))
		} else {
			result.WriteByte('_')
		}
	}
	value := result.String()
	if value == "" || !((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) {
		return "tool_" + value
	}
	return value
}
