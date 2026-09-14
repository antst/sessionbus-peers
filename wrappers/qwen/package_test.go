// SPDX-License-Identifier: MIT
package qwen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledMCPExecutableFollowsPublicLinkAndKeepsPrivateBasename(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	installed := filepath.Join(root, "permanent", Product)
	must(t, os.MkdirAll(filepath.Dir(installed), 0700))
	must(t, os.WriteFile(installed, []byte("binary fixture"), 0700))
	public := filepath.Join(root, Product)
	must(t, os.Symlink(installed, public))
	alias := filepath.Join(filepath.Dir(installed), PrivateAlias)
	must(t, os.Symlink(Product, alias))
	got, err := installedMCPExecutable(public)
	must(t, err)
	check(t, got == alias, "private path=%q, want=%q", got, alias)
	must(t, os.Remove(alias))
	if _, err := installedMCPExecutable(public); err == nil {
		t.Fatal("missing alias accepted")
	}
	must(t, os.WriteFile(alias, []byte("different executable"), 0700))
	if _, err := installedMCPExecutable(public); err == nil {
		t.Fatal("unrelated alias executable accepted")
	}
	must(t, os.Remove(alias))
	must(t, os.Symlink(Product, alias))
	must(t, os.Chmod(installed, 0600))
	if _, err := installedMCPExecutable(public); err == nil {
		t.Fatal("nonexecutable accepted")
	}
}
