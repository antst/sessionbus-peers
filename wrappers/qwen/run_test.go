// SPDX-License-Identifier: MIT
package qwen

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/antst/sessionbus-peers/internal/testsocket"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

type controlledLane struct {
	*Wrapper
	reportEntered, reportRelease chan struct{}
	admitted                     chan string
}

func (p *controlledLane) Deliver(ctx context.Context, d kit.DeliveryRequest, _ *kit.Run) (kit.DeliveryReceipt, error) {
	m, r, err := p.queueDelivery(ctx, d)
	if m == nil {
		return r, err
	}
	p.admitted <- d.Body
	return p.waitDelivery(ctx, m)
}
func (p *controlledLane) Open(context.Context, kit.OpenRequest) (kit.OpenResult, error) {
	return kit.OpenResult{SessionID: p.id}, nil
}
func (p *controlledLane) Close(context.Context, kit.SessionCloseRequest) error {
	p.cancel()
	p.client.close()
	<-p.client.done
	return nil
}
func (p *controlledLane) Run(ctx context.Context, r *kit.Run, seed kit.RunInput) (kit.TurnResult, error) {
	p.mu.Lock()
	p.run = r
	p.mu.Unlock()
	go p.retireRun(r)
	report := r.ReportDelivery
	if p.reportEntered != nil {
		report = func(value kit.DeliveryReceipt, err error) error {
			close(p.reportEntered)
			select {
			case <-p.reportRelease:
			case <-ctx.Done():
				return ctx.Err()
			}
			return r.ReportDelivery(value, err)
		}
	}
	return p.executeRun(ctx, r, seed, report)
}

type laneFixture struct {
	p         *controlledLane
	worker    *kit.Worker
	bus       net.Conn
	native    net.Conn
	reader    *bufio.Reader
	responses chan protocol.Frame
	ready     chan protocol.Frame
	writeMu   sync.Mutex
	nextID    int64
}

