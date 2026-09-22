// SPDX-License-Identifier: MIT

package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/sessionbus/peer-common/host"
	"github.com/sessionbus/peer-common/testsocket"
)

type turnDone struct {
	result sessionkit.TurnResult
	err    error
}

func TestWrapperFreshOpenAndClose(t *testing.T) {
	t.Setenv("GO_WANT_CODEX_PROCESS", "1")
	evidence := filepath.Join(t.TempDir(), "child.json")
	t.Setenv("CODEX_TEST_EVIDENCE", evidence)
	t.Setenv(host.TokenEnv, "must-not-reach-child")
	workspace := t.TempDir()
	t.Setenv("CODEX_TEST_CWD", workspace)
	original := laneCommand
	laneCommand = func(_ string, arguments ...string) *exec.Cmd {
		return exec.Command(os.Args[0], append([]string{"-test.run=TestCodexProcess", "--"}, arguments...)...)
	}
	t.Cleanup(func() { laneCommand = original })
	socket := filepath.Join(testsocket.Directory(t), "sessionbus.sock")
	p := New()
	p.SetCall(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	result, err := p.Open(context.Background(), sessionkit.OpenRequest{
		Name: "parent/lane@host", Open: sessionkit.OpenOptions{Cwd: workspace, Model: "gpt-test", ReasoningEffort: "high", Arguments: []string{"--enable", "feature"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "thread-1" {
		t.Fatalf("session = %q", result.SessionID)
	}
	if err = p.Close(context.Background(), sessionkit.SessionCloseRequest{}); err != nil {
		t.Fatal(err)
	}
	var observed struct {
		Args, Env []string
		Calls     []appRequest
	}
	body, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(body, &observed) != nil {
		t.Fatalf("evidence = %s", body)
	}
	wantTail := []string{"--enable", "feature"}
	if !slices.Equal(observed.Args[len(observed.Args)-len(wantTail):], wantTail) {
		t.Fatalf("args = %v", observed.Args)
	}
	if slices.Contains(observed.Env, host.TokenEnv) || !slices.Contains(observed.Env, EndpointEnv) {
		t.Fatalf("env names = %v", observed.Env)
	}
	for _, call := range observed.Calls {
		if call.Method == "thread/delete" || call.Method == "thread/archive" {
			t.Fatalf("close mutated history: %+v", call)
		}
		if strings.Contains(string(call.Params), `"approvalPolicy"`) || strings.Contains(string(call.Params), `"code_mode_host"`) {
			t.Fatalf("implicit policy override: %+v", call)
		}
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(socket), "locks")); !os.IsNotExist(err) {
		t.Fatalf("lock created: %v", err)
	}
	if _, err = os.Stat(p.endpoint.path); !os.IsNotExist(err) {
		t.Fatalf("endpoint remains: %v", err)
	}
}

func TestWrapperLaneBypassUsesAppServerPolicy(t *testing.T) {
	t.Setenv("GO_WANT_CODEX_PROCESS", "1")
	evidence := filepath.Join(t.TempDir(), "child.json")
	t.Setenv("CODEX_TEST_EVIDENCE", evidence)
	workspace := t.TempDir()
	t.Setenv("CODEX_TEST_CWD", workspace)
	original := laneCommand
	laneCommand = func(_ string, arguments ...string) *exec.Cmd {
		return exec.Command(os.Args[0], append([]string{"-test.run=TestCodexProcess", "--"}, arguments...)...)
	}
	t.Cleanup(func() { laneCommand = original })
	p := New()
	p.SetCall(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	result, err := p.Open(context.Background(), sessionkit.OpenRequest{
		Name: "parent/bypass@host",
		Open: sessionkit.OpenOptions{
			Cwd:       workspace,
			Arguments: []string{"--enable", "feature", codexNativeBypass},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "thread-1" {
		t.Fatalf("session = %q", result.SessionID)
	}
	if err = p.Close(context.Background(), sessionkit.SessionCloseRequest{}); err != nil {
		t.Fatal(err)
	}
	var observed struct {
		Args  []string
		Calls []appRequest
	}
	body, err := os.ReadFile(evidence)
	if err != nil || json.Unmarshal(body, &observed) != nil {
		t.Fatalf("evidence = %s, %v", body, err)
	}
	if slices.Contains(observed.Args, codexNativeBypass) || !slices.Contains(observed.Args, sessionbusApprovalConfig) {
		t.Fatalf("native arguments = %q", observed.Args)
	}
	seen := map[string]bool{}
	for _, call := range observed.Calls {
		if call.Method != "thread/start" && call.Method != "thread/resume" {
			continue
		}
		var params map[string]any
		if json.Unmarshal(call.Params, &params) != nil || params["approvalPolicy"] != "never" || params["sandbox"] != "danger-full-access" {
			t.Fatalf("%s params = %s", call.Method, call.Params)
		}
		seen[call.Method] = true
	}
	if !seen["thread/start"] || !seen["thread/resume"] {
		t.Fatalf("policy-bearing calls = %v", seen)
	}
}

func TestCodexProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODEX_PROCESS") != "1" {
		return
	}
	if os.Getenv("CODEX_TEST_LARGE_EXIT") == "1" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"method": "turn/completed", "params": map[string]any{"threadId": "thread-1", "turn": map[string]any{
				"id": "turn-1", "status": "completed", "completedAt": 1, "items": []any{map[string]string{"type": "agentMessage", "phase": "final_answer", "text": strings.Repeat("x", 300000) + "tail"}},
			}},
		})
		os.Exit(0)
	}
	separator := slices.Index(os.Args, "--")
	names := []string{}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		names = append(names, name)
	}
	calls := []appRequest{}
	title := ""
	var mcp net.Conn
	var mcpDecoder *json.Decoder
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	for {
		var request appRequest
		if decoder.Decode(&request) != nil {
			body, _ := json.Marshal(map[string]any{"Args": os.Args[separator+1:], "Env": names, "Calls": calls})
			_ = os.WriteFile(os.Getenv("CODEX_TEST_EVIDENCE"), body, 0o600)
			os.Exit(0)
		}
		calls = append(calls, request)
		if request.Method == "turn/start" && os.Getenv("CODEX_TEST_DIE_TURN") == "1" {
			os.Exit(7)
		}
		result := any(map[string]any{})
		switch request.Method {
		case "thread/start":
			mcp, _ = net.Dial("unix", os.Getenv(EndpointEnv))
			if mcp != nil {
				_ = json.NewEncoder(mcp).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]string{"protocolVersion": "2024-11-05"}})
				mcpDecoder = json.NewDecoder(mcp)
				var frame any
				_ = mcpDecoder.Decode(&frame)
			}
			startup := first(os.Getenv("CODEX_TEST_STARTUP_STATUS"), "ready")
			_ = encoder.Encode(map[string]any{"method": "mcpServer/startupStatus/updated", "params": map[string]string{"threadId": "thread-1", "name": "sessionbus", "status": startup}})
			approval, sandbox := effectiveTestPolicy(request.Params)
			result = map[string]any{"thread": map[string]string{"id": "thread-1", "name": title}, "cwd": os.Getenv("CODEX_TEST_CWD"), "approvalPolicy": approval, "sandbox": map[string]string{"type": sandbox}}
		case "thread/name/set":
			var params struct{ Name string }
			_ = json.Unmarshal(request.Params, &params)
			title = params.Name
		case "mcpServerStatus/list":
			result = map[string]any{"data": []any{map[string]any{"name": "sessionbus", "pluginId": "codex@sessionbus-peers", "runtimeStatus": "connected", "tools": map[string]any{"sessionbus": map[string]string{"name": "sessionbus"}}}}}
		case "thread/resume":
			approval, sandbox := effectiveTestPolicy(request.Params)
			result = map[string]any{"thread": map[string]string{"id": "thread-1", "name": title}, "cwd": os.Getenv("CODEX_TEST_CWD"), "approvalPolicy": approval, "sandbox": map[string]string{"type": sandbox}}
		}
		if request.ID != 0 {
			_ = encoder.Encode(map[string]any{"id": request.ID, "result": result})
		}
	}
}

