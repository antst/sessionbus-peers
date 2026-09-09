// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
	"os"
	"path/filepath"
)

func main() {
	var err error
	if filepath.Base(os.Args[0]) == interactive.PrivateAlias {
		var owner *interactive.Owner
		owner, err = interactive.NewOwner(interactive.Environment(os.Environ()))
		if err == nil {
			err = interactive.Serve(owner, os.Stdin, os.Stdout)
		}
	} else {
		err = interactive.Launch(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude-peer:", err)
		os.Exit(1)
	}
}