func newLaneFixture(t *testing.T, heldReport bool) *laneFixture {
	t.Helper()
	listener, err := net.Listen("unix", filepath.Join(testsocket.Directory(t), "bus.sock"))
	must(t, err)
	t.Setenv("SESSIONBUS_SOCKET", listener.Addr().String())
	t.Setenv("SESSIONBUS_LAUNCH_TOKEN", "qwen-test")
	t.Setenv("SESSIONBUS_LOCAL_KEY", "")
	local, native := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	base := &Wrapper{id: fixtureID, opened: true, ctx: ctx, cancel: cancel}
	base.client = newDuplexACP(local, local, base.receive, base.answer)
	product := &controlledLane{Wrapper: base, admitted: make(chan string, 256)}
	if heldReport {
		product.reportEntered = make(chan struct{})
		product.reportRelease = make(chan struct{})
	}
	worker := kit.NewWorker(product)
	product.SetCaller(worker.Caller())
	served := make(chan error, 1)
	go func() { served <- worker.Serve(context.Background()) }()
	bus, err := listener.Accept()
	must(t, err)
	f := &laneFixture{p: product, worker: worker, bus: bus, native: native, reader: bufio.NewReader(native), responses: make(chan protocol.Frame, 256), ready: make(chan protocol.Frame, 256)}
	hello := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		r := bufio.NewReader(bus)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			frame, err := protocol.DecodeFrame(line[:len(line)-1])
			if err != nil {
				panic(err)
			}
			if frame.Method != "" {
				body, err := protocol.ResultBytes(frame.ID, frame.Method, struct{}{})
				if err != nil {
					panic(err)
				}
				f.writeMu.Lock()
				_, err = bus.Write(body)
				f.writeMu.Unlock()
				if err != nil {
					return
				}
				if frame.Method == "session.hello" {
					close(hello)
				} else {
					f.ready <- frame
				}
			} else {
				f.responses <- frame
			}
		}
	}()
	t.Cleanup(func() {
		bus.Close()
		native.Close()
		base.client.close()
		cancel()
		listener.Close()
		<-served
		<-readerDone
	})
	<-hello
	f.send(t, "session.open", kit.OpenRequest{Name: "lane@local", Groups: []string{}, Policy: &kit.LanePolicy{IdleMessage: "run"}})
	if frame := f.response(t); frame.Error != nil {
		t.Fatal(frame.Error)
	}
	return f
}
func (f *laneFixture) send(t *testing.T, method string, params any) int64 {
	t.Helper()
	f.nextID++
	id := f.nextID
	body, err := protocol.RequestBytes(id, method, params)
	must(t, err)
	f.writeMu.Lock()
	_, err = f.bus.Write(body)
	f.writeMu.Unlock()
	must(t, err)
	return id
}
func (f *laneFixture) response(t *testing.T) protocol.Frame {
	t.Helper()
	frame := <-f.responses
	return frame
}
func (f *laneFixture) execute(t *testing.T, seq int, input string) acpFrame {
	t.Helper()
	f.send(t, "turn.execute", protocol.ExecuteRequest{SessionID: fixtureID + "@local", RunID: fmt.Sprintf("g/%d", seq), Input: input})
	if frame := f.response(t); frame.Error != nil {
		t.Fatal(frame.Error)
	}
	return acpRead(t, f.reader)
}
func (f *laneFixture) terminal(t *testing.T, prompt acpFrame, reason string) {
	t.Helper()
	acpWrite(t, f.native, `{"jsonrpc":"2.0","id":`+string(prompt.ID)+`,"result":{"stopReason":"`+reason+`"}}`)
}
func (f *laneFixture) status(t *testing.T, seq int) kit.RunStatus {
	t.Helper()
	f.send(t, "turn.status", kit.ReadRequest{SessionID: fixtureID + "@local", RunID: fmt.Sprintf("g/%d", seq)})
	frame := f.response(t)
	var status kit.RunStatus
	must(t, protocol.UnmarshalResult("turn.status", frame.Result, &status))
	return status
}
func (f *laneFixture) chunk(t *testing.T, id, text string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": id, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}}}})
	acpWrite(t, f.native, string(body))
}

