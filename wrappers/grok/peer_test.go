// SPDX-License-Identifier: MIT

package grok

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/antst/sessionbus-peers/internal/testsocket"
	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/mcp"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

type peerDeliveryResult struct {
	receipt sessionkit.DeliveryReceipt
	err     error
}

func TestInteractivePlan(t *testing.T) {
	for _, test := range []struct {
		args, native []string
		groups, name string
	}{
		{[]string{"--resume", "test1", "-g", "test", "--group=test2,test3", "--yolo"}, []string{"--resume", "test1", "--always-approve"}, `["test","test2","test3"]`, ""},
		{[]string{"--group", "a,b", "--future", "value with spaces", "-n", "first", "--group", "c", "--name=second", "--", "-g", "literal", "--yolo"}, []string{"--future", "value with spaces", "--", "-g", "literal", "--yolo"}, `["a","b","c"]`, "second"},
		{[]string{"--resume"}, []string{"--resume"}, `[]`, ""},
		{[]string{"--continue", "--fork-session"}, []string{"--continue", "--fork-session"}, `[]`, ""},
		{[]string{"--resume", testSessionID, "-r" + testSessionID}, []string{"--resume", testSessionID, "-r" + testSessionID}, `[]`, ""},
		{[]string{"--session-id", testSessionID, "--peer-name", "alias"}, []string{"--session-id", testSessionID}, `[]`, "alias"},
	} {
		plan, err := InteractivePlan(test.args, []string{"PATH=/bin", host.SessionIDEnv + "=inherited", host.NameEnv + "=inherited", host.GroupsEnv + `=["inherited"]`})
		must(t, err)
		check(t, slices.Equal(plan.Args, test.native), "argv changed: %#v", plan.Args)
		check(t, environment(plan.Env, host.SessionIDEnv) == "", "launcher invented/inherited session ID")
		check(t, environment(plan.Env, host.GroupsEnv) == test.groups && environment(plan.Env, host.NameEnv) == test.name, "owned options = %#v", plan.Env)
		check(t, environment(plan.Env, ManagedEnv) == "launch", "managed topology absent")
	}
	for _, args := range [][]string{{"--no-leader"}, {"--leader"}, {"--leader-socket=elsewhere"}, {"-g"}, {"--group", "--"}, {"-n"}} {
		_, err := InteractivePlan(args, nil)
		check(t, err != nil, "conflict/missing value accepted: %#v", args)
	}
	for _, args := range [][]string{{"--single", "prompt"}, {"-pprompt"}, {"--prompt-file", "prompt.txt"}, {"--prompt-json", `[]`}, {"--output-format", "json"}, {"--json-schema", `{}`}, {"--max-turns", "1"}, {"--include-partial-messages"}} {
		_, err := InteractivePlan(args, nil)
		check(t, err != nil && strings.Contains(err.Error(), "Sessionbus Grok lane"), "headless accepted: %#v", args)
	}
	for _, args := range [][]string{{"sessions", "list"}, {"--version"}, {"plugin", "list", "--json"}} {
		plan, err := InteractivePlan(args, []string{ManagedEnv + "=inherited"})
		must(t, err)
		check(t, slices.Equal(plan.Args, args) && environment(plan.Env, ManagedEnv) == "", "native command wrapped: %#v", plan)
	}
	for _, args := range [][]string{nil, {"--always-approve"}, {"--always-approve", "--permission-mode=default"}, {"--permission-mode", "default", "--always-approve"}, {"--permission-mode", "custom-native-value"}} {
		check(t, slices.Equal(interactivePolicy(args), args), "explicit native policy rewritten: %#v", args)
	}
	check(t, len(interactivePolicy([]string{"--", "--always-approve"})) == 0, "post-delimiter operand selected policy")
}

