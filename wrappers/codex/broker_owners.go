// SPDX-License-Identifier: MIT

package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/antst/sessionbus-peers/wrappers/host"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

const brokerReconnectInterval = 2 * time.Second

type brokerOwners struct {
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	mux          *brokerMux
	groups       []string
	socket       string
	owners       map[string]*brokerResident
	startup      map[string]string
	initialName  string
	nameSelected bool
	ended        bool
	dial         func(context.Context, string, string) (net.Conn, error)
	retry        func(context.Context) bool
	work         sync.WaitGroup
}
type brokerResident struct {
	owner          *brokerOwners
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.Mutex
	identity       kit.Identity
	generation     uint64
	admitted       bool
	tools          bool
	checking       bool
	toolGeneration uint64
	closed         bool
	connection     *kit.Connection
	caller         *kit.Caller
	changed        chan struct{}
	handlers       sync.WaitGroup
}

func newBrokerOwners(ctx context.Context, groups []string, socket string) *brokerOwners {
	ownerContext, cancel := context.WithCancel(ctx)
	return &brokerOwners{ctx: ownerContext, cancel: cancel, groups: append([]string{}, groups...), socket: socket, owners: map[string]*brokerResident{}, startup: map[string]string{}, dial: (&net.Dialer{}).DialContext, retry: waitBrokerReconnect}
}
func (o *brokerOwners) setMux(m *brokerMux) { o.mu.Lock(); o.mux = m; o.mu.Unlock() }
func (o *brokerOwners) observe(method string, params, result json.RawMessage) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ended {
		return nil
	}
	switch method {
	case "thread/start", "thread/resume", "thread/fork", "thread/started":
		raw := result
		if method == "thread/started" {
			raw = params
		}
		if len(raw) == 0 {
			return nil
		}
		var reply threadReply
		if json.Unmarshal(raw, &reply) != nil || reply.Thread.ID == "" {
			return errors.New("native settled thread omitted identity")
		}
		if o.owners[reply.Thread.ID] != nil {
			return nil
		} // Earlier native thread/started/name events remain authoritative.
		if len(o.owners) >= brokerRouteLimit {
			return errors.New("native loaded thread limit reached")
		}
		identity := kit.Identity{Protocol: 1, Product: "codex-peer", SessionID: reply.Thread.ID, Name: reply.Thread.Name, Groups: o.groups, Info: map[string]any{"cwd": reply.Thread.Cwd}}
		if _, err := protocol.EncodeParams("session.hello", identity); err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(o.ctx)
		r := &brokerResident{owner: o, ctx: ctx, cancel: cancel, identity: identity, generation: 1, changed: make(chan struct{}, 1)}
		o.owners[identity.SessionID] = r
		o.work.Add(1)
		go r.publish()
		if o.startup[identity.SessionID] == "ready" {
			r.checkToolsLocked()
		}
	case "thread/closed":
		var event struct{ ThreadID string }
		if json.Unmarshal(params, &event) != nil || event.ThreadID == "" {
			return errors.New("invalid native thread close")
		}
		if r := o.owners[event.ThreadID]; r != nil {
			r.end()
			delete(o.owners, event.ThreadID)
		}
		delete(o.startup, event.ThreadID)
	case "thread/name/updated":
		var event struct {
			ThreadID   string
			ThreadName *string
		}
		if json.Unmarshal(params, &event) != nil || event.ThreadID == "" {
			return errors.New("invalid native thread name")
		}
		if r := o.owners[event.ThreadID]; r != nil {
			r.mu.Lock()
			name := ""
			if event.ThreadName != nil {
				name = *event.ThreadName
			}
			r.identity.Name = name
			r.generation++
			r.admitted = false
			r.mu.Unlock()
			r.wake()
		}
	case "mcpServer/startupStatus/updated":
		var event struct{ ThreadID, Name, Status string }
		if json.Unmarshal(params, &event) != nil {
			return errors.New("invalid native MCP startup")
		}
		if event.Name != laneServer || event.ThreadID == "" {
			return nil
		}
		if _, known := o.startup[event.ThreadID]; !known && len(o.startup) >= brokerRouteLimit {
			return errors.New("native pending startup limit reached")
		}
		o.startup[event.ThreadID] = event.Status
		if r := o.owners[event.ThreadID]; r != nil {
			r.mu.Lock()
			r.tools = false
			r.admitted = false
			r.toolGeneration++
			r.mu.Unlock()
			if event.Status == "ready" {
				r.checkToolsLocked()
			}
		}
	}
	return nil
}

