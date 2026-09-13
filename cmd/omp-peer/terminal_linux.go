// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func ompTerminal(file *os.File) bool {
	_, err := unix.IoctlGetTermios(int(file.Fd()), unix.TCGETS)
	return err == nil
}
