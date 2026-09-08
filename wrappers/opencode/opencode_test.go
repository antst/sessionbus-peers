// SPDX-License-Identifier: MIT

package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/antst/sessionbus-peers/wrappers/host"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

type fakeRun struct {
	done, admitted chan struct{}
	once           sync.Once
	interrupted    bool
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func newFakeRun() *fakeRun                       { return &fakeRun{done: make(chan struct{}), admitted: make(chan struct{})} }
func (r *fakeRun) Admitted()                     { r.once.Do(func() { close(r.admitted) }) }
func (r *fakeRun) AdmittedDone() <-chan struct{} { return r.admitted }
func (r *fakeRun) Done() <-chan struct{}         { return r.done }
func (r *fakeRun) Interrupted() bool             { return r.interrupted }

const (
	sealedStopSessionID       = "ses_f88168539ffeSpCdVxN5R9xypN"
	sealedStopAssistantID     = "msg_077e9f21c001oQ5PZ1tRxkTHiI"
	sealedFailedSessionID     = "ses_f83f6ee01ffey9UY55CELb3nvL"
	sealedPermissionSessionID = "ses_f882707e5ffe1otPaUJ5e36u4W"
	sealedStopFrame           = `data: {"id":"evt_077e9fa13001zVSVzkFtsAVFYx","type":"session.next.step.ended","durable":{"aggregateID":"ses_f88168539ffeSpCdVxN5R9xypN","seq":9,"version":2},"data":{"timestamp":1788718217747,"sessionID":"ses_f88168539ffeSpCdVxN5R9xypN","assistantMessageID":"msg_077e9f21c001oQ5PZ1tRxkTHiI","finish":"stop","cost":0,"tokens":{"input":3314,"output":13,"reasoning":147,"cache":{"read":0,"write":0}}}}`
	queueStopFrame            = `data: {"id":"evt_queue_stop","type":"session.next.step.ended","data":{"sessionID":"ses_f88168539ffeSpCdVxN5R9xypN","assistantMessageID":"msg_queue_answer","finish":"stop"}}`
	nonStopFrame              = `data: {"id":"evt_non_stop","type":"session.next.step.ended","data":{"sessionID":"ses_f88168539ffeSpCdVxN5R9xypN","assistantMessageID":"msg_tool","finish":"tool-calls"}}`
	sealedStepFailedFrame     = `data: {"id":"evt_07c091a18001vzHIrL1AjfwJCd","type":"session.next.step.failed","durable":{"aggregateID":"ses_f83f6ee01ffey9UY55CELb3nvL","seq":8,"version":2},"data":{"timestamp":1788787366424,"sessionID":"ses_f83f6ee01ffey9UY55CELb3nvL","assistantMessageID":"msg_07c0919ed001jZalRhLfnKaGQM","error":{"type":"unknown","message":"Provider turn interrupted"}}}`
	sealedPermissionFrame     = `data: {"id":"evt_077d90743002TZvkrIhKwAarzm","type":"permission.asked","properties":{"id":"per_077d90743001KPEP6Whcwh0xQM","sessionID":"ses_f882707e5ffe1otPaUJ5e36u4W","permission":"bash","patterns":["printf OC_PERM"],"metadata":{"command":"printf OC_PERM"},"always":["printf *"],"tool":{"messageID":"msg_077d8f8ae0011mriC0AMY3OXq1","callID":"call_9b1a0b3842114b7dba535250"}}}`
)

func durableSubscriberEvent(t *testing.T, frame string) nativeEvent {
	return durableSubscriberEventFor(t, sealedStopSessionID, frame)
}

func durableSubscriberEventFor(t *testing.T, sessionID, frame string) nativeEvent {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/session/"+sessionID+"/event" || request.URL.RawQuery != "directory=%2Fwork" || request.Header.Get("x-opencode-directory") != "/work" {
			t.Fatalf("subscriber request = %s?%s, directory header = %q", request.URL.Path, request.URL.RawQuery, request.Header.Get("x-opencode-directory"))
		}
		_, _ = response.Write([]byte(frame + "\n\n"))
	}))
	defer server.Close()
	observed := make(chan nativeEvent, 1)
	done, err := testClient(server).subscribeSession(context.Background(), sessionID, func(_ context.Context, event nativeEvent) error {
		observed <- event
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var event nativeEvent
	select {
	case event = <-observed:
	case err = <-done:
		t.Fatalf("event was not observed: %v", err)
	}
	if err = <-done; err == nil || err.Error() != "OpenCode event stream ended" {
		t.Fatalf("stream end = %v", err)
	}
	return event
}

func TestLegacySubscriberIgnoresCapturedDurableStep(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/event" || request.URL.RawQuery != "directory=%2Fwork" || request.Header.Get("x-opencode-directory") != "/work" {
			t.Fatalf("subscriber request = %s?%s, directory header = %q", request.URL.Path, request.URL.RawQuery, request.Header.Get("x-opencode-directory"))
		}
		_, _ = response.Write([]byte(sealedStopFrame + "\n\n"))
	}))
	defer server.Close()
	client := testClient(server)
	signals := &nativeRunSignals{terminal: make(chan nativeTerminal, 1)}
	p := &Wrapper{id: sealedStopSessionID, runSignals: signals}
	done, err := client.subscribe(context.Background(), func(ctx context.Context, event nativeEvent) error {
		return p.observe(ctx, client, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil || err.Error() != "OpenCode event stream ended" {
		t.Fatalf("stream end = %v", err)
	}
	select {
	case terminal := <-signals.terminal:
		t.Fatalf("legacy stream accepted durable terminal %#v", terminal)
	default:
	}
}

type workerProduct struct{ *Wrapper }

func (*workerProduct) Open(context.Context, sessionkit.OpenRequest) (sessionkit.OpenResult, error) {
	return sessionkit.OpenResult{SessionID: "ses_exact"}, nil
}
func (*workerProduct) Close(context.Context, sessionkit.SessionCloseRequest) error { return nil }

func startWorker(t *testing.T, wrapper *Wrapper) (net.Conn, *bufio.Reader, *sessionkit.Worker) {
	path := filepath.Join(t.TempDir(), "sessionbus.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(host.SocketEnv, path)
	t.Setenv(host.TokenEnv, "token")
	worker := sessionkit.NewWorker(&workerProduct{Wrapper: wrapper})
	go func() { _ = worker.Serve(context.Background()) }()
	connection, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	if _, err = reader.ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Write([]byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n")); err != nil {
		t.Fatal(err)
	}
	writeWorkerRequest(t, connection, 1, "session.open", map[string]any{"name": "lane@local", "groups": []string{}, "open": map[string]any{}})
	readWorkerResponse(t, reader, 1)
	t.Cleanup(func() {
		_ = connection.Close()
		<-worker.Closed()
		_ = listener.Close()
	})
	return connection, reader, worker
}

func writeWorkerRequest(t *testing.T, connection net.Conn, id int, method string, params any) {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Write(append(body, '\n')); err != nil {
		t.Fatal(err)
	}
}

func readWorkerResponse(t *testing.T, reader *bufio.Reader, id int) json.RawMessage {
	t.Helper()
	body, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err = json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != id || len(response.Error) != 0 {
		t.Fatalf("response %d = %s", id, body)
	}
	return response.Result
}

func testClient(server *httptest.Server) *nativeClient {
	return &nativeClient{endpoint: server.URL, username: "user", password: "pass", directory: "/work", http: server.Client()}
}

func TestHelloDescribesOnlyHonouredOpenSurface(t *testing.T) {
	hello, err := (&Wrapper{}).Hello(context.Background())
	if err != nil || hello.Product != Product || hello.Version != "1.18.29" || !slices.Equal(hello.SupportedOpenFields, []string{"cwd", "permission_mode", "model", "arguments"}) {
		t.Fatalf("hello = %#v/%v", hello, err)
	}
	if names := []string{hello.ExtraArguments[0].Name, hello.ExtraArguments[1].Name, hello.ExtraArguments[2].Name, hello.ExtraArguments[3].Name, hello.ExtraArguments[4].Name, hello.ExtraArguments[5].Name}; !slices.Equal(names, []string{"--agent", "--print-logs", "--log-level", "--mdns", "--mdns-domain", "--cors"}) {
		t.Fatalf("extra arguments = %#v", hello.ExtraArguments)
	}
}

func TestReadinessWaitsForExactPluginTool(t *testing.T) {
	tools := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if username, password, ok := request.BasicAuth(); !ok || username != "user" || password != "pass" {
			t.Fatalf("auth = %q/%q/%v", username, password, ok)
		}
		if directory := request.Header.Get("x-opencode-directory"); directory != "/work" {
			t.Fatalf("directory header = %q", directory)
		}
		if request.URL.Path == "/doc" {
			paths := map[string]any{}
			for _, path := range []string{"/session", "/session/{sessionID}", "/event", "/api/session/{sessionID}/event", "/api/session/{sessionID}/prompt", "/api/session/{sessionID}/message", "/api/session/{sessionID}/interrupt", "/api/session/{sessionID}/model", "/api/session/{sessionID}/agent", "/experimental/tool/ids"} {
				paths[path] = map[string]any{}
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"paths": paths})
			return
		}
		tools++
		if tools == 1 {
			_, _ = response.Write([]byte(`[]`))
		} else {
			_, _ = response.Write([]byte(`["sessionbus"]`))
		}
	}))
	defer server.Close()
	client := testClient(server)
	ready, err := client.ready(context.Background())
	if err != nil || ready {
		t.Fatalf("first ready = %v/%v", ready, err)
	}
	ready, err = client.ready(context.Background())
	if err != nil || !ready {
		t.Fatalf("second ready = %v/%v", ready, err)
	}
}

func TestReadinessRejectsNullToolInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/doc" {
			paths := map[string]any{}
			for _, path := range []string{"/session", "/session/{sessionID}", "/event", "/api/session/{sessionID}/event", "/api/session/{sessionID}/prompt", "/api/session/{sessionID}/message", "/api/session/{sessionID}/interrupt", "/api/session/{sessionID}/model", "/api/session/{sessionID}/agent", "/experimental/tool/ids"} {
				paths[path] = map[string]any{}
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"paths": paths})
			return
		}
		_, _ = response.Write([]byte("null"))
	}))
	defer server.Close()
	if _, err := testClient(server).ready(context.Background()); err == nil || err.Error() != "OpenCode tool inventory is malformed" {
		t.Fatalf("error = %v", err)
	}
}

func TestNativeRepliesMustIdentifyCommittedState(t *testing.T) {
	rows := []struct {
		name, response, want string
		call                 func(*nativeClient) error
	}{
		{"create cwd", `{"id":"ses_exact","title":"title","directory":"/else"}`, "ambiguous session", func(client *nativeClient) error {
			_, err := client.create(context.Background(), "title", "ask")
			return err
		}},
		{"resume id", `{"id":"ses_other","title":"title","directory":"/work"}`, "different session", func(client *nativeClient) error { _, err := client.get(context.Background(), "ses_exact"); return err }},
		{"resume settings", `{"id":"ses_exact","title":"other","directory":"/work"}`, "confirm resumed session settings", func(client *nativeClient) error {
			_, err := client.update(context.Background(), "ses_exact", "title", "ask")
			return err
		}},
		{"prompt echo", `{"data":{"admittedSeq":0,"id":"msg_wrong","sessionID":"ses_exact","prompt":{"text":"go"},"delivery":"steer"}}`, "invalid input admission", func(client *nativeClient) error {
			_, err := client.prompt(context.Background(), "ses_exact", "msg_exact", "go", "steer", true)
			return err
		}},
		{"prompt session", `{"data":{"admittedSeq":0,"id":"msg_exact","sessionID":"ses_other","prompt":{"text":"go"},"delivery":"steer"}}`, "invalid input admission", func(client *nativeClient) error {
			_, err := client.prompt(context.Background(), "ses_exact", "msg_exact", "go", "steer", true)
			return err
		}},
		{"prompt text", `{"data":{"admittedSeq":0,"id":"msg_exact","sessionID":"ses_exact","prompt":{"text":"else"},"delivery":"steer"}}`, "invalid input admission", func(client *nativeClient) error {
			_, err := client.prompt(context.Background(), "ses_exact", "msg_exact", "go", "steer", true)
			return err
		}},
		{"prompt delivery", `{"data":{"admittedSeq":0,"id":"msg_exact","sessionID":"ses_exact","prompt":{"text":"go"},"delivery":"queue"}}`, "invalid input admission", func(client *nativeClient) error {
			_, err := client.prompt(context.Background(), "ses_exact", "msg_exact", "go", "steer", true)
			return err
		}},
		{"prompt negative sequence", `{"data":{"admittedSeq":-1,"id":"msg_exact","sessionID":"ses_exact","prompt":{"text":"go"},"delivery":"steer"}}`, "invalid input admission", func(client *nativeClient) error {
			_, err := client.prompt(context.Background(), "ses_exact", "msg_exact", "go", "steer", true)
			return err
		}},
		{"prompt sequence absent", `{"data":{"id":"msg_exact","sessionID":"ses_exact","prompt":{"text":"go"},"delivery":"steer"}}`, "invalid input admission", func(client *nativeClient) error {
			_, err := client.prompt(context.Background(), "ses_exact", "msg_exact", "go", "steer", true)
			return err
		}},
		{"delete false", `false`, "did not confirm session deletion", func(client *nativeClient) error { return client.remove(context.Background(), "ses_exact") }},
		{"permission false", `false`, "did not confirm permission rejection", func(client *nativeClient) error {
			return client.rejectPermission(context.Background(), "ses_exact", "per_exact")
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(row.response)) }))
			defer server.Close()
			if err := row.call(testClient(server)); err == nil || !strings.Contains(err.Error(), row.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestNativeResponseBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(strings.Repeat("x", nativeLimit+1)))
	}))
	defer server.Close()
	if _, err := testClient(server).get(context.Background(), "ses_exact"); err == nil || err.Error() != "OpenCode response exceeds 1 MiB" {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateUsesExactPermissionRule(t *testing.T) {
	for _, action := range []string{"ask", "allow"} {
		t.Run(action, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				var body struct {
					Title      string              `json:"title"`
					Permission []map[string]string `json:"permission"`
				}
				if json.NewDecoder(request.Body).Decode(&body) != nil || body.Title != "title" || len(body.Permission) != 1 || body.Permission[0]["permission"] != "*" || body.Permission[0]["pattern"] != "*" || body.Permission[0]["action"] != action {
					t.Fatalf("body = %#v", body)
				}
				_ = json.NewEncoder(response).Encode(nativeSession{ID: "ses_exact", Title: "title", Directory: "/work"})
			}))
			defer server.Close()
			if _, err := testClient(server).create(context.Background(), "title", action); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResumeReadsThenAppliesAndVerifiesExactSettings(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		if request.URL.Path != "/session/ses_exact" || request.URL.Query().Get("directory") != "/work" {
			t.Fatalf("request = %s", request.URL)
		}
		if request.Method == http.MethodPatch {
			var body struct {
				Title      string              `json:"title"`
				Permission []map[string]string `json:"permission"`
			}
			if json.NewDecoder(request.Body).Decode(&body) != nil || body.Title != "Resumed title" || len(body.Permission) != 1 || body.Permission[0]["permission"] != "*" || body.Permission[0]["pattern"] != "*" || body.Permission[0]["action"] != "allow" {
				t.Fatalf("patch body = %#v", body)
			}
		}
		_ = json.NewEncoder(response).Encode(nativeSession{ID: "ses_exact", Title: "Resumed title", Directory: "/work"})
	}))
	defer server.Close()
	session, err := testClient(server).resume(context.Background(), "ses_exact", "Resumed title", "allow")
	if err != nil || session.ID != "ses_exact" || session.Title != "Resumed title" || !slices.Equal(methods, []string{http.MethodGet, http.MethodPatch}) {
		t.Fatalf("resume = %#v/%v, methods = %#v", session, err, methods)
	}
}