func TestNativeTitleEventsRefreshPeerWithoutDelivery(t *testing.T) {
	root := testsocket.Directory(t)
	socket := filepath.Join(root, "bus")
	server, hellos := fakeDaemon(t, socket)
	defer server.Close()
	t.Setenv(host.SocketEnv, socket)
	recordPath := filepath.Join(root, "record")
	changed := filepath.Join(root, "changed")
	t.Setenv("GROK_TEST_RECORD", recordPath)
	t.Setenv("GROK_TEST_SESSION_ID", testSessionID)
	t.Setenv("GROK_TEST_TITLES", "native-title,")
	t.Setenv("GROK_TEST_ROSTER_CHANGE", changed)
	env := managedPeerEnv(os.Environ(), testSessionID, filepath.Join(root, "leader.sock"))
	env = setEnvironment(env, host.GroupsEnv, `["peer-group"]`)
	backend, err := NewPeerBackend(context.Background(), env)
	must(t, err)
	defer backend.Shutdown()
	check(t, len(records(t, recordPath)) == 0, "native observer started before MCP initialize")
	backend.Initialized()
	initial := <-hellos
	check(t, initial.SessionID == testSessionID && initial.Name == "native-title" && slices.Equal(initial.Groups, []string{"peer-group"}), "initial native identity=%+v", initial)
	initial.ack <- true
	<-backend.ready
	must(t, os.WriteFile(changed, nil, 0600))
	renamed := <-hellos
	check(t, renamed.SessionID == testSessionID && renamed.Name == "", "empty native title replaced with invented name: %+v", renamed)
	renamed.ack <- true
	<-renamed.done
	listed, err := backend.Caller().List(context.Background(), sessionkit.SessionListRequest{})
	must(t, err)
	check(t, len(listed.Sessions) == 1, "peer absent")
	check(t, countFrames(records(t, recordPath), "_x.ai/interject") == 0, "title update required delivery")
}
func managedPeerEnv(env []string, id, leader string) []string {
	env = setEnvironment(env, grokSessionIDEnv, id)
	env = setEnvironment(env, grokLeaderSocketEnv, leader)
	return setEnvironment(env, ManagedEnv, leader)
}

func TestProductSessionIDsCreateDistinctPeers(t *testing.T) {
	for index, id := range []string{testSessionID, "01a07800-94fb-7b12-b531-2f0509e033f1"} {
		root := testsocket.Directory(t)
		socket := filepath.Join(root, "sessionbus.sock")
		server, hellos := fakeDaemon(t, socket)
		t.Setenv(host.SocketEnv, socket)
		t.Setenv("GROK_TEST_SESSION_ID", id)
		environment := setEnvironment(setEnvironment(os.Environ(), grokSessionIDEnv, id), grokLeaderSocketEnv, filepath.Join(root, "leader.sock"))
		environment = setEnvironment(environment, ManagedEnv, environmentValue(environment, grokLeaderSocketEnv))
		opened := make(chan *PeerBackend, 1)
		go func() {
			backend, _ := NewPeerBackend(context.Background(), environment)
			backend.Initialized()
			opened <- backend
		}()
		hello := <-hellos
		check(t, hello.SessionID == id && hello.Name == "", "helper %d hello = %#v", index+1, hello)
		hello.ack <- true
		backend := <-opened
		backend.mu.Lock()
		actual := backend.identity.SessionID
		backend.mu.Unlock()
		check(t, actual == id, "helper %d identity changed", index+1)
		backend.Shutdown()
		server.Close()
	}
}

