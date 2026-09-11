// SPDX-License-Identifier: MIT

package opencode

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/antst/sessionbus-peers/wrappers/host"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

type modelRef struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
}

var serverRules = []host.ArgumentRule{
	{Name: "--agent", TakesValue: true},
	{Name: "--print-logs"},
	{Name: "--log-level", TakesValue: true},
	{Name: "--mdns"},
	{Name: "--mdns-domain", TakesValue: true},
	{Name: "--cors", TakesValue: true},
	{Name: "--hostname", TakesValue: true, ConflictField: "topology"},
	{Name: "--port", TakesValue: true, ConflictField: "topology"},
	{Name: "-m", TakesValue: true, ConflictField: "model"},
	{Name: "--model", TakesValue: true, ConflictField: "model"},
	{Name: "-c", ConflictField: "session_id"},
	{Name: "--continue", ConflictField: "session_id"},
	{Name: "-s", TakesValue: true, ConflictField: "session_id"},
	{Name: "--session", TakesValue: true, ConflictField: "session_id"},
	{Name: "--fork", ConflictField: "session_id"},
	{Name: "--prompt", TakesValue: true, ConflictField: "arguments"},
	{Name: "--auto", ConflictField: "permission_mode"},
	{Name: "--mini", ConflictField: "arguments"},
	{Name: "--no-replay", ConflictField: "arguments"},
	{Name: "--replay-limit", TakesValue: true, ConflictField: "arguments"},
}

func launchArguments(open sessionkit.OpenOptions) ([]string, *modelRef, string, error) {
	if open.PermissionMode != "" && open.PermissionMode != "default" && open.PermissionMode != "bypassPermissions" {
		return nil, nil, "", fmt.Errorf("unsupported value permission_mode=%s", open.PermissionMode)
	}
	if open.ReasoningEffort != "" {
		return nil, nil, "", errors.New("OpenCode does not support reasoning_effort")
	}
	validated, err := host.BuildArguments(open.Arguments, serverRules)
	if err != nil {
		return nil, nil, "", err
	}
	arguments, agent := make([]string, 0, len(validated)), ""
	for index := 0; index < len(validated); index++ {
		argument := validated[index]
		name, value, attached := strings.Cut(argument, "=")
		if name == "--agent" {
			if agent != "" {
				return nil, nil, "", errors.New("--agent accepts one value")
			}
			if !attached {
				index++
				value = validated[index]
			}
			agent = value
			if strings.TrimSpace(agent) == "" || strings.ContainsAny(agent, "\x00\r\n") {
				return nil, nil, "", errors.New("--agent requires a value")
			}
			continue
		}
		arguments = append(arguments, argument)
		if rule := slices.IndexFunc(serverRules, func(rule host.ArgumentRule) bool { return rule.Name == name }); rule >= 0 && serverRules[rule].TakesValue && !attached {
			index++
			arguments = append(arguments, validated[index])
		}
	}
	var model *modelRef
	if open.Model != "" {
		provider, name, found := strings.Cut(open.Model, "/")
		if !found || !nativeValue(provider) || !nativeValue(name) {
			return nil, nil, "", errors.New("model requires one provider/model value")
		}
		model = &modelRef{ID: name, ProviderID: provider}
	}
	return arguments, model, agent, nil
}

func nativeValue(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 256 && !strings.ContainsAny(value, "\x00\r\n")
}

var interactiveValueOptions = []string{"--log-level", "--port", "--hostname", "--mdns-domain", "--cors", "-m", "--model", "-s", "--session", "--prompt", "--agent", "--replay-limit"}
var passthroughCommands = []string{"completion", "acp", "mcp", "attach", "run", "debug", "providers", "agent", "upgrade", "uninstall", "serve", "web", "models", "stats", "export", "import", "github", "pr", "session", "plugin", "db"}

// resumeAlias rewrites the wrapper's --resume <id> and --resume=<id> into
// the native --session forms in place, before the literal "--". The value is
// forwarded verbatim; native OpenCode remains the selection authority.
func resumeAlias(arguments []string) ([]string, error) {
	result := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return append(result, arguments[index:]...), nil
		}
		switch {
		case argument == "--resume":
			if index+1 == len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
				return nil, errors.New("--resume requires a native session ID")
			}
			result, index = append(result, "--session", arguments[index+1]), index+1
			continue
		case strings.HasPrefix(argument, "--resume="):
			_, id, _ := strings.Cut(argument, "=")
			if strings.TrimSpace(id) == "" {
				return nil, errors.New("--resume requires a native session ID")
			}
			result = append(result, "--session="+id)
			continue
		}
		result = append(result, argument)
		if slices.Contains(interactiveValueOptions, argument) && index+1 < len(arguments) {
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