func TestConfigureUsesCapturedV2Shapes(t *testing.T) {
	requests := make(chan struct {
		path string
		body string
	}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal("malformed request body")
		}
		requests <- struct {
			path string
			body string
		}{request.URL.Path, string(body)}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := testClient(server).configure(context.Background(), "ses_exact", &modelRef{ProviderID: "openai", ID: "gpt"}, "build"); err != nil {
		t.Fatal(err)
	}
	model, agent := <-requests, <-requests
	if model.path != "/api/session/ses_exact/model" || model.body != `{"model":{"id":"gpt","providerID":"openai"}}` {
		t.Fatalf("model = %#v", model)
	}
	if agent.path != "/api/session/ses_exact/agent" || agent.body != `{"agent":"build"}` {
		t.Fatalf("agent = %#v", agent)
	}
}

func TestOpenBarrierWaitsForPluginAndReportsExit(t *testing.T) {
	previous, previousWait := retryReady, bootstrapWait
	retryReady = func(context.Context) error { return nil }
	bootstrapWait = time.Millisecond
	defer func() { retryReady, bootstrapWait = previous, previousWait }()
	attempts, readyCalls := 0, 0
	probeDone := make(chan struct{})
	err := waitReady(context.Background(), make(chan struct{}), func(ctx context.Context) error {
		attempts++
		if attempts == 1 {
			return &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		}
		<-ctx.Done()
		close(probeDone)
		return ctx.Err()
	}, func(context.Context) (bool, error) {
		readyCalls++
		return readyCalls == 2, nil
	})
	if err != nil || attempts != 2 || readyCalls != 2 {
		t.Fatalf("wait = %v after %d bootstrap/%d ready attempts", err, attempts, readyCalls)
	}
	<-probeDone
	exited := make(chan struct{})
	entered, stopped, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		result <- waitReady(context.Background(), exited, func(ctx context.Context) error {
			close(entered)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		}, func(context.Context) (bool, error) { return true, nil })
	}()
	<-entered
	close(exited)
	err = <-result
	if err == nil || err.Error() != "OpenCode exited before plugin readiness" {
		t.Fatalf("exit error = %v", err)
	}
	<-stopped
	bootstrapWait = time.Hour
	cancelled, cancel := context.WithCancel(context.Background())
	entered, stopped = make(chan struct{}), make(chan struct{})
	go func() {
		result <- waitReady(cancelled, make(chan struct{}), func(ctx context.Context) error {
			close(entered)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		}, func(context.Context) (bool, error) { return true, nil })
	}()
	<-entered
	cancel()
	if err = <-result; err != context.Canceled {
		t.Fatalf("cancel error = %v", err)
	}
	<-stopped
}

func TestWrapperOpenOwnsServerPluginAndSessionLifecycle(t *testing.T) {
	directory, record := t.TempDir(), filepath.Join(t.TempDir(), "requests.jsonl")
	socket := filepath.Join(directory, "sessionbus.sock")
	for _, name := range []string{host.SocketEnv, host.LocalKeyEnv, host.TokenEnv, host.SessionIDEnv, host.NameEnv, host.GroupsEnv} {
		t.Setenv(name, "must-not-reach-child")
	}
	p := New(socket, "provisional", fakeOpenCode(t, "", record))
	p.SetCall(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	opened, err := p.Open(context.Background(), sessionkit.OpenRequest{Name: "Lane title@local", Open: sessionkit.OpenOptions{Cwd: directory, PermissionMode: "bypassPermissions", Model: "openai/gpt", Arguments: []string{"--agent", "build", "--log-level", "INFO", "--mdns"}}})
	if err != nil || opened.SessionID != "ses_exact" {
		t.Fatalf("open = %#v/%v", opened, err)
	}
	if err = p.Close(context.Background(), sessionkit.SessionCloseRequest{Forget: true}); err != nil {
		t.Fatal(err)
	}
	rows := readProcessRows(t, record)
	start := rows[0]
	if fmt.Sprint(start["args"]) != "[serve --hostname 127.0.0.1 --port "+fmt.Sprint(start["port"])+" --log-level INFO --mdns]" || start["lane_socket"] != filepath.Join(directory, "lanes", "provisional.sock") || start["auth_complete"] != true {
		t.Fatalf("start = %#v", start)
	}
	for _, name := range []string{host.SocketEnv, host.LocalKeyEnv, host.TokenEnv, host.SessionIDEnv, host.NameEnv, host.GroupsEnv} {
		if slices.ContainsFunc(start["env_names"].([]any), func(value any) bool { return value == name }) {
			t.Fatalf("%s reached child: %#v", name, start)
		}
	}
	joined := fmt.Sprint(rows[1:])
	for _, fragment := range []string{"GET /doc", "GET /experimental/tool/ids?directory=", "GET /event?directory=", "GET /api/session/ses_exact/event?directory=", "POST /session?directory=", `permission:[map[action:allow pattern:* permission:*]]`, `id:gpt`, `providerID:openai`, `agent:build`, "DELETE /session/ses_exact?directory="} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("request log omitted %q: %s", fragment, joined)
		}
	}
	if _, err = os.Stat(filepath.Join(directory, "locks", "opencode", "ses_exact")); err != nil {
		t.Fatalf("renamed lock: %v", err)
	}
	if _, err = os.Stat(filepath.Join(directory, "lanes", "provisional.sock")); !os.IsNotExist(err) {
		t.Fatalf("lane socket remains: %v", err)
	}
}

