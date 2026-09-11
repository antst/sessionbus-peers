// SPDX-License-Identifier: MIT
package opencode

import (
	"github.com/antst/sessionbus-peers/wrappers/host"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
	"slices"
	"testing"
)

func TestOpenArgumentsAndInteractiveArity(t *testing.T) {
	arguments, model, agent, err := launchArguments(sessionkit.OpenOptions{PermissionMode: "default", Model: "openai/gpt", Arguments: []string{"--agent", "build", "--log-level", "INFO"}})
	if err != nil || !slices.Equal(arguments, []string{"--log-level", "INFO"}) || model.ProviderID != "openai" || model.ID != "gpt" || agent != "build" {
		t.Fatalf("arguments = %#v/%#v/%q/%v", arguments, model, agent, err)
	}
	if _, _, _, err = launchArguments(sessionkit.OpenOptions{Arguments: []string{"--pure"}}); err == nil || err.Error() != "unsupported argument --pure" {
		t.Fatalf("lane --pure = %v", err)
	}
	if _, _, _, err = launchArguments(sessionkit.OpenOptions{PermissionMode: "plan"}); err == nil || err.Error() != "unsupported value permission_mode=plan" {
		t.Fatalf("permission mode = %v", err)
	}
	arguments, _, agent, err = launchArguments(sessionkit.OpenOptions{Arguments: []string{"--log-level", "--agent"}})
	if err != nil || agent != "" || !slices.Equal(arguments, []string{"--log-level", "--agent"}) {
		t.Fatalf("native value arity = %#v/%q/%v", arguments, agent, err)
	}
	plan, native, err := InteractivePlan([]string{"--log-level", "-g", "team"}, []string{"PATH=/bin"})
	if err != nil || native || !slices.Equal(plan.Args, []string{"--log-level", "-g", "team"}) {
		t.Fatalf("arity plan = %#v/%v/%v", plan, native, err)
	}
	if !slices.Contains(plan.Env, host.SocketEnv+"="+sessionkit.Socket()) {
		t.Fatalf("default socket missing from %#v", plan.Env)
	}
	plan, native, err = InteractivePlan([]string{"run", "-g"}, []string{"PATH=/bin"})
	if err != nil || !native || !slices.Equal(plan.Args, []string{"run", "-g"}) {
		t.Fatalf("passthrough = %#v/%v/%v", plan, native, err)
	}
	plan, native, err = InteractivePlan([]string{"/work/project", "run", "-g", "team"}, []string{"PATH=/bin"})
	if err != nil || native || !slices.Equal(plan.Args, []string{"/work/project", "run"}) {
		t.Fatalf("project = %#v/%v/%v", plan, native, err)
	}
	if _, _, err = InteractivePlan([]string{"--pure=true"}, nil); err == nil {
		t.Fatal("--pure accepted")
	}
	plan, native, err = InteractivePlan([]string{"--log-level", "--pure"}, nil)
	if err != nil || native || !slices.Equal(plan.Args, []string{"--log-level", "--pure"}) {
		t.Fatalf("native pure value = %#v/%v/%v", plan, native, err)
	}
}

func TestNativeIDsFollowProductPrefixAndBusIdentityBounds(t *testing.T) {
	if !validNativeID("ses_日本") || validNativeID("ses bad") || validNativeID("ses_\xff") {
		t.Fatal("native session id boundary changed")
	}
	if !validPermissionID("per.dotted") || validPermissionID("request") {
		t.Fatal("native permission id prefix changed")
	}
}
