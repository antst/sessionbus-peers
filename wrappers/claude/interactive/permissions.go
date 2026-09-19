// SPDX-License-Identifier: MIT

package interactive

import (
	"errors"
	"strings"
)

var errManagedToolDenied = errors.New("Claude launch arguments cannot deny the managed Sessionbus tool")

// ValidateManagedToolArguments rejects only a caller's exact native CLI deny
// of the tool that every managed Claude launch requires. Claude 2.1.276 treats
// the separate deny form as variadic, while an attached value belongs to that
// occurrence alone. Other native permissions remain Claude-owned.
func ValidateManagedToolArguments(arguments []string) error {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			break
		}
		option, attachedValue, attached := strings.Cut(argument, "=")
		if option == "--disallowedTools" || option == "--disallowed-tools" {
			values := []string(nil)
			if attached {
				values = append(values, attachedValue)
			} else if index+1 < len(arguments) {
				index++
				values = append(values, arguments[index])
				for arguments[index] != "--" && index+1 < len(arguments) && arguments[index+1] != "--" && !claudeOptionLike(arguments[index+1]) {
					index++
					values = append(values, arguments[index])
				}
			}
			if containsManagedToolRule(values) {
				return errManagedToolDenied
			}
			continue
		}
		if !attached && claudeOptionTakesValue(option) && index+1 < len(arguments) {
			// The first value of a native value-taking option remains data even
			// when it begins with a dash. Do not reinterpret it as our guard.
			index++
		}
	}
	return nil
}

func claudeOptionLike(argument string) bool {
	return len(argument) > 1 && strings.HasPrefix(argument, "-")
}

func containsManagedToolRule(values []string) bool {
	for _, value := range values {
		var rule strings.Builder
		parenthesized := false
		flush := func() bool {
			managed := strings.TrimSpace(rule.String()) == PublicTool
			rule.Reset()
			return managed
		}
		for _, character := range value {
			switch character {
			case '(':
				parenthesized = true
				rule.WriteRune(character)
			case ')':
				parenthesized = false
				rule.WriteRune(character)
			case ',', ' ':
				if parenthesized {
					rule.WriteRune(character)
				} else if flush() {
					return true
				}
			default:
				rule.WriteRune(character)
			}
		}
		if flush() {
			return true
		}
	}
	return false
}