func TestWrapperNeverResolvesExecutableFromPath(t *testing.T) {
	directory, marker := t.TempDir(), filepath.Join(t.TempDir(), "resolved-from-path")
	poison := filepath.Join(directory, "opencode")
	if err := os.WriteFile(poison, []byte("#!/bin/sh\ntouch \"$OPENCODE_PATH_MARKER\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("OPENCODE_PATH_MARKER", marker)
	p := New(filepath.Join(directory, "sessionbus.sock"), "provisional", "opencode")
	if _, err := p.Open(context.Background(), sessionkit.OpenRequest{Name: "Lane title@local"}); err == nil || err.Error() != "OpenCode executable path must be absolute" {
		t.Fatalf("open error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("PATH product was executed: %v", err)
	}
}

func TestOpenFailureDeletesOnlyFreshNativeSession(t *testing.T) {
	for _, row := range []struct {
		name       string
		resume     bool
		cancel     bool
		wantDelete bool
	}{{"fresh HTTP failure", false, false, true}, {"resume HTTP failure", true, false, false}, {"fresh caller cancellation", false, true, true}, {"resume caller cancellation", true, true, false}} {
		t.Run(row.name, func(t *testing.T) {
			directory, record := t.TempDir(), filepath.Join(t.TempDir(), "requests.jsonl")
			mode := "model-error"
			if row.cancel {
				mode = "model-block"
			}
			p := New(filepath.Join(directory, "sessionbus.sock"), "provisional", fakeOpenCode(t, mode, record))
			p.SetCall(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
			request := sessionkit.OpenRequest{Name: "Lane title@local", Open: sessionkit.OpenOptions{Cwd: directory, Model: "openai/gpt"}}
			if row.resume {
				request.ResumeSessionID = "ses_exact"
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			result := make(chan error, 1)
			go func() { _, err := p.Open(ctx, request); result <- err }()
			if row.cancel {
				started := false
				for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
					recorded, _ := os.ReadFile(record)
					if strings.Contains(string(recorded), "POST /api/session/ses_exact/model") {
						started = true
						break
					}
				}
				cancel()
				if !started {
					if err := <-result; err != nil {
						t.Fatalf("configure did not start; open ended with %v", err)
					}
					t.Fatal("configure did not start")
				}
			}
			err := <-result
			cancel()
			if row.cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("open cancellation = %v", err)
			}
			if !row.cancel && (err == nil || !strings.Contains(err.Error(), "model returned HTTP 400")) {
				t.Fatalf("open error = %v", err)
			}
			deleted := strings.Contains(fmt.Sprint(readProcessRows(t, record)), "DELETE /session/ses_exact?directory=")
			if deleted != row.wantDelete {
				t.Fatalf("deleted = %v, want %v", deleted, row.wantDelete)
			}
			assertProcessGone(t, int(readProcessRows(t, record)[0]["pid"].(float64)))
		})
	}
}

func TestOpenCodeProcess(t *testing.T) {
	if os.Getenv("OPENCODE_TEST_CHILD") != "1" {
		return
	}
	separator := slices.Index(os.Args, "--")
	arguments := os.Args[separator+1:]
	port := ""
	for index, argument := range arguments {
		if argument == "--port" && index+1 < len(arguments) {
			port = arguments[index+1]
		}
	}
	names := []string{}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		names = append(names, name)
	}
	record := os.Getenv("OPENCODE_TEST_RECORD")
	mode := os.Getenv("OPENCODE_TEST_MODE")
	var mu sync.Mutex
	promptID := ""
	durableLines := make(chan string, 4)
	write := func(value any) {
		mu.Lock()
		defer mu.Unlock()
		file, err := os.OpenFile(record, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(2)
		}
		_ = json.NewEncoder(file).Encode(value)
		_ = file.Close()
	}
	write(map[string]any{"pid": os.Getpid(), "args": arguments, "port": port, "env_names": names, "lane_socket": os.Getenv(LaneSocketEnv), "auth_complete": os.Getenv("OPENCODE_SERVER_USERNAME") == "sessionbus" && os.Getenv("OPENCODE_SERVER_PASSWORD") != ""})
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body any
		if request.Body != nil {
			_ = json.NewDecoder(request.Body).Decode(&body)
		}
		write(map[string]any{"request": request.Method + " " + request.URL.String(), "body": body})
		username, password, ok := request.BasicAuth()
		if !ok || username != "sessionbus" || password == "" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case request.URL.Path == "/doc":
			paths := map[string]any{}
			for _, path := range []string{"/session", "/session/{sessionID}", "/event", "/api/session/{sessionID}/event", "/api/session/{sessionID}/prompt", "/api/session/{sessionID}/message", "/api/session/{sessionID}/interrupt", "/api/session/{sessionID}/model", "/api/session/{sessionID}/agent", "/experimental/tool/ids"} {
				paths[path] = map[string]any{}
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"paths": paths})
		case request.URL.Path == "/experimental/tool/ids":
			_ = json.NewEncoder(response).Encode([]string{ToolName})
		case request.URL.Path == "/event":
			response.Header().Set("Content-Type", "text/event-stream")
			response.WriteHeader(http.StatusOK)
			_, _ = response.Write([]byte("data: {\"type\":\"server.connected\",\"properties\":{}}\n\n"))
			response.(http.Flusher).Flush()
			<-request.Context().Done()
		case request.URL.Path == "/api/session/ses_exact/event":
			response.Header().Set("Content-Type", "text/event-stream")
			response.WriteHeader(http.StatusOK)
			response.(http.Flusher).Flush()
			for {
				select {
				case line := <-durableLines:
					if line == "exit" {
						os.Exit(0)
					}
					_, _ = response.Write([]byte("data: " + line + "\n\n"))
					response.(http.Flusher).Flush()
				case <-request.Context().Done():
					return
				}
			}
		case request.Method == http.MethodPost && request.URL.Path == "/session":
			_ = json.NewEncoder(response).Encode(nativeSession{ID: "ses_exact", Title: "Lane title", Directory: request.URL.Query().Get("directory")})
		case request.Method == http.MethodGet && request.URL.Path == "/session/ses_exact":
			_ = json.NewEncoder(response).Encode(nativeSession{ID: "ses_exact", Title: "Previous title", Directory: request.URL.Query().Get("directory")})
		case request.Method == http.MethodPatch && request.URL.Path == "/session/ses_exact":
			_ = json.NewEncoder(response).Encode(nativeSession{ID: "ses_exact", Title: "Lane title", Directory: request.URL.Query().Get("directory")})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/model") && mode == "model-error":
			response.WriteHeader(http.StatusBadRequest)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/model") && mode == "model-block":
			<-request.Context().Done()
		case request.Method == http.MethodPost && (strings.HasSuffix(request.URL.Path, "/model") || strings.HasSuffix(request.URL.Path, "/agent")):
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/prompt"):
			input := body.(map[string]any)
			mu.Lock()
			promptID, _ = input["id"].(string)
			mu.Unlock()
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": input["id"], "sessionID": "ses_exact", "prompt": input["prompt"], "delivery": input["delivery"]}})
			response.(http.Flusher).Flush()
			if mode == "terminal-exit" {
				durableLines <- `{"type":"session.next.step.ended","data":{"sessionID":"ses_exact","assistantMessageID":"msg_answer","finish":"stop"}}`
			} else if mode == "no-terminal-exit" {
				durableLines <- "exit"
			}
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message") && mode == "terminal-exit":
			mu.Lock()
			messageID := promptID
			mu.Unlock()
			encoded, _ := json.Marshal(map[string]any{"data": []any{
				map[string]any{"id": messageID, "type": "user", "text": "large", "time": map[string]any{"created": 1}},
				map[string]any{"id": "msg_answer", "type": "assistant", "time": map[string]any{"created": 2, "completed": 3}, "content": []any{map[string]any{"type": "text", "text": strings.Repeat("x", 300004)}}, "finish": "stop"},
			}, "cursor": map[string]string{}})
			response.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
			_, _ = response.Write(encoded)
			response.(http.Flusher).Flush()
			os.Exit(0)
		case request.Method == http.MethodDelete && request.URL.Path == "/session/ses_exact":
			_ = json.NewEncoder(response).Encode(true)
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	})
	if err := http.ListenAndServe("127.0.0.1:"+port, handler); err != nil {
		os.Exit(3)
	}
}

func readProcessRows(t *testing.T, path string) []map[string]any {
	t.Helper()
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(string(encoded)), "\n") {
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			t.Fatalf("process row = %q", line)
		}
		rows = append(rows, row)
	}
	return rows
}

