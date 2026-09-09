// SPDX-License-Identifier: MIT
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/antst/sessionbus-peers/internal/testsocket"
)

func TestBrokerNativeFixture(t *testing.T) {
	// The product environment deliberately removes SESSIONBUS_*; this dedicated
	// Go fixture marker uses a separate test variable inherited by no real launch.
	marker := os.Getenv("BROKER_NATIVE_FIXTURE_MARKER")
	if marker == "" {
		return
	}
	_, err := io.Copy(io.Discard, os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	// Tail bytes are written after EOF and must be drained before broker returns.
	fmt.Fprintln(os.Stdout, strings.Repeat("x", 65536))
	fmt.Fprintln(os.Stderr, "controlled native fixture finished after stdin EOF")
	if os.WriteFile(marker, []byte("EOF; native cleanup complete"), 0600) != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
func TestBrokerProcessCancellationClosesSoleStdinAndJoins(t *testing.T) {
	root := testsocket.Directory(t)
	// Match the production launcher directly under the OS temp root. The old
	// extra nested fixture directory exceeded macOS's Unix socket path limit.
	dir, err := os.MkdirTemp("", "sessionbus-codex-launch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	marker := filepath.Join(root, "native-finished")
	t.Setenv("BROKER_NATIVE_FIXTURE_MARKER", marker)
	previous := brokerNativeCommand
	brokerNativeCommand = func(_ string, args ...string) *exec.Cmd {
		expected := append(append([]string{"app-server", "--stdio"}, ActivationArguments()...), "-c", "model=literal")
		if !reflect.DeepEqual(args, expected) {
			t.Errorf("native args %v", args)
		}
		return exec.Command(os.Args[0], "-test.run=^TestBrokerNativeFixture$")
	}
	t.Cleanup(func() { brokerNativeCommand = previous })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyRead, readyWrite := io.Pipe()
	defer readyRead.Close()
	done := make(chan error, 1)
	go func() {
		done <- runInteractiveBroker(ctx, brokerLaunch{Parent: os.Getppid(), Native: "fixture", Dir: dir, BusSocket: "/bus", Config: []string{"-c", "model=literal"}}, readyWrite)
		_ = readyWrite.Close()
	}()
	var ready map[string]bool
	if err := json.NewDecoder(bufio.NewReader(readyRead)).Decode(&ready); err != nil || !ready["Ready"] {
		cancel()
		t.Fatalf("ready=%v decode=%v broker=%v", ready, err, <-done)
	}
	for _, name := range []string{"tui.sock", "owner.sock"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("broker did not join native cleanup")
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "EOF; native cleanup complete" {
		t.Fatal(string(body), err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("owned endpoints remain", err)
	}
}
func TestBrokerEnvironmentPreservesNativeHomeAndOnlyLaunchEndpoint(t *testing.T) {
	env := brokerEnvironment([]string{"HOME=/real/home", "PATH=/real/bin", "CODEX_HOME=/real/config", "SESSIONBUS_GROUPS=old", "SESSIONBUS_LAUNCH_TOKEN=secret", "SESSIONBUS_SOCKET=/old", EndpointEnv + "=/old", "KEEP=literal"}, "/launch/owner.sock")
	want := []string{"HOME=/real/home", "PATH=/real/bin", "CODEX_HOME=/real/config", "KEEP=literal", EndpointEnv + "=/launch/owner.sock"}
	if !reflect.DeepEqual(env, want) {
		t.Fatal(env)
	}
}
