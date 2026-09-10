// SPDX-License-Identifier: MIT

package qwen

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// Inherited blocking stdio is not registered with Go's poller. Closing its
// os.File cannot interrupt an in-flight syscall. Register an owned nonblocking
// duplicate before any I/O instead. Dup shares status flags with the original;
// this helper is only for the forwarder's exclusively owned transports.
func pollableForwardFile(file *os.File) (*os.File, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0 {
		return nil, fmt.Errorf("Qwen MCP transport %s must be a pipe or socket", file.Name())
	}
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	fd := -1
	var dupErr error
	if err = raw.Control(func(original uintptr) {
		fd, dupErr = unix.FcntlInt(original, unix.F_DUPFD_CLOEXEC, 0)
	}); err != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		return nil, err
	}
	if dupErr != nil {
		return nil, dupErr
	}
	if err = unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	pollable := os.NewFile(uintptr(fd), file.Name())
	// A zero deadline installs no timer. It verifies poll registration rather
	// than promising that an unsupported blocking descriptor can be cancelled.
	if err = pollable.SetDeadline(time.Time{}); err != nil {
		_ = pollable.Close()
		return nil, fmt.Errorf("Qwen MCP transport is not pollable: %w", err)
	}
	return pollable, nil
}
