// SPDX-License-Identifier: MIT

package omp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antst/sessionbus-peers/internal/testsocket"
	"github.com/antst/sessionbus-peers/wrappers/pifamily"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

type ownerNativeFixture struct {
	mu           sync.Mutex
	descriptions map[string]ownerDescribeResult
	stages       []ownerStageRequest
	shutdowns    []ownerDescribeRequest
	stageHook    func(ownerStageRequest)
	stageResult  func(ownerStageRequest) ownerStageResult
	stageRaw     func(ownerStageRequest) json.RawMessage
}

func (fixture *ownerNativeFixture) handle(_ context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	switch method {
	case "native.describe":
		var request ownerDescribeRequest
		if decodeOwnerJSON(raw, &request) != nil {
			return nil, pifamily.NewBridgeCallError("bad_request", "invalid native description")
		}
		result, ok := fixture.descriptions[request.OwnerToken]
		if !ok || result.SessionID != request.SessionID {
			return nil, pifamily.NewBridgeCallError("bad_request", "unknown native description")
		}
		return json.Marshal(result)
	case "native.stage":
		var request ownerStageRequest
		if decodeOwnerJSON(raw, &request) != nil {
			return nil, pifamily.NewBridgeCallError("bad_request", "invalid native stage")
		}
		fixture.stages = append(fixture.stages, request)
		if fixture.stageHook != nil {
			fixture.stageHook(request)
		}
		if fixture.stageRaw != nil {
			return fixture.stageRaw(request), nil
		}
		if fixture.stageResult != nil {
			return json.Marshal(fixture.stageResult(request))
		}
		queued := true
		return json.Marshal(ownerStageResult{
			OwnerToken: request.OwnerToken, SessionID: request.SessionID,
			MessageID: request.MessageID, Queued: &queued,
		})
	case "native.shutdown":
		var request ownerDescribeRequest
		if decodeOwnerJSON(raw, &request) != nil {
			return nil, pifamily.NewBridgeCallError("bad_request", "invalid native shutdown")
		}
		fixture.shutdowns = append(fixture.shutdowns, request)
		return json.Marshal(ownerShutdownResult{OwnerToken: request.OwnerToken, SessionID: request.SessionID, Requested: true})
	default:
		return nil, pifamily.NewBridgeCallError("method_not_found", "unexpected native method")
	}
}

func ownerTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func ownerBusListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("unix", filepath.Join(testsocket.Directory(t), "bus.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func ownerAccept(t *testing.T, listener net.Listener) (net.Conn, *bufio.Scanner) {
	t.Helper()
	if unix, ok := listener.(*net.UnixListener); ok {
		_ = unix.SetDeadline(time.Now().Add(5 * time.Second))
	}
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if unix, ok := listener.(*net.UnixListener); ok {
		_ = unix.SetDeadline(time.Time{})
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, bufio.NewScanner(conn)
}

func ownerFrame(t *testing.T, scanner *bufio.Scanner) protocol.Frame {
	t.Helper()
	if !scanner.Scan() {
		t.Fatalf("missing public frame: %v", scanner.Err())
	}
	frame, err := protocol.DecodeFrame(scanner.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func ownerWrite(t *testing.T, conn net.Conn, body []byte) {
	t.Helper()
	if _, err := conn.Write(body); err != nil {
		t.Fatal(err)
	}
}

func ownerHello(t *testing.T, conn net.Conn, scanner *bufio.Scanner) kit.PeerIdentity {
	t.Helper()
	frame := ownerFrame(t, scanner)
	if !frame.Request || frame.Method != "session.hello" {
		t.Fatalf("public hello = %+v", frame)
	}
	params, err := protocol.DecodeParams(frame.Method, frame.Params)
	if err != nil {
		t.Fatal(err)
	}
	hello, ok := params.(*protocol.PeerHello)
	if !ok {
		t.Fatalf("public hello params = %T", params)
	}
	body, err := protocol.ResultBytes(frame.ID, frame.Method, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	ownerWrite(t, conn, body)
	return *hello
}

func ownerRegistryPair(t *testing.T, options OwnerRegistryOptions, fixture *ownerNativeFixture) (*OwnerRegistry, *pifamily.Bridge) {
	t.Helper()
	registry, err := NewOwnerRegistry(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	assigned := make(chan struct{})
	hostBridge, err := pifamily.NewBridge(left, pifamily.BridgeHost, func(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
		<-assigned
		return registry.HandleBridge(ctx, method, raw)
	}, pifamily.BridgeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if err = registry.AssignBridge(hostBridge); err != nil {
		t.Fatal(err)
	}
	close(assigned)
	nativeBridge, err := pifamily.NewBridge(right, pifamily.BridgeNative, fixture.handle, pifamily.BridgeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if err = hostBridge.Ready(ownerTestContext(t)); err != nil {
		t.Fatal(err)
	}
	if err = nativeBridge.Ready(ownerTestContext(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = registry.Close()
		_ = nativeBridge.Close()
	})
	return registry, nativeBridge
}

func ownerReady(t *testing.T, registry *OwnerRegistry, native *pifamily.Bridge, listener net.Listener, request ownerReadyRequest) (kit.PeerIdentity, net.Conn, *bufio.Scanner) {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- native.Call(ownerTestContext(t), "owner.ready", request, nil) }()
	conn, scanner := ownerAccept(t, listener)
	if !scanner.Scan() {
		t.Fatalf("missing public hello: %v; owner.ready: %v; registry: %v", scanner.Err(), <-result, registry.Err())
	}
	frame, err := protocol.DecodeFrame(scanner.Bytes())
	if err != nil || !frame.Request || frame.Method != "session.hello" {
		t.Fatalf("public hello frame = %+v, %v", frame, err)
	}
	params, err := protocol.DecodeParams(frame.Method, frame.Params)
	if err != nil {
		t.Fatal(err)
	}
	hello, ok := params.(*protocol.PeerHello)
	if !ok {
		t.Fatalf("public hello params = %T", params)
	}
	body, err := protocol.ResultBytes(frame.ID, frame.Method, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	ownerWrite(t, conn, body)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	return *hello, conn, scanner
}

func TestOwnerRegistryRejectsInvalidConstruction(t *testing.T) {
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return nil, nil })
	for name, options := range map[string]OwnerRegistryOptions{
		"nil context":         {Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus"), Directory: directory, PrimaryCaller: caller},
		"wrong topology":      {Topology: "print", Socket: filepath.Join(directory, "bus"), Directory: directory},
		"relative socket":     {Topology: ownerTopologyInteractive, Socket: "bus", Directory: directory},
		"lane without caller": {Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus"), Directory: directory},
		"interactive caller":  {Topology: ownerTopologyInteractive, Socket: filepath.Join(directory, "bus"), Directory: directory, PrimaryCaller: caller},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if name == "nil context" {
				ctx = nil
			}
			if registry, err := NewOwnerRegistry(ctx, options); err == nil || registry != nil {
				t.Fatalf("invalid registry = %#v, %v", registry, err)
			}
		})
	}
}

func TestOwnerRegistryKeepsLanePrimaryAndChildCallersDistinct(t *testing.T) {
	listener := ownerBusListener(t)
	directory := t.TempDir()
	type publicCall struct {
		method string
		params any
	}
	primaryCalls := make(chan publicCall, 1)
	primaryCaller := kit.NewCaller(func(_ context.Context, method string, params any) (json.RawMessage, error) {
		primaryCalls <- publicCall{method, params}
		return json.RawMessage(`{"sessions":[]}`), nil
	})
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token":  {OwnerToken: "main-token", SessionID: "main-session", Name: "", CWD: "/work/main"},
		"child-token": {OwnerToken: "child-token", SessionID: "child-session", Name: "child", CWD: "/work/child"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: listener.Addr().String(), Directory: directory,
		InitialName: "requested", Groups: []string{"shared"}, PrimaryCaller: primaryCaller,
	}, fixture)

	var readyResult struct {
		OwnerToken string `json:"owner_token"`
		SessionID  string `json:"session_id"`
	}
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary,
		Mode: ownerModeRPC, OwnerToken: "main-token", SessionID: "main-session",
	}, &readyResult); err != nil {
		t.Fatal(err)
	}
	if readyResult.OwnerToken != "main-token" || readyResult.SessionID != "main-session" {
		t.Fatalf("primary ready = %+v", readyResult)
	}
	select {
	case <-registry.Ready():
	case <-ownerTestContext(t).Done():
		t.Fatal("lane primary did not become ready")
	}
	primary, ok := registry.Primary()
	if !ok || primary.Name != "" || primary.CWD != "/work/main" || primary.Scope != ownerScopePrimary {
		t.Fatalf("primary binding = %+v, %v", primary, ok)
	}

	var toolResult struct {
		OwnerToken string          `json:"owner_token"`
		SessionID  string          `json:"session_id"`
		CallID     string          `json:"call_id"`
		Result     json.RawMessage `json:"result"`
	}
	if err := native.Call(ownerTestContext(t), "tool.call", ownerToolRequest{
		OwnerToken: "main-token", SessionID: "main-session", CallID: "main-call",
		Action: "list", Arguments: json.RawMessage(`{}`),
	}, &toolResult); err != nil {
		t.Fatal(err)
	}
	if toolResult.OwnerToken != "main-token" || toolResult.CallID != "main-call" || string(toolResult.Result) != `{"sessions":[]}` {
		t.Fatalf("primary tool result = %+v", toolResult)
	}
	if call := <-primaryCalls; call.method != "session.list" {
		t.Fatalf("primary public call = %+v", call)
	}

	childReady := make(chan error, 1)
	go func() {
		childReady <- native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
			Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopeChild,
			Mode: ownerModePrint, OwnerToken: "child-token", SessionID: "child-session", Name: "event-name",
		}, nil)
	}()
	childConn, childScanner := ownerAccept(t, listener)
	hello := ownerHello(t, childConn, childScanner)
	if err := <-childReady; err != nil {
		t.Fatal(err)
	}
	if hello.Product != Product || hello.SessionID != "child-session" || hello.Name != "child" ||
		!slices.Equal(hello.Groups, []string{"shared"}) || hello.Info["cwd"] != "/work/child" {
		t.Fatalf("child identity = %+v", hello)
	}

	childTool := make(chan error, 1)
	go func() {
		childTool <- native.Call(ownerTestContext(t), "tool.call", ownerToolRequest{
			OwnerToken: "child-token", SessionID: "child-session", CallID: "child-call",
			Action: "list", Arguments: json.RawMessage(`{}`),
		}, nil)
	}()
	frame := ownerFrame(t, childScanner)
	if !frame.Request || frame.Method != "session.list" {
		t.Fatalf("child public tool frame = %+v", frame)
	}
	body, err := protocol.ResultBytes(frame.ID, frame.Method, kit.SessionListResult{Sessions: []kit.SessionSummary{}})
	if err != nil {
		t.Fatal(err)
	}
	ownerWrite(t, childConn, body)
	if err = <-childTool; err != nil {
		t.Fatal(err)
	}
	select {
	case extra := <-primaryCalls:
		t.Fatalf("child borrowed primary caller: %+v", extra)
	default:
	}

	if err = native.Call(ownerTestContext(t), "session_end", ownerEndRequest{
		Topology: ownerTopologyLane, Scope: ownerScopeChild, Mode: ownerModePrint,
		OwnerToken: "child-token", SessionID: "child-session", Reason: "task-complete",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if childScanner.Scan() || childScanner.Err() != nil {
		t.Fatalf("child connection survived end: %v", childScanner.Err())
	}
	if got, ok := registry.Primary(); !ok || got.OwnerToken != "main-token" {
		t.Fatalf("child end changed primary = %+v, %v", got, ok)
	}

	if err = native.Call(ownerTestContext(t), "session_end", ownerEndRequest{
		Topology: ownerTopologyLane, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session", Reason: "quit",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if reason, ok := registry.GracefulEnd(); !ok || reason != "quit" {
		t.Fatalf("graceful primary end = %q, %v", reason, ok)
	}
}

func TestOwnerRegistryAcceptsCleanBridgeEOFOnlyAfterAcknowledgedEnd(t *testing.T) {
	for _, test := range []struct {
		name     string
		priorErr error
	}{
		{name: "clean"},
		{name: "preserves earlier failure", priorErr: errors.New("earlier registry failure")},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) {
				return json.RawMessage(`{}`), nil
			})
			fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
				"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
			}}
			registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
				Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory,
				PrimaryCaller: caller,
			}, fixture)
			if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
				Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
				OwnerToken: "main-token", SessionID: "main-session",
			}, nil); err != nil {
				t.Fatal(err)
			}
			if test.priorErr != nil {
				registry.recordError(test.priorErr)
			}
			if err := native.Call(ownerTestContext(t), "session_end", ownerEndRequest{
				Topology: ownerTopologyLane, Scope: ownerScopePrimary, Mode: ownerModeRPC,
				OwnerToken: "main-token", SessionID: "main-session", Reason: "quit",
			}, nil); err != nil {
				t.Fatal(err)
			}
			if err := native.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-registry.Done():
			case <-ownerTestContext(t).Done():
				t.Fatal("clean bridge EOF did not settle registry")
			}
			err := registry.Close()
			if test.priorErr == nil && err != nil {
				t.Fatalf("clean acknowledged end = %v", err)
			}
			if test.priorErr != nil && !errors.Is(err, test.priorErr) {
				t.Fatalf("earlier registry failure was lost: %v", err)
			}
		})
	}
}