func effectiveTestPolicy(raw json.RawMessage) (string, string) {
	var params struct {
		ApprovalPolicy string `json:"approvalPolicy"`
		Sandbox        string `json:"sandbox"`
	}
	_ = json.Unmarshal(raw, &params)
	approval, sandbox := "never", "readOnly"
	if params.ApprovalPolicy != "" {
		approval = params.ApprovalPolicy
	}
	if params.Sandbox == "danger-full-access" {
		sandbox = "dangerFullAccess"
	}
	return approval, sandbox
}

func TestAbnormalRunCarriesNothingIntoReopen(t *testing.T) {
	t.Setenv("GO_WANT_CODEX_PROCESS", "1")
	t.Setenv("CODEX_TEST_DIE_TURN", "1")
	t.Setenv("CODEX_TEST_EVIDENCE", filepath.Join(t.TempDir(), "child.json"))
	workspace := t.TempDir()
	t.Setenv("CODEX_TEST_CWD", workspace)
	original := laneCommand
	laneCommand = func(_ string, arguments ...string) *exec.Cmd {
		return exec.Command(os.Args[0], append([]string{"-test.run=TestCodexProcess", "--"}, arguments...)...)
	}
	socket := filepath.Join(testsocket.Directory(t), "sessionbus.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(host.TokenEnv, "token")
	t.Setenv(host.SocketEnv, socket)
	p := New()
	p.SetCall(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	worker := sessionkit.NewWorker(p)
	p.SetShutdown(worker.Shutdown)
	served := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { served <- worker.Serve(ctx) }()
	connection, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = connection.Close()
		_ = listener.Close()
		<-worker.Closed()
		<-served
		laneCommand = original
	})
	decoder, encoder := json.NewDecoder(connection), json.NewEncoder(connection)
	var response map[string]any
	if decoder.Decode(&response) != nil || encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": response["id"], "result": map[string]any{}}) != nil {
		t.Fatal("worker hello")
	}
	if encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session.open", "params": map[string]any{"name": "reopen@local", "groups": []string{}, "open": map[string]any{"cwd": workspace}}}) != nil || decoder.Decode(&response) != nil || response["error"] != nil {
		t.Fatalf("open = %#v", response)
	}
	if encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "turn.execute", "params": map[string]any{"run_id": "g/1", "session_id": "thread-1@local", "input": "die"}}) != nil || decoder.Decode(&response) != nil {
		t.Fatal("turn.execute")
	}
	// Execute responds with admission; abnormal native completion is now a
	// separate metadata event before worker retirement, not a body response.
	response = nil
	if decoder.Decode(&response) != nil || response["method"] != "turn.ready" {
		t.Fatal(response)
	}
	terminal := response["params"].(map[string]any)
	if terminal["state"] != "unavailable" || terminal["outcome"] != nil {
		t.Fatal(terminal)
	}
	if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": response["id"], "result": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	<-worker.Closed()
	// Worker shutdown and retireRun independently await Run.Done; Closed does
	// not join the latter. Wait for its postcondition, not a scheduler yield count.
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		p.mu.Lock()
		carried := p.run
		p.mu.Unlock()
		if carried == nil {
			break
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatalf("completed Run survived the abnormal app-server exit: %p", carried)
		}
	}
}

