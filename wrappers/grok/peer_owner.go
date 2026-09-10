// SPDX-License-Identifier: MIT
package grok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/antst/sessionbus-peers/wrappers/host"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

type PeerBackend struct {
	mu                               sync.Mutex
	identity                         kit.PeerIdentity
	leader, cwd, socket, initialName string
	ctx                              context.Context
	cancel                           context.CancelFunc
	conn                             *kit.Connection
	caller                           *kit.Caller
	observer                         *acpClient
	process                          *nativeProcess
	ready, done                      chan struct{}
	changed                          chan struct{}
	deliveryGate                     chan struct{}
	initialized                      sync.Once
	work                             sync.WaitGroup
	admitted, ending                 bool
	err                              error
	slots                            chan struct{}
}

func NewPeerBackend(ctx context.Context, env []string) (*PeerBackend, error) {
	if !ManagedHelper(env) {
		return nil, errors.New("Grok peer identity is unavailable; start Grok with grok-peer")
	}
	groups := []string{}
	if raw := environmentValue(env, host.GroupsEnv); raw != "" {
		if json.Unmarshal([]byte(raw), &groups) != nil || groups == nil {
			return nil, errors.New("SESSIONBUS_GROUPS must be a JSON array")
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	b := &PeerBackend{identity: kit.PeerIdentity{Protocol: 1, Product: Product, SessionID: environmentValue(env, grokSessionIDEnv), Groups: groups, Info: map[string]any{"cwd": cwd}}, leader: environmentValue(env, grokLeaderSocketEnv), cwd: cwd, socket: first(environmentValue(env, host.SocketEnv), kit.Socket()), ctx: lifetime, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{}), changed: make(chan struct{}, 1), deliveryGate: make(chan struct{}, 1), slots: make(chan struct{}, maxACPPending)}
	b.deliveryGate <- struct{}{}
	if name := environmentValue(env, host.NameEnv); name != "" {
		won, e := claimInitialName(b.leader)
		if e != nil {
			cancel()
			return nil, e
		}
		if won {
			b.initialName = name
		}
	}
	b.caller = kit.NewCaller(b.Call)
	return b, nil
}
func claimInitialName(leader string) (bool, error) {
	dir := filepath.Dir(leader)
	info, err := os.Lstat(dir)
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return false, errors.New("Grok launch name claim requires a private runtime directory")
	}
	file, err := os.OpenFile(filepath.Join(dir, "initial-name.claim"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, file.Close()
}

// Native ACP must not run before Grok receives its MCP initialize response.
func (b *PeerBackend) Initialized() { b.initialized.Do(func() { go b.run() }) }
func (b *PeerBackend) run() {
	defer close(b.done)
	defer func() {
		b.mu.Lock()
		b.ending = true
		c, o, process := b.conn, b.observer, b.process
		b.mu.Unlock()
		b.cancel()
		if c != nil {
			_ = c.Close()
		}
		stopPeerClient(o, process)
	}()
	err := b.runOwner()
	b.mu.Lock()
	if b.err == nil {
		b.err = err
	}
	b.mu.Unlock()
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "sessionbus: Grok helper:", err)
	}
}
func (b *PeerBackend) runOwner() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	observer, process, err := startPeerClient(b.ctx, b.ctx, b.leader, b.cwd, func(frame acpFrame) {
		if !replayFrame(frame) && frame.Method == "_x.ai/sessions/changed" {
			select {
			case b.changed <- struct{}{}:
			default:
			}
		}
	})
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.observer, b.process = observer, process
	ending := b.ending
	b.mu.Unlock()
	if ending {
		return context.Canceled
	}
	var row peerSession
	for {
		row, err = roster(b.ctx, observer, b.identity.SessionID)
		if err == nil {
			break
		}
		if !errors.Is(err, errNoLeader) {
			return err
		}
		select {
		case <-b.changed:
		case <-observer.done:
			return errors.New("native actor ended before publication")
		case <-b.ctx.Done():
			return b.ctx.Err()
		}
	}
	if b.initialName != "" {
		var result struct {
			Success bool `json:"success"`
		}
		if err = observer.request(b.ctx, "_x.ai/session/rename", map[string]string{"sessionId": b.identity.SessionID, "title": b.initialName}, &result); err != nil {
			return err
		}
		if !result.Success {
			return errors.New("native initial rename failed")
		}
		row, err = roster(b.ctx, observer, b.identity.SessionID)
		if err != nil {
			return err
		}
		if row.Title != b.initialName {
			return errors.New("native initial title does not match requested name")
		}
	}
	fd, err := (&net.Dialer{}).DialContext(b.ctx, "unix", b.socket)
	if err != nil {
		return err
	}
	var c *kit.Connection
	assigned := make(chan struct{})
	c = kit.NewConnection(fd, func(ctx context.Context, r *kit.Request) { <-assigned; b.handle(ctx, c, r) })
	b.mu.Lock()
	b.conn = c
	ending = b.ending
	b.mu.Unlock()
	close(assigned)
	if ending {
		_ = c.Close()
		return context.Canceled
	}
	if err = b.publish(c, row); err != nil {
		return err
	}
	close(b.ready)
	for {
		select {
		case <-b.changed:
			row, err = roster(b.ctx, observer, b.identity.SessionID)
			if err != nil {
				return err
			}
			if err = b.publish(c, row); err != nil {
				return err
			}
		case <-observer.done:
			return errors.New("Grok native observer ended")
		case <-c.Done():
			return errors.New("Sessionbus owner connection ended")
		case <-b.ctx.Done():
			return b.ctx.Err()
		}
	}
}
func (b *PeerBackend) publish(c *kit.Connection, row peerSession) error {
	b.mu.Lock()
	if b.ending {
		b.mu.Unlock()
		return context.Canceled
	}
	next := b.identity
	next.Name = row.Title
	next.Info = map[string]any{"cwd": row.Cwd}
	if b.admitted && next.Name == b.identity.Name && row.Cwd == b.cwd {
		b.mu.Unlock()
		return nil
	}
	b.mu.Unlock()
	var result json.RawMessage
	return c.CallObserved(b.ctx, "session.hello", next, &result, func() error {
		b.mu.Lock()
		defer b.mu.Unlock()
		if !b.ending {
			b.identity = next
			b.cwd = row.Cwd
			b.admitted = true
		}
		return nil
	})
}
func (b *PeerBackend) waitReady(ctx context.Context) error {
	select {
	case <-b.ready:
	case <-b.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	if b.ending || !b.admitted {
		return errors.New("Grok peer is unavailable")
	}
	return nil
}
func (b *PeerBackend) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := b.waitReady(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	c := b.conn
	b.mu.Unlock()
	var result json.RawMessage
	err := c.Call(ctx, method, params, &result)
	return result, err
}
func (b *PeerBackend) Caller() *kit.Caller                                  { return b.caller }
func (b *PeerBackend) Prepare(ctx context.Context, _ json.RawMessage) error { return b.waitReady(ctx) }
func (b *PeerBackend) Action(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	return b.caller.Action(ctx, action, args)
}
func (b *PeerBackend) End() { b.Shutdown() }
func (b *PeerBackend) Shutdown() {
	b.mu.Lock()
	b.ending = true
	c, o := b.conn, b.observer
	b.mu.Unlock()
	b.cancel()
	if c != nil {
		_ = c.Close()
	}
	if o != nil {
		o.close()
	}
	b.initialized.Do(func() { close(b.done) })
	<-b.done
	b.work.Wait()
}
func (b *PeerBackend) handle(ctx context.Context, c *kit.Connection, r *kit.Request) {
	b.mu.Lock()
	if b.ending {
		b.mu.Unlock()
		return
	}
	select {
	case b.slots <- struct{}{}:
	default:
		b.mu.Unlock()
		_ = c.Close()
		return
	}
	b.work.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.work.Done()
		defer func() { <-b.slots }()
		var err error
		switch r.Method {
		case "session.superseded":
			b.mu.Lock()
			b.ending = true
			b.admitted = false
			b.mu.Unlock()
			err = c.Result(r, struct{}{})
			b.cancel()
		case "message.deliver":
			request, ok := r.Params.(*kit.DeliveryRequest)
			if !ok {
				_ = c.Close()
				return
			}
			b.mu.Lock()
			id := b.identity
			b.mu.Unlock()
			receipt, e := b.deliver(ctx, id, *request)
			if e != nil {
				err = c.Error(r, -32603, "uncertain_native_admission")
			} else {
				err = c.Result(r, receipt)
			}
		default:
			err = c.Error(r, -32601, nil)
		}
		if err != nil {
			_ = c.Close()
		}
	}()
}
func (b *PeerBackend) deliver(ctx context.Context, identity kit.PeerIdentity, request kit.DeliveryRequest) (kit.DeliveryReceipt, error) {
	if err := b.waitReady(ctx); err != nil {
		return kit.DeliveryReceipt{}, err
	}
	select {
	case <-ctx.Done():
		return kit.DeliveryReceipt{}, ctx.Err()
	case <-b.ctx.Done():
		return kit.DeliveryReceipt{}, b.ctx.Err()
	case <-b.deliveryGate:
	}
	defer func() { b.deliveryGate <- struct{}{} }()
	b.mu.Lock()
	observer, ending, id := b.observer, b.ending, b.identity.SessionID
	b.mu.Unlock()
	if ending || identity.SessionID != id {
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "shutting down"}, nil
	}
	message, err := host.RenderNativeMessage(request)
	if err != nil {
		return kit.DeliveryReceipt{}, err
	}
	if err = observer.interject(ctx, id, request.MessageID, message); err != nil {
		return kit.DeliveryReceipt{}, err
	}
	return kit.DeliveryReceipt{Disposition: "injected"}, nil
}