func TestPeerMCPServesWhileBusAdmissionIsHeld(t *testing.T) {
	root := testsocket.Directory(t)
	socket := filepath.Join(root, "sessionbus.sock")
	server, hellos := fakeDaemon(t, socket)
	defer server.Close()
	t.Setenv(host.SocketEnv, socket)
	environment := setEnvironment(setEnvironment(os.Environ(), grokSessionIDEnv, testSessionID), grokLeaderSocketEnv, filepath.Join(root, "leader.sock"))
	environment = setEnvironment(environment, ManagedEnv, environmentValue(environment, grokLeaderSocketEnv))
	backend, err := NewPeerBackend(context.Background(), environment)
	must(t, err)
	input, writeInput := io.Pipe()
	readOutput, output := io.Pipe()
	served := make(chan error, 1)
	go func() {
		served <- mcp.ServeSessionbus(backend, input, output, mcp.ReportHandler{})
		_ = output.Close()
	}()
	encoder := json.NewEncoder(writeInput)
	scanner := bufio.NewScanner(readOutput)
	check(t, mcpResponse(t, encoder, scanner, 1, "initialize", map[string]any{"protocolVersion": "2025-06-18"})["result"] != nil, "MCP initialize failed")
	check(t, mcpResponse(t, encoder, scanner, 2, "tools/list", map[string]any{})["result"] != nil, "MCP tools/list failed")
	hello := <-hellos
	hello.ack <- false
	<-backend.done
	terminal := mcpResponse(t, encoder, scanner, 3, "tools/call", map[string]any{"name": "sessionbus", "arguments": map[string]any{"action": "list"}})
	failed, _ := terminal["result"].(map[string]any)
	check(t, failed["isError"] == true, "terminal tools/call = %#v", terminal)
	must(t, writeInput.Close())
	must(t, <-served)
	backend.Shutdown()
}

func TestPeerDeliveryOwnerShutdownCrossesBlockedInterject(t *testing.T) {
	root := testsocket.Directory(t)
	socket := filepath.Join(root, "sessionbus.sock")
	server, hellos := fakeDaemon(t, socket)
	defer server.Close()
	t.Setenv(host.SocketEnv, socket)
	recordPath := filepath.Join(root, "record")
	t.Setenv("GROK_TEST_RECORD", recordPath)
	t.Setenv("GROK_TEST_SESSION_ID", testSessionID)
	t.Setenv("GROK_TEST_INTERJECT_BLOCK", filepath.Join(root, "never"))
	observerPID := filepath.Join(root, "observer.pid")
	t.Setenv("GROK_TEST_OBSERVER_PID", observerPID)
	environment := setEnvironment(setEnvironment(os.Environ(), grokSessionIDEnv, testSessionID), grokLeaderSocketEnv, filepath.Join(root, "leader.sock"))
	environment = setEnvironment(environment, ManagedEnv, environmentValue(environment, grokLeaderSocketEnv))
	backend, err := NewPeerBackend(context.Background(), environment)
	must(t, err)
	backend.Initialized()
	hello := <-hellos
	hello.ack <- true
	<-backend.ready
	delivered := deliverPeer(backend, context.Background(), delivery("blocked"))
	waitFrame(t, recordPath, "_x.ai/interject", 1)
	pidfd := interactivePidfd(t, observerPID)
	defer closeProcessHandle(pidfd)
	closed := make(chan struct{})
	go func() { backend.Shutdown(); close(closed) }()
	<-closed
	result := <-delivered
	check(t, result.receipt.Disposition != "injected", "shutdown invented admission: %#v / %v", result.receipt, result.err)
	check(t, !processRunning(t, pidfd), "blocked observer survived helper shutdown")
}

