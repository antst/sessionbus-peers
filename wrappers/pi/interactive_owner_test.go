// SPDX-License-Identifier: MIT

package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antst/sessionbus-peers/internal/testsocket"
	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/pifamily"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

type interactiveNativeFixture struct {
	mu      sync.Mutex
	id      string
	name    string
	cwd     string
	replies []bool
	appends []interactiveAppendRequest
}

func (f *interactiveNativeFixture) set(id, name, cwd string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.id, f.name, f.cwd = id, name, cwd
}

func (f *interactiveNativeFixture) handle(_ context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result any
	switch method {
	case "native.describe":
		var request struct {
			SessionID string `json:"session_id"`
		}
		if err := decodeInteractiveParams(raw, &request); err != nil || request.SessionID != f.id {
			return nil, pifamily.NewBridgeCallError("bad_request", "wrong native description")
		}
		result = interactiveDescribeResult{f.id, f.name, f.cwd}
	case "native.append":
		var request interactiveAppendRequest
		if err := decodeInteractiveParams(raw, &request); err != nil || request.SessionID != f.id {
			return nil, pifamily.NewBridgeCallError("bad_request", "wrong native append")
		}
		f.appends = append(f.appends, request)
		accepted := true
		if len(f.replies) > 0 {
			accepted, f.replies = f.replies[0], f.replies[1:]
		}
		if accepted {
			result = interactiveAppendResult{SessionID: f.id, MessageID: request.MessageID, Accepted: true, EntryID: "native-entry"}
		} else {
			result = interactiveAppendResult{SessionID: f.id, MessageID: request.MessageID, Reason: "busy"}
		}
	default:
		return nil, pifamily.NewBridgeCallError("method_not_found", "unexpected native method")
	}
	return json.Marshal(result)
}

func interactiveOwnerPair(t *testing.T, socket, directory, initialName string, groups []string, fixture *interactiveNativeFixture) (*interactiveOwner, *pifamily.Bridge) {
	t.Helper()
	left, right := net.Pipe()
	owner, err := newInteractiveOwner(context.Background(), socket, directory, initialName, groups)
	if err != nil {
		t.Fatal(err)
	}
	assigned := make(chan struct{})
	hostBridge, err := pifamily.NewBridge(left, pifamily.BridgeHost, func(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
		<-assigned
		return owner.handleBridge(ctx, method, raw)
	}, pifamily.BridgeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.assignBridge(hostBridge); err != nil {
		t.Fatal(err)
	}
	close(assigned)
	native, err := pifamily.NewBridge(right, pifamily.BridgeNative, fixture.handle, pifamily.BridgeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = hostBridge.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if err = native.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = owner.Close()
		_ = hostBridge.Close()
		_ = native.Close()
	})
	return owner, native
}

