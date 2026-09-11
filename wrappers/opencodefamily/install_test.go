// SPDX-License-Identifier: MIT

package opencodefamily

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func installerFixture(t *testing.T) InstallOptions {
	t.Helper()
	root := t.TempDir()
	plugin := filepath.Join(root, "plugin")
	if err := os.MkdirAll(plugin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "package.json"), []byte(`{"name":"@sessionbus/opencode"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "config")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return InstallOptions{Directory: dir, Specifier: (&url.URL{Scheme: "file", Path: plugin}).String()}
}

func putInstallConfig(t *testing.T, options InstallOptions, name, body string) string {
	t.Helper()
	f := filepath.Join(options.Directory, name)
	if err := os.WriteFile(f, []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	return f
}

func readInstalledConfig(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func installedPluginEntries(t *testing.T, file string, nested bool) []string {
	t.Helper()
	doc, err := readConfigDocument(file)
	if err != nil {
		t.Fatal(err)
	}
	node := doc.tree
	if nested {
		node, err = node.property("tui")
		if err != nil || node == nil {
			t.Fatal("missing nested native TUI configuration", err)
		}
	}
	node, err = node.property("plugin")
	if err != nil || node == nil {
		t.Fatal("missing plugin array", err)
	}
	result := []string{}
	for _, child := range node.items {
		spec, err := configEntrySpecifier(child)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, spec)
	}
	return result
}

func TestConfigurePluginPreservesCommentsTuplesAndNativeLayers(t *testing.T) {
	o := installerFixture(t)
	tuple := `["other-plugin", { "native": true, "large":9007199254740993 }]`
	file := putInstallConfig(t, o, "opencode.jsonc", "{// keep root\n\"plugin\":["+tuple+", // keep tuple\n\"@sessionbus/opencode@old\",],\"unknown\":1e+30,}\n")
	alt := putInstallConfig(t, o, "config.json", `{"model":"native/model","plugin":[["@sessionbus/opencode@older",{"x":1}],"last-plugin"]}`)
	tui := putInstallConfig(t, o, "tui.json", `{"theme":"native","tui":{"plugin":["tui-other","@sessionbus/opencode@old"]}}`)
	if changed, err := ConfigureOpenCodePlugin(o); err != nil || !changed {
		t.Fatal(changed, err)
	}
	body := readInstalledConfig(t, file)
	for _, retained := range []string{"// keep root", "// keep tuple", tuple, `"unknown":1e+30`} {
		if !strings.Contains(body, retained) {
			t.Fatalf("lost literal %s: %s", retained, body)
		}
	}
	for file, want := range map[string][]string{file: {"other-plugin", o.Specifier}, alt: {"last-plugin"}} {
		if got := installedPluginEntries(t, file, false); !reflect.DeepEqual(got, want) {
			t.Fatal(file, got, want)
		}
	}
	if got := installedPluginEntries(t, tui, true); !reflect.DeepEqual(got, []string{"tui-other", o.Specifier}) {
		t.Fatal(got)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatal("existing mode changed", info, err)
	}
	before := map[string]string{file: body, alt: readInstalledConfig(t, alt), tui: readInstalledConfig(t, tui)}
	if changed, err := ConfigureOpenCodePlugin(o); err != nil || changed {
		t.Fatal("second installation must be byte-identical no-op", changed, err)
	}
	for file, body := range before {
		if readInstalledConfig(t, file) != body {
			t.Fatal("repeat changed bytes", file)
		}
	}
	o.Remove = true
	if changed, err := ConfigureOpenCodePlugin(o); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := ConfigureOpenCodePlugin(o); err != nil || changed {
		t.Fatal(changed, err)
	}
	if got := installedPluginEntries(t, file, false); !reflect.DeepEqual(got, []string{"other-plugin"}) {
		t.Fatal(got)
	}
}

func TestConfigurePluginSpecifierFormsAndExactOwnership(t *testing.T) {
	for _, spec := range []string{"@sessionbus/opencode@0.1.0-pre.1", "file:/tmp/sessionbus-opencode-old.tgz", "https://packages.example/sessionbus-opencode-old.tgz?download=1", "http://127.0.0.1/sessionbus-opencode-old.tgz#sha256=old", ""} {
		t.Run(spec, func(t *testing.T) {
			o := installerFixture(t)
			if spec != "" {
				o.Specifier = spec
			}
			file := putInstallConfig(t, o, "opencode.jsonc", `{"plugin":["not@sessionbus/opencode",["https://example.invalid/unrelated.tgz?note=@sessionbus/opencode",{}],"@sessionbus/opencode@old","file:/tmp/sessionbus-opencode-old.tgz","https://packages.example/sessionbus-opencode-old.tgz"]}`)
			if _, err := ConfigureOpenCodePlugin(o); err != nil {
				t.Fatal(err)
			}
			want := []string{"not@sessionbus/opencode", "https://example.invalid/unrelated.tgz?note=@sessionbus/opencode", o.Specifier}
			if got := installedPluginEntries(t, file, false); !reflect.DeepEqual(got, want) {
				t.Fatal(got, want)
			}
			if changed, err := ConfigureOpenCodePlugin(o); err != nil || changed {
				t.Fatal(changed, err)
			}
			o.Remove = true
			if _, err := ConfigureOpenCodePlugin(o); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConfigurePluginRejectsBeforeAnyMutation(t *testing.T) {
	o := installerFixture(t)
	first := putInstallConfig(t, o, "opencode.jsonc", "{// retained\n\"plugin\":[\"other\"]}\n")
	bad := putInstallConfig(t, o, "tui.json", `{broken`)
	before := readInstalledConfig(t, first)
	if _, err := ConfigureOpenCodePlugin(o); err == nil {
		t.Fatal("accepted invalid later native config")
	}
	if readInstalledConfig(t, first) != before || readInstalledConfig(t, bad) != `{broken` {
		t.Fatal("changed files before all documents validated")
	}
	for _, spec := range []string{"https://example.invalid/unrelated.tgz?note=@sessionbus/opencode", "ftp://example/sessionbus-opencode-old.tgz", "sessionbus-opencode-old.tgz", "@sessionbus/opencode@", "@sessionbus/opencode@bad version"} {
		o.Specifier = spec
		if _, err := ConfigureOpenCodePlugin(o); err == nil {
			t.Fatal("accepted unrelated or unsupported specifier", spec)
		}
	}
}

func TestConfigurePluginPartialCommitRestoresOriginalBytes(t *testing.T) {
	o := installerFixture(t)
	files := map[string]string{}
	for _, name := range []string{"opencode.jsonc", "opencode.json", "tui.json"} {
		file := putInstallConfig(t, o, name, "{// original\n\"plugin\":[\"@sessionbus/opencode@old\",\"other\"]}\n")
		files[file] = readInstalledConfig(t, file)
	}
	errDisk := errors.New("disk disappeared after actual first rename")
	_, err := configurePlugin(o, func(changes []configChange) error {
		return commitConfigChangesWithHook(changes, func(n int) error {
			if n == 1 {
				return errDisk
			}
			return nil
		})
	})
	if !errors.Is(err, errDisk) {
		t.Fatal(err)
	}
	for file, original := range files {
		if readInstalledConfig(t, file) != original {
			t.Fatal("rollback lost original bytes", file)
		}
	}
	entries, err := os.ReadDir(o.Directory)
	if err != nil || len(entries) != len(files) {
		t.Fatal("transaction residue", entries, err)
	}
}

func TestConfigurePluginBoundsAndRegularOpenedDescriptor(t *testing.T) {
	for _, content := range []string{strings.Repeat(" ", maxInstallConfig+1), `{"x":` + strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130) + "}", `{"plugin":[],"plugin":[]}`, `{"plugin":[42]}`, `{"x":01}`, `{"x":"unterminated}`} {
		t.Run("invalid", func(t *testing.T) {
			o := installerFixture(t)
			file := putInstallConfig(t, o, "opencode.jsonc", content)
			if _, err := ConfigureOpenCodePlugin(o); err == nil {
				t.Fatal("accepted malformed/unbounded input")
			}
			if readInstalledConfig(t, file) != content {
				t.Fatal("invalid file changed")
			}
		})
	}
	o := installerFixture(t)
	if err := unix.Mkfifo(filepath.Join(o.Directory, "opencode.jsonc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigureOpenCodePlugin(o); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatal("FIFO was not refused without opening a blocking reader", err)
	}
}

func TestConfigurePluginPreservesConfigSymlinkAndRejectsConcurrentChange(t *testing.T) {
	o := installerFixture(t)
	target := putInstallConfig(t, o, "native-settings", `{"plugin":["other"]}`)
	file := filepath.Join(o.Directory, "opencode.jsonc")
	if err := os.Symlink(target, file); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigureOpenCodePlugin(o); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(file); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("lost native configuration symlink", err)
	}
	o.Specifier = "@sessionbus/opencode@next"
	_, err := configurePlugin(o, func(changes []configChange) error {
		if err := os.WriteFile(target, []byte(`{"plugin":["user-edit"]}`), 0o600); err != nil {
			return err
		}
		return commitConfigChanges(changes)
	})
	if err == nil || readInstalledConfig(t, target) != `{"plugin":["user-edit"]}` {
		t.Fatal("overwrote concurrent edit", err)
	}
}

func TestConfigurePluginRejectsCrossGroupHardLinks(t *testing.T) {
	for _, name := range []string{"tui.jsonc", "tui.json"} {
		t.Run(name, func(t *testing.T) {
			options := installerFixture(t)
			first := putInstallConfig(t, options, "config.json", `{"plugin":["other","@sessionbus/opencode@old"]}`)
			second := filepath.Join(options.Directory, name)
			if err := os.Link(first, second); err != nil {
				t.Fatal(err)
			}
			before := readInstalledConfig(t, first)
			changed, err := ConfigureOpenCodePlugin(options)
			if err == nil || changed {
				t.Fatalf("hard-link accepted: changed=%v error=%v", changed, err)
			}
			if readInstalledConfig(t, first) != before || readInstalledConfig(t, second) != before {
				t.Fatal("rejection changed bytes")
			}
			a, _ := os.Stat(first)
			b, _ := os.Stat(second)
			if !os.SameFile(a, b) {
				t.Fatal("rejection broke hard link")
			}
		})
	}
}