func TestPeerDeliveryOwnerSerializesTwoReceipts(t *testing.T) {
	root := testsocket.Directory(t)
	socket := filepath.Join(root, "sessionbus.sock")
	server, hellos := fakeDaemon(t, socket)
	defer server.Close()
	t.Setenv(host.SocketEnv, socket)
	recordPath := filepath.Join(root, "record")
	t.Setenv("GROK_TEST_RECORD", recordPath)
	t.Setenv("GROK_TEST_SESSION_ID", testSessionID)
	cwd, err := os.Getwd()
	must(t, err)
	t.Setenv("GROK_TEST_CWD", cwd)
	environment := setEnvironment(setEnvironment(os.Environ(), grokSessionIDEnv, testSessionID), grokLeaderSocketEnv, filepath.Join(root, "leader.sock"))
	environment = setEnvironment(environment, ManagedEnv, environmentValue(environment, grokLeaderSocketEnv))
	backend, err := NewPeerBackend(context.Background(), environment)
	must(t, err)
	backend.Initialized()
	hello := <-hellos
	hello.ack <- true
	<-backend.ready
	results := make(chan peerDeliveryResult, 2)
	for _, message := range []string{"first", "second"} {
		go func(message string) {
			request := delivery(message)
			request.MessageID = message
			receipt, err := backend.deliver(context.Background(), sessionkit.PeerIdentity{SessionID: testSessionID}, request)
			results <- peerDeliveryResult{receipt: receipt, err: err}
		}(message)
	}
	for range 2 {
		result := <-results
		must(t, result.err)
		check(t, result.receipt.Disposition == "injected", "delivery receipt = %#v", result.receipt)
	}
	check(t, len(peerClientPIDs(t, records(t, recordPath))) == 1, "serialized deliveries opened more than one observer")
	check(t, countFrames(records(t, recordPath), "_x.ai/interject") == 2, "serialized deliveries lost an interject")
	backend.Shutdown()
}

func TestPeerHelperRequiresProductIdentity(t *testing.T) {
	_, err := NewPeerBackend(context.Background(), nil)
	check(t, err != nil && strings.Contains(err.Error(), "start Grok with grok-peer"), "missing identity = %v", err)
}

func TestInteractiveLauncherOwnsLeaderHoldAndTUI(t *testing.T) {
	root := testsocket.Directory(t)
	recordPath := filepath.Join(root, "record")
	socket := filepath.Join(root, "sessionbus.sock")
	t.Setenv(host.SocketEnv, socket)
	t.Setenv("GROK_TEST_RECORD", recordPath)
	started := filepath.Join(root, "interactive-started")
	leaderPID, holdPID, tuiPID := filepath.Join(root, "leader.pid"), filepath.Join(root, "hold.pid"), filepath.Join(root, "tui.pid")
	t.Setenv("GROK_TEST_INTERACTIVE_STARTED", started)
	t.Setenv("GROK_TEST_LEADER_PID", leaderPID)
	t.Setenv("GROK_TEST_OBSERVER_PID", holdPID)
	t.Setenv("GROK_TEST_INTERACTIVE_PID", tuiPID)
	plan, err := InteractivePlan([]string{"--session-id", testSessionID, "--group", "team", "--cwd", root, "--model", "--yolo"}, os.Environ())
	must(t, err)
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunInteractive(ctx, plan) }()
	<-fileReady(started)
	waitFrame(t, recordPath, "authenticate", 1)
	frames := records(t, recordPath)
	foundArgs := false
	for _, raw := range frames {
		var start struct {
			Kind  string `json:"kind"`
			Value struct {
				Arguments []string `json:"arguments"`
			} `json:"value"`
		}
		must(t, json.Unmarshal(raw, &start))
		if start.Kind == "START" && slices.Contains(start.Value.Arguments, "--leader") && !slices.Contains(start.Value.Arguments, "stdio") {
			index := slices.Index(start.Value.Arguments, "--model")
			check(t, index >= 0 && index+1 < len(start.Value.Arguments) && start.Value.Arguments[index+1] == "--yolo", "native model value rewritten: %q", start.Value.Arguments)
			check(t, !slices.Contains(start.Value.Arguments, "--always-approve"), "model value selected native bypass: %q", start.Value.Arguments)
			foundArgs = true
		}
	}
	check(t, foundArgs, "native interactive argv missing")
	clients := peerClientPIDs(t, frames)
	check(t, len(clients) == 1 && slices.Equal(peerClientMethods(frames, clients[0]), []string{"initialize", "authenticate"}), "startup hold was not the only quiet ACP client: %#v", frames)
	check(t, countFrames(frames, "_x.ai/sessions/list") == 0, "launcher queried the roster")
	check(t, containsStartEnv(frames, "leader", host.SocketEnv, socket) && containsStartEnv(frames, "leader", host.GroupsEnv, `["team"]`), "leader did not inherit helper bus identity")
	check(t, !containsStart(frames, "SESSIONBUS_LANE_SOCKET"), "interactive launcher published a private action endpoint")
	leaderPidfd, holdPidfd, tuiPidfd := interactivePidfd(t, leaderPID), interactivePidfd(t, holdPID), interactivePidfd(t, tuiPID)
	defer closeProcessHandle(leaderPidfd)
	defer closeProcessHandle(holdPidfd)
	defer closeProcessHandle(tuiPidfd)
	cancel(testSignal{syscall.SIGINT})
	var exited *exec.ExitError
	err = <-done
	check(t, errors.As(err, &exited) && exited.ProcessState.Sys().(syscall.WaitStatus).Signal() == syscall.SIGINT, "signalled Grok child = %v", err)
	check(t, !processRunning(t, leaderPidfd) && !processRunning(t, holdPidfd) && !processRunning(t, tuiPidfd), "interactive dependency survived shutdown")
	check(t, !exists(filepath.Join(root, "lanes", host.LaunchTokenDigest(testSessionID)+".sock")), "peer endpoint remains")
}

