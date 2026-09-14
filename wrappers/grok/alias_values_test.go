// SPDX-License-Identifier: MIT
package grok

import (
	"reflect"
	"strings"
	"testing"
)

// Retained Fable report vectors call the actual production parser unchanged.
func TestAliasTokensRemainRequiredNativeValues(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{[]string{"--model", "--yolo"}, []string{"--model", "--yolo"}},
		{[]string{"--resume", "--yolo"}, []string{"--resume", "--always-approve"}},
		{[]string{"--yolo", "--resume", "title x"}, []string{"--always-approve", "--resume", "title x"}},
		{[]string{"--resume", "abc", "--", "--yolo"}, []string{"--resume", "abc", "--", "--yolo"}},
		{[]string{"--yolo=1"}, []string{"--yolo=1"}},
	} {
		t.Run(strings.Join(tc.input, "/"), func(t *testing.T) {
			plan, err := InteractivePlan(tc.input, nil)
			got := plan.Args
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("input=%q output=%q want=%q error=%v", tc.input, got, tc.want, err)
			}
		})
	}
}

func TestAliasNativeValueBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{[]string{"--model=--yolo", "--yolo"}, []string{"--model=--yolo", "--always-approve"}},
		{[]string{"-m--yolo", "--yolo"}, []string{"-m--yolo", "--always-approve"}},
		{[]string{"--model", "--", "--yolo"}, []string{"--model", "--", "--yolo"}},
		{[]string{"--model"}, []string{"--model"}},
		{[]string{"--worktree", "--yolo"}, []string{"--worktree", "--always-approve"}},
		{[]string{"--system-prompt", "--yolo"}, []string{"--system-prompt", "--yolo"}},
		{[]string{"--effort", "--yolo"}, []string{"--effort", "--yolo"}},
		{[]string{"--allowedTools", "--yolo"}, []string{"--allowedTools", "--yolo"}},
	} {
		t.Run(strings.Join(tc.input, "/"), func(t *testing.T) {
			plan, err := InteractivePlan(tc.input, nil)
			got := plan.Args
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("input=%q output=%q want=%q error=%v", tc.input, got, tc.want, err)
			}
		})
	}
}
