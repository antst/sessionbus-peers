// SPDX-License-Identifier: MIT
package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antst/sessionbus-peers/internal/testsocket"
	"github.com/antst/sessionbus-peers/wrappers/host"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

func TestMain(m *testing.M) {
	if os.Getenv("OPENCODE_TEST_NATIVE") == "1" {
		fakeNativeHTTP()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fakePart(session, id, kind, text string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"sessionID": session, "messageID": id, "type": kind, "text": text})
	return b
}
func fakeNativeHTTP() {
	cwd, _ := os.Getwd()
	id := "ses_native"
	if strings.Join(os.Args[1:], " ") != "serve --hostname 127.0.0.1 --port 0" {
		os.Exit(5)
	}
	for _, key := range []string{host.SocketEnv, host.LocalKeyEnv, host.TokenEnv, host.SessionIDEnv, host.NameEnv, host.GroupsEnv} {
		if os.Getenv(key) != "" {
			os.Exit(6)
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	var mu sync.Mutex
	var history []withParts
	var aborts int
	var hold chan struct{}
	var interrupted bool
	var permission any
	events := make(chan any, 256)
	var helper net.Conn
	var init sync.Once
	emit := func(kind string, properties any) { events <- map[string]any{"type": kind, "properties": properties} }
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != os.Getenv("OPENCODE_SERVER_USERNAME") || p != os.Getenv("OPENCODE_SERVER_PASSWORD") || r.Header.Get("x-opencode-directory") != cwd || r.URL.Query().Get("directory") != cwd {
			http.Error(w, "scope", 403)
			return
		}
		reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.URL.Path == "/event":
			init.Do(func() {
				helper, err = net.Dial("unix", os.Getenv(LaneSocketEnv))
				if err != nil {
					panic(err)
				}
				_, _ = io.WriteString(helper, "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"fake-native\",\"version\":\"1\"}}}\n")
				_, err = bufio.NewReader(helper).ReadBytes('\n')
				if err != nil {
					panic(err)
				}
			})
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"server.connected\",\"properties\":{}}\n\n")
			w.(http.Flusher).Flush()
			for {
				select {
				case e := <-events:
					b, _ := json.Marshal(e)
					_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		case r.URL.Path == "/experimental/tool/ids":
			reply([]string{ToolName})
		case r.URL.Path == "/session" && r.Method == "POST" || r.URL.Path == "/session/"+id && r.Method == "PATCH":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			permission = req["permission"]
			mu.Unlock()
			reply(nativeSession{ID: id, Title: req["title"].(string), Directory: cwd})
		case r.URL.Path == "/session/"+id && r.Method == "GET":
			reply(nativeSession{ID: id, Title: "lane", Directory: cwd})
		case r.URL.Path == "/session/ses_child":
			reply(nativeSession{ID: "ses_child", ParentID: id, Directory: cwd})
		case r.URL.Path == "/session/ses_other":
			reply(nativeSession{ID: "ses_other", Directory: cwd})
		case r.URL.Path == "/session/"+id && r.Method == "DELETE":
			reply(true)
		case r.URL.Path == "/session/"+id+"/message" && r.Method == "GET":
			mu.Lock()
			copy := append([]withParts{}, history...)
			mu.Unlock()
			reply(copy)
		case r.URL.Path == "/session/"+id+"/message" && r.Method == "POST":
			var req struct {
				MessageID string `json:"messageID"`
				NoReply   bool   `json:"noReply"`
				Parts     []struct {
					Text string `json:"text"`
				} `json:"parts"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			text := req.Parts[0].Text
			user := withParts{Info: nativeInfo{ID: req.MessageID, SessionID: id, Role: "user"}, Parts: []json.RawMessage{fakePart(id, req.MessageID, "text", text)}}
			mu.Lock()
			history = append(history, user)
			mu.Unlock()
			emit("message.updated", map[string]any{"info": user.Info})
			if req.NoReply {
				reply(user)
				return
			}
			mu.Lock()
			interrupted = false
			hold = make(chan struct{})
			gate := hold
			mu.Unlock()
			emit("session.status", map[string]any{"sessionID": id, "status": map[string]string{"type": "busy"}})
			if strings.HasPrefix(text, "hold") {
				select {
				case <-gate:
				case <-r.Context().Done():
					return
				}
			}
			mu.Lock()
			parent := req.MessageID
			for _, m := range history {
				if m.Info.Role == "user" {
					parent = m.Info.ID
				}
			}
			answer := withParts{Info: nativeInfo{ID: "msg_answer_" + strconv.Itoa(len(history)), SessionID: id, ParentID: parent, Role: "assistant", Finish: "stop"}}
			now := float64(1)
			answer.Info.Time.Completed = &now
			answer.Parts = []json.RawMessage{fakePart(id, answer.Info.ID, "text", "answer:"+text)}
			if interrupted && text == "hold-summary" {
				answer.Info.Summary = true
			}
			if interrupted && text != "hold-summary" {
				answer.Info.Error = json.RawMessage(`{"name":"MessageAbortedError","data":{"message":"aborted"}}`)
			}
			history = append(history, answer)
			hold = nil
			mu.Unlock()
			reply(answer)
		case r.URL.Path == "/session/"+id+"/abort":
			mu.Lock()
			aborts++
			interrupted = true
			if hold != nil {
				close(hold)
				hold = nil
			}
			mu.Unlock()
			reply(true)
		case r.URL.Path == "/fixture/release":
			mu.Lock()
			if hold != nil {
				close(hold)
				hold = nil
			}
			mu.Unlock()
			reply(true)
		case r.URL.Path == "/fixture/state":
			mu.Lock()
			reply(map[string]any{"aborts": aborts, "messages": history, "permission": permission})
			mu.Unlock()
		default:
			http.Error(w, r.URL.Path, 404)
		}
	})}
	fmt.Printf("opencode server listening on http://%s\n", l.Addr())
	_ = srv.Serve(l)
}

type workerFixture struct {
	p       *Wrapper
	worker  *kit.Worker
	c       net.Conn
	mu      sync.Mutex
	next    int64
	pending map[int64]chan protocol.Frame
	ready   chan protocol.Frame
	done    chan struct{}
	ctx     context.Context
}

func newWorkerFixture(t *testing.T) *workerFixture {
	return newWorkerProductFixture(t, nil)
}
func newWorkerProductFixture(t *testing.T, decorate func(*Wrapper) kit.WorkerCallbacks) *workerFixture {
	t.Helper()
	dir := testsocket.Directory(t)
	socket := filepath.Join(dir, "bus.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(host.SocketEnv, socket)
	t.Setenv(host.TokenEnv, "token")
	t.Setenv("OPENCODE_TEST_NATIVE", "1")
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := New(socket, "unused", path)
	var product kit.WorkerCallbacks = p
	if decorate != nil {
		product = decorate(p)
	}
	worker := kit.NewWorker(product)
	p.SetCaller(worker.Caller())
	p.SetShutdown(worker.Shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	served := make(chan error, 1)
	go func() { served <- worker.Serve(ctx) }()
	c, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	f := &workerFixture{p: p, worker: worker, c: c, pending: map[int64]chan protocol.Frame{}, ready: make(chan protocol.Frame, 256), done: make(chan struct{}), ctx: ctx}
	hello := make(chan struct{})
	go func() {
		defer close(f.done)
		r := bufio.NewReader(c)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			frame, err := protocol.DecodeFrame(line[:len(line)-1])
			if err != nil {
				panic(err)
			}
			f.mu.Lock()
			if frame.Method != "" {
				result := map[string]any{}
				if frame.Method == "session.list" {
					result["sessions"] = []any{}
				}
				b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "result": result})
				_, _ = c.Write(append(b, '\n'))
				if frame.Method == "session.hello" {
					close(hello)
				} else if frame.Method == "turn.ready" {
					f.ready <- frame
				}
			} else {
				if ch := f.pending[frame.ID]; ch != nil {
					delete(f.pending, frame.ID)
					ch <- frame
				}
			}
			f.mu.Unlock()
		}
	}()
	t.Cleanup(func() { cancel(); c.Close(); <-served; <-f.done; l.Close() })
	select {
	case <-hello:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var opened kit.OpenResult
	f.call(t, "session.open", kit.OpenRequest{Name: "lane@local", Groups: []string{}, Open: kit.OpenOptions{Cwd: t.TempDir()}}, &opened)
	if opened.SessionID != "ses_native" {
		t.Fatal(opened)
	}
	return f
}
func (f *workerFixture) call(t *testing.T, method string, params any, out any) {
	t.Helper()
	f.mu.Lock()
	f.next++
	id := f.next
	ch := make(chan protocol.Frame, 1)
	f.pending[id] = ch
	b, err := protocol.RequestBytes(id, method, params)
	if err != nil {
		f.mu.Unlock()
		t.Fatal(err)
	}
	_, err = f.c.Write(b)
	f.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-ch:
		if frame.Error != nil {
			t.Fatalf("%s: %+v", method, frame.Error)
		}
		if out != nil {
			if e := json.Unmarshal(frame.Result, out); e != nil {
				t.Fatal(e)
			}
		}
	case <-f.ctx.Done():
		t.Fatalf("%s: %v", method, f.ctx.Err())
	}
}
func (f *workerFixture) start(t *testing.T, seq int, text string) {
	f.call(t, "turn.execute", protocol.ExecuteRequest{SessionID: "ses_native@local", RunID: fmt.Sprintf("g/%d", seq), Input: text}, nil)
}
func (f *workerFixture) wait(t *testing.T, seq int) kit.RunStatus {
	var result kit.RunStatus
	f.call(t, "turn.wait", kit.WaitRequest{SessionID: "ses_native@local", RunID: fmt.Sprintf("g/%d", seq)}, &result)
	return result
}
func fixtureDelivery() kit.DeliveryRequest {
	return kit.DeliveryRequest{MessageID: "delivery", From: kit.DeliverySource{SessionID: "sender@local", Product: "claude", Groups: []string{}}, Body: "marker"}
}
func TestLegacyWorkerOpenStageRunAndNext(t *testing.T) {
	f := newWorkerFixture(t)
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", fixtureDelivery(), &receipt)
	if receipt.Disposition != "queued_for_next_turn" {
		t.Fatal(receipt)
	}
	f.start(t, 1, "explicit")
	result := f.wait(t, 1)
	if result.Result == nil || result.Result.Outcome != "completed" || !strings.Contains(result.Result.Result, "marker") {
		t.Fatalf("result=%+v", result)
	}
	f.start(t, 2, "next")
	next := f.wait(t, 2)
	if next.Result == nil || next.Result.Result != "answer:next" {
		t.Fatalf("next=%+v", next)
	}
	b, e := f.p.client.call(f.ctx, "GET", "/fixture/state", nil, 200)
	if e != nil {
		t.Fatal(e)
	}
	var state struct{ Permission any }
	_ = json.Unmarshal(b, &state)
	if state.Permission != nil {
		t.Fatal("default permission overwritten")
	}
	if err := f.p.ownsSession(f.ctx, "ses_child"); err != nil {
		t.Fatal(err)
	}
	if err := f.p.ownsSession(f.ctx, "ses_other"); err == nil {
		t.Fatal("unrelated task accepted")
	}
}
func TestLegacyWorkerActiveSavedInputInterruptAndNext(t *testing.T) {
	f := newWorkerFixture(t)
	f.start(t, 1, "hold")
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", fixtureDelivery(), &receipt)
	if receipt.Disposition != "written" {
		t.Fatal(receipt)
	}
	f.call(t, "turn.interrupt", map[string]any{"session_id": "ses_native@local"}, nil)
	result := f.wait(t, 1)
	if result.Result == nil || result.Result.Outcome != "interrupted" || result.Result.NativeStopReason != "MessageAbortedError" {
		t.Fatalf("result=%+v", result)
	}
	f.start(t, 2, "healthy")
	next := f.wait(t, 2)
	if next.Result == nil || next.Result.Result != "answer:healthy" {
		t.Fatalf("next=%+v", next)
	}
}

func TestLegacyWorkerSeedWrittenAndRetainedCursor(t *testing.T) {
	f := newWorkerFixture(t)
	d := fixtureDelivery()
	d.RunID = "g/1"
	var receipt kit.DeliveryReceipt
	f.call(t, "message.deliver", d, &receipt)
	if receipt.Disposition != "written" {
		t.Fatal(receipt)
	}
	result := f.wait(t, 1)
	if result.Result == nil || !strings.Contains(result.Result.Result, "marker") {
		t.Fatalf("seed=%+v", result)
	}
	var status kit.RunStatus
	f.call(t, "turn.status", kit.ReadRequest{SessionID: "ses_native@local", RunID: "g/1"}, &status)
	if status.Result == nil || status.Result.Result != result.Result.Result {
		t.Fatal("cursor consumed by wait")
	}
	f.call(t, "turn.ack", kit.RunRef{SessionID: "ses_native@local", RunID: "g/1"}, nil)
	f.start(t, 2, "next")
	if next := f.wait(t, 2); next.Result == nil || next.Result.Result != "answer:next" {
		t.Fatalf("next=%+v", next)
	}
}
func TestLegacyWorkerCancelledSummaryIsNotUserCompletion(t *testing.T) {
	f := newWorkerFixture(t)
	f.start(t, 1, "hold-summary")
	f.call(t, "turn.interrupt", map[string]any{"session_id": "ses_native@local"}, nil)
	r := f.wait(t, 1)
	if r.Result == nil || r.Result.Outcome != "interrupted" || r.Result.NativeStopReason != "" || r.Result.Result != "" {
		t.Fatalf("summary became terminal: %+v", r)
	}
}

type reportHeldProduct struct {
	*Wrapper
	entered, release chan struct{}
}

func (p *reportHeldProduct) Run(ctx context.Context, r *kit.Run, input kit.RunInput) (kit.TurnResult, error) {
	return p.executeRun(ctx, r, input, func(v kit.DeliveryReceipt, e error) error {
		close(p.entered)
		select {
		case <-p.release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return r.ReportDelivery(v, e)
	})
}
func TestLegacyWorkerTerminalWhileSeedReportHeldAndBusLoss(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			held := &reportHeldProduct{entered: make(chan struct{}), release: make(chan struct{})}
			var once sync.Once
			release := func() { once.Do(func() { close(held.release) }) }
			defer release()
			f := newWorkerProductFixture(t, func(p *Wrapper) kit.WorkerCallbacks { held.Wrapper = p; return held })
			d := fixtureDelivery()
			d.RunID = "g/1"
			f.mu.Lock()
			f.next++
			id := f.next
			response := make(chan protocol.Frame, 1)
			f.pending[id] = response
			b, err := protocol.RequestBytes(id, "message.deliver", d)
			if err != nil {
				f.mu.Unlock()
				t.Fatal(err)
			}
			_, err = f.c.Write(b)
			f.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-held.entered:
			case <-f.ctx.Done():
				t.Fatal(f.ctx.Err())
			}
			f.p.mu.Lock()
			run := f.p.active
			f.p.mu.Unlock()
			<-run.original.done
			if run.original.err != nil {
				t.Fatal(run.original.err)
			}
			select {
			case <-run.run.Done():
				t.Fatal("shared Run retired while report owned")
			default:
			}
			if lost {
				f.c.Close()
				select {
				case <-f.worker.Closed():
				case <-f.ctx.Done():
					t.Fatal("bus loss did not join held report")
				}
				return
			}
			release()
			select {
			case r := <-response:
				if r.Error != nil {
					t.Fatal(r.Error)
				}
			case <-f.ctx.Done():
				t.Fatal(f.ctx.Err())
			}
			if r := f.wait(t, 1); r.Result == nil || r.Result.Outcome != "completed" {
				t.Fatalf("lost terminal=%+v", r)
			}
		})
	}
}
func TestLegacyResidentToolUsesNativeChildAncestry(t *testing.T) {
	f := newWorkerFixture(t)
	c, err := net.Dial("unix", f.p.endpoint.Path)
	if err != nil {
		t.Fatal(err)
	}
	// Close the product before this initialized resident: unexpected resident EOF
	// intentionally retires its owner, while test cleanup must remain joined.
	t.Cleanup(func() { f.c.Close(); c.Close() })
	r := bufio.NewReader(c)
	send := func(id int, method string, params any) map[string]json.RawMessage {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if _, e := c.Write(append(b, '\n')); e != nil {
			t.Fatal(e)
		}
		line, e := r.ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		var out map[string]json.RawMessage
		if e = json.Unmarshal(line, &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	send(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "native", "version": "1"}})
	for i, id := range []string{"ses_child", "ses_other", ""} {
		result := send(i+2, "tools/call", map[string]any{"name": "sessionbus", "arguments": map[string]any{"action": "list", "arguments": map[string]any{}}, "_meta": map[string]any{"sessionbus.opencode": map[string]string{"session_id": id, "message_id": "msg_native_tool"}}})
		if i == 0 {
			if strings.Contains(string(result["result"]), `"isError":true`) || len(result["error"]) > 0 {
				t.Fatalf("child rejected: %s", result)
			}
		} else if !strings.Contains(string(result["result"]), `"isError":true`) && len(result["error"]) == 0 {
			t.Fatalf("unrelated/missing identity accepted: %s", result)
		}
	}
}
