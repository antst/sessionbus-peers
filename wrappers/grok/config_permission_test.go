// SPDX-License-Identifier: MIT

package grok

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestEnsureSessionbusPermissionPreservesNativeRepresentation(t *testing.T) {
	for _, test := range []struct {
		name, before, after string
	}{
		{
			name:   "missing permission",
			before: "model = \"grok\"\n",
			after:  "model = \"grok\"\n\n[permission]\nallow = [\"MCPTool(sessionbus__sessionbus)\"]\n",
		},
		{
			name:   "empty file",
			before: "",
			after:  "[permission]\nallow = [\"MCPTool(sessionbus__sessionbus)\"]\n",
		},
		{
			name:   "compact allow",
			before: "# retained\n[permission]\nallow = [\"Read(*)\"] # retained\nask = [\"Bash(*)\"]\n",
			after:  "# retained\n[permission]\nallow = [\"Read(*)\", \"MCPTool(sessionbus__sessionbus)\"] # retained\nask = [\"Bash(*)\"]\n",
		},
		{
			name:   "compact multiline trailing comma",
			before: "[permission]\nallow = [\n  \"Read(*)\", # retained\n]\ndeny = [\"MCPTool(blocked__*)\"]\n",
			after:  "[permission]\nallow = [\n  \"Read(*)\", # retained\n \"MCPTool(sessionbus__sessionbus)\"]\ndeny = [\"MCPTool(blocked__*)\"]\n",
		},
		{
			name:   "compact deny without allow",
			before: "[permission] # retained\ndeny = [\"MCPTool(blocked__*)\"]\n",
			after:  "[permission] # retained\nallow = [\"MCPTool(sessionbus__sessionbus)\"]\ndeny = [\"MCPTool(blocked__*)\"]\n",
		},
		{
			name: "unrelated multiline string",
			before: "note = \"\"\"\n[permission]\nallow = []\n\"\"\"\n" +
				"[permission]\ndeny = [\"MCPTool(blocked__*)\"]\n",
			after: "note = \"\"\"\n[permission]\nallow = []\n\"\"\"\n" +
				"[permission]\nallow = [\"MCPTool(sessionbus__sessionbus)\"]\ndeny = [\"MCPTool(blocked__*)\"]\n",
		},
		{
			name:   "UTF-8 BOM compact",
			before: "\ufeff[permission]\ndeny = [\"MCPTool(blocked__*)\"]\n",
			after:  "\ufeff[permission]\nallow = [\"MCPTool(sessionbus__sessionbus)\"]\ndeny = [\"MCPTool(blocked__*)\"]\n",
		},
		{
			name:   "dotted compact",
			before: "permission.deny = [\"MCPTool(blocked__*)\"]\nmodel = \"grok\"\n",
			after:  "permission.allow = [\"MCPTool(sessionbus__sessionbus)\"]\npermission.deny = [\"MCPTool(blocked__*)\"]\nmodel = \"grok\"\n",
		},
		{
			name:   "structured",
			before: "[[permission.rules]]\naction = \"deny\"\ntool = \"mcp\"\npattern = \"blocked__*\"\n",
			after: "[[permission.rules]]\naction = \"deny\"\ntool = \"mcp\"\npattern = \"blocked__*\"\n\n" +
				"[[permission.rules]]\naction = \"allow\"\ntool = \"mcp\"\npattern = \"sessionbus__sessionbus\"\n",
		},
		{
			name:   "structured inline array",
			before: "[permission]\nrules = [{ action = 'deny', tool = 'mcp', pattern = 'blocked__*' }] # retained\n",
			after:  "[permission]\nrules = [{ action = 'deny', tool = 'mcp', pattern = 'blocked__*' }, { action = \"allow\", tool = \"mcp\", pattern = \"sessionbus__sessionbus\" }] # retained\n",
		},
		{
			name:   "structured empty inline array",
			before: "[permission]\nrules = []\n",
			after:  "[permission]\nrules = [{ action = \"allow\", tool = \"mcp\", pattern = \"sessionbus__sessionbus\" }]\n",
		},
		{
			name:   "empty permission table is structured",
			before: "[permission]\nprompt_policy = \"ask\"\n",
			after: "[permission]\nprompt_policy = \"ask\"\n\n" +
				"[[permission.rules]]\naction = \"allow\"\ntool = \"mcp\"\npattern = \"sessionbus__sessionbus\"\n",
		},
		{
			name: "mixed uses compact",
			before: "[permission]\ndeny = [\"MCPTool(blocked__*)\"]\n" +
				"[[permission.rules]]\naction = \"deny\"\ntool = \"mcp\"\npattern = \"structured__*\"\n",
			after: "[permission]\nallow = [\"MCPTool(sessionbus__sessionbus)\"]\ndeny = [\"MCPTool(blocked__*)\"]\n" +
				"[[permission.rules]]\naction = \"deny\"\ntool = \"mcp\"\npattern = \"structured__*\"\n",
		},
		{
			name:   "CRLF",
			before: "model = \"grok\"\r\n",
			after:  "model = \"grok\"\r\n\r\n[permission]\r\nallow = [\"MCPTool(sessionbus__sessionbus)\"]\r\n",
		},
		{
			name:   "no trailing newline",
			before: "model = \"grok\"",
			after:  "model = \"grok\"\n\n[permission]\nallow = [\"MCPTool(sessionbus__sessionbus)\"]\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, grokConfigFile)
			if err := os.WriteFile(path, []byte(test.before), 0640); err != nil {
				t.Fatal(err)
			}
			if err := ensureSessionbusPermissionAt(home); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.after {
				t.Fatalf("config:\n%s\nwant:\n%s", got, test.after)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0640 {
				t.Fatalf("mode = %o", info.Mode().Perm())
			}
		})
	}
}