func TestLargeTerminalFrameDrainsAfterExit(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestCodexProcess")
	command.Env = append(os.Environ(), "GO_WANT_CODEX_PROCESS=1", "CODEX_TEST_LARGE_EXIT=1")
	child, input, output, err := startNative(command)
	if err != nil {
		t.Fatal(err)
	}
	p := &Wrapper{id: "thread-1", child: child}
	native := &turn{owner: p, id: "turn-1", started: true, ready: make(chan error, 1), done: make(chan error, 1)}
	p.active = native
	p.app = newAppClient(input, output, p.receive, p.fail)
	go p.watch(child, p.app.done)
	result, err := native.Wait(context.Background())
	if err != nil {
		t.Fatalf("preserved terminal did not complete the turn: %v", err)
	}
	if result.Outcome != "completed" || len(result.Result) != 300004 || !strings.HasSuffix(result.Result, "tail") {
		t.Fatalf("terminal = %#v", result)
	}
	if err = p.Close(context.Background(), sessionkit.SessionCloseRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestLaneRunSteerAndTerminal(t *testing.T) {
	p, server := testLane(t)
	started := make(chan host.Turn, 1)
	go func() {
		turn, err := p.start(context.Background(), "original")
		if err != nil {
			t.Error(err)
		}
		started <- turn
	}()
	request := readAppRequest(t, server)
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-1", "status": "inProgress"}}})
	select {
	case turn := <-started:
		t.Fatalf("turn published before turn/started: %v", turn)
	default:
	}
	writeRaw(t, server, `{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","items":[],"itemsView":"notLoaded","status":"inProgress","error":null,"startedAt":1788680430,"completedAt":null,"durationMs":null}},"emittedAtMs":1788680430493}`)
	native := <-started
	injected := make(chan error, 1)
	go func() {
		outcome, err := p.injectExpected(context.Background(), "delivery", native.(*turn))
		if outcome != host.Injected && err == nil {
			err = context.Canceled
		}
		injected <- err
	}()
	request = readAppRequest(t, server)
	if request.Method != "turn/steer" || !strings.Contains(string(request.Params), `"expectedTurnId":"turn-1"`) {
		t.Fatalf("steer = %#v", request)
	}
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]string{"turnId": "turn-1"}})
	if err := <-injected; err != nil {
		t.Fatal(err)
	}
	writeRaw(t, server, `{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed"}},"emittedAtMs":1788679081756}`)
	waited := make(chan turnDone, 1)
	go func() { result, waitErr := native.Wait(context.Background()); waited <- turnDone{result, waitErr} }()
	request = readAppRequest(t, server)
	if request.Method != "thread/turns/list" || !strings.Contains(string(request.Params), `"itemsView":"full"`) {
		t.Fatalf("read = %#v", request)
	}
	completed := int64(1788679081)
	firstAnswer := strings.Repeat("x", 3506-len("MIDRUN=WORD-7X")) + "MIDRUN=WORD-7X"
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]any{"data": []any{
		map[string]any{"id": "other-turn", "items": []any{map[string]string{"type": "agentMessage", "text": "FOREIGN", "phase": "final_answer"}}, "status": "completed", "completedAt": completed},
		map[string]any{"id": "turn-1", "items": []any{
			map[string]string{"type": "agentMessage", "text": "commentary before the final", "phase": "commentary"},
			map[string]string{"type": "agentMessage", "text": firstAnswer, "phase": "final_answer"},
			map[string]string{"type": "userMessage", "text": "injected steering message"},
			map[string]string{"type": "toolMessage", "text": "tool output", "phase": "final_answer"},
			map[string]string{"type": "agentMessage", "text": "MIDRUN=KILO-7K", "phase": "final_answer"},
		}, "status": "completed", "completedAt": completed},
	}}})
	done := <-waited
	result, err := done.result, done.err
	want := firstAnswer + "\n\nMIDRUN=KILO-7K"
	if err != nil || result.Outcome != "completed" || result.Result != want || len(result.Result) != 3522 || result.NativeStopReason != "completed" {
		t.Fatalf("terminal = %#v, %v", result, err)
	}
}

