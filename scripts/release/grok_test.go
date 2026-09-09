// SPDX-License-Identifier: MIT
package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGrokReinstallUsesNativeRepositoryInventory(t *testing.T) {
	home, src, bin := t.TempDir(), t.TempDir(), t.TempDir()
	script, err := os.ReadFile("install-product")
	if err != nil {
		t.Fatal(err)
	}
	put := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0700); err != nil {
			t.Fatal(err)
		}
	}
	put(filepath.Join(src, "install"), string(script))
	for _, f := range []string{"LICENSE", "THIRD-PARTY-NOTICES.txt", "README.md", "grok-peer", "plugin/manifest"} {
		put(filepath.Join(src, f), "fixture")
	}
	put(filepath.Join(src, "ROLE"), "grok")
	put(filepath.Join(bin, "grok"), `#!/bin/sh
set -eu
case "$2" in
 list) if [ -f "$HOME/native-repo" ]; then printf '[{"name":"sessionbus","repo_key":"plugin-1234"}]\n'; else printf '[]\n'; fi;;
 uninstall) test "$3" = sessionbus; test "$4" = --keep-data; rm "$HOME/native-repo";;
 install) test ! -f "$HOME/native-repo"; printf installed > "$HOME/native-repo";;
 *) exit 4;;
esac
`)
	for i := 0; i < 2; i++ {
		c := exec.Command("sh", filepath.Join(src, "install"))
		c.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("install %d: %v %s", i, err, out)
		}
	}
}
