// SPDX-License-Identifier: MIT

package omp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/pifamily"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

const (
	ompNativeOwnerHelperEnv   = "OMP_SESSIONBUS_NATIVE_OWNER_HELPER"
	ompNativeOwnerScenarioEnv = "OMP_SESSIONBUS_NATIVE_OWNER_SCENARIO"
	ompNativeOwnerCaptureEnv  = "OMP_SESSIONBUS_NATIVE_OWNER_CAPTURE"
	ompNativeOwnerSID         = "omp-native-session-1"
	ompNativeOwnerToken       = "omp-owner-token-1"
	ompNativeOwnerName        = "OMP native fixture"
	ompNativeOwnerReadyFrame  = `{"type":"ready","protocolVersion":1,"supportedProtocolVersions":[1,2],"maxFrameBytes":1048576,"maxReassembledFrameBytes":67108864}`
)

type ompNativeOwnerCapture struct {
	Launch ompLaunch `json:"launch"`
	Args   []string  `json:"args"`
}

type ompNativeOwnerHelper struct {
	launch   ompLaunch
	cwd      string
	scenario string
	shutdown chan struct{}
	state    chan struct{}
	once     sync.Once
}

func TestOMPNativeOwnerHelper(t *testing.T) {
	if os.Getenv(ompNativeOwnerHelperEnv) == "" {
		return
	}
	if err := runOMPNativeOwnerHelper(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(17)
	}
	os.Exit(0)
}

func runOMPNativeOwnerHelper() error {
	var launch ompLaunch
	if err := json.Unmarshal([]byte(os.Getenv(launchEnvironmentName)), &launch); err != nil {
		return err
	}
	if launch.OwnerPID != os.Getppid() || (launch.Topology != ownerTopologyLane && launch.Topology != ownerTopologyInteractive) {
		return errors.New("invalid native owner launch descriptor")
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		return errors.New("native owner helper arguments are missing")
	}
	capture := ompNativeOwnerCapture{Launch: launch, Args: slices.Clone(os.Args[separator+1:])}
	body, err := json.Marshal(capture)
	if err != nil {
		return err
	}
	if err = os.WriteFile(os.Getenv(ompNativeOwnerCaptureEnv), body, 0o600); err != nil {
		return err
	}
	if _, err = fmt.Fprintln(os.Stdout, ompNativeOwnerReadyFrame); err != nil {
		return err
	}
	connection, err := net.Dial("unix", launch.Socket)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		_ = connection.Close()
		return err
	}
	helper := &ompNativeOwnerHelper{
		launch: launch, cwd: cwd, scenario: os.Getenv(ompNativeOwnerScenarioEnv),
		shutdown: make(chan struct{}), state: make(chan struct{}),
	}
	bridge, err := pifamily.NewBridge(connection, pifamily.BridgeNative, helper.handleBridge, pifamily.BridgeLimits{})
	if err != nil {
		return err
	}
	if err = bridge.Ready(context.Background()); err != nil {
		return err
	}
	if helper.scenario == "hold_owner_ready" {
		<-bridge.Done()
		return bridge.Err()
	}
	mode := ownerModeRPC
	if launch.Topology == ownerTopologyInteractive {
		mode = ownerModeTUI
	}
	if err = bridge.Call(context.Background(), "owner.ready", ownerReadyRequest{
		Topology: launch.Topology, Directory: launch.Directory, Scope: ownerScopePrimary,
		Mode: mode, OwnerToken: ompNativeOwnerToken, SessionID: ompNativeOwnerSID, Name: ompNativeOwnerName,
	}, nil); err != nil {
		return err
	}
	commandsDone := make(chan error, 1)
	go func() { commandsDone <- helper.serveCommands() }()
	if helper.scenario == "wrong_state" {
		<-bridge.Done()
		return bridge.Err()
	}
	<-helper.state
	if helper.scenario == "exit_after_ready" {
		os.Exit(23)
	}
	<-helper.shutdown
	if helper.scenario == "hold_shutdown" {
		select {}
	}
	if helper.scenario == "malformed_shutdown" {
		_, _ = fmt.Fprintln(os.Stdout, "{")
	}
	if err = bridge.Call(context.Background(), "session_end", ownerEndRequest{
		Topology: launch.Topology, Scope: ownerScopePrimary, Mode: mode,
		OwnerToken: ompNativeOwnerToken, SessionID: ompNativeOwnerSID, Reason: "quit",
	}, nil); err != nil {
		return err
	}
	if err = bridge.Close(); err != nil && !errors.Is(err, pifamily.ErrBridgeClosed) {
		return err
	}
	if err = <-commandsDone; err != nil {
		return err
	}
	return nil
}

