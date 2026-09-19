// SPDX-License-Identifier: MIT

package grok

import (
	"fmt"
	"os"
	"strings"
)

const sessionbusNativeTool = "sessionbus__sessionbus"
const sessionbusNativeRule = "MCPTool(" + sessionbusNativeTool + ")"

func sessionbusLeaderPolicy() []string {
	return []string{"--allow", sessionbusNativeRule}
}

// interactivePolicy projects only native policy controls onto the private
// leader. arguments is already the InteractivePlan projection, so values owned
// by another native option must not be reinterpreted as wrapper controls.
func interactivePolicy(arguments []string) ([]string, error) {
	policy := sessionbusLeaderPolicy()
	for i := 0; i < len(arguments); i++ {
		argument := arguments[i]
		if argument == "--" {
			break
		}
		key, value, attached := strings.Cut(argument, "=")
		switch key {
		case "--allow", "--allowedTools", "--deny", "--disallowedTools", "--permission-mode":
			policy = append(policy, argument)
			if !attached {
				if i+1 == len(arguments) || arguments[i+1] == "--" {
					continue
				}
				i++
				value = arguments[i]
				policy = append(policy, value)
			}
			if (key == "--deny" || key == "--disallowedTools") && (attached || !optionLikeValue(value)) {
				for _, rule := range strings.Split(value, ",") {
					if denyReachesSessionbus(rule) {
						return nil, fmt.Errorf("%s rule %q disables the managed Sessionbus tool", key, rule)
					}
				}
			}
			continue
		case "--always-approve", "--dangerously-skip-permissions":
			if !attached {
				policy = append(policy, argument)
			}
		}
		if !attached && grokOptionTakesValue(key) && i+1 < len(arguments) && arguments[i+1] != "--" {
			i++
		}
	}
	return policy, nil
}

func optionLikeValue(value string) bool {
	return value != "-" && strings.HasPrefix(value, "-")
}

func denyReachesSessionbus(raw string) bool {
	rule := strings.TrimSpace(raw)
	if open := firstUnescaped(rule, '('); open >= 0 {
		close := lastUnescaped(rule[open+1:], ')')
		if close < 0 || strings.TrimSpace(rule[:open]) != "MCPTool" {
			return false
		}
		content := strings.TrimSpace(rule[open+1 : open+1+close])
		pattern := ""
		if content != "" && content != "*" {
			pattern = unescapeRuleContent(content)
		}
		pattern = strings.TrimPrefix(pattern, "domain:")
		return pattern == "" || pattern == "*" || grokGlobMatches(pattern, sessionbusNativeTool)
	}

	switch rule {
	case "MCPTool":
		return true
	case "EnterWorktree", "NotebookEdit", "NotebookRead", "Bash", "Read", "Edit", "Write",
		"Grep", "Glob", "WebFetch", "WebSearch", "AgentMessage", "SendSubagentMessage", "SendAgentMessage":
		return false
	}

	if rest, found := strings.CutPrefix(rule, "mcp__"); found && rest != "" {
		if rest == "*" {
			return true
		}
		pattern := rest
		if !strings.Contains(rest, "__") {
			pattern += "__*"
		}
		return grokGlobMatches(pattern, sessionbusNativeTool)
	}
	return rule == "" || rule == "*" || grokGlobMatches(rule, sessionbusNativeTool)
}

func firstUnescaped(value string, target byte) int {
	for index := 0; index < len(value); index++ {
		if value[index] == target && unescapedAt(value, index) {
			return index
		}
	}
	return -1
}

func lastUnescaped(value string, target byte) int {
	for index := len(value) - 1; index >= 0; index-- {
		if value[index] == target && unescapedAt(value, index) {
			return index
		}
	}
	return -1
}

func unescapedAt(value string, index int) bool {
	backslashes := 0
	for index > 0 && value[index-1] == '\\' {
		backslashes++
		index--
	}
	return backslashes%2 == 0
}

