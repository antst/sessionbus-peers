// SPDX-License-Identifier: MIT

package opencodefamily

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCompiledMaintenanceEntryNeedsNoNodeOrNativeRuntime(t *testing.T) {
	testCompiledMaintenanceEntry(t, "opencode")
}

func TestCompiledKiloMaintenanceEntryNeedsNoNodeOrNativeRuntime(t *testing.T) {
	testCompiledMaintenanceEntry(t, "kilo")
}

func testCompiledMaintenanceEntry(t *testing.T, product string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, product+"-peer")
	build := exec.Command("go", "build", "-o", bin, "./cmd/"+product+"-peer")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	home := filepath.Join(dir, "home")
	config := filepath.Join(dir, "native-config")
	plugin := filepath.Join(dir, "permanent plugin #1")
	if err := os.MkdirAll(plugin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "package.json"), []byte(`{"name":"@sessionbus/`+product+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		command := exec.Command(bin, "--sessionbus-install", "--plugin-dir", plugin)
		command.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + config, "PATH=" + filepath.Join(dir, "no-executables")}
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("install%d: %v %s", i, err, out)
		}
	}
	want := (&url.URL{Scheme: "file", Path: plugin}).String()
	for _, file := range []string{product + ".jsonc", "tui.jsonc"} {
		got := installedPluginEntries(t, filepath.Join(config, product, file), false)
		if len(got) != 1 || got[0] != want {
			t.Fatal("wrong native registration", file, got)
		}
	}
	command := exec.Command(bin, "--sessionbus-install", "--remove")
	command.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + config, "PATH="}
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("remove: %v %s", err, out)
	}
	for _, file := range []string{product + ".jsonc", "tui.jsonc"} {
		if got := installedPluginEntries(t, filepath.Join(config, product, file), false); len(got) != 0 {
			t.Fatal("retained own entry", got)
		}
	}
}