func TestWorkerSeedWrittenBeforeTerminalAndNonconsumingCursor(t *testing.T) {
	f := newLaneFixture(t, false)
	idleObserved := make(chan struct{})
	original := f.p.client.notify
	f.p.client.notify = func(method string, raw json.RawMessage) {
		original(method, raw)
		if method == "fixture/idle" {
			close(idleObserved)
		}
	}
	d := delivery("wake-marker")
	d.RunID = "g/1"
	f.send(t, "message.deliver", d)
	prompt := acpRead(t, f.reader)
	if prompt.Method != "session/prompt" || !strings.Contains(string(prompt.Params), "wake-marker") {
		t.Fatal(prompt)
	}
	frame := f.response(t)
	var receipt kit.DeliveryReceipt
	must(t, protocol.UnmarshalResult("message.deliver", frame.Result, &receipt))
	if receipt.Disposition != "written" {
		t.Fatal(receipt)
	}
	f.chunk(t, "foreign", "wrong")
	f.chunk(t, fixtureID, "answer")
	f.terminal(t, prompt, "end_turn")
	<-f.ready
	f.chunk(t, fixtureID, "late-idle")
	acpWrite(t, f.native, `{"jsonrpc":"2.0","method":"fixture/idle","params":{}}`)
	<-idleObserved
	for range 2 {
		status := f.status(t, 1)
		if status.State != "done" || status.Result.Result != "answer" {
			t.Fatal(status)
		}
	}
	f.send(t, "turn.ack", kit.RunRef{SessionID: fixtureID + "@local", RunID: "g/1"})
	if frame = f.response(t); frame.Error != nil {
		t.Fatal(frame.Error)
	}
	next := f.execute(t, 2, "next")
	f.terminal(t, next, "end_turn")
	<-f.ready
	if status := f.status(t, 2); status.Result.Result != "" {
		t.Fatal(status)
	}
}
func TestWorkerTerminalWhileSeedReceiptHeld(t *testing.T) {
	for _, loss := range []bool{false, true} {
		t.Run(fmt.Sprint(loss), func(t *testing.T) {
			f := newLaneFixture(t, true)
			d := delivery("seed")
			d.RunID = "g/1"
			f.send(t, "message.deliver", d)
			prompt := acpRead(t, f.reader)
			<-f.p.reportEntered
			f.chunk(t, fixtureID, "answer")
			f.terminal(t, prompt, "end_turn")
			// A following native request proves the reader passed the terminal while
			// the Run goroutine remains held at the real receipt boundary.
			acpWrite(t, f.native, `{"jsonrpc":"2.0","id":99,"method":"unknown","params":{}}`)
			acpRead(t, f.reader)
			if loss {
				f.bus.Close()
				<-f.worker.Closed()
				return
			}
			close(f.p.reportRelease)
			if frame := f.response(t); frame.Error != nil {
				t.Fatal(frame.Error)
			}
			<-f.ready
			if status := f.status(t, 1); status.Result.Result != "answer" {
				t.Fatal(status)
			}
		})
	}
}
func TestWorkerNativeDrainFirstTenAndNoPullStage(t *testing.T) {
	f := newLaneFixture(t, false)
	prompt := f.execute(t, 1, "active")
	for i := 0; i < 12; i++ {
		f.send(t, "message.deliver", delivery(fmt.Sprintf("item-%02d", i)))
		if got := <-f.p.admitted; got != fmt.Sprintf("item-%02d", i) {
			t.Fatal(got)
		}
	}

	acpWrite(t, f.native, `{"jsonrpc":"2.0","id":90,"method":"craft/drainMidTurnQueue","params":{"sessionId":"`+fixtureID+`"}}`)
	response := acpRead(t, f.reader)
	var drained struct {
		Messages        []string `json:"messages"`
		HasQueuedPrompt bool     `json:"hasQueuedPrompt"`
	}
	must(t, json.Unmarshal(response.Result, &drained))
	if len(drained.Messages) != 10 || drained.HasQueuedPrompt {
		t.Fatal(string(response.Result))
	}
	for i, m := range drained.Messages {
		if !strings.Contains(m, fmt.Sprintf("item-%02d", i)) {
			t.Fatal(drained.Messages)
		}
	}
	for range 10 {
		frame := f.response(t)
		var r kit.DeliveryReceipt
		must(t, protocol.UnmarshalResult("message.deliver", frame.Result, &r))
		if r.Disposition != "written" {
			t.Fatal(r)
		}
	}
	f.terminal(t, prompt, "end_turn")
	for range 2 {
		frame := f.response(t)
		var r kit.DeliveryReceipt
		must(t, protocol.UnmarshalResult("message.deliver", frame.Result, &r))
		if r.Disposition != "queued_for_next_turn" {
			t.Fatal(r)
		}
	}
	<-f.ready
	next := f.execute(t, 2, "next")
	if strings.Contains(string(next.Params), "item-00") || !strings.Contains(string(next.Params), "item-10") || !strings.Contains(string(next.Params), "item-11") {
		t.Fatal(string(next.Params))
	}
	f.terminal(t, next, "end_turn")
	<-f.ready
}
func TestWorkerCancelAndTerminalBothOrdersHealthyNext(t *testing.T) {
	for _, terminalFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(terminalFirst), func(t *testing.T) {
			f := newLaneFixture(t, false)
			prompt := f.execute(t, 1, "hold")
			f.send(t, "turn.interrupt", map[string]string{"session_id": fixtureID + "@local"})
			if frame := f.response(t); frame.Error != nil {
				t.Fatal(frame.Error)
			}
			cancel := acpRead(t, f.reader)
			if cancel.Method != "craft/cancelPendingPrompt" {
				t.Fatal(cancel.Method)
			}
			if terminalFirst {
				f.terminal(t, prompt, "cancelled")
			}
			acpWrite(t, f.native, `{"jsonrpc":"2.0","id":`+string(cancel.ID)+`,"result":{"cancelled":true}}`)
			if !terminalFirst {
				f.terminal(t, prompt, "cancelled")
			}
			<-f.ready
			if status := f.status(t, 1); status.Result.Outcome != "interrupted" {
				t.Fatal(status)
			}
			next := f.execute(t, 2, "next")
			if next.Method != "session/prompt" {
				t.Fatal(next)
			}
			f.terminal(t, next, "end_turn")
			<-f.ready
		})
	}
}

