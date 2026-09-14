// SPDX-License-Identifier: MIT
package interactive

import (
	"reflect"
	"strings"
	"testing"
)

// Retained Fable report vectors call the actual production parser unchanged.
func TestAliasTokensRemainRequiredNativeValues(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{[]string{"--system-prompt", "--yolo"}, []string{"--system-prompt", "--yolo"}},
		{[]string{"--model", "--yolo"}, []string{"--model", "--yolo"}},
		{[]string{"--yolo", "--resume", "ses_x"}, []string{"--dangerously-skip-permissions", "--resume", "ses_x"}},
		{[]string{"--resume", "abc", "--", "--yolo"}, []string{"--resume", "abc", "--", "--yolo"}},
		{[]string{"--append-system-prompt", "--resume"}, []string{"--append-system-prompt", "--resume"}},
	} {
		t.Run(strings.Join(tc.input, "/"), func(t *testing.T) {
			got, _, err := LaunchPlan(tc.input, nil, "/cwd", "/plugin", 1000)
			if len(got) >= 4 {
				got = got[4:]
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("input=%q output=%q want=%q error=%v", tc.input, got, tc.want, err)
			}
		})
	}
}

func TestAliasNativeValueBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{[]string{"--model=--yolo", "--yolo"}, []string{"--model=--yolo", "--dangerously-skip-permissions"}},
		{[]string{"--model", "--", "--yolo"}, []string{"--model", "--", "--yolo"}},
		{[]string{"--model"}, []string{"--model"}},
		{[]string{"--name", "--yolo", "--resume"}, []string{"--name", "--yolo", "--resume"}},
		{[]string{"--allowed-tools", "--yolo"}, []string{"--allowed-tools", "--yolo"}},
		{[]string{"--debug", "--yolo"}, []string{"--debug", "--dangerously-skip-permissions"}},
		{[]string{"--worktree", "--yolo"}, []string{"--worktree", "--dangerously-skip-permissions"}},
	} {
		t.Run(strings.Join(tc.input, "/"), func(t *testing.T) {
			got, _, err := LaunchPlan(tc.input, nil, "/cwd", "/plugin", 1000)
			if err == nil {
				got = got[4:]
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("input=%q output=%q want=%q error=%v", tc.input, got, tc.want, err)
			}
		})
	}
}
