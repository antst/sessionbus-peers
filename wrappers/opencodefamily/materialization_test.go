// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

// Holds only the test callback return, after the real product has completely
// projected/validated its native response. It introduces no production hook.
type materializedProduct struct {
	*Wrapper
	returned chan struct{}
	release  chan struct{}
	result   kit.TurnResult
	err      error
}

func (p *materializedProduct) Run(ctx context.Context, run *kit.Run, input kit.RunInput) (kit.TurnResult, error) {
	p.result, p.err = p.Wrapper.Run(ctx, run, input)
	close(p.returned)
	<-p.release
	return p.result, p.err
}
func TestLegacyNativeLossAcrossResultMaterialization(t *testing.T) {
	for _, materialized := range []bool{false, true} {
		for _, loss := range []string{"/fixture/end-events", "/fixture/exit"} {
			name := map[bool]string{false: "projection pending", true: "materialized"}[materialized] + loss
			t.Run(name, func(t *testing.T) {
				held := &materializedProduct{returned: make(chan struct{}), release: make(chan struct{})}
				var once sync.Once
				release := func() { once.Do(func() { close(held.release) }) }
				f := newWorkerProductFixture(t, func(p *Wrapper) kit.WorkerCallbacks { held.Wrapper = p; return held })
				t.Cleanup(release)
				input := "projection-held"
				if materialized {
					input = "materialized"
				}
				f.start(t, 1, input)
				if materialized {
					select {
					case <-held.returned:
					case <-f.ctx.Done():
						t.Fatal("native result not materialized")
					}
					if held.err != nil || held.result.Outcome != "completed" || held.result.Result != "answer:materialized" {
						t.Fatalf("materialization=%+v/%v", held.result, held.err)
					}
				} else {
					if _, err := f.p.client.call(f.ctx, "GET", "/fixture/projection-pending", nil, 200); err != nil {
						t.Fatal(err)
					}
					select {
					case <-held.returned:
						t.Fatal("held native history returned prematurely")
					default:
					}
				}
				// Native control response can be interrupted by owner cancellation. The
				// observed projection/callback boundary above determines the schedule.
				_, _ = f.p.client.call(f.ctx, "POST", loss, nil, 200)
				select {
				case <-f.p.ctx.Done():
				case <-f.ctx.Done():
					t.Fatal("native loss not observed")
				}
				if !materialized {
					select {
					case <-held.returned:
					case <-f.ctx.Done():
						t.Fatal("projection did not settle on native loss")
					}
					if held.err == nil {
						t.Fatalf("missing projection became success: %+v", held.result)
					}
				}
				release()
				var frame protocol.Frame
				select {
				case frame = <-f.ready:
				case <-f.ctx.Done():
					t.Fatal("Worker did not publish terminal before shutdown")
				}
				var ready protocol.TurnReady
				if err := json.Unmarshal(frame.Params, &ready); err != nil {
					t.Fatal(err)
				}
				if materialized {
					if ready.State != "done" || ready.Outcome != "completed" {
						t.Fatalf("materialized result retroactively lost: %+v", ready)
					}
					if held.result.Result != "answer:materialized" {
						t.Fatal("materialized body changed")
					}
				} else if ready.State != "unavailable" || ready.Reason == "" {
					t.Fatalf("pending projection result: %+v", ready)
				}
				// Worker Shutdown ends cursor availability. This test proves callback body
				// and terminal publication, not collection racing that transport teardown.
				select {
				case <-f.done:
				case <-f.ctx.Done():
					t.Fatal("bus transport did not finish")
				}
				f.p.mu.Lock()
				child := f.p.childDone
				f.p.mu.Unlock()
				select {
				case <-child:
				case <-f.ctx.Done():
					t.Fatal("native child not reaped")
				}
			})
		}
	}
}