func TestLaneBypassTurnUsesAppServerPolicy(t *testing.T) {
	p, server := testLane(t)
	p.sandbox = "danger-full-access"
	started := make(chan host.Turn, 1)
	errors := make(chan error, 1)
	go func() {
		turn, err := p.start(context.Background(), "bypass")
		if err != nil {
			errors <- err
			return
		}
		started <- turn
	}()
	request := readAppRequest(t, server)
	var params struct {
		ApprovalPolicy string `json:"approvalPolicy"`
		SandboxPolicy  struct {
			Type string `json:"type"`
		} `json:"sandboxPolicy"`
	}
	if request.Method != "turn/start" || json.Unmarshal(request.Params, &params) != nil || params.ApprovalPolicy != "never" || params.SandboxPolicy.Type != "dangerFullAccess" {
		t.Fatalf("turn/start = %s %s", request.Method, request.Params)
	}
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-bypass", "status": "inProgress"}}})
	writeRaw(t, server, `{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-bypass","status":"inProgress"}}}`)
	var native host.Turn
	select {
	case err := <-errors:
		t.Fatal(err)
	case native = <-started:
	}
	p.clear(native.(*turn))
}

func TestTerminalFinalAnswerProjection(t *testing.T) {
	completed := int64(1)
	tests := []struct {
		name  string
		items []nativeItem
		want  string
	}{
		{name: "single byte exact", items: []nativeItem{{Type: "agentMessage", Phase: "final_answer", Text: "\n exact bytes \t\n"}}, want: "\n exact bytes \t\n"},
		{name: "whitespace byte exact", items: []nativeItem{{Type: "agentMessage", Phase: "final_answer", Text: " \t\n"}}, want: " \t\n"},
		{name: "ordered and not deduplicated", items: []nativeItem{{Type: "agentMessage", Phase: "final_answer", Text: "same"}, {Type: "agentMessage", Phase: "final_answer", Text: "same"}}, want: "same\n\nsame"},
		{name: "empty ignored and whitespace retained", items: []nativeItem{{Type: "agentMessage", Phase: "final_answer"}, {Type: "agentMessage", Phase: "final_answer", Text: " \t\n"}, {Type: "agentMessage", Phase: "final_answer", Text: "tail "}}, want: " \t\n\n\ntail "},
		{name: "commentary and tools excluded", items: []nativeItem{{Type: "agentMessage", Phase: "commentary", Text: "thinking"}, {Type: "toolMessage", Phase: "final_answer", Text: "tool"}, {Type: "agentMessage", Phase: "final_answer", Text: "answer"}}, want: "answer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := terminal(nativeTurn{ID: "turn", Status: "completed", CompletedAt: &completed, Items: test.items})
			if err != nil || result.Result != test.want {
				t.Fatalf("terminal = %#v, %v", result, err)
			}
		})
	}
}

