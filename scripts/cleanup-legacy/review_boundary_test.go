// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReviewNativeRemovalRejectsConfigOutsideHome(t *testing.T) {
	c := fixture(t)
	externalDir := t.TempDir()
	external := filepath.Join(externalDir, "config.toml")
	put(t, external, "external original")
	path := filepath.Join(c.home, ".codex")
	if err := os.Symlink(externalDir, path); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(c.home, ".local/bin/codex")
	put(t, bin, "")
	if err := os.Chmod(bin, 0700); err != nil {
		t.Fatal(err)
	}
	writes := 0
	c.run = func(_ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "mcp list --json":
			return []byte(`[{"name":"agent_sessions","transport":{"command":"agent-sessions","args":["mcp"]}}]`), nil
		case "plugin list --json":
			return []byte(`{"installed":[]}`), nil
		case "plugin marketplace list --json":
			return []byte(`{"marketplaces":[]}`), nil
		default:
			writes++
			return nil, nil
		}
	}
	if err := c.scan(); err != nil {
		return
	}
	err := c.execute(true)
	if err == nil || writes != 0 {
		t.Fatalf("native mutation allowed through external config symlink: err=%v commands=%d", err, writes)
	}
}
func TestReviewClaudeMixedScopesRefuseBeforePlan(t *testing.T) {
	c := fixture(t)
	bin := filepath.Join(c.home, ".local/bin/claude")
	put(t, bin, "")
	if err := os.Chmod(bin, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(c.home, ".claude/plugins/installed_plugins.json"), `{"plugins":{"agent-sessions@agent-sessions":[{"scope":"user"},{"scope":"project"}]}}`)
	if err := c.claude(); err == nil {
		t.Fatalf("non-user scope after user was ignored; actions=%v", c.actions)
	}
}

func TestReviewServiceOwnershipRequiresExecutableNotComment(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux service selection")
	}
	c := fixture(t)
	path := filepath.Join(c.config, "systemd/user/agent-sessions.service")
	put(t, path, "[Unit]\nDescription=Replacement service (previous /agent-sessions)\n[Service]\nExecStart=/usr/bin/unrelated-service\n")
	c.run = func(_ string, args ...string) ([]byte, error) { return []byte("inactive\n"), nil }
	if err := c.services(); err == nil {
		t.Fatalf("unrelated executable selected by legacy token in Description: actions=%v", c.actions)
	}
}