func fakeOpenCode(t *testing.T, mode, record string) string {
	t.Helper()
	t.Setenv("OPENCODE_TEST_CHILD", "1")
	t.Setenv("OPENCODE_TEST_MODE", mode)
	t.Setenv("OPENCODE_TEST_RECORD", record)
	t.Setenv("OPENCODE_TEST_BINARY", os.Args[0])
	path := filepath.Join(t.TempDir(), "opencode-stub")
	script := "#!/bin/sh\nexec \"$OPENCODE_TEST_BINARY\" -test.run '^TestOpenCodeProcess$' -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertProcessGone(t *testing.T, pid int) {
	t.Helper()
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("stub process %d remains: %v", pid, err)
	}
}

func TestRunAdmissionPrecedesActiveDelivery(t *testing.T) {
	baseSeen, releaseBase := make(chan struct{}), make(chan struct{})
	stopEvent := durableSubscriberEvent(t, sealedStopFrame)
	var mu sync.Mutex
	order := []string{}
	baseID := ""
	var p *Wrapper
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/prompt"):
			var body struct {
				ID     string `json:"id"`
				Prompt struct {
					Text string `json:"text"`
				} `json:"prompt"`
				Delivery string `json:"delivery"`
				Resume   bool   `json:"resume"`
			}
			_ = json.NewDecoder(request.Body).Decode(&body)
			mu.Lock()
			order = append(order, body.Delivery)
			if len(order) == 1 {
				baseID = body.ID
			}
			position := len(order)
			mu.Unlock()
			if position == 1 {
				close(baseSeen)
				<-releaseBase
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": position - 1, "id": body.ID, "sessionID": sealedStopSessionID, "prompt": map[string]string{"text": body.Prompt.Text}, "delivery": body.Delivery, "timeCreated": 1}})
			if position == 2 {
				p.observeTerminal(stopEvent)
			}
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
			mu.Lock()
			id := baseID
			mu.Unlock()
			_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{map[string]any{"id": id, "type": "user", "text": "base", "time": map[string]any{"created": 1}}, map[string]any{"id": sealedStopAssistantID, "type": "assistant", "time": map[string]any{"created": 2, "completed": 3}, "content": []any{map[string]any{"type": "text", "text": "done"}}, "finish": "stop"}}, "cursor": map[string]string{}})
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	p = &Wrapper{opened: true, id: sealedStopSessionID, client: testClient(server)}
	run := newFakeRun()
	runResult := make(chan error, 1)
	go func() {
		result, err := p.run(context.Background(), run, "base")
		if err == nil && (result.Result != "done" || result.NativeStopReason != "stop") {
			err = fmt.Errorf("result = %#v", result)
		}
		runResult <- err
	}()
	<-baseSeen
	delivered := make(chan sessionkit.DeliveryReceipt, 1)
	deliveryErr := make(chan error, 1)
	go func() {
		receipt, err := p.deliver(context.Background(), delivery(), run)
		delivered <- receipt
		deliveryErr <- err
	}()
	select {
	case <-delivered:
		t.Fatal("delivery passed native run admission")
	default:
	}
	close(releaseBase)
	if err := <-runResult; err != nil {
		t.Fatal(err)
	}
	receipt := <-delivered
	if err := <-deliveryErr; err != nil || receipt.Disposition != "injected" {
		t.Fatalf("delivery = %#v/%v", receipt, err)
	}
	mu.Lock()
	got := slices.Clone(order)
	mu.Unlock()
	if !slices.Equal(got, []string{"steer", "steer"}) {
		t.Fatalf("native order = %#v", got)
	}
}

func TestRunIgnoresTerminalBeforeAdmittedInput(t *testing.T) {
	var p *Wrapper
	stopEvent := durableSubscriberEvent(t, sealedStopFrame)
	history := 0
	messageID := ""
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/prompt"):
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			messageID, _ = body["id"].(string)
			p.observeTerminal(stopEvent)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": body["id"], "sessionID": sealedStopSessionID, "prompt": body["prompt"], "delivery": "steer"}})
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
			history++
			if history == 1 {
				_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{}, "cursor": map[string]string{}})
				p.observeTerminal(stopEvent)
				return
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{
				map[string]any{"id": messageID, "type": "user"},
				map[string]any{"id": sealedStopAssistantID, "type": "assistant", "time": map[string]any{"completed": 2}, "content": []any{map[string]any{"type": "text", "text": "done"}}, "finish": "stop"},
			}, "cursor": map[string]string{}})
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	p = &Wrapper{opened: true, id: sealedStopSessionID, client: testClient(server)}
	result, err := p.run(context.Background(), newFakeRun(), "go")
	if err != nil || result.Result != "done" || history != 2 {
		t.Fatalf("result = %#v/%v after %d histories", result, err, history)
	}
}

func TestRunWaitsForStopAfterNonStopStep(t *testing.T) {
	var p *Wrapper
	nonStop, stop := durableSubscriberEvent(t, nonStopFrame), durableSubscriberEvent(t, sealedStopFrame)
	messageID := ""
	var history atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/prompt"):
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			messageID, _ = body["id"].(string)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": messageID, "sessionID": sealedStopSessionID, "prompt": body["prompt"], "delivery": "steer"}})
			p.observeTerminal(nonStop)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
			history.Add(1)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{
				map[string]any{"id": messageID, "type": "user"},
				map[string]any{"id": sealedStopAssistantID, "type": "assistant", "time": map[string]any{"completed": 2}, "content": []any{map[string]any{"type": "text", "text": "done"}}, "finish": "stop"},
			}, "cursor": map[string]string{}})
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	p = &Wrapper{opened: true, id: sealedStopSessionID, client: testClient(server)}
	run := newFakeRun()
	done := make(chan struct {
		result sessionkit.TurnResult
		err    error
	}, 1)
	go func() {
		result, err := p.run(context.Background(), run, "go")
		done <- struct {
			result sessionkit.TurnResult
			err    error
		}{result, err}
	}()
	<-run.AdmittedDone()
	if history.Load() != 0 {
		t.Fatalf("non-stop step read history %d times", history.Load())
	}
	p.observeTerminal(stop)
	terminal := <-done
	if terminal.err != nil || terminal.result.Result != "done" || history.Load() != 1 {
		t.Fatalf("result = %#v/%v after %d history reads", terminal.result, terminal.err, history.Load())
	}
}