func TestLaneInterruptAfterNativeStart(t *testing.T) {
	p, server := testLane(t)
	started := make(chan host.Turn, 1)
	go func() { turn, _ := p.start(context.Background(), "block"); started <- turn }()
	request := readAppRequest(t, server)
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]any{"turn": map[string]string{"id": "turn-2"}}})
	writeRaw(t, server, `{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-2","status":"inProgress"}}}`)
	native := <-started
	interrupted := make(chan error, 1)
	go func() { interrupted <- native.Interrupt(context.Background()) }()
	request = readAppRequest(t, server)
	if request.Method != "turn/interrupt" {
		t.Fatalf("interrupt = %#v", request)
	}
	writeRaw(t, server, `{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-2","items":[],"itemsView":"notLoaded","status":"interrupted","error":null,"startedAt":1788680430,"completedAt":1788680430,"durationMs":22}},"emittedAtMs":1788680430509}`)
	waited := make(chan turnDone, 1)
	go func() { result, waitErr := native.Wait(context.Background()); waited <- turnDone{result, waitErr} }()
	read := readAppRequest(t, server)
	if read.Method != "thread/turns/list" {
		t.Fatalf("read = %#v", read)
	}
	completed := int64(1788680430)
	writeApp(t, server, map[string]any{"id": read.ID, "result": map[string]any{"data": []any{map[string]any{"id": "turn-2", "status": "interrupted", "completedAt": completed}}}})
	done := <-waited
	result, err := done.result, done.err
	if err != nil || result.Outcome != "interrupted" {
		t.Fatalf("terminal = %#v, %v", result, err)
	}
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]any{}})
	if err := <-interrupted; err != nil {
		t.Fatal(err)
	}
}

func TestLaneTerminalBeforeSteerResponseRetainsAdmission(t *testing.T) {
	p, server := testLane(t)
	started := make(chan host.Turn, 1)
	go func() { turn, _ := p.start(context.Background(), "original"); started <- turn }()
	request := readAppRequest(t, server)
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]any{"turn": map[string]string{"id": "turn-3"}}})
	writeRaw(t, server, `{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-3","status":"inProgress"}}}`)
	native := <-started
	injected := make(chan struct {
		outcome host.Injection
		err     error
	}, 1)
	go func() {
		outcome, err := p.injectExpected(context.Background(), "late", native.(*turn))
		injected <- struct {
			outcome host.Injection
			err     error
		}{outcome, err}
	}()
	request = readAppRequest(t, server)
	writeRaw(t, server, `{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-3","status":"completed"}}}`)
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]string{"turnId": "turn-3"}})
	if got := <-injected; got.outcome != host.Injected || got.err != nil {
		t.Fatalf("inject = %v, %v", got.outcome, got.err)
	}
	waited := make(chan error, 1)
	go func() { _, err := native.Wait(context.Background()); waited <- err }()
	request = readAppRequest(t, server)
	completed := int64(1)
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]any{"data": []any{map[string]any{"id": "turn-3", "status": "completed", "completedAt": completed, "items": []any{map[string]string{"type": "agentMessage", "phase": "final_answer", "text": "done"}}}}}})
	if err := <-waited; err != nil {
		t.Fatal(err)
	}
}

