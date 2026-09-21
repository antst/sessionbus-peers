// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"errors"
	"strings"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestIdleDeliveryDefersBeforeNativeSubmission(t *testing.T) {
	p, _ := testLane(t)
	receipt, err := p.Deliver(context.Background(), kit.DeliveryRequest{
		MessageID: "m",
		Body:      "must wake in a managed run",
		From:      kit.DeliverySource{SessionID: "sender@local", Product: "codex-peer", Groups: []string{"g"}},
	}, nil)
	var protocolError *kit.ProtocolError
	if !errors.As(err, &protocolError) || protocolError.Code != -32004 || receipt.Disposition != "" {
		t.Fatalf("delivery = %+v, %v", receipt, err)
	}
}

func TestActiveDeliveryUsesNativeSteerAdmission(t *testing.T) {
	p, server := testLane(t)
	p.active = &turn{owner: p, id: "turn-1", started: true}
	done := make(chan struct {
		receipt kit.DeliveryReceipt
		err     error
	}, 1)
	go func() {
		receipt, err := p.Deliver(context.Background(), deliveryRequest(), nil)
		done <- struct {
			receipt kit.DeliveryReceipt
			err     error
		}{receipt, err}
	}()
	request := readAppRequest(t, server)
	if request.Method != "turn/steer" || !strings.Contains(string(request.Params), `"expectedTurnId":"turn-1"`) {
		t.Fatalf("request = %+v", request)
	}
	writeApp(t, server, map[string]any{"id": request.ID, "result": map[string]string{"turnId": "turn-1"}})
	result := <-done
	if result.err != nil || result.receipt.Disposition != "injected" {
		t.Fatalf("delivery = %+v, %v", result.receipt, result.err)
	}
}

func TestActiveDeliveryMapsOnlyProvenUnsubmittedSteerErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		message  string
		wantCode int
		wantData string
	}{
		{name: "ended", message: "no active turn to steer", wantCode: -32004},
		{name: "changed", message: "expected active turn id `turn-1` but found `turn-2`", wantCode: -32004},
		{name: "policy remains uncertain", message: "cannot steer a review turn", wantCode: -32603, wantData: `"uncertain_native_admission"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, server := testLane(t)
			p.active = &turn{owner: p, id: "turn-1", started: true}
			done := make(chan error, 1)
			go func() {
				receipt, err := p.Deliver(context.Background(), deliveryRequest(), nil)
				if receipt.Disposition != "" {
					err = errors.Join(err, errors.New("unexpected receipt "+receipt.Disposition))
				}
				done <- err
			}()
			request := readAppRequest(t, server)
			writeApp(t, server, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": test.message}})
			err := <-done
			var protocolError *kit.ProtocolError
			if !errors.As(err, &protocolError) || protocolError.Code != test.wantCode || string(protocolError.Data) != test.wantData {
				t.Fatalf("error = %#v, %v", protocolError, err)
			}
		})
	}
}

func deliveryRequest() kit.DeliveryRequest {
	return kit.DeliveryRequest{
		MessageID: "m",
		Body:      "must wake in a managed run",
		From:      kit.DeliverySource{SessionID: "sender@local", Product: "codex-peer", Groups: []string{"g"}},
	}
}