func TestRunReconcilesSealedStepFailure(t *testing.T) {
	for _, row := range []struct {
		name, answer, want string
	}{{"native failure without action", "", `OpenCode step failed: {"type":"unknown","message":"Provider turn interrupted"}`}, {"completed stop", "done", ""}} {
		t.Run(row.name, func(t *testing.T) {
			var p *Wrapper
			failure := durableSubscriberEventFor(t, sealedFailedSessionID, sealedStepFailedFrame)
			messageID := ""
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				switch {
				case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/prompt"):
					var body map[string]any
					_ = json.NewDecoder(request.Body).Decode(&body)
					messageID, _ = body["id"].(string)
					_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": messageID, "sessionID": sealedFailedSessionID, "prompt": body["prompt"], "delivery": "steer"}})
					p.observeTerminal(failure)
				case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
					history := []any{map[string]any{"id": messageID, "type": "user"}}
					if row.answer != "" {
						history = append(history, map[string]any{"id": "msg_answer", "type": "assistant", "time": map[string]any{"completed": 2}, "content": []any{map[string]any{"type": "text", "text": row.answer}}, "finish": "stop"})
					}
					_ = json.NewEncoder(response).Encode(map[string]any{"data": history, "cursor": map[string]string{}})
				default:
					t.Fatalf("unexpected request %s %s", request.Method, request.URL)
				}
			}))
			defer server.Close()
			p = &Wrapper{opened: true, id: sealedFailedSessionID, client: testClient(server)}
			result, err := p.run(context.Background(), newFakeRun(), "go")
			if row.want == "" && (err != nil || result.Result != row.answer || result.NativeStopReason != "stop") {
				t.Fatalf("result = %#v/%v", result, err)
			}
			if row.want != "" && (err == nil || err.Error() != row.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestInterruptActionWinsLateNativeFailure(t *testing.T) {
	failure := durableSubscriberEventFor(t, sealedFailedSessionID, sealedStepFailedFrame)
	var historyReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/interrupt"):
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
			historyReads.Add(1)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{
				map[string]any{"id": "msg_input", "type": "user"},
				map[string]any{"id": "msg_failed", "type": "assistant", "time": map[string]any{"completed": 2}, "finish": "error", "error": map[string]any{"type": "unknown", "message": "Provider turn interrupted"}},
			}, "cursor": map[string]string{}})
		default:
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	p := &Wrapper{opened: true, id: sealedFailedSessionID, client: testClient(server)}
	run := newFakeRun()
	client, id, signals, err := p.beginRun(context.Background(), run.Done())
	if err != nil {
		t.Fatal(err)
	}
	run.Admitted()
	if err = p.interrupt(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	p.observeTerminal(failure)
	_, _, err = waitForNativeTerminal(context.Background(), client, id, "msg_input", signals)
	if err == nil || err.Error() != "OpenCode run ended by interrupt" || historyReads.Load() != 1 {
		t.Fatalf("terminal = %v after %d history reads", err, historyReads.Load())
	}
}

func TestRunIgnoresQueueDeliveredStopInsideRunWindow(t *testing.T) {
	var p *Wrapper
	queueStop, runStop := durableSubscriberEvent(t, queueStopFrame), durableSubscriberEvent(t, sealedStopFrame)
	messageID := ""
	historyReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/prompt"):
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			messageID, _ = body["id"].(string)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": messageID, "sessionID": sealedStopSessionID, "prompt": body["prompt"], "delivery": "steer"}})
			p.observeTerminal(queueStop)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
			historyReads++
			history := []any{
				map[string]any{"id": "msg_queue", "type": "user"},
				map[string]any{"id": "msg_queue_answer", "type": "assistant", "time": map[string]any{"completed": 1}, "finish": "stop"},
				map[string]any{"id": messageID, "type": "user"},
			}
			if historyReads == 1 {
				_ = json.NewEncoder(response).Encode(map[string]any{"data": history, "cursor": map[string]string{}})
				p.observeTerminal(runStop)
				return
			}
			history = append(history, map[string]any{"id": sealedStopAssistantID, "type": "assistant", "time": map[string]any{"completed": 2}, "content": []any{map[string]any{"type": "text", "text": "done"}}, "finish": "stop"})
			_ = json.NewEncoder(response).Encode(map[string]any{"data": history, "cursor": map[string]string{}})
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	p = &Wrapper{opened: true, id: sealedStopSessionID, client: testClient(server)}
	result, err := p.run(context.Background(), newFakeRun(), "go")
	if err != nil || result.Result != "done" || historyReads != 2 {
		t.Fatalf("result = %#v/%v after %d history reads", result, err, historyReads)
	}
}

func TestRunWakesOnEventStreamFailure(t *testing.T) {
	admitted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/prompt") {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": body["id"], "sessionID": "ses_exact", "prompt": body["prompt"], "delivery": "steer"}})
		close(admitted)
	}))
	defer server.Close()
	shutdown := make(chan struct{})
	p := &Wrapper{opened: true, id: "ses_exact", client: testClient(server), shutdown: func() { close(shutdown) }}
	run := newFakeRun()
	result := make(chan error, 1)
	go func() { _, err := p.run(context.Background(), run, "go"); result <- err }()
	<-admitted
	events := make(chan error, 1)
	events <- errors.New("OpenCode event stream ended")
	close(events)
	go p.watchEvents(events)
	if err := <-result; err == nil || err.Error() != "OpenCode event stream ended" {
		t.Fatalf("run error = %v", err)
	}
	close(run.done)
	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join failed run")
	}
}

func TestHeldPromptIsJoinedOnEventStreamFailure(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	client := &nativeClient{endpoint: "http://native", directory: "/work", http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/prompt") {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
		}
		close(started)
		<-request.Context().Done()
		close(stopped)
		return nil, request.Context().Err()
	})}}
	shutdown := make(chan struct{})
	p := &Wrapper{opened: true, id: "ses_exact", client: client, shutdown: func() { close(shutdown) }}
	run := newFakeRun()
	result := make(chan error, 1)
	go func() { _, err := p.run(context.Background(), run, "go"); result <- err }()
	<-started
	events := make(chan error, 1)
	events <- errors.New("OpenCode event stream ended while prompt was held")
	close(events)
	go p.watchEvents(events)
	if err := <-result; err == nil || err.Error() != "OpenCode event stream ended while prompt was held" {
		t.Fatalf("run error = %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("held prompt request was not joined")
	}
	select {
	case <-shutdown:
		t.Fatal("shutdown preceded Run.Done")
	default:
	}
	close(run.done)
	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join failed run")
	}
}

func TestFinishedRunQueuesDelivery(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = json.NewDecoder(request.Body).Decode(&requestBody)
		_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": requestBody["id"], "sessionID": "ses_exact", "prompt": requestBody["prompt"], "delivery": "queue", "timeCreated": 1}})
	}))
	defer server.Close()
	p := &Wrapper{opened: true, id: "ses_exact", client: testClient(server)}
	run := newFakeRun()
	run.Admitted()
	close(run.done)
	receipt, err := p.deliver(context.Background(), delivery(), run)
	if err != nil || receipt.Disposition != "queued_for_next_turn" || requestBody["resume"] != false {
		t.Fatalf("delivery = %#v/%v, body=%#v", receipt, err, requestBody)
	}
}

func TestStopBeforeInterruptCrossingReturnsCompletedRun(t *testing.T) {
	stopEvent := durableSubscriberEvent(t, sealedStopFrame)
	var historyReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/interrupt"):
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
			historyReads.Add(1)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{
				map[string]any{"id": "msg_input", "type": "user"},
				map[string]any{"id": sealedStopAssistantID, "type": "assistant", "time": map[string]any{"completed": 2}, "content": []any{map[string]any{"type": "text", "text": "done"}}, "finish": "stop"},
			}, "cursor": map[string]string{}})
		default:
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	p := &Wrapper{opened: true, id: sealedStopSessionID, client: testClient(server)}
	run := newFakeRun()
	client, id, signals, err := p.beginRun(context.Background(), run.Done())
	if err != nil {
		t.Fatal(err)
	}
	run.Admitted()
	p.observeTerminal(stopEvent)
	if err = p.interrupt(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	result, stop, err := waitForNativeTerminal(context.Background(), client, id, "msg_input", signals)
	if err != nil || result != "done" || stop != "stop" || historyReads.Load() != 1 {
		t.Fatalf("terminal = %q/%q/%v after %d history reads", result, stop, err, historyReads.Load())
	}
}

