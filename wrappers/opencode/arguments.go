// SPDX-License-Identifier: MIT

package opencode

import (
	"slices"
	"strings"

	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/opencodefamily"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

var interactiveValueOptions = opencodefamily.OpenCodeInteractiveValueOptions()
var passthroughCommands = []string{"completion", "acp", "mcp", "attach", "run", "debug", "providers", "agent", "upgrade", "uninstall", "serve", "web", "models", "stats", "export", "import", "github", "pr", "session", "plugin", "db"}

func InteractivePlan(arguments, environment []string) (host.ExecPlan, bool, error) {
	positional := false
	pureBooleanValue := false
	plan, native, err := host.ClassifiedInteractivePlan("opencode", arguments, environment, host.PeerIdentity{}, func(value string) bool {
		return slices.Contains(interactiveValueOptions, value)
	}, func(value string) bool {
		if pureBooleanValue {
			pureBooleanValue = false
			if value == "true" || value == "false" {
				return false
			}
		}
		if value == "--pure" {
			pureBooleanValue = true
		}
		if value == "-h" || value == "--help" || value == "-v" || value == "--version" {
			return true
		}
		if strings.HasPrefix(value, "-") || positional {
			return false
		}
		positional = true
		return slices.Contains(passthroughCommands, value)
	})
	if err != nil || native {
		return plan, native, err
	}
	if err := validateManagedTopology(plan.Args, plan.Env); err != nil {
		return host.ExecPlan{}, false, err
	}
	plan.Env, err = normalizeManagedIdentity(plan.Env)
	if err != nil {
		return host.ExecPlan{}, false, err
	}
	if !slices.ContainsFunc(plan.Env, func(value string) bool { return strings.HasPrefix(value, host.SocketEnv+"=") }) {
		plan.Env = append(plan.Env, host.SocketEnv+"="+sessionkit.Socket())
	}
	return plan, false, nil
}
