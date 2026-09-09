// SPDX-License-Identifier: MIT

package codex

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestBrokerWebSocketFragmentationPingAndSingleClient(t *testing.T) {
	m, p := muxFixture(t, nil)
	path := filepath.Join(t.TempDir(), "tui.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server, done := serveBrokerTUI(m, listener)
	defer func() { _ = server.Close(); <-done }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}}
	ws, _, err := websocket.Dial(ctx, "ws://localhost/rpc", &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	f := frame("initialize", "native-init", map[string]string{"padding": strings.Repeat("λ", 40000)})
	b, _ := json.Marshal(f)
	writer, err := ws.Writer(ctx, websocket.MessageText)
	if err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < len(b); offset += 1024 {
		end := min(offset+1024, len(b))
		if _, err = writer.Write(b[offset:end]); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	got := receiveNative(t, p)
	if string(got["params"]) != string(f["params"]) {
		t.Fatal("fragmented payload changed")
	}
	if err = p.Write(brokerFrame{"id": got["id"], "result": brokerRaw(map[string]any{})}); err != nil {
		t.Fatal(err)
	}
	_, raw, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var response brokerFrame
	_ = json.Unmarshal(raw, &response)
	if string(response["id"]) != `"native-init"` {
		t.Fatal(string(raw))
	}
	// Ping needs this client's reader to process the Pong too.
	readDone := make(chan struct{})
	go func() { defer close(readDone); _, _, _ = ws.Read(ctx) }()
	if err = ws.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	second, responseHTTP, err := websocket.Dial(ctx, "ws://localhost/rpc", &websocket.DialOptions{HTTPClient: client})
	if err == nil {
		_ = second.CloseNow()
		t.Fatal("accepted second TUI")
	}
	if responseHTTP.StatusCode != http.StatusConflict {
		t.Fatal(responseHTTP.Status)
	}
	_ = ws.CloseNow()
	<-readDone
}