// Called with owner.mu. Status checks are event-triggered, never polling.
func (r *brokerResident) checkToolsLocked() {
	r.mu.Lock()
	if r.checking || r.closed {
		r.mu.Unlock()
		return
	}
	r.checking = true
	toolGeneration := r.toolGeneration
	id := r.identity.SessionID
	r.mu.Unlock()
	mux := r.owner.mux
	r.owner.work.Add(1)
	go func() {
		defer r.owner.work.Done()
		valid := false
		if mux != nil {
			cursor := ""
			seen := map[string]bool{}
			for {
				var page struct {
					Data       []laneServerStatus
					NextCursor *string
				}
				p := map[string]any{"threadId": id, "limit": 100, "detail": "toolsAndAuthOnly"}
				if cursor != "" {
					p["cursor"] = cursor
				}
				if err := mux.call(r.ctx, "mcpServerStatus/list", p, &page); err != nil {
					break
				}
				found := false
				for _, s := range page.Data {
					if s.Name == laneServer {
						found = true
						valid = s.PluginID == lanePlugin && s.RuntimeStatus == "connected" && s.Tools["sessionbus"] != nil
						break
					}
				}
				if found || page.NextCursor == nil || *page.NextCursor == "" {
					break
				}
				cursor = *page.NextCursor
				if seen[cursor] || len(seen) >= brokerRouteLimit {
					break
				}
				seen[cursor] = true
			}
		}
		r.owner.mu.Lock()
		stillReady := r.owner.startup[id] == "ready" && !r.owner.ended
		r.mu.Lock()
		r.checking = false
		same := r.toolGeneration == toolGeneration
		r.tools = valid && stillReady && same && !r.closed
		r.mu.Unlock()
		if !same && stillReady {
			r.checkToolsLocked()
		}
		r.owner.mu.Unlock()
		r.wake()
	}()
}
func (r *brokerResident) wake() {
	select {
	case r.changed <- struct{}{}:
	default:
	}
}
func (r *brokerResident) end() {
	r.retire(true)
}

// retireFromPublisher closes an owner which ended without a native terminal.
// A supersession handler marks the resident closed before acknowledging the
// terminal request and retains the connection until that owned handler (or
// an explicit owner End) releases it.
func (r *brokerResident) retireFromPublisher() {
	r.retire(false)
}

func (r *brokerResident) retire(closeAlreadyRetired bool) {
	r.mu.Lock()
	if r.closed && !closeAlreadyRetired {
		r.mu.Unlock()
		return
	}
	if !r.closed {
		r.closed = true
		r.admitted = false
		r.cancel()
	}
	r.admitted = false
	c := r.connection
	r.connection = nil
	r.caller = nil
	r.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}
func (o *brokerOwners) End() {
	o.mu.Lock()
	if !o.ended {
		o.ended = true
		o.cancel()
		for _, r := range o.owners {
			r.end()
		}
	}
	o.mu.Unlock()
	o.work.Wait()
}

