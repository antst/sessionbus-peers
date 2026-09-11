// SPDX-License-Identifier: MIT
package codex

import (
	"reflect"
	"strings"
	"testing"
)

// Retained Fable report vectors call the actual production parser unchanged.
func TestAliasTokensRemainRequiredNativeValues(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{[]string{"--model", "--yolo"}, []string{"--model", "--yolo"}},
		{[]string{"--model", "--resume"}, []string{"--model", "--resume"}},
		{[]string{"-m", "--yolo"}, []string{"-m", "--yolo"}},
		{[]string{"--yolo", "--resume", "ses_x"}, []string{"--dangerously-bypass-approvals-and-sandbox", "resume", "ses_x"}},
		{[]string{"--resume", "abc", "--yolo", "--", "--yolo", "--resume"}, []string{"resume", "abc", "--dangerously-bypass-approvals-and-sandbox", "--", "--yolo", "--resume"}},
	} {
		t.Run(strings.Join(tc.input, "/"), func(t *testing.T) {
			out, err := parseInteractiveOptions(tc.input)
			got := out.native
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("input=%q output=%q want=%q error=%v", tc.input, got, tc.want, err)
			}
		})
	}
}

func TestAliasNativeValueBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{[]string{"--model=--resume", "--yolo"}, []string{"--model=--resume", "--dangerously-bypass-approvals-and-sandbox"}},
		{[]string{"-m--yolo", "--resume=abc"}, []string{"-m--yolo", "resume", "abc"}},
		{[]string{"--model", "--", "--yolo", "--resume"}, []string{"--model", "--", "--yolo", "--resume"}},
		{[]string{"--model"}, []string{"--model"}},
		{[]string{"--model", "-g", "--yolo"}, []string{"--model", "-g", "--dangerously-bypass-approvals-and-sandbox"}},
		{[]string{"-c", "--yolo"}, []string{"-c", "--dangerously-bypass-approvals-and-sandbox"}},
	} {
		t.Run(strings.Join(tc.input, "/"), func(t *testing.T) {
			out, err := parseInteractiveOptions(tc.input)
			got := out.native
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("input=%q output=%q want=%q error=%v", tc.input, got, tc.want, err)
			}
		})
	}
}