func (helper *ompNativeOwnerHelper) handleBridge(_ context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "native.describe":
		var request ownerDescribeRequest
		if decodeOwnerJSON(raw, &request) != nil || request.OwnerToken != ompNativeOwnerToken || request.SessionID != ompNativeOwnerSID {
			return nil, pifamily.NewBridgeCallError("bad_request", "invalid native description")
		}
		return json.Marshal(ownerDescribeResult{
			OwnerToken: ompNativeOwnerToken, SessionID: ompNativeOwnerSID,
			Name: ompNativeOwnerName, CWD: helper.cwd,
		})
	case "native.shutdown":
		var request ownerDescribeRequest
		if decodeOwnerJSON(raw, &request) != nil || request.OwnerToken != ompNativeOwnerToken || request.SessionID != ompNativeOwnerSID {
			return nil, pifamily.NewBridgeCallError("bad_request", "invalid native shutdown")
		}
		helper.once.Do(func() { close(helper.shutdown) })
		return json.Marshal(ownerShutdownResult{
			OwnerToken: ompNativeOwnerToken, SessionID: ompNativeOwnerSID, Requested: true,
		})
	default:
		return nil, pifamily.NewBridgeCallError("method_not_found", "unexpected native owner method")
	}
}

func (helper *ompNativeOwnerHelper) serveCommands() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), nativeRPCPhysicalFrameBytes)
	for scanner.Scan() {
		var command map[string]json.RawMessage
		if json.Unmarshal(scanner.Bytes(), &command) != nil {
			return errors.New("invalid native owner command")
		}
		var id, kind string
		if json.Unmarshal(command["id"], &id) != nil || json.Unmarshal(command["type"], &kind) != nil {
			return errors.New("invalid native owner command envelope")
		}
		var data any
		switch kind {
		case "negotiate_protocol":
			data = map[string]int{"protocolVersion": 2}
		case "get_state":
			sessionID := ompNativeOwnerSID
			if helper.scenario == "wrong_state" {
				sessionID = "wrong-native-session"
			}
			data = map[string]any{
				"sessionId": sessionID, "sessionName": ompNativeOwnerName,
				"isStreaming": false, "isCompacting": false, "queuedMessageCount": 0,
			}
		default:
			return fmt.Errorf("unexpected native owner command %q", kind)
		}
		response, err := json.Marshal(map[string]any{
			"id": id, "type": "response", "command": kind, "success": true, "data": data,
		})
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintln(os.Stdout, string(response)); err != nil {
			return err
		}
		if kind == "get_state" {
			close(helper.state)
		}
	}
	return scanner.Err()
}

func ompNativeOwnerHelperCommand(name string, arguments ...string) *exec.Cmd {
	owned := []string{"-test.run=^TestOMPNativeOwnerHelper$", "--", name}
	owned = append(owned, arguments...)
	return exec.Command(os.Args[0], owned...)
}

func nativeOwnerTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func nativeOwnerFixture(t *testing.T, scenario string) (NativeOwnerOptions, string) {
	t.Helper()
	directory := t.TempDir()
	extension := filepath.Join(directory, "extension.mjs")
	if err := os.WriteFile(extension, []byte("export default {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(directory, "capture.json")
	t.Setenv(ompNativeOwnerHelperEnv, "1")
	t.Setenv(ompNativeOwnerScenarioEnv, scenario)
	t.Setenv(ompNativeOwnerCaptureEnv, capture)
	previous := ompCommand
	ompCommand = ompNativeOwnerHelperCommand
	t.Cleanup(func() { ompCommand = previous })
	return NativeOwnerOptions{
		DaemonSocket: filepath.Join(directory, "daemon.sock"), Provisional: "provisional",
		CWD: directory, Topology: ownerTopologyLane, InitialName: "fixture@local",
		Extension: extension, PrimaryCaller: &kit.Caller{},
		Native: NativeExecutable{
			RuntimePath: filepath.Join(directory, "bun"),
			EntryPath:   filepath.Join(directory, "package", "dist", "cli.js"),
		},
		Arguments: []string{"--model", "fixture", "--", "literal"},
	}, capture
}

func waitNativeOwnerReady(t *testing.T, owner *NativeOwner) {
	t.Helper()
	select {
	case <-owner.Ready():
	case <-owner.Done():
		t.Fatalf("OMP native owner ended before ready: %v", owner.Err())
	case <-nativeOwnerTestContext(t).Done():
		t.Fatal("OMP native owner readiness timed out")
	}
}

func TestNativeOwnerJoinsStartupShutdownAndOwnedResources(t *testing.T) {
	options, capturePath := nativeOwnerFixture(t, "")
	owner, err := StartNativeOwner(nativeOwnerTestContext(t), options)
	if err != nil {
		t.Fatal(err)
	}
	waitNativeOwnerReady(t, owner)
	binding, ok := owner.Primary()
	if !ok || binding.SessionID != ompNativeOwnerSID || binding.OwnerToken != ompNativeOwnerToken ||
		binding.Name != ompNativeOwnerName || binding.CWD != options.CWD || binding.Scope != ownerScopePrimary || binding.Mode != ownerModeRPC {
		t.Fatalf("primary = %+v, %v", binding, ok)
	}
	closeResults := make(chan error, 2)
	closeContext := nativeOwnerTestContext(t)
	go func() { closeResults <- owner.Close(closeContext) }()
	go func() { closeResults <- owner.Close(closeContext) }()
	for range 2 {
		if err = <-closeResults; err != nil {
			t.Fatal(err)
		}
	}
	if reason, ok := owner.GracefulEnd(); !ok || reason != "quit" {
		t.Fatalf("graceful end = %q, %v", reason, ok)
	}
	var capture ompNativeOwnerCapture
	body, err := os.ReadFile(capturePath)
	if err != nil || json.Unmarshal(body, &capture) != nil {
		t.Fatalf("capture = %q, %v", body, err)
	}
	want := []string{
		options.Native.RuntimePath, options.Native.EntryPath,
		"--extension", options.Extension, "--mode", "rpc-ui", "--allow-home",
		"--model", "fixture", "--", "literal",
	}
	if !slices.Equal(capture.Args, want) {
		t.Fatalf("native argv = %#v, want %#v", capture.Args, want)
	}
	if capture.Launch.Topology != ownerTopologyLane || capture.Launch.Directory == "" || capture.Launch.Socket == "" {
		t.Fatalf("launch = %+v", capture.Launch)
	}
	if _, err = os.Stat(capture.Launch.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private directory survived Close: %v", err)
	}
	lock, err := host.AcquireSessionLock(options.DaemonSocket, "omp", ompNativeOwnerSID)
	if err != nil {
		t.Fatalf("session lock remained owned after Close: %v", err)
	}
	_ = lock.Close()
}

func TestNativeOwnerCloseCancelsHeldStartupAndJoins(t *testing.T) {
	options, capturePath := nativeOwnerFixture(t, "hold_owner_ready")
	owner, err := StartNativeOwner(nativeOwnerTestContext(t), options)
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err = os.Stat(capturePath); err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-owner.Done():
			t.Fatalf("owner ended before held startup: %v", owner.Err())
		default:
		}
	}
	err = owner.Close(nativeOwnerTestContext(t))
	if err == nil {
		t.Fatal("held startup Close unexpectedly succeeded")
	}
	select {
	case <-owner.Done():
	default:
		t.Fatal("held startup Close did not join owner")
	}
	var capture ompNativeOwnerCapture
	body, readErr := os.ReadFile(capturePath)
	if readErr != nil || json.Unmarshal(body, &capture) != nil {
		t.Fatalf("capture = %q, %v", body, readErr)
	}
	if _, statErr := os.Stat(capture.Launch.Directory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("held startup directory survived: %v", statErr)
	}
}

func TestNativeOwnerRejectsContradictoryStateAndJoins(t *testing.T) {
	options, capturePath := nativeOwnerFixture(t, "wrong_state")
	owner, err := StartNativeOwner(nativeOwnerTestContext(t), options)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-owner.Ready():
		t.Fatal("contradictory native state reached readiness")
	case <-owner.Done():
	case <-nativeOwnerTestContext(t).Done():
		t.Fatal("contradictory native state did not retire")
	}
	if owner.Err() == nil {
		t.Fatal("contradictory native state was not retained")
	}
	var capture ompNativeOwnerCapture
	body, readErr := os.ReadFile(capturePath)
	if readErr != nil || json.Unmarshal(body, &capture) != nil {
		t.Fatalf("capture = %q, %v", body, readErr)
	}
	if _, statErr := os.Stat(capture.Launch.Directory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed startup directory survived: %v", statErr)
	}
}

