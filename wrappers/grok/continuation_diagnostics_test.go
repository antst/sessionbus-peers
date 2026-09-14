// SPDX-License-Identifier: MIT
package grok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

type continuationACPRead struct {
	frame acpFrame
	err   error
}

type continuationDeliveryResult struct {
	receipt kit.DeliveryReceipt
	err     error
}

// Each stream has exactly one decoder owner. The one-element channels allow
// the test to select native submission against an early delivery result without
// abandoning a read or racing a later status/receipt reader. Cleanup must close
// the actual streams after stop and before join.
func continuationReaders(bus *workerReader, observer *json.Decoder) (<-chan continuationACPRead, func(), func()) {
	stop := make(chan struct{})
	busDone, observerDone := make(chan struct{}), make(chan struct{})
	busFrames := make(chan workerRead, 1)
	observerFrames := make(chan continuationACPRead, 1)
	bus.frames = busFrames
	go func() {
		defer close(busDone)
		defer close(busFrames)
		for {
			var next workerRead
			line, err := bus.reader.ReadBytes('\n')
			next.err = err
			if err == nil {
				next.err = json.Unmarshal(line, &next.response)
			}
			select {
			case busFrames <- next:
			case <-stop:
				return
			}
			if next.err != nil {
				return
			}
		}
	}()
	go func() {
		defer close(observerDone)
		defer close(observerFrames)
		for {
			var next continuationACPRead
			next.err = observer.Decode(&next.frame)
			select {
			case observerFrames <- next:
			case <-stop:
				return
			}
			if next.err != nil {
				return
			}
		}
	}()
	return observerFrames, func() { close(stop) }, func() { <-busDone; <-observerDone }
}

func (h *continuationHarness) readObserver(t *testing.T) acpFrame {
	t.Helper()
	next, ok := <-h.observerRead
	if !ok {
		t.Fatal("native observer frame reader closed")
	}
	must(t, next.err)
	return next.frame
}

func (h *continuationHarness) expectInterject(t *testing.T, deliveryID int, direct <-chan continuationDeliveryResult) acpFrame {
	t.Helper()
	f, err := h.interjectOrDelivery(t, deliveryID, direct)
	must(t, err)
	check(t, f.Method == "_x.ai/interject", "not native interject: %+v", f)
	return f
}

// All bus maps remain owned by the calling test goroutine. Responses for other
// requests and turn.ready notifications keep the existing correlation/ack path.
func (h *continuationHarness) interjectOrDelivery(t *testing.T, deliveryID int, direct <-chan continuationDeliveryResult) (acpFrame, error) {
	t.Helper()
	if response, ok := h.bus.pending[deliveryID]; deliveryID != 0 && ok {
		return acpFrame{}, earlyWorkerDelivery(response)
	}
	for {
		select {
		case next, ok := <-h.observerRead:
			if !ok {
				return acpFrame{}, fmt.Errorf("native observer closed before interject: %w", io.EOF)
			}
			return next.frame, next.err
		case next, ok := <-h.bus.frames:
			if !ok {
				return acpFrame{}, fmt.Errorf("worker closed before interject: %w", io.EOF)
			}
			if next.err != nil {
				return acpFrame{}, fmt.Errorf("worker read before interject: %w", next.err)
			}
			response := next.response
			if workerReady(t, h.bus, response) {
				continue
			}
			response.ID = h.bus.requestIDs[response.ID]
			h.bus.pending[response.ID] = response
			if deliveryID != 0 && response.ID == deliveryID {
				return acpFrame{}, earlyWorkerDelivery(response)
			}
		case result := <-direct:
			return acpFrame{}, fmt.Errorf("Deliver returned before native interject: receipt=%+v error=%v", result.receipt, result.err)
		}
	}
}

func earlyWorkerDelivery(response workerResponse) error {
	return fmt.Errorf("Worker delivery %d returned before native interject: result=%s error=%s", response.ID, response.Result, response.Error)
}

func TestContinuationDiagnosticsSurfaceEarlyWorkerDelivery(t *testing.T) {
	h := newContinuationHarness(t)
	// The actual Worker has no Run. Its legitimate queued response must diagnose
	// a test expecting interject instead of stranding the observer read.
	writeWorkerRequest(t, h.bus, 2, "turn.status", map[string]string{"session_id": testSessionID + "@local"})
	writeWorkerRequest(t, h.bus, 3, "message.deliver", delivery("idle"))
	_, err := h.interjectOrDelivery(t, 3, nil)
	check(t, err != nil && strings.Contains(err.Error(), "queued_for_next_turn"), "missing actual Worker result: %v", err)
	status := readWorkerResponse(t, h.bus, 2)
	check(t, status.ID == 2 && status.Error != nil, "missing-Run status error lost: %+v", status)
	var receipt kit.DeliveryReceipt
	must(t, json.Unmarshal(readWorkerResponse(t, h.bus, 3).Result, &receipt))
	check(t, receipt.Disposition == "queued_for_next_turn", "diagnostic consumed original receipt")
}

func TestContinuationDiagnosticsSurfaceEarlyDirectDelivery(t *testing.T) {
	h := newContinuationHarness(t)
	returned := make(chan continuationDeliveryResult, 1)
	go func() {
		r, err := h.p.Deliver(context.Background(), delivery("idle"), nil)
		returned <- continuationDeliveryResult{r, err}
	}()
	_, err := h.interjectOrDelivery(t, 0, returned)
	check(t, err != nil && strings.Contains(err.Error(), "queued_for_next_turn"), "missing actual direct result: %v", err)
}

func TestContinuationDiagnosticsObserveNativeFrame(t *testing.T) {
	h := newContinuationHarness(t)
	done := make(chan error, 1)
	go func() { done <- h.p.observer.request(context.Background(), "_x.ai/interject", nil, nil) }()
	f := h.expectInterject(t, 0, nil)
	replyACP(t, h.observerWrite, f, map[string]any{})
	must(t, <-done)
}

func TestContinuationDiagnosticsObserveNativeEOF(t *testing.T) {
	h := newContinuationHarness(t)
	h.p.observer.close()
	_, err := h.interjectOrDelivery(t, 0, nil)
	check(t, err != nil, "native EOF accepted as interject")
}
