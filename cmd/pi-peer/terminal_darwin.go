// SPDX-License-Identifier: MIT

//go:build darwin

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func piTerminal(file *os.File) bool {
	_, err := unix.IoctlGetTermios(int(file.Fd()), unix.TIOCGETA)
	return err == nil
}
