// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/mcp"
	"github.com/antst/sessionbus-peers/wrappers/qwen"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runEntry(ctx, filepath.Base(os.Args[0]), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runEntry(ctx context.Context, basename string, arguments []string) error {
	if basename == qwen.PrivateAlias {
		if len(arguments) != 0 {
			return errors.New("private MCP entry accepts no arguments")
		}
		endpoint := os.Getenv(qwen.LaneEndpointEnv)
		if endpoint == "" {
			return errors.New("Qwen lane MCP endpoint is missing")
		}
		return qwen.ForwardLaneMCP(ctx, endpoint, os.Stdin, os.Stdout)
	}
	return run(ctx, arguments)
}

func run(ctx context.Context, arguments []string) error {
	if !host.LaneMode() {
		if len(arguments) == 1 && arguments[0] == "mcp" {
			return runMCP(ctx)
		}
		plan, err := qwen.InteractivePlan(arguments, os.Environ())
		if err != nil {
			return err
		}
		path, err := exec.LookPath(plan.Path)
		if err != nil {
			return err
		}
		return syscall.Exec(path, append([]string{path}, plan.Args...), plan.Env)
	}
	if len(arguments) != 0 {
		return errors.New("lane mode accepts no arguments")
	}
	product := qwen.New(os.Getenv(host.SocketEnv))
	worker := sessionkit.NewWorker(product)
	product.SetShutdown(worker.Shutdown)
	product.SetCaller(worker.Caller())
	return worker.Serve(ctx)
}

func runMCP(ctx context.Context) error {
	return serveMCP(ctx, os.Stdin, os.Stdout)
}

func serveMCP(ctx context.Context, input io.Reader, output io.Writer) error {
	if os.Getenv(mcp.LaneSocketEnv) != "" {
		backend, err := mcp.NewLaneBackend()
		if err != nil {
			return err
		}
		return (&mcp.Server{Backend: backend}).Serve(ctx, input, output)
	}
	backend := qwen.NewPeerBackend()
	if err := backend.Start(); err != nil {
		return err
	}
	defer backend.Shutdown()
	return (&mcp.Server{Backend: backend}).Serve(ctx, input, output)
}