func TestOwnerRegistryRejectsBridgeEOFBeforeAcknowledgedEnd(t *testing.T) {
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory,
		PrimaryCaller: caller,
	}, fixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := native.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-registry.Done():
	case <-ownerTestContext(t).Done():
		t.Fatal("unacknowledged bridge EOF did not retire registry")
	}
	if err := registry.Close(); err == nil || !strings.Contains(err.Error(), "OMP private bridge ended") {
		t.Fatalf("unacknowledged bridge EOF = %v", err)
	}
}

func TestOwnerRegistryStagesInteractiveDeliveryAndConfirmsExactBatch(t *testing.T) {
	listener := ownerBusListener(t)
	directory := t.TempDir()
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"owner-one": {OwnerToken: "owner-one", SessionID: "session-one", Name: "native", CWD: "/work/one"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
		InitialName: "requested", Groups: []string{"interactive"},
	}, fixture)
	hello, public, scanner := ownerReady(t, registry, native, listener, ownerReadyRequest{
		Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary,
		Mode: ownerModeTUI, OwnerToken: "owner-one", SessionID: "session-one", Name: "event",
	})
	if hello.Product != Product || hello.SessionID != "session-one" || hello.Name != "native" || hello.Info["cwd"] != "/work/one" {
		t.Fatalf("interactive identity = %+v", hello)
	}

	delivery := kit.DeliveryRequest{
		MessageID: " message opaque ",
		From:      kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{"interactive"}},
		Body:      "hello",
	}
	body, err := protocol.RequestBytes(40, "message.deliver", delivery)
	if err != nil {
		t.Fatal(err)
	}
	ownerWrite(t, public, body)
	frame := ownerFrame(t, scanner)
	if frame.Request || frame.ID != 40 || frame.Error != nil {
		t.Fatalf("delivery receipt frame = %+v", frame)
	}
	var receipt kit.DeliveryReceipt
	if err = protocol.UnmarshalResult("message.deliver", frame.Result, &receipt); err != nil || receipt.Disposition != "queued_for_next_turn" {
		t.Fatalf("delivery receipt = %+v, %v", receipt, err)
	}
	fixture.mu.Lock()
	stages := slices.Clone(fixture.stages)
	fixture.mu.Unlock()
	if len(stages) != 1 || stages[0].OwnerToken != "owner-one" || stages[0].SessionID != "session-one" ||
		stages[0].MessageID != delivery.MessageID || !strings.Contains(stages[0].Body, "hello") {
		t.Fatalf("native stages = %+v", stages)
	}

	for index, phase := range []string{"claimed", "message_start", "message_end", "context"} {
		var observed ownerDeliveryObserveRequest
		err = native.Call(ownerTestContext(t), "delivery.observe", ownerDeliveryObserveRequest{
			OwnerToken: "owner-one", SessionID: "session-one", BatchToken: "batch-one",
			ReportSequence: uint64(index + 1), Phase: phase, MessageIDs: []string{delivery.MessageID},
		}, &observed)
		if err != nil || observed.Phase != phase || !reflect.DeepEqual(observed.MessageIDs, []string{delivery.MessageID}) {
			t.Fatalf("%s observation = %+v, %v", phase, observed, err)
		}
	}
	if err = native.Call(ownerTestContext(t), "delivery.observe", ownerDeliveryObserveRequest{
		OwnerToken: "owner-one", SessionID: "session-one", BatchToken: "batch-one",
		ReportSequence: 5, Phase: "context", MessageIDs: []string{delivery.MessageID},
	}, nil); err == nil {
		t.Fatal("duplicate completed delivery batch was accepted")
	}
	select {
	case <-registry.Done():
	case <-ownerTestContext(t).Done():
		t.Fatal("invalid delivery observation did not fail the registry")
	}
}