func unescapeRuleContent(value string) string {
	value = strings.ReplaceAll(value, `\(`, `(`)
	value = strings.ReplaceAll(value, `\)`, `)`)
	return strings.ReplaceAll(value, `\\`, `\`)
}

type grokGlobKind uint8

const (
	grokGlobLiteral grokGlobKind = iota
	grokGlobAnyChar
	grokGlobAnySequence
	grokGlobAnyRecursiveSequence
	grokGlobWithin
	grokGlobExcept
)

type grokGlobRange struct {
	start, end rune
	ranged     bool
}

type grokGlobToken struct {
	kind   grokGlobKind
	value  rune
	ranges []grokGlobRange
}

func grokGlobMatches(pattern, target string) bool {
	tokens, ok := compileGrokGlob(pattern)
	if !ok {
		return false
	}
	targetRunes := []rune(target)
	type state struct {
		pattern, target  int
		followsSeparator bool
	}
	memo, known := map[state]bool{}, map[state]bool{}
	var match func(int, int, bool) bool
	match = func(patternIndex, targetIndex int, followsSeparator bool) bool {
		key := state{patternIndex, targetIndex, followsSeparator}
		if known[key] {
			return memo[key]
		}
		known[key] = true
		if patternIndex == len(tokens) {
			memo[key] = targetIndex == len(targetRunes)
			return memo[key]
		}
		token := tokens[patternIndex]
		switch token.kind {
		case grokGlobAnySequence, grokGlobAnyRecursiveSequence:
			if match(patternIndex+1, targetIndex, followsSeparator) {
				memo[key] = true
				return true
			}
			for next := targetIndex; next < len(targetRunes); next++ {
				separator := grokPathSeparator(targetRunes[next])
				if token.kind == grokGlobAnyRecursiveSequence && !separator && next+1 != len(targetRunes) {
					continue
				}
				if token.kind == grokGlobAnySequence || separator || next+1 == len(targetRunes) {
					if match(patternIndex+1, next+1, separator) {
						memo[key] = true
						return true
					}
				}
			}
			return false
		}
		if targetIndex == len(targetRunes) {
			return false
		}
		value := targetRunes[targetIndex]
		matched := false
		switch token.kind {
		case grokGlobLiteral:
			matched = grokGlobRunesEqual(value, token.value)
		case grokGlobAnyChar:
			matched = true
		case grokGlobWithin:
			matched = grokGlobRangesContain(token.ranges, value)
		case grokGlobExcept:
			matched = !grokGlobRangesContain(token.ranges, value)
		}
		if matched && match(patternIndex+1, targetIndex+1, grokPathSeparator(value)) {
			memo[key] = true
			return true
		}
		return false
	}
	return match(0, 0, true)
}

func compileGrokGlob(pattern string) ([]grokGlobToken, bool) {
	characters := []rune(pattern)
	tokens := []grokGlobToken{}
	for index := 0; index < len(characters); {
		switch characters[index] {
		case '?':
			tokens = append(tokens, grokGlobToken{kind: grokGlobAnyChar})
			index++
		case '*':
			start := index
			for index < len(characters) && characters[index] == '*' {
				index++
			}
			count := index - start
			if count > 2 {
				return nil, false
			}
			if count == 1 {
				tokens = append(tokens, grokGlobToken{kind: grokGlobAnySequence})
				continue
			}
			if start != 0 && !grokPathSeparator(characters[start-1]) {
				return nil, false
			}
			if index < len(characters) && grokPathSeparator(characters[index]) {
				index++
			} else if index != len(characters) {
				return nil, false
			}
			tokens = append(tokens, grokGlobToken{kind: grokGlobAnyRecursiveSequence})
		case '[':
			negated := index+1 < len(characters) && characters[index+1] == '!'
			contentStart := index + 1
			searchStart := index + 2
			minimum := 3
			if negated {
				contentStart++
				searchStart++
				minimum = 4
			}
			if index+minimum > len(characters) {
				return nil, false
			}
			closeIndex := -1
			for cursor := searchStart; cursor < len(characters); cursor++ {
				if characters[cursor] == ']' {
					closeIndex = cursor
					break
				}
			}
			if closeIndex < 0 {
				return nil, false
			}
			kind := grokGlobWithin
			if negated {
				kind = grokGlobExcept
			}
			tokens = append(tokens, grokGlobToken{kind: kind, ranges: compileGrokGlobRanges(characters[contentStart:closeIndex])})
			index = closeIndex + 1
		default:
			tokens = append(tokens, grokGlobToken{kind: grokGlobLiteral, value: characters[index]})
			index++
		}
	}
	return tokens, true
}

func compileGrokGlobRanges(characters []rune) []grokGlobRange {
	ranges := []grokGlobRange{}
	for index := 0; index < len(characters); {
		if index+2 < len(characters) && characters[index+1] == '-' {
			ranges = append(ranges, grokGlobRange{start: characters[index], end: characters[index+2], ranged: true})
			index += 3
		} else {
			ranges = append(ranges, grokGlobRange{start: characters[index], end: characters[index]})
			index++
		}
	}
	return ranges
}

func grokGlobRangesContain(ranges []grokGlobRange, value rune) bool {
	for _, candidate := range ranges {
		if !candidate.ranged {
			if grokGlobRunesEqual(value, candidate.start) {
				return true
			}
			continue
		}
		if isASCIILetter(value) && isASCIILetter(candidate.start) && isASCIILetter(candidate.end) {
			lower := asciiLower(value)
			if lower >= asciiLower(candidate.start) && lower <= asciiLower(candidate.end) {
				return true
			}
		}
		if value >= candidate.start && value <= candidate.end {
			return true
		}
	}
	return false
}

func grokGlobRunesEqual(left, right rune) bool {
	if grokPathSeparator(left) && grokPathSeparator(right) {
		return true
	}
	if left <= 127 && right <= 127 {
		return asciiLower(left) == asciiLower(right)
	}
	return left == right
}

func grokPathSeparator(value rune) bool {
	return value <= 255 && os.IsPathSeparator(uint8(value))
}

func isASCIILetter(value rune) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func asciiLower(value rune) rune {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}
