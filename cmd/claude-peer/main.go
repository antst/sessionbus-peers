// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/antst/sessionbus-peers/internal/peerversion"
	"github.com/antst/sessionbus-peers/wrappers/claude"
	"github.com/antst/sessionbus-peers/wrappers/claude/interactive"
	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func main() {
	arguments, report, handled := peerversion.Resolve("claude-peer", os.Args[0], os.Args[1:])
	if handled {
		fmt.Fprintln(os.Stdout, report)
		return
	}
	if err := run(arguments); err != nil {
		fmt.Fprintln(os.Stderr, "claude-peer:", err)
		os.Exit(1)
	}
}
func run(arguments []string) error {
	ctx := context.Background()
	switch filepath.Base(os.Args[0]) {
	case interactive.PrivateAlias:
		if endpoint := os.Getenv(claude.LaneEndpointEnv); endpoint != "" {
			return claude.Forward(ctx, endpoint, os.Stdin, os.Stdout)
		}
		owner, err := interactive.NewOwner(interactive.Environment(os.Environ()))
		if err != nil {
			return err
		}
		owner.StartStartupObservation(interactive.Environment(os.Environ()))
		return interactive.Serve(owner, os.Stdin, os.Stdout)
	case claude.HookAlias:
		endpoint := os.Getenv(claude.LaneEndpointEnv)
		if endpoint == "" {
			return errors.New("lane report endpoint is required")
		}
		return claude.InitialReport(ctx, endpoint, os.Getenv("CLAUDE_PID"), os.Stdin)
	}
	if _, workerMode := os.LookupEnv("SESSIONBUS_LAUNCH_TOKEN"); !workerMode {
		return interactive.Launch(arguments)
	}
	if len(arguments) != 0 {
		return errors.New("lane worker arguments belong in session.open")
	}
	root, err := interactive.InstalledRoot()
	if err != nil {
		return err
	}
	product := claude.New(root)
	worker := kit.NewWorker(product)
	product.SetCaller(worker.Caller())
	product.SetShutdown(worker.Shutdown)
	defer product.Close(context.Background(), kit.SessionCloseRequest{})
	return worker.Serve(ctx)
}