func TestStartupHoldExitStopsInteractiveOwner(t *testing.T) {
	root := testsocket.Directory(t)
	recordPath := filepath.Join(root, "record")
	t.Setenv(host.SocketEnv, filepath.Join(root, "sessionbus.sock"))
	t.Setenv("GROK_TEST_RECORD", recordPath)
	tuiPID, leaderPID, holdPID := filepath.Join(root, "tui.pid"), filepath.Join(root, "leader.pid"), filepath.Join(root, "hold.pid")
	release := filepath.Join(root, "release-hold")
	t.Setenv("GROK_TEST_INTERACTIVE_PID", tuiPID)
	t.Setenv("GROK_TEST_LEADER_PID", leaderPID)
	t.Setenv("GROK_TEST_OBSERVER_PID", holdPID)
	t.Setenv("GROK_TEST_HOLD_EXIT", release)
	plan, err := InteractivePlan([]string{"--session-id", testSessionID, "--cwd", root}, os.Environ())
	must(t, err)
	done := make(chan error, 1)
	go func() { done <- RunInteractive(context.Background(), plan) }()
	tuiPidfd, leaderPidfd, holdPidfd := interactivePidfd(t, tuiPID), interactivePidfd(t, leaderPID), interactivePidfd(t, holdPID)
	defer closeProcessHandle(tuiPidfd)
	defer closeProcessHandle(leaderPidfd)
	defer closeProcessHandle(holdPidfd)
	must(t, os.WriteFile(release, nil, 0o600))
	err = <-done
	check(t, strings.Contains(err.Error(), "Grok startup hold closed"), "launcher error = %v", err)
	check(t, !processRunning(t, tuiPidfd) && !processRunning(t, holdPidfd) && !processRunning(t, leaderPidfd), "bootstrap process survived startup-hold failure")
	clients := peerClientPIDs(t, records(t, recordPath))
	check(t, len(clients) == 1 && slices.Equal(peerClientMethods(records(t, recordPath), clients[0]), []string{"initialize", "authenticate"}), "startup hold was not quiet")
}

