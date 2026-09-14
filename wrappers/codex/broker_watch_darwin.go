// SPDX-License-Identifier: MIT

package codex

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"sync"
)

func watchBrokerParent(expected int) (<-chan error, func(), error) {
	if expected <= 1 || os.Getppid() != expected {
		return nil, nil, errors.New("broker parent changed before watch")
	}
	queue, err := unix.Kqueue()
	if err != nil {
		return nil, nil, err
	}
	unix.CloseOnExec(queue)
	var pipe [2]int
	if err = unix.Pipe(pipe[:]); err != nil {
		_ = unix.Close(queue)
		return nil, nil, err
	}
	unix.CloseOnExec(pipe[0])
	unix.CloseOnExec(pipe[1])
	changes := []unix.Kevent_t{{Ident: uint64(expected), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ENABLE | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT}, {Ident: uint64(pipe[0]), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD | unix.EV_ENABLE}}
	_, err = unix.Kevent(queue, changes, nil, nil)
	if err != nil || os.Getppid() != expected {
		_ = unix.Close(queue)
		_ = unix.Close(pipe[0])
		_ = unix.Close(pipe[1])
		if err == nil {
			err = errors.New("broker parent changed during watch")
		}
		return nil, nil, err
	}
	done := make(chan error, 1)
	var once sync.Once
	stop := func() { once.Do(func() { _ = unix.Close(pipe[1]) }) }
	go func() {
		defer close(done)
		defer unix.Close(queue)
		defer unix.Close(pipe[0])
		events := make([]unix.Kevent_t, 2)
		for {
			n, err := unix.Kevent(queue, nil, events, nil)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				done <- err
				return
			}
			for _, e := range events[:n] {
				if e.Filter == unix.EVFILT_READ {
					return
				}
				if e.Flags&unix.EV_ERROR != 0 {
					done <- unix.Errno(e.Data)
					return
				}
				if e.Filter == unix.EVFILT_PROC {
					done <- errors.New("native TUI parent exited")
					return
				}
			}
		}
	}()
	return done, stop, nil
}
