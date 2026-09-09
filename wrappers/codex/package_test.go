// SPDX-License-Identifier: MIT
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCodexInstallProcess(t *testing.T) {
	mode := os.Getenv("CODEX_INSTALL_FIXTURE")
	if mode == "" {
		return
	}
	if mode == "marketplace" {
		os.Exit(0)
	}
	if mode == "add" {
		fmt.Printf(`{"pluginId":"codex@sessionbus-peers","installedPath":%q}`, os.Getenv("CODEX_INSTALL_PATH"))
		os.Exit(0)
	}
	dec, enc := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	var methods []string
	for {
		var frame appFrame
		if err := dec.Decode(&frame); err != nil {
			if err != io.EOF {
				os.Exit(8)
			}
			break
		}
		methods = append(methods, frame.Method)
		if frame.Method == "initialized" {
			continue
		}
		var result any = map[string]any{}
		if frame.Method == "config/value/write" {
			var params map[string]any
			if json.Unmarshal(frame.Params, &params) != nil {
				os.Exit(9)
			}
			expected := map[string]any{"keyPath": `plugins."codex@sessionbus-peers".enabled`, "value": false, "mergeStrategy": "replace"}
			if !reflect.DeepEqual(params, expected) {
				os.Exit(10)
			}
			result = map[string]string{"status": os.Getenv("CODEX_INSTALL_STATUS"), "filePath": "/real-user/config.toml", "version": "fixture"}
		} else if frame.Method != "initialize" {
			os.Exit(11)
		}
		if enc.Encode(map[string]any{"id": frame.ID, "result": result}) != nil {
			os.Exit(12)
		}
	}
	if !reflect.DeepEqual(methods, []string{"initialize", "initialized", "config/value/write"}) {
		os.Exit(13)
	}
	os.Exit(0)
}
func installerStandIn(t *testing.T, status string) {
	t.Helper()
	t.Setenv("CODEX_INSTALL_STATUS", status)
	old := laneCommand
	laneCommand = func(name string, args ...string) *exec.Cmd {
		if name != "codex" || !reflect.DeepEqual(args, []string{"app-server", "--stdio"}) {
			t.Fatalf("unexpected native argv %s %q", name, args)
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestCodexInstallProcess")
		cmd.Env = append(os.Environ(), "CODEX_INSTALL_FIXTURE=config")
		return cmd
	}
	t.Cleanup(func() { laneCommand = old })
}
func TestNativeInstallerMakesOnlyExactUserKeyEdit(t *testing.T) {
	for _, status := range []string{"ok", "okOverridden"} {
		t.Run(status, func(t *testing.T) {
			installerStandIn(t, status)
			result, err := disableInstalledPlugin(context.Background())
			if result.Status != status || (err == nil) != (status == "ok") {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}
func TestPackageRegistersAbsoluteAliasAndReturnedNativePath(t *testing.T) {
	installerStandIn(t, "ok")
	root := t.TempDir()
	market := filepath.Join(t.TempDir(), "market place")
	native := filepath.Join(t.TempDir(), "native installed path")
	if err := os.MkdirAll(filepath.Join(market, "codex"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(native, ".codex-plugin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(native, ".codex-plugin", "plugin.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_INSTALL_PATH", native)
	old := installNativeCommand
	defer func() { installNativeCommand = old }()
	var invoked [][]string
	installNativeCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "codex" {
			t.Fatal(name)
		}
		invoked = append(invoked, append([]string(nil), args...))
		mode := "add"
		if reflect.DeepEqual(args, []string{"plugin", "marketplace", "add", market}) {
			mode = "marketplace"
		} else if !reflect.DeepEqual(args, []string{"plugin", "add", PluginID, "--json"}) {
			t.Fatalf("argv=%q", args)
		}
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestCodexInstallProcess")
		cmd.Env = append(os.Environ(), "CODEX_INSTALL_FIXTURE="+mode)
		return cmd
	}
	var output bytes.Buffer
	if err := registerPlugin(context.Background(), root, market, &output); err != nil {
		t.Fatal(err)
	}
	if len(invoked) != 2 {
		t.Fatal(invoked)
	}
	path, err := installedCodexPlugin(root)
	if err != nil || path != native {
		t.Fatalf("path=%q err=%v", path, err)
	}
	b, err := os.ReadFile(filepath.Join(market, "codex", ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if json.Unmarshal(b, &got) != nil {
		t.Fatal(string(b))
	}
	want := map[string]any{"mcpServers": map[string]any{"sessionbus": map[string]any{"command": filepath.Join(root, "bin", MCPAlias), "env_vars": []any{"SESSIONBUS_GROUPS", "SESSIONBUS_SOCKET", EndpointEnv}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MCP config=%s", b)
	}
	if !strings.Contains(output.String(), "configWrite") {
		t.Fatal(output.String())
	}
	if err := os.Remove(filepath.Join(native, ".codex-plugin", "plugin.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := installedCodexPlugin(root); err == nil {
		t.Fatal("missing actual native plugin accepted")
	}
}
func TestActivationPrefixPreservesCallerOverride(t *testing.T) {
	native := []string{"-c", `plugins.codex@sessionbus-peers.enabled=false`, "--", "operand"}
	actual := append(ActivationArguments(), native...)
	want := []string{"-c", "features.plugins=true", "-c", `plugins.codex@sessionbus-peers.enabled=true`, "-c", `plugins.codex@sessionbus-peers.enabled=false`, "--", "operand"}
	if !reflect.DeepEqual(actual, want) {
		t.Fatal(actual)
	}
}
