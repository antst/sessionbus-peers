// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/host"
)

func TestLaneModeRejectsArguments(t *testing.T) {
	t.Setenv(host.TokenEnv, "token")
	if err := run(context.Background(), []string{"mcp"}); err == nil || err.Error() != "lane mode accepts no arguments" {
		t.Fatalf("run = %v", err)
	}
}

func TestMCPRequiresManagedLauncher(t *testing.T) {
	t.Setenv("SESSIONBUS_LANE_SOCKET", "")
	err := runMCP(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start Grok with grok-peer") {
		t.Fatalf("runMCP = %v", err)
	}
}

func TestSharedMCPEOFSettlesActualCaller(t *testing.T) {
	input, writer := io.Pipe()
	var output bytes.Buffer
	entered, settled, ended := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	caller := kit.NewCaller(func(ctx context.Context, method string, args any) (json.RawMessage, error) {
		if method != "session.list" {
			t.Errorf("method=%s", method)
		}
		close(entered)
		<-ctx.Done()
		close(settled)
		return nil, ctx.Err()
	})
	owner := grokMCPOwner{action: caller.Action, end: func() { once.Do(func() { close(ended) }) }}
	done := make(chan error, 1)
	go func() { done <- serveMCP(context.Background(), owner, input, &output) }()
	_, err := io.WriteString(writer, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sessionbus","arguments":{"action":"list","arguments":{}}}}`+"\n")
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	_ = writer.Close()
	<-settled
	<-ended
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("late reply after EOF: %s", output.String())
	}
}
