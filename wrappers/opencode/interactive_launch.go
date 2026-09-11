// SPDX-License-Identifier: MIT

package opencode

import (
	"context"
	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/opencodefamily"
)

const InteractiveLaunchEnv = opencodefamily.OpenCodeInteractiveLaunchEnv

type interactiveLaunchBinding = opencodefamily.InteractiveLaunchBinding

// RunInteractive keeps the existing OpenCode entry over the shared native lifetime.
func RunInteractive(ctx context.Context, plan host.ExecPlan) error {
	return opencodefamily.RunOpenCodeInteractive(ctx, plan)
}
