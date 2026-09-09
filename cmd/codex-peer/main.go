// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/antst/sessionbus-peers/wrappers/codex"
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
		if len(arguments) == 1 && arguments[0] == "mcp" {
			return runMCP(ctx)
		}
		plan, coordinated, err := codex.InteractivePlan(arguments, os.Environ())
		if err != nil {
			return err
		}
		path, err := exec.LookPath(plan.Path)
		if err != nil {
			return err
		}
		if coordinated {
			if err = codex.StartPeerDaemon(ctx, path); err != nil {
				return err
			}
		}
		return syscall.Exec(path, append([]string{path}, plan.Args...), plan.Env)
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

func runMCP(ctx context.Context) error {
	if os.Getenv(codex.EndpointEnv) != "" {
		backend, err := mcp.NewLaneBackend()
		if err != nil {
			return err
		}
		return (&mcp.Server{Backend: backend}).Serve(ctx, os.Stdin, os.Stdout)
	}
	backend, err := codex.NewPeerBackend(ctx)
	if err != nil {
		return err
	}
	defer backend.Shutdown()
	serveCtx, shutdown := context.WithCancel(ctx)
	defer shutdown()
	backend.SetShutdown(shutdown)
	err = (&mcp.Server{Backend: backend}).Serve(serveCtx, os.Stdin, os.Stdout)
	if serveCtx.Err() != nil && ctx.Err() == nil {
		return nil
	}
	return err
}
