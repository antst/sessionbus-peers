// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"encoding/json"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestNativeLifetimeOutlivesCompletedOpenContext(t *testing.T) {
	t.Setenv("GO_WANT_CODEX_PROCESS", "1")
	t.Setenv("CODEX_TEST_EVIDENCE", filepath.Join(t.TempDir(), "child.json"))
	cwd := t.TempDir()
	t.Setenv("CODEX_TEST_CWD", cwd)
	original := laneCommand
	laneCommand = func(_ string, args ...string) *exec.Cmd {
		return exec.Command(os.Args[0], append([]string{"-test.run=TestCodexProcess", "--"}, args...)...)
	}
	defer func() { laneCommand = original }()
	p := New()
	p.SetCall(func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := p.Open(ctx, kit.OpenRequest{Name: "lifetime@local", Open: kit.OpenOptions{Cwd: cwd}}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if p.ctx.Err() != nil {
		t.Fatal("completed Open context owns native lifetime")
	}
	if err := p.child.command.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(context.Background(), kit.SessionCloseRequest{Forget: true}); err != nil {
		t.Fatal(err)
	}
	if err := p.child.Wait(); err != nil {
		t.Fatalf("normal close killed native: %v", err)
	}
	if _, err := os.Stat(p.endpoint.path); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
func TestStartupCancellationCannotCommitOpen(t *testing.T) {
	for _, order := range []string{"cancel-first", "commit-first"} {
		t.Run(order, func(t *testing.T) {
			p := New()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stop, err := p.startLifetime(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			defer p.cancel()
			p.id = "native"
			if order == "cancel-first" {
				cancel()
			}
			_, err = p.commitOpen(ctx, kit.OpenRequest{Name: "name@host"}, nativeThread{ID: "native", Name: "name"}, stop)
			if order == "cancel-first" {
				if !errors.Is(err, context.Canceled) || p.opened {
					t.Fatalf("err=%v opened=%v", err, p.opened)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				cancel()
				if p.ctx.Err() != nil {
					t.Fatal("late cancellation killed committed lifetime")
				}
			}
		})
	}
}
func TestClosingExpectedEOFDoesNotAbortNativeLifetime(t *testing.T) {
	p := New()
	p.ctx, p.cancel = context.WithCancel(context.Background())
	defer p.cancel()
	p.opened, p.closing = true, true
	p.nativeFailure(io.EOF)
	if p.ctx.Err() != nil || p.failure != nil {
		t.Fatal("expected EOF killed normal close")
	}
	p.nativeFailure(errors.New("invalid native frame"))
	if p.ctx.Err() == nil || p.failure == nil {
		t.Fatal("malformed frame ignored during close")
	}
}

type closeErrorInput struct {
	io.WriteCloser
	err error
}

func (w closeErrorInput) Close() error { return errors.Join(w.WriteCloser.Close(), w.err) }
func TestCodexCloseProcess(t *testing.T) {
	mode := os.Getenv("CODEX_CLOSE_FIXTURE")
	if mode == "" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	if mode == "nonzero" {
		os.Exit(23)
	}
	if mode == "malformed" {
		_, _ = os.Stdout.WriteString("not-json\n")
	}
	os.Exit(0)
}
func TestCloseRetainsExitDrainAndInputErrors(t *testing.T) {
	for _, mode := range []string{"normal", "nonzero", "malformed", "input-error"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestCodexCloseProcess")
			cmd.Env = append(os.Environ(), "CODEX_CLOSE_FIXTURE="+mode)
			child, input, output, err := startNative(cmd)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stop := context.AfterFunc(ctx, child.abort)
			defer stop()
			p := New()
			p.ctx, p.cancel, p.child, p.opened = ctx, cancel, child, true
			var writer io.WriteCloser = input
			inputErr := errors.New("fixture stdin close failed")
			if mode == "input-error" {
				writer = closeErrorInput{input, inputErr}
			}
			p.app = newAppClient(writer, output, nil, p.nativeFailure)
			err = p.Close(context.Background(), kit.SessionCloseRequest{})
			switch mode {
			case "normal":
				if err != nil {
					t.Fatal(err)
				}
			case "nonzero":
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 23 {
					t.Fatalf("exit lost: %v", err)
				}
			case "malformed":
				if err == nil {
					t.Fatal("drain failure lost")
				}
			case "input-error":
				if !errors.Is(err, inputErr) {
					t.Fatalf("input error lost: %v", err)
				}
			}
		})
	}
}
