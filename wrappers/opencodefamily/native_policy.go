// SPDX-License-Identifier: MIT
package opencodefamily

import "errors"

// These are the two source-bound native products, not an extensible dialect.
// The zero value preserves existing OpenCode fixtures and behavior.
type nativeKind uint8

const (
	openCodeNative nativeKind = iota
	kiloNative
)

func (k nativeKind) title() string {
	if k == kiloNative {
		return "Kilo"
	}
	return "OpenCode"
}
func (k nativeKind) name() string {
	if k == kiloNative {
		return "kilo"
	}
	return "opencode"
}
func (k nativeKind) envPrefix() string {
	if k == kiloNative {
		return "KILO"
	}
	return "OPENCODE"
}
func (k nativeKind) err(message string) error { return errors.New(k.title() + " " + message) }

// NewKilo uses the released Kilo native policies; callers cannot inject policy
// callbacks or generate a native session identity through this constructor.
func NewKilo(socket, provisional, executable string) *Wrapper {
	p := NewOpenCode(socket, provisional, executable)
	p.kind = kiloNative
	return p
}
