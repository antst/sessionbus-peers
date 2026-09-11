// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// Exact retained installed bytes, never executed or included in a product build.
//
//go:embed testdata/kilo-legacy-agent-sessions.txt
var kiloLegacyFixture []byte

func kiloInstallerFixture(t *testing.T) InstallOptions {
	t.Helper()
	o := installerFixture(t)
	u, err := url.Parse(o.Specifier)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(u.Path, "package.json"), []byte(`{"name":"@sessionbus/kilo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return o
}

func putKiloLegacy(t *testing.T, o InstallOptions) string {
	t.Helper()
	if fmt.Sprintf("%x", sha256.Sum256(kiloLegacyFixture)) != kiloLegacyEntrySHA256 {
		t.Fatal("legacy fixture provenance changed")
	}
	file := filepath.Join(o.Directory, "plugins", "agent-sessions.js")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, kiloLegacyFixture, 0o640); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestKiloInstallerReconcilesSevenDocumentsAndExactLegacy(t *testing.T) {
	o := kiloInstallerFixture(t)
	files := []string{"kilo.jsonc", "kilo.json", "opencode.jsonc", "opencode.json", "config.json", "tui.jsonc", "tui.json"}
	tuple := `["other-plugin", {"literal":9007199254740993}]`
	for _, name := range files {
		body := "{// keep " + name + "\n\"plugin\":[" + tuple + ",\"@sessionbus/kilo@old\",\"@sessionbus/opencode@keep\"],\"unknown\":1e30}"
		if name == "tui.jsonc" {
			body = "{// keep tui.jsonc\n\"plugin\":[\"outer\",\"@sessionbus/kilo@old\"],\"tui\":{\"plugin\":[" + tuple + ",\"@sessionbus/kilo@nested\",\"@sessionbus/opencode@keep\"]},\"unknown\":1e30}"
		}
		putInstallConfig(t, o, name, body)
	}
	legacy := putKiloLegacy(t, o)
	other := putInstallConfig(t, o, "package.json", `{"dependencies":{"@kilocode/plugin":"7.5.6"}}`)
	shared := filepath.Join(o.Directory, "shared")
	if err := os.Mkdir(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	companion := filepath.Join(shared, "live-session.js")
	if err := os.WriteFile(companion, []byte("retained companion"), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, err := ConfigureKiloPlugin(o); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if _, err := os.Lstat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("legacy autoload retained", err)
	}
	for _, name := range files {
		file := filepath.Join(o.Directory, name)
		body := readInstalledConfig(t, file)
		for _, literal := range []string{"// keep " + name, tuple, `"unknown":1e30`} {
			if !strings.Contains(body, literal) {
				t.Fatalf("lost %s in %s", literal, body)
			}
		}
		want := []string{"other-plugin", "@sessionbus/opencode@keep"}
		if name == "kilo.jsonc" || name == "tui.jsonc" {
			want = append(want, o.Specifier)
		}
		if got := installedPluginEntries(t, file, name == "tui.jsonc"); !reflect.DeepEqual(got, want) {
			t.Fatal(name, got, want)
		}
		if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o640 {
			t.Fatal("mode changed", info, err)
		}
	}
	if got := installedPluginEntries(t, filepath.Join(o.Directory, "tui.jsonc"), false); !reflect.DeepEqual(got, []string{"outer"}) {
		t.Fatal(got)
	}
	if readInstalledConfig(t, other) != `{"dependencies":{"@kilocode/plugin":"7.5.6"}}` || readInstalledConfig(t, companion) != "retained companion" {
		t.Fatal("unrelated installation changed")
	}
	if changed, err := ConfigureKiloPlugin(o); err != nil || changed {
		t.Fatal("repeat must be no-op", changed, err)
	}
	o.Remove = true
	if changed, err := ConfigureKiloPlugin(o); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := ConfigureKiloPlugin(o); err != nil || changed {
		t.Fatal(changed, err)
	}
	for _, name := range files {
		got := installedPluginEntries(t, filepath.Join(o.Directory, name), name == "tui.jsonc")
		if !reflect.DeepEqual(got, []string{"other-plugin", "@sessionbus/opencode@keep"}) {
			t.Fatal(name, got)
		}
	}
}

func TestKiloInstallerRefusesUnrecognizedOrRedirectedLegacyBeforeChanges(t *testing.T) {
	for _, mode := range []string{"changed", "symlink", "directory-symlink", "fifo"} {
		t.Run(mode, func(t *testing.T) {
			o := kiloInstallerFixture(t)
			file := putInstallConfig(t, o, "kilo.jsonc", `{"plugin":["other"]}`)
			before := readInstalledConfig(t, file)
			legacy := putKiloLegacy(t, o)
			if err := os.Remove(legacy); err != nil {
				t.Fatal(err)
			}
			external := filepath.Join(t.TempDir(), "entry.js")
			if err := os.WriteFile(external, kiloLegacyFixture, 0o600); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "changed":
				if err := os.WriteFile(legacy, append(append([]byte{}, kiloLegacyFixture...), '\n'), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(external, legacy); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				if err := os.Remove(filepath.Dir(legacy)); err != nil {
					t.Fatal(err)
				}
				target := filepath.Dir(external)
				if err := os.WriteFile(filepath.Join(target, "agent-sessions.js"), kiloLegacyFixture, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Dir(legacy)); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(legacy, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if changed, err := ConfigureKiloPlugin(o); err == nil || changed {
				t.Fatal("accepted unowned legacy", changed, err)
			}
			if readInstalledConfig(t, file) != before {
				t.Fatal("config mutated before ownership validation")
			}
			if b, err := os.ReadFile(external); err != nil || !bytes.Equal(b, kiloLegacyFixture) {
				t.Fatal("external changed", err)
			}
			if _, err := os.Lstat(filepath.Join(o.Directory, "tui.jsonc")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("new config appeared", err)
			}
		})
	}
}

func TestKiloInstallerLegacyRemovalRollsBackWithAllConfigChanges(t *testing.T) {
	for _, created := range []bool{false, true} {
		t.Run(fmt.Sprint(created), func(t *testing.T) {
			o := kiloInstallerFixture(t)
			originals := map[string]string{}
			if !created {
				for _, name := range []string{"kilo.jsonc", "tui.jsonc"} {
					file := putInstallConfig(t, o, name, `{"plugin":["@sessionbus/kilo@old","other"]}`)
					originals[file] = readInstalledConfig(t, file)
				}
			}
			legacy := putKiloLegacy(t, o)
			forced := errors.New("failure after actual legacy removal")
			_, err := configurePluginFor(kiloNative, o, func(changes []configChange) error {
				return commitConfigChangesWithHook(changes, func(n int) error {
					if n == len(changes) {
						if _, err := os.Lstat(legacy); !errors.Is(err, os.ErrNotExist) {
							t.Fatal("removal not reached", err)
						}
						return forced
					}
					return nil
				})
			})
			if !errors.Is(err, forced) {
				t.Fatal(err)
			}
			for file, before := range originals {
				if readInstalledConfig(t, file) != before {
					t.Fatal("rollback lost bytes", file)
				}
			}
			if created {
				for _, name := range []string{"kilo.jsonc", "tui.jsonc"} {
					if _, err := os.Lstat(filepath.Join(o.Directory, name)); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("rollback retained created config", err)
					}
				}
			}
			if b, err := os.ReadFile(legacy); err != nil || !bytes.Equal(b, kiloLegacyFixture) {
				t.Fatal("legacy rollback lost bytes", err)
			}
			if info, err := os.Stat(legacy); err != nil || info.Mode().Perm() != 0o640 {
				t.Fatal("legacy mode lost", info, err)
			}
			if err := filepath.WalkDir(o.Directory, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if strings.HasPrefix(d.Name(), ".sessionbus-") {
					t.Fatalf("transaction residue: %s", path)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestKiloInstallerRechecksLegacyBeforeMutationAndRemoval(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(fmt.Sprint(during), func(t *testing.T) {
			o := kiloInstallerFixture(t)
			file := putInstallConfig(t, o, "kilo.jsonc", `{"plugin":["other"]}`)
			before := readInstalledConfig(t, file)
			legacy := putKiloLegacy(t, o)
			mutate := func() {
				if err := os.WriteFile(legacy, []byte("concurrent edit"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := configurePluginFor(kiloNative, o, func(changes []configChange) error {
				if !during {
					mutate()
				}
				return commitConfigChangesWithHook(changes, func(n int) error {
					if during && n == 1 {
						mutate()
					}
					return nil
				})
			})
			if err == nil {
				t.Fatal("concurrent legacy edit accepted")
			}
			if readInstalledConfig(t, file) != before || readInstalledConfig(t, legacy) != "concurrent edit" {
				t.Fatal("overwrote concurrent edit or failed rollback")
			}
		})
	}
}

func TestKiloInstallerValidatesSeventhDocumentBeforeLegacyRemoval(t *testing.T) {
	o := kiloInstallerFixture(t)
	file := putInstallConfig(t, o, "kilo.jsonc", `{"plugin":["other"]}`)
	legacy := putKiloLegacy(t, o)
	putInstallConfig(t, o, "tui.json", "{broken")
	if changed, err := ConfigureKiloPlugin(o); err == nil || changed {
		t.Fatal(changed, err)
	}
	if readInstalledConfig(t, file) != `{"plugin":["other"]}` {
		t.Fatal("early config changed")
	}
	if b, err := os.ReadFile(legacy); err != nil || !bytes.Equal(b, kiloLegacyFixture) {
		t.Fatal("legacy changed", err)
	}
}

func TestKiloInstallerRemovesLegacyWithoutCreatingConfig(t *testing.T) {
	o := kiloInstallerFixture(t)
	legacy := putKiloLegacy(t, o)
	o.Remove = true
	if changed, err := ConfigureKiloPlugin(o); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if _, err := os.Lstat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("legacy retained", err)
	}
	for _, group := range kiloNative.installConfigNames() {
		for _, name := range group {
			if _, err := os.Lstat(filepath.Join(o.Directory, name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("remove created config", name, err)
			}
		}
	}
	if changed, err := ConfigureKiloPlugin(o); err != nil || changed {
		t.Fatal(changed, err)
	}
}

func TestKiloInstallerConfigAliasesAndOtherProductSpecifiers(t *testing.T) {
	for _, alias := range []string{"hardlink", "symlink"} {
		t.Run(alias, func(t *testing.T) {
			o := kiloInstallerFixture(t)
			file := putInstallConfig(t, o, "kilo.jsonc", `{"plugin":["@sessionbus/kilo@old"]}`)
			second := filepath.Join(o.Directory, "opencode.jsonc")
			var err error
			if alias == "hardlink" {
				err = os.Link(file, second)
			} else {
				err = os.Symlink(file, second)
			}
			if err != nil {
				t.Fatal(err)
			}
			legacy := putKiloLegacy(t, o)
			if changed, err := ConfigureKiloPlugin(o); err == nil || changed {
				t.Fatal("alias accepted", changed, err)
			}
			if readInstalledConfig(t, file) != `{"plugin":["@sessionbus/kilo@old"]}` {
				t.Fatal("alias rejection changed config")
			}
			if b, err := os.ReadFile(legacy); err != nil || !bytes.Equal(b, kiloLegacyFixture) {
				t.Fatal("alias rejection changed legacy", err)
			}
		})
	}
	for _, spec := range []string{"@sessionbus/opencode", "@sessionbus/kilo@", "@sessionbus/kilo@bad version", "https://example/other.tgz?note=@sessionbus/kilo"} {
		o := kiloInstallerFixture(t)
		o.Specifier = spec
		if _, err := ConfigureKiloPlugin(o); err == nil {
			t.Fatal("unowned specifier accepted", spec)
		}
	}
}
