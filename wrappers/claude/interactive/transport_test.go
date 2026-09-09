// SPDX-License-Identifier: MIT

package interactive

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
)

func nativeReportFrame(id int, event, session, title string) any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": HiddenTool, "arguments": map[string]string{"hook_event_name": event, "session_id": session, "session_title": title}}}
}

func TestServeEOFAndEndCancelBlockedTransport(t *testing.T) {
	for _, transport := range []string{"dial", "hello-write"} {
		for _, ending := range []string{"EOF", "SessionEnd", "End"} {
			t.Run(transport+"/"+ending, func(t *testing.T) {
				o, _ := testOwner(t)
				entered := make(chan struct{})
				released := make(chan struct{})
				var peer net.Conn
				o.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
					if transport == "dial" {
						close(entered)
						<-ctx.Done()
						close(released)
						return nil, ctx.Err()
					}
					a, b := net.Pipe()
					peer = b
					var once sync.Once
					return writeConn{Conn: a, write: func(body []byte) (int, error) {
						once.Do(func() { close(entered) })
						n, err := a.Write(body)
						close(released)
						return n, err
					}}, nil
				}
				h := newMCP(t, o)
				h.send(t, nativeReportFrame(1, "Stop", "id", ""))
				<-entered
				switch ending {
				case "EOF":
					_ = h.input.Close()
					<-h.done
				case "End":
					o.End()
				case "SessionEnd":
					h.send(t, nativeReportFrame(2, "SessionEnd", "id", ""))
					h.send(t, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "ping"})
					for {
						f := h.next(t)
						if string(f["id"]) == "3" {
							break
						}
					}
				}
				// Only closing/cancelling the owner releases transport, never the test.
				<-released
				if peer != nil {
					_ = peer.Close()
				}
			})
		}
	}
}

func TestNewestUnsubmittedReportSurvivesOverlappingDial(t *testing.T) {
	o, _ := testOwner(t)
	entered, release := make(chan struct{}), make(chan struct{})
	wires := make(chan *wire, 1)
	o.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		a, b := net.Pipe()
		wires <- newWire(t, b)
		return a, nil
	}
	old := report(t, o, "UserPromptSubmit", "id", "old")
	<-entered
	latest := report(t, o, "UserPromptSubmit", "id", "latest")
	close(release)
	w := <-wires
	f := w.next(t)
	var identity map[string]any
	_ = json.Unmarshal(f.Params, &identity)
	if identity["name"] != "latest" {
		t.Fatal(identity)
	}
	w.reply(t, f, map[string]any{})
	if err := <-latest; err != nil {
		t.Fatal(err)
	}
	if err := <-old; err == nil {
		t.Fatal("obsolete unsubmitted report claimed success")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ended || o.admitted == nil || o.admitted.Name != "latest" {
		t.Fatal("obsolete work ended or poisoned latest report")
	}
}

func TestSubmittedOldHelloCannotFollowNewHello(t *testing.T) {
	o, _ := testOwner(t)
	entered := make(chan struct{})
	server := make(chan net.Conn, 1)
	o.dial = func(context.Context, string, string) (net.Conn, error) {
		a, b := net.Pipe()
		server <- b
		var once sync.Once
		return writeConn{Conn: a, write: func(body []byte) (int, error) { once.Do(func() { close(entered) }); return a.Write(body) }}, nil
	}
	old := report(t, o, "UserPromptSubmit", "id", "old")
	<-entered
	latest := report(t, o, "UserPromptSubmit", "id", "latest")
	w := newWire(t, <-server)
	first := w.next(t)
	var identity map[string]any
	_ = json.Unmarshal(first.Params, &identity)
	if identity["name"] != "old" {
		t.Fatal(identity)
	}
	w.reply(t, first, map[string]any{})
	if err := <-old; err != nil {
		t.Fatal(err)
	}
	o.mu.Lock()
	admitted := o.admitted
	o.mu.Unlock()
	if admitted != nil {
		t.Fatal("late old acknowledgment admitted")
	}
	second := w.next(t)
	_ = json.Unmarshal(second.Params, &identity)
	if identity["name"] != "latest" {
		t.Fatal(identity)
	}
	w.reply(t, second, map[string]any{})
	if err := <-latest; err != nil {
		t.Fatal(err)
	}
}
