// SPDX-License-Identifier: MIT
package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/antst/sessionbus-peers/wrappers/host"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

type laneRun struct {
	run        *kit.Run
	initial    string
	original   *httpOperation
	started    chan struct{}
	startOnce  sync.Once
	userSeen   bool
	accepting  bool
	deliveries sync.WaitGroup
	count      int
	interrupt  *nativeInterrupt
}
type nativeInterrupt struct {
	done chan struct{}
	err  error
	ack  bool
}

func (p *Wrapper) promptBody(id, text string, noReply bool) map[string]any {
	b := map[string]any{"messageID": id, "parts": []map[string]string{{"type": "text", "text": text}}}
	if noReply {
		b["noReply"] = true
	}
	if p.model != nil {
		b["model"] = map[string]string{"providerID": p.model.ProviderID, "modelID": p.model.ID}
	}
	if p.agent != "" {
		b["agent"] = p.agent
	}
	return b
}
func (p *Wrapper) Run(ctx context.Context, run *kit.Run, input kit.RunInput) (kit.TurnResult, error) {
	return p.executeRun(ctx, run, input, run.ReportDelivery)
}
func (p *Wrapper) executeRun(ctx context.Context, run *kit.Run, input kit.RunInput, report func(kit.DeliveryReceipt, error) error) (kit.TurnResult, error) {
	var text string
	var err error
	if input.Delivery != nil {
		text, err = host.RenderNativeMessage(*input.Delivery)
	} else if input.Text != nil {
		text = *input.Text
	} else {
		err = errors.New("missing Run input")
	}
	if err != nil {
		return kit.TurnResult{}, err
	}
	if strings.TrimSpace(text) == "" {
		return kit.TurnResult{}, errors.New("empty native Run input")
	}
	id, err := randomMessageID()
	if err != nil {
		return kit.TurnResult{}, err
	}
	p.mu.Lock()
	if p.run != nil {
		select {
		case <-p.run.Done():
			p.run = nil
		default:
		}
	}
	if !p.opened || p.closing || p.ctx.Err() != nil || p.run != nil {
		p.mu.Unlock()
		return kit.TurnResult{}, errors.New("OpenCode lane unavailable or busy")
	}
	combined := append(append([]string{}, p.staged...), text)
	b, err := encodeNative(p.promptBody(id, strings.Join(combined, "\n"), false))
	if err != nil {
		p.mu.Unlock()
		return kit.TurnResult{}, err
	}
	request, err := p.client.prepare(p.ctx, "POST", sessionPath(p.id)+"/message", b)
	if err != nil || ctx.Err() != nil {
		p.mu.Unlock()
		return kit.TurnResult{}, errors.Join(err, ctx.Err())
	}
	t := &laneRun{run: run, initial: id, started: make(chan struct{}), accepting: true, count: 1}
	p.run, p.active = run, t
	op, err := p.client.begin(request, 200)
	if err != nil {
		p.run, p.active = nil, nil
		p.mu.Unlock()
		return kit.TurnResult{}, err
	}
	t.original = op
	p.staged = nil
	p.stagedBytes = 0
	p.mu.Unlock()
	// SDK admission is ordering of our owned operation, not native consumption.
	run.Admitted()
	stop := context.AfterFunc(ctx, func() { p.fail(ctx.Err()) })
	defer stop()
	defer func() {
		p.mu.Lock()
		if p.active == t {
			p.active = nil
		}
		p.mu.Unlock()
	}()
	if run.Interrupted() {
		p.ensureInterrupt(t)
	}
	if input.Delivery != nil {
		<-op.written
		var receiptErr error
		// ReportDelivery writes on the bus connection. Native owner loss must
		// release a genuinely blocked bus write as well as native HTTP work.
		shutdownDone := make(chan struct{})
		stopShutdown := context.AfterFunc(p.ctx, func() {
			if p.shutdown != nil {
				p.shutdown()
			}
			close(shutdownDone)
		})
		if op.writeErr == nil {
			receiptErr = report(kit.DeliveryReceipt{Disposition: "written"}, nil)
		} else {
			receiptErr = report(kit.DeliveryReceipt{}, op.writeErr)
		}
		if !stopShutdown() {
			<-shutdownDone
		}
		if receiptErr != nil {
			p.fail(receiptErr)
		}
	}
	raw, err := op.wait()
	p.mu.Lock()
	t.accepting = false
	interrupt := t.interrupt
	p.mu.Unlock()
	t.deliveries.Wait()
	if interrupt != nil {
		<-interrupt.done
		if interrupt.err != nil {
			p.fail(interrupt.err)
			err = errors.Join(err, interrupt.err)
		}
	}
	if err != nil {
		p.fail(err)
		return kit.TurnResult{}, err
	}
	if p.ctx.Err() != nil {
		return kit.TurnResult{}, context.Cause(p.ctx)
	}
	final, err := decodeParts(raw, p.id)
	interrupted := interrupt != nil && interrupt.ack
	if err != nil {
		p.fail(err)
		return kit.TurnResult{}, err
	}
	output, err := p.client.historyResult(p.ctx, p.id, id, final)
	if err != nil {
		// Native cancel's lastAssistant fallback can predate the admitted input.
		// Only that exact stale-result condition permits empty interrupted output.
		if interrupted && (errors.Is(err, errPriorAssistant) || final.Info.Role != "assistant") {
			return kit.TurnResult{Outcome: "interrupted"}, nil
		}
		return kit.TurnResult{}, err
	}
	if final.Info.Summary && (len(final.Info.Error) == 0 || string(final.Info.Error) == "null") {
		if interrupted {
			return kit.TurnResult{Outcome: "interrupted", Result: output}, nil
		}
		return kit.TurnResult{}, errors.New("native Run returned only an internal summary")
	}
	result := kit.TurnResult{Outcome: "completed", Result: output, NativeStopReason: final.Info.Finish}
	if len(final.Info.Error) > 0 && string(final.Info.Error) != "null" {
		var cause struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(final.Info.Error, &cause) != nil || cause.Name == "" {
			return kit.TurnResult{}, errors.New("malformed native terminal error")
		}
		result.Outcome = "failed"
		result.NativeStopReason = cause.Name
		if cause.Name == "MessageAbortedError" {
			result.Outcome = "interrupted"
		}
	} else if final.Info.Time.Completed == nil || final.Info.Finish == "" || final.Info.Finish == "tool-calls" || final.Info.Finish == "unknown" {
		if interrupted {
			return kit.TurnResult{Outcome: "interrupted"}, nil
		}
		return kit.TurnResult{}, errors.New("native terminal lacks completed assistant")
	}
	return result, nil
}
func (p *Wrapper) ensureInterrupt(t *laneRun) *nativeInterrupt {
	p.mu.Lock()
	defer p.mu.Unlock()
	if t.interrupt != nil {
		return t.interrupt
	}
	op := &nativeInterrupt{done: make(chan struct{})}
	t.interrupt = op
	go func() {
		defer close(op.done)
		select {
		case <-t.original.done:
			return
		case <-t.started:
		case <-p.ctx.Done():
			op.err = context.Cause(p.ctx)
			return
		}
		select {
		case <-t.original.done:
			return
		default:
		}
		r, e := p.client.prepare(p.ctx, "POST", sessionPath(p.id)+"/abort", []byte(`{}`))
		if e != nil {
			op.err = e
			return
		}
		call, e := p.client.beginControl(r, 200)
		if e != nil {
			op.err = e
			p.fail(e)
			return
		}
		b, e := call.wait()
		if e == nil && strings.TrimSpace(string(b)) != "true" {
			e = errors.New("native abort not acknowledged")
		}
		op.err = e
		op.ack = e == nil
		if e != nil {
			p.fail(e)
		}
	}()
	return op
}
func (p *Wrapper) Interrupt(ctx context.Context, run *kit.Run) error {
	p.mu.Lock()
	t := p.active
	p.mu.Unlock()
	if t == nil || t.run != run {
		return nil
	}
	op := p.ensureInterrupt(t)
	select {
	case <-op.done:
		return op.err
	case <-ctx.Done():
		p.fail(ctx.Err())
		<-op.done
		return ctx.Err()
	}
}
func (p *Wrapper) Deliver(ctx context.Context, request kit.DeliveryRequest, run *kit.Run) (kit.DeliveryReceipt, error) {
	text, err := host.RenderNativeMessage(request)
	if err != nil {
		return kit.DeliveryReceipt{}, err
	}
	if len(text) > maxNativeRequest {
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "message_too_large"}, nil
	}
	if run != nil {
		select {
		case <-run.AdmittedDone():
		case <-run.Done():
		case <-ctx.Done():
			return kit.DeliveryReceipt{}, ctx.Err()
		}
	}
	p.mu.Lock()
	if !p.opened || p.closing || p.ctx.Err() != nil {
		p.mu.Unlock()
		return kit.DeliveryReceipt{}, errors.New("OpenCode lane unavailable")
	}
	if ctx.Err() != nil {
		p.mu.Unlock()
		return kit.DeliveryReceipt{}, ctx.Err()
	}
	t := p.active
	if t == nil || t.run != run || !t.accepting {
		if len(p.staged) >= 256 || p.stagedBytes+len(text) > maxNativeRequest {
			p.mu.Unlock()
			return kit.DeliveryReceipt{Disposition: "rejected", Reason: "stage_full"}, nil
		}
		p.staged = append(p.staged, text)
		p.stagedBytes += len(text)
		p.mu.Unlock()
		return kit.DeliveryReceipt{Disposition: "queued_for_next_turn"}, nil
	}
	if t.count >= 256 {
		p.mu.Unlock()
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "run_input_limit"}, nil
	}
	id, err := randomMessageID()
	if err != nil {
		p.mu.Unlock()
		return kit.DeliveryReceipt{}, err
	}
	b, err := encodeNative(p.promptBody(id, text, true))
	if err != nil {
		p.mu.Unlock()
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "message_too_large"}, nil
	}
	life, cancel := context.WithCancel(p.ctx)
	stop := context.AfterFunc(ctx, cancel)
	r, err := p.client.prepare(life, "POST", sessionPath(p.id)+"/message", b)
	if err != nil {
		p.mu.Unlock()
		stop()
		cancel()
		return kit.DeliveryReceipt{}, err
	}
	t.deliveries.Add(1)
	op, err := p.client.begin(r, 200)
	if err != nil {
		t.deliveries.Done()
		p.mu.Unlock()
		stop()
		cancel()
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "native_capacity"}, nil
	}
	t.count++
	p.mu.Unlock()
	defer t.deliveries.Done()
	defer stop()
	defer cancel()
	raw, err := op.wait()
	if err != nil {
		p.fail(err)
		return kit.DeliveryReceipt{}, err
	}
	m, err := decodeParts(raw, p.id)
	if err == nil && (m.Info.Role != "user" || m.Info.ID != id) {
		err = errors.New("native delivery returned different user")
	}
	var saved strings.Builder
	if err == nil {
		for _, part := range m.Parts {
			var v struct{ Type, Text string }
			_ = json.Unmarshal(part, &v)
			if v.Type == "text" {
				saved.WriteString(v.Text)
			}
		}
		if saved.String() != text {
			err = errors.New("native saved delivery content differs")
		}
	}
	if err != nil {
		p.fail(err)
		return kit.DeliveryReceipt{}, err
	}
	return kit.DeliveryReceipt{Disposition: "written"}, nil
}
func (p *Wrapper) observe(raw []byte) error {
	var e struct {
		Type       string `json:"type"`
		Properties struct {
			SessionID string     `json:"sessionID"`
			ID        string     `json:"id"`
			Info      nativeInfo `json:"info"`
			Status    struct {
				Type string `json:"type"`
			} `json:"status"`
		} `json:"properties"`
	}
	if json.Unmarshal(raw, &e) != nil || e.Type == "" {
		return errors.New("malformed native event")
	}
	p.mu.Lock()
	t := p.active
	id := p.id
	if t != nil {
		if e.Type == "message.updated" && e.Properties.Info.SessionID == id && e.Properties.Info.ID == t.initial && e.Properties.Info.Role == "user" {
			t.userSeen = true
		}
		if e.Type == "session.status" && e.Properties.SessionID == id && e.Properties.Status.Type == "busy" && t.userSeen {
			t.startOnce.Do(func() { close(t.started) })
		}
	}
	p.mu.Unlock()
	if e.Type != "permission.asked" && e.Type != "question.asked" {
		return nil
	}
	if !validNativeID(e.Properties.SessionID) || e.Properties.ID == "" {
		return errors.New("invalid native permission/question request")
	}
	// Keep the stream reader available for the ordered startup/cancel gate.
	// Native ancestry lookups and rejection responses are bounded joined work.
	if len(e.Properties.ID) > 4096 || strings.ContainsAny(e.Properties.ID, "\x00\r\n") {
		return errors.New("invalid native request ID")
	}
	select {
	case p.eventWork <- struct{}{}:
	default:
		return errors.New("native event work limit reached")
	}
	p.workers.Add(1)
	go func() {
		defer p.workers.Done()
		defer func() { <-p.eventWork }()
		if err := p.ownsSession(p.ctx, e.Properties.SessionID); err != nil {
			p.fail(err)
			return
		}
		path := "/question/" + url.PathEscape(e.Properties.ID) + "/reject"
		var body any = map[string]any{}
		if e.Type == "permission.asked" {
			path = "/permission/" + url.PathEscape(e.Properties.ID) + "/reply"
			body = map[string]string{"reply": "reject"}
		}
		if _, err := p.client.call(p.ctx, "POST", path, body, 200); err != nil {
			p.fail(err)
		}
	}()
	return nil
}
