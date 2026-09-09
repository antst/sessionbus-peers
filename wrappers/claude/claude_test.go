// SPDX-License-Identifier: MIT
package claude

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestNativeArgumentsPreserveCallerSuffix(t *testing.T) {
	var request kit.OpenRequest
	if err := json.Unmarshal([]byte(`{"name":"parent/child@local","resume_session_id":"native-id","open":{"model":"native-model","permission_mode":"default","reasoning_effort":"high","arguments":["--model","last-model","--","literal"]}}`), &request); err != nil {
		t.Fatal(err)
	}
	actual := launchArguments(request, "/installed plugin", "/owned settings")
	expected := []string{"--allowedTools", interactive.PublicTool, "--plugin-dir", "/installed plugin", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--replay-user-messages", "--settings", "/owned settings", "--name", "parent/child", "--resume", "native-id", "--permission-mode", "default", "--model", "native-model", "--effort", "high", "--model", "last-model", "--", "literal"}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("argv %#v", actual)
	}
}
func TestRequiredToolsNeedsConnectedPresence(t *testing.T) {
	for _, tc := range []struct {
		value string
		ready bool
	}{
		{`{"mcpServers":[{"name":"plugin_sessionbus_sessionbus","status":"connected","tools":[{"name":"sessionbus"}]}]}`, true},
		{`{"mcpServers":[{"name":"plugin_sessionbus_sessionbus","status":"pending","tools":[{"name":"sessionbus"}]}]}`, false},
		{`{"mcpServers":[{"name":"plugin_sessionbus_sessionbus","status":"connected"}]}`, false},
		{`{"mcpServers":[{"name":"other","status":"connected","tools":[{"name":"sessionbus"}]}]}`, false},
	} {
		if got := requiredTools(json.RawMessage(tc.value)); (got == nil) != tc.ready {
			t.Fatalf("%s: %v", tc.value, got)
		}
	}
}
func TestFailedOpenRemovesItsEndpoint(t *testing.T) {
	// Native lookup fails before spawn; Open itself must release its allocated listener.
	t.Setenv("PATH", t.TempDir())
	p := New(t.TempDir())
	_, err := p.Open(context.Background(), kit.OpenRequest{})
	if err == nil {
		t.Fatal("missing native executable accepted")
	}
	if p.endpoint == nil {
		t.Fatal("test did not allocate endpoint")
	}
	if _, err := os.Stat(p.endpoint.dir); !os.IsNotExist(err) {
		t.Fatalf("endpoint retained: %v", err)
	}
	if !p.closing {
		t.Fatal("unsuccessful Open did not close")
	}
}
