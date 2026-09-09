// SPDX-License-Identifier: MIT
package claude

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func endpointFixture(t *testing.T) (*Wrapper, *laneEndpoint) {
	t.Helper()
	p := New(t.TempDir())
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.native = &exec.Cmd{Process: &os.Process{Pid: 12345}}
	close(p.spawnReady)
	e, err := newLaneEndpoint(p)
	if err != nil {
		t.Fatal(err)
	}
	p.endpoint = e
	t.Cleanup(func() { _ = p.Close(context.Background(), kit.SessionCloseRequest{}) })
	return p, e
}
func TestInitialReportBindsEmitterAndTransientClose(t *testing.T) {
	p, e := endpointFixture(t)
	input := `{"hook_event_name":"SessionStart","session_id":"root-id","session_title":"native title"}`
	if err := InitialReport(context.Background(), e.path, "99999", strings.NewReader(input)); err == nil {
		t.Fatal("foreign emitter accepted")
	}
	if err := InitialReport(context.Background(), e.path, "12345", strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	<-p.reportReady
	p.mu.Lock()
	id, title, failed := p.identity, p.title, p.failure
	p.mu.Unlock()
	if id != "root-id" || title != "native title" || failed != nil {
		t.Fatalf("%q %q %v", id, title, failed)
	}
	// Each initial connection is closed. A subsequent admitted initial report is
	// still possible; the transient connection cannot end the worker.
	if err := InitialReport(context.Background(), e.path, "12345", strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
}
func TestEstablishedForwarderEOFEndsWorker(t *testing.T) {
	p, e := endpointFixture(t)
	ended := make(chan struct{}, 1)
	p.SetShutdown(func() { ended <- struct{}{} })
	c, err := net.Dial("unix", e.path)
	if err != nil {
		t.Fatal(err)
	}
	enc, dec := json.NewEncoder(c), json.NewDecoder(c)
	if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}); err != nil {
		t.Fatal(err)
	}
	var reply map[string]json.RawMessage
	if err := dec.Decode(&reply); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	<-ended
	p.mu.Lock()
	failed := p.failure
	p.mu.Unlock()
	if failed == nil || !strings.Contains(failed.Error(), "forwarder disconnected") {
		t.Fatal(failed)
	}
}
func TestForwardUsesExistingMCPAndClosesOnStdioEOF(t *testing.T) {
	p, e := endpointFixture(t)
	ended := make(chan struct{}, 1)
	p.SetShutdown(func() { ended <- struct{}{} })
	input, inputWriter := io.Pipe()
	outputReader, output := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Forward(context.Background(), e.path, input, output); _ = output.Close() }()
	encoder, decoder := json.NewEncoder(inputWriter), json.NewDecoder(outputReader)
	if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 7, "method": "tools/list"}); err != nil {
		t.Fatal(err)
	}
	var response struct {
		ID     int
		Result struct{ Tools []struct{ Name string } }
	}
	if err := decoder.Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ID != 7 || len(response.Result.Tools) != 1 || response.Result.Tools[0].Name != "sessionbus" {
		t.Fatalf("%+v", response)
	}
	_ = inputWriter.Close()
	<-ended
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = outputReader.Close()
}
func TestInitialHookRequiresNativeEmitterAndRootFields(t *testing.T) {
	_, e := endpointFixture(t)
	for i, body := range []string{`{"hook_event_name":"SessionStart","session_id":"root-id","agent_id":"nested"}`, `{"hook_event_name":"SessionStart","session_id":""}`} {
		if err := InitialReport(context.Background(), e.path, strconv.Itoa(12345), strings.NewReader(body)); err == nil {
			t.Fatalf("invalid report %d accepted", i)
		}
	}
}
func TestExplicitInteractiveLaunchDropsInheritedLaneEndpoint(t *testing.T) {
	_, env, err := interactive.LaunchPlan([]string{"-n", "child", "-g", "own"}, map[string]string{LaneEndpointEnv: "/parent/owner.sock"}, "/cwd", "/plugin", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range env {
		if strings.HasPrefix(value, LaneEndpointEnv+"=") {
			t.Fatal("child inherited parent forwarder")
		}
	}
}