func waitBrokerReconnect(ctx context.Context) bool {
	timer := time.NewTimer(brokerReconnectInterval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *brokerResident) clearConnection(c *kit.Connection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.connection == c {
		r.connection = nil
		r.caller = nil
		r.admitted = false
	}
}

func (r *brokerResident) publish() {
	defer func() {
		r.retireFromPublisher()
		r.handlers.Wait()
		r.owner.work.Done()
	}()
	var c *kit.Connection
	for {
		r.mu.Lock()
		tools, admitted := r.tools, r.admitted
		r.mu.Unlock()
		if tools && !admitted {
			if c == nil {
				fd, err := r.owner.dial(r.ctx, "unix", r.owner.socket)
				if err != nil {
					if !r.owner.retry(r.ctx) {
						return
					}
					continue
				}
				published := make(chan struct{})
				var attempt *kit.Connection
				attempt = kit.NewConnection(fd, func(_ context.Context, request *kit.Request) { <-published; r.handle(attempt, request) })
				c = attempt
				r.mu.Lock()
				if r.closed {
					r.mu.Unlock()
					close(published)
					_ = c.Close()
					return
				}
				r.connection = c
				r.caller = kit.NewCaller(func(ctx context.Context, method string, params any) (json.RawMessage, error) {
					var out json.RawMessage
					err := attempt.Call(ctx, method, params, &out)
					return out, err
				})
				r.mu.Unlock()
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
			r.mu.Lock()
			tools, admitted = r.tools, r.admitted
			identity, generation := r.identity, r.generation
			r.mu.Unlock()
			if !tools || admitted {
				continue
			}
			var hello json.RawMessage
			err := c.CallObserved(r.ctx, "session.hello", identity, &hello, func() error {
				r.mu.Lock()
				defer r.mu.Unlock()
				if !r.closed && r.generation == generation && r.tools {
					r.admitted = true
				}
				return nil
			})
			if err != nil {
				r.clearConnection(c)
				_ = c.Close()
				c = nil
				if brokerInvalidHello(err) {
					return
				}
				if !r.owner.retry(r.ctx) {
					return
				}
				continue
			}
			r.mu.Lock()
			same := r.generation == generation
			r.mu.Unlock()
			if !same {
				continue
			}
		}
		var connectionDone <-chan struct{}
		if c != nil {
			connectionDone = c.Done()
		}
		select {
		case <-r.ctx.Done():
			return
		case <-connectionDone:
			r.clearConnection(c)
			c = nil
			if !r.owner.retry(r.ctx) {
				return
			}
		case <-r.changed:
		}
	}
}

func brokerInvalidHello(err error) bool {
	var failure *kit.ProtocolError
	return errors.As(err, &failure) && failure.Code == protocol.InvalidHello
}

func (r *brokerResident) handle(source *kit.Connection, request *kit.Request) {
	r.mu.Lock()
	c, admitted := r.connection, r.admitted && !r.closed
	if c == nil || c != source || r.closed {
		r.mu.Unlock()
		_ = source.Close()
		return
	}
	if request.Method == "session.superseded" {
		r.closed = true
		r.admitted = false
		r.tools = false
		r.generation++
		r.cancel()
		r.handlers.Add(1)
		r.mu.Unlock()
		go func() { defer r.handlers.Done(); _ = c.Result(request, struct{}{}); r.end() }()
		return
	}
	r.handlers.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.handlers.Done()
		var err error
		if request.Method != "message.deliver" {
			err = c.Error(request, -32600, nil)
		} else if !admitted {
			err = c.Result(request, kit.DeliveryReceipt{Disposition: "rejected", Reason: "unpublished_before_submission"})
		} else if p, ok := request.Params.(*protocol.DeliveryRequest); ok {
			receipt, deliveryErr := r.deliver(c.Context(), *p)
			if deliveryErr != nil {
				err = c.Error(request, -32603, "uncertain_native_admission")
			} else {
				err = c.Result(request, receipt)
			}
		} else {
			err = errors.New("invalid delivery request")
		}
		if err != nil {
			_ = c.Close()
		}
	}()
}
func (r *brokerResident) deliver(ctx context.Context, request kit.DeliveryRequest) (kit.DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "closing"}, nil
	}
	message, err := host.RenderNativeMessage(request)
	if err != nil {
		return kit.DeliveryReceipt{}, err
	}
	r.owner.mu.Lock()
	mux := r.owner.mux
	r.owner.mu.Unlock()
	id := r.identity.SessionID
	var current threadReply
	if err = mux.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": false}, &current); err != nil {
		return kit.DeliveryReceipt{}, err
	}
	if current.Thread.ID != id {
		return kit.DeliveryReceipt{}, errors.New("native delivery thread mismatch")
	}
	status, err := statusType(current.Thread.Status)
	if err != nil {
		return kit.DeliveryReceipt{}, err
	}
	switch status {
	case "active":
		var page struct{ Data []nativeTurn }
		if err = mux.call(ctx, "thread/turns/list", map[string]any{"threadId": id, "limit": 1, "sortDirection": "desc", "itemsView": "notLoaded"}, &page); err != nil {
			return kit.DeliveryReceipt{}, err
		}
		if len(page.Data) != 1 || page.Data[0].ID == "" || page.Data[0].Status != "inProgress" {
			return kit.DeliveryReceipt{}, errors.New("native active turn unavailable")
		}
		var reply struct{ TurnID string }
		err = mux.call(ctx, "turn/steer", map[string]any{"threadId": id, "expectedTurnId": page.Data[0].ID, "input": textInput(message)}, &reply)
		if err == nil && reply.TurnID != page.Data[0].ID {
			err = errors.New("native steer turn mismatch")
		}
	case "idle":
		var reply turnReply
		err = mux.call(ctx, "turn/start", map[string]any{"threadId": id, "input": textInput(message)}, &reply)
		if err == nil && reply.Turn.ID == "" {
			err = errors.New("native delivery returned no turn")
		}
	default:
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "not_loaded"}, nil
	}
	// A successful native acknowledgement remains injected across later terminal
	// events or cancellation. A transport failure never becomes native refusal.
	return kit.DeliveryReceipt{Disposition: "injected"}, err
}