func TestWorkerStageSurvivesPrewriteFrameRefusal(t *testing.T) {
	f := newLaneFixture(t, false)
	f.send(t, "message.deliver", delivery("staged-marker-"+strings.Repeat("<", 120000)))
	frame := f.response(t)
	var receipt kit.DeliveryReceipt
	must(t, protocol.UnmarshalResult("message.deliver", frame.Result, &receipt))
	if receipt.Disposition != "queued_for_next_turn" {
		t.Fatal(receipt)
	}
	f.send(t, "turn.execute", protocol.ExecuteRequest{SessionID: fixtureID + "@local", RunID: "g/1", Input: strings.Repeat(`"`, 170000)})
	if frame = f.response(t); frame.Error != nil {
		t.Fatal(frame.Error)
	}
	<-f.ready
	if status := f.status(t, 1); status.State != "unavailable" {
		t.Fatal(status)
	}
	next := f.execute(t, 2, "next")
	if !strings.Contains(string(next.Params), "staged-marker-") {
		t.Fatal("prewrite validation lost staged input")
	}
	f.terminal(t, next, "end_turn")
	<-f.ready
}
func TestWorkerHeldDrainWriteJoinsTerminalWithoutReplay(t *testing.T) {
	f := newLaneFixture(t, false)
	barrier := make(chan struct{})
	original := f.p.client.notify
	f.p.client.notify = func(method string, params json.RawMessage) {
		original(method, params)
		if method == "fixture/barrier" {
			close(barrier)
		}
	}
	prompt := f.execute(t, 1, "active")
	f.send(t, "message.deliver", delivery("write-owned-marker"))
	<-f.p.admitted
	acpWrite(t, f.native, `{"jsonrpc":"2.0","id":91,"method":"craft/drainMidTurnQueue","params":{"sessionId":"`+fixtureID+`"}}`)
	// Leave native's response unread. The same reader must still observe terminal.
	f.terminal(t, prompt, "end_turn")
	acpWrite(t, f.native, `{"jsonrpc":"2.0","method":"fixture/barrier","params":{}}`)
	<-barrier
	f.p.mu.Lock()
	terminal, batch := f.p.active.terminal, f.p.active.batch
	f.p.mu.Unlock()
	if !terminal || batch == nil {
		t.Fatal("terminal did not retain its admitted drain write")
	}
	select {
	case <-f.ready:
		t.Fatal("run retired before admitted write settled")
	default:
	}
	response := acpRead(t, f.reader)
	if !strings.Contains(string(response.Result), "write-owned-marker") {
		t.Fatal(string(response.Result))
	}
	frame := f.response(t)
	var receipt kit.DeliveryReceipt
	must(t, protocol.UnmarshalResult("message.deliver", frame.Result, &receipt))
	if receipt.Disposition != "written" {
		t.Fatal(receipt)
	}
	<-f.ready
	next := f.execute(t, 2, "next")
	if strings.Contains(string(next.Params), "write-owned-marker") {
		t.Fatal("submitted drain input replayed")
	}
	f.terminal(t, next, "end_turn")
	<-f.ready
}