func TestInteractiveLauncherReturnsProductExit(t *testing.T) {
	root := testsocket.Directory(t)
	t.Setenv(host.SocketEnv, filepath.Join(root, "sessionbus.sock"))
	leaderPID, holdPID := filepath.Join(root, "leader.pid"), filepath.Join(root, "hold.pid")
	interactivePID, release := filepath.Join(root, "interactive.pid"), filepath.Join(root, "release")
	t.Setenv("GROK_TEST_LEADER_PID", leaderPID)
	t.Setenv("GROK_TEST_OBSERVER_PID", holdPID)
	t.Setenv("GROK_TEST_INTERACTIVE_PID", interactivePID)
	t.Setenv("GROK_TEST_INTERACTIVE_EXIT", "7")
	t.Setenv("GROK_TEST_INTERACTIVE_EXIT_BARRIER", release)
	plan, err := InteractivePlan([]string{"--session-id", testSessionID, "--cwd", root}, os.Environ())
	must(t, err)
	done := make(chan error, 1)
	go func() { done <- RunInteractive(context.Background(), plan) }()
	leaderPidfd, holdPidfd := interactivePidfd(t, leaderPID), interactivePidfd(t, holdPID)
	must(t, os.WriteFile(release, nil, 0o600))
	err = <-done
	var exited *exec.ExitError
	check(t, errors.As(err, &exited) && exited.ExitCode() == 7, "exit = %v", err)
	check(t, !processRunning(t, leaderPidfd) && !processRunning(t, holdPidfd), "Grok dependencies survived the TUI")
	closeProcessHandle(leaderPidfd)
	closeProcessHandle(holdPidfd)
}

func TestLeaderCreatesDefaultStateRoot(t *testing.T) {
	root := filepath.Join(testsocket.Directory(t), "state")
	t.Setenv(host.SocketEnv, "")
	t.Setenv("XDG_RUNTIME_DIR", root)
	socket := sessionkit.Socket()
	check(t, !exists(filepath.Dir(socket)), "default run directory already exists")
	leader, err := startLeader(context.Background(), socket, host.LaunchTokenDigest(testSessionID), t.TempDir(), "default", os.Environ())
	must(t, err)
	check(t, grokSocketReady(leaderSocket(socket, host.LaunchTokenDigest(testSessionID))), "leader socket was not created")
	must(t, closeNative("leader", leader))
}

func TestExactRosterAuthority(t *testing.T) {
	yolo, resident := true, true
	live := peerSession{SessionID: testSessionID, Resident: &resident, Activity: "idle", Yolo: &yolo}
	for _, test := range []struct {
		name     string
		sessions []peerSession
		noLeader bool
	}{
		{"absent", nil, true},
		{"duplicate", []peerSession{live, live}, false},
		{"missing resident", []peerSession{{SessionID: testSessionID, Activity: "idle", Yolo: &yolo}}, false},
		{"not resident", []peerSession{{SessionID: testSessionID, Resident: new(bool), Activity: "idle", Yolo: &yolo}}, false},
		{"missing yolo", []peerSession{{SessionID: testSessionID, Resident: &resident, Activity: "idle"}}, false},
		{"unknown activity", []peerSession{{SessionID: testSessionID, Resident: &resident, Activity: "starting", Yolo: &yolo}}, false},
	} {
		_, err := exactRoster(test.sessions, testSessionID)
		check(t, err != nil && errors.Is(err, errNoLeader) == test.noLeader, "%s = %v", test.name, err)
	}
	_, err := exactRoster([]peerSession{live}, testSessionID)
	must(t, err)
}

