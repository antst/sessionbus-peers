// SPDX-License-Identifier: MIT
package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/antst/sessionbus-peers/internal/testsocket"
)

func startupRow() startupIdentity {
	return startupIdentity{PID: 42, SessionID: "native-startup", ProcStart: "123", Kind: "interactive", Entrypoint: "cli", Socket: "/native.sock"}
}

func startupHello(t *testing.T, w *wire, id, name string) {
	t.Helper()
	f := w.next(t)
	var fields map[string]any
	if err := json.Unmarshal(f.Params, &fields); err != nil {
		t.Fatal(err)
	}
	gotName, _ := fields["name"].(string)
	if f.Method != "session.hello" || fields["session_id"] != id || gotName != name {
		t.Fatalf("unexpected startup hello: %s %s", f.Method, f.Params)
	}
	w.reply(t, f, map[string]any{})
}

func TestStartupPublishesWithoutHookAndFollowsNativeName(t *testing.T) {
	o, wires := testOwner(t)
	var mu sync.Mutex
	row := startupRow()
	o.startStartupObservation(func() (startupIdentity, error) {
		mu.Lock()
		defer mu.Unlock()
		return row, nil
	}, time.Millisecond)
	w := <-wires
	startupHello(t, w, row.SessionID, "")
	mu.Lock()
	row.Name = "native-title"
	mu.Unlock()
	startupHello(t, w, "native-startup", "native-title")
	// The first hook takes over even when its native SID differs after resume.
	done := report(t, o, "UserPromptSubmit", "resumed-native", "resumed-title")
	newWire := <-wires
	startupHello(t, newWire, "resumed-native", "resumed-title")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	o.startupWork.Wait()
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.hookObserved || o.admitted == nil || o.admitted.SessionID != "resumed-native" {
		t.Fatal("native hook did not retain authority")
	}
}

func TestStartupReadCannotCrossNativeHookOrEnd(t *testing.T) {
	for _, ending := range []string{"SessionEnd", "Stop", "End"} {
		t.Run(ending, func(t *testing.T) {
			o, wires := testOwner(t)
			entered, release := make(chan struct{}), make(chan struct{})
			o.startStartupObservation(func() (startupIdentity, error) {
				close(entered)
				<-release
				return startupRow(), nil
			}, time.Hour)
			<-entered
			if ending == "End" {
				ended := make(chan struct{})
				go func() { o.End(); close(ended) }()
				// Observe terminal admission under the owner mutex before releasing
				// the held read, without assuming that the goroutine ran first.
				deadline := time.NewTimer(5 * time.Second)
				defer deadline.Stop()
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				for {
					o.mu.Lock()
					terminal := o.ended
					o.mu.Unlock()
					if terminal {
						break
					}
					select {
					case <-ticker.C:
					case <-deadline.C:
						t.Fatal("End did not mark terminal")
					}
				}
				close(release)
				<-ended
			} else {
				done := report(t, o, ending, "new-native", "new-name")
				close(release)
				if ending == "Stop" {
					w := <-wires
					startupHello(t, w, "new-native", "new-name")
					if err := <-done; err != nil {
						t.Fatal(err)
					}
				}
				o.startupWork.Wait()
			}
			select {
			case <-wires:
				t.Fatal("stale registry read published after authority handoff")
			default:
			}
		})
	}
}

func TestStartupParentLossWithdrawsAndCannotRepublish(t *testing.T) {
	o, wires := testOwner(t)
	var mu sync.Mutex
	lost := false
	o.startStartupObservation(func() (startupIdentity, error) {
		mu.Lock()
		defer mu.Unlock()
		if lost {
			return startupIdentity{}, errStartupParentEnded
		}
		return startupRow(), nil
	}, time.Millisecond)
	w := <-wires
	startupHello(t, w, "native-startup", "")
	mu.Lock()
	lost = true
	mu.Unlock()
	o.startupWork.Wait()
	if _, err := o.Action(context.Background(), "list", json.RawMessage(`{}`)); err == nil {
		t.Fatal("dead native parent remained callable")
	}
	if _, err := o.BeginReport(json.RawMessage(`{"hook_event_name":"Stop","session_id":"new"}`)); err == nil {
		t.Fatal("dead native parent was revived")
	}
}

func TestStartupRegistryOwnProcessAndBoundedNativeFile(t *testing.T) {
	directory := testsocket.Directory(t)
	socket := filepath.Join(directory, "native.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	path := filepath.Join(directory, "42.json")
	process := startupProcess{start: "123", generation: "123:exact"}
	r := &startupReader{pid: 42, path: path, process: process, inspect: func(int) (startupProcess, error) { return process, nil }}
	row := startupRow()
	row.Socket = socket
	write := func(value startupIdentity) {
		t.Helper()
		body, _ := json.Marshal(value)
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(row)
	if got, err := r.read(); err != nil || got != row {
		t.Fatalf("own registry: %+v %v", got, err)
	}
	for _, source := range []string{"derived", "future-label", "user", ""} {
		label := row
		label.Name, label.NameSource = "native-name", source
		write(label)
		got, err := r.read()
		if err != nil {
			t.Fatal(err)
		}
		want := ""
		if source == "user" || source == "" {
			want = label.Name
		}
		if got.Name != want {
			t.Fatalf("name source %q became title %q", source, got.Name)
		}
	}
	for _, field := range []string{"pid", "generation", "kind", "socket", "id"} {
		t.Run(field, func(t *testing.T) {
			bad := row
			switch field {
			case "pid":
				bad.PID++
			case "generation":
				bad.ProcStart = "old"
			case "kind":
				bad.Kind = "worker"
			case "socket":
				bad.Socket = path
			case "id":
				bad.SessionID = ""
			}
			write(bad)
			if _, err := r.read(); err == nil {
				t.Fatal("accepted foreign or incomplete record")
			}
		})
	}
	for _, body := range [][]byte{nil, []byte(`{"pid":`), make([]byte, nativeRegistryLimit+1)} {
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := r.read(); err == nil {
			t.Fatal("accepted partial or oversized record")
		}
	}
	write(row)
	if _, err := r.read(); err != nil {
		t.Fatal("transient partial read did not recover", err)
	}
	other := path + ".real"
	if err := os.Rename(path, other); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path); err != nil {
		t.Fatal(err)
	}
	if _, err := r.read(); err == nil {
		t.Fatal("accepted symlink record")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(other, path); err != nil {
		t.Fatal(err)
	}
	process.generation = "replacement"
	if _, err := r.read(); !errors.Is(err, errStartupParentEnded) {
		t.Fatal("did not reject replaced parent", err)
	}
}

func TestStartupRegistryMatchesActualProcessGeneration(t *testing.T) {
	process, err := inspectStartupProcess(os.Getpid())
	if err != nil || process.start == "" || process.generation == "" {
		t.Fatalf("live process: %+v %v", process, err)
	}
	directory := testsocket.Directory(t)
	socket := filepath.Join(directory, "native.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	path := filepath.Join(directory, "own.json")
	row := startupRow()
	row.PID, row.ProcStart, row.Socket, row.StartedAt = os.Getpid(), process.start, socket, time.Now().UnixMilli()
	body, _ := json.Marshal(row)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	r := &startupReader{pid: os.Getpid(), path: path, process: process, inspect: inspectStartupProcess}
	if _, err := r.read(); err != nil {
		t.Fatal(err)
	}
	r.process.generation += "replaced"
	if _, err := r.read(); !errors.Is(err, errStartupParentEnded) {
		t.Fatal("accepted wrong live process token", err)
	}
}
