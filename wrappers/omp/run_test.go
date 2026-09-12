// SPDX-License-Identifier: MIT

package omp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/antst/sessionbus-peers/internal/testsocket"
	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/pifamily"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

type ompHeldDoneContext struct {
	context.Context
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (ctx *ompHeldDoneContext) Done() <-chan struct{} {
	ctx.once.Do(func() {
		close(ctx.entered)
		<-ctx.release
	})
	return ctx.Context.Done()
}

type ompRunFixture struct {
	wrapper  *Wrapper
	rpc      *nativeRPC
	peer     *nativeRPCTestPeer
	registry *OwnerRegistry
	native   *pifamily.Bridge
	binding  OwnerBinding
}

func newOMPRunFixture(t *testing.T) *ompRunFixture {
	t.Helper()
	directory := t.TempDir()
	caller := kit.NewCaller(func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	ownerFixture := &ownerNativeFixture{descriptions: map[string]ownerDescribeResult{
		"main-token": {OwnerToken: "main-token", SessionID: "main-session", Name: "main", CWD: directory},
	}}
	registry, native := ownerRegistryPair(t, OwnerRegistryOptions{
		Topology: ownerTopologyLane, Socket: filepath.Join(directory, "daemon.sock"), Directory: directory, PrimaryCaller: caller,
	}, ownerFixture)
	if err := native.Call(ownerTestContext(t), "owner.ready", ownerReadyRequest{
		Topology: ownerTopologyLane, Directory: directory, Scope: ownerScopePrimary, Mode: ownerModeRPC,
		OwnerToken: "main-token", SessionID: "main-session", Name: "main",
	}, nil); err != nil {
		t.Fatal(err)
	}
	binding, ok := registry.Primary()
	if !ok {
		t.Fatal("primary binding missing")
	}
	wrapper := New(filepath.Join(directory, "daemon.sock"), "provisional", NativeExecutable{}, filepath.Join(directory, "extension.mjs"))
	wrapper.ctx, wrapper.cancel = context.WithCancelCause(context.Background())
	wrapper.binding, wrapper.opened = binding, true
	rpc, peer := newNativeRPCTest(t, wrapper.observeNative, nativeRPCLimits{})
	wrapper.owner = &NativeOwner{ctx: wrapper.ctx, rpc: rpc, registry: registry}
	t.Cleanup(func() { wrapper.cancel(errNativeOwnerClosed) })
	return &ompRunFixture{wrapper: wrapper, rpc: rpc, peer: peer, registry: registry, native: native, binding: binding}
}

func attachHeldOMPRPC(t *testing.T, fixture *ompRunFixture) (*heldNativeRPCWriter, func()) {
	t.Helper()
	commands, input := io.Pipe()
	output, events := io.Pipe()
	held := &heldNativeRPCWriter{WriteCloser: input, wrote: make(chan struct{}), release: make(chan struct{})}
	rpc, err := newNativeRPC(held, output, fixture.wrapper.observeNative, nativeRPCLimits{})
	if err != nil {
		t.Fatal(err)
	}
	peer := &nativeRPCTestPeer{commands: bufio.NewReader(commands), events: events}
	readyNativeRPCTest(t, rpc, peer)
	fixture.rpc, fixture.peer, fixture.wrapper.owner.rpc = rpc, peer, rpc
	var once sync.Once
	release := func() { once.Do(func() { close(held.release) }) }
	t.Cleanup(func() {
		release()
		_ = commands.Close()
		_ = events.Close()
		_ = rpc.Close()
	})
	return held, release
}

func (fixture *ompRunFixture) start(t *testing.T, prompt string) <-chan struct {
	turn *ompNativeTurn
	err  error
} {
	t.Helper()
	result := make(chan struct {
		turn *ompNativeTurn
		err  error
	}, 1)
	go func() {
		turn, err := fixture.wrapper.startNativeTurn(nativeRPCTestContext(t), nil, prompt)
		result <- struct {
			turn *ompNativeTurn
			err  error
		}{turn, err}
	}()
	return result
}

func (fixture *ompRunFixture) prompt(t *testing.T, prompt string, data string) (string, <-chan struct {
	turn *ompNativeTurn
	err  error
}) {
	t.Helper()
	result := fixture.start(t, prompt)
	command := fixture.peer.read(t)
	id := nativeRPCField(t, command, "id")
	if nativeRPCField(t, command, "type") != "prompt" || string(command["message"]) != mustOMPJSONText(t, prompt) {
		t.Fatalf("prompt command = %#v", command)
	}
	response := `{"id":` + mustOMPJSONText(t, id) + `,"type":"response","command":"prompt","success":true`
	if data != "" {
		response += `,"data":` + data
	}
	fixture.peer.write(t, response+`}`)
	return id, result
}

func (fixture *ompRunFixture) preflight(t *testing.T, sequence uint64, token, prompt string) {
	t.Helper()
	if err := fixture.native.Call(ownerTestContext(t), "run.preflight", ownerPreflightRequest{
		OwnerToken: fixture.binding.OwnerToken, SessionID: fixture.binding.SessionID,
		ReportSequence: sequence, RunToken: token, Prompt: prompt,
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func mustOMPJSONText(t *testing.T, value string) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestOMPRunJoinsLatePreflightAndCurrentTerminalAssistant(t *testing.T) {
	fixture := newOMPRunFixture(t)
	_, started := fixture.prompt(t, "owned prompt", `{"agentInvoked":true}`)
	fixture.peer.write(t, `{"type":"agent_start"}`)
	fixture.peer.write(t, `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"native answer"}],"stopReason":"stop"}}`)
	fixture.peer.write(t, `{"type":"agent_end","messages":[],"isTerminal":true}`)
	fixture.preflight(t, 1, "run-one", "owned prompt")
	start := <-started
	if start.err != nil || start.turn == nil {
		t.Fatalf("start = %#v, %v", start.turn, start.err)
	}
	result, err := start.turn.Wait(nativeRPCTestContext(t))
	if err != nil || result.Outcome != "completed" || result.Result != "native answer" || result.NativeStopReason != "stop" {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestOMPImmediateNoAgentIsFiniteWithoutBorrowedAssistant(t *testing.T) {
	fixture := newOMPRunFixture(t)
	_, started := fixture.prompt(t, "/local", `{"agentInvoked":false}`)
	start := <-started
	if start.err != nil || start.turn == nil {
		t.Fatalf("start = %#v, %v", start.turn, start.err)
	}
	result, err := start.turn.Wait(nativeRPCTestContext(t))
	if err != nil || result.Outcome != "completed" || result.Result != "" || result.NativeStopReason != "no_agent" {
		t.Fatalf("no-agent result = %+v, %v", result, err)
	}
	fixture.registry.mu.Lock()
	preflights := len(fixture.registry.bindings[fixture.binding.OwnerToken].preflights)
	fixture.registry.mu.Unlock()
	if preflights != 0 {
		t.Fatal("no-agent consumed a foreign preflight")
	}
}

func TestOMPPromptResultNoAgentIsFiniteAndCorrelated(t *testing.T) {
	fixture := newOMPRunFixture(t)
	id, started := fixture.prompt(t, "/local", "")
	fixture.peer.write(t, `{"type":"prompt_result","id":`+mustOMPJSONText(t, id)+`,"agentInvoked":false}`)
	start := <-started
	if start.err != nil || start.turn == nil {
		t.Fatalf("start = %#v, %v", start.turn, start.err)
	}
	result, err := start.turn.Wait(nativeRPCTestContext(t))
	if err != nil || result.Outcome != "completed" || result.Result != "" || result.NativeStopReason != "no_agent" {
		t.Fatalf("prompt_result no-agent = %+v, %v", result, err)
	}
}

type ompWorkerProduct struct{ *Wrapper }

func (product *ompWorkerProduct) Open(context.Context, kit.OpenRequest) (kit.OpenResult, error) {
	return kit.OpenResult{SessionID: product.binding.SessionID}, nil
}

func (*ompWorkerProduct) Close(context.Context, kit.SessionCloseRequest) error { return nil }

func TestOMPWorkerReportsSubmittedNoAgentDeliveryBeforeTerminal(t *testing.T) {
	fixture := newOMPRunFixture(t)
	listener, err := net.Listen("unix", filepath.Join(testsocket.Directory(t), "bus.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(host.SocketEnv, listener.Addr().String())
	t.Setenv(host.TokenEnv, "omp-worker-token")
	t.Setenv(host.LocalKeyEnv, "")
	worker := kit.NewWorker(&ompWorkerProduct{fixture.wrapper})
	fixture.wrapper.SetCaller(worker.Caller())
	served := make(chan error, 1)
	go func() { served <- worker.Serve(context.Background()) }()
	connection, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = listener.Close()
		worker.Shutdown()
		<-worker.Closed()
		<-served
	})
	reader := bufio.NewReader(connection)
	read := func() protocol.Frame {
		t.Helper()
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			t.Fatal(readErr)
		}
		frame, decodeErr := protocol.DecodeFrame(line[:len(line)-1])
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		return frame
	}
	write := func(body []byte) {
		t.Helper()
		if _, writeErr := connection.Write(body); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	hello := read()
	ack, err := protocol.ResultBytes(hello.ID, hello.Method, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	write(ack)
	open, err := protocol.RequestBytes(1, "session.open", kit.OpenRequest{Name: "managed@local", Groups: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	write(open)
	if frame := read(); frame.ID != 1 || frame.Error != nil {
		t.Fatalf("Open response = %+v", frame)
	}
	delivery, err := protocol.RequestBytes(2, "message.deliver", kit.DeliveryRequest{
		RunID: "g/1", MessageID: "delivery-no-agent", Body: "/local",
		From: kit.DeliverySource{SessionID: "sender@local", Product: "fixture", Groups: []string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	write(delivery)
	command := fixture.peer.read(t)
	id := nativeRPCField(t, command, "id")
	fixture.peer.write(t, `{"id":`+mustOMPJSONText(t, id)+`,"type":"response","command":"prompt","success":true,"data":{"agentInvoked":false}}`)
	receiptFrame := read()
	var receipt kit.DeliveryReceipt
	if receiptFrame.ID != 2 || receiptFrame.Error != nil ||
		protocol.UnmarshalResult("message.deliver", receiptFrame.Result, &receipt) != nil ||
		receipt.Disposition != "rejected" || receipt.Reason != "native_submission_refused" {
		t.Fatalf("no-agent delivery receipt = %+v, decoded %+v", receiptFrame, receipt)
	}
	terminal := read()
	if terminal.Method != "turn.ready" {
		t.Fatalf("frame after delivery receipt = %+v", terminal)
	}
	var ready struct {
		State   string `json:"state"`
		Outcome string `json:"outcome"`
	}
	if json.Unmarshal(terminal.Params, &ready) != nil || ready.State != "done" || ready.Outcome != "completed" {
		t.Fatalf("no-agent terminal = %s", terminal.Params)
	}
	ack, err = protocol.ResultBytes(terminal.ID, terminal.Method, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	write(ack)
}

func TestOMPRunRejectsForeignLeadingPreflight(t *testing.T) {
	fixture := newOMPRunFixture(t)
	_, started := fixture.prompt(t, "owned prompt", "")
	fixture.preflight(t, 1, "foreign", "other prompt")
	fixture.preflight(t, 2, "matching", "owned prompt")
	fixture.peer.write(t, `{"type":"agent_start"}`)
	start := <-started
	if start.err != nil || start.turn == nil {
		t.Fatalf("start = %#v, %v", start.turn, start.err)
	}
	if _, err := start.turn.Wait(nativeRPCTestContext(t)); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("foreign preflight result = %v", err)
	}
}

func TestOMPRunCancelsOwnedRuntimeUIAndJoinsWrite(t *testing.T) {
	fixture := newOMPRunFixture(t)
	_, started := fixture.prompt(t, "owned prompt", "")
	fixture.preflight(t, 1, "run-one", "owned prompt")
	fixture.peer.write(t, `{"type":"agent_start"}`)
	start := <-started
	if start.err != nil || start.turn == nil {
		t.Fatalf("start = %#v, %v", start.turn, start.err)
	}
	if err := fixture.wrapper.observeNative(json.RawMessage(`{"type":"extension_ui_request","id":"open-one","method":"open_url"}`)); err != nil {
		t.Fatalf("open_url notification = %v", err)
	}
	start.turn.mu.Lock()
	pendingBefore := start.turn.pendingUI
	start.turn.mu.Unlock()
	if pendingBefore != 0 {
		t.Fatal("open_url notification was treated as a pending dialog")
	}
	fixture.peer.write(t, `{"type":"extension_ui_request","id":"dialog-one","method":"confirm","title":"approve","message":"continue"}`)
	cancel := fixture.peer.read(t)
	if nativeRPCField(t, cancel, "type") != "extension_ui_response" || nativeRPCField(t, cancel, "id") != "dialog-one" || string(cancel["cancelled"]) != "true" {
		t.Fatalf("UI cancellation = %#v", cancel)
	}
	fixture.peer.write(t, `{"type":"message_end","message":{"role":"assistant","content":"","stopReason":"aborted"}}`)
	fixture.peer.write(t, `{"type":"agent_end","messages":[],"isTerminal":true}`)
	result, err := start.turn.Wait(nativeRPCTestContext(t))
	if err != nil || result.Outcome != "interrupted" || result.NativeStopReason != "aborted" {
		t.Fatalf("UI-canceled result = %+v, %v", result, err)
	}
}

func TestOMPTerminalCancelsAndJoinsUnsettledUIBeforeResult(t *testing.T) {
	for _, test := range []struct {
		name       string
		foreign    bool
		cancelWait bool
	}{
		{name: "wait context", cancelWait: true},
		{name: "foreign retirement", foreign: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOMPRunFixture(t)
			held, release := attachHeldOMPRPC(t, fixture)
			_, started := fixture.prompt(t, "owned prompt", "")
			fixture.preflight(t, 1, "run-one", "owned prompt")
			fixture.peer.write(t, `{"type":"agent_start"}`)
			start := <-started
			if start.err != nil || start.turn == nil {
				t.Fatalf("start = %#v, %v", start.turn, start.err)
			}

			held.enabled.Store(true)
			if err := fixture.wrapper.observeNative(json.RawMessage(`{"type":"extension_ui_request","id":"held-dialog","method":"confirm"}`)); err != nil {
				t.Fatal(err)
			}
			cancel := fixture.peer.read(t)
			if nativeRPCField(t, cancel, "type") != "extension_ui_response" || nativeRPCField(t, cancel, "id") != "held-dialog" {
				t.Fatalf("held UI cancellation = %#v", cancel)
			}
			<-held.wrote
			if err := start.turn.recordEvent("message_end", json.RawMessage(`{"type":"message_end","message":{"role":"assistant","content":"finished","stopReason":"stop"}}`)); err != nil {
				t.Fatal(err)
			}
			if err := start.turn.recordEvent("agent_end", json.RawMessage(`{"type":"agent_end","messages":[],"isTerminal":true}`)); err != nil {
				t.Fatal(err)
			}
			if test.foreign {
				retired := fixture.wrapper.observeNative(json.RawMessage(`{"type":"agent_start"}`))
				if retired == nil {
					t.Fatal("foreign post-terminal work was accepted")
				}
				fixture.rpc.stop(retired, false)
				<-fixture.rpc.ctx.Done()
			}
			waitCtx, cancelWait := context.WithCancel(nativeRPCTestContext(t))
			if test.cancelWait {
				cancelWait()
			} else {
				defer cancelWait()
			}
			waited := make(chan error, 1)
			go func() {
				_, err := start.turn.Wait(waitCtx)
				waited <- err
			}()
			var waitErr error
			select {
			case waitErr = <-waited:
			case <-nativeRPCTestContext(t).Done():
				t.Fatal("Wait did not cancel and join the unsettled UI operation")
			}
			if waitErr == nil || !errors.Is(waitErr, errOMPUIUnsettled) {
				t.Fatalf("unsettled UI result = %v", waitErr)
			}
			if test.cancelWait && !errors.Is(waitErr, context.Canceled) {
				t.Fatalf("Wait cancellation was lost: %v", waitErr)
			}
			start.turn.mu.Lock()
			pending := start.turn.pendingUI
			start.turn.mu.Unlock()
			if pending != 0 {
				t.Fatalf("Wait returned with %d owned UI operations", pending)
			}
			release()
			waitNativeRPCDone(t, fixture.rpc)
			waitNativeRPCStats(t, fixture.rpc, nativeRPCStats{})
		})
	}
}

func TestOMPManagedExtensionFailureAndPostTerminalWorkRetireRPC(t *testing.T) {
	fixture := newOMPRunFixture(t)
	ambient := `{"type":"extension_error","extensionPath":"/ambient.mjs","event":"agent_end","error":"ambient"}`
	if err := fixture.wrapper.observeNative(json.RawMessage(ambient)); err != nil {
		t.Fatalf("ambient extension error = %v", err)
	}
	managed := `{"type":"extension_error","extensionPath":` + mustOMPJSONText(t, fixture.wrapper.extension) + `,"event":"agent_end","error":"managed"}`
	if err := fixture.wrapper.observeNative(json.RawMessage(managed)); err == nil {
		t.Fatal("managed extension error outside Run was accepted")
	}
	inside := newOMPRunFixture(t)
	_, insideStarted := inside.prompt(t, "owned prompt", "")
	inside.preflight(t, 1, "run-one", "owned prompt")
	inside.peer.write(t, `{"type":"agent_start"}`)
	insideStart := <-insideStarted
	inside.peer.write(t, `{"type":"extension_error","extensionPath":`+mustOMPJSONText(t, inside.wrapper.extension)+`,"event":"before_agent_start","error":"managed failure"}`)
	if _, err := insideStart.turn.Wait(nativeRPCTestContext(t)); err == nil || !strings.Contains(err.Error(), "managed extension failed") {
		t.Fatalf("managed extension Run result = %v", err)
	}

	second := newOMPRunFixture(t)
	_, started := second.prompt(t, "owned prompt", "")
	second.preflight(t, 1, "run-one", "owned prompt")
	second.peer.write(t, `{"type":"agent_start"}`)
	start := <-started
	second.peer.write(t, `{"type":"message_end","message":{"role":"assistant","content":"done","stopReason":"stop"}}`)
	second.peer.write(t, `{"type":"agent_end","messages":[],"isTerminal":true}`)
	// A later foreign start retires future reuse without revoking the terminal
	// which was already snapshotted under the event lock.
	second.peer.write(t, `{"type":"agent_start"}`)
	waitNativeRPCDone(t, second.rpc)
	result, err := start.turn.Wait(nativeRPCTestContext(t))
	if err != nil || result.Outcome != "completed" || result.Result != "done" {
		t.Fatalf("cached terminal result = %+v, %v", result, err)
	}
	if second.rpc.Err() == nil || !strings.Contains(second.rpc.Err().Error(), "after the owned terminal") {
		t.Fatalf("post-terminal RPC error = %v", second.rpc.Err())
	}
	second.wrapper.mu.Lock()
	lost := second.wrapper.failure
	second.wrapper.mu.Unlock()
	if lost == nil || !strings.Contains(lost.Error(), "after the owned terminal") {
		t.Fatalf("post-terminal owner retirement = %v", lost)
	}
}

func TestOMPHeldCompletionWaitKeepsTerminalAcrossForeignRetirement(t *testing.T) {
	wrapper := &Wrapper{}
	wrapper.ctx, wrapper.cancel = context.WithCancelCause(context.Background())
	t.Cleanup(func() { wrapper.cancel(errNativeOwnerClosed) })
	turn := newOMPNativeTurn(wrapper, nil, "owned", OwnerBinding{})
	turn.native = &nativePrompt{pending: &nativeRPCPending{lateReady: make(chan struct{})}}
	held := &ompHeldDoneContext{Context: context.Background(), entered: make(chan struct{}), release: make(chan struct{})}
	waited := make(chan error, 1)
	go func() { waited <- turn.awaitCompletion(held) }()
	<-held.entered
	if err := turn.recordEvent("agent_start", json.RawMessage(`{"type":"agent_start"}`)); err != nil {
		t.Fatal(err)
	}
	if err := turn.recordEvent("message_end", json.RawMessage(`{"type":"message_end","message":{"role":"assistant","content":"owned result","stopReason":"stop"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := turn.recordEvent("agent_end", json.RawMessage(`{"type":"agent_end","isTerminal":true}`)); err != nil {
		t.Fatal(err)
	}
	retired := turn.recordEvent("agent_start", json.RawMessage(`{"type":"agent_start"}`))
	if retired == nil {
		t.Fatal("foreign post-terminal work was accepted")
	}
	wrapper.cancel(retired)
	close(held.release)
	if err := <-waited; err != nil {
		t.Fatalf("held terminal wait = %v", err)
	}
	if !turn.hasResult() || !errors.Is(turn.retirement, retired) {
		t.Fatalf("terminal/retirement = %v, %v", turn.hasResult(), turn.retirement)
	}
}

func TestOMPAssistantDecoderRejectsUnknownStopAndUsesTextParts(t *testing.T) {
	assistant, ok, err := decodeOMPAssistant(json.RawMessage(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hidden"},{"type":"text","text":"visible"}],"stopReason":"stop"}}`))
	if err != nil || !ok || assistant.text != "visible" {
		t.Fatalf("assistant = %+v, %v, %v", assistant, ok, err)
	}
	if _, _, err = decodeOMPAssistant(json.RawMessage(`{"type":"message_end","message":{"role":"assistant","content":"old","stopReason":"mystery"}}`)); err == nil {
		t.Fatal("unknown stop reason accepted")
	}
	if _, err = decodeOMPTerminal(json.RawMessage(`{"type":"agent_end","messages":[],"messageCount":2}`)); err == nil {
		t.Fatal("elided agent_end was accepted without terminal authority")
	}
	if terminal, err := decodeOMPTerminal(json.RawMessage(`{"type":"agent_end","isTerminal":false}`)); err != nil || terminal {
		t.Fatalf("nonterminal agent_end = %v, %v", terminal, err)
	}
	for name, body := range map[string]string{
		"null content": `{"type":"message_end","message":{"role":"assistant","content":null,"stopReason":"stop"}}`,
		"null text":    `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":null}],"stopReason":"stop"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeOMPAssistant(json.RawMessage(body)); err == nil {
				t.Fatal("null assistant text was accepted")
			}
		})
	}
}
