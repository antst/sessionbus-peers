// SPDX-License-Identifier: MIT

package sessionbus_peers_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	peersModule             = "github.com/antst/sessionbus-peers"
	sdkModule               = "github.com/antst/sessionbus/bus/sdk/go"
	sdkVersion              = "v0.1.0-pre.2.0.20260909142026-a7c10044f3b4"
	citationCount           = 255
	citationReachabilitySHA = "21daf6711345d78f02bdc5f2062bdd3b0c726633bb9f565adc325dbd06bec6d9"
	factsHeader             = "> Historical source note: citations to pre-split Sessionbus paths resolve in\n> the Forgejo `ai/sessionbus` repository through its `legacy-*` branches.\n> Citations to product source resolve in the external repository and full\n> commit recorded by the split archive manifest. Host evidence paths are\n> immutable external artifacts, not repository paths."
)

func TestRepositoryBoundary(t *testing.T) {
	allowed := map[string]bool{
		".forgejo": true, ".git": true,
		".github": true, ".gitignore": true,
		".golangci.yml": true, "LICENSE": true, "README.md": true,
		"architecture_test.go": true, "claude": true, "cmd": true,
		"docs": true, "go.mod": true, "go.sum": true, "grok": true,
		"internal": true, "opencode": true, "qwen": true, "scripts": true, "wrappers": true,
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			t.Errorf("path is outside the peers boundary: %s", entry.Name())
		}
	}
	for _, path := range []string{"bus", "integrations", "deploy", "examples", ".specify", ".agents", ".codex-plugin", "hooks", "skills", "go.work", "go.work.sum", "Makefile"} {
		if _, err := os.Stat(filepath.FromSlash(path)); !os.IsNotExist(err) {
			t.Errorf("legacy or cross-repository path remains: %s", path)
		}
	}
	wantCommands := []string{"claude-peer", "codex-peer", "grok-peer", "opencode-peer", "qwen-peer"}
	gotCommands := directoryNames(t, "cmd")
	if !equalStrings(gotCommands, wantCommands) {
		t.Errorf("peer command roots = %v, want %v", gotCommands, wantCommands)
	}
	if got := directoryNames(t, "internal"); !equalStrings(got, []string{"testsocket"}) {
		t.Errorf("internal roots = %v, want [testsocket]", got)
	}
	if _, err := os.Stat(".github/workflows/release.yml"); !os.IsNotExist(err) {
		t.Fatal("initial peers root must not contain a release workflow")
	}
}

func TestModuleAndImportBoundary(t *testing.T) {
	module := read(t, "go.mod")
	if !bytes.Contains(module, []byte("module "+peersModule+"\n")) || !bytes.Contains(module, []byte(sdkModule+" "+sdkVersion)) {
		t.Fatalf("go.mod violates the peers module shape:\n%s", module)
	}
	if bytes.Contains(module, []byte("replace ")) {
		t.Fatal("peers go.mod contains a filesystem replacement")
	}
	for _, path := range []string{"go.work", "go.work.sum"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s must not be committed", path)
		}
	}
	checkGoImports(t, ".", func(path, imported string) {
		if strings.HasPrefix(imported, "github.com/antst/sessionbus/bus/internal/") {
			t.Errorf("%s imports daemon internal package %s", path, imported)
		}
		if strings.HasPrefix(imported, "github.com/antst/sessionbus/") && imported != sdkModule && !strings.HasPrefix(imported, sdkModule+"/") {
			t.Errorf("%s imports non-SDK Sessionbus package %s", path, imported)
		}
		if strings.HasPrefix(imported, "github.com/antst/sessionbus/wrappers/") {
			t.Errorf("%s retains pre-split wrapper import %s", path, imported)
		}
	})
	command := exec.Command("go", "list", "-m", "all")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("independent peers module graph: %v\n%s", err, output)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
		if bytes.Equal(line, []byte("github.com/antst/sessionbus")) {
			t.Fatalf("module graph contains the daemon root:\n%s", output)
		}
	}
}

