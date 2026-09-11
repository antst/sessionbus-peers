// SPDX-License-Identifier: MIT

package opencode

import (
	"errors"
	"slices"
	"strings"

	"github.com/antst/sessionbus-peers/wrappers/host"
	"github.com/antst/sessionbus-peers/wrappers/opencodefamily"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

var interactiveValueOptions = opencodefamily.OpenCodeInteractiveValueOptions()
var passthroughCommands = []string{"completion", "acp", "mcp", "attach", "run", "debug", "providers", "agent", "upgrade", "uninstall", "serve", "web", "models", "stats", "export", "import", "github", "pr", "session", "plugin", "db"}

// resumeAlias rewrites the wrapper's --resume <id> and --resume=<id> into the
// native two-token "-s <id>" form in place, before the literal "--". Wrapper
// value flags (-g/--group, -n/--peer-name) and native value options keep their
// following token untouched, so a value of "--resume" is never rewritten. The
// ID is forwarded verbatim; native OpenCode remains the selection authority.
func resumeAlias(arguments []string) ([]string, error) {
	result := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return append(result, arguments[index:]...), nil
		}
		switch {
		case argument == "--resume":
			if index+1 == len(arguments) || arguments[index+1] == "--" || strings.TrimSpace(arguments[index+1]) == "" {
				return nil, errors.New("--resume requires a native session ID")
			}
			result, index = append(result, "-s", arguments[index+1]), index+1
			continue
		case strings.HasPrefix(argument, "--resume="):
			_, id, _ := strings.Cut(argument, "=")
			if strings.TrimSpace(id) == "" {
				return nil, errors.New("--resume requires a native session ID")
			}
			result = append(result, "-s", id)
			continue
		}
		result = append(result, argument)
		wrapperValue := argument == "-g" || argument == "--group" || argument == "-n" || argument == "--peer-name"
		if (wrapperValue || slices.Contains(interactiveValueOptions, argument)) && index+1 < len(arguments) {
			index++
			result = append(result, arguments[index])
		}
	}
	return result, nil
}

func InteractivePlan(arguments, environment []string) (host.ExecPlan, bool, error) {
	positional := false
	pureBooleanValue := false
	aliased, aliasErr := resumeAlias(arguments)
	if aliasErr != nil {
		// A native subcommand or help request keeps its argv untouched.
		aliased = arguments
	}
	plan, native, err := host.ClassifiedInteractivePlan("opencode", aliased, environment, host.PeerIdentity{}, func(value string) bool {
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
	if native {
		return host.ExecPlan{Path: plan.Path, Args: arguments, Env: plan.Env}, true, err
	}
	if err != nil {
		return plan, native, err
	}
	if aliasErr != nil {
		return host.ExecPlan{}, false, aliasErr
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
