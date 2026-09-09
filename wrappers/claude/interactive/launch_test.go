// SPDX-License-Identifier: MIT

package interactive

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLaunchPreservesNativeArgumentsAndGroupGrammar(t *testing.T) {
	cases := []struct {
		args   []string
		native []string
		groups string
	}{
		{[]string{"--resume", "test1", "-g", "test,test2", "--yolo"}, []string{"--resume", "test1", "--dangerously-skip-permissions"}, `["test","test2"]`},
		{[]string{"--resume", "test1", "-g", "test,test2", "--dangerously-skip-permissions"}, []string{"--resume", "test1", "--dangerously-skip-permissions"}, `["test","test2"]`},
		{[]string{"--resume", "test1", "-g", "test", "-g", "test2,test3", "--yolo"}, []string{"--resume", "test1", "--dangerously-skip-permissions"}, `["test","test2","test3"]`},
		{[]string{"--group=first", "--resume", "test1", "-g", "test", "--group", "test2,test3", "--yolo", "--group="}, []string{"--resume", "test1", "--dangerously-skip-permissions"}, `["first","test","test2","test3"]`},
		{[]string{"--", "--group", "literal", "--group=x", "--yolo"}, []string{"--", "--group", "literal", "--group=x", "--yolo"}, `[]`},
		{[]string{"-g", "first", "--resume", "test1", "--yolo", "-g", "last"}, []string{"--resume", "test1", "--dangerously-skip-permissions"}, `["first","last"]`},
		{[]string{"-n", "a", "--", "-g", "native", "--yolo"}, []string{"-n", "a", "--", "-g", "native", "--yolo"}, `[]`},
		{[]string{"-n", "", "-g", "a,, a,a", "-n", "b", "--", "positional"}, []string{"-n", "", "-n", "b", "--", "positional"}, `["a",""," a","a"]`},
		{[]string{"mcp", "--unknown", "-g", ""}, []string{"mcp", "--unknown"}, `[]`},
		{[]string{"--", "-g"}, []string{"--", "-g"}, `[]`},
		{[]string{"--unknown", "a value", "-g", "one", "--model", "first", "--model", "", "--future=value"}, []string{"--unknown", "a value", "--model", "first", "--model", "", "--future=value"}, `["one"]`},
	}
	for _, tc := range cases {
		args, values, err := LaunchPlan(tc.args, map[string]string{"SESSIONBUS_GROUPS": `["inherited"]`, "KEEP": "value", "SESSIONBUS_SOCKET": "relative.sock"}, "/cwd", "/root with spaces/plugin", 1000)
		if err != nil {
			t.Fatal(err)
		}
		want := append([]string{"--allowedTools", PublicTool, "--plugin-dir", "/root with spaces/plugin"}, tc.native...)
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("argv=%q want%q", args, want)
		}
		env := Environment(values)
		if env["SESSIONBUS_GROUPS"] != tc.groups || env["KEEP"] != "value" || env["SESSIONBUS_SOCKET"] != "/cwd/relative.sock" {
			t.Fatalf("unexpected launch environment")
		}
	}
}

func TestLaunchRejectsMissingGroupValueBeforeNativeBoundary(t *testing.T) {
	for _, args := range [][]string{{"-g"}, {"--group"}, {"--resume", "test1", "-g"}, {"-g", "--", "literal"}, {"--group", "--", "literal"}} {
		if _, _, err := LaunchPlan(args, nil, "/cwd", "/plugin", 1000); err == nil || !strings.HasSuffix(err.Error(), " requires a group list") {
			t.Fatalf("args=%q err=%v", args, err)
		}
	}
}

func TestNativeLookupAndSocketResolution(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	p, err := NativePath(map[string]string{"PATH": "missing:"}, dir)
	if err != nil || p != filepath.Join(dir, "claude") {
		t.Fatalf("path=%s err=%v", p, err)
	}
	if p := SocketPath(map[string]string{}, dir, 123); p != "/tmp/sessionbus-123/presence.sock" {
		t.Fatal(p)
	}
	if p := SocketPath(map[string]string{"XDG_RUNTIME_DIR": "/runtime"}, dir, 123); p != "/runtime/sessionbus/presence.sock" {
		t.Fatal(p)
	}
}
