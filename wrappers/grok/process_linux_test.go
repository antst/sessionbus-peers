// SPDX-License-Identifier: MIT

//go:build linux

package grok

import (
	"testing"

	"golang.org/x/sys/unix"
)

type processHandle struct{ fd int }

func pidfd(t *testing.T, pid int) processHandle {
	t.Helper()
	fd, err := unix.PidfdOpen(pid, 0)
	must(t, err)
	return processHandle{fd: fd}
}

func closeProcessHandle(handle processHandle) { _ = unix.Close(handle.fd) }

func processRunning(t *testing.T, handle processHandle) bool {
	t.Helper()
	poll := []unix.PollFd{{Fd: int32(handle.fd), Events: unix.POLLIN}}
	count, err := unix.Poll(poll, 0)
	must(t, err)
	return count == 0
}

func waitProcessExit(t *testing.T, handle processHandle) {
	t.Helper()
	poll := []unix.PollFd{{Fd: int32(handle.fd), Events: unix.POLLIN}}
	_, err := unix.Poll(poll, -1)
	must(t, err)
}
