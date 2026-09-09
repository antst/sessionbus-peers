// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func residentFixture(t *testing.T) (*brokerOwners, *brokerMux, *muxPipe, <-chan *muxPipe) {
	t.Helper()
	o := newBrokerOwners(context.Background(), []string{"launch"}, "/bus")
	buses := make(chan *muxPipe, 8)
	o.dial = func(context.Context, string, string) (net.Conn, error) {
		a, b := net.Pipe()
		p := &muxPipe{Conn: b, dec: json.NewDecoder(b)}
		buses <- p
		t.Cleanup(func() { _ = p.Close() })
		return a, nil
	}
	m, p := muxFixture(t, o.observe)
	o.setMux(m)
	t.Cleanup(o.End)
	initializeMux(t, m, p)
	return o, m, p, buses
}
func nativeEvent(t *testing.T, p *muxPipe, m *brokerMux, method string, params any) {
	t.Helper()
	if err := p.Write(frame(method, nil, params)); err != nil {
		t.Fatal(err)
	}
	receiveTUI(t, m)
}
func loaded(t *testing.T, o *brokerOwners, m *brokerMux, p *muxPipe, id string) {
	t.Helper()
	nativeEvent(t, p, m, "thread/started", map[string]any{"thread": map[string]any{"id": id, "cwd": "/real", "status": map[string]string{"type": "idle"}}})
}
func readyCatalog(t *testing.T, m *brokerMux, p *muxPipe, id string) {
	t.Helper()
	nativeEvent(t, p, m, "mcpServer/startupStatus/updated", map[string]string{"threadId": id, "name": laneServer, "status": "ready"})
	request := receiveNative(t, p)
	if string(request["method"]) != `"mcpServerStatus/list"` {
		t.Fatal(request)
	}
	if err := p.Write(brokerFrame{"jsonrpc": brokerRaw("2.0"), "id": request["id"], "result": brokerRaw(map[string]any{"data": []any{map[string]any{"name": laneServer, "pluginId": PluginID, "runtimeStatus": "connected", "tools": map[string]any{"sessionbus": map[string]any{}}}}})}); err != nil {
		t.Fatal(err)
	}
}
func receiveBus(t *testing.T, buses <-chan *muxPipe) *muxPipe {
	t.Helper()
	select {
	case p := <-buses:
		return p
	case <-time.After(3 * time.Second):
		t.Fatal("resident did not connect")
	}
	return nil
}
func acceptHello(t *testing.T, p *muxPipe) brokerFrame {
	t.Helper()
	hello := receiveNative(t, p)
	if string(hello["method"]) != `"session.hello"` {
		t.Fatal(hello)
	}
	if err := p.Write(brokerFrame{"jsonrpc": brokerRaw("2.0"), "id": hello["id"], "result": brokerRaw(map[string]any{})}); err != nil {
		t.Fatal(err)
	}
	return hello
}
func residentBarrier(t *testing.T, p *muxPipe) {
	t.Helper() // A same-reader inbound request must follow the hello observer.
	if err := p.Write(frame("message.deliver", 2, kit.DeliveryRequest{MessageID: "preflight", From: kit.DeliverySource{SessionID: "source@local", Product: "fixture", Groups: []string{"launch"}}, Body: "x"})); err != nil {
		t.Fatal(err)
	}
}
func TestBrokerResidentIdentityReadyRenameAndClosed(t *testing.T) {
	o, m, p, buses := residentFixture(t)
	for _, meta := range []string{`{}`, `{"threadId":"unknown"}`} {
		if _, err := o.action(context.Background(), "list", json.RawMessage(`{}`), json.RawMessage(meta)); err == nil {
			t.Fatal("unsettled identity accepted")
		}
	}
	// Early startup is bound to this exact thread; no helper-spawn attribution.
	readyCatalogBefore := func() {
		nativeEvent(t, p, m, "mcpServer/startupStatus/updated", map[string]string{"threadId": "one", "name": laneServer, "status": "ready"})
	}
	readyCatalogBefore()
	loaded(t, o, m, p, "one")
	request := receiveNative(t, p)
	if err := p.Write(brokerFrame{"jsonrpc": brokerRaw("2.0"), "id": request["id"], "result": brokerRaw(map[string]any{"data": []any{map[string]any{"name": laneServer, "pluginId": PluginID, "runtimeStatus": "connected", "tools": map[string]any{"sessionbus": map[string]any{}}}}})}); err != nil {
		t.Fatal(err)
	}
	bus := receiveBus(t, buses)
	hello := acceptHello(t, bus)
	var identity kit.Identity
	if err := json.Unmarshal(hello["params"], &identity); err != nil {
		t.Fatal(err)
	}
	if identity.SessionID != "one" || identity.Name != "" || len(identity.Groups) != 1 || identity.Groups[0] != "launch" {
		t.Fatal(identity)
	}
	// Rename and clear keep the same connection and native ID.
	for _, name := range []any{"new title", nil} {
		nativeEvent(t, p, m, "thread/name/updated", map[string]any{"threadId": "one", "threadName": name})
		h := acceptHello(t, bus)
		var value map[string]any
		_ = json.Unmarshal(h["params"], &value)
		if value["session_id"] != "one" || value["name"] != name {
			t.Fatal(value)
		}
	}
	nativeEvent(t, p, m, "thread/closed", map[string]string{"threadId": "one"})
	var frame brokerFrame
	_ = bus.SetReadDeadline(time.Now().Add(time.Second))
	if err := bus.Read(&frame); err == nil {
		t.Fatal("closed thread retained connection")
	}
	if _, err := o.action(context.Background(), "list", json.RawMessage(`{}`), json.RawMessage(`{"threadId":"one"}`)); err == nil {
		t.Fatal("closed thread accepted action")
	}
}
func TestBrokerResidentToolAndDeliveryMakeProgressTogether(t *testing.T) {
	o, m, p, buses := residentFixture(t)
	loaded(t, o, m, p, "one")
	readyCatalog(t, m, p, "one")
	bus := receiveBus(t, buses)
	acceptHello(t, bus)
	// A delivery after hello is processed without waiting on a public tool call.
	residentBarrier(t, bus)
	read := receiveNative(t, p)
	if string(read["method"]) != `"thread/read"` {
		t.Fatal(read)
	}
	if err := p.Write(brokerFrame{"jsonrpc": brokerRaw("2.0"), "id": read["id"], "result": brokerRaw(map[string]any{"thread": map[string]any{"id": "one", "status": map[string]string{"type": "idle"}}})}); err != nil {
		t.Fatal(err)
	}
	turn := receiveNative(t, p)
	if string(turn["method"]) != `"turn/start"` {
		t.Fatal(turn)
	}
	action := make(chan error, 1)
	go func() {
		_, err := o.action(context.Background(), "list", json.RawMessage(`{}`), json.RawMessage(`{"threadId":"one","preserved":"native"}`))
		action <- err
	}()
	public := receiveNative(t, bus)
	if string(public["method"]) != `"session.list"` {
		t.Fatal(public)
	}
	if err := p.Write(brokerFrame{"jsonrpc": brokerRaw("2.0"), "id": turn["id"], "result": brokerRaw(map[string]any{"turn": map[string]string{"id": "new-turn"}})}); err != nil {
		t.Fatal(err)
	}
	receipt := receiveNative(t, bus)
	if string(receipt["id"]) != "2" || string(receipt["result"]) != `{"disposition":"injected"}` {
		t.Fatal(receipt)
	}
	if err := bus.Write(brokerFrame{"jsonrpc": brokerRaw("2.0"), "id": public["id"], "result": brokerRaw(map[string]any{"sessions": []any{}})}); err != nil {
		t.Fatal(err)
	}
	if err := <-action; err != nil {
		t.Fatal(err)
	}
	// EOF closes this resident permanently; there is no Peer/reconnect loop.
	_ = bus.Close()
	o.End()
	select {
	case unexpected := <-buses:
		_ = unexpected.Close()
		t.Fatal("unexpected reconnect")
	default:
	}
}
func TestBrokerInitialNameOnlyCorrelatedTUISelection(t *testing.T) {
	o, m, p, _ := residentFixture(t)
	o.initialName = "initial name"
	// Install the selection callback before sending any selection request.
	m.mu.Lock()
	m.selectThread = o.selectThread
	m.mu.Unlock()
	loaded(t, o, m, p, "subthread")
	if err := m.fromTUI(frame("thread/resume", 9, map[string]string{"threadId": "selected"})); err != nil {
		t.Fatal(err)
	}
	selection := receiveNative(t, p)
	if err := p.Write(brokerFrame{"jsonrpc": brokerRaw("2.0"), "id": selection["id"], "result": brokerRaw(map[string]any{"thread": map[string]any{"id": "selected", "cwd": "/real"}})}); err != nil {
		t.Fatal(err)
	}
	receiveTUI(t, m)
	naming := receiveNative(t, p)
	var params map[string]string
	_ = json.Unmarshal(naming["params"], &params)
	if string(naming["method"]) != `"thread/name/set"` || params["threadId"] != "selected" || params["name"] != "initial name" {
		t.Fatal(naming)
	}
	if err := p.Write(brokerFrame{"jsonrpc": brokerRaw("2.0"), "id": naming["id"], "result": brokerRaw(map[string]any{})}); err != nil {
		t.Fatal(err)
	}
	if err := o.selectThread("thread/fork", brokerRaw(map[string]any{"thread": map[string]string{"id": "later"}})); err != nil {
		t.Fatal(err)
	}
	if err := m.fromTUI(frame("thread/read", 10, map[string]string{"threadId": "selected"})); err != nil {
		t.Fatal(err)
	}
	if next := receiveNative(t, p); string(next["method"]) != `"thread/read"` {
		t.Fatal("renamed subsequent selection")
	}
}

