// SPDX-License-Identifier: MIT

package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

const reconnectInterval = 2 * time.Second

type resident struct {
	ctx     context.Context
	cancel  context.CancelFunc
	changed chan struct{}
}

type reportWaiter struct {
	done      chan error
	submitted bool
}

type Owner struct {
	mu                   sync.Mutex
	groups               []string
	socket, nativeSocket string
	dial                 Dial
	retry                func(context.Context) bool
	deliver              func(Recipient, kit.DeliveryRequest) (kit.DeliveryReceipt, error)
	identity, admitted   *kit.Identity
	generation           uint64
	ended                bool
	resident             *resident
	connection           *kit.Connection
	caller               *kit.Caller
	waiters              map[uint64][]*reportWaiter
	submitted            map[uint64]bool
	pendingReports       int
	publishers, handlers sync.WaitGroup
	startupCancel        context.CancelFunc
	startupWork          sync.WaitGroup
	hookObserved         bool
}

func unavailable() error {
	return &kit.ProtocolError{Code: protocol.NotConnected, Message: "not_connected"}
}
func missing(value string) bool {
	return value == "" || len(value) > 3 && strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") && !strings.Contains(value[2:len(value)-1], "}")
}

func NewOwner(env map[string]string) (*Owner, error) {
	if env["SESSIONBUS_LAUNCH_TOKEN"] != "" {
		return nil, errors.New("interactive entry cannot consume a lane launch token")
	}
	groups := []string{}
	if raw := env["SESSIONBUS_GROUPS"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &groups); err != nil || groups == nil {
			return nil, errors.New("invalid launch groups JSON")
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	d := (&net.Dialer{}).DialContext
	o := &Owner{groups: groups, socket: SocketPath(env, cwd, os.Getuid()), nativeSocket: env["CLAUDE_CODE_MESSAGING_SOCKET"], dial: d, retry: waitReconnect, waiters: map[uint64][]*reportWaiter{}, submitted: map[uint64]bool{}}
	o.deliver = func(c Recipient, r kit.DeliveryRequest) (kit.DeliveryReceipt, error) { return DeliverNative(c, r, d) }
	return o, nil
}

// BeginReport applies native state in stdin order. Cancellable publication work
// runs separately; obsolete, unsubmitted generations return unavailable, while
// an already-submitted hello retains its own result and can never admit stale state.
func (o *Owner) BeginReport(raw json.RawMessage) (<-chan error, error) {
	event, err := ParseNativeReport(raw)
	if err != nil || (event.Event != "UserPromptSubmit" && event.Event != "Stop" && event.Event != "SessionEnd") {
		return nil, errors.New("unusable native identity report")
	}
	o.mu.Lock()
	o.hookObserved = true
	if o.startupCancel != nil {
		o.startupCancel()
	}
	return o.beginIdentityLocked(event)
}

// beginIdentityLocked consumes the mutex and preserves the existing hook
// publication ordering for both native reports and the startup witness.
func (o *Owner) beginIdentityLocked(event NativeReport) (<-chan error, error) {
	if o.ended {
		o.mu.Unlock()
		return nil, unavailable()
	}
	name := event.Title
	if missing(name) {
		name = ""
		if o.identity != nil && o.identity.SessionID == event.ID {
			name = o.identity.Name
		}
	}
	identity := &kit.Identity{Protocol: 1, Product: "claude-peer", SessionID: event.ID, Name: name, Groups: o.groups, Info: map[string]any{}}
	if _, err := protocol.EncodeParams("session.hello", identity); err != nil {
		o.mu.Unlock()
		return nil, errors.New("native identity or launch groups violate the Sessionbus wire")
	}
	if event.Event == "SessionEnd" {
		if o.identity != nil && o.identity.SessionID == event.ID {
			o.withdrawLocked(unavailable())
		}
		o.mu.Unlock()
		return nil, nil
	}
	if o.admitted != nil && o.admitted.SessionID == event.ID && o.admitted.Name == name && o.connection != nil && o.connection.Context().Err() == nil {
		o.mu.Unlock()
		return nil, nil
	}
	if o.identity != nil && o.identity.SessionID == event.ID && o.identity.Name == name {
		observed, err := o.addWaiterLocked(o.generation)
		o.mu.Unlock()
		return observed, err
	}
	created := false
	if o.identity == nil || o.identity.SessionID != event.ID {
		o.withdrawLocked(unavailable())
		ctx, cancel := context.WithCancel(context.Background())
		o.resident = &resident{ctx: ctx, cancel: cancel, changed: make(chan struct{}, 1)}
		o.publishers.Add(1)
		go o.publish(o.resident)
		created = true
	}
	o.resolveUnsubmittedWaitersLocked(unavailable())
	o.identity = identity
	o.admitted = nil
	o.generation++
	observed, err := o.addWaiterLocked(o.generation)
	if err != nil {
		o.mu.Unlock()
		return nil, err
	}
	if !created {
		o.wakeLocked()
	}
	o.mu.Unlock()
	return observed, nil
}

func waitReconnect(ctx context.Context) bool {
	timer := time.NewTimer(reconnectInterval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (o *Owner) publish(r *resident) {
	defer o.publishers.Done()
	var c *kit.Connection
	for {
		o.mu.Lock()
		if o.ended || o.resident != r || r.ctx.Err() != nil {
			o.mu.Unlock()
			return
		}
		o.mu.Unlock()
		if c == nil {
			fd, err := o.dial(r.ctx, "unix", o.socket)
			if err != nil {
				if !o.retry(r.ctx) {
					return
				}
				continue
			}
			published := make(chan struct{})
			var attempt *kit.Connection
			attempt = kit.NewConnection(fd, func(_ context.Context, request *kit.Request) { <-published; o.handle(r, attempt, request) })
			c = attempt
			o.mu.Lock()
			if o.ended || o.resident != r || r.ctx.Err() != nil {
				o.mu.Unlock()
				close(published)
				_ = c.Close()
				return
			}
			o.connection = c
			o.caller = kit.NewCaller(func(ctx context.Context, method string, params any) (json.RawMessage, error) {
				var result json.RawMessage
				err := attempt.Call(ctx, method, params, &result)
				return result, err
			})
			o.mu.Unlock()
			close(published)
		}
		for {
			select {
			case <-r.changed:
				continue
			default:
			}
			break
		}
		o.mu.Lock()
		if o.ended || o.resident != r || r.ctx.Err() != nil || o.connection != c {
			o.mu.Unlock()
			return
		}
		identity, generation := *o.identity, o.generation
		o.submitted[generation] = true
		for _, waiter := range o.waiters[generation] {
			waiter.submitted = true
		}
		o.mu.Unlock()
		var result json.RawMessage
		err := c.CallObserved(r.ctx, "session.hello", identity, &result, func() error {
			o.mu.Lock()
			defer o.mu.Unlock()
			if !o.ended && o.resident == r && o.connection == c && o.generation == generation {
				admitted := identity
				o.admitted = &admitted
			}
			return nil
		})
		if err != nil && invalidHello(err) {
			o.mu.Lock()
			if o.resident == r && !o.ended {
				o.ended = true
				o.withdrawLocked(err)
			} else {
				o.resolveGenerationLocked(generation, err)
			}
			o.mu.Unlock()
			_ = c.Close()
			return
		}
		o.mu.Lock()
		if err == nil {
			o.resolveGenerationLocked(generation, nil)
		} else {
			o.resolveGenerationLocked(generation, unavailable())
		}
		o.mu.Unlock()
		if err != nil {
			o.clearConnection(r, c)
			_ = c.Close()
			c = nil
			if r.ctx.Err() != nil || !o.retry(r.ctx) {
				return
			}
			continue
		}
		o.mu.Lock()
		same := !o.ended && o.resident == r && o.generation == generation
		o.mu.Unlock()
		if !same {
			continue
		}
		select {
		case <-r.ctx.Done():
			return
		case <-c.Done():
			o.clearConnection(r, c)
			c = nil
			if !o.retry(r.ctx) {
				return
			}
		case <-r.changed:
		}
	}
}

func invalidHello(err error) bool {
	var failure *kit.ProtocolError
	return errors.As(err, &failure) && failure.Code == protocol.InvalidHello
}

func (o *Owner) clearConnection(r *resident, c *kit.Connection) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.resident == r && o.connection == c {
		o.connection = nil
		o.caller = nil
		o.admitted = nil
	}
}

func (o *Owner) addWaiterLocked(generation uint64) (<-chan error, error) {
	if o.pendingReports >= protocol.MaxOperations {
		return nil, errors.New("native identity report backlog full")
	}
	waiter := &reportWaiter{done: make(chan error, 1), submitted: o.submitted[generation]}
	o.waiters[generation] = append(o.waiters[generation], waiter)
	o.pendingReports++
	return waiter.done, nil
}

func (o *Owner) resolveGenerationLocked(generation uint64, err error) {
	for _, waiter := range o.waiters[generation] {
		waiter.done <- err
		o.pendingReports--
	}
	delete(o.waiters, generation)
	delete(o.submitted, generation)
}

func (o *Owner) resolveUnsubmittedWaitersLocked(err error) {
	for generation, waiters := range o.waiters {
		kept := waiters[:0]
		for _, waiter := range waiters {
			if waiter.submitted {
				kept = append(kept, waiter)
				continue
			}
			waiter.done <- err
			o.pendingReports--
		}
		if len(kept) == 0 {
			delete(o.waiters, generation)
			delete(o.submitted, generation)
		} else {
			o.waiters[generation] = kept
		}
	}
}

func (o *Owner) resolveWaitersLocked(err error) {
	for generation := range o.waiters {
		o.resolveGenerationLocked(generation, err)
	}
}

func (o *Owner) wakeLocked() {
	if o.resident == nil {
		return
	}
	select {
	case o.resident.changed <- struct{}{}:
	default:
	}
}

func (o *Owner) withdrawLocked(err error) {
	o.generation++
	o.admitted = nil
	o.identity = nil
	o.resolveWaitersLocked(err)
	if o.resident != nil {
		o.resident.cancel()
	}
	c := o.connection
	o.connection = nil
	o.caller = nil
	if c != nil {
		_ = c.Close()
	}
	o.resident = nil
}

func (o *Owner) End() {
	o.mu.Lock()
	o.ended = true
	if o.startupCancel != nil {
		o.startupCancel()
	}
	o.withdrawLocked(unavailable())
	o.mu.Unlock()
	o.startupWork.Wait()
	o.publishers.Wait()
	o.handlers.Wait()
}

func (o *Owner) Action(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	o.mu.Lock()
	if o.ended || o.admitted == nil || o.connection == nil || o.resident == nil || o.resident.ctx.Err() != nil || o.connection.Context().Err() != nil {
		o.mu.Unlock()
		return nil, unavailable()
	}
	caller, identityContext, connection := o.caller, o.resident.ctx, o.connection
	o.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(identityContext, cancel)
	defer stop()
	if identityContext.Err() != nil {
		return nil, unavailable()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := caller.Action(ctx, action, args)
	if err != nil {
		o.mu.Lock()
		lost := o.connection != connection || o.admitted == nil || connection.Context().Err() != nil
		o.mu.Unlock()
		if lost || errors.Is(err, context.Canceled) && identityContext.Err() != nil {
			return nil, unavailable()
		}
	}
	return result, err
}

func (o *Owner) handle(resident *resident, c *kit.Connection, r *kit.Request) {
	o.mu.Lock()
	if o.connection != c || o.resident != resident || o.ended {
		o.mu.Unlock()
		_ = c.Close()
		return
	}
	if r.Method == "session.superseded" {
		o.ended = true
		o.generation++
		o.admitted = nil
		o.resolveWaitersLocked(unavailable())
		resident.cancel()
		o.handlers.Add(1)
		o.mu.Unlock()
		go func() {
			defer o.handlers.Done()
			_ = c.Result(r, struct{}{})
			o.mu.Lock()
			defer o.mu.Unlock()
			if o.connection == c {
				o.withdrawLocked(unavailable())
			}
		}()
		return
	}
	var captured *Recipient
	if o.admitted != nil && !o.ended {
		captured = &Recipient{SessionID: o.admitted.SessionID, Socket: o.nativeSocket, Context: c.Context()}
	}
	o.handlers.Add(1)
	o.mu.Unlock()
	go func() {
		defer o.handlers.Done()
		var err error
		switch {
		case r.Method != "message.deliver":
			err = c.Error(r, -32600, nil)
		case captured == nil:
			err = c.Result(r, kit.DeliveryReceipt{Disposition: "rejected", Reason: "unpublished_before_submission"})
		default:
			request, ok := r.Params.(*protocol.DeliveryRequest)
			if !ok {
				_ = c.Close()
				return
			}
			result, deliveryErr := o.deliver(*captured, *request)
			if deliveryErr != nil {
				err = c.Error(r, -32603, "uncertain_submission")
			} else {
				err = c.Result(r, result)
			}
		}
		if err != nil {
			_ = c.Close()
		}
	}()
}

type NativeReport struct {
	Event string `json:"hook_event_name"`
	ID    string `json:"session_id"`
	Title string `json:"session_title"`
	Agent string `json:"agent_id"`
}

// ParseNativeReport preserves the native hook placeholder and field-type rules.
func ParseNativeReport(raw json.RawMessage) (NativeReport, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return NativeReport{}, errors.New("unusable native identity report")
	}
	for _, key := range []string{"hook_event_name", "session_id", "session_title", "agent_id"} {
		if value, present := fields[key]; present {
			var text string
			if string(value) == "null" || json.Unmarshal(value, &text) != nil {
				return NativeReport{}, errors.New("unusable native identity report")
			}
		}
	}
	var event NativeReport
	if err := json.Unmarshal(raw, &event); err != nil || missing(event.ID) || !missing(event.Agent) {
		return NativeReport{}, errors.New("unusable native identity report")
	}
	return event, nil
}

func MissingNativeField(value string) bool { return missing(value) }