func interactiveBusListener(t *testing.T) net.Listener {
	t.Helper()
	path := filepath.Join(testsocket.Directory(t), "bus.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func interactiveAccept(t *testing.T, listener net.Listener) net.Conn {
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
	return conn
}

func interactiveFrame(t *testing.T, scanner *bufio.Scanner) protocol.Frame {
	t.Helper()
	if !scanner.Scan() {
		t.Fatalf("missing frame: %v", scanner.Err())
	}
	frame, err := protocol.DecodeFrame(scanner.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func interactiveWrite(t *testing.T, conn net.Conn, body []byte) {
	t.Helper()
	if _, err := conn.Write(body); err != nil {
		t.Fatal(err)
	}
}

func interactiveHello(t *testing.T, conn net.Conn, scanner *bufio.Scanner) kit.PeerIdentity {
	t.Helper()
	frame := interactiveFrame(t, scanner)
	if !frame.Request || frame.Method != "session.hello" {
		t.Fatalf("hello frame = %+v", frame)
	}
	params, err := protocol.DecodeParams(frame.Method, frame.Params)
	if err != nil {
		t.Fatal(err)
	}
	hello, ok := params.(*protocol.PeerHello)
	if !ok {
		t.Fatalf("hello params = %T", params)
	}
	body, err := protocol.ResultBytes(frame.ID, frame.Method, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	interactiveWrite(t, conn, body)
	return *hello
}

func interactiveReady(t *testing.T, native *pifamily.Bridge, listener net.Listener, request interactiveReadyRequest) (kit.PeerIdentity, net.Conn, *bufio.Scanner) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		var result struct {
			SessionID string `json:"session_id"`
		}
		err := native.Call(context.Background(), "owner.ready", request, &result)
		if err == nil && result.SessionID != request.SessionID {
			err = context.Canceled
		}
		done <- err
	}()
	conn := interactiveAccept(t, listener)
	scanner := bufio.NewScanner(conn)
	hello := interactiveHello(t, conn, scanner)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner.ready did not finish")
	}
	return hello, conn, scanner
}

func TestInteractiveOwnerPublishesRebindsAndRoutesExactToolIdentity(t *testing.T) {
	listener := interactiveBusListener(t)
	nativeState := &interactiveNativeFixture{}
	owner, native := interactiveOwnerPair(t, listener.Addr().String(), t.TempDir(), "wrapper fallback", []string{"team"}, nativeState)
	nativeState.set("native-one", "renamed during ready", "/work/one")
	hello, first, firstScanner := interactiveReady(t, native, listener, interactiveReadyRequest{interactiveTopology, owner.directory, "native-one", "stale event title"})
	if hello.Product != Product || hello.SessionID != "native-one" || hello.Name != "renamed during ready" ||
		!reflect.DeepEqual(hello.Groups, []string{"team"}) || hello.Info["cwd"] != "/work/one" {
		t.Fatalf("first hello = %+v", hello)
	}

	toolDone := make(chan struct {
		result json.RawMessage
		err    error
	}, 1)
	go func() {
		var result json.RawMessage
		err := native.Call(context.Background(), "tool.call", interactiveToolRequest{
			SessionID: "native-one", CallID: "native-call", Action: "list", Arguments: json.RawMessage(`{}`),
		}, &result)
		toolDone <- struct {
			result json.RawMessage
			err    error
		}{result, err}
	}()
	call := interactiveFrame(t, firstScanner)
	if !call.Request || call.Method != "session.list" {
		t.Fatalf("public call = %+v", call)
	}
	list := kit.SessionListResult{SelfInfo: &kit.SessionSelfInfo{SessionID: "native-one", Product: Product, Groups: []string{"team"}}, Sessions: []kit.SessionSummary{}}
	body, err := protocol.ResultBytes(call.ID, call.Method, list)
	if err != nil {
		t.Fatal(err)
	}
	interactiveWrite(t, first, body)
	tool := <-toolDone
	if tool.err != nil {
		t.Fatal(tool.err)
	}
	var routed struct {
		SessionID string                `json:"session_id"`
		CallID    string                `json:"call_id"`
		Result    kit.SessionListResult `json:"result"`
	}
	if json.Unmarshal(tool.result, &routed) != nil || routed.SessionID != "native-one" || routed.CallID != "native-call" ||
		routed.Result.SelfInfo == nil || routed.Result.SelfInfo.SessionID != "native-one" {
		t.Fatalf("tool result = %s", tool.result)
	}

	var ended map[string]string
	if err = native.Call(context.Background(), "session_end", interactiveEndRequest{interactiveTopology, "native-one", "resume"}, &ended); err != nil || ended["session_id"] != "native-one" {
		t.Fatalf("session_end = %#v, %v", ended, err)
	}
	nativeState.set("native-two", "", "/work/two")
	hello, _, _ = interactiveReady(t, native, listener, interactiveReadyRequest{interactiveTopology, owner.directory, "native-two", ""})
	if hello.SessionID != "native-two" || hello.Name != "wrapper fallback" || hello.Info["cwd"] != "/work/two" {
		t.Fatalf("replacement hello = %+v", hello)
	}
}

func TestInteractiveOwnerPreservesFIFOAcrossBusyAndIdleDeliveries(t *testing.T) {
	listener := interactiveBusListener(t)
	nativeState := &interactiveNativeFixture{replies: []bool{false, true, true, true}}
	owner, native := interactiveOwnerPair(t, listener.Addr().String(), t.TempDir(), "", []string{"team"}, nativeState)
	nativeState.set("native-one", "native title", "/work")
	_, bus, scanner := interactiveReady(t, native, listener, interactiveReadyRequest{interactiveTopology, owner.directory, "native-one", "native title"})

	delivery := kit.DeliveryRequest{MessageID: "opaque busy id", From: kit.DeliverySource{SessionID: "sender", Name: "Sender", Product: "fixture", Groups: []string{"team"}}, Body: "busy body"}
	body, err := protocol.RequestBytes(1, "message.deliver", delivery)
	if err != nil {
		t.Fatal(err)
	}
	interactiveWrite(t, bus, body)
	frame := interactiveFrame(t, scanner)
	var receipt kit.DeliveryReceipt
	if frame.Request || protocol.UnmarshalResult("message.deliver", frame.Result, &receipt) != nil || receipt.Disposition != "queued_for_next_turn" {
		t.Fatalf("busy receipt = %+v frame=%+v", receipt, frame)
	}

	// Native would accept now, but the older owned delivery must drain first.
	delivery.MessageID, delivery.Body = "opaque later id", "later body"
	body, err = protocol.RequestBytes(2, "message.deliver", delivery)
	if err != nil {
		t.Fatal(err)
	}
	interactiveWrite(t, bus, body)
	frame = interactiveFrame(t, scanner)
	if frame.Request || protocol.UnmarshalResult("message.deliver", frame.Result, &receipt) != nil || receipt.Disposition != "queued_for_next_turn" {
		t.Fatalf("ordered receipt = %+v frame=%+v", receipt, frame)
	}

	var drained struct {
		SessionID string `json:"session_id"`
		Drained   int    `json:"drained"`
	}
	if err = native.Call(context.Background(), "owner.drain", interactiveDrainRequest{"native-one", "before_agent_start"}, &drained); err != nil || drained.SessionID != "native-one" || drained.Drained != 2 {
		t.Fatalf("first drain = %+v, %v", drained, err)
	}
	if err = native.Call(context.Background(), "owner.drain", interactiveDrainRequest{"native-one", "agent_settled"}, &drained); err != nil || drained.Drained != 0 {
		t.Fatalf("second drain = %+v, %v", drained, err)
	}

	delivery.MessageID, delivery.Body = "delivery-idle", "idle body"
	body, err = protocol.RequestBytes(3, "message.deliver", delivery)
	if err != nil {
		t.Fatal(err)
	}
	interactiveWrite(t, bus, body)
	frame = interactiveFrame(t, scanner)
	if frame.Request || protocol.UnmarshalResult("message.deliver", frame.Result, &receipt) != nil || receipt.Disposition != "written" {
		t.Fatalf("idle receipt = %+v frame=%+v", receipt, frame)
	}

	nativeState.mu.Lock()
	appends := append([]interactiveAppendRequest(nil), nativeState.appends...)
	nativeState.mu.Unlock()
	if len(appends) != 4 || appends[0].MessageID != "opaque busy id" || appends[1].MessageID != "opaque busy id" ||
		appends[2].MessageID != "opaque later id" || appends[3].MessageID != "delivery-idle" {
		t.Fatalf("native appends = %+v", appends)
	}
	want, err := host.RenderNativeMessage(kit.DeliveryRequest{MessageID: "delivery-idle", From: delivery.From, Body: "idle body"})
	if err != nil || appends[3].Body != want {
		t.Fatalf("rendered append = %q, want %q, err=%v", appends[3].Body, want, err)
	}
}

func TestInteractiveOwnerCanceledDeliveryBeforeAdmissionDoesNotRetireOwner(t *testing.T) {
	listener := interactiveBusListener(t)
	nativeState := &interactiveNativeFixture{}
	owner, native := interactiveOwnerPair(t, listener.Addr().String(), t.TempDir(), "", []string{"team"}, nativeState)
	nativeState.set("native-one", "title", "/work")
	_, _, _ = interactiveReady(t, native, listener, interactiveReadyRequest{interactiveTopology, owner.directory, "native-one", "title"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := owner.deliver(ctx, owner.generation, kit.DeliveryRequest{
		MessageID: "opaque canceled id", From: kit.DeliverySource{SessionID: "sender", Product: "fixture"}, Body: "body",
	})
	if !errors.Is(err, errInteractiveDeliveryCanceled) || owner.Err() != nil {
		t.Fatalf("canceled delivery = %v, owner = %v", err, owner.Err())
	}
	nativeState.mu.Lock()
	defer nativeState.mu.Unlock()
	if len(nativeState.appends) != 0 {
		t.Fatalf("canceled delivery reached native: %+v", nativeState.appends)
	}
}

func TestInteractiveOwnerOldGenerationCannotUseSameIDReplacement(t *testing.T) {
	listener := interactiveBusListener(t)
	nativeState := &interactiveNativeFixture{}
	owner, native := interactiveOwnerPair(t, listener.Addr().String(), t.TempDir(), "", []string{"team"}, nativeState)
	nativeState.set("native-one", "first", "/work")
	_, first, _ := interactiveReady(t, native, listener, interactiveReadyRequest{interactiveTopology, owner.directory, "native-one", "first"})
	owner.mu.Lock()
	old, generation := owner.conn, owner.generation
	owner.mu.Unlock()
	var ended map[string]string
	if err := native.Call(context.Background(), "session_end", interactiveEndRequest{interactiveTopology, "native-one", "reload"}, &ended); err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	nativeState.set("native-one", "reloaded", "/work")
	_, _, _ = interactiveReady(t, native, listener, interactiveReadyRequest{interactiveTopology, owner.directory, "native-one", "reloaded"})
	if _, err := owner.callPublic(context.Background(), "native-one", old, generation, "session.list", kit.SessionListRequest{}); err == nil || !strings.Contains(err.Error(), "current session") {
		t.Fatalf("old generation call = %v", err)
	}
}

func TestInteractiveOwnerCloseJoinsUnadmittedHelloWatcher(t *testing.T) {
	listener := interactiveBusListener(t)
	owner, err := newInteractiveOwner(context.Background(), listener.Addr().String(), t.TempDir(), "", []string{"team"})
	if err != nil {
		t.Fatal(err)
	}
	published := make(chan error, 1)
	go func() { published <- owner.publish(context.Background(), "native-one", "title", "/work") }()
	bus := interactiveAccept(t, listener)
	scanner := bufio.NewScanner(bus)
	frame := interactiveFrame(t, scanner)
	if !frame.Request || frame.Method != "session.hello" {
		t.Fatalf("held hello = %+v", frame)
	}
	closed := make(chan error, 1)
	go func() { closed <- owner.Close() }()
	select {
	case err = <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not join the unadmitted public watcher")
	}
	select {
	case err = <-published:
		if err == nil {
			t.Fatal("held hello was admitted after Close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("held hello did not settle")
	}
}

func TestInteractiveOwnerRejectsPoisonedDeliveryBeforeExistingQueue(t *testing.T) {
	owner, err := newInteractiveOwner(context.Background(), "/bus.sock", t.TempDir(), "", []string{"team"})
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	defer right.Close()
	connection := kit.NewConnection(left, func(context.Context, *kit.Request) {})
	defer owner.Close()
	owner.mu.Lock()
	owner.conn = connection
	owner.sessionID = "native-one"
	owner.generation = 9
	owner.queue = []interactiveQueuedDelivery{{messageID: "older", body: "older", bytes: 10}}
	owner.queueBytes = 10
	owner.mu.Unlock()
	for _, request := range []kit.DeliveryRequest{
		{MessageID: strings.Repeat(" ", 257), From: kit.DeliverySource{SessionID: "sender", Product: "fixture"}, Body: "body"},
		{MessageID: "new", From: kit.DeliverySource{SessionID: "sender", Product: "fixture"}, Body: strings.Repeat("x", maxInteractiveTextBytes)},
	} {
		if _, err = owner.deliver(context.Background(), 9, request); err == nil {
			t.Fatalf("poisoned delivery accepted: id=%d body=%d", len(request.MessageID), len(request.Body))
		}
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if len(owner.queue) != 1 || owner.queue[0].messageID != "older" || owner.queueBytes != 10 {
		t.Fatalf("queue changed after rejected delivery: %+v bytes=%d", owner.queue, owner.queueBytes)
	}
}
