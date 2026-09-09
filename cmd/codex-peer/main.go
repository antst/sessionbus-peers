// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/antst/sessionbus-peers/wrappers/codex"
	"github.com/antst/sessionbus-peers/wrappers/host"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	if filepath.Base(os.Args[0]) == codex.BrokerAlias {
		if len(arguments) != 0 {
			return errors.New("private broker accepts no arguments")
		}
		return codex.RunInteractiveBroker(ctx)
	}
	if filepath.Base(os.Args[0]) == codex.InstallAlias {
		if len(arguments) != 1 {
			return errors.New("installer requires the permanent marketplace path")
		}
		return codex.RegisterPlugin(ctx, arguments[0], os.Stdout)
	}
	if filepath.Base(os.Args[0]) == codex.MCPAlias {
		if os.Getenv(codex.EndpointEnv) == "" {
			return errors.New("Codex MCP requires its launch endpoint")
		}
		return codex.Forward(ctx, os.Getenv(codex.EndpointEnv), os.Stdin, os.Stdout)
	}
	if !host.LaneMode() {
		return codex.LaunchInteractive(ctx, arguments)
	}
	if len(arguments) != 0 {
		return errors.New("lane mode accepts no arguments")
	}
	product := codex.New()
	worker := sessionkit.NewWorker(product)
	product.SetShutdown(worker.Shutdown)
	product.SetCaller(worker.Caller())
	return worker.Serve(ctx)
}