func TestOwnerRegistryReturnsNativeQueueCapacityWithoutRetiringOwner(t *testing.T) {
	listener := ownerBusListener(t)
	directory := t.TempDir()
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"owner-token": {OwnerToken: "owner-token", SessionID: "native-session", CWD: "/work"},
	}}
	fixture.stageResult = func(request ownerStageRequest) ownerStageResult {
		queued := request.MessageID != "full"
		result := ownerStageResult{
			OwnerToken: request.OwnerToken, SessionID: request.SessionID, MessageID: request.MessageID, Queued: &queued,
		}
		if !queued {
			result.Reason = "queue_full"
		}
		return result
	}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
	}, fixture)
	_, _, _ = ownerReady(t, registry, native, listener, ownerReadyRequest{
		Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary,
		Mode: ownerModeTUI, OwnerToken: "owner-token", SessionID: "native-session",
	})
	state, err := registry.current("owner-token", "native-session")
	if err != nil {
		t.Fatal(err)
	}
	source := kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}
	receipt, err := registry.deliver(ownerTestContext(t), state, kit.DeliveryRequest{MessageID: "full", From: source, Body: "one"})
	if err != nil || receipt.Disposition != "rejected" || receipt.Reason != "queue_full" {
		t.Fatalf("native queue rejection = %+v, %v", receipt, err)
	}
	registry.mu.Lock()
	if len(state.staged) != 0 || state.retainedBytes != ownerBindingRetainedBytes(state.OwnerBinding) {
		registry.mu.Unlock()
		t.Fatalf("rejected stage was retained: staged=%d retained=%d", len(state.staged), state.retainedBytes)
	}
	registry.mu.Unlock()
	receipt, err = registry.deliver(ownerTestContext(t), state, kit.DeliveryRequest{MessageID: "next", From: source, Body: "two"})
	if err != nil || receipt.Disposition != "queued_for_next_turn" {
		t.Fatalf("delivery after capacity rejection = %+v, %v", receipt, err)
	}
	if err := registry.Err(); err != nil {
		t.Fatalf("native capacity rejection retired owner: %v", err)
	}
}

