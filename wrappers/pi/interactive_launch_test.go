// SPDX-License-Identifier: MIT

package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/pifamily"
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

const piInteractiveChildEnv = "PI_INTERACTIVE_TEST_CHILD"

type piInteractiveChildCapture struct {
	Args       []string                    `json:"args"`
	Descriptor interactiveLaunchDescriptor `json:"descriptor"`
	Parent     int                         `json:"parent"`
	Leaked     []string                    `json:"leaked"`
	Signal     string                      `json:"signal,omitempty"`
}

func TestPiInteractiveNativeChild(t *testing.T) {
	mode := os.Getenv(piInteractiveChildEnv)
	if mode == "" {
		return
	}
	index := 0
	for index < len(os.Args) && os.Args[index] != "--" {
		index++
	}
	var descriptor interactiveLaunchDescriptor
	if index == len(os.Args) || json.Unmarshal([]byte(os.Getenv(InteractiveLaunchEnv)), &descriptor) != nil {
		os.Exit(91)
	}
	capture := piInteractiveChildCapture{Args: slicesClone(os.Args[index+1:]), Descriptor: descriptor, Parent: os.Getppid()}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "SESSIONBUS_") && key != InteractiveLaunchEnv {
			capture.Leaked = append(capture.Leaked, key)
		}
	}
	conn, err := net.Dial("unix", descriptor.Socket)
	if err != nil {
		os.Exit(92)
	}
	native, err := pifamily.NewBridge(conn, pifamily.BridgeNative, func(_ context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
		var result any
		switch method {
		case "native.describe":
			var request struct {
				SessionID string `json:"session_id"`
			}
			if decodeInteractiveParams(raw, &request) != nil || request.SessionID != "native-session" {
				return nil, pifamily.NewBridgeCallError("bad_request", "wrong native description")
			}
			result = interactiveDescribeResult{"native-session", "native title", "/native/work"}
		case "native.append":
			return nil, pifamily.NewBridgeCallError("bad_request", "unexpected append")
		default:
			return nil, pifamily.NewBridgeCallError("method_not_found", "unexpected native call")
		}
		return json.Marshal(result)
	}, pifamily.BridgeLimits{})
	if err != nil || native.Ready(context.Background()) != nil {
		os.Exit(93)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGHUP)
	var ready map[string]string
	if native.Call(context.Background(), "owner.ready", interactiveReadyRequest{
		Topology: interactiveTopology, Directory: descriptor.Directory, SessionID: "native-session", Name: "ready title",
	}, &ready) != nil || ready["session_id"] != "native-session" {
		os.Exit(94)
	}
	writeCapture := func() {
		body, marshalErr := json.Marshal(capture)
		if marshalErr != nil || os.WriteFile(os.Getenv("PI_INTERACTIVE_TEST_CAPTURE"), body, 0o600) != nil {
			os.Exit(95)
		}
	}
	writeCapture()
	if mode == "bridge-loss" {
		_ = native.Close()
	}
	if mode == "exit-37" {
		var ended map[string]string
		if native.Call(context.Background(), "session_end", interactiveEndRequest{interactiveTopology, "native-session", "quit"}, &ended) != nil {
			os.Exit(96)
		}
		_ = native.Close()
		os.Exit(37)
	}
	received := <-signals
	signal.Stop(signals)
	capture.Signal = received.String()
	writeCapture()
	if mode != "bridge-loss" {
		var ended map[string]string
		if native.Call(context.Background(), "session_end", interactiveEndRequest{interactiveTopology, "native-session", "quit"}, &ended) != nil || ended["session_id"] != "native-session" {
			os.Exit(97)
		}
		_ = native.Close()
	}
	os.Exit(0)
}

func slicesClone[T any](values []T) []T { return append([]T(nil), values...) }

func installPiInteractiveCommandFixture(t *testing.T) {
	t.Helper()
	original := piInteractiveCommand
	piInteractiveCommand = func(_ string, arguments ...string) *exec.Cmd {
		args := []string{"-test.run=^TestPiInteractiveNativeChild$", "--"}
		return exec.Command(os.Args[0], append(args, arguments...)...)
	}
	t.Cleanup(func() { piInteractiveCommand = original })
}

