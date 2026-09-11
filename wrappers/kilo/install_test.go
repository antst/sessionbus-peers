// SPDX-License-Identifier: MIT
package kilo

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestKiloMaintenanceFacadeUsesNativeConfigAndNoRuntime(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	plugin := filepath.Join(root, "permanent package #1")
	if err := os.MkdirAll(plugin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "package.json"), []byte(`{"name":"@sessionbus/kilo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("PATH", "")
	skill := filepath.Join(plugin, "skills", "sessionbus", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("fixture skill"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{nil, {"--plugin-dir", "relative"}, {"--specifier", "@sessionbus/opencode@old"}, {"--remove", "extra"}} {
		if err := InstallPlugin(args); err == nil {
			t.Fatal("invalid maintenance arguments accepted", args)
		}
	}
	for i := 0; i < 2; i++ {
		if err := InstallPlugin([]string{"--plugin-dir", plugin}); err != nil {
			t.Fatal(err)
		}
	}
	want := (&url.URL{Scheme: "file", Path: plugin}).String()
	for _, name := range []string{"kilo.jsonc", "tui.jsonc"} {
		b, err := os.ReadFile(filepath.Join(config, "kilo", name))
		var doc struct {
			Plugin []string `json:"plugin"`
		}
		if err != nil || json.Unmarshal(b, &doc) != nil || len(doc.Plugin) != 1 || doc.Plugin[0] != want {
			t.Fatalf("wrong native registration %s: %s/%v", name, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(config, "opencode")); !os.IsNotExist(err) {
		t.Fatal("other product config touched", err)
	}
	if changed, err := ConfigurePlugin(InstallOptions{Specifier: want}); err != nil || changed {
		t.Fatal("facade repeat differs", changed, err)
	}
	if err := InstallPlugin([]string{"--remove"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := ConfigurePlugin(InstallOptions{Remove: true}); err != nil || changed {
		t.Fatal("repeat removal differs", changed, err)
	}
}
