// SPDX-License-Identifier: MIT

package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"sync"

	"github.com/antst/sessionbus-peers/wrappers/mcp"
)

func netListenBroker(path string) (net.Listener, error) { return net.Listen("unix", path) }

type brokerEndpoint struct {
	path        string
	listener    net.Listener
	mu          sync.Mutex
	connections map[net.Conn]bool
	closed      bool
}

func newBrokerEndpoint(dir string, owners *brokerOwners) (*brokerEndpoint, error) {
	path := filepath.Join(dir, "owner.sock")
	listener, err := netListenBroker(path)
	if err != nil {
		return nil, err
	}
	e := &brokerEndpoint{path: path, listener: listener, connections: map[net.Conn]bool{}}
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			e.mu.Lock()
			if e.closed || len(e.connections) >= brokerRouteLimit {
				e.mu.Unlock()
				_ = c.Close()
				continue
			}
			e.connections[c] = true
			e.mu.Unlock()
			go func() {
				_ = mcp.ServeSessionbus(&brokerToolOwner{owners: owners}, c, c, mcp.ReportHandler{})
				_ = c.Close()
				e.mu.Lock()
				delete(e.connections, c)
				e.mu.Unlock()
			}()
		}
	}()
	return e, nil
}
func (e *brokerEndpoint) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	_ = e.listener.Close()
	for c := range e.connections {
		_ = c.Close()
	}
	return nil
}

type brokerToolOwner struct{ owners *brokerOwners }

func (b *brokerToolOwner) Action(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("native metadata is required")
}
func (b *brokerToolOwner) ActionWithMeta(ctx context.Context, action string, args, meta json.RawMessage) (json.RawMessage, error) {
	return b.owners.action(ctx, action, args, meta)
}

// A forwarder has no bus ownership. Native exact thread/closed or whole-server
// loss retires its row; EOF on an unidentified MCP child cannot identify a row.
func (b *brokerToolOwner) End() {}