func TestPeerShutdownKillsItsObserverProcessGroup(t *testing.T) {
	root := testsocket.Directory(t)
	socket := filepath.Join(root, "sessionbus.sock")
	server, hellos := fakeDaemon(t, socket)
	defer server.Close()
	t.Setenv(host.SocketEnv, socket)
	t.Setenv("GROK_TEST_SESSION_ID", testSessionID)
	t.Setenv("GROK_TEST_DESCENDANT_PID", filepath.Join(root, "descendant.pid"))
	cwd, err := os.Getwd()
	must(t, err)
	t.Setenv("GROK_TEST_CWD", cwd)
	environment := setEnvironment(setEnvironment(os.Environ(), grokSessionIDEnv, testSessionID), grokLeaderSocketEnv, filepath.Join(root, "leader.sock"))
	environment = setEnvironment(environment, ManagedEnv, environmentValue(environment, grokLeaderSocketEnv))
	opened := make(chan *PeerBackend, 1)
	go func() {
		backend, _ := NewPeerBackend(context.Background(), environment)
		backend.Initialized()
		opened <- backend
	}()
	hello := <-hellos
	hello.ack <- true
	backend := <-opened
	receipt, err := backend.deliver(context.Background(), sessionkit.PeerIdentity{SessionID: testSessionID}, delivery("peer message"))
	must(t, err)
	check(t, receipt.Disposition == "injected", "delivery = %#v", receipt)
	body, err := os.ReadFile(filepath.Join(root, "descendant.pid"))
	must(t, err)
	var pid int
	_, err = fmt.Sscan(string(body), &pid)
	must(t, err)
	pidfd := pidfd(t, pid)
	defer closeProcessHandle(pidfd)
	backend.Shutdown()
	waitProcessExit(t, pidfd)
	check(t, !processRunning(t, pidfd), "observer descendant survived shutdown")
}

type hello struct {
	SessionID string         `json:"session_id"`
	Name      string         `json:"name"`
	Product   string         `json:"product"`
	Groups    []string       `json:"groups"`
	Info      map[string]any `json:"info"`
	ack       chan bool
	done      chan struct{}
}

type testSignal struct{ os.Signal }

func (s testSignal) Error() string           { return s.String() }
func (s testSignal) CaughtSignal() os.Signal { return s.Signal }

func fakeDaemon(t *testing.T, path string) (net.Listener, <-chan hello) {
	t.Helper()
	listener, err := net.Listen("unix", path)
	must(t, err)
	hellos := make(chan hello, 4)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		scanner := bufio.NewScanner(connection)
		var write sync.Mutex
		var admitted hello
		for scanner.Scan() {
			var frame struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(scanner.Bytes(), &frame) != nil {
				return
			}
			if frame.Method == "session.hello" {
				var value hello
				_ = json.Unmarshal(frame.Params, &value)
				value.ack = make(chan bool, 1)
				value.done = make(chan struct{})
				hellos <- value
				go func(id int64, value hello) {
					defer close(value.done)
					accepted := <-value.ack
					write.Lock()
					if accepted {
						admitted = value
						_ = json.NewEncoder(connection).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
					} else {
						_ = json.NewEncoder(connection).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32602, "message": "invalid_hello"}})
					}
					write.Unlock()
				}(frame.ID, value)
				continue
			}
			if frame.Method == "session.list" {
				write.Lock()
				row := sessionkit.SessionSummary{SessionID: admitted.SessionID + "@local", Kind: "peer", Product: admitted.Product, Name: admitted.Name + "@local", Groups: admitted.Groups, Connected: true, Info: admitted.Info}
				_ = json.NewEncoder(connection).Encode(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "result": sessionkit.SessionListResult{Sessions: []sessionkit.SessionSummary{row}}})
				write.Unlock()
				continue
			}
			if frame.Method != "" {
				write.Lock()
				_ = json.NewEncoder(connection).Encode(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "result": map[string]any{}})
				write.Unlock()
			}
		}
	}()
	return listener, hellos
}

func deliverPeer(backend *PeerBackend, ctx context.Context, request sessionkit.DeliveryRequest) <-chan peerDeliveryResult {
	result := make(chan peerDeliveryResult, 1)
	go func() {
		receipt, err := backend.deliver(ctx, sessionkit.PeerIdentity{SessionID: testSessionID}, request)
		result <- peerDeliveryResult{receipt: receipt, err: err}
	}()
	return result
}

