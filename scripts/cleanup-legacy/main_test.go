// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T) *cleaner {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir()) // Match main's canonical home on macOS too.
	if err != nil {
		t.Fatal(err)
	}
	return &cleaner{home: home, config: filepath.Join(home, ".config"), out: &bytes.Buffer{}, run: func(bin string, args ...string) ([]byte, error) {
		t.Fatalf("unexpected command: %s %v", bin, args)
		return nil, nil
	}}
}

func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestQuarantinePreservesCurrentAliasAndLegacyTarget(t *testing.T) {
	c := fixture(t)
	old := filepath.Join(c.home, ".local/libexec/agent-sessions")
	current := filepath.Join(c.home, ".local/libexec/sessionbus/codex/bin/codex-peer")
	put(t, filepath.Join(old, ".codex-plugin/plugin.json"), `{"name":"agent-sessions"}`)
	put(t, filepath.Join(old, "host/current/bin/agent-sessions"), "old binary")
	put(t, current, "current binary")
	bin := filepath.Join(c.home, ".local/bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"codex-peer": current, "codex-peer-lane": filepath.Join(old, "host/current/bin/agent-sessions")} {
		if err := os.Symlink(target, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the destructive primitive independently of host /proc support.
	if err := c.move(filepath.Join(bin, "codex-peer-lane"), "old alias"); err != nil {
		t.Fatal(err)
	}
	if err := c.move(old, "old tree"); err != nil {
		t.Fatal(err)
	}
	if err := c.execute(false); err != nil {
		t.Fatal(err)
	}
	if !exists(old) || c.backup != "" {
		t.Fatal("preview mutated installation")
	}
	if err := c.execute(true); err != nil {
		t.Fatal(err)
	}
	if exists(old) || exists(filepath.Join(bin, "codex-peer-lane")) {
		t.Fatal("legacy files remain")
	}
	if b, err := os.ReadFile(filepath.Join(bin, "codex-peer")); err != nil || string(b) != "current binary" {
		t.Fatalf("current bin changed: %s %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(c.backup, "files/.local/libexec/agent-sessions/host/current/bin/agent-sessions")); err != nil || string(b) != "old binary" {
		t.Fatal("backup lost", err)
	}
}

func TestConfigEditPreservesSymlinkConcurrentNativeChangesAndBackup(t *testing.T) {
	c := fixture(t)
	real := filepath.Join(c.home, "dotfiles/claude.json")
	path := filepath.Join(c.home, ".claude/settings.json")
	put(t, real, `{"enabledPlugins":{"agent-sessions@agent-sessions":true,"current@market":true},"number":9007199254740993}`)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, path); err != nil {
		t.Fatal(err)
	}
	if err := c.removeKey(path, "enabledPlugins", legacyPlugin); err != nil {
		t.Fatal(err)
	}
	// A native command or user changed another key after the plan was made.
	before := `{"enabledPlugins":{"agent-sessions@agent-sessions":true,"current@market":false},"number":9007199254740993}`
	put(t, real, before)
	if err := c.execute(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(path); err != nil {
		t.Fatal("config symlink replaced", err)
	}
	d, err := readObject(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(d["number"]) != "9007199254740993" {
		t.Fatal("number changed")
	}
	var plugins map[string]bool
	if err := json.Unmarshal(d["enabledPlugins"], &plugins); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plugins, map[string]bool{"current@market": false}) {
		t.Fatal(plugins)
	}
	b, err := os.ReadFile(filepath.Join(c.backup, "config/.claude/settings.json"))
	if err != nil || string(b) != before {
		t.Fatal("wrong backup", err)
	}
	second := fixture(t)
	second.home = c.home
	if err := second.removeKey(path, "enabledPlugins", legacyPlugin); err != nil || len(second.actions) != 0 {
		t.Fatal("not idempotent", err)
	}
}

func TestRejectIndirectParentAndReplacedAlias(t *testing.T) {
	c := fixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(c.home, ".local")); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(outside, "bin/agent-sessions"), "unrelated")
	if err := c.move(filepath.Join(c.home, ".local/bin/agent-sessions"), "test"); err == nil {
		t.Fatal("followed indirect parent")
	}
	p := filepath.Join(c.home, "alias")
	if err := os.Symlink("old", p); err != nil {
		t.Fatal(err)
	}
	if err := c.move(p, "test"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("new", p); err != nil {
		t.Fatal(err)
	}
	c.backup = t.TempDir()
	if err := c.actions[0].do(); err == nil {
		t.Fatal("removed replacement")
	}
}

func TestNativeMCPRemovalIsOwnershipCheckedAndExact(t *testing.T) {
	c := fixture(t)
	bin := filepath.Join(c.home, ".local/bin/codex")
	put(t, bin, "")
	if err := os.Chmod(bin, 0700); err != nil {
		t.Fatal(err)
	}
	var writes []string
	c.run = func(_ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "mcp list --json":
			return []byte(`[{"name":"agent_sessions","transport":{"command":"/home/u/.local/bin/codex-peer","args":["mcp"]}},{"name":"other","transport":{"command":"unrelated"}}]`), nil
		case "plugin list --json":
			return []byte(`{"installed":[{"pluginId":"codex@sessionbus-peers"}]}`), nil
		case "plugin marketplace list --json":
			return []byte(`{"marketplaces":[{"name":"sessionbus-peers"}]}`), nil
		default:
			writes = append(writes, strings.Join(args, " "))
			return []byte("removed\n"), nil
		}
	}
	if err := c.codex(); err != nil {
		t.Fatal(err)
	}
	if err := c.execute(false); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 0 {
		t.Fatal("preview wrote config")
	}
	if err := c.execute(true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(writes, []string{"mcp remove agent_sessions"}) {
		t.Fatal(writes)
	}
	if legacyCommand("unrelated", []string{"mcp"}) || legacyCommand("codex-peer", []string{"--help"}) {
		t.Fatal("name alone authorized removal")
	}
}

func TestGrokCleanupUsesNativeInstalledRepository(t *testing.T) {
	c := fixture(t)
	path := filepath.Join(c.home, ".grok/installed-plugins/grok-123/.grok-plugin/plugin.json")
	put(t, path, `{"name":"agent-sessions"}`)
	bin := filepath.Join(c.home, ".local/bin/grok")
	put(t, bin, "")
	if err := os.Chmod(bin, 0700); err != nil {
		t.Fatal(err)
	}
	removed := false
	c.run = func(_ string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "plugin list --json" {
			if removed {
				return []byte(`[]`), nil
			}
			return json.Marshal([]map[string]string{{"name": "agent-sessions", "status": "installed", "path": filepath.Dir(filepath.Dir(path))}})
		}
		if strings.Join(args, " ") != "plugin uninstall agent-sessions --keep-data" {
			t.Fatalf("unexpected mutation %v", args)
		}
		removed = true
		return nil, nil
	}
	if err := c.grok(); err != nil {
		t.Fatal(err)
	}
	if err := c.execute(true); err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("legacy native repository missed")
	}
	c.actions = nil
	if err := c.grok(); err != nil {
		t.Fatal(err)
	}
	if len(c.actions) != 0 {
		t.Fatal("repeat cleanup not empty")
	}
}
