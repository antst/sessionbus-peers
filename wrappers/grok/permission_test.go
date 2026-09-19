// SPDX-License-Identifier: MIT

package grok

import (
	"slices"
	"strings"
	"testing"
)

func TestGrokGlobMatchesNativeOracleCorpus(t *testing.T) {
	for _, test := range []struct {
		pattern string
		matched bool
	}{
		{"sessionbus__sessionbus", true},
		{"SESSIONBUS__SESSIONBUS", true},
		{"sessionbus__*", true},
		{"sessionbus__sessionbu?", true},
		{"sessionbus__sessionbu[s]", true},
		{"sessionbus__sessionbu[!x]", true},
		{"sessionbus__sessionbu[a-z]", true},
		{"sessionbus__sessionbu[A-ß]", true},
		{"sessionbus__sessionbuſ", false},
		{"*", true},
		{"**", true},
		{"**/sessionbus__sessionbus", true},
		{"sessionbus**", false},
		{"sessionbus__**", false},
		{"***", false},
		{`sessionbus__sessionbus\*`, false},
		{"sessionbus__sessionbus[*]", false},
		{"sessionbus__sessionbus[", false},
		{"sessionbus__sessionbus[]", false},
		{"sessionbus__sessionbus[[]", false},
		{"sessionbus__sessionbus[]]", false},
	} {
		t.Run(test.pattern, func(t *testing.T) {
			if matched := grokGlobMatches(test.pattern, sessionbusNativeTool); matched != test.matched {
				t.Fatalf("grokGlobMatches(%q) = %t, want %t", test.pattern, matched, test.matched)
			}
		})
	}
}

func TestDenyReachesSessionbusMatchesNativeRuleParser(t *testing.T) {
	for _, test := range []struct {
		rule    string
		matched bool
	}{
		{"MCPTool", true},
		{"MCPTool()", true},
		{"MCPTool(*)", true},
		{"MCPTool(domain:)", true},
		{"MCPTool(sessionbus__sessionbus)", true},
		{"MCPTool(SESSIONBUS__SESSIONBUS)", true},
		{"MCPTool(domain:sessionbus__sessionbus)", true},
		{"MCPTool(sessionbus__sessionbus)ignored", true},
		{"MCPTool(sessionbus__sessionbu[A-ß])", true},
		{"mcp__*", true},
		{"mcp__sessionbus", true},
		{"mcp__sessionbus__sessionbus", true},
		{"sessionbus__*", true},
		{"SESSIONBUS__SESSIONBUS", true},
		{"", true},
		{"*", true},
		{"MCPTool(other__tool)", false},
		{"MCPTool(sessionbus__sessionbus)ignored)", false},
		{"MCPTool(sessionbus__**)", false},
		{"MCPTool(sessionbus__sessionbus\\*)", false},
		{"Unknown(sessionbus__sessionbus)", false},
		{"Read(sessionbus__*)", false},
		{"Read", false},
		{"mcp__other", false},
		{"mcp__", false},
	} {
		t.Run(test.rule, func(t *testing.T) {
			if matched := denyReachesSessionbus(test.rule); matched != test.matched {
				t.Fatalf("denyReachesSessionbus(%q) = %t, want %t", test.rule, matched, test.matched)
			}
		})
	}
}

func TestInteractivePolicyProjection(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{"ordinary", nil, sessionbusLeaderPolicy()},
		{"wrapper yolo projection", []string{"--always-approve"}, appendGrant("--always-approve")},
		{"native bypass", []string{"--dangerously-skip-permissions"}, appendGrant("--dangerously-skip-permissions")},
		{"permission attached", []string{"--permission-mode=default"}, appendGrant("--permission-mode=default")},
		{"permission separate", []string{"--permission-mode", "default"}, appendGrant("--permission-mode", "default")},
		{"repeated rules", []string{"--allow=MCPTool(other__*)", "--allowedTools", "Read(*)", "--deny=Read(/tmp)", "--disallowedTools", "MCPTool(other__tool)"}, appendGrant("--allow=MCPTool(other__*)", "--allowedTools", "Read(*)", "--deny=Read(/tmp)", "--disallowedTools", "MCPTool(other__tool)")},
		{"known values own controls", []string{"--model", "--deny", "--cwd", "--always-approve", "--rules", "--permission-mode=default"}, sessionbusLeaderPolicy()},
		{"post delimiter", []string{"--", "--deny=MCPTool(sessionbus__sessionbus)", "--always-approve"}, sessionbusLeaderPolicy()},
		{"attached option value", []string{"--deny=--allow"}, appendGrant("--deny=--allow")},
		{"builtin removal is not policy", []string{"--disallowed-tools", "Read,Write", "--always-approve"}, appendGrant("--always-approve")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := interactivePolicy(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("policy = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestInteractivePolicyRejectsManagedDeny(t *testing.T) {
	for _, args := range [][]string{
		{"--deny", "MCPTool(sessionbus__sessionbus)"},
		{"--deny=MCPTool(SESSIONBUS__SESSIONBUS)"},
		{"--disallowedTools", "Read(*),mcp__sessionbus"},
		{"--deny", "unrelated,,other"},
		{"--deny="},
		{"--deny", "MCPTool(domain:sessionbus__sessionbus)"},
		{"--deny", "MCPTool(sessionbus__sessionbus)ignored"},
		{"--deny", "**/sessionbus__sessionbus"},
	} {
		_, err := interactivePolicy(args)
		if err == nil || !strings.Contains(err.Error(), "disables the managed Sessionbus tool") {
			t.Fatalf("managed deny accepted: %#v: %v", args, err)
		}
	}
}

func TestInteractivePolicyRetainsNativeMalformedValueFailure(t *testing.T) {
	for _, test := range []struct {
		args, want []string
	}{
		{[]string{"--deny"}, appendGrant("--deny")},
		{[]string{"--deny", "--"}, appendGrant("--deny")},
		{[]string{"--deny", "--allow", "two"}, appendGrant("--deny", "--allow")},
		{[]string{"--allowedTools", "--yolo"}, appendGrant("--allowedTools", "--yolo")},
	} {
		policy, err := interactivePolicy(test.args)
		if err != nil || !slices.Equal(policy, test.want) {
			t.Fatalf("malformed policy = %#v, %v, want %#v for %#v", policy, err, test.want, test.args)
		}
	}
}

func appendGrant(arguments ...string) []string {
	return append(sessionbusLeaderPolicy(), arguments...)
}
