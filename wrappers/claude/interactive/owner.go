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

	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

type Owner struct {
	mu, reports          sync.Mutex
	groups               []string
	socket, nativeSocket string
	dial                 Dial
	deliver              func(Recipient, kit.DeliveryRequest) (kit.DeliveryReceipt, error)
	identity, admitted   *kit.Identity
	generation           uint64
	ended                bool
	connection           *kit.Connection
	caller               *kit.Caller
	ctx                  context.Context
	cancel               context.CancelFunc
}

func unavailable() error { return &kit.ProtocolError{Code: -32002, Message: "not_connected"} }
func missing(value string) bool {
	return value == "" || len(value) > 3 && strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") && !strings.Contains(value[2:len(value)-1], "}")
}

func NewOwner(env map[string]string) (*Owner, error) {
	if env["SESSIONBUS_LAUNCH_TOKEN"] != "" {
		return nil, errors.New("Claude lane mode is unavailable in this interactive candidate")
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
	o := &Owner{groups: groups, socket: SocketPath(env, cwd, os.Getuid()), nativeSocket: env["CLAUDE_CODE_MESSAGING_SOCKET"], dial: d}
	o.deliver = func(c Recipient, r kit.DeliveryRequest) (kit.DeliveryReceipt, error) { return DeliverNative(c, r, d) }
	return o, nil
}

// BeginReport applies reports in stdin order and sends hello before returning;
// waiting for its acknowledgment never blocks the MCP input reader.
func (o *Owner) BeginReport(raw json.RawMessage) (<-chan error, error) {
	o.reports.Lock()
	defer o.reports.Unlock()
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errors.New("unusable native identity report")
	}
	for _, key := range []string{"hook_event_name", "session_id", "session_title", "agent_id"} {
		if value, present := fields[key]; present {
			var text string
			if string(value) == "null" || json.Unmarshal(value, &text) != nil {
				return nil, errors.New("unusable native identity report")
			}
		}
	}
	var event struct {
		Event string `json:"hook_event_name"`
		ID    string `json:"session_id"`
		Title string `json:"session_title"`
		Agent string `json:"agent_id"`
	}
	if err := json.Unmarshal(raw, &event); err != nil || (event.Event != "UserPromptSubmit" && event.Event != "Stop" && event.Event != "SessionEnd") || missing(event.ID) || !missing(event.Agent) {
		return nil, errors.New("unusable native identity report")
	}
	o.mu.Lock()
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
			o.withdrawLocked()
		}
		o.mu.Unlock()
		return nil, nil
	}
	if o.admitted != nil && o.admitted.SessionID == event.ID && o.admitted.Name == name {
		o.mu.Unlock()
		return nil, nil
	}
	if o.identity == nil || o.identity.SessionID != event.ID {
		o.withdrawLocked()
		o.ctx, o.cancel = context.WithCancel(context.Background())
	}
	o.identity = identity
	o.admitted = nil
	o.generation++
	generation := o.generation
	if o.connection == nil {
		fd, err := o.dial(o.ctx, "unix", o.socket)
		if err != nil {
			o.ended = true
			o.withdrawLocked()
			o.mu.Unlock()
			return nil, err
		}
		ready := make(chan struct{})
		var c *kit.Connection
		c = kit.NewConnection(fd, func(_ context.Context, r *kit.Request) { <-ready; o.handle(c, r) })
		o.connection = c
		o.caller = kit.NewCaller(func(ctx context.Context, method string, params any) (json.RawMessage, error) {
			var result json.RawMessage
			err := c.Call(ctx, method, params, &result)
			return result, err
		})
		close(ready)
		go func() {
			<-c.Done()
			o.mu.Lock()
			defer o.mu.Unlock()
			if o.connection == c && !o.ended {
				o.ended = true
				o.withdrawLocked()
			}
		}()
	}
	c := o.connection
	o.mu.Unlock()
	var result json.RawMessage
	done := c.Begin("session.hello", identity, &result, func() error {
		o.mu.Lock()
		defer o.mu.Unlock()
		if !o.ended && o.connection == c && o.generation == generation {
			o.admitted = identity
		}
		return nil
	})
	observed := make(chan error, 1)
	go func() {
		err := <-done
		if err != nil {
			o.mu.Lock()
			if o.connection == c && !o.ended {
				o.ended = true
				o.withdrawLocked()
			}
			o.mu.Unlock()
		}
		observed <- err
	}()
	return observed, nil
}

func (o *Owner) withdrawLocked() {
	o.generation++
	o.admitted = nil
	o.identity = nil
	if o.cancel != nil {
		o.cancel()
	}
	c := o.connection
	o.connection = nil
	o.caller = nil
	if c != nil {
		_ = c.Close()
	}
}

func (o *Owner) End() { o.mu.Lock(); defer o.mu.Unlock(); o.ended = true; o.withdrawLocked() }

func (o *Owner) Action(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	o.mu.Lock()
	if o.ended || o.admitted == nil || o.connection == nil || o.ctx.Err() != nil {
		o.mu.Unlock()
		return nil, unavailable()
	}
	caller, identityContext := o.caller, o.ctx
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
	if err != nil && errors.Is(err, context.Canceled) && identityContext.Err() != nil {
		return nil, unavailable()
	}
	return result, err
}

func (o *Owner) handle(c *kit.Connection, r *kit.Request) {
	o.mu.Lock()
	if o.connection != c {
		o.mu.Unlock()
		_ = c.Close()
		return
	}
	if r.Method == "session.superseded" {
		o.ended = true
		o.generation++
		o.admitted = nil
		if o.cancel != nil {
			o.cancel()
		}
		o.mu.Unlock()
		go func() {
			_ = c.Result(r, struct{}{})
			o.mu.Lock()
			defer o.mu.Unlock()
			if o.connection == c {
				o.withdrawLocked()
			}
		}()
		return
	}
	var captured *Recipient
	if o.admitted != nil && !o.ended {
		captured = &Recipient{SessionID: o.admitted.SessionID, Socket: o.nativeSocket, Context: o.ctx}
	}
	o.mu.Unlock()
	go func() {
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
