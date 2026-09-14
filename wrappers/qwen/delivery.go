// SPDX-License-Identifier: MIT
package qwen

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/antst/sessionbus-peers/wrappers/host"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

type queuedMessage struct {
	text                string
	ctx                 context.Context
	reply               chan deliveryReply
	selected, submitted bool
	settled             bool
}
type deliveryReply struct {
	receipt kit.DeliveryReceipt
	err     error
}
type drainBatch struct {
	entries []*queuedMessage
	done    chan struct{}
}

func (p *Wrapper) Deliver(ctx context.Context, request kit.DeliveryRequest, _ *kit.Run) (kit.DeliveryReceipt, error) {
	m, receipt, err := p.queueDelivery(ctx, request)
	if m == nil {
		return receipt, err
	}
	return p.waitDelivery(ctx, m)
}
func (p *Wrapper) queueDelivery(ctx context.Context, request kit.DeliveryRequest) (*queuedMessage, kit.DeliveryReceipt, error) {
	text, err := host.RenderNativeMessage(request)
	if err != nil {
		return nil, kit.DeliveryReceipt{Disposition: "rejected", Reason: "invalid_input"}, nil
	}
	if err = ctx.Err(); err != nil {
		return nil, kit.DeliveryReceipt{}, err
	}
	p.mu.Lock()
	if p.closing || !p.opened {
		p.mu.Unlock()
		return nil, kit.DeliveryReceipt{Disposition: "rejected", Reason: "native_session_unavailable"}, nil
	}
	// Bound rendered/escaped native frames conservatively (JSON can expand 6x).
	if len(text) > maxACPFrame/8 || len(p.staged) >= maxACPPending || len(text) > maxACPFrame/8-p.stagedBytes {
		p.mu.Unlock()
		return nil, kit.DeliveryReceipt{Disposition: "rejected", Reason: "delivery_capacity"}, nil
	}
	m := &queuedMessage{text: text, ctx: ctx}
	p.staged = append(p.staged, m)
	p.stagedBytes += len(text)
	if p.active == nil || p.active.terminal {
		p.mu.Unlock()
		return nil, kit.DeliveryReceipt{Disposition: "queued_for_next_turn"}, nil
	}
	m.reply = make(chan deliveryReply, 1)
	p.mu.Unlock()
	return m, kit.DeliveryReceipt{}, nil
}
func (p *Wrapper) waitDelivery(ctx context.Context, m *queuedMessage) (kit.DeliveryReceipt, error) {
	reply := m.reply
	select {
	case got := <-reply:
		return got.receipt, got.err
	case <-ctx.Done():
		p.mu.Lock()
		if m.settled {
			p.mu.Unlock()
			got := <-reply
			return got.receipt, got.err
		}
		if !m.selected && !m.submitted {
			for i, q := range p.staged {
				if q == m {
					p.staged = append(p.staged[:i], p.staged[i+1:]...)
					p.stagedBytes -= len(m.text)
					break
				}
			}
			m.settled = true
			p.mu.Unlock()
			return kit.DeliveryReceipt{}, ctx.Err()
		}
		p.mu.Unlock()
		// Selection must settle its real write; cancellation cannot undo it.
		got := <-reply
		return got.receipt, got.err
	}
}
func (p *Wrapper) stageUnsentLocked(failure error) {
	for _, m := range p.staged {
		if m.reply != nil && !m.settled {
			m.reply <- deliveryReply{receipt: kit.DeliveryReceipt{Disposition: "queued_for_next_turn"}, err: failure}
			m.settled = true
		}
	}
}
func (p *Wrapper) answer(method string, raw json.RawMessage) (*acpResponse, error) {
	if method == "session/request_permission" {
		return &acpResponse{Result: map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}}, nil
	}
	if method != "craft/drainMidTurnQueue" {
		return nil, &acpError{Code: -32601, Message: "unsupported Qwen ACP client request " + method}
	}
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(raw, &params) != nil || params.SessionID == "" {
		return nil, &acpError{Code: -32602, Message: "invalid Qwen drain parameters"}
	}
	empty := func() *acpResponse {
		return &acpResponse{Result: map[string]any{"messages": []string{}, "hasQueuedPrompt": false}}
	}
	p.mu.Lock()
	if params.SessionID != p.id {
		p.mu.Unlock()
		return nil, &acpError{Code: -32602, Message: "Qwen drain requested another session"}
	}
	t := p.active
	if p.closing || t == nil || t.terminal || t.batch != nil || len(p.staged) == 0 {
		p.mu.Unlock()
		return empty(), nil
	}
	count := min(10, len(p.staged))
	b := &drainBatch{entries: append([]*queuedMessage(nil), p.staged[:count]...), done: make(chan struct{})}
	p.staged = p.staged[count:]
	for _, m := range b.entries {
		m.selected = true
		p.stagedBytes -= len(m.text)
	}
	t.batch = b
	p.mu.Unlock()
	return &acpResponse{Prepare: func() (any, error) {
		p.mu.Lock()
		defer p.mu.Unlock()
		messages := []string{}
		for _, m := range b.entries {
			if m.ctx.Err() == nil || m.reply == nil || m.settled {
				messages = append(messages, m.text)
				m.submitted = true
			}
		}
		return map[string]any{"messages": messages, "hasQueuedPrompt": false}, nil
	}, Finish: func(err error) {
		p.mu.Lock()
		for _, m := range b.entries {
			if m.reply != nil && !m.settled {
				got := deliveryReply{receipt: kit.DeliveryReceipt{Disposition: "written"}, err: err}
				if !m.submitted {
					got = deliveryReply{err: m.ctx.Err()}
					if got.err == nil {
						got.err = errors.New("Qwen drain ended before submission")
					}
				}
				m.reply <- got
				m.settled = true
			}
		}
		if t.batch == b {
			t.batch = nil
		}
		close(b.done)
		p.mu.Unlock()
	}}, nil
}
