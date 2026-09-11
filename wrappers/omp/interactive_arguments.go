// SPDX-License-Identifier: MIT

package omp

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/antst/sessionbus-peers/wrappers/host"
)

// Native 3b3a6dc cli/flag-tables.ts. Keep one traversal for native value
// ownership and wrapper identity projection so flag-shaped values remain data.
var ompStringValueFlags = map[string]bool{
	"--cwd": true, "--config": true, "--add-dir": true, "--mode": true,
	"--fork": true, "--provider": true, "--model": true, "--smol": true,
	"--slow": true, "--plan": true, "--prewalk-into": true,
	"--plan-yolo-into": true, "--max-time": true, "--service-tier": true,
	"--api-key": true, "--system-prompt": true, "--append-system-prompt": true,
	"--provider-session-id": true, "--prompt-cache-key": true,
	"--session-dir": true, "--models": true, "--tools": true,
	"--thinking": true, "--export": true, "--hook": true,
	"--extension": true, "-e": true, "--trusted-extension": true,
	"--plugin-dir": true, "--skills": true, "--approval-mode": true,
}

var ompOptionalValueFlags = map[string]bool{"--resume": true, "-r": true, "--session": true}

var ompValuelessFlags = map[string]bool{
	"--help": true, "--version": true, "--allow-home": true,
	"--continue": true, "--from-claude": true, "--from-codex": true,
	"--no-session": true, "--no-tools": true, "--no-lsp": true,
	"--no-pty": true, "--hide-thinking": true, "--advisor": true,
	"--external-thinking": true, "--prewalk": true, "--no-prewalk": true,
	"--plan-yolo": true, "--print": true, "--print-thoughts": true,
	"--no-extensions": true, "--no-skills": true, "--no-rules": true,
	"--no-title": true, "--auto-approve": true, "--yolo": true,
}

var ompCommands = map[string]bool{
	"launch": true, "acp": true, "auth-broker": true, "auth-gateway": true,
	"agents": true, "bench": true, "browser-relay": true, "cleanse": true,
	"commit": true, "completions": true, "__complete": true, "compress": true,
	"config": true, "dry-balance": true, "gc": true, "grep": true,
	"gallery": true, "git": true, "grievances": true, "images": true,
	"img": true, "if-bench": true, "install": true, "join": true,
	"models": true, "plugin": true, "ps": true, "say": true, "share": true,
	"setup": true, "shell": true, "read": true, "render": true, "ssh": true,
	"stats": true, "update": true, "usage": true, "tiny-models": true,
	"token": true, "ttsr": true, "worktree": true, "wt": true,
	"search": true, "q": true,
}

var ompReservedWords = map[string]bool{
	"extensions": true, "list": true, "remove": true, "uninstall": true,
	"marketplace": true, "discover": true, "upgrade": true,
	"enable": true, "disable": true,
}