func TestUncorrelatedStopBeforeInterruptReturnsActionError(t *testing.T) {
	queueStop := durableSubscriberEvent(t, queueStopFrame)
	var signals *nativeRunSignals
	var historyReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/interrupt"):
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
			historyReads.Add(1)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{map[string]any{"id": "msg_input", "type": "user"}}, "cursor": map[string]string{}})
			signals.fail(errors.New("uncorrelated stop consumed preserved action"))
		default:
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	p := &Wrapper{opened: true, id: sealedStopSessionID, client: testClient(server)}
	run := newFakeRun()
	client, id, started, err := p.beginRun(context.Background(), run.Done())
	if err != nil {
		t.Fatal(err)
	}
	signals = started
	run.Admitted()
	p.observeTerminal(queueStop)
	if err = p.interrupt(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	_, _, err = waitForNativeTerminal(context.Background(), client, id, "msg_input", signals)
	if err == nil || err.Error() != "OpenCode run ended by interrupt" || historyReads.Load() != 1 {
		t.Fatalf("terminal = %v after %d history reads", err, historyReads.Load())
	}
}

func TestInterruptWaitsForAdmissionThenEndsRun(t *testing.T) {
	promptStarted, releasePrompt := make(chan struct{}), make(chan struct{})
	called := make(chan struct{}, 1)
	var historyReads atomic.Int32
	messageID := ""
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/prompt"):
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			messageID, _ = body["id"].(string)
			close(promptStarted)
			<-releasePrompt
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": body["id"], "sessionID": "ses_exact", "prompt": body["prompt"], "delivery": "steer"}})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/interrupt"):
			called <- struct{}{}
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/message"):
			historyReads.Add(1)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{map[string]any{"id": messageID, "type": "user"}}, "cursor": map[string]string{}})
		default:
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	p := &Wrapper{opened: true, id: "ses_exact", client: testClient(server)}
	run := newFakeRun()
	runDone, interruptDone := make(chan error, 1), make(chan error, 1)
	go func() { _, err := p.run(context.Background(), run, "go"); runDone <- err }()
	<-promptStarted
	go func() { interruptDone <- p.interrupt(context.Background(), run) }()
	select {
	case <-called:
		t.Fatal("interrupt preceded native run admission")
	default:
	}
	close(releasePrompt)
	if err := <-interruptDone; err != nil {
		t.Fatal(err)
	}
	<-called
	if err := <-runDone; err == nil || err.Error() != "OpenCode run ended by interrupt" {
		t.Fatalf("run error = %v", err)
	}
	if historyReads.Load() != 1 {
		t.Fatalf("interrupt read history %d times", historyReads.Load())
	}

	failed := newFakeRun()
	close(failed.done)
	if err := p.interrupt(context.Background(), failed); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("failed run was interrupted natively")
	default:
	}
}

func TestIdleRunQueuesDelivery(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = json.NewDecoder(request.Body).Decode(&requestBody)
		_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": requestBody["id"], "sessionID": "ses_exact", "prompt": requestBody["prompt"], "delivery": "queue", "timeCreated": 1}})
	}))
	defer server.Close()
	p := &Wrapper{opened: true, id: "ses_exact", client: testClient(server)}
	connection, reader, _ := startWorker(t, p)
	writeWorkerRequest(t, connection, 2, "message.deliver", delivery())
	var receipt sessionkit.DeliveryReceipt
	if err := json.Unmarshal(readWorkerResponse(t, reader, 2), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != "queued_for_next_turn" || requestBody["resume"] != false {
		t.Fatalf("delivery = %#v, body=%#v", receipt, requestBody)
	}
	writeWorkerRequest(t, connection, 3, "session.close", map[string]any{"session_id": "ses_exact@local"})
	readWorkerResponse(t, reader, 3)
}

func TestCloseDeletesOnlyForExplicitForget(t *testing.T) {
	deletes := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/session/ses_exact" {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		deletes++
		_, _ = response.Write([]byte("true"))
	}))
	defer server.Close()
	for _, row := range []struct {
		forget bool
		want   int
	}{{false, 0}, {true, 1}} {
		p := &Wrapper{id: "ses_exact", client: testClient(server)}
		if err := p.Close(context.Background(), sessionkit.SessionCloseRequest{Forget: row.forget}); err != nil {
			t.Fatal(err)
		}
		if deletes != row.want {
			t.Fatalf("forget %v: deletes = %d", row.forget, deletes)
		}
	}
}

func TestPermissionRejectEndsRun(t *testing.T) {
	rejected := make(chan map[string]string, 1)
	emit := make(chan struct{})
	var historyReads atomic.Int32
	promptID := ""
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if directory := request.Header.Get("x-opencode-directory"); directory != "/work" {
			t.Fatalf("directory header = %q", directory)
		}
		switch request.URL.Path {
		case "/event":
			if request.URL.RawQuery != "directory=%2Fwork" {
				t.Fatalf("legacy subscriber query = %q", request.URL.RawQuery)
			}
			response.Header().Set("Content-Type", "text/event-stream")
			response.WriteHeader(http.StatusOK)
			response.(http.Flusher).Flush()
			<-emit
			_, _ = response.Write([]byte(sealedPermissionFrame + "\n\n"))
			response.(http.Flusher).Flush()
			<-request.Context().Done()
		case "/api/session/" + sealedPermissionSessionID + "/prompt":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			promptID, _ = body["id"].(string)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{"admittedSeq": 0, "id": body["id"], "sessionID": sealedPermissionSessionID, "prompt": body["prompt"], "delivery": "steer"}})
			close(emit)
		case "/api/session/" + sealedPermissionSessionID + "/message":
			historyReads.Add(1)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{map[string]any{"id": promptID, "type": "user"}}, "cursor": map[string]string{}})
		case "/session/" + sealedPermissionSessionID + "/permissions/per_077d90743001KPEP6Whcwh0xQM":
			var body map[string]string
			_ = json.NewDecoder(request.Body).Decode(&body)
			rejected <- body
			_, _ = response.Write([]byte("true"))
		default:
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()
	client := testClient(server)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &Wrapper{opened: true, id: sealedPermissionSessionID, client: client}
	done, err := client.subscribe(ctx, func(ctx context.Context, event nativeEvent) error { return p.observe(ctx, client, event) })
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := p.run(context.Background(), newFakeRun(), "go")
	if runErr == nil || runErr.Error() != "OpenCode run ended by permission rejection" {
		t.Fatalf("run error = %v", runErr)
	}
	if body := <-rejected; body["response"] != "reject" {
		t.Fatalf("permission body = %#v", body)
	}
	if historyReads.Load() != 1 {
		t.Fatalf("permission rejection read history %d times", historyReads.Load())
	}
	cancel()
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("stream end = %v", err)
	}
}

func TestBootstrapEventCancellationDrainsStream(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		response.(http.Flusher).Flush()
		close(started)
		<-request.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- testClient(server).bootstrap(ctx) }()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("bootstrap error = %v", err)
	}
	<-stopped
}

func TestResultProjectsEveryCompletedAssistantAfterInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"data":[{"id":"msg_before","type":"assistant","time":{"created":1,"completed":2},"content":[{"type":"text","text":"before"}],"finish":"stop"},{"id":"msg_input","type":"user","text":"go","time":{"created":3}},{"id":"msg_one","type":"assistant","time":{"created":4,"completed":5},"content":[{"type":"text","text":"one"}],"finish":"tool-calls"},{"id":"msg_two","type":"assistant","time":{"created":6,"completed":7},"content":[{"type":"text","text":"two"}],"finish":"stop"}],"cursor":{}}`))
	}))
	defer server.Close()
	result, stop, _, err := testClient(server).result(context.Background(), "ses_exact", "msg_input", "")
	if err != nil || result != "one\ntwo" || stop != "stop" {
		t.Fatalf("result = %q/%q/%v", result, stop, err)
	}
}

func TestResultStopsAtCorrelatedAssistantBeforeLaterCompletedReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"data":[{"id":"msg_input","type":"user","text":"go"},{"id":"msg_one","type":"assistant","time":{"completed":1},"content":[{"type":"text","text":"one"}],"finish":"tool-calls"},{"id":"msg_correlated","type":"assistant","time":{"completed":2},"content":[{"type":"text","text":"two"}],"finish":"stop"},{"id":"msg_later","type":"assistant","time":{"completed":3},"content":[{"type":"text","text":"later"}],"finish":"stop"}],"cursor":{}}`))
	}))
	defer server.Close()
	result, stop, matched, err := testClient(server).result(context.Background(), "ses_exact", "msg_input", "msg_correlated")
	if err != nil || result != "one\ntwo" || stop != "stop" || !matched {
		t.Fatalf("result = %q/%q/%v/%v", result, stop, matched, err)
	}
}

