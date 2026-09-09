// SPDX-License-Identifier: MIT
package codex

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestCodexArchiveContainsOneArtifactAndOneGenericSkill(t *testing.T) {
	root := filepath.Join("..", "..")
	output := filepath.Join(t.TempDir(), "archive output")
	cmd := exec.Command("sh", "scripts/package-codex", output)
	cmd.Dir = root
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pack: %v\n%s", err, raw)
	}
	file, err := os.Open(filepath.Join(output, "codex-peer-"+runtime.GOOS+"-"+runtime.GOARCH+".tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var regular []string
	aliases := map[string]string{}
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeSymlink:
			aliases[h.Name] = h.Linkname
		case tar.TypeReg:
			regular = append(regular, h.Name)
			if h.Name == "marketplace/codex/.codex-plugin/plugin.json" {
				var m struct{ Name, Version string }
				if json.NewDecoder(reader).Decode(&m) != nil || m.Name != "codex" || !strings.HasPrefix(m.Version, "0.5.0-codex.g") {
					t.Fatalf("manifest=%+v", m)
				}
			}
		default:
			t.Fatalf("unexpected tar member type %d: %s", h.Typeflag, h.Name)
		}
	}
	sort.Strings(regular)
	want := []string{"LICENSE", "README.md", "bin/codex-peer", "install", "marketplace/.agents/plugins/marketplace.json", "marketplace/codex/.codex-plugin/plugin.json", "marketplace/codex/.mcp.json", "marketplace/codex/skills/sessionbus/SKILL.md", "uninstall"}
	sort.Strings(want)
	if !reflect.DeepEqual(regular, want) {
		t.Fatalf("regular payload=%q", regular)
	}
	wantAliases := map[string]string{"bin/" + MCPAlias: "codex-peer", "bin/" + BrokerAlias: "codex-peer", "bin/" + InstallAlias: "codex-peer"}
	if !reflect.DeepEqual(aliases, wantAliases) {
		t.Fatalf("private aliases=%v", aliases)
	}
}
