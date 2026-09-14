// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func skillFixture(t *testing.T, kind nativeKind) (InstallOptions, string) {
	t.Helper()
	o := installerFixture(t)
	if kind == kiloNative {
		o = kiloInstallerFixture(t)
	}
	u, err := url.Parse(o.Specifier)
	if err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(u.Path, "skills", "sessionbus", "SKILL.md")
	if err = os.MkdirAll(filepath.Dir(skill), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(skill, []byte("---\nname: sessionbus\ndescription: fixture\n---\nFixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(filepath.Join(u.Path, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	return o, physical
}
func installedSkillPaths(t *testing.T, file string) []string {
	t.Helper()
	doc, err := readConfigDocument(file)
	if err != nil {
		t.Fatal(err)
	}
	skills, err := doc.tree.property("skills")
	if err != nil {
		t.Fatal(err)
	}
	if skills == nil {
		return nil
	}
	paths, err := skills.property("paths")
	if err != nil {
		t.Fatal(err)
	}
	if paths == nil {
		return nil
	}
	var result []string
	for _, p := range paths.items {
		result = append(result, p.text)
	}
	return result
}

func TestLocalBundledSkillRegisteredWithExactPluginAlreadyInstalled(t *testing.T) {
	for _, kind := range []nativeKind{openCodeNative, kiloNative} {
		t.Run(kind.name(), func(t *testing.T) {
			o, root := skillFixture(t, kind)
			spec, _ := json.Marshal(o.Specifier)
			file := putInstallConfig(t, o, kind.name()+".jsonc", `{"plugin":[`+string(spec)+`],"unrelated":1e30}`)
			putInstallConfig(t, o, "tui.jsonc", `{"plugin":[`+string(spec)+`]}`)
			if changed, err := configurePluginFor(kind, o, commitConfigChanges); err != nil || !changed {
				t.Fatal(changed, err)
			}
			if got := installedSkillPaths(t, file); !reflect.DeepEqual(got, []string{root}) {
				t.Fatalf("local rendered skill undiscoverable: %v", got)
			}
			if changed, err := configurePluginFor(kind, o, commitConfigChanges); err != nil || changed {
				t.Fatal("repeat", changed, err)
			}
			o.Remove = true
			if _, err := configurePluginFor(kind, o, commitConfigChanges); err != nil {
				t.Fatal(err)
			}
			if got := installedSkillPaths(t, file); len(got) != 0 {
				t.Fatal("owned skill retained", got)
			}
		})
	}
}

func TestLocalSkillUsesLastNativePathsArrayAndPreservesOtherLayers(t *testing.T) {
	for _, kind := range []nativeKind{openCodeNative, kiloNative} {
		t.Run(kind.name(), func(t *testing.T) {
			o, root := skillFixture(t, kind)
			encoded, _ := json.Marshal(root)
			earlier := putInstallConfig(t, o, "config.json", `{"skills":{"paths":["earlier-user",`+string(encoded)+`]},"unknown":1e30}`)
			last := putInstallConfig(t, o, "opencode.jsonc", `{// literal retained
"skills":{"paths":["later-user"],"urls":["https://example.invalid/skills"]},"unrelated":true}`)
			tui := putInstallConfig(t, o, "tui.jsonc", `{"skills":{"paths":["native-tui-unused"]}}`)
			if _, err := configurePluginFor(kind, o, commitConfigChanges); err != nil {
				t.Fatal(err)
			}
			if got := installedSkillPaths(t, earlier); !reflect.DeepEqual(got, []string{"earlier-user"}) {
				t.Fatal(got)
			}
			if got := installedSkillPaths(t, last); !reflect.DeepEqual(got, []string{"later-user", root}) {
				t.Fatal("later native array hides own path", got)
			}
			if got := installedSkillPaths(t, tui); !reflect.DeepEqual(got, []string{"native-tui-unused"}) {
				t.Fatal(got)
			}
			body := readInstalledConfig(t, last)
			for _, literal := range []string{"// literal retained", `"urls":["https://example.invalid/skills"]`, `"unrelated":true`} {
				if !strings.Contains(body, literal) {
					t.Fatal(body)
				}
			}
			// Native mergeDeep replaces arrays: the last paths property is authoritative.
			var effective []string
			order := []string{"config.json", "opencode.json", "opencode.jsonc"}
			if kind == kiloNative {
				order = []string{"config.json", "kilo.json", "kilo.jsonc", "opencode.json", "opencode.jsonc"}
			}
			for _, name := range order {
				file := filepath.Join(o.Directory, name)
				if _, err := os.Stat(file); err != nil {
					continue
				}
				doc, err := readConfigDocument(file)
				if err != nil {
					t.Fatal(err)
				}
				skills, _ := doc.tree.property("skills")
				if skills != nil {
					paths, _ := skills.property("paths")
					if paths != nil {
						effective = installedSkillPaths(t, file)
					}
				}
			}
			if !reflect.DeepEqual(effective, []string{"later-user", root}) {
				t.Fatal(effective)
			}
		})
	}
}

func TestSkillEmptyLaterArrayOverridesEarlierAndUnknownRootsRemain(t *testing.T) {
	o, root := skillFixture(t, kiloNative)
	putInstallConfig(t, o, "kilo.jsonc", `{"skills":{"paths":["earlier-user"]}}`)
	last := putInstallConfig(t, o, "opencode.jsonc", `{"skills":{"paths":[]}}`)
	if _, err := ConfigureKiloPlugin(o); err != nil {
		t.Fatal(err)
	}
	if got := installedSkillPaths(t, last); !reflect.DeepEqual(got, []string{root}) {
		t.Fatal(got)
	}
	foreign, _ := json.Marshal(filepath.Join(t.TempDir(), "sessionbus", "kilo", "plugin", "skills"))
	putInstallConfig(t, o, "opencode.jsonc", `{"skills":{"paths":[`+string(foreign)+`]}}`)
	if _, err := ConfigureKiloPlugin(o); err != nil {
		t.Fatal(err)
	}
	if len(installedSkillPaths(t, last)) != 2 {
		t.Fatal("unowned suffix path removed")
	}
}

func TestSkillPreflightAndRollbackPreserveEveryDocument(t *testing.T) {
	for _, bad := range []string{`{"skills":[]}`, `{"skills":{"paths":"wrong"}}`, `{"skills":{"paths":[null]}}`, `{"skills":{"paths":[],"paths":[]}}`} {
		t.Run(bad, func(t *testing.T) {
			o, _ := skillFixture(t, kiloNative)
			file := putInstallConfig(t, o, "kilo.jsonc", `{"plugin":["@sessionbus/kilo@old"]}`)
			putInstallConfig(t, o, "opencode.jsonc", bad)
			before := readInstalledConfig(t, file)
			called := false
			if _, err := configurePluginFor(kiloNative, o, func([]configChange) error { called = true; return nil }); err == nil || called {
				t.Fatal("invalid config reached mutation", err, called)
			}
			if readInstalledConfig(t, file) != before {
				t.Fatal("preflight mutation")
			}
		})
	}
	o, _ := skillFixture(t, kiloNative)
	first := putInstallConfig(t, o, "kilo.jsonc", `{"plugin":["@sessionbus/kilo@old"],"skills":{"paths":["user"]}}`)
	second := putInstallConfig(t, o, "tui.jsonc", `{"plugin":["@sessionbus/kilo@old"]}`)
	before1, before2 := readInstalledConfig(t, first), readInstalledConfig(t, second)
	sentinel := errors.New("after actual first rename")
	if _, err := configurePluginFor(kiloNative, o, func(changes []configChange) error {
		return commitConfigChangesWithHook(changes, func(n int) error {
			if n == 1 {
				return sentinel
			}
			return nil
		})
	}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if readInstalledConfig(t, first) != before1 || readInstalledConfig(t, second) != before2 {
		t.Fatal("rollback lost plugin/skill snapshots")
	}
}

func TestLocalSkillRemovalTransitionAndMissingPayload(t *testing.T) {
	for _, kind := range []nativeKind{openCodeNative, kiloNative} {
		t.Run(kind.name(), func(t *testing.T) {
			o, root := skillFixture(t, kind)
			if _, err := configurePluginFor(kind, o, commitConfigChanges); err != nil {
				t.Fatal(err)
			}
			o.Specifier = kind.installPackage() + "@next"
			if _, err := configurePluginFor(kind, o, commitConfigChanges); err != nil {
				t.Fatal(err)
			}
			if got := installedSkillPaths(t, filepath.Join(o.Directory, kind.name()+".jsonc")); len(got) != 0 {
				t.Fatal("remote transition retains local skill", got)
			}
			o.Specifier = (&url.URL{Scheme: "file", Path: filepath.Dir(root)}).String()
			if err := os.Remove(filepath.Join(root, "sessionbus", "SKILL.md")); err != nil {
				t.Fatal(err)
			}
			called := false
			if _, err := configurePluginFor(kind, o, func([]configChange) error { called = true; return nil }); err == nil || called {
				t.Fatal("missing payload accepted", err, called)
			}
		})
	}
}

func TestDesiredSkillTreeRejectsExtraOrRedirectedEntriesBeforeConfig(t *testing.T) {
	for _, variant := range []string{"extra-skill", "extra-file", "skills-link", "sessionbus-link", "file-link", "fifo"} {
		t.Run(variant, func(t *testing.T) {
			o, root := skillFixture(t, kiloNative)
			file := putInstallConfig(t, o, "kilo.jsonc", `{"plugin":["@sessionbus/kilo@old"]}`)
			before := readInstalledConfig(t, file)
			switch variant {
			case "extra-skill":
				if err := os.Mkdir(filepath.Join(root, "foreign"), 0700); err != nil {
					t.Fatal(err)
				}
			case "extra-file":
				if err := os.WriteFile(filepath.Join(root, "sessionbus", "other.md"), []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
			case "skills-link", "sessionbus-link":
				path := root
				if variant == "sessionbus-link" {
					path = filepath.Join(root, "sessionbus")
				}
				moved := filepath.Join(t.TempDir(), "redirect")
				if err := os.Rename(path, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, path); err != nil {
					t.Fatal(err)
				}
			case "file-link", "fifo":
				path := filepath.Join(root, "sessionbus", "SKILL.md")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if variant == "file-link" {
					other := filepath.Join(t.TempDir(), "SKILL.md")
					if err := os.WriteFile(other, []byte("foreign"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(other, path); err != nil {
						t.Fatal(err)
					}
				} else if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			if _, err := configurePluginFor(kiloNative, o, func([]configChange) error { called = true; return nil }); err == nil || called {
				t.Fatal("invalid desired skill tree admitted", err, called)
			}
			if readInstalledConfig(t, file) != before {
				t.Fatal("invalid tree mutated config")
			}
		})
	}
}

func TestOwnedSkillPathsReconcileAllGlobalLayersWithoutOtherProduct(t *testing.T) {
	for _, kind := range []nativeKind{openCodeNative, kiloNative} {
		t.Run(kind.name(), func(t *testing.T) {
			o, root := skillFixture(t, kind)
			_, oldRoot := skillFixture(t, kind)
			otherKind := kiloNative
			if kind == kiloNative {
				otherKind = openCodeNative
			}
			_, otherRoot := skillFixture(t, otherKind)
			paths, _ := json.Marshal([]string{oldRoot, root, root, otherRoot})
			order := []string{"config.json", "opencode.json", "opencode.jsonc"}
			if kind == kiloNative {
				order = []string{"config.json", "kilo.json", "kilo.jsonc", "opencode.json", "opencode.jsonc"}
			}
			for _, name := range order {
				putInstallConfig(t, o, name, `{"skills":{"paths":`+string(paths)+`}}`)
			}
			if _, err := configurePluginFor(kind, o, commitConfigChanges); err != nil {
				t.Fatal(err)
			}
			for i, name := range order {
				want := []string{otherRoot}
				if i == len(order)-1 {
					want = append(want, root)
				}
				if got := installedSkillPaths(t, filepath.Join(o.Directory, name)); !reflect.DeepEqual(got, want) {
					t.Fatal(name, got, want)
				}
			}
			o.Specifier = kind.installPackage() + "@next"
			if _, err := configurePluginFor(kind, o, commitConfigChanges); err != nil {
				t.Fatal(err)
			}
			for _, name := range order {
				if got := installedSkillPaths(t, filepath.Join(o.Directory, name)); !reflect.DeepEqual(got, []string{otherRoot}) {
					t.Fatal(name, got)
				}
			}
		})
	}
}
