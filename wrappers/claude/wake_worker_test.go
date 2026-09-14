// SPDX-License-Identifier: MIT
package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"

	"github.com/antst/sessionbus-peers/internal/testsocket"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

type wakeWorkerProduct struct {
	*workerStreamProduct
	reportEntered, reportRelease chan struct{}
	returned                     chan error
}

func (p *wakeWorkerProduct) Run(ctx context.Context, r *kit.Run, input kit.RunInput) (kit.TurnResult, error) {
	if p.reportEntered == nil {
		result, err := p.Wrapper.Run(ctx, r, input)
		p.returned <- err
		return result, err
	}
	// A controlled call boundary before the actual public ReportDelivery, not a
	// claim that a real Unix socket write is blocked. stream tests cover that I/O.
	p.mu.Lock()
	p.activeDone = r.Done()
	p.mu.Unlock()
	body, err := deliveryInput(*input.Delivery)
	if err != nil {
		return kit.TurnResult{}, err
	}
	result, err := p.stream.execute(ctx, "native-id", body, r.Admitted, func(receipt kit.DeliveryReceipt, failure error) error {
		close(p.reportEntered)
		<-p.reportRelease
		return r.ReportDelivery(receipt, failure)
	})
	p.returned <- err
	return result, err
}

func TestActualWorkerWakeReceiptTerminalAndBusLoss(t *testing.T) {
	for _, mode := range []string{"direct-wrapper", "terminal-before-receipt", "bus-loss-at-report"} {
		t.Run(mode, func(t *testing.T) {
			loss := mode == "bus-loss-at-report"
			listener, err := net.Listen("unix", filepath.Join(testsocket.Directory(t), "bus.sock"))
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			t.Setenv("SESSIONBUS_SOCKET", listener.Addr().String())
			t.Setenv("SESSIONBUS_LAUNCH_TOKEN", "wake-test")
			t.Setenv("SESSIONBUS_LOCAL_KEY", "")
			f := streamFixture(t)
			base := New(t.TempDir())
			base.ctx, base.cancel = context.WithCancel(context.Background())
			base.stream = f.s
			p := &wakeWorkerProduct{workerStreamProduct: &workerStreamProduct{base}, reportEntered: make(chan struct{}), reportRelease: make(chan struct{}), returned: make(chan error, 1)}
			if mode == "direct-wrapper" {
				p.reportEntered = nil
				p.reportRelease = nil
			}
			worker := kit.NewWorker(p)
			base.SetCaller(worker.Caller())
			base.SetShutdown(worker.Shutdown)
			done := make(chan error, 1)
			go func() { done <- worker.Serve(context.Background()) }()
			c, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = c.Close(); base.cancel() })
			decoder := bufio.NewReader(c)
			next := func() protocol.Frame {
				t.Helper()
				line, e := decoder.ReadBytes('\n')
				if e != nil {
					t.Fatal(e)
				}
				frame, e := protocol.DecodeFrame(line[:len(line)-1])
				if e != nil {
					t.Fatal(e)
				}
				return frame
			}
			send := func(b []byte, e error) {
				t.Helper()
				if e != nil {
					t.Fatal(e)
				}
				if _, e = c.Write(b); e != nil {
					t.Fatal(e)
				}
			}
			hello := next()
			send(protocol.ResultBytes(hello.ID, "session.hello", struct{}{}))
			send(protocol.RequestBytes(1, "session.open", kit.OpenRequest{Name: "parent/child@local", Groups: []string{}, Policy: &kit.LanePolicy{IdleMessage: "run"}}))
			_ = next()
			delivery := kit.DeliveryRequest{MessageID: "m", RunID: "g/1", Body: "authorized message", From: kit.DeliverySource{SessionID: "parent@local", Product: "claude-peer", Groups: []string{"g"}}}
			send(protocol.RequestBytes(2, "message.deliver", delivery))
			frame := f.next(t)
			id := rawString(t, frame["uuid"])
			if string(frame["shouldQuery"]) != "true" {
				t.Fatal(frame)
			}
			var envelope struct {
				From    kit.DeliverySource `json:"from"`
				Message string             `json:"message"`
			}
			var message struct{ Content string }
			if json.Unmarshal(frame["message"], &message) != nil || json.Unmarshal([]byte(message.Content), &envelope) != nil || envelope.From.SessionID != delivery.From.SessionID || envelope.Message != delivery.Body {
				t.Fatal("delivery source or body lost")
			}
			f.send(t, map[string]any{"type": "user", "session_id": "native-id", "uuid": id, "isReplay": true})
			if p.reportEntered != nil {
				<-p.reportEntered
			}
			f.send(t, map[string]any{"type": "result", "session_id": "native-id", "subtype": "success", "result": "retained wake answer", "user_message_uuid": id})
			f.barrier(t)
			if loss {
				_ = c.Close()
				close(p.reportRelease)
				if e := <-p.returned; e == nil {
					t.Fatal("lost bus receipt succeeded")
				}
				<-done
				return
			}
			if p.reportRelease != nil {
				close(p.reportRelease)
			}
			receipt := next()
			var r kit.DeliveryReceipt
			if receipt.ID != 2 || protocol.UnmarshalResult("message.deliver", receipt.Result, &r) != nil || r.Disposition != "injected" {
				t.Fatal(receipt)
			}
			ready := next()
			if ready.Method != "turn.ready" {
				t.Fatal(ready)
			}
			send(protocol.ResultBytes(ready.ID, "turn.ready", struct{}{}))
			// Two independent wire reads demonstrate non-consuming Worker retention.
			for _, callID := range []int64{3, 4} {
				send(protocol.RequestBytes(callID, "turn.status", kit.ReadRequest{SessionID: "native-id@local", RunID: "g/1"}))
				status := next()
				var result kit.RunStatus
				if protocol.UnmarshalResult("turn.status", status.Result, &result) != nil || result.Result == nil || result.Result.Result != "retained wake answer" {
					t.Fatal(status)
				}
			}
			send(protocol.RequestBytes(5, "turn.ack", kit.RunRef{SessionID: "native-id@local", RunID: "g/1"}))
			if ack := next(); ack.ID != 5 || ack.Error != nil {
				t.Fatal(ack)
			}
			if e := <-p.returned; e != nil {
				t.Fatal(e)
			}
			_ = c.Close()
			<-done
		})
	}
}
