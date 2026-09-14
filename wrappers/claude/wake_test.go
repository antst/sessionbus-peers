// SPDX-License-Identifier: MIT
package claude

import (
	"context"
	"errors"
	"io"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestWakeReplayFixesReceiptWhileTerminalDrains(t *testing.T) {
	f := streamFixture(t)
	receipt := make(chan kit.DeliveryReceipt, 1)
	release := make(chan struct{})
	done := make(chan runResult, 1)
	go func() {
		v, err := f.s.execute(context.Background(), "native-id", "message body", func() {}, func(r kit.DeliveryReceipt, err error) error {
			if err != nil {
				return err
			}
			receipt <- r
			<-release
			return nil
		})
		done <- runResult{v, err}
	}()
	frame := f.next(t)
	id := rawString(t, frame["uuid"])
	if string(frame["shouldQuery"]) != "true" || rawString(t, frame["session_id"]) != "native-id" {
		t.Fatal("waking message did not use the native query path")
	}
	f.send(t, map[string]any{"type": "user", "session_id": "foreign", "uuid": id, "isReplay": true})
	f.barrier(t)
	select {
	case <-receipt:
		t.Fatal("uncorrelated replay admitted waking delivery")
	default:
	}
	f.send(t, map[string]any{"type": "user", "session_id": "native-id", "uuid": id, "isReplay": true})
	if r := <-receipt; r.Disposition != "injected" {
		t.Fatal(r)
	}
	f.send(t, map[string]any{"type": "result", "session_id": "native-id", "subtype": "success", "result": "wake result", "user_message_uuid": id})
	// Receipt transport remains blocked. The actual native reader must still
	// process the terminal and this subsequent correlated control response.
	f.barrier(t)
	select {
	case <-done:
		t.Fatal("Run returned before its delivery reply completed")
	default:
	}
	close(release)
	if r := <-done; r.err != nil || r.value.Result != "wake result" || r.value.Outcome != "completed" {
		t.Fatalf("%+v", r)
	}
}

func TestWakeReceiptTransportFailureRetiresNativeStream(t *testing.T) {
	f := streamFixture(t)
	reader, writer := io.Pipe()
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := f.s.execute(context.Background(), "native-id", "message", func() {}, func(kit.DeliveryReceipt, error) error {
			close(entered)
			_, err := writer.Write([]byte("receipt"))
			return err
		})
		done <- err
	}()
	frame := f.next(t)
	f.send(t, map[string]any{"type": "user", "session_id": "native-id", "uuid": rawString(t, frame["uuid"]), "isReplay": true})
	<-entered
	f.barrier(t)
	_ = reader.CloseWithError(io.ErrClosedPipe)
	if err := <-done; !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	<-f.s.done
	_ = writer.Close()
}

func TestWakeWithoutReplayPreservesSubmissionUncertainty(t *testing.T) {
	for _, submitted := range []bool{false, true} {
		t.Run(map[bool]string{false: "pre-cancel", true: "after-write"}[submitted], func(t *testing.T) {
			f := streamFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !submitted {
				cancel()
			}
			type answer struct {
				r kit.DeliveryReceipt
				e error
			}
			receipt := make(chan answer, 1)
			done := make(chan error, 1)
			go func() {
				_, err := f.s.execute(ctx, "native-id", "message", func() {}, func(r kit.DeliveryReceipt, err error) error {
					receipt <- answer{r, err}
					return nil
				})
				done <- err
			}()
			if submitted {
				_ = f.next(t)
				f.barrier(t)
				cancel()
			}
			r := <-receipt
			if submitted {
				var protocol *kit.ProtocolError
				if !errors.As(r.e, &protocol) || protocol.Code != -32603 || r.r.Disposition != "" {
					t.Fatalf("attempted input was not uncertain: %+v", r)
				}
			} else if r.e != nil || r.r.Disposition != "rejected" || r.r.Reason != "not_submitted" {
				t.Fatalf("unsubmitted input: %+v", r)
			}
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
