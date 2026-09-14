// SPDX-License-Identifier: MIT

package main

import (
	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
	"testing"
)

func TestTokenModeNeverFallsThroughToInteractive(t *testing.T) {
	_, _, err := interactive.LaunchPlan(nil, map[string]string{"SESSIONBUS_LAUNCH_TOKEN": "token"}, "/cwd", "/plugin", 1000)
	if err == nil {
		t.Fatal("token launch accepted as interactive")
	}
}