func TestBrokerMCPForwarderCatalogEOFDoesNotWithdrawResident(t *testing.T) {
	o, m, p, _ := residentFixture(t)
	loaded(t, o, m, p, "loaded")
	endpoint, err := newBrokerEndpoint(t.TempDir(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	c, err := net.Dial("unix", endpoint.path)
	if err != nil {
		t.Fatal(err)
	}
	peer := &muxPipe{Conn: c, dec: json.NewDecoder(c)}
	defer peer.Close()
	for _, method := range []string{"initialize", "tools/list"} {
		params := map[string]any{}
		if method == "initialize" {
			params["protocolVersion"] = "2024-11-05"
		}
		if err := peer.Write(frame(method, 1, params)); err != nil {
			t.Fatal(err)
		}
		if reply := receiveNative(t, peer); reply["error"] != nil {
			t.Fatal(reply)
		}
	}
	if err := peer.Write(frame("tools/call", 2, map[string]any{"name": "sessionbus", "arguments": map[string]any{"action": "list", "arguments": map[string]any{}}, "_meta": map[string]any{"threadId": "unknown"}})); err != nil {
		t.Fatal(err)
	}
	reply := receiveNative(t, peer)
	if !strings.Contains(string(reply["result"]), "not loaded") {
		t.Fatal(reply)
	}
	_ = c.Close()
	o.mu.Lock()
	resident := o.owners["loaded"]
	o.mu.Unlock()
	resident.mu.Lock()
	closed := resident.closed
	resident.mu.Unlock()
	if closed {
		t.Fatal("unidentified catalog connection withdrew native resident")
	}
}

func TestBrokerActiveDeliveryUsesExpectedNativeTurnAndNoReplay(t *testing.T) {
	for _, loss := range []bool{false, true} {
		t.Run(map[bool]string{false: "native-ack", true: "uncertain-write"}[loss], func(t *testing.T) {
			o, m, p, _ := residentFixture(t)
			loaded(t, o, m, p, "one")
			o.mu.Lock()
			resident := o.owners["one"]
			o.mu.Unlock()
			type outcome struct {
				receipt kit.DeliveryReceipt
				err     error
			}
			done := make(chan outcome, 1)
			go func() {
				receipt, err := resident.deliver(context.Background(), kit.DeliveryRequest{MessageID: "message", From: kit.DeliverySource{SessionID: "sender@local", Product: "fixture", Groups: []string{}}, Body: "native input"})
				done <- outcome{receipt, err}
			}()
			request := receiveNative(t, p)
			if string(request["method"]) != `"thread/read"` {
				t.Fatal(request)
			}
			if err := p.Write(brokerFrame{"id": request["id"], "result": brokerRaw(map[string]any{"thread": map[string]any{"id": "one", "status": map[string]string{"type": "active"}}})}); err != nil {
				t.Fatal(err)
			}
			turns := receiveNative(t, p)
			if string(turns["method"]) != `"thread/turns/list"` {
				t.Fatal(turns)
			}
			if err := p.Write(brokerFrame{"id": turns["id"], "result": brokerRaw(map[string]any{"data": []any{map[string]string{"id": "native-turn", "status": "inProgress"}}})}); err != nil {
				t.Fatal(err)
			}
			steer := receiveNative(t, p)
			var params map[string]any
			_ = json.Unmarshal(steer["params"], &params)
			if string(steer["method"]) != `"turn/steer"` || params["threadId"] != "one" || params["expectedTurnId"] != "native-turn" {
				t.Fatal(steer)
			}
			if loss {
				_ = p.Close()
			} else if err := p.Write(brokerFrame{"id": steer["id"], "result": brokerRaw(map[string]string{"turnId": "native-turn"})}); err != nil {
				t.Fatal(err)
			}
			result := <-done
			if loss {
				if result.err == nil {
					t.Fatal("transport loss invented native receipt")
				}
			} else if result.err != nil || result.receipt.Disposition != "injected" {
				t.Fatal(result)
			}
			m.mu.Lock()
			remaining := len(m.calls)
			m.mu.Unlock()
			if remaining != 0 {
				t.Fatal("retained or replayed native call", remaining)
			}
		})
	}
}
