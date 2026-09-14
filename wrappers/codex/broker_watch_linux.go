// SPDX-License-Identifier: MIT

package codex

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"sync"
)

// watchBrokerParent observes the actual parent identity across launcher exec.
// The cancellation pipe wakes Poll; no PID polling or timer is involved.
func watchBrokerParent(expected int) (<-chan error, func(), error) {
	if expected <= 1 || os.Getppid() != expected {
		return nil, nil, errors.New("broker parent changed before watch")
	}
	pidfd, err := unix.PidfdOpen(expected, 0)
	if err != nil {
		return nil, nil, err
	}
	var pipe [2]int
	if err = unix.Pipe2(pipe[:], unix.O_CLOEXEC); err != nil {
		_ = unix.Close(pidfd)
		return nil, nil, err
	}
	if os.Getppid() != expected {
		_ = unix.Close(pidfd)
		_ = unix.Close(pipe[0])
		_ = unix.Close(pipe[1])
		return nil, nil, errors.New("broker parent changed during watch")
	}
	done := make(chan error, 1)
	var once sync.Once
	stop := func() { once.Do(func() { _ = unix.Close(pipe[1]) }) }
	go func() {
		defer close(done)
		defer unix.Close(pidfd)
		defer unix.Close(pipe[0])
		fds := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}, {Fd: int32(pipe[0]), Events: unix.POLLIN}}
		for {
			_, err := unix.Poll(fds, -1)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				done <- err
				return
			}
			if fds[1].Revents != 0 {
				return
			}
			if fds[0].Revents != 0 {
				done <- errors.New("native TUI parent exited")
				return
			}
		}
	}()
	return done, stop, nil
}
