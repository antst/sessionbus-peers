// SPDX-License-Identifier: MIT
package qwen

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInteractiveNativeParentLossSettlesBindingAndBlockedHello(t *testing.T) {
	for _, blockedHello := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-registry", true: "held-hello"}[blockedHello], func(t *testing.T) {
			child := exec.Command("cat")
			input, e := child.StdinPipe()
			must(t, e)
			defer input.Close()
			must(t, child.Start())
			defer child.Process.Kill()
			b, _, registry := ownerFixture(t, "")
			b.parent, e = inspectNativeProcess(child.Process.Pid)
			must(t, e)
			if blockedHello {
				registry()
				listener, e := net.Listen("unix", b.launch.Socket)
				must(t, e)
				defer listener.Close()
				written := make(chan struct{})
				closed := make(chan struct{})
				go func() {
					defer close(closed)
					fd, e := listener.Accept()
					if e != nil {
						return
					}
					defer fd.Close()
					var request any
					if json.NewDecoder(fd).Decode(&request) != nil {
						return
					}
					close(written)
					_, _ = io.Copy(io.Discard, fd)
				}()
				b.Initialized()
				select {
				case <-written:
				case <-time.After(5 * time.Second):
					t.Fatal("hello not submitted")
				}
				must(t, child.Process.Kill())
				_ = child.Wait()
				select {
				case <-closed:
				case <-time.After(5 * time.Second):
					t.Fatal("native loss did not close held hello")
				}
			} else {
				b.Initialized()
				must(t, child.Process.Kill())
				_ = child.Wait()
			}
			select {
			case <-b.done:
			case <-time.After(5 * time.Second):
				t.Fatal("native loss left owner alive")
			}
			check(t, b.failure() != nil, "missing native failure")
		})
	}
}

func TestInteractiveBindingRejectsForeignAncestryAndRegistry(t *testing.T) {
	p, e := inspectNativeProcess(os.Getpid())
	must(t, e)
	_, e = bindNativeParent(os.Getpid(), os.Getpid(), "wrong-start")
	check(t, e != nil, "accepted replaced launcher")
	b, _, registry := ownerFixture(t, "")
	registry()
	session := initialNativeSession{ID: fixtureID, CWD: filepath.Join(b.home, "project😀")}
	ok, e := readInitialRegistry(b.home, p, session)
	must(t, e)
	check(t, ok, "valid registry rejected")
	session.ID = "foreign"
	_, e = readInitialRegistry(b.home, p, session)
	check(t, e != nil, "accepted registry with wrong native identity")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = b.Call(ctx, "session.list", map[string]any{})
	check(t, e != nil, "canceled binding call succeeded")
}

func TestInteractiveRecordsBoundIncompleteInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events")
	must(t, os.WriteFile(path, []byte(strings.Repeat("x", maxInteractiveRecord+1)), 0600))
	r := nativeRecords{path: path}
	e := r.read(func([]byte) error { return nil })
	check(t, e != nil, "unbounded incomplete event accepted")
}

func TestInteractiveOwnerClaimAllowsOnlyOneAttempt(t *testing.T) {
	first, _, registry := ownerFixture(t, "chosen")
	registry()
	binding, e := json.Marshal(first.launch)
	must(t, e)
	create := func() *interactiveOwner {
		b, e := newInteractiveOwner(context.Background(), []string{InteractiveEnv + "=" + string(binding), nativeSessionEnv + "=" + fixtureID, "QWEN_HOME=" + first.home}, os.Getpid())
		must(t, e)
		t.Cleanup(b.End)
		return b
	}
	second := create()
	first.Initialized()
	second.Initialized()
	var loser, winner *interactiveOwner
	select {
	case <-first.done:
		loser, winner = first, second
	case <-second.done:
		loser, winner = second, first
	case <-time.After(5 * time.Second):
		t.Fatal("overlapping owner did not fail")
	}
	check(t, strings.Contains(loser.failure().Error(), "already had an integration owner") && loser.caller == nil, "losing helper admitted Caller: %v", loser.failure())
	winner.End()
	third := create()
	third.Initialized()
	select {
	case <-third.done:
	case <-time.After(5 * time.Second):
		t.Fatal("failed first owner allowed retry")
	}
	check(t, third.caller == nil && strings.Contains(third.failure().Error(), "already had an integration owner"), "later helper retried naming/Caller")
	claim, e := os.ReadFile(filepath.Join(first.launch.Directory, "owner.claim"))
	must(t, e)
	check(t, len(claim) == 0, "claim stored native metadata")
	input, e := os.ReadFile(filepath.Join(first.launch.Directory, "input.jsonl"))
	must(t, e)
	check(t, strings.Count(string(input), "/rename chosen") <= 1, "rename replayed")
}

func TestInteractiveWatcherDistinguishesLauncherDeath(t *testing.T) {
	child := exec.Command("cat")
	in, e := child.StdinPipe()
	must(t, e)
	defer in.Close()
	must(t, child.Start())
	defer child.Process.Kill()
	native, e := inspectNativeProcess(os.Getpid())
	must(t, e)
	launcher, e := inspectNativeProcess(child.Process.Pid)
	must(t, e)
	watch, e := newInteractiveWatch(native, launcher)
	must(t, e)
	defer watch.close()
	must(t, child.Process.Kill())
	_ = child.Wait()
	select {
	case err := <-watch.failed:
		check(t, strings.Contains(err.Error(), "launcher exited"), "wrong lifetime cause: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("launcher loss not observed")
	}
	_, e = inspectNativeProcess(native.pid)
	must(t, e) // No cross-owner native kill.
}
