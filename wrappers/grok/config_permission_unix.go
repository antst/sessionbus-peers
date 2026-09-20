// SPDX-License-Identifier: MIT

//go:build unix

package grok

import (
	"errors"
	"os"
	"syscall"
)

func preserveConfigOwner(file *os.File, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("read Grok config ownership")
	}
	return file.Chown(int(stat.Uid), int(stat.Gid))
}
