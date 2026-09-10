// SPDX-License-Identifier: MIT
package qwen

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// Qwen 0.23 renameCommand.parseArgs splits on ECMAScript /\s+/ and joins
// positional tokens with one space; SESSION_TITLE_MAX_LENGTH is 200 JS
// string code units. Keep the expected confirmation identical to that title.
func nativeInitialName(raw string) (string, error) {
	if !utf8.ValidString(raw) || strings.ContainsAny(raw, "\x00\r\n") {
		return "", errors.New("Qwen initial name must be valid UTF-8, single-line and contain no NUL")
	}
	name := strings.Join(strings.FieldsFunc(raw, nativeNameWhitespace), " ")
	if name == "" {
		return "", errors.New("Qwen initial name must be nonempty")
	}
	units := 0
	for _, r := range name {
		units++
		if r > 0xffff {
			units++
		}
	}
	if units > 200 {
		return "", errors.New("Qwen initial name exceeds 200 UTF-16 code units")
	}
	return name, nil
}

func nativeNameWhitespace(r rune) bool {
	// ECMAScript WhiteSpace and LineTerminator, not Go's Unicode White_Space
	// property: FEFF is whitespace here, but U+0085 is not.
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680,
		0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}
