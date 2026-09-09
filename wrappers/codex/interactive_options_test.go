// SPDX-License-Identifier: MIT

package codex

import (
	"reflect"
	"testing"
)

func TestInteractiveConfigSpellingsAndWrapperGroups(t *testing.T) {
	tests := []struct {
		name                         string
		args, native, config, groups []string
	}{
		{"repeat", []string{"-g", "a,b", "-c", "model=-c", "--resume", "--group=c", "--config=x=1", "-cy=2", "-c=z=3", "--yolo"}, []string{"-c", "model=-c", "resume", "--config=x=1", "-cy=2", "-c=z=3", "--dangerously-bypass-approvals-and-sandbox"}, []string{"-c", "model=-c", "--config=x=1", "-cy=2", "-c=z=3"}, []string{"a", "b", "c"}},
		{"delimiter", []string{"-ga", "--", "-g", "b", "--remote=x", "-c", "x=2"}, []string{"--", "-g", "b", "--remote=x", "-c", "x=2"}, nil, []string{"a"}},
		{"missing-value", []string{"-c", "--config=x=1", "--", "-c"}, []string{"-c", "--config=x=1", "--", "-c"}, []string{"-c", "--config=x=1"}, []string{}},
		{"literal-value", []string{"--config", "value=--remote", "-c", "value=-g", "-p", "personal"}, []string{"--config", "value=--remote", "-c", "value=-g", "-p", "personal"}, []string{"--config", "value=--remote", "-c", "value=-g"}, []string{}},
		{"empty-and-hyphen-attached", []string{"--config=", "--config=-c=literal", "-g=one", "--resume=native-id"}, []string{"--config=", "--config=-c=literal", "resume", "native-id"}, []string{"--config=", "--config=-c=literal"}, []string{"one"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseInteractiveOptions(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(o.native, tt.native) || !reflect.DeepEqual(o.config, tt.config) || !reflect.DeepEqual(o.groups, tt.groups) {
				t.Fatalf("got %#v", o)
			}
		})
	}
}
func TestInteractiveRejectsCallerRemote(t *testing.T) {
	for _, arg := range []string{"--remote", "--remote=unix:///other", "--remote-auth-token-env", "--remote-auth-token-env=X"} {
		if _, err := parseInteractiveOptions([]string{"resume", arg}); err == nil {
			t.Fatal(arg)
		}
	}
}