func mcpResponse(t *testing.T, encoder *json.Encoder, scanner *bufio.Scanner, id int, method string, params any) map[string]any {
	t.Helper()
	must(t, encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}))
	check(t, scanner.Scan(), "MCP response absent: %v", scanner.Err())
	var response map[string]any
	must(t, json.Unmarshal(scanner.Bytes(), &response))
	check(t, response["id"] == float64(id), "MCP response id = %#v", response)
	return response
}

func interactivePidfd(t *testing.T, path string) processHandle {
	t.Helper()
	<-fileReady(path)
	body, err := os.ReadFile(path)
	must(t, err)
	var pid int
	_, err = fmt.Sscan(string(body), &pid)
	must(t, err)
	return pidfd(t, pid)
}

func peerClientPIDs(t *testing.T, rows []json.RawMessage) []int {
	t.Helper()
	var result []int
	for _, raw := range rows {
		var record struct {
			Kind  string `json:"kind"`
			Value struct {
				PID       int      `json:"pid"`
				Arguments []string `json:"arguments"`
			} `json:"value"`
		}
		must(t, json.Unmarshal(raw, &record))
		if record.Kind == "START" && slices.Contains(record.Value.Arguments, "--leader") && slices.Contains(record.Value.Arguments, "stdio") {
			result = append(result, record.Value.PID)
		}
	}
	return result
}

func peerClientMethods(rows []json.RawMessage, pid int) []string {
	var result []string
	for _, raw := range rows {
		var record struct {
			Kind  string `json:"kind"`
			Value struct {
				PID    int    `json:"_testPID"`
				Method string `json:"method"`
			} `json:"value"`
		}
		if json.Unmarshal(raw, &record) == nil && record.Kind == "FRAME" && record.Value.PID == pid {
			result = append(result, record.Value.Method)
		}
	}
	return result
}

func containsStartEnv(rows []json.RawMessage, argument, name, value string) bool {
	for _, raw := range rows {
		var record struct {
			Kind  string `json:"kind"`
			Value struct {
				Arguments   []string          `json:"arguments"`
				Environment map[string]string `json:"environment"`
			} `json:"value"`
		}
		if json.Unmarshal(raw, &record) == nil && record.Kind == "START" && slices.Contains(record.Value.Arguments, argument) && record.Value.Environment[name] == value {
			return true
		}
	}
	return false
}

func environment(values []string, name string) string {
	for _, value := range values {
		if key, body, found := strings.Cut(value, "="); found && key == name {
			return body
		}
	}
	return ""
}

func TestResumingSameNativeIDUsesNewLaunchResources(t *testing.T) {
	root := testsocket.Directory(t)
	recordPath := filepath.Join(root, "record")
	t.Setenv(host.SocketEnv, filepath.Join(root, "bus"))
	t.Setenv("GROK_TEST_RECORD", recordPath)
	t.Setenv("GROK_TEST_INTERACTIVE_EXIT", "7")
	for range 2 {
		plan, err := InteractivePlan([]string{"--resume", testSessionID, "-n", "initial"}, os.Environ())
		must(t, err)
		var exited *exec.ExitError
		check(t, errors.As(RunInteractive(context.Background(), plan), &exited) && exited.ExitCode() == 7, "native exit was not retained")
	}
	paths := []string{}
	for _, raw := range records(t, recordPath) {
		var record struct {
			Kind  string `json:"kind"`
			Value struct {
				Arguments []string `json:"arguments"`
			} `json:"value"`
		}
		must(t, json.Unmarshal(raw, &record))
		if record.Kind != "START" || !slices.Contains(record.Value.Arguments, "leader") {
			continue
		}
		paths = append(paths, nativeOption(record.Value.Arguments, "--leader-socket"))
	}
	check(t, len(paths) == 2 && paths[0] != paths[1], "same native ID reused launch identity: %#v", paths)
	for _, path := range paths {
		check(t, !exists(filepath.Dir(path)), "launch directory remains: %s", path)
	}
}
