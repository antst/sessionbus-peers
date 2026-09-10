// SPDX-License-Identifier: MIT
package qwen

import (
	"bufio"
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type nativeEventStream struct {
	initial      chan initialNativeSession
	done, closed chan struct{}
	cancel       context.CancelFunc
}

// A single claimed helper owns this reader. RDWR avoids a false EOF while
// native Qwen has not yet opened its supported FIFO output. Cancellation owns
// the close; actual native/launcher lifetime comes from the OS process watcher.
func openNativeEvents(ctx context.Context, path string, fail func(error)) (*nativeEventStream, error) {
	f, err := os.OpenFile(path, os.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		_ = f.Close()
		return nil, errors.Join(errors.New("native event output must be the launch FIFO"), err)
	}
	if err = f.SetReadDeadline(time.Time{}); err != nil {
		_ = f.Close()
		return nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	s := &nativeEventStream{initial: make(chan initialNativeSession, 1), done: make(chan struct{}), closed: make(chan struct{}), cancel: cancel}
	context.AfterFunc(lifetime, func() { _ = f.Close(); close(s.closed) })
	go func() {
		defer close(s.done)
		reader := bufio.NewReaderSize(f, 32<<10)
		var session initialNativeSession
		var record []byte
		length := 0
		for {
			part, e := reader.ReadSlice('\n')
			length += len(part)
			if length > maxInteractiveRecord {
				fail(errors.New("native event record exceeds 8 MiB"))
				return
			}
			if session.ID == "" {
				record = append(record, part...)
			}
			if e == nil {
				if session.ID == "" {
					if err = session.observe(record); err != nil {
						fail(err)
						return
					}
					if session.ID != "" {
						s.initial <- session
					}
				}
				// After identity, output is drained without retaining answers.
				record = nil
				length = 0
				continue
			}
			if e == bufio.ErrBufferFull {
				continue
			}
			if lifetime.Err() == nil {
				fail(e)
			}
			return
		}
	}()
	return s, nil
}
func (s *nativeEventStream) close() { s.cancel(); <-s.closed; <-s.done }
