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
	"--models", "--tools", "-t", "--exclude-tools", "-xt", "--thinking", "--extension", "-e",
	"--skill", "--prompt-template", "--theme", "--use-theme", "--export",
	"--tui-mode",
}

var passthroughCommands = []string{"install", "remove", "uninstall", "update", "list", "config", "auth"}

// InteractivePlan classifies native maintenance/help before adding managed
// identity. Passthrough retains the original argv and environment byte values.
func InteractivePlan(arguments, environment []string) (host.ExecPlan, bool, error) {
	native, err := nativeNonTUI(arguments)
	if err != nil {
		return host.ExecPlan{}, false, err
	}
	if native {
		return host.ExecPlan{Path: "pi", Args: arguments, Env: environment}, true, nil
	}
	plan, _, err := host.ClassifiedInteractivePlan("pi", arguments, environment, host.PeerIdentity{}, func(value string) bool {
		if strings.Contains(value, "=") {
			return false
		}
		return slices.Contains(interactiveValueOptions, value)
	}, nil)
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

func nativeNonTUI(arguments []string) (bool, error) {
	if len(arguments) > 0 && slices.Contains(passthroughCommands, arguments[0]) {
		return true, nil
	}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return false, nil
		}
		name, value, attached := strings.Cut(argument, "=")
		if name == "-g" || name == "--group" || name == "-n" || name == "--peer-name" {
			if attached {
				if strings.TrimSpace(value) == "" {
					return false, wrapperValueError(name)
				}
				continue
			}
			if index+1 == len(arguments) || arguments[index+1] == "--" || strings.TrimSpace(arguments[index+1]) == "" {
				return false, wrapperValueError(name)
			}
			index++
			continue
		}
		switch argument {
		case "-h", "--help", "-v", "--version", "-p", "--print", "--list-models":
			return true, nil
		case "--export":
			// A value is required before Pi selects its one-shot export path.
			if index+1 < len(arguments) {
				return true, nil
			}
		}
		if slices.Contains(interactiveValueOptions, argument) {
			if index+1 < len(arguments) {
				index++
			}
			continue
		}
		// Extension flags are long options. Pi consumes their following plain
		// value, so a word such as "install" there is not a native command.
		if strings.HasPrefix(argument, "--") && !strings.Contains(argument, "=") && index+1 < len(arguments) &&
			!strings.HasPrefix(arguments[index+1], "-") && !strings.HasPrefix(arguments[index+1], "@") {
			index++
		}
	}
	return false, nil
}

func wrapperValueError(name string) error {
	if name == "-g" || name == "--group" {
		return errors.New("-g/--group requires a non-empty value")
	}
	return errors.New("-n/--peer-name requires a non-empty value")
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
