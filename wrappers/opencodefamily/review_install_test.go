// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReviewInstallerRejectsHardLinkedNativeConfigs(t *testing.T) {
	o := installerFixture(t)
	first := putInstallConfig(t, o, "opencode.jsonc", `{"plugin":["other","@sessionbus/opencode@old"]}`)
	second := filepath.Join(o.Directory, "opencode.json")
	if err := os.Link(first, second); err != nil {
		t.Fatal(err)
	}
	before := readInstalledConfig(t, first)
	changed, err := ConfigureOpenCodePlugin(o)
	if err == nil {
		t.Fatalf("hard-linked native configurations accepted: changed=%v, first=%s, second=%s", changed, readInstalledConfig(t, first), readInstalledConfig(t, second))
	}
	if readInstalledConfig(t, first) != before || readInstalledConfig(t, second) != before {
		t.Fatal("alias rejection changed config")
	}
	a, _ := os.Stat(first)
	b, _ := os.Stat(second)
	if !os.SameFile(a, b) {
		t.Fatal("alias rejection broke native hard link")
	}
}