func TestResultRejectsTerminalWithoutCompletedAssistant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"data":[{"id":"msg_input","type":"user","text":"go","time":{"created":1}}],"cursor":{}}`))
	}))
	defer server.Close()
	if _, _, _, err := testClient(server).result(context.Background(), "ses_exact", "msg_input", ""); err == nil || err.Error() != "OpenCode terminal history omitted a completed assistant" {
		t.Fatalf("error = %v", err)
	}
}

func TestWrapperDrainsOwnedTerminalBeforeChildExitShutdown(t *testing.T) {
	testWrapperChildExit(t, "terminal-exit", true)
}

func TestWrapperFailsThenJoinsRunBeforeChildExitShutdown(t *testing.T) {
	testWrapperChildExit(t, "no-terminal-exit", false)
}

func testWrapperChildExit(t *testing.T, mode string, wantTerminal bool) {
	directory, record := t.TempDir(), filepath.Join(t.TempDir(), "requests.jsonl")
	p := New(filepath.Join(directory, "sessionbus.sock"), "provisional", fakeOpenCode(t, mode, record))
	p.SetCall(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	shutdown := make(chan struct{})
	var shutdownOnce sync.Once
	p.SetShutdown(func() { shutdownOnce.Do(func() { close(shutdown) }) })
	if _, err := p.Open(context.Background(), sessionkit.OpenRequest{Name: "Lane title@local", Open: sessionkit.OpenOptions{Cwd: directory}}); err != nil {
		t.Fatal(err)
	}
	run := newFakeRun()
	result, err := p.run(context.Background(), run, "large")
	if wantTerminal {
		if err != nil || result.Outcome != "completed" || len(result.Result) != 300004 || result.NativeStopReason != "stop" {
			t.Fatalf("terminal = %#v/%v", result, err)
		}
	} else if err == nil {
		t.Fatalf("missing terminal completed: %#v", result)
	}
	select {
	case <-shutdown:
		t.Fatal("worker shutdown preceded Run.Done")
	default:
	}
	close(run.done)
	select {
	case <-shutdown:
	case <-time.After(2 * time.Second):
		t.Fatal("worker shutdown did not join Run.Done")
	}
	if err := p.Close(context.Background(), sessionkit.SessionCloseRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestResultKeepsAscendingOrderAcrossPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Query().Get("limit") != "100" {
			t.Fatalf("query = %s", request.URL.RawQuery)
		}
		if requests == 1 {
			if request.URL.Query().Get("order") != "asc" {
				t.Fatalf("initial order = %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{"data":[{"id":"msg_input","type":"user"}],"cursor":{"next":"page-2"}}`))
			return
		}
		if request.URL.Query().Get("cursor") != "page-2" || request.URL.Query().Has("order") {
			t.Fatalf("cursor = %s", request.URL.RawQuery)
		}
		_, _ = response.Write([]byte(`{"data":[{"id":"msg_answer","type":"assistant","time":{"completed":2},"content":[{"type":"text","text":"done"}],"finish":"stop"}],"cursor":{}}`))
	}))
	defer server.Close()
	result, stop, _, err := testClient(server).result(context.Background(), "ses_exact", "msg_input", "")
	if err != nil || result != "done" || stop != "stop" || requests != 2 {
		t.Fatalf("result = %q/%q/%v after %d requests", result, stop, err, requests)
	}
}

func TestOpenArgumentsAndInteractiveArity(t *testing.T) {
	arguments, model, agent, err := launchArguments(sessionkit.OpenOptions{PermissionMode: "default", Model: "openai/gpt", Arguments: []string{"--agent", "build", "--log-level", "INFO"}})
	if err != nil || !slices.Equal(arguments, []string{"--log-level", "INFO"}) || model.ProviderID != "openai" || model.ID != "gpt" || agent != "build" {
		t.Fatalf("arguments = %#v/%#v/%q/%v", arguments, model, agent, err)
	}
	if _, _, _, err = launchArguments(sessionkit.OpenOptions{Arguments: []string{"--pure"}}); err == nil || err.Error() != "unsupported argument --pure" {
		t.Fatalf("lane --pure = %v", err)
	}
	if _, _, _, err = launchArguments(sessionkit.OpenOptions{PermissionMode: "plan"}); err == nil || err.Error() != "unsupported value permission_mode=plan" {
		t.Fatalf("permission mode = %v", err)
	}
	arguments, _, agent, err = launchArguments(sessionkit.OpenOptions{Arguments: []string{"--log-level", "--agent"}})
	if err != nil || agent != "" || !slices.Equal(arguments, []string{"--log-level", "--agent"}) {
		t.Fatalf("native value arity = %#v/%q/%v", arguments, agent, err)
	}
	plan, native, err := InteractivePlan([]string{"--log-level", "-g", "team"}, []string{"PATH=/bin"})
	if err != nil || native || !slices.Equal(plan.Args, []string{"--log-level", "-g", "team"}) {
		t.Fatalf("arity plan = %#v/%v/%v", plan, native, err)
	}
	if !slices.Contains(plan.Env, host.SocketEnv+"="+sessionkit.Socket()) {
		t.Fatalf("default socket missing from %#v", plan.Env)
	}
	plan, native, err = InteractivePlan([]string{"run", "-g"}, []string{"PATH=/bin"})
	if err != nil || !native || !slices.Equal(plan.Args, []string{"run", "-g"}) {
		t.Fatalf("passthrough = %#v/%v/%v", plan, native, err)
	}
	plan, native, err = InteractivePlan([]string{"/work/project", "run", "-g", "team"}, []string{"PATH=/bin"})
	if err != nil || native || !slices.Equal(plan.Args, []string{"/work/project", "run"}) {
		t.Fatalf("project = %#v/%v/%v", plan, native, err)
	}
	if _, _, err = InteractivePlan([]string{"--pure=true"}, nil); err == nil {
		t.Fatal("--pure accepted")
	}
	plan, native, err = InteractivePlan([]string{"--log-level", "--pure"}, nil)
	if err != nil || native || !slices.Equal(plan.Args, []string{"--log-level", "--pure"}) {
		t.Fatalf("native pure value = %#v/%v/%v", plan, native, err)
	}
}

func TestNativeIDsFollowProductPrefixAndBusIdentityBounds(t *testing.T) {
	if !validNativeID("ses_日本") || validNativeID("ses bad") || validNativeID("ses_\xff") {
		t.Fatal("native session id boundary changed")
	}
	if !validPermissionID("per.dotted") || validPermissionID("request") {
		t.Fatal("native permission id prefix changed")
	}
}

func delivery() sessionkit.DeliveryRequest {
	return sessionkit.DeliveryRequest{MessageID: "delivery", From: sessionkit.DeliverySource{SessionID: "sender@local", Name: "Sender@local", Product: "codex", Groups: []string{}}, Body: "hello"}
}
