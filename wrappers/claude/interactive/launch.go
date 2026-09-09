// SPDX-License-Identifier: MIT

package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const PrivateAlias = "sessionbus-mcp"
const LaneEndpointEnv = "SESSIONBUS_CLAUDE_ENDPOINT"
const PublicTool = "mcp__plugin_sessionbus_sessionbus__sessionbus"

func SocketPath(env map[string]string, cwd string, uid int) string {
	p := env["SESSIONBUS_SOCKET"]
	if p == "" {
		if root := env["XDG_RUNTIME_DIR"]; root != "" {
			p = filepath.Join(root, "sessionbus/presence.sock")
		} else {
			p = fmt.Sprintf("/tmp/sessionbus-%d/presence.sock", uid)
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p)
}

func Environment(values []string) map[string]string {
	env := make(map[string]string, len(values))
	for _, value := range values {
		k, v, ok := strings.Cut(value, "=")
		if ok {
			env[k] = v
		}
	}
	return env
}

func LaunchPlan(args []string, env map[string]string, cwd, root string, uid int) ([]string, []string, error) {
	if env["SESSIONBUS_LAUNCH_TOKEN"] != "" {
		return nil, nil, errors.New("interactive entry cannot consume a lane launch token")
	}
	groups := []string{}
	native := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--":
			native = append(native, args[i:]...)
			i = len(args)
		case "-g":
			if i+1 == len(args) || args[i+1] == "--" {
				return nil, nil, errors.New("-g requires a group list")
			}
			i++
			if args[i] != "" {
				groups = append(groups, strings.Split(args[i], ",")...)
			}
		case "--yolo":
			native = append(native, "--dangerously-skip-permissions")
		default:
			native = append(native, args[i])
		}
	}
	encoded, _ := json.Marshal(groups)
	values := make([]string, 0, len(env)+2)
	for k, v := range env {
		if k != "SESSIONBUS_GROUPS" && k != "SESSIONBUS_SOCKET" && k != LaneEndpointEnv {
			values = append(values, k+"="+v)
		}
	}
	values = append(values, "SESSIONBUS_GROUPS="+string(encoded), "SESSIONBUS_SOCKET="+SocketPath(env, cwd, uid))
	return append([]string{"--allowedTools", PublicTool, "--plugin-dir", root}, native...), values, nil
}

// Preserve native PATH lookup, including relative/empty entries, without Go's
// exec.ErrDot policy changing the reviewed launch semantics.
func NativePath(env map[string]string, cwd string) (string, error) {
	for _, dir := range strings.Split(env["PATH"], string(os.PathListSeparator)) {
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
		p := filepath.Join(dir, "claude")
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && syscall.Access(p, 1) == nil {
			return p, nil
		}
	}
	return "", errors.New("claude executable was not found on PATH")
}

func InstalledRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	root := filepath.Join(filepath.Dir(exe), "plugin")
	if st, err := os.Stat(filepath.Join(root, ".mcp.json")); err != nil || !st.Mode().IsRegular() {
		return "", errors.New("installed Claude plugin is missing beside the executable")
	}
	return root, nil
}

func Launch(args []string) error {
	root, err := InstalledRoot()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	env := Environment(os.Environ())
	argv, values, err := LaunchPlan(args, env, cwd, root, os.Getuid())
	if err != nil {
		return err
	}
	native, err := NativePath(env, cwd)
	if err != nil {
		return err
	}
	return syscall.Exec(native, append([]string{native}, argv...), values)
}
