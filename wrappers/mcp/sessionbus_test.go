// SPDX-License-Identifier: MIT
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

type genericOwner struct{ ended bool }

func (*genericOwner) Action(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	panic("no public action expected")
}
func (o *genericOwner) End() { o.ended = true }

func TestSessionbusNativeReportsAreExplicitAndHidden(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_native_handler", true: "native_handler"}[enabled], func(t *testing.T) {
			owner := &genericOwner{}
			called := false
			report := ReportHandler{}
			if enabled {
				report = ReportHandler{Name: "identity", Begin: func(raw json.RawMessage) (<-chan error, error) {
					called = true
					if string(raw) != `{"native":true}` {
						t.Fatalf("raw=%s", raw)
					}
					return nil, nil
				}}
			}
			input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"identity","arguments":{"native":true}}}
`
			var output bytes.Buffer
			if err := ServeSessionbus(owner, io.NopCloser(strings.NewReader(input)), &output, report); err != nil {
				t.Fatal(err)
			}
			if !owner.ended || called != enabled {
				t.Fatalf("ended=%v called=%v", owner.ended, called)
			}
			decoder := json.NewDecoder(&output)
			var listing struct {
				Result struct{ Tools []struct{ Name string } }
			}
			if err := decoder.Decode(&listing); err != nil {
				t.Fatal(err)
			}
			if len(listing.Result.Tools) != 1 || listing.Result.Tools[0].Name != "sessionbus" {
				t.Fatalf("tools=%+v", listing)
			}
			var reply struct {
				Error  *struct{ Code int }
				Result json.RawMessage
			}
			if err := decoder.Decode(&reply); err != nil {
				t.Fatal(err)
			}
			if enabled && reply.Error != nil || !enabled && (reply.Error == nil || reply.Error.Code != -32602) {
				t.Fatalf("reply=%+v", reply)
			}
		})
	}
}

func TestSessionbusReportCannotShadowPublicTool(t *testing.T) {
	for _, name := range []string{"", " ", "sessionbus"} {
		called := false
		var output bytes.Buffer
		err := ServeSessionbus(&genericOwner{}, io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sessionbus","arguments":{}}}`)), &output, ReportHandler{Name: name, Begin: func(json.RawMessage) (<-chan error, error) { called = true; return nil, nil }})
		if err == nil || called || output.Len() != 0 {
			t.Fatalf("name=%q err=%v called=%v output=%s", name, err, called, output.String())
		}
	}
}

type metadataOwner struct {
	genericOwner
	received chan json.RawMessage
}

func (o *metadataOwner) ActionWithMeta(_ context.Context, _ string, _ json.RawMessage, meta json.RawMessage) (json.RawMessage, error) {
	o.received <- append(json.RawMessage(nil), meta...)
	return json.RawMessage(`{}`), nil
}
func TestSessionbusMetadataIsPerNativeRequest(t *testing.T) {
	input, writer := io.Pipe()
	output, reader := io.Pipe()
	owner := &metadataOwner{received: make(chan json.RawMessage, 1)}
	done := make(chan error, 1)
	go func() { done <- ServeSessionbus(owner, input, reader, ReportHandler{}) }()
	encoder, decoder := json.NewEncoder(writer), json.NewDecoder(output)
	for _, id := range []string{"first", "second"} {
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": "sessionbus", "arguments": map[string]any{"action": "list", "arguments": map[string]any{}}, "_meta": map[string]string{"threadId": id}}}); err != nil {
			t.Fatal(err)
		}
		if got := string(<-owner.received); got != `{"threadId":"`+id+`"}` {
			t.Fatalf("metadata=%s", got)
		}
		var reply map[string]any
		if err := decoder.Decode(&reply); err != nil {
			t.Fatal(err)
		}
		if reply["id"] != id || reply["error"] != nil {
			t.Fatal(reply)
		}
	}
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	output.Close()
	reader.Close()
}
