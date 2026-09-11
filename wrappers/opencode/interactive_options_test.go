// SPDX-License-Identifier: MIT

package opencode

import (
	"slices"
	"strings"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/host"
)

func TestManagedNativeTopologyAndPure(t *testing.T) {
	for _, args := range [][]string{{"--hostname=elsewhere"}, {"--port", "1"}, {"--mdns=false"}, {"--no-mdns"}, {"--mdnsDomain=x"}, {"--cors=x"}, {"--pure"}, {"--pure=true"}, {"--pure=garbage"}, {"--no-pure=true"}} {
		if _, _, err := InteractivePlan(args, nil); err == nil {
			t.Fatalf("accepted conflict %v", args)
		}
	}
	for _, args := range [][]string{{"--pure=false"}, {"--pure", "false"}, {"--no-pure"}, {"--log-level", "--pure"}, {"--", "--hostname=elsewhere", "--pure"}} {
		plan, native, err := InteractivePlan(args, nil)
		if err != nil || native || !slices.Equal(plan.Args, args) {
			t.Fatalf("changed compatible native args %v: %+v/%v/%v", args, plan, native, err)
		}
	}
	for _, value := range []string{"true", "yes", "on", "1", "y"} {
		if _, _, err := InteractivePlan([]string{"--no-pure"}, []string{"OPENCODE_PURE=" + value}); err == nil {
			t.Fatalf("inherited native true %q", value)
		}
	}
	for _, value := range []string{"false", "no", "off", "0", "n", "TRUE", " true", ""} {
		plan, _, err := InteractivePlan(nil, []string{"OPENCODE_PURE=" + value})
		if err != nil || interactiveEnv(plan.Env, "OPENCODE_PURE") != value {
			t.Fatalf("rewrote native env %q: %v", value, err)
		}
	}
	plan, native, err := InteractivePlan([]string{"--help", "--pure"}, []string{"OPENCODE_PURE=1"})
	if err != nil || !native || !slices.Equal(plan.Args, []string{"--help", "--pure"}) {
		t.Fatal("native help altered", err)
	}
	plan, native, err = InteractivePlan([]string{"--pure", "false", "run", "literal"}, nil)
	if err != nil || !native || !slices.Equal(plan.Args, []string{"--pure", "false", "run", "literal"}) {
		t.Fatal("native subcommand misclassified after boolean value", err)
	}
}

func TestManagedGroupsAndNativeSelectorPreservation(t *testing.T) {
	args := []string{"--session", "ses_exact", "-g", "a,b", "--group=c,a", "-n", "literal name", "--fork", "--", "-g", "native"}
	plan, native, err := InteractivePlan(args, nil)
	if err != nil || native {
		t.Fatal(err)
	}
	if !slices.Equal(plan.Args, []string{"--session", "ses_exact", "--fork", "--", "-g", "native"}) {
		t.Fatal(plan.Args)
	}
	if interactiveEnv(plan.Env, host.GroupsEnv) != `["a","b","c"]` || interactiveEnv(plan.Env, host.NameEnv) != "literal name" {
		t.Fatal(plan.Env)
	}
	if interactiveEnv(plan.Env, host.SessionIDEnv) != "" {
		t.Fatal("invented native ID")
	}
	for _, args := range [][]string{{"-g", "a,,b"}, {"-n", strings.Repeat("x", 129)}} {
		if _, _, err := InteractivePlan(args, nil); err == nil {
			t.Fatal("invalid identity accepted", args)
		}
	}
}
