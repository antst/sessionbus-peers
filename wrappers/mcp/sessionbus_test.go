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
