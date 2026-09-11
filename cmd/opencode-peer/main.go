// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/opencode"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

func main() {
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if !host.LaneMode() {
		// Native TUI receives foreground terminal SIGINT itself. Observing it
		// here avoids termination or translating it into a duplicate TERM.
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
		signals = []os.Signal{syscall.SIGTERM}
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
		return opencode.InstallPlugin(arguments[1:])
	}
	if !host.LaneMode() {
		plan, passthrough, err := opencode.InteractivePlan(arguments, os.Environ())
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
		return opencode.RunInteractive(ctx, plan)
	}
	if len(arguments) != 0 {
		return errors.New("lane mode accepts no arguments")
	}
	executable, err := exec.LookPath("opencode")
	if err != nil {
		return err
	}
	product := opencode.New(os.Getenv(host.SocketEnv), host.LaunchTokenDigest(os.Getenv(host.TokenEnv)), executable)
	worker := sessionkit.NewWorker(product)
	product.SetShutdown(worker.Shutdown)
	product.SetCall(func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		var result json.RawMessage
		err := worker.Call(ctx, method, params, &result)
		return result, err
	})
	return worker.Serve(ctx)
}