// InteractivePlan keeps native command behavior byte-for-byte on passthrough
// and projects only wrapper group/name flags for a managed terminal launch.
func InteractivePlan(native NativeExecutable, arguments, environment []string) (host.ExecPlan, bool, error) {
	if !filepath.IsAbs(native.RuntimePath) || !filepath.IsAbs(native.EntryPath) ||
		strings.ContainsRune(native.RuntimePath, 0) || strings.ContainsRune(native.EntryPath, 0) {
		return host.ExecPlan{}, false, errors.New("OMP native runtime and entry must be absolute")
	}
	for _, argument := range arguments {
		if strings.ContainsRune(argument, 0) {
			return host.ExecPlan{}, false, errors.New("OMP argument contains NUL")
		}
	}
	if ompNativePassthrough(arguments) {
		return ompNativeExecPlan(native, arguments, environment), true, nil
	}

	forwarded := make([]string, 0, len(arguments)+1)
	forwarded = append(forwarded, native.EntryPath)
	groups := []string{}
	name := ""
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			forwarded = append(forwarded, arguments[index:]...)
			break
		}
		if ompFlagName(argument) == "--trusted-extension" {
			return host.ExecPlan{}, false, errors.New("argument conflicts with managed OMP extension: --trusted-extension")
		}
		if argument == "-g" || argument == "--group" || argument == "-n" || argument == "--peer-name" {
			if index+1 == len(arguments) || arguments[index+1] == "--" || strings.TrimSpace(arguments[index+1]) == "" {
				return host.ExecPlan{}, false, ompWrapperValueError(argument)
			}
			value := arguments[index+1]
			index++
			if argument == "-g" || argument == "--group" {
				groups = append(groups, value)
			} else {
				name = value
			}
			continue
		}
		wrapper, value, attached := ompAttachedWrapperValue(argument)
		if wrapper != "" {
			if !attached || strings.TrimSpace(value) == "" {
				return host.ExecPlan{}, false, ompWrapperValueError(wrapper)
			}
			if wrapper == "-g" || wrapper == "--group" {
				groups = append(groups, value)
			} else {
				name = value
			}
			continue
		}
		if ompConsumesNativeValue(arguments, index) {
			forwarded = append(forwarded, argument, arguments[index+1])
			index++
			continue
		}
		forwarded = append(forwarded, argument)
	}
	encoded, err := json.Marshal(groups)
	if err != nil {
		return host.ExecPlan{}, false, err
	}
	environment = ompSetEnvironment(environment, host.GroupsEnv, string(encoded))
	environment = ompSetEnvironment(environment, host.SessionIDEnv, "")
	environment = ompSetEnvironment(environment, host.NameEnv, name)
	return host.ExecPlan{Path: native.RuntimePath, Args: forwarded, Env: environment}, false, nil
}

func ompNativeExecPlan(native NativeExecutable, arguments, environment []string) host.ExecPlan {
	args := make([]string, 0, len(arguments)+1)
	args = append(args, native.EntryPath)
	args = append(args, arguments...)
	return host.ExecPlan{Path: native.RuntimePath, Args: args, Env: environment}
}

func ompNativePassthrough(arguments []string) bool {
	residual, oneShot := ompProfileResidual(arguments)
	if oneShot || len(residual) == 0 {
		return oneShot
	}
	first := residual[0]
	if first == "--help" || first == "-h" || first == "--version" || first == "-v" ||
		first == "help" || first == "--smoke-test" || first == "--license" || ompReservedWord(residual) {
		return true
	}
	launchArguments := residual
	if ompCommands[first] {
		if first != "launch" {
			return true
		}
		launchArguments = residual[1:]
	} else if index := ompLeadingCommand(residual); index >= 0 {
		if residual[index] != "launch" {
			return true
		}
		launchArguments = append(slices.Clone(residual[:index]), residual[index+1:]...)
	}
	return ompLaunchNonInteractive(launchArguments)
}

// ompProfileResidual mirrors native profile-bootstrap ownership only as far as
// routing needs it. Invalid/missing global values and every alias are one-shot
// native outcomes and therefore passthrough.
func ompProfileResidual(arguments []string) ([]string, bool) {
	residual := make([]string, 0, len(arguments))
	passThrough, sawSubcommand, canDispatch, insertBoundary := false, false, true, false
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if passThrough || sawSubcommand {
			residual = append(residual, argument)
			continue
		}
		if insertBoundary {
			if !strings.HasPrefix(argument, "-") {
				residual = append(residual, ompProfileBoundary)
			}
			insertBoundary = false
		}
		if argument == "--" {
			passThrough = true
			residual = append(residual, argument)
			continue
		}
		name, value, attached := strings.Cut(argument, "=")
		if name == "--profile" || name == "--alias" {
			if attached {
				if value == "" {
					return residual, true
				}
				if name == "--alias" {
					return residual, true
				}
				insertBoundary = ompNeedsProfileBoundary(residual)
				continue
			}
			if index+1 == len(arguments) || strings.HasPrefix(arguments[index+1], "-") || arguments[index+1] == "" {
				return residual, true
			}
			index++
			if name == "--alias" {
				return residual, true
			}
			insertBoundary = ompNeedsProfileBoundary(residual)
			continue
		}
		residual = append(residual, argument)
		if ompProfileConsumesValue(arguments, index) {
			residual = append(residual, arguments[index+1])
			index++
			canDispatch = false
			continue
		}
		if canDispatch && ompCommands[argument] && argument != "launch" && argument != "acp" {
			sawSubcommand = true
		}
		canDispatch = false
	}
	return residual, false
}

