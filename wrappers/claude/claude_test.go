// SPDX-License-Identifier: MIT
package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/antst/sessionbus-peers/internal/testsocket"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
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
		{`{"mcpServers":[{"name":"plugin:sessionbus:sessionbus","status":"connected","tools":[{"name":"sessionbus"}]}]}`, true},
		{`{"mcpServers":[{"name":"plugin:sessionbus:sessionbus","status":"pending","tools":[{"name":"sessionbus"}]}]}`, false},
		{`{"mcpServers":[{"name":"plugin:sessionbus:sessionbus","status":"connected"}]}`, false},
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

func TestWorkerHelloUsesActualProtocolSchema(t *testing.T) {
	h, err := New("unused").Hello(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(protocol.WorkerHello{Protocol: 1, LaunchToken: "offline-test-token", HelloDescription: h})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.DecodeParams("session.hello", raw); err != nil {
		t.Fatal(err)
	}
	description, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var result kit.LaneDescribeResult
	if err := protocol.UnmarshalResult("lane.describe", description, &result); err != nil {
		t.Fatal(err)
	}
}

func TestOpenTitleRequiresNativeConfirmation(t *testing.T) {
	for _, title := range []string{"", "${session_title}", "other"} {
		if confirmTitle("parent/child@local", title) == nil {
			t.Fatalf("unconfirmed %q admitted", title)
		}
	}
	if err := confirmTitle("parent/child@local", "parent/child"); err != nil {
		t.Fatal(err)
	}
}

// Only the native boundary is controlled here. Public hello/open/run and result
// serialization use the installed kit Worker and protocol over an actual socket.
type workerStreamProduct struct{ *Wrapper }

func (p *workerStreamProduct) Open(context.Context, kit.OpenRequest) (kit.OpenResult, error) {
	p.mu.Lock()
	p.opened = true
	p.identity = "native-id"
	p.mu.Unlock()
	return kit.OpenResult{SessionID: "native-id"}, nil
}
func TestWorkerSerializesTerminalBeforeNativeEOFShutdown(t *testing.T) {
	path := filepath.Join(testsocket.Directory(t), "bus.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("SESSIONBUS_SOCKET", path)
	t.Setenv("SESSIONBUS_LAUNCH_TOKEN", "controlled-token")
	t.Setenv("SESSIONBUS_LOCAL_KEY", "")
	p := New(t.TempDir())
	p.ctx, p.cancel = context.WithCancel(context.Background())
	nativeInput, workerInput := io.Pipe()
	workerOutput, nativeOutput := io.Pipe()
	p.stream = newStream(workerInput, workerOutput, p.fail)
	worker := kit.NewWorker(&workerStreamProduct{p})
	p.SetCaller(worker.Caller())
	p.SetShutdown(worker.Shutdown)
	done := make(chan error, 1)
	go func() { done <- worker.Serve(context.Background()) }()
	c, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	reader := bufio.NewReader(c)
	receive := func() protocol.Frame {
		t.Helper()
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		f, err := protocol.DecodeFrame(line[:len(line)-1])
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	send := func(body []byte, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	hello := receive()
	if _, err = protocol.DecodeParams("session.hello", hello.Params); err != nil {
		t.Fatal(err)
	}
	send(protocol.ResultBytes(hello.ID, "session.hello", struct{}{}))
	send(protocol.RequestBytes(1, "session.open", kit.OpenRequest{Name: "parent/child@local", Groups: []string{}}))
	opened := receive()
	var openResult kit.OpenResult
	if err = protocol.UnmarshalResult("session.open", opened.Result, &openResult); err != nil {
		t.Fatal(err)
	}
	send(protocol.RequestBytes(2, "turn.run", kit.TurnRunRequest{SessionID: "native-id", Input: "prompt"}))
	var input map[string]json.RawMessage
	if err = json.NewDecoder(nativeInput).Decode(&input); err != nil {
		t.Fatal(err)
	}
	id := rawString(t, input["uuid"])
	encoder := json.NewEncoder(nativeOutput)
	if err = encoder.Encode(map[string]any{"type": "user", "session_id": "native-id", "uuid": id, "isReplay": true}); err != nil {
		t.Fatal(err)
	}
	if err = encoder.Encode(map[string]any{"type": "result", "session_id": "native-id", "subtype": "success", "result": "retained terminal", "user_message_uuid": id}); err != nil {
		t.Fatal(err)
	}
	_ = nativeOutput.Close()
	result := receive()
	var terminal kit.TurnResult
	if err = protocol.UnmarshalResult("turn.run", result.Result, &terminal); err != nil {
		t.Fatal(err)
	}
	if result.ID != 2 || terminal.Outcome != "completed" || terminal.Result != "retained terminal" {
		t.Fatalf("%+v %+v", result, terminal)
	}
	if _, err = reader.ReadByte(); err != io.EOF {
		t.Fatalf("worker did not retire after terminal: %v", err)
	}
	<-done
	_ = nativeInput.Close()
}
