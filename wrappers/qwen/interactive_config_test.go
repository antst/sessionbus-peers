// SPDX-License-Identifier: MIT
package qwen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var testManagedMCP = json.RawMessage(`{"command":"/owned/qwen-peer-mcp","env":{"SESSIONBUS_QWEN_ENDPOINT":"/owned/socket"}}`)

func TestInteractiveMCPCompositionKeepsNativePositionAndRawFields(t *testing.T) {
	caller := `{"mcpServers":{"other":{"command":"native","args":["--","quoted\\value"],"future":{"large":9007199254740993,"fraction":1.2300,"nested":[false,null]}},"array":[]},"unknownOuter":{"keep":true}}`
	for _, attached := range []bool{false, true} {
		args := []string{"--resume", "native title", "--mcp-config", caller, "--future", "opaque", "--", "--mcp-config", "literal"}
		valueIndex := 3
		if attached {
			args = append([]string{"--resume", "native title", "--mcp-config=" + caller}, args[4:]...)
			valueIndex = 2
		}
		before := append([]string(nil), args...)
		got, err := composeInteractiveMCP(args, testManagedMCP)
		must(t, err)
		check(t, reflect.DeepEqual(args, before), "mutated caller argv")
		body := got[valueIndex]
		if attached {
			body = strings.TrimPrefix(body, "--mcp-config=")
		}
		var decoded map[string]json.RawMessage
		must(t, json.Unmarshal([]byte(body), &decoded))
		var servers map[string]json.RawMessage
		must(t, json.Unmarshal(decoded["mcpServers"], &servers))
		check(t, string(decoded["unknownOuter"]) == `{"keep":true}`, "outer field changed: %s", body)
		check(t, strings.Contains(string(servers["other"]), `9007199254740993`) && strings.Contains(string(servers["other"]), `1.2300`), "numeric tokens rewritten: %s", body)
		check(t, string(servers["array"]) == "[]" && string(servers["sessionbus"]) == string(testManagedMCP), "server fields changed: %s", body)
		got[valueIndex] = before[valueIndex]
		check(t, reflect.DeepEqual(got, before), "other native args changed: %#v", got)
	}
}

func TestInteractiveMCPFileCommentsAndFilePriority(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	content := []byte("{\n // comment\n \"other\": {\"url\":\"https://example.invalid/*literal*/\", /* block */ \"escape\":\"a\\\"//b\"}\n}\n")
	must(t, os.WriteFile(file, content, 0600))
	link := filepath.Join(t.TempDir(), "config-link")
	must(t, os.Symlink(file, link))
	got, err := composeInteractiveMCP([]string{"--mcp-config", link}, testManagedMCP)
	must(t, err)
	var servers map[string]json.RawMessage
	must(t, json.Unmarshal([]byte(got[1]), &servers))
	check(t, len(servers) == 2 && strings.Contains(string(servers["other"]), "https://example.invalid/*literal*/"), "comment processing altered strings: %s", got[1])
	after, err := os.ReadFile(file)
	must(t, err)
	check(t, string(after) == string(content), "caller file mutated")
	if _, err := composeInteractiveMCP([]string{"--mcp-config", string(content)}, testManagedMCP); err == nil {
		t.Fatal("inline comments accepted")
	}
	// Even a valid inline JSON spelling denotes an existing file first.
	t.Chdir(t.TempDir())
	must(t, os.WriteFile("{}", []byte(`{"fromFile":{}}`), 0600))
	got, err = composeInteractiveMCP([]string{"--mcp-config", "{}"}, testManagedMCP)
	must(t, err)
	check(t, strings.Contains(got[1], "fromFile"), "file priority lost: %s", got[1])
}

func TestInteractiveMCPAbsentAndNativeBoundary(t *testing.T) {
	args := []string{"--resume", "--", "--mcp-config", "literal"}
	got, err := composeInteractiveMCP(args, testManagedMCP)
	must(t, err)
	check(t, got[0] == "--mcp-config" && reflect.DeepEqual(got[2:], args), "native boundary changed: %#v", got)
}

func TestInteractiveMCPConflictsInvalidAndBounds(t *testing.T) {
	for _, args := range [][]string{
		{"--mcp-config"}, {"--mcp-config", "--"}, {"--mcp-config="},
		{"--mcp-config", "{}", "--mcp-config={}"},
		{"--mcp-config", `{"sessionbus":{}}`},
		{"--mcp-config", `{"mcpServers":{"sessionbus":{"disabled":true}}}`},
		{"--mcp-config", `{"mcpServers":null}`}, {"--mcp-config", `{"mcpServers":[]}`},
		{"--mcp-config", `[]`}, {"--mcp-config", `null`}, {"--mcp-config", `{"other":null}`},
		{"--mcp-config", `{"other":false}`}, {"--mcp-config", `{"other":{},}`},
		{"--mcp-config", strings.Repeat(" ", maxInteractiveMCPConfig+1)},
		{"--mcp-config", `{"other":{"text":"` + strings.Repeat("x", maxInteractiveMCPConfig-30) + `"}}`},
	} {
		if _, err := composeInteractiveMCP(args, testManagedMCP); err == nil {
			t.Fatalf("accepted invalid/conflicting config (%d args, value bytes %d)", len(args), len(args[len(args)-1]))
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "large.json")
	must(t, os.WriteFile(file, []byte(strings.Repeat(" ", maxInteractiveMCPConfig+1)), 0600))
	for _, value := range []string{file, dir} {
		if _, err := composeInteractiveMCP([]string{"--mcp-config", value}, testManagedMCP); err == nil {
			t.Fatal("accepted oversized/non-regular config file")
		}
	}
}