func TestNativeOwnerRetiresUnexpectedChildExit(t *testing.T) {
	options, _ := nativeOwnerFixture(t, "exit_after_ready")
	owner, err := StartNativeOwner(nativeOwnerTestContext(t), options)
	if err != nil {
		t.Fatal(err)
	}
	waitNativeOwnerReady(t, owner)
	select {
	case <-owner.Done():
	case <-nativeOwnerTestContext(t).Done():
		t.Fatal("unexpected child exit did not retire owner")
	}
	if owner.Err() == nil {
		t.Fatal("unexpected child exit was not retained")
	}
}

func TestNativeOwnerParentCancellationForcesAndJoins(t *testing.T) {
	options, capturePath := nativeOwnerFixture(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	owner, err := StartNativeOwner(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	waitNativeOwnerReady(t, owner)
	cancel()
	select {
	case <-owner.Done():
	case <-nativeOwnerTestContext(t).Done():
		t.Fatal("parent cancellation did not join owner")
	}
	if !errors.Is(owner.Err(), context.Canceled) {
		t.Fatalf("parent cancellation = %v", owner.Err())
	}
	var capture ompNativeOwnerCapture
	body, readErr := os.ReadFile(capturePath)
	if readErr != nil || json.Unmarshal(body, &capture) != nil {
		t.Fatalf("capture = %q, %v", body, readErr)
	}
	if _, statErr := os.Stat(capture.Launch.Directory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("canceled owner directory survived: %v", statErr)
	}
}

func TestNativeOwnerCloseCancellationForcesAndJoinsChild(t *testing.T) {
	options, capturePath := nativeOwnerFixture(t, "hold_shutdown")
	owner, err := StartNativeOwner(nativeOwnerTestContext(t), options)
	if err != nil {
		t.Fatal(err)
	}
	waitNativeOwnerReady(t, owner)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = owner.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("forced Close = %v", err)
	}
	var capture ompNativeOwnerCapture
	body, readErr := os.ReadFile(capturePath)
	if readErr != nil || json.Unmarshal(body, &capture) != nil {
		t.Fatalf("capture = %q, %v", body, readErr)
	}
	if _, statErr := os.Stat(capture.Launch.Directory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("forced Close directory survived: %v", statErr)
	}
}

func TestNativeOwnerPreservesClosingProtocolFailure(t *testing.T) {
	options, _ := nativeOwnerFixture(t, "malformed_shutdown")
	owner, err := StartNativeOwner(nativeOwnerTestContext(t), options)
	if err != nil {
		t.Fatal(err)
	}
	waitNativeOwnerReady(t, owner)
	err = owner.Close(nativeOwnerTestContext(t))
	if !errors.Is(err, errNativeRPCProtocol) {
		t.Fatalf("closing protocol failure = %v", err)
	}
	if reason, ok := owner.GracefulEnd(); !ok || reason != "quit" {
		t.Fatalf("graceful native end was lost: %q, %v", reason, ok)
	}
}

func TestDecodeNativeOwnerStateRequiresTypedReadiness(t *testing.T) {
	for name, body := range map[string]string{
		"missing id":       `{"isStreaming":false,"isCompacting":false,"queuedMessageCount":0}`,
		"null streaming":   `{"sessionId":"id","isStreaming":null,"isCompacting":false,"queuedMessageCount":0}`,
		"missing compact":  `{"sessionId":"id","isStreaming":false,"queuedMessageCount":0}`,
		"missing queue":    `{"sessionId":"id","isStreaming":false,"isCompacting":false}`,
		"negative queue":   `{"sessionId":"id","isStreaming":false,"isCompacting":false,"queuedMessageCount":-1}`,
		"wrong session id": `{"sessionId":"bad id","isStreaming":false,"isCompacting":false,"queuedMessageCount":0}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeNativeOwnerState(json.RawMessage(body)); err == nil {
				t.Fatal("invalid native state was accepted")
			}
		})
	}
}

func TestNativeOwnerRejectsNonphysicalExtension(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.mjs")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	extension := filepath.Join(directory, "extension.mjs")
	if err := os.Symlink(target, extension); err != nil {
		t.Fatal(err)
	}
	err := validateNativeOwnerOptions(NativeOwnerOptions{
		DaemonSocket: filepath.Join(directory, "daemon.sock"), Provisional: "provisional",
		CWD: directory, Topology: ownerTopologyLane, Extension: extension,
		PrimaryCaller: &kit.Caller{},
	})
	if err == nil {
		t.Fatal("symlinked managed extension was accepted")
	}
}