func TestCompletedTurnRequiresFinalAnswer(t *testing.T) {
	completed := int64(1)
	for _, items := range [][]nativeItem{nil, {{Type: "agentMessage", Phase: "final_answer"}}} {
		if _, err := terminal(nativeTurn{ID: "turn-4", Status: "completed", CompletedAt: &completed, Items: items}); err == nil || !strings.Contains(err.Error(), "no final answer") {
			t.Fatalf("items = %#v, error = %v", items, err)
		}
	}
}

func TestCapturedFailedTurn(t *testing.T) {
	raw := `{"method":"turn/completed","params":{"threadId":"01a075f0-66ba-7b52-87a7-5dca3de53066","turn":{"id":"01a075f0-6737-7b02-ae86-c9be43e17c19","items":[],"itemsView":"notLoaded","status":"failed","error":{"message":"{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The 'sessionbus-invalid-model' model is not supported when using Codex with a ChatGPT account.\"}}","codexErrorInfo":"other","additionalDetails":null,"misalignment":null},"startedAt":1788685084,"completedAt":1788685085,"durationMs":1261}},"emittedAtMs":1788685085742}`
	var frame appFrame
	var event struct {
		Turn nativeTurn `json:"turn"`
	}
	if json.Unmarshal([]byte(raw), &frame) != nil || json.Unmarshal(frame.Params, &event) != nil {
		t.Fatal("decode captured failed turn")
	}
	result, err := terminal(event.Turn)
	if err != nil || result.Outcome != "failed" || result.NativeStopReason != "failed" || !strings.Contains(result.Result, "sessionbus-invalid-model") {
		t.Fatalf("terminal = %#v, %v", result, err)
	}
}

func TestOpenEffectiveSettings(t *testing.T) {
	p := &Wrapper{approval: "never", sandbox: "danger-full-access"}
	reply := threadReply{Cwd: "/work", ApprovalPolicy: "never"}
	reply.Sandbox.Type = "dangerFullAccess"
	if err := p.checkEffective("thread/resume", "/work", reply); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*threadReply){
		func(r *threadReply) { r.Cwd = "/other" },
		func(r *threadReply) { r.ApprovalPolicy = "on-request" },
		func(r *threadReply) { r.Sandbox.Type = "readOnly" },
	} {
		changed := reply
		mutate(&changed)
		if err := p.checkEffective("thread/resume", "/work", changed); err == nil {
			t.Fatalf("accepted %#v", changed)
		}
	}
}

type appRequest struct {
	ID     int
	Method string
	Params json.RawMessage
}

func testLane(t *testing.T) (*Wrapper, net.Conn) {
	client, server := net.Pipe()
	p := &Wrapper{id: "thread-1", approval: "never"}
	p.app = newAppClient(client, client, p.receive, p.fail)
	t.Cleanup(func() { _ = server.Close() })
	return p, server
}

func readAppRequest(t *testing.T, connection net.Conn) appRequest {
	t.Helper()
	line, err := bufio.NewReader(connection).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var request appRequest
	if json.Unmarshal(line, &request) != nil {
		t.Fatalf("request = %s", line)
	}
	return request
}

func writeApp(t *testing.T, connection net.Conn, value any) {
	t.Helper()
	if json.NewEncoder(connection).Encode(value) != nil {
		t.Fatal("write response")
	}
}
func writeRaw(t *testing.T, connection net.Conn, value string) {
	t.Helper()
	if _, err := connection.Write([]byte(value + "\n")); err != nil {
		t.Fatal(err)
	}
}