func (o *brokerOwners) action(ctx context.Context, action string, args, meta json.RawMessage) (json.RawMessage, error) {
	var native struct{ ThreadID string }
	if json.Unmarshal(meta, &native) != nil || native.ThreadID == "" {
		return nil, errors.New("native tool metadata omitted threadId")
	}
	o.mu.Lock()
	r := o.owners[native.ThreadID]
	o.mu.Unlock()
	if r == nil {
		return nil, errors.New("native tool thread is not loaded on this broker")
	}
	r.mu.Lock()
	admitted, caller, connection := r.admitted && r.tools && !r.closed, r.caller, r.connection
	r.mu.Unlock()
	if !admitted || caller == nil {
		return nil, errors.New("native Sessionbus tool is not ready")
	}
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	result, err := caller.Action(callCtx, action, args)
	if err != nil {
		r.mu.Lock()
		lost := r.connection != connection || r.admitted == false || connection == nil || connection.Context().Err() != nil
		r.mu.Unlock()
		if lost {
			return nil, &kit.ProtocolError{Code: protocol.NotConnected, Message: "not_connected"}
		}
	}
	return result, err
}

// selectThread runs only for a correlated successful TUI response. Naming runs
// outside the reader and never attributes every spawned subthread to the option.
func (o *brokerOwners) selectThread(method string, result json.RawMessage) error {
	if method != "thread/start" && method != "thread/resume" && method != "thread/fork" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ended || o.initialName == "" || o.nameSelected {
		return nil
	}
	var selected threadReply
	if json.Unmarshal(result, &selected) != nil || selected.Thread.ID == "" {
		return errors.New("initial native selection omitted thread ID")
	}
	o.nameSelected = true
	mux, id, name := o.mux, selected.Thread.ID, o.initialName
	o.work.Add(1)
	go func() {
		defer o.work.Done()
		if err := mux.call(o.ctx, "thread/name/set", map[string]string{"threadId": id, "name": name}, &struct{}{}); err != nil {
			mux.fail(err)
		}
	}()
	return nil
}
