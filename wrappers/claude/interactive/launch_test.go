// SPDX-License-Identifier: MIT

package interactive

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLaunchPreservesNativeArgumentsAndGroupGrammar(t *testing.T) {
	cases := []struct {
		args   []string
		native []string
		groups string
	}{
		{[]string{"-n", "a", "--", "-g", "native"}, []string{"-n", "a", "--"}, `["native"]`},
		{[]string{"-n", "", "-n", "b", "--", "positional", "-g", "a,, a,a"}, []string{"-n", "", "-n", "b", "--", "positional"}, `["a",""," a","a"]`},
		{[]string{"mcp", "--unknown", "-g", ""}, []string{"mcp", "--unknown"}, `[]`},
		{[]string{"--", "-g"}, []string{"--", "-g"}, `[]`},
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