func TestOwnerRegistryRejectsAmbiguousNativeQueueCapacity(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "missing queued", raw: `{"owner_token":"owner-token","session_id":"native-session","message_id":"bad","reason":"queue_full"}`},
		{name: "null queued", raw: `{"owner_token":"owner-token","session_id":"native-session","message_id":"bad","queued":null,"reason":"queue_full"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener := ownerBusListener(t)
			directory := t.TempDir()
			fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
				"owner-token": {OwnerToken: "owner-token", SessionID: "native-session", CWD: "/work"},
			}}
			fixture.stageRaw = func(ownerStageRequest) json.RawMessage { return json.RawMessage(test.raw) }
			registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
				Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
			}, fixture)
			_, _, _ = ownerReady(t, registry, native, listener, ownerReadyRequest{
				Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary,
				Mode: ownerModeTUI, OwnerToken: "owner-token", SessionID: "native-session",
			})
			state, err := registry.current("owner-token", "native-session")
			if err != nil {
				t.Fatal(err)
			}
			_, err = registry.deliver(ownerTestContext(t), state, kit.DeliveryRequest{
				MessageID: "bad", From: kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}, Body: "body",
			})
			if err == nil || !strings.Contains(err.Error(), "acknowledgement is invalid") {
				t.Fatalf("ambiguous native capacity result = %v", err)
			}
			select {
			case <-registry.Done():
			case <-ownerTestContext(t).Done():
				t.Fatal("ambiguous native capacity result did not retire registry")
			}
		})
	}
}

func TestOwnerRegistryRejectsCapacityAfterNativeClaim(t *testing.T) {
	listener := ownerBusListener(t)
	directory := t.TempDir()
	entered, release := make(chan struct{}), make(chan struct{})
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"owner-token": {OwnerToken: "owner-token", SessionID: "native-session", CWD: "/work"},
	}}
	fixture.stageHook = func(ownerStageRequest) { close(entered); <-release }
	fixture.stageResult = func(request ownerStageRequest) ownerStageResult {
		queued := false
		return ownerStageResult{
			OwnerToken: request.OwnerToken, SessionID: request.SessionID, MessageID: request.MessageID,
			Queued: &queued, Reason: "queue_full",
		}
	}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
	}, fixture)
	_, _, _ = ownerReady(t, registry, native, listener, ownerReadyRequest{
		Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary,
		Mode: ownerModeTUI, OwnerToken: "owner-token", SessionID: "native-session",
	})
	state, err := registry.current("owner-token", "native-session")
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, callErr := registry.deliver(ownerTestContext(t), state, kit.DeliveryRequest{
			MessageID: "claimed", From: kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}, Body: "body",
		})
		result <- callErr
	}()
	select {
	case <-entered:
	case <-ownerTestContext(t).Done():
		t.Fatal("native stage did not begin")
	}
	if err = native.Call(ownerTestContext(t), "delivery.observe", ownerDeliveryObserveRequest{
		OwnerToken: "owner-token", SessionID: "native-session", ReportSequence: 1,
		BatchToken: "batch-one", Phase: "claimed", MessageIDs: []string{"claimed"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err = <-result; err == nil || !strings.Contains(err.Error(), "already claimed") {
		t.Fatalf("capacity result after claim = %v", err)
	}
	select {
	case <-registry.Done():
	case <-ownerTestContext(t).Done():
		t.Fatal("capacity result after claim did not retire registry")
	}
}

func TestOwnerRegistryAdmitsHelloBeforeFollowingPublicRequest(t *testing.T) {
	listener := ownerBusListener(t)
	directory := t.TempDir()
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"owner-token": {OwnerToken: "owner-token", SessionID: "native-session", Name: "native", CWD: "/work"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
	}, fixture)
	ready := make(chan error, 1)
	go func() {
		ready <- native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
			Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary,
			Mode: ownerModeTUI, OwnerToken: "owner-token", SessionID: "native-session", Name: "native",
		}, nil)
	}()
	public, scanner := ownerAccept(t, listener)
	hello := ownerFrame(t, scanner)
	helloResult, err := protocol.ResultBytes(hello.ID, hello.Method, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	deliveryResult, err := protocol.RequestBytes(72, "message.deliver", kit.DeliveryRequest{
		MessageID: "immediate", From: kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}, Body: "next frame",
	})
	if err != nil {
		t.Fatal(err)
	}
	ownerWrite(t, public, append(helloResult, deliveryResult...))
	if err = <-ready; err != nil {
		t.Fatal(err)
	}
	if primary, ok := registry.Primary(); !ok || primary.OwnerToken != "owner-token" {
		t.Fatalf("primary after combined hello/request write = %+v, %v", primary, ok)
	}
	frame := ownerFrame(t, scanner)
	if frame.ID != 72 || frame.Error != nil {
		t.Fatalf("immediate post-hello delivery = %+v", frame)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.stages) != 1 || fixture.stages[0].MessageID != "immediate" {
		t.Fatalf("immediate native stage = %+v", fixture.stages)
	}
}

func TestOwnerRegistryStageCancellationBoundaries(t *testing.T) {
	t.Run("before stage admission leaves no reservation", func(t *testing.T) {
		listener := ownerBusListener(t)
		directory := t.TempDir()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
			"owner-token": {OwnerToken: "owner-token", SessionID: "native-session", Name: "native", CWD: "/work"},
		}}
		fixture.stageHook = func(ownerStageRequest) { once.Do(func() { close(entered); <-release }) }
		registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
			Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
		}, fixture)
		_, _, _ = ownerReady(t, registry, native, listener, ownerReadyRequest{
			Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary,
			Mode: ownerModeTUI, OwnerToken: "owner-token", SessionID: "native-session", Name: "native",
		})
		state, err := registry.current("owner-token", "native-session")
		if err != nil {
			t.Fatal(err)
		}
		firstDone := make(chan error, 1)
		go func() {
			_, callErr := registry.deliver(ownerTestContext(t), state, kit.DeliveryRequest{
				MessageID: "first", From: kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}, Body: "first",
			})
			firstDone <- callErr
		}()
		select {
		case <-entered:
		case <-ownerTestContext(t).Done():
			t.Fatal("first stage did not hold admission")
		}
		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err = registry.deliver(canceled, state, kit.DeliveryRequest{
			MessageID: "canceled", From: kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}, Body: "canceled",
		}); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled pre-stage delivery = %v", err)
		}
		if _, err = registry.observeDelivery(mustOwnerJSON(t, ownerDeliveryObserveRequest{
			OwnerToken: "owner-token", SessionID: "native-session", ReportSequence: 1,
			BatchToken: "first-batch", Phase: "claimed", MessageIDs: []string{"first"},
		})); err != nil {
			t.Fatal(err)
		}
		close(release)
		if err = <-firstDone; err != nil {
			t.Fatal(err)
		}
		if _, err = registry.deliver(ownerTestContext(t), state, kit.DeliveryRequest{
			MessageID: "third", From: kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}, Body: "third",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err = registry.observeDelivery(mustOwnerJSON(t, ownerDeliveryObserveRequest{
			OwnerToken: "owner-token", SessionID: "native-session", ReportSequence: 2,
			BatchToken: "third-batch", Phase: "claimed", MessageIDs: []string{"third"},
		})); err != nil {
			t.Fatalf("claim after canceled waiter = %v", err)
		}
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		got := []string{fixture.stages[0].MessageID, fixture.stages[1].MessageID}
		if !slices.Equal(got, []string{"first", "third"}) {
			t.Fatalf("native stages after pre-admission cancellation = %#v", got)
		}
	})

	t.Run("after native stage retires uncertain owner", func(t *testing.T) {
		listener := ownerBusListener(t)
		directory := t.TempDir()
		entered, release := make(chan struct{}), make(chan struct{})
		fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
			"owner-token": {OwnerToken: "owner-token", SessionID: "native-session", Name: "native", CWD: "/work"},
		}}
		fixture.stageHook = func(ownerStageRequest) { close(entered); <-release }
		registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
			Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
		}, fixture)
		_, _, _ = ownerReady(t, registry, native, listener, ownerReadyRequest{
			Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary,
			Mode: ownerModeTUI, OwnerToken: "owner-token", SessionID: "native-session", Name: "native",
		})
		state, err := registry.current("owner-token", "native-session")
		if err != nil {
			t.Fatal(err)
		}
		callCtx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, callErr := registry.deliver(callCtx, state, kit.DeliveryRequest{
				MessageID: "uncertain", From: kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}, Body: "uncertain",
			})
			result <- callErr
		}()
		select {
		case <-entered:
		case <-ownerTestContext(t).Done():
			t.Fatal("native stage was not observed")
		}
		cancel()
		if err = <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled submitted stage = %v", err)
		}
		select {
		case <-registry.Done():
		case <-ownerTestContext(t).Done():
			t.Fatal("uncertain native stage did not retire registry")
		}
		if err = registry.Err(); err == nil || !strings.Contains(err.Error(), "stage outcome is uncertain") {
			t.Fatalf("uncertain stage registry error = %v", err)
		}
		close(release)
	})
}

func TestOwnerRegistryChildFirstDoesNotAdoptPrimary(t *testing.T) {
	listener := ownerBusListener(t)
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"child-token": {OwnerToken: "child-token", SessionID: "child-session", Name: "child", CWD: "/work/child"},
		"main-token":  {OwnerToken: "main-token", SessionID: "main-session", Name: "main", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: listener.Addr().String(), Directory: directory, PrimaryCaller: caller,
	}, fixture)
	childReady := make(chan error, 1)
	go func() {
		childReady <- native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
			Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopeChild, Mode: ownerModePrint,
			OwnerToken: "child-token", SessionID: "child-session", Name: "child",
		}, nil)
	}()
	childConn, childScanner := ownerAccept(t, listener)
	_ = ownerHello(t, childConn, childScanner)
	if err := <-childReady; err != nil {
		t.Fatal(err)
	}
	select {
	case <-registry.Ready():
		t.Fatal("child-first readiness adopted the primary")
	default:
	}
	if _, ok := registry.Primary(); ok {
		t.Fatal("child-first binding was returned as primary")
	}
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session", Name: "main",
	}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-registry.Ready():
	case <-ownerTestContext(t).Done():
		t.Fatal("explicit primary did not close readiness")
	}
}

func TestOwnerRegistryClaimCanPrecedeStageResponseAndReportsReorder(t *testing.T) {
	listener := ownerBusListener(t)
	directory := t.TempDir()
	stageEntered, releaseStage := make(chan struct{}), make(chan struct{})
	var stageOnce sync.Once
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"owner-token": {OwnerToken: "owner-token", SessionID: "native-session", Name: "native", CWD: "/work"},
	}}
	fixture.stageHook = func(ownerStageRequest) {
		stageOnce.Do(func() {
			close(stageEntered)
			<-releaseStage
		})
	}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
	}, fixture)
	_, public, scanner := ownerReady(t, registry, native, listener, ownerReadyRequest{
		Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeTUI,
		OwnerToken: "owner-token", SessionID: "native-session", Name: "native",
	})

	first := kit.DeliveryRequest{MessageID: "first", From: kit.DeliverySource{SessionID: "sender", Product: "codex-peer", Groups: []string{}}, Body: "first"}
	second := kit.DeliveryRequest{MessageID: "second", From: first.From, Body: "second"}
	firstBody, err := protocol.RequestBytes(50, "message.deliver", first)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := protocol.RequestBytes(51, "message.deliver", second)
	if err != nil {
		t.Fatal(err)
	}
	ownerWrite(t, public, append(firstBody, secondBody...))
	select {
	case <-stageEntered:
	case <-ownerTestContext(t).Done():
		t.Fatal("first native stage did not begin")
	}
	if err = native.Call(ownerTestContext(t), "delivery.observe", ownerDeliveryObserveRequest{
		OwnerToken: "owner-token", SessionID: "native-session", ReportSequence: 1,
		BatchToken: "batch", Phase: "claimed", MessageIDs: []string{"first"},
	}, nil); err != nil {
		t.Fatalf("claim before stage response = %v", err)
	}
	close(releaseStage)
	for id := int64(50); id <= 51; id++ {
		frame := ownerFrame(t, scanner)
		if frame.ID != id || frame.Error != nil {
			t.Fatalf("ordered delivery receipt %d = %+v", id, frame)
		}
	}
	fixture.mu.Lock()
	stagedIDs := []string{fixture.stages[0].MessageID, fixture.stages[1].MessageID}
	fixture.mu.Unlock()
	if !slices.Equal(stagedIDs, []string{"first", "second"}) {
		t.Fatalf("native stage order = %#v", stagedIDs)
	}

	// Model Bridge handler scheduling independently of native issue order: the
	// later report can reach registry state first, but its source sequence keeps
	// it pending until the missing predecessor is applied.
	if _, err = registry.observeDelivery(mustOwnerJSON(t, ownerDeliveryObserveRequest{
		OwnerToken: "owner-token", SessionID: "native-session", ReportSequence: 3,
		BatchToken: "batch", Phase: "message_end", MessageIDs: []string{"first"},
	})); err != nil {
		t.Fatalf("later scheduled report = %v", err)
	}
	if _, err = registry.observeDelivery(mustOwnerJSON(t, ownerDeliveryObserveRequest{
		OwnerToken: "owner-token", SessionID: "native-session", ReportSequence: 2,
		BatchToken: "batch", Phase: "message_start", MessageIDs: []string{"first"},
	})); err != nil {
		t.Fatalf("earlier scheduled report = %v", err)
	}
	if err = native.Call(ownerTestContext(t), "delivery.observe", ownerDeliveryObserveRequest{
		OwnerToken: "owner-token", SessionID: "native-session", ReportSequence: 4,
		BatchToken: "batch", Phase: "context", MessageIDs: []string{"first"},
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func mustOwnerJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestOwnerRegistryInteractiveSwitchWithdrawsBeforeReplacement(t *testing.T) {
	listener := ownerBusListener(t)
	directory := t.TempDir()
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"old-token": {OwnerToken: "old-token", SessionID: "same-session", Name: "old", CWD: "/work/old"},
		"new-token": {OwnerToken: "new-token", SessionID: "same-session", Name: "new", CWD: "/work/new"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyInteractive, Socket: listener.Addr().String(), Directory: directory,
	}, fixture)
	_, oldConn, oldScanner := ownerReady(t, registry, native, listener, ownerReadyRequest{
		Topology: ownerTopologyInteractive, Directory: directory, Scope: ownerScopePrimary,
		Mode: ownerModeTUI, OwnerToken: "old-token", SessionID: "same-session", Name: "old",
	})

	switched := make(chan error, 1)
	go func() {
		switched <- native.Call(ownerTestContext(t), "owner.switch", ownerSwitchRequest{
			Topology: ownerTopologyInteractive, Scope: ownerScopePrimary, Mode: ownerModeTUI,
			PreviousOwnerToken: "old-token", OwnerToken: "new-token",
			PreviousSessionID: "same-session", SessionID: "same-session", Name: "new", Reason: "reload",
		}, nil)
	}()
	newConn, newScanner := ownerAccept(t, listener)
	newHello := ownerHello(t, newConn, newScanner)
	if err := <-switched; err != nil {
		t.Fatal(err)
	}
	if newHello.SessionID != "same-session" || newHello.Name != "new" || newHello.Info["cwd"] != "/work/new" {
		t.Fatalf("replacement hello = %+v", newHello)
	}
	if oldScanner.Scan() || oldScanner.Err() != nil {
		t.Fatalf("old connection survived replacement: %v", oldScanner.Err())
	}
	_ = oldConn.Close()
	if primary, ok := registry.Primary(); !ok || primary.OwnerToken != "new-token" || primary.CWD != "/work/new" {
		t.Fatalf("replacement primary = %+v, %v", primary, ok)
	}

	err := native.Call(ownerTestContext(t), "session_end", ownerEndRequest{
		Topology: ownerTopologyInteractive, Scope: ownerScopePrimary, Mode: ownerModeTUI,
		OwnerToken: "old-token", SessionID: "same-session", Reason: "delayed",
	}, nil)
	var bridgeErr *pifamily.BridgeCallError
	if !errors.As(err, &bridgeErr) || bridgeErr.Code != "stale_owner" {
		t.Fatalf("delayed old end = %#v", err)
	}
	if primary, ok := registry.Primary(); !ok || primary.OwnerToken != "new-token" {
		t.Fatalf("delayed old end changed primary = %+v, %v", primary, ok)
	}
}

func TestOwnerRegistryToolCallPinsCallerGeneration(t *testing.T) {
	directory := t.TempDir()
	started, release := make(chan struct{}), make(chan struct{})
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) {
		close(started)
		<-release
		return json.RawMessage(`{"sessions":[]}`), nil
	})
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
	}, fixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if primary, ok := registry.Primary(); !ok || primary.OwnerToken != "main-token" {
		t.Fatalf("primary before caller withdrawal = %+v, %v", primary, ok)
	}
	toolDone := make(chan error, 1)
	go func() {
		toolDone <- native.Call(ownerTestContext(t), "tool.call", ownerToolRequest{
			OwnerToken: "main-token", SessionID: "main-session", CallID: "call-one",
			Action: "list", Arguments: json.RawMessage(`{}`),
		}, nil)
	}()
	select {
	case <-started:
	case <-ownerTestContext(t).Done():
		t.Fatal("public tool call did not start")
	}
	if err := native.Call(ownerTestContext(t), "session_end", ownerEndRequest{
		Topology: ownerTopologyLane, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session", Reason: "ended",
	}, nil); err != nil {
		t.Fatal(err)
	}
	close(release)
	err := <-toolDone
	var bridgeErr *pifamily.BridgeCallError
	if !errors.As(err, &bridgeErr) || bridgeErr.Code != "stale_owner" {
		t.Fatalf("tool result after owner withdrawal = %#v", err)
	}
}

func TestOwnerRegistryConsumedEvidenceDoesNotImposeLifetimeTurnLimit(t *testing.T) {
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
	}, fixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session",
	}, nil); err != nil {
		t.Fatal(err)
	}
	var sequence uint64
	for index := 0; index < maxOwnerTrackedItems+8; index++ {
		sequence++
		runToken := fmt.Sprintf("run-%d", index)
		prompt := fmt.Sprintf("prompt-%d", index)
		if _, err := registry.recordPreflight(mustOwnerJSON(t, ownerPreflightRequest{
			OwnerToken: "main-token", SessionID: "main-session", ReportSequence: sequence,
			RunToken: runToken, Prompt: prompt,
		})); err != nil {
			t.Fatalf("preflight %d = %v", index, err)
		}
		if got, err := registry.takePreflight("main-token", "main-session", runToken); err != nil || got != prompt {
			t.Fatalf("consumed preflight %d = %q, %v", index, got, err)
		}
	}
	state, err := registry.current("main-token", "main-session")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxOwnerTrackedItems+8; index++ {
		messageID := fmt.Sprintf("message-%d", index)
		batchToken := fmt.Sprintf("batch-%d", index)
		registry.mu.Lock()
		if err = registry.reserveStateLocked(state, len(messageID)); err != nil {
			registry.mu.Unlock()
			t.Fatal(err)
		}
		state.staged = append(state.staged, &ownerStagedDelivery{messageID: messageID, acknowledged: true})
		registry.mu.Unlock()
		for _, phase := range []string{"claimed", "message_start", "message_end", "context"} {
			sequence++
			if _, err = registry.observeDelivery(mustOwnerJSON(t, ownerDeliveryObserveRequest{
				OwnerToken: "main-token", SessionID: "main-session", ReportSequence: sequence,
				BatchToken: batchToken, Phase: phase, MessageIDs: []string{messageID},
			})); err != nil {
				t.Fatalf("batch %d phase %s = %v", index, phase, err)
			}
		}
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(state.preflights) != 0 || len(state.consumedPreflights) != maxOwnerTrackedItems || len(state.consumedPreflightSet) != maxOwnerTrackedItems ||
		len(state.batches) != 0 || len(state.completed) != maxOwnerTrackedItems || len(state.completedSet) != maxOwnerTrackedItems {
		t.Fatalf("retained evidence = preflights %d/%d/%d, batches %d/%d/%d", len(state.preflights), len(state.consumedPreflights),
			len(state.consumedPreflightSet), len(state.batches), len(state.completed), len(state.completedSet))
	}
}

func TestOwnerRegistryWaitsForNextMatchingPreflightAndConsumesAccounting(t *testing.T) {
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
	}, fixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session",
	}, nil); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		token string
		err   error
	}
	result := make(chan outcome, 1)
	go func() {
		token, err := registry.waitPreflight(ownerTestContext(t), "main-token", "main-session", "owned prompt")
		result <- outcome{token, err}
	}()
	if _, err := registry.recordPreflight(mustOwnerJSON(t, ownerPreflightRequest{
		OwnerToken: "main-token", SessionID: "main-session", ReportSequence: 1,
		RunToken: "run-one", Prompt: "owned prompt",
	})); err != nil {
		t.Fatal(err)
	}
	got := <-result
	if got.err != nil || got.token != "run-one" {
		t.Fatalf("preflight wait = %#v", got)
	}
	registry.mu.Lock()
	state := registry.bindings["main-token"]
	if len(state.preflights) != 0 || len(state.preflightOrder) != 0 || len(state.consumedPreflights) != 1 || state.consumedPreflights[0] != "run-one" {
		t.Fatalf("preflight evidence = pending %v/%v, consumed %v", state.preflights, state.preflightOrder, state.consumedPreflights)
	}
	registry.mu.Unlock()
}

func TestOwnerRegistryPreflightWaitDoesNotSearchPastForeignWitness(t *testing.T) {
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
	}, fixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session",
	}, nil); err != nil {
		t.Fatal(err)
	}
	for sequence, value := range []struct{ token, prompt string }{{"foreign-run", "foreign"}, {"owned-run", "owned"}} {
		if _, err := registry.recordPreflight(mustOwnerJSON(t, ownerPreflightRequest{
			OwnerToken: "main-token", SessionID: "main-session", ReportSequence: uint64(sequence + 1),
			RunToken: value.token, Prompt: value.prompt,
		})); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := registry.waitPreflight(ownerTestContext(t), "main-token", "main-session", "owned"); err == nil {
		t.Fatal("foreign leading preflight was accepted")
	}
	if registry.Err() == nil {
		t.Fatal("foreign leading preflight did not retire the registry")
	}
}

func TestOwnerRegistryPreflightWaitWakesOnCancellationAndOwnerEnd(t *testing.T) {
	for _, ending := range []bool{false, true} {
		t.Run(fmt.Sprintf("ending-%v", ending), func(t *testing.T) {
			directory := t.TempDir()
			caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
			fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
				"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
			}}
			registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
				Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
			}, fixture)
			if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
				Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
				OwnerToken: "main-token", SessionID: "main-session",
			}, nil); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				_, err := registry.waitPreflight(ctx, "main-token", "main-session", "owned")
				result <- err
			}()
			if ending {
				if err := native.Call(ownerTestContext(t), "session_end", ownerEndRequest{
					Topology: ownerTopologyLane, Scope: ownerScopePrimary, Mode: ownerModeRPC,
					OwnerToken: "main-token", SessionID: "main-session", Reason: "shutdown",
				}, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			if err := <-result; err == nil {
				t.Fatal("preflight wait did not report cancellation or owner end")
			}
			cancel()
		})
	}
}

func TestOwnerRegistryCanceledPreflightWaitPreservesArrivedEvidence(t *testing.T) {
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
	}, fixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.recordPreflight(mustOwnerJSON(t, ownerPreflightRequest{
		OwnerToken: "main-token", SessionID: "main-session", ReportSequence: 1,
		RunToken: "run-one", Prompt: "owned prompt",
	})); err != nil {
		t.Fatal(err)
	}
	registry.mu.Lock()
	state := registry.bindings["main-token"]
	retained := state.retainedBytes
	registry.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := registry.waitPreflight(ctx, "main-token", "main-session", "owned prompt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled preflight wait = %v", err)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if state.retainedBytes != retained || state.preflights["run-one"] != "owned prompt" ||
		!reflect.DeepEqual(state.preflightOrder, []string{"run-one"}) {
		t.Fatalf("canceled evidence = retained %d/%d, preflights %v, order %v", state.retainedBytes, retained, state.preflights, state.preflightOrder)
	}
}

func TestOwnerRegistryFailedPreflightWaitPreservesArrivedEvidence(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled-%v", canceled), func(t *testing.T) {
			directory := t.TempDir()
			caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
			fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
				"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
			}}
			registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
				Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
			}, fixture)
			if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
				Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
				OwnerToken: "main-token", SessionID: "main-session",
			}, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := registry.recordPreflight(mustOwnerJSON(t, ownerPreflightRequest{
				OwnerToken: "main-token", SessionID: "main-session", ReportSequence: 1,
				RunToken: "run-one", Prompt: "owned prompt",
			})); err != nil {
				t.Fatal(err)
			}
			registry.mu.Lock()
			state := registry.bindings["main-token"]
			retained := state.retainedBytes
			registry.mu.Unlock()
			failure := errors.New("controlled registry loss")
			registry.recordError(failure)
			if canceled {
				registry.cancel()
			}
			if _, err := registry.waitPreflight(ownerTestContext(t), "main-token", "main-session", "owned prompt"); err == nil || !strings.Contains(err.Error(), failure.Error()) {
				t.Fatalf("failed preflight wait = %v", err)
			}
			registry.mu.Lock()
			defer registry.mu.Unlock()
			if state.retainedBytes != retained || state.preflights["run-one"] != "owned prompt" ||
				!reflect.DeepEqual(state.preflightOrder, []string{"run-one"}) {
				t.Fatalf("failed evidence = retained %d/%d, preflights %v, order %v", state.retainedBytes, retained, state.preflights, state.preflightOrder)
			}
		})
	}
}

func TestOwnerRegistryBoundsReorderedReportPayloadBytes(t *testing.T) {
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
	}, fixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session",
	}, nil); err != nil {
		t.Fatal(err)
	}
	prompt := strings.Repeat("x", maxOwnerTextBytes)
	accepted := 0
	for index := 0; index < maxOwnerRetainedBytes/maxOwnerTextBytes+2; index++ {
		_, err := registry.recordPreflight(mustOwnerJSON(t, ownerPreflightRequest{
			OwnerToken: "main-token", SessionID: "main-session", ReportSequence: uint64(index + 2),
			RunToken: fmt.Sprintf("run-%d", index), Prompt: prompt,
		}))
		if err != nil {
			break
		}
		accepted++
	}
	if accepted == 0 || accepted >= maxOwnerRetainedBytes/maxOwnerTextBytes+2 {
		t.Fatalf("reordered reports accepted before byte bound = %d", accepted)
	}
	registry.mu.Lock()
	retained := registry.retainedBytes
	registry.mu.Unlock()
	if retained > maxOwnerRetainedBytes {
		t.Fatalf("retained report payload bytes = %d", retained)
	}
	select {
	case <-registry.Done():
	case <-ownerTestContext(t).Done():
		t.Fatal("retained report byte overflow did not retire registry")
	}
}

func TestOwnerRegistryShutdownIsPrimaryRequestOnly(t *testing.T) {
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	fixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", CWD: "/work/main"},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "bus.sock"), Directory: directory, PrimaryCaller: caller,
	}, fixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := registry.shutdownPrimary(ownerTestContext(t)); err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	shutdowns := slices.Clone(fixture.shutdowns)
	fixture.mu.Unlock()
	if !reflect.DeepEqual(shutdowns, []ownerDescribeRequest{{OwnerToken: "main-token", SessionID: "main-session"}}) {
		t.Fatalf("native shutdowns = %+v", shutdowns)
	}
	if err := native.Call(ownerTestContext(t), "session_end", ownerEndRequest{
		Topology: ownerTopologyLane, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session", Reason: "quit",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := registry.shutdownPrimary(ownerTestContext(t)); err == nil {
		t.Fatal("shutdown without primary was accepted")
	}
}
