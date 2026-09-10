// SPDX-License-Identifier: MIT
package qwen

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestNativeFIFOReaderBeforeWriterAndJoinedCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.fifo")
	must(t, unix.Mkfifo(path, 0600))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failed := make(chan error, 1)
	stream, e := openNativeEvents(ctx, path, func(err error) { failed <- err })
	must(t, e)
	defer stream.close()
	writer, e := os.OpenFile(path, os.O_WRONLY|unix.O_NONBLOCK, 0)
	must(t, e)
	must(t, json.NewEncoder(writer).Encode(map[string]any{"type": "system", "subtype": "session_start", "data": map[string]any{"session_id": fixtureID, "cwd": "/native"}}))
	select {
	case session := <-stream.initial:
		check(t, session.ID == fixtureID && session.CWD == "/native", "session=%+v", session)
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("FIFO initial event missing")
	}
	must(t, writer.Close())
	// Our RDWR lifetime keeps this read open across native writer gaps. The
	// process watcher, not a temporary FIFO EOF, owns native-exit attribution.
	select {
	case <-stream.done:
		t.Fatal("writer close falsely ended native lifetime")
	default:
	}
	cancel()
	stream.close()
	select {
	case err := <-failed:
		t.Fatalf("owned FIFO cancellation reported failure: %v", err)
	default:
	}
}

func TestNativeFIFOOnlyInitialIdentityAndNoOutputStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.fifo")
	must(t, unix.Mkfifo(path, 0600))
	writer, e := os.OpenFile(path, os.O_RDWR|unix.O_NONBLOCK, 0)
	must(t, e)
	defer writer.Close()
	must(t, json.NewEncoder(writer).Encode(map[string]any{"type": "system", "subtype": "session_start", "data": map[string]any{"session_id": fixtureID, "cwd": "/native"}}))
	failed := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, e := openNativeEvents(ctx, path, func(err error) { failed <- err })
	must(t, e)
	defer stream.close()
	select {
	case <-stream.initial:
	case <-time.After(5 * time.Second):
		t.Fatal("buffered native-before-reader event lost")
	}
	must(t, json.NewEncoder(writer).Encode(map[string]any{"type": "system", "subtype": "session_start", "data": map[string]any{"session_id": "later", "cwd": "/later"}}))
	cancel()
	stream.close()
	select {
	case s := <-stream.initial:
		t.Fatalf("later session reattributed: %+v", s)
	default:
	}
	info, e := os.Stat(path)
	must(t, e)
	check(t, info.Mode()&os.ModeNamedPipe != 0 && info.Size() == 0, "native output became stored transcript")
}