func ompLeadingCommand(arguments []string) int {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return -1
		}
		if !strings.HasPrefix(argument, "-") {
			if ompCommands[argument] {
				return index
			}
			return -1
		}
		if ompConsumesNativeValue(arguments, index) {
			index++
		}
	}
	return -1
}

func ompLaunchNonInteractive(arguments []string) bool {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return false
		}
		name, value, attached := strings.Cut(argument, "=")
		switch name {
		case "--help", "-h", "--version", "-v", "--print", "-p":
			return true
		case "--mode":
			if attached || index+1 < len(arguments) {
				return true
			}
		case "--export":
			if attached && value != "" || !attached && index+1 < len(arguments) && arguments[index+1] != "" {
				return true
			}
		}
		if ompConsumesNativeValue(arguments, index) {
			index++
		}
	}
	return false
}

const ompProfileBoundary = "--omp-profile-boundary"

func ompNeedsProfileBoundary(arguments []string) bool {
	if len(arguments) == 0 {
		return false
	}
	previous := arguments[len(arguments)-1]
	name, _, attached := strings.Cut(previous, "=")
	return !attached && (name == "--plan" || ompOptionalValueFlags[name] || ompUnknownLongValue(name))
}

func ompProfileConsumesValue(arguments []string, index int) bool {
	argument := arguments[index]
	name, _, attached := strings.Cut(argument, "=")
	if attached || index+1 >= len(arguments) {
		return false
	}
	next := arguments[index+1]
	if name == "--plan" {
		return !strings.HasPrefix(next, "-")
	}
	if ompStringValueFlags[name] {
		return true
	}
	return (ompOptionalValueFlags[name] && next != "" && !strings.HasPrefix(next, "-")) ||
		(ompUnknownLongValue(name) && !strings.HasPrefix(next, "-"))
}

func ompConsumesNativeValue(arguments []string, index int) bool {
	argument := arguments[index]
	name, _, attached := strings.Cut(argument, "=")
	if attached || index+1 >= len(arguments) {
		return false
	}
	next := arguments[index+1]
	if ompStringValueFlags[name] {
		return true
	}
	return (ompOptionalValueFlags[name] && next != "" && !strings.HasPrefix(next, "-")) ||
		(ompUnknownLongValue(name) && !strings.HasPrefix(next, "-"))
}

func ompUnknownLongValue(name string) bool {
	return strings.HasPrefix(name, "--") && !ompStringValueFlags[name] && !ompOptionalValueFlags[name] && !ompValuelessFlags[name]
}

func ompReservedWord(arguments []string) bool {
	first := arguments[0]
	if !ompReservedWords[first] {
		return false
	}
	if len(arguments) == 1 {
		return true
	}
	if first == "marketplace" && slices.Contains([]string{"add", "remove", "rm", "update", "list"}, arguments[1]) {
		return true
	}
	return slices.ContainsFunc(arguments[1:], func(argument string) bool {
		return !strings.HasPrefix(argument, "-") && strings.Contains(argument, "@")
	})
}

func ompFlagName(argument string) string {
	name, _, _ := strings.Cut(argument, "=")
	return name
}

func ompAttachedWrapperValue(argument string) (string, string, bool) {
	name, value, attached := strings.Cut(argument, "=")
	if name == "-g" || name == "--group" || name == "-n" || name == "--peer-name" {
		return name, value, attached
	}
	return "", "", false
}

func ompWrapperValueError(name string) error {
	if name == "-g" || name == "--group" {
		return errors.New("-g/--group requires a non-empty value")
	}
	return errors.New("-n/--peer-name requires a non-empty value")
}

func ompSetEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	result := slices.DeleteFunc(slices.Clone(environment), func(entry string) bool {
		return strings.HasPrefix(entry, prefix)
	})
	if value != "" {
		result = append(result, prefix+value)
	}
	return result
}
