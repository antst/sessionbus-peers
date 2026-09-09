// SPDX-License-Identifier: MIT

package interactive

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

type writeConn struct {
	net.Conn
	write func([]byte) (int, error)
}

func (c writeConn) Write(b []byte) (int, error) { return c.write(b) }

func TestNativeWriteCapturesExactFrameAndCloses(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	frames := make(chan map[string]any, 1)
	closed := make(chan struct{})
	go func() {
		r := bufio.NewReader(b)
		line, _ := r.ReadBytes('\n')
		var f map[string]any
		_ = json.Unmarshal(line, &f)
		frames <- f
		_, _ = r.ReadByte()
		close(closed)
	}()
	receipt, err := DeliverNative(Recipient{SessionID: "captured-id", Socket: "native", Context: context.Background()}, kit.DeliveryRequest{MessageID: "message", From: kit.DeliverySource{SessionID: "sender"}, Body: "exact body"}, func(context.Context, string, string) (net.Conn, error) { return a, nil })
	if err != nil || receipt.Disposition != "written" {
		t.Fatalf("%#v %v", receipt, err)
	}
	f := <-frames
	if f["session_id"] != "captured-id" || f["from"] != "sender" || f["msg_id"] != "message" || f["msgV"] != float64(1) || f["message"].(map[string]any)["content"] != "exact body" {
		t.Fatal(f)
	}
	<-closed
}
func TestNativePreSubmissionAndUncertainBoundaries(t *testing.T) {
	for _, kind := range []string{"missing", "cancelled", "connect", "partial", "write-error", "cancel-during-write"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			captured := Recipient{SessionID: "id", Socket: "native", Context: ctx}
			if kind == "missing" {
				captured.Socket = ""
			}
			if kind == "cancelled" {
				cancel()
			}
			writes := 0
			dials := 0
			dial := func(context.Context, string, string) (net.Conn, error) {
				dials++
				if kind == "connect" {
					return nil, errors.New("connect failure")
				}
				a, b := net.Pipe()
				t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
				return writeConn{Conn: a, write: func(data []byte) (int, error) {
					writes++
					switch kind {
					case "partial":
						return len(data) - 1, nil
					case "cancel-during-write":
						cancel()
						return 0, io.ErrClosedPipe
					default:
						return 0, errors.New("write failure")
					}
				}}, nil
			}
			r, err := DeliverNative(captured, kit.DeliveryRequest{}, dial)
			if kind == "missing" || kind == "cancelled" || kind == "connect" {
				if err != nil || r.Disposition != "rejected" || writes != 0 {
					t.Fatalf("%#v %v writes%d", r, err, writes)
				}
				if kind != "connect" && dials != 0 {
					t.Fatal("unavailable delivery dialed")
				}
			} else {
				var p *kit.ProtocolError
				if !errors.As(err, &p) || p.Code != -32603 || string(p.Data) != `"uncertain_submission"` || r.Disposition != "" || writes != 1 {
					t.Fatalf("%#v %v writes%d", r, err, writes)
				}
			}
		})
	}
}
