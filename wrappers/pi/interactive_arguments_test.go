// SPDX-License-Identifier: MIT

package pi

import (
	"reflect"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/host"
)

func TestInteractivePlanProjectsWrapperIdentityAndKeepsNativeArguments(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	args := []string{"--model", "deepseek/model", "-g", "one,two", "--name", "native title", "-n", "peer title", "--resume"}
	plan, passthrough, err := InteractivePlan(args, []string{"OTHER=kept", host.SocketEnv + "=/bus.sock"})
	if err != nil || passthrough {
		t.Fatalf("plan: %#v passthrough=%v err=%v", plan, passthrough, err)
	}
	wantArgs := []string{"--model", "deepseek/model", "--name", "native title", "--resume"}
	if !reflect.DeepEqual(plan.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", plan.Args, wantArgs)
	}
	if got := interactiveEnvironmentValue(plan.Env, host.GroupsEnv); got != `["one,two"]` {
		t.Fatalf("groups = %q", got)
	}
	if got := interactiveEnvironmentValue(plan.Env, host.NameEnv); got != "peer title" {
		t.Fatalf("name = %q", got)
	}
	if got := interactiveEnvironmentValue(plan.Env, host.SocketEnv); got != "/bus.sock" {
		t.Fatalf("socket = %q", got)
	}
}

func TestInteractivePlanNativeValueAndBoundaryProtectWrapperFlags(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	for _, args := range [][]string{
		{"--model", "-g", "prompt"},
		{"--model", "--mode", "prompt"},
		{"-t", "-g", "prompt"},
		{"-xt", "--peer-name", "prompt"},
		{"--tui-mode", "-g", "prompt"},
		{"--model=-g", "prompt"},
		{"--", "-g", "--mode"},
	} {
		plan, passthrough, err := InteractivePlan(args, nil)
		if err != nil || passthrough || !reflect.DeepEqual(plan.Args, args) {
			t.Fatalf("%#v -> %#v passthrough=%v err=%v", args, plan.Args, passthrough, err)
		}
	}
}

func TestInteractivePlanNativeMaintenancePassthroughIsBytePreserving(t *testing.T) {
	for _, args := range [][]string{
		{"--help", "-g"},
		{"install", "-g", "package"},
		{"--print", "-g"},
		{"-p", "prompt", "-g"},
		{"--export", "session.jsonl", "output.html"},
		{"--list-models", "deepseek"},
	} {
		environment := []string{"BAD WRAPPER ENV", "OTHER=value"}
		plan, passthrough, err := InteractivePlan(args, environment)
		if err != nil || !passthrough {
			t.Fatalf("%#v passthrough=%v err=%v", args, passthrough, err)
		}
		if !reflect.DeepEqual(plan.Args, args) || !reflect.DeepEqual(plan.Env, environment) {
			t.Fatalf("passthrough changed: %#v", plan)
		}
	}
}

func TestInteractivePlanDoesNotTreatExtensionFlagValueAsCommand(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	for _, args := range [][]string{
		{"--extension-flag", "auth", "prompt"},
		{"--extension-flag", "install", "prompt"},
		{"--model", "--help", "prompt"},
		{"--export"},
	} {
		plan, passthrough, err := InteractivePlan(args, nil)
		if err != nil || passthrough || !reflect.DeepEqual(plan.Args, args) {
			t.Fatalf("%#v -> %#v passthrough=%v err=%v", args, plan.Args, passthrough, err)
		}
	}
}

func TestInteractivePlanRecognizesMaintenanceOnlyAsFirstArgument(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	plan, passthrough, err := InteractivePlan([]string{"--offline", "auth", "print-api-key", "-g", "team"}, nil)
	if err != nil || passthrough {
		t.Fatalf("passthrough=%v err=%v", passthrough, err)
	}
	if !reflect.DeepEqual(plan.Args, []string{"--offline", "auth", "print-api-key"}) {
		t.Fatalf("args = %#v", plan.Args)
	}
	if got := interactiveEnvironmentValue(plan.Env, host.GroupsEnv); got != `["team"]` {
		t.Fatalf("groups = %q", got)
	}
}

func TestInteractivePlanSelectsPeerShortNameAndNativeLongName(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	plan, passthrough, err := InteractivePlan([]string{"-n", "peer name", "--name", "native title"}, nil)
	if err != nil || passthrough {
		t.Fatalf("passthrough=%v err=%v", passthrough, err)
	}
	if !reflect.DeepEqual(plan.Args, []string{"--name", "native title"}) {
		t.Fatalf("args = %#v", plan.Args)
	}
	if got := interactiveEnvironmentValue(plan.Env, host.NameEnv); got != "peer name" {
		t.Fatalf("peer name = %q", got)
	}
}

func TestInteractivePlanRejectsTopologyOverride(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	for _, args := range [][]string{{"--mode", "rpc"}, {"--mode=json"}} {
		if _, native, err := InteractivePlan(args, nil); err == nil || native {
			t.Fatalf("%#v accepted: native=%v err=%v", args, native, err)
		}
	}
}

func TestInteractivePlanRejectsInvalidWrapperValue(t *testing.T) {
	if _, native, err := InteractivePlan([]string{"-g"}, nil); err == nil || native {
		t.Fatalf("missing group accepted: native=%v err=%v", native, err)
	}
}
