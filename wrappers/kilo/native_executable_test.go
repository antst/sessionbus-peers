// SPDX-License-Identifier: MIT

package kilo

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// Use the actual Go test executable's format, without running it as a native CLI.
func nativeFixture(t *testing.T, path string) string {
	t.Helper()
	current, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
func npmFixture(t *testing.T) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "node_modules", "@kilocode", "cli")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(root, "bin", "kilo")
	if err := os.WriteFile(shim, []byte("#!/usr/bin/env node\nthrow new Error('must never execute npm shim')\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"@kilocode/cli","version":"7.6.2","bin":{"kilo":"./bin/kilo"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	front := filepath.Join(t.TempDir(), "kilo")
	if err := os.Symlink(shim, front); err != nil {
		t.Fatal(err)
	}
	return root, front
}
func TestNativeResolverDirectOverrideAndResources(t *testing.T) {
	native := nativeFixture(t, filepath.Join(t.TempDir(), "native with spaces"))
	resource := filepath.Join(filepath.Dir(native), "tree-sitter")
	if err := os.Mkdir(resource, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resource, "tree-sitter.wasm"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KILO_BIN_PATH", native)
	result, err := ResolveNativeExecutable("missing-kilo-frontdoor")
	if err != nil || result.Path != native {
		t.Fatalf("override: %#v %v", result, err)
	}
	for _, row := range []struct{ before, after []string }{
		{[]string{"OTHER=kept"}, []string{"OTHER=kept", "KILO_TREE_SITTER_WASM_DIR=" + resource}},
		{[]string{"KILO_TREE_SITTER_WASM_DIR=/explicit", "OTHER=kept"}, []string{"KILO_TREE_SITTER_WASM_DIR=/explicit", "OTHER=kept"}},
		{[]string{"KILO_TREE_SITTER_WASM_DIR=/old", "KILO_TREE_SITTER_WASM_DIR="}, []string{"KILO_TREE_SITTER_WASM_DIR=" + resource}},
	} {
		before := append([]string(nil), row.before...)
		if got := result.Environment(row.before); !reflect.DeepEqual(got, row.after) {
			t.Fatalf("environment %v, want %v", got, row.after)
		}
		if !reflect.DeepEqual(before, row.before) {
			t.Fatal("input environment mutated")
		}
	}
	t.Setenv("KILO_BIN_PATH", "")
	direct, err := ResolveNativeExecutable(native)
	if err != nil || direct.Path != native {
		t.Fatalf("direct frontdoor: %#v %v", direct, err)
	}
}
func TestNativeResolverUsesOnlyNativeSelectedCache(t *testing.T) {
	t.Setenv("KILO_BIN_PATH", "")
	root, front := npmFixture(t)
	nativeFixture(t, filepath.Join(root, "node_modules", "@kilocode", "cli-linux-x64", "bin", "kilo"))
	if _, err := ResolveNativeExecutable(front); err == nil || !strings.Contains(err.Error(), "cache is unavailable") {
		t.Fatalf("missing selected cache must not invoke platform fallback: %v", err)
	}
	cache := nativeFixture(t, filepath.Join(root, "bin", ".kilo"))
	result, err := ResolveNativeExecutable(front)
	if err != nil || result.Path != cache {
		t.Fatalf("cache: %#v %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"unrelated","version":"7.6.2","bin":{"kilo":"./bin/kilo"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNativeExecutable(front); err == nil {
		t.Fatal("unrelated package accepted")
	}
}
func TestNativeResolverBadOverrideDoesNotFallBack(t *testing.T) {
	root, front := npmFixture(t)
	nativeFixture(t, filepath.Join(root, "bin", ".kilo"))
	override := filepath.Join(t.TempDir(), "override")
	if err := os.WriteFile(override, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KILO_BIN_PATH", override)
	if _, err := ResolveNativeExecutable(front); err == nil || !strings.Contains(err.Error(), "KILO_BIN_PATH") {
		t.Fatalf("invalid override must not fall back: %v", err)
	}
}
func TestNativeResolverBareOverrideResourceDirectoryMatchesShim(t *testing.T) {
	dir := t.TempDir()
	native := nativeFixture(t, filepath.Join(dir, "selected-native"))
	t.Setenv("PATH", dir)
	t.Setenv("KILO_BIN_PATH", "selected-native")
	// The native shim searches cwd/tree-sitter for this spelling, not PATH's dir.
	if err := os.Mkdir(filepath.Join(dir, "tree-sitter"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tree-sitter", "tree-sitter.wasm"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	result, err := ResolveNativeExecutable("kilo")
	if err != nil || result.Path != native || result.resourceDirectory != "" {
		t.Fatalf("bare override: %#v %v", result, err)
	}
}
func TestNativeResolverRejectsNonregularAndOversizedMetadata(t *testing.T) {
	t.Setenv("KILO_BIN_PATH", "")
	root, front := npmFixture(t)
	nativeFixture(t, filepath.Join(root, "bin", ".kilo"))
	manifest := filepath.Join(root, "package.json")
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNativeExecutable(front); err == nil {
		t.Fatal("FIFO metadata accepted")
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, bytes.Repeat([]byte(" "), 64*1024+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNativeExecutable(front); err == nil {
		t.Fatal("oversized metadata accepted")
	}
}

func TestNativeResolverRelativeAndSymlinkOverrideResources(t *testing.T) {
	for _, mode := range []string{"relative", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			cwd := t.TempDir()
			t.Chdir(cwd)
			selected := filepath.Join(cwd, "selected", "kilo")
			if mode == "relative" {
				nativeFixture(t, selected)
			} else {
				native := nativeFixture(t, filepath.Join(cwd, "physical", "kilo"))
				if err := os.MkdirAll(filepath.Dir(selected), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(native, selected); err != nil {
					t.Fatal(err)
				}
			}
			resource := filepath.Join(filepath.Dir(selected), "tree-sitter")
			if err := os.Mkdir(resource, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(resource, "tree-sitter.wasm"), []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
			override := selected
			if mode == "relative" {
				override = filepath.Join("selected", "kilo")
			}
			t.Setenv("KILO_BIN_PATH", override)
			result, err := ResolveNativeExecutable("missing-frontdoor")
			if err != nil || result.Path != selected || result.resourceDirectory != resource {
				t.Fatalf("%s override: %#v %v", mode, result, err)
			}
		})
	}
}
