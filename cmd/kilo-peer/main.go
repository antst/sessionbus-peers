// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/kilo"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

func main() {
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
	if !host.LaneMode() {
		// Native TUI receives foreground terminal SIGINT itself. Observing it
		// here avoids termination or translating it into a duplicate TERM.
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
		signals = []os.Signal{syscall.SIGTERM, syscall.SIGHUP}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), signals...)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code := 1
		var native *exec.ExitError
		if errors.As(err, &native) {
			if native.ExitCode() >= 0 {
				code = native.ExitCode()
			} else if status, ok := native.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				code = 128 + int(status.Signal())
			}
		}
		os.Exit(code)
	}
}

func run(ctx context.Context, arguments []string) error {
	if len(arguments) > 0 && arguments[0] == "--sessionbus-install" {
		return kilo.InstallPlugin(arguments[1:])
	}
	if !host.LaneMode() {
		plan, passthrough, err := kilo.InteractivePlan(arguments, os.Environ())
		if err != nil {
			return err
		}
		if passthrough {
			path, err := exec.LookPath(plan.Path)
			if err != nil {
				return err
			}
			return syscall.Exec(path, append([]string{path}, plan.Args...), plan.Env)
		}
		return kilo.RunInteractive(ctx, plan)
	}
	if len(arguments) != 0 {
		return errors.New("lane mode accepts no arguments")
	}
	native, err := kilo.ResolveNativeExecutable("kilo")
	if err != nil {
		return err
	}
	// The Worker constructs its native child later, after Open supplies cwd.
	// Carry only the resolver's native resource selection into this process-local
	// environment; the lane owns its auth and launch-marker projection.
	for _, entry := range native.Environment(os.Environ()) {
		if value, ok := strings.CutPrefix(entry, "KILO_TREE_SITTER_WASM_DIR="); ok {
			if err := os.Setenv("KILO_TREE_SITTER_WASM_DIR", value); err != nil {
				return err
			}
		}
	}
	product := kilo.New(os.Getenv(host.SocketEnv), host.LaunchTokenDigest(os.Getenv(host.TokenEnv)), native.Path)
	worker := sessionkit.NewWorker(product)
	product.SetShutdown(worker.Shutdown)
	product.SetCaller(worker.Caller())
	return worker.Serve(ctx)
}
