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
	// Go excludes FIFOs opened through os.OpenFile from Darwin's poller:
	// kqueue can miss the final writer's close (Go issue 24164). This reader
	// deliberately keeps its own writer open, never uses writer EOF as a
	// lifetime signal, and owns cancellation through Close. Register this
	// nonblocking descriptor explicitly so data readiness and Close can wake
	// the reader on both platforms. No extra descriptor or timer is added.
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var info unix.Stat_t
	err = unix.Fstat(fd, &info)
	if err != nil || info.Mode&unix.S_IFMT != unix.S_IFIFO {
		_ = unix.Close(fd)
		return nil, errors.Join(errors.New("native event output must be the launch FIFO"), err)
	}
	f := os.NewFile(uintptr(fd), path)
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
