// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/antst/sessionbus-peers/wrappers/claude"
	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/mcp"
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
	// Held lane source only. The Node package owns the interactive executable.
	if !host.LaneMode() {
		if len(arguments) == 1 && arguments[0] == "mcp" && os.Getenv(mcp.LaneSocketEnv) != "" {
			return runMCP(ctx)
		}
		return errors.New("Go Claude interactive path removed; use the @sessionbus/claude interactive package")
	}
	if len(arguments) != 0 {
		return errors.New("lane mode accepts no arguments")
	}
	product := claude.New(os.Getenv(host.SocketEnv))
	worker := sessionkit.NewWorker(product)
	product.SetShutdown(worker.Shutdown)
	product.SetCall(func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		var result json.RawMessage
		err := worker.Call(ctx, method, params, &result)
		return result, err
	})
	return worker.Serve(ctx)
}

func runMCP(ctx context.Context) error {
	if os.Getenv(mcp.LaneSocketEnv) != "" {
		backend, err := mcp.NewLaneBackend()
		if err != nil {
			return err
		}
		return (&mcp.Server{Backend: backend}).Serve(ctx, os.Stdin, os.Stdout)
	}
	return errors.New("held Claude lane MCP endpoint is required")
}
