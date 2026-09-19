// SPDX-License-Identifier: MIT

package interactive

import "testing"

func TestManagedToolRejectsExactNativeDeny(t *testing.T) {
	for _, arguments := range [][]string{
		{"--disallowedTools", PublicTool},
		{"--disallowed-tools", PublicTool},
		{"--disallowedTools=" + PublicTool},
		{"--disallowed-tools=Bash," + PublicTool},
		{"--disallowedTools", "Bash(git status)", PublicTool, "--model", "sonnet"},
		{"--disallowedTools", "Read", "--disallowed-tools", "Bash(git status) " + PublicTool},
		{"--system-prompt", "--", "--disallowedTools", PublicTool},
		{"--disallowedTools", "--", PublicTool},
	} {
		if err := ValidateManagedToolArguments(arguments); err == nil {
			t.Fatalf("exact managed deny accepted: %q", arguments)
		}
		if _, _, err := LaunchPlan(arguments, nil, "/cwd", "/plugin", 1000); err == nil {
			t.Fatalf("interactive launch accepted exact managed deny: %q", arguments)
		}
	}
}

func TestManagedToolPreservesOtherNativePolicyAndBoundaries(t *testing.T) {
	for _, arguments := range [][]string{
		{"--disallowedTools", "Bash(git status),Read"},
		{"--disallowed-tools=OtherTool", "prompt"},
		{"--system-prompt", "--disallowedTools", PublicTool},
		{"--system-prompt=--disallowedTools", PublicTool},
		{"--allowedTools", "--disallowedTools", PublicTool},
		{"--disallowedTools", "Read", "--model", PublicTool},
		{"--", "--disallowedTools", PublicTool},
	} {
		if err := ValidateManagedToolArguments(arguments); err != nil {
			t.Fatalf("native arguments %q rejected: %v", arguments, err)
		}
	}
}

func TestManagedToolRuleSplitterMatchesNativeLists(t *testing.T) {
	if !containsManagedToolRule([]string{"Bash(git status, --short), " + PublicTool}) {
		t.Fatal("managed tool absent from comma-separated native rule list")
	}
	if containsManagedToolRule([]string{"Bash(" + PublicTool + ", status) OtherTool"}) {
		t.Fatal("rule content was treated as a separate tool")
	}
	if containsManagedToolRule([]string{"OtherTool\t" + PublicTool}) {
		t.Fatal("native literal-space splitter incorrectly treated a tab as a separator")
	}
	if !containsManagedToolRule([]string{"\ufeff" + PublicTool + "\ufeff"}) {
		t.Fatal("native ECMAScript trim did not remove byte-order marks")
	}
	if containsManagedToolRule([]string{"\u0085" + PublicTool}) {
		t.Fatal("native ECMAScript trim incorrectly removed NEXT LINE")
	}
}

func TestInteractiveGuardUsesProjectedNativeArguments(t *testing.T) {
	for _, group := range []string{"-g", "--group"} {
		arguments, values, err := LaunchPlan([]string{group, "--disallowedTools", PublicTool}, nil, "/cwd", "/plugin", 1000)
		if err != nil {
			t.Fatalf("%s flag-looking group value rejected: %v", group, err)
		}
		if got := Environment(values)["SESSIONBUS_GROUPS"]; got != `["--disallowedTools"]` {
			t.Fatalf("%s groups = %s", group, got)
		}
		if got := arguments[len(arguments)-1]; got != PublicTool {
			t.Fatalf("%s native positional = %q", group, got)
		}
		if _, _, err := LaunchPlan([]string{group, "team", "--disallowedTools", PublicTool}, nil, "/cwd", "/plugin", 1000); err == nil {
			t.Fatalf("%s genuine native deny accepted", group)
		}
	}
}

func TestManagedToolGuardPreservesCurrentNativeRequiredValues(t *testing.T) {
	for _, option := range []string{"--system-prompt-file", "--permission-prompt-tool", "--managed-settings"} {
		arguments := []string{option, "--disallowedTools", PublicTool}
		if err := ValidateManagedToolArguments(arguments); err != nil {
			t.Fatalf("%s flag-looking value rejected: %v", option, err)
		}
		projected, _, err := LaunchPlan(arguments, nil, "/cwd", "/plugin", 1000)
		if err != nil {
			t.Fatalf("%s interactive value rejected: %v", option, err)
		}
		if got := projected[len(projected)-3:]; got[0] != option || got[1] != "--disallowedTools" || got[2] != PublicTool {
			t.Fatalf("%s native values changed: %q", option, got)
		}
	}
	for _, arguments := range [][]string{{"--channels", "--disallowedTools", PublicTool}} {
		if err := ValidateManagedToolArguments(arguments); err != nil {
			t.Fatalf("native values %q rejected: %v", arguments, err)
		}
	}
	for _, arguments := range [][]string{
		{"--channels", "server", "--disallowedTools", PublicTool},
		{"--resume", "session", "--disallowedTools", PublicTool},
		{"--resume", "--disallowedTools", PublicTool},
	} {
		if err := ValidateManagedToolArguments(arguments); err == nil {
			t.Fatalf("genuine native deny accepted after %q", arguments)
		}
	}
}
