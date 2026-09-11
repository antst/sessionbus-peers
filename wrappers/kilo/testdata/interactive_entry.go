// SPDX-License-Identifier: MIT
//go:build ignore

// Compiled launcher fixture. The final product maintenance/Worker front door is
// composed separately; this driver exercises the exported managed launcher.
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/antst/sessionbus-peers/wrappers/kilo"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func main() {
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	plan, native, err := kilo.InteractivePlan(os.Args[1:], os.Environ())
	if err == nil && native {
		err = errors.New("unexpected passthrough in managed fixture")
	}
	if err == nil {
		err = kilo.RunInteractive(ctx, plan)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code := 1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
			if code < 0 {
				if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
					code = 128 + int(status.Signal())
				}
			}
		}
		os.Exit(code)
	}
}
