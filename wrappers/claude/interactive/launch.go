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
		return nil, nil, errors.New("Claude lane mode is unavailable in this interactive candidate")
	}
	groups := []string{}
	if len(args) >= 2 && args[len(args)-2] == "-g" {
		if value := args[len(args)-1]; value != "" {
			groups = strings.Split(value, ",")
		}
		args = args[:len(args)-2]
	}
	encoded, _ := json.Marshal(groups)
	values := make([]string, 0, len(env)+2)
	for k, v := range env {
		if k != "SESSIONBUS_GROUPS" && k != "SESSIONBUS_SOCKET" {
			values = append(values, k+"="+v)
		}
	}
	values = append(values, "SESSIONBUS_GROUPS="+string(encoded), "SESSIONBUS_SOCKET="+SocketPath(env, cwd, uid))
	return append([]string{"--allowedTools", PublicTool, "--plugin-dir", root}, args...), values, nil
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

func Launch(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	root := filepath.Join(filepath.Dir(exe), "plugin")
	if st, err := os.Stat(filepath.Join(root, ".mcp.json")); err != nil || !st.Mode().IsRegular() {
		return errors.New("installed Claude plugin is missing beside the executable")
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
