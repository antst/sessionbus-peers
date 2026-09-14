// SPDX-License-Identifier: MIT

package opencodefamily

import (
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
	"slices"
	"testing"
)

func TestOpenArguments(t *testing.T) {
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
}

func TestNativeIDsFollowProductPrefixAndBusIdentityBounds(t *testing.T) {
	if !validNativeID("ses_日本") || validNativeID("ses bad") || validNativeID("ses_\xff") {
		t.Fatal("native session id boundary changed")
	}
	if !validPermissionID("per.dotted") || validPermissionID("request") {
		t.Fatal("native permission id prefix changed")
	}
}
