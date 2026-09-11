// SPDX-License-Identifier: MIT

package pi

import (
	"errors"
	"slices"
	"strings"

	"github.com/antst/sessionbus-peers/wrappers/host"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

const InteractiveLaunchEnv = "SESSIONBUS_PI_LAUNCH"

// Native d981de1 cli/args.ts and command dispatcher. This list exists only so
// wrapper flags are not parsed out of a native option's following value.
var interactiveValueOptions = []string{
	"--provider", "--model", "--api-key", "--system-prompt", "--append-system-prompt",
	"--mode", "--session", "--session-id", "--fork", "--session-dir", "--name",
	"--models", "--tools", "--exclude-tools", "--thinking", "--extension", "-e",
	"--skill", "--prompt-template", "--theme", "--use-theme", "--export",
}

var passthroughCommands = []string{"install", "remove", "uninstall", "update", "list", "config", "auth"}

// InteractivePlan classifies native maintenance/help before adding managed
// identity. Passthrough retains the original argv and environment byte values.
func InteractivePlan(arguments, environment []string) (host.ExecPlan, bool, error) {
	positional := false
	plan, native, err := host.ClassifiedInteractivePlan("pi", arguments, environment, host.PeerIdentity{}, func(value string) bool {
		if strings.Contains(value, "=") {
			return false
		}
		return slices.Contains(interactiveValueOptions, value)
	}, func(value string) bool {
		if value == "-h" || value == "--help" || value == "-v" || value == "--version" {
			return true
		}
		if strings.HasPrefix(value, "-") || positional {
			return false
		}
		positional = true
		return slices.Contains(passthroughCommands, value)
	})
	if native {
		return host.ExecPlan{Path: plan.Path, Args: arguments, Env: environment}, true, err
	}
	if err != nil {
		return host.ExecPlan{}, false, err
	}
	for index := 0; index < len(plan.Args); index++ {
		argument := plan.Args[index]
		if argument == "--" {
			break
		}
		name, _, attached := strings.Cut(argument, "=")
		if name == "--mode" {
			return host.ExecPlan{}, false, errors.New("argument conflicts with managed Pi topology: --mode")
		}
		if slices.Contains(interactiveValueOptions, name) && !attached {
			index++
		}
	}
	if interactiveEnvironmentValue(plan.Env, host.SocketEnv) == "" {
		plan.Env = setInteractiveEnvironment(plan.Env, host.SocketEnv, sessionkit.Socket())
	}
	return plan, false, nil
}

func interactiveEnvironmentValue(environment []string, name string) string {
	prefix := name + "="
	for index := len(environment) - 1; index >= 0; index-- {
		if strings.HasPrefix(environment[index], prefix) {
			return strings.TrimPrefix(environment[index], prefix)
		}
	}
	return ""
}

func setInteractiveEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	result := slices.DeleteFunc(slices.Clone(environment), func(entry string) bool {
		return strings.HasPrefix(entry, prefix)
	})
	if value != "" {
		result = append(result, prefix+value)
	}
	return result
}
