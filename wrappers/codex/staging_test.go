// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestNativeStageRequiresAckAndPreservesRunBoundary(t *testing.T) {
	for _, mode := range []string{"idle", "start", "start-terminal", "after-ack"} {
		t.Run(mode, func(t *testing.T) {
			p, server := testLane(t)
			type result struct {
				receipt kit.DeliveryReceipt
				err     error
			}
			done := make(chan result, 1)
			request := kit.DeliveryRequest{MessageID: "m", Body: "staged marker", From: kit.DeliverySource{SessionID: "sender@local", Product: "codex-peer", Groups: []string{"g"}}}
			go func() { r, e := p.Deliver(context.Background(), request, nil); done <- result{r, e} }()
			call := readAppRequest(t, server)
			if call.Method != "thread/inject_items" {
				t.Fatal(call)
			}
			var body struct {
				ThreadID string
				Items    []struct {
					Type, Role string
					Content    []struct{ Type, Text string }
				}
			}
			if json.Unmarshal(call.Params, &body) != nil || body.ThreadID != "thread-1" || len(body.Items) != 1 || body.Items[0].Role != "user" || body.Items[0].Type != "message" || len(body.Items[0].Content) != 1 || body.Items[0].Content[0].Type != "input_text" {
				t.Fatalf("items=%s", call.Params)
			}
			select {
			case r := <-done:
				t.Fatalf("write fabricated receipt: %+v", r)
			default:
			}
			if mode == "start" || mode == "start-terminal" {
				writeRaw(t, server, `{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"external","status":"inProgress"}}}`)
				if mode == "start-terminal" {
					writeRaw(t, server, `{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"external","status":"completed"}}}`)
				}
			}
			writeApp(t, server, map[string]any{"id": call.ID, "result": map[string]any{}})
			if mode == "after-ack" {
				writeRaw(t, server, `{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"later","status":"inProgress"}}}`)
			}
			got := <-done
			if mode == "idle" || mode == "after-ack" {
				if got.err != nil || got.receipt.Disposition != "queued_for_next_turn" {
					t.Fatalf("%+v", got)
				}
			} else {
				var e *kit.ProtocolError
				if !errors.As(got.err, &e) || e.Code != -32603 || string(e.Data) != `"uncertain_native_admission"` || got.receipt.Disposition != "" {
					t.Fatalf("%+v", got)
				}
			}
		})
	}
}