func TestFactsHeadersAndReachabilityAudit(t *testing.T) {
	entries, err := os.ReadDir("docs/products")
	if err != nil {
		t.Fatal(err)
	}
	citationPattern := regexp.MustCompile("`([0-9a-f]{7,40}:[^`\\s]+)`")
	citations := make([]string, 0, citationCount)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join("docs/products", entry.Name())
		body := read(t, path)
		lines := bytes.Split(body, []byte{'\n'})
		if len(lines) < 8 || len(lines[0]) == 0 || !bytes.Equal(bytes.Join(lines[2:7], []byte{'\n'}), []byte(factsHeader)) {
			t.Errorf("facts header is absent or misplaced: %s", path)
		}
		for _, match := range citationPattern.FindAllSubmatch(body, -1) {
			citations = append(citations, string(match[1]))
		}
	}
	sort.Strings(citations)
	digest := sha256.Sum256([]byte(strings.Join(citations, "\n") + "\n"))
	if len(citations) != citationCount || hex.EncodeToString(digest[:]) != citationReachabilitySHA {
		t.Fatalf("historical citations differ from the resolved split archive audit: count=%d sha256=%s", len(citations), hex.EncodeToString(digest[:]))
	}
}

func TestFormerBrandGuardForProductFacts(t *testing.T) {
	err := filepath.WalkDir("docs/products", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		cleaned, cleanErr := removeHistoricalExceptions(read(t, path))
		if cleanErr != nil {
			t.Errorf("%s: %v", path, cleanErr)
			return nil
		}
		if containsFormerBrand(cleaned) {
			t.Errorf("former brand remains outside a fenced capture, evidence path, or commit-qualified citation: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSPDXAndLicenseCoverage(t *testing.T) {
	if !bytes.Contains(read(t, "LICENSE"), []byte("MIT License")) {
		t.Fatal("root MIT license is missing")
	}
	if err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if ignoredDirectory(path) {
				return filepath.SkipDir
			}
			return nil
		}
		body := read(t, path)
		if bytes.Contains(body, []byte("SPDX-License-Identifier: "+"GPL")) {
			t.Errorf("GPL SPDX identifier remains in peers tree: %s", path)
		}
		if filepath.Ext(path) == ".go" && firstOrSecondLine(body) != "// SPDX-License-Identifier: MIT" {
			t.Errorf("Go source lacks MIT SPDX header: %s", path)
		}
		if filepath.Ext(path) == ".mjs" && firstOrSecondLine(body) != "// SPDX-License-Identifier: MIT" {
			t.Errorf("JavaScript source lacks MIT SPDX header: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"docs/designs/claude-0.5.0/held-lane-skills/codex-lane/scripts/lane-preflight",
		"docs/designs/claude-0.5.0/held-lane-skills/grok-lane/scripts/lane-preflight",
		"grok/scripts/native-entry", "scripts/codex-mcp", "scripts/test-codex-mcp",
	} {
		if firstOrSecondLine(read(t, path)) != "# SPDX-License-Identifier: MIT" {
			t.Errorf("shell source lacks MIT SPDX header: %s", path)
		}
	}
}

func TestOpenCodePackageBoundary(t *testing.T) {
	var manifest struct {
		Name       string            `json:"name"`
		Bin        map[string]string `json:"bin"`
		Files      []string          `json:"files"`
		Repository struct {
			Type      string `json:"type"`
			URL       string `json:"url"`
			Directory string `json:"directory"`
		} `json:"repository"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(read(t, "opencode/package.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "@sessionbus/opencode" || manifest.Bin["sessionbus-opencode-install"] != "bin.mjs" {
		t.Fatalf("OpenCode name/bin is invalid: %#v", manifest)
	}
	wantFiles := []string{"README.md", "bin.mjs", "commands", "install.mjs", "sessionbus.mjs", "skills"}
	sort.Strings(manifest.Files)
	if !equalStrings(manifest.Files, wantFiles) {
		t.Errorf("OpenCode package files = %v, want %v", manifest.Files, wantFiles)
	}
	if manifest.Repository.Type != "git" || manifest.Repository.URL != "git+https://github.com/antst/sessionbus-peers.git" || manifest.Repository.Directory != "opencode" {
		t.Errorf("OpenCode repository metadata is invalid: %#v", manifest.Repository)
	}
	if manifest.Dependencies["@sessionbus/kit"] != "0.1.0-pre.2" || strings.HasPrefix(manifest.Dependencies["@sessionbus/kit"], "file:") {
		t.Errorf("OpenCode kit dependency is not exact: %q", manifest.Dependencies["@sessionbus/kit"])
	}
	workflow := read(t, ".github/workflows/pkg-pr-new.yml")
	if !bytes.Contains(workflow, []byte("publish ./opencode")) || bytes.Contains(workflow, []byte("integrations/opencode")) {
		t.Fatal("pkg.pr.new does not publish only the rehomed OpenCode package")
	}
}

func TestRepositoryURLsAndRemovedPaths(t *testing.T) {
	for _, root := range []string{"claude", "grok", "opencode", "qwen", "scripts", "wrappers/README.md"} {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if ignoredDirectory(path) {
					return filepath.SkipDir
				}
				return nil
			}
			body := read(t, path)
			if bytes.Contains(body, []byte("github.com/antst/sessionbus.git")) || bytes.Contains(body, []byte("integrations/opencode")) {
				t.Errorf("active peer asset points to pre-split repository path: %s", path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetainedManifestCommandReachability(t *testing.T) {
	installed := map[string]bool{
		"${CLAUDE_PLUGIN_ROOT}/bin/sessionbus-mcp": true,
		"claude-peer": true,
		"codex-peer":  true,
		"grok-peer":   true,
		"qwen-peer":   true,
	}
	for _, item := range []struct {
		path    string
		command string
	}{
		{"claude/.mcp.json", manifestCommand(t, "claude/.mcp.json")},
		{"qwen/mcp.json", manifestCommand(t, "qwen/mcp.json")},
	} {
		if !installed[item.command] {
			t.Errorf("%s names an unresolved installed command %q", item.path, item.command)
		}
	}
	if command := manifestCommand(t, "grok/.mcp.json"); command != "${GROK_PLUGIN_ROOT}/scripts/native-entry" {
		t.Errorf("Grok manifest command = %q", command)
	}
	if info, err := os.Stat("grok/scripts/native-entry"); err != nil || info.Mode()&0o111 == 0 {
		t.Errorf("Grok manifest entry is absent or not executable: %v", err)
	}
	entry := read(t, "grok/scripts/native-entry")
	if !bytes.Contains(entry, []byte("/.local/bin/grok-peer")) || !bytes.Contains(entry, []byte("exec \"$peer_binary\" mcp")) {
		t.Error("Grok manifest entry does not resolve to the installed grok-peer binary")
	}
	var openCode struct {
		Bin map[string]string `json:"bin"`
	}
	if err := json.Unmarshal(read(t, "opencode/package.json"), &openCode); err != nil {
		t.Fatal(err)
	}
	if target := openCode.Bin["sessionbus-opencode-install"]; target != "bin.mjs" || !regular(t, filepath.Join("opencode", target)) {
		t.Errorf("OpenCode bin target is unresolved: %q", target)
	}
	if _, err := os.Stat(".claude-plugin"); !os.IsNotExist(err) {
		t.Fatal("obsolete repository marketplace must not provide an alternate Claude install route")
	}
	var plugin struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(read(t, "claude/.claude-plugin/plugin.json"), &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin.Name != "sessionbus" || plugin.Version != "0.5.0" || !strings.Contains(plugin.Description, "Claude lanes") {
		t.Errorf("Claude native plugin metadata is inconsistent with both modes: %#v", plugin)
	}
	pack := read(t, "scripts/package-claude")
	if !bytes.Contains(pack, []byte("cp -R claude/.claude-plugin")) {
		t.Error("Claude archive must include the product-local native plugin metadata")
	}
}

func TestReadmeIsTheSourceInstallAuthority(t *testing.T) {
	readme := read(t, "README.md")
	for _, exact := range []string{
		`git clone https://github.com/antst/sessionbus.git && cd sessionbus && GOBIN="$HOME/.local/bin" go install ./bus/cmd/...`,
		`git clone https://github.com/antst/sessionbus-peers.git && cd sessionbus-peers && go test -race ./... && GOBIN="$HOME/.local/bin" go install ./cmd/codex-peer ./cmd/grok-peer ./cmd/qwen-peer ./cmd/opencode-peer`,
		"`go install <pkg>@version` is not available",
		"this README is the installation authority until then",
	} {
		if !bytes.Contains(readme, []byte(exact)) {
			t.Errorf("root README lacks required install statement %q", exact)
		}
	}
	for _, binary := range []string{"claude-peer", "codex-peer", "grok-peer", "qwen-peer", "opencode-peer"} {
		if !bytes.Contains(readme, []byte(binary)) {
			t.Errorf("root README omits installed binary %s", binary)
		}
	}
}

func manifestCommand(t *testing.T, path string) string {
	t.Helper()
	var manifest struct {
		Servers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(read(t, path), &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest.Servers["sessionbus"].Command
}

func regular(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func checkGoImports(t *testing.T, root string, check func(string, string)) {
	t.Helper()
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if ignoredDirectory(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, specification := range file.Imports {
			imported, err := strconv.Unquote(specification.Path.Value)
			if err != nil {
				return err
			}
			check(path, imported)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func ignoredDirectory(path string) bool {
	base := filepath.Base(path)
	return base == ".git" || base == "node_modules" || base == "dist" || base == "bin"
}

func removeHistoricalExceptions(body []byte) ([]byte, error) {
	evidence := regexp.MustCompile(`/home/antst/agentbus-evidence/[^\x60\s]+`)
	citation := regexp.MustCompile("`[0-9a-f]{7,40}:[^`]+`")
	cleaned := make([]byte, 0, len(body))
	fenced := false
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("```")) {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		line = evidence.ReplaceAll(line, nil)
		line = citation.ReplaceAll(line, nil)
		cleaned = append(cleaned, line...)
		cleaned = append(cleaned, '\n')
	}
	if fenced {
		return nil, os.ErrInvalid
	}
	return cleaned, nil
}

func containsFormerBrand(body []byte) bool {
	lower := bytes.ToLower(body)
	for _, former := range [][]byte{
		[]byte("agent" + "bus"), []byte("agent" + "_sessions"),
		[]byte("agent" + "-sessions"),
	} {
		if bytes.Contains(lower, former) {
			return true
		}
	}
	words := bytes.Join(bytes.Fields(body), []byte{' '})
	return bytes.Contains(words, []byte("Agent "+"Sessions"))
}

func directoryNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func firstOrSecondLine(body []byte) string {
	lines := bytes.SplitN(body, []byte{'\n'}, 3)
	if len(lines) > 0 && bytes.HasPrefix(lines[0], []byte("#!")) && len(lines) > 1 {
		return string(lines[1])
	}
	if len(lines) > 0 {
		return string(lines[0])
	}
	return ""
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestClaudeInteractivePackageBoundary(t *testing.T) {
	for _, name := range []string{"launch.go", "owner.go", "mcp.go", "delivery.go", "tools.go"} {
		if !regular(t, filepath.Join("wrappers/claude/interactive", name)) {
			t.Errorf("missing Go module %s", name)
		}
	}
	if manifestCommand(t, "claude/.mcp.json") != "${CLAUDE_PLUGIN_ROOT}/bin/sessionbus-mcp" {
		t.Fatal("private MCP alias changed")
	}
	if _, err := os.Stat("claude/package.json"); !os.IsNotExist(err) {
		t.Fatal("active plugin must not require npm")
	}
	for _, name := range []string{"main.mjs", "mcp.mjs", "owner.mjs", "delivery.mjs", "tools.mjs"} {
		if !regular(t, filepath.Join("docs/designs/claude-0.5.0/node-reference", name)) {
			t.Errorf("reviewed Node reference missing: %s", name)
		}
	}
	if !bytes.Contains(read(t, "wrappers/claude/claude.go"), []byte("func (p *Wrapper) Open")) {
		t.Fatal("held lane source removed")
	}
	if !bytes.Contains(read(t, "cmd/claude-peer/main.go"), []byte("interactive.PrivateAlias")) {
		t.Fatal("private alias dispatch missing")
	}
}