func TestEnsureSessionbusPermissionIsIdempotent(t *testing.T) {
	for _, content := range []string{
		"[permission]\nallow = [\"  MCPTool(sessionbus__sessionbus)  \"]\n",
		"[[permission.rules]]\naction = \"allow\"\ntool = \"mcp\"\npattern = \"sessionbus__sessionbus\"\n",
		"[[permission.rules]]\naction = \"allow\"\ntool = \"mcp\"\npattern = \"sessionbus__sessionbus\"\npattern_mode = \"glob\"\n",
	} {
		home := t.TempDir()
		path := filepath.Join(home, grokConfigFile)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := ensureSessionbusPermissionAt(home); err != nil {
			t.Fatal(err)
		}
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content || !os.SameFile(before, after) {
			t.Fatalf("no-op rewrote config: same_inode=%t\n%s", os.SameFile(before, after), got)
		}
	}
}

func TestEnsureSessionbusPermissionRejectsUnsafeInputWithoutRewrite(t *testing.T) {
	for _, content := range []string{
		"not toml [[[",
		"permission = \"not a table\"\n",
		"[permission]\ndeny = [\"Read(*)\"]\nallow = \"not an array\"\n",
		"[permission]\nrules = \"not tables\"\n",
		"permission = { deny = [\"Read(*)\"] }\n",
	} {
		home := t.TempDir()
		path := filepath.Join(home, grokConfigFile)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		before, _ := os.Stat(path)
		err := ensureSessionbusPermissionAt(home)
		if err == nil || !strings.Contains(err.Error(), "refusing to update Grok config") {
			t.Fatalf("input accepted: %q: %v", content, err)
		}
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		after, _ := os.Stat(path)
		if string(got) != content || !os.SameFile(before, after) {
			t.Fatalf("rejected input changed: %q", got)
		}
	}
}

func TestEnsureSessionbusPermissionSanitizesTOMLError(t *testing.T) {
	home := t.TempDir()
	secret := "EXAMPLE_PRIVATE_VALUE"
	if err := os.WriteFile(filepath.Join(home, grokConfigFile), []byte("token = "+secret+" @\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := ensureSessionbusPermissionAt(home)
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "invalid TOML at line") {
		t.Fatalf("unsanitized error: %v", err)
	}
}

func TestEnsureSessionbusPermissionConcurrentStartsAddOneRule(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, grokConfigFile)
	if err := os.WriteFile(path, []byte("[permission]\ndeny = [\"MCPTool(blocked__*)\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsSeen := make(chan error, 16)
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			errorsSeen <- ensureSessionbusPermissionAt(home)
		}()
	}
	close(start)
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(got, []byte(sessionbusNativeRule)) != 1 {
		t.Fatalf("rule count = %d\n%s", bytes.Count(got, []byte(sessionbusNativeRule)), got)
	}
}

func TestEnsureSessionbusPermissionFollowsConfigSymlink(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	targetDir := filepath.Join(root, "target")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "real.toml")
	if err := os.WriteFile(target, []byte("[permission]\ndeny = [\"Read(*)\"]\n"), 0640); err != nil {
		t.Fatal(err)
	}
	slot := filepath.Join(home, grokConfigFile)
	if err := os.Symlink(target, slot); err != nil {
		t.Fatal(err)
	}
	if err := ensureSessionbusPermissionAt(home); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(slot)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config symlink replaced: %v %v", info, err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(sessionbusNativeRule)) {
		t.Fatalf("target not updated: %s", got)
	}
	updated, _ := os.Stat(target)
	if updated.Mode().Perm() != 0640 {
		t.Fatalf("target mode = %o", updated.Mode().Perm())
	}
}

func TestEnsureSessionbusPermissionCreatesNativeHomeAndPrivateConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "missing", "grok")
	if err := ensureSessionbusPermissionAt(home); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(home, grokConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("new config mode = %o", info.Mode().Perm())
	}
}

func TestAtomicConfigWriteRefusesInPlaceConcurrentChange(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, grokConfigFile)
	original := []byte("model = \"before\"\n")
	changed := []byte("model = \"changed\"\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	dest, err := bindGrokConfigDestination(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	err = writeGrokConfigAtomically(path, dest, original, []byte("replacement"))
	if err == nil || !strings.Contains(err.Error(), "contents changed") {
		t.Fatalf("concurrent write accepted: %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(got, changed) {
		t.Fatalf("concurrent content replaced: %q %v", got, readErr)
	}
}

func TestResolvedGrokHomeMatchesNativePrecedence(t *testing.T) {
	home, cwd := filepath.Join(t.TempDir(), "home"), filepath.Join(t.TempDir(), "cwd")
	if got, err := resolvedGrokHome([]string{"HOME=" + home, "GROK_HOME=ignored", "GROK_HOME=relative-grok-home"}, cwd); err != nil || got != filepath.Join(cwd, "relative-grok-home") {
		t.Fatalf("GROK_HOME resolution = %q, %v", got, err)
	}
	if got, err := resolvedGrokHome([]string{"HOME=" + home, "GROK_HOME="}, cwd); err != nil || got != filepath.Join(home, ".grok") {
		t.Fatalf("HOME resolution = %q, %v", got, err)
	}
	if got, err := resolvedGrokHome([]string{"HOME=relative-home"}, cwd); err != nil || got != filepath.Join(cwd, "relative-home", ".grok") {
		t.Fatalf("relative HOME resolution = %q, %v", got, err)
	}
}
