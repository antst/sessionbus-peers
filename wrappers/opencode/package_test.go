// SPDX-License-Identifier: MIT

package opencode

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLiteralArchiveInstallsTwiceWithoutNodeAndImportsNativeEntries(t *testing.T) {
	out := t.TempDir()
	build := exec.Command("sh", "../../scripts/package-product", "opencode", out)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("archive build: %v\n%s", err, output)
	}
	archive := filepath.Join(out, fmt.Sprintf("opencode-peer-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH))
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zipped.Close()
	reader := tar.NewReader(zipped)
	payload := filepath.Join(out, "payload")
	modules := map[string]bool{}
	skills := 0
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if filepath.IsAbs(h.Name) || strings.Contains(h.Name, "..") {
			t.Fatal("unsafe archive", h.Name)
		}
		target := filepath.Join(payload, h.Name)
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if h.Typeflag != tar.TypeReg {
			t.Fatal("unexpected archive type", h.Name, h.Typeflag)
		}
		if strings.HasSuffix(h.Name, ".test.mjs") || strings.Contains(h.Name, "fixture") || strings.HasSuffix(h.Name, "/bin.mjs") || strings.HasSuffix(h.Name, "/install.mjs") || strings.HasSuffix(h.Name, "/sessionbus.mjs") {
			t.Fatal("stale/development payload", h.Name)
		}
		if strings.HasPrefix(h.Name, "plugin/node_modules/") && !strings.HasPrefix(h.Name, "plugin/node_modules/@sessionbus/kit/") && h.Name != "plugin/node_modules/.package-lock.json" {
			t.Fatal("unexpected JS runtime dependency", h.Name)
		}
		if strings.HasSuffix(h.Name, "/SKILL.md") {
			skills++
			if h.Name != "plugin/skills/sessionbus/SKILL.md" {
				t.Fatal("non-generic skill", h.Name)
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, body, os.FileMode(h.Mode)); err != nil {
			t.Fatal(err)
		}
		modules[h.Name] = true
	}
	if skills != 1 {
		t.Fatal("wrong generic skill count", skills)
	}
	for _, entry := range []string{"server.mjs", "tui.mjs", "peer.mjs", "owners.mjs", "delivery.mjs", "sessionbus-tool.json"} {
		if !modules["plugin/"+entry] {
			t.Fatal("missing native module", entry)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	check := exec.Command(node, "--input-type=module", "-e", `import server from './server.mjs'; import tui from './tui.mjs'; if (Object.keys(await server.server()).length || await tui.tui({}) !== undefined) throw Error('ordinary activation');`)
	check.Dir = filepath.Join(payload, "plugin")
	check.Env = []string{"PATH=" + os.Getenv("PATH")}
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("actual packed entry imports: %v %s", err, output)
	}
	home := filepath.Join(out, "real home #test")
	tools := filepath.Join(out, "tools")
	if err := os.MkdirAll(tools, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cat", "dirname", "mkdir", "mktemp", "cp", "install", "mv", "rm", "ln"} {
		executable, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(executable, filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tools, "opencode"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "opencode.jsonc"), []byte("{// retained comment\n\"plugin\":[[\"other\",{\"number\":1e30}],\"@sessionbus/opencode@old\"]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".local", "libexec", "sessionbus", "opencode")
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		stale := filepath.Join(root, "plugin", "skills", "opencode-lane")
		if err := os.MkdirAll(stale, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stale, "SKILL.md"), []byte("obsolete"), 0o644); err != nil {
			t.Fatal(err)
		}
		command := exec.Command(shell, filepath.Join(payload, "install"))
		command.Env = []string{"HOME=" + home, "PATH=" + tools}
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("literal install%d without Node: %v %s", round, err, output)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Fatal("stale skill retained", err)
		}
	}
	want := (&url.URL{Scheme: "file", Path: filepath.Join(root, "plugin")}).String()
	for _, name := range []string{"opencode.jsonc", "tui.jsonc"} {
		entries := installedPluginEntries(t, filepath.Join(config, name), false)
		count := 0
		for _, entry := range entries {
			if entry == want {
				count++
			}
		}
		if count != 1 {
			t.Fatal("wrong native config registration", name, entries)
		}
	}
	nativeConfig, err := os.ReadFile(filepath.Join(config, "opencode.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nativeConfig), "// retained comment") || !strings.Contains(string(nativeConfig), `["other",{"number":1e30}]`) {
		t.Fatal("unrelated native config rewritten", string(nativeConfig))
	}
	var manifest struct {
		Dependencies map[string]string `json:"dependencies"`
		Bin          any               `json:"bin"`
	}
	b, err := os.ReadFile(filepath.Join(root, "plugin", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Bin != nil || len(manifest.Dependencies) != 1 || manifest.Dependencies["@sessionbus/kit"] != "https://pkg.pr.new/@sessionbus/kit@8cc6a59" {
		t.Fatal("wrong installed dependency or installer", manifest)
	}
}
