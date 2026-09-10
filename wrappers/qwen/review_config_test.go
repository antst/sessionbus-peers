// SPDX-License-Identifier: MIT
package qwen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReviewEmptyNativeConfig(t *testing.T) {
	if _, err := composeInteractiveMCP([]string{"--mcp-config="}, testManagedMCP); err != nil {
		t.Fatalf("native treats empty flag as no caller servers: %v", err)
	}
}
func TestReviewBareCRDoesNotEndNativeLineComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{// comment\r\"other\":{}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := composeInteractiveMCP([]string{"--mcp-config", path}, testManagedMCP); err == nil {
		t.Fatal("accepted config that native comment stripping leaves invalid")
	}
}
