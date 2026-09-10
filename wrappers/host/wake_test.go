// SPDX-License-Identifier: MIT
package host_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/antst/sessionbus-peers/internal/testsocket"
	"github.com/antst/sessionbus-peers/wrappers/opencode"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

// Only Open/Close are controlled. Hello and Run dispatch to the actual wrapper.
// A nil product receiver makes any access to native runtime state fail, rather
// than accidentally starting a product if the pre-native guard regresses.
type unsupportedWakeProduct struct {
	kit.WorkerCallbacks
	opens atomic.Int32
}

func (p *unsupportedWakeProduct) Open(context.Context, kit.OpenRequest) (kit.OpenResult, error) {
	p.opens.Add(1)
	return kit.OpenResult{SessionID: "fixture-id"}, nil
}
func (*unsupportedWakeProduct) Close(context.Context, kit.SessionCloseRequest) error { return nil }

func TestUnimplementedWakeRejectedBeforeProductWork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		product kit.WorkerCallbacks
	}{
		{"opencode", (*opencode.Wrapper)(nil)},
	} {
		for _, mode := range []string{"reject-wake", "stage"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				p := &unsupportedWakeProduct{WorkerCallbacks: tc.product}
				description, err := p.Hello(context.Background())
				if err != nil || description.SupportsMessageRun {
					t.Fatalf("unsupported wake advertised: %+v / %v", description, err)
				}
				listener, err := net.Listen("unix", filepath.Join(testsocket.Directory(t), "bus.sock"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = listener.Close() })
				t.Setenv("SESSIONBUS_SOCKET", listener.Addr().String())
				t.Setenv("SESSIONBUS_LAUNCH_TOKEN", "unsupported-wake-test")
				t.Setenv("SESSIONBUS_LOCAL_KEY", "")
				worker := kit.NewWorker(p)
				go func() { _ = worker.Serve(context.Background()) }()
				c, err := listener.Accept()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = c.Close(); <-worker.Closed() })
				reader := bufio.NewReader(c)
				receive := func() protocol.Frame {
					t.Helper()
					line, e := reader.ReadBytes('\n')
					if e != nil {
						t.Fatal(e)
					}
					f, e := protocol.DecodeFrame(line[:len(line)-1])
					if e != nil {
						t.Fatal(e)
					}
					return f
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
				hello := receive()
				decoded, err := protocol.DecodeParams("session.hello", hello.Params)
				if err != nil {
					t.Fatal(err)
				}
				if decoded.(*protocol.WorkerHello).SupportsMessageRun {
					t.Fatal("wire hello advertised wake")
				}
				send(protocol.ResultBytes(hello.ID, "session.hello", struct{}{}))
				if mode == "reject-wake" {
					send(protocol.RequestBytes(1, "session.open", kit.OpenRequest{Name: "parent/fixture@local", Groups: []string{}, Policy: &kit.LanePolicy{IdleMessage: "run"}}))
					refused := receive()
					if refused.ID != 1 || refused.Error == nil || refused.Error.Code != protocol.UnsupportedOpen || p.opens.Load() != 0 {
						t.Fatalf("wake Open reached product: %+v, opens=%d", refused, p.opens.Load())
					}
					return
				}
				// An independent worker gets one default-stage Open attempt.
				// Failed Open retry on the same worker is not a product contract.
				send(protocol.RequestBytes(2, "session.open", kit.OpenRequest{Name: "parent/fixture@local", Groups: []string{}}))
				opened := receive()
				if opened.ID != 2 || opened.Error != nil || p.opens.Load() != 1 {
					t.Fatal(opened)
				}
				// A deliberately misrouted private seed exercises the wrapper's defensive
				// Run guard through a real kit Run.ReportDelivery, not a fabricated Run value.
				delivery := kit.DeliveryRequest{MessageID: "message", RunID: "g/1", Body: "do not submit", From: kit.DeliverySource{SessionID: "parent@local", Product: "fixture", Groups: []string{}}}
				send(protocol.RequestBytes(3, "message.deliver", delivery))
				answer := receive()
				var receipt kit.DeliveryReceipt
				if answer.ID != 3 || protocol.UnmarshalResult("message.deliver", answer.Result, &receipt) != nil || receipt.Disposition != "rejected" || receipt.Reason != "unsupported_delivery_seed" {
					t.Fatal(answer)
				}
				ready := receive()
				var state protocol.TurnReady
				if ready.Method != "turn.ready" || json.Unmarshal(ready.Params, &state) != nil || state.State != "unavailable" || state.Outcome != "" || state.Reason != "delivery-seeded runs are not supported by this product" {
					t.Fatalf("fabricated native terminal: %+v", ready)
				}
				send(protocol.ResultBytes(ready.ID, "turn.ready", struct{}{}))
				send(protocol.RequestBytes(4, "session.close", kit.SessionCloseRequest{SessionID: "fixture-id@local"}))
				closed := receive()
				if closed.ID != 4 || closed.Error != nil {
					t.Fatal(closed)
				}
			})
		}
	}
}
