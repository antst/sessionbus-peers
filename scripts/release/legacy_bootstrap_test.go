// SPDX-License-Identifier: MIT
package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLegacyBootstrapKeepsPublishedLinksOnCombinedAssets(t *testing.T) {
	for role, repo := range map[string]string{"claude": "claude-peer", "grok": "grok-peer", "qwen": "qwen-peer", "opencode": "opencode-kilo", "kilo": "opencode-kilo", "pi": "pi-omp", "omp": "pi-omp"} {
		for _, tc := range []struct{ version, mirror, base string }{
			{"", "", "https://github.com/sessionbus/codex-peer/releases/download/v0.5.3"},
			{"latest", "", "https://github.com/sessionbus/codex-peer/releases/download/v0.5.3"},
			{"v0.5.2", "", "https://github.com/sessionbus/codex-peer/releases/download/v0.5.2"},
			{"development", "", "https://github.com/sessionbus/codex-peer/releases/download/development"},
			{"latest", "file:///offline/combined", "file:///offline/combined"},
		} {
			t.Run(role+"/"+tc.version+tc.mirror, func(t *testing.T) {
				dir := t.TempDir()
				log := filepath.Join(dir, "requests")
				if err := os.WriteFile(filepath.Join(dir, "curl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LEGACY_BOOTSTRAP_LOG\"\nexit 39\n"), 0700); err != nil {
					t.Fatal(err)
				}
				c := exec.Command("sh", filepath.Join("..", "install-"+role+".sh"))
				for _, e := range os.Environ() {
					if !strings.HasPrefix(e, "PATH=") && !strings.HasPrefix(e, "SESSIONBUS_VERSION=") && !strings.HasPrefix(e, "SESSIONBUS_DOWNLOAD_ROOT=") && !strings.HasPrefix(e, "LEGACY_BOOTSTRAP_LOG=") {
						c.Env = append(c.Env, e)
					}
				}
				c.Env = append(c.Env, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "LEGACY_BOOTSTRAP_LOG="+log)
				if tc.version != "" {
					c.Env = append(c.Env, "SESSIONBUS_VERSION="+tc.version)
				}
				if tc.mirror != "" {
					c.Env = append(c.Env, "SESSIONBUS_DOWNLOAD_ROOT="+tc.mirror)
				}
				out, err := c.CombinedOutput()
				if err == nil {
					t.Fatalf("download failure ignored: %s", out)
				}
				if !strings.Contains(string(out), "https://github.com/sessionbus/"+repo) {
					t.Fatalf("missing destination: %s", out)
				}
				requests, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				want := tc.base + "/" + role + "-peer-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
				if !strings.Contains("\n"+string(requests), "\n"+want+"\n") || strings.Contains(string(requests), "/releases/latest") {
					t.Fatalf("unexpected request: %s; want %s", requests, want)
				}
			})
		}
	}
}
