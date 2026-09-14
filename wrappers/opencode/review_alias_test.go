// SPDX-License-Identifier: MIT
package opencode

import (
	"slices"
	"strings"
	"testing"
)

func TestReviewResumeAliasDoesNotRewriteWrapperValues(t *testing.T) {
	for _, flag := range []string{"-n", "--peer-name", "-g", "--group"} {
		t.Run(flag, func(t *testing.T) {
			plan, native, err := InteractivePlan([]string{flag, "--resume", "--agent", "build"}, nil)
			for _, value := range plan.Env {
				if (strings.HasPrefix(value, "SESSIONBUS_SESSION_NAME=") || strings.HasPrefix(value, "SESSIONBUS_GROUPS=")) && strings.Contains(value, "--session") {
					t.Errorf("wrapper value rewritten: %s", value)
				}
			}
			if err != nil || native || !slices.Equal(plan.Args, []string{"--agent", "build"}) {
				t.Fatalf("wrapper value became an alias: args=%q native=%v err=%v", plan.Args, native, err)
			}
		})
	}
}

func TestReviewResumeAliasCannotConsumeTerminator(t *testing.T) {
	if plan, native, err := InteractivePlan([]string{"--resume", "--", "--agent", "build"}, nil); err == nil {
		t.Fatalf("missing ID consumed literal terminator: args=%q native=%v", plan.Args, native)
	}
}
