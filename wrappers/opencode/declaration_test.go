// SPDX-License-Identifier: MIT

package opencode

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/mcp"
)

func TestNativeDeclarationEqualsSharedTool(t *testing.T) {
	want, err := json.MarshalIndent(mcp.Tool(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../opencode/sessionbus-tool.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, append(want, '\n')) {
		t.Fatal("run go run ./internal/cmd/gen-opencode-tool opencode/sessionbus-tool.json")
	}
}
