// SPDX-License-Identifier: MIT

package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/antst/sessionbus-peers/wrappers/host"
	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

var peerDaemonCommand = exec.CommandContext

// These are native App Server arguments, validated by Codex itself.
func processArguments(arguments []string) ([]string, error) {
	if err := validateManagedConfig(arguments); err != nil {
		return nil, err
	}
	return append([]string(nil), arguments...), nil
}

var managedConfigPaths = [][]string{
	{"features", "plugins"},
	{"plugins", PluginID, "enabled"},
	{"plugins", PluginID, "mcp_servers", "sessionbus", "enabled"},
	{"plugins", PluginID, "mcp_servers", "sessionbus", "default_tools_approval_mode"},
	{"plugins", PluginID, "mcp_servers", "sessionbus", "enabled_tools"},
	{"plugins", PluginID, "mcp_servers", "sessionbus", "disabled_tools"},
	{"plugins", PluginID, "mcp_servers", "sessionbus", "tools", "sessionbus"},
}

// validateManagedConfig rejects only arguments that can disable or replace the
// managed plugin or its Sessionbus tool. Native CLI config keys split on dots
// without unquoting; only their values use TOML syntax.
func validateManagedConfig(arguments []string) error {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return nil
		}
		value, found := "", false
		switch {
		case argument == "-c" || argument == "--config":
			if index+1 < len(arguments) && arguments[index+1] != "--" && !strings.HasPrefix(arguments[index+1], "-") {
				index++
				value, found = arguments[index], true
			}
		case strings.HasPrefix(argument, "--config="):
			value, found = strings.TrimPrefix(argument, "--config="), true
		case strings.HasPrefix(argument, "-c") && len(argument) > len("-c"):
			value, found = strings.TrimPrefix(argument, "-c"), true
			value = strings.TrimPrefix(value, "=")
		case argument == "--disable":
			if index+1 < len(arguments) && arguments[index+1] != "--" {
				index++
				if arguments[index] == "plugins" {
					return errors.New("disabling the plugins feature conflicts with the managed Sessionbus grant")
				}
			}
		case strings.HasPrefix(argument, "--disable="):
			if strings.TrimPrefix(argument, "--disable=") == "plugins" {
				return errors.New("disabling the plugins feature conflicts with the managed Sessionbus grant")
			}
		}
		if !found {
			continue
		}
		path, ok := codexConfigPath(value)
		if !ok {
			continue // Native Codex owns malformed and non-assignment config values.
		}
		for _, managed := range managedConfigPaths {
			if pathPrefix(path, managed) || pathPrefix(managed, path) {
				return fmt.Errorf("configuration %q conflicts with the managed Sessionbus grant", strings.Join(path, "."))
			}
		}
	}
	return nil
}

func pathPrefix(path, target []string) bool {
	if len(path) > len(target) {
		return false
	}
	for index := range path {
		if path[index] != target[index] {
			return false
		}
	}
	return true
}

// codexConfigPath mirrors native CLI override parsing: split once on '=', trim
// the whole key, and split it literally on dots. Quote bytes remain key bytes.
func codexConfigPath(value string) ([]string, bool) {
	key, _, found := strings.Cut(value, "=")
	key = strings.TrimSpace(key)
	if !found || key == "" {
		return nil, false
	}
	return strings.Split(key, "."), true
}
func permission(value string) (string, string, error) { return value, "", nil }

func namePart(name string) (string, error) {
	index := strings.LastIndexByte(name, '@')
	if index < 1 {
		return "", errors.New("Codex lane name is invalid")
	}
	return name[:index], nil
}

func InteractivePlan(arguments, environment []string) (host.ExecPlan, bool, error) {
	remote := false
	plan, passthrough, err := host.ClassifiedInteractivePlan("codex", arguments, environment, host.PeerIdentity{}, func(argument string) bool {
		key, _, attached := strings.Cut(argument, "=")
		if key == "--remote" || key == "--remote-auth-token-env" {
			remote = true
		}
		return !attached && codexOptionTakesValue(key)
	}, func(argument string) bool {
		return argument == "-h" || argument == "--help" || argument == "-V" || argument == "--version" || codexSubcommand(argument)
	})
	if err != nil {
		return host.ExecPlan{}, false, err
	}
	if passthrough {
		return plan, false, nil
	}
	if err := validateManagedConfig(plan.Args); err != nil {
		return host.ExecPlan{}, false, err
	}
	if slices.ContainsFunc(plan.Env, func(value string) bool {
		return strings.HasPrefix(value, host.GroupsEnv+"=") && value != host.GroupsEnv+"=[]"
	}) {
		return host.ExecPlan{}, false, errors.New("Codex peer groups are configured by the installed sessionbus MCP entry; reinstall with --codex-groups")
	}
	if remote {
		return host.ExecPlan{}, false, errors.New("caller-controlled --remote options are not supported")
	}
	if !slices.ContainsFunc(plan.Env, func(value string) bool { return strings.HasPrefix(value, host.SocketEnv+"=") }) {
		plan.Env = append(plan.Env, host.SocketEnv+"="+sessionkit.Socket())
	}
	socket, err := appServerSocket()
	if err != nil {
		return host.ExecPlan{}, false, err
	}
	plan.Args = append([]string{"--remote", "unix://" + socket}, plan.Args...)
	return plan, true, nil
}

func appServerSocket() (string, error) {
	home := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".codex")
	}
	return filepath.Join(home, "app-server-control", "app-server-control.sock"), nil
}

func StartPeerDaemon(ctx context.Context, path string) error {
	command := peerDaemonCommand(ctx, path, "app-server", "daemon", "start")
	command.Env = slices.DeleteFunc(os.Environ(), func(value string) bool {
		key, _, _ := strings.Cut(value, "=")
		return strings.HasPrefix(key, "SESSIONBUS_")
	})
	if err := command.Run(); err != nil {
		return fmt.Errorf("start Codex App Server: %w", err)
	}
	return nil
}

func codexSubcommand(argument string) bool {
	switch argument {
	case "agents", "exec", "e", "review", "login", "logout", "mcp", "plugin", "mcp-server", "app-server", "remote-control", "completion", "update", "doctor", "sandbox", "debug", "apply", "a", "queue", "archive", "delete", "unarchive", "cloud", "app", "exec-server", "features", "help", "migrate-rollouts":
		return true
	}
	return false
}

func codexOptionTakesValue(name string) bool {
	_, found := map[string]bool{
		"-a": true, "--ask-for-approval": true, "-c": true, "--config": true,
		"-C": true, "--cd": true, "--disable": true, "--enable": true,
		"-i": true, "--image": true, "-m": true, "--model": true,
		"--local-provider": true, "-p": true, "--profile": true,
		"--remote": true, "--remote-auth-token-env": true,
		"-s": true, "--sandbox": true, "--add-dir": true,
	}[name]
	return found
}