func readPiChildCapture(t *testing.T, path string) piInteractiveChildCapture {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		body, err := os.ReadFile(path)
		var capture piInteractiveChildCapture
		if err == nil && json.Unmarshal(body, &capture) == nil {
			return capture
		}
		if time.Now().After(deadline) {
			t.Fatalf("missing native capture: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

func servePiInteractiveHello(t *testing.T, listener net.Listener, cancel context.CancelFunc) (kit.PeerIdentity, <-chan struct{}) {
	t.Helper()
	identity := make(chan kit.PeerIdentity, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		if !scanner.Scan() {
			return
		}
		frame, err := protocol.DecodeFrame(scanner.Bytes())
		if err != nil || !frame.Request || frame.Method != "session.hello" {
			return
		}
		params, err := protocol.DecodeParams(frame.Method, frame.Params)
		hello, ok := params.(*protocol.PeerHello)
		if err != nil || !ok {
			return
		}
		body, err := protocol.ResultBytes(frame.ID, frame.Method, struct{}{})
		if err != nil {
			return
		}
		if _, err = conn.Write(body); err != nil {
			return
		}
		identity <- *hello
		if cancel != nil {
			cancel()
		}
		for scanner.Scan() {
		}
	}()
	select {
	case hello := <-identity:
		return hello, done
	case <-time.After(5 * time.Second):
		t.Fatal("Pi public hello did not arrive")
		return kit.PeerIdentity{}, done
	}
}

func piInteractiveLaunchPlan(t *testing.T, listener net.Listener, capture, mode string) host.ExecPlan {
	t.Helper()
	t.Setenv(piInteractiveChildEnv, mode)
	t.Setenv("PI_INTERACTIVE_TEST_CAPTURE", capture)
	return host.ExecPlan{
		Path: "/native/pi",
		Args: []string{"--model", "fixture/model"},
		Env: append(os.Environ(),
			host.SocketEnv+"="+listener.Addr().String(),
			host.GroupsEnv+`=["team"]`,
			host.NameEnv+"=wrapper title",
			host.SessionIDEnv+"=stale",
			host.TokenEnv+"=stale",
		),
	}
}

func TestRunPiInteractiveOwnsBridgeIdentityAndGracefulSignalCleanup(t *testing.T) {
	installPiInteractiveCommandFixture(t)
	listener := interactiveBusListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	capturePath := filepath.Join(t.TempDir(), "native.json")
	plan := piInteractiveLaunchPlan(t, listener, capturePath, "signal")
	result := make(chan error, 1)
	go func() { result <- runInteractiveResolved(ctx, plan, "/native/pi", "/plugin/pi/extension.mjs") }()
	hello, busDone := servePiInteractiveHello(t, listener, cancel)
	if hello.Product != Product || hello.SessionID != "native-session" || hello.Name != "native title" || hello.Info["cwd"] != "/native/work" || !reflect.DeepEqual(hello.Groups, []string{"team"}) {
		t.Fatalf("hello = %+v", hello)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	<-busDone
	capture := readPiChildCapture(t, capturePath)
	if capture.Parent != os.Getpid() || capture.Descriptor.OwnerPID != os.Getpid() || capture.Descriptor.Topology != interactiveTopology ||
		capture.Descriptor.Directory == "" || capture.Descriptor.Socket != filepath.Join(capture.Descriptor.Directory, interactiveBridgeSocket) ||
		!reflect.DeepEqual(capture.Args, []string{"--extension", "/plugin/pi/extension.mjs", "--model", "fixture/model"}) || len(capture.Leaked) != 0 || capture.Signal != "terminated" {
		t.Fatalf("native capture = %+v", capture)
	}
	if _, err := os.Stat(capture.Descriptor.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private directory remains: %v", err)
	}
}

func TestRunPiInteractivePropagatesNativeExitAndContainsBridgeLoss(t *testing.T) {
	for _, mode := range []string{"exit-37", "bridge-loss"} {
		t.Run(mode, func(t *testing.T) {
			installPiInteractiveCommandFixture(t)
			listener := interactiveBusListener(t)
			capturePath := filepath.Join(t.TempDir(), "native.json")
			plan := piInteractiveLaunchPlan(t, listener, capturePath, mode)
			result := make(chan error, 1)
			go func() {
				result <- runInteractiveResolved(context.Background(), plan, "/native/pi", "/plugin/pi/extension.mjs")
			}()
			hello, busDone := servePiInteractiveHello(t, listener, nil)
			if hello.SessionID != "native-session" {
				t.Fatalf("hello = %+v", hello)
			}
			err := <-result
			<-busDone
			capture := readPiChildCapture(t, capturePath)
			if mode == "exit-37" {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 37 {
					t.Fatalf("native exit = %v", err)
				}
			} else if err == nil || !errors.Is(err, pifamily.ErrBridgeClosed) || capture.Signal != "terminated" {
				t.Fatalf("bridge loss = %v, capture=%+v", err, capture)
			}
			if _, statErr := os.Stat(capture.Descriptor.Directory); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("private directory remains: %v", statErr)
			}
		})
	}
}
