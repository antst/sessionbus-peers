// SPDX-License-Identifier: MIT

package opencodefamily

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/antst/sessionbus-peers/wrappers/host"
)

const OpenCodeInteractiveLaunchEnv = "SESSIONBUS_OPENCODE_LAUNCH"

type InteractiveLaunchBinding struct {
	Directory string   `json:"directory"`
	PID       int      `json:"pid"`
	Socket    string   `json:"socket"`
	Name      string   `json:"name"`
	Groups    []string `json:"groups"`
}

// RunOpenCodeInteractive retains only direct-child/transient-directory ownership. All
// interactive Peer/Caller state belongs to the native TUI plugin. Abrupt launcher
// death cannot promise native retirement or removal of the launch directory.
func RunOpenCodeInteractive(ctx context.Context, plan host.ExecPlan) (err error) {
	path, err := exec.LookPath(plan.Path)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = ValidateOpenCodeTopology(plan.Args, plan.Env); err != nil {
		return err
	}
	socket := InteractiveEnvironmentValue(plan.Env, host.SocketEnv)
	if !filepath.IsAbs(socket) {
		return errors.New("managed Sessionbus socket must be absolute")
	}
	if InteractiveEnvironmentValue(plan.Env, host.LocalKeyEnv) != "" {
		return errors.New("local key transport is not supported")
	}
	root := filepath.Dir(socket)
	if err = os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	directory, err := os.MkdirTemp(root, "opencode-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(directory)) }()
	if len(filepath.Join(directory, "actions.sock")) >= 104 {
		return errors.New("managed OpenCode endpoint exceeds Unix socket path limit")
	}
	var groups []string
	if err = json.Unmarshal([]byte(InteractiveEnvironmentValue(plan.Env, host.GroupsEnv)), &groups); err != nil {
		return err
	}
	binding, err := json.Marshal(InteractiveLaunchBinding{Directory: directory, PID: os.Getpid(), Socket: socket, Name: InteractiveEnvironmentValue(plan.Env, host.NameEnv), Groups: groups})
	if err != nil {
		return err
	}
	if len(binding) > 64*1024 {
		return errors.New("managed launch metadata exceeds 64 KiB")
	}
	environment := slices.DeleteFunc(slices.Clone(plan.Env), func(entry string) bool { return strings.HasPrefix(entry, "SESSIONBUS_") })
	environment = append(environment, OpenCodeInteractiveLaunchEnv+"="+string(binding))
	if InteractiveEnvironmentValue(environment, "OPENCODE_SERVER_PASSWORD") == "" {
		var secret [32]byte
		if _, err = rand.Read(secret[:]); err != nil {
			return err
		}
		environment = setInteractiveEnvironment(environment, "OPENCODE_SERVER_PASSWORD", hex.EncodeToString(secret[:]))
	}
	// Existing nonempty native credentials remain native settings. Both the TUI
	// process and its native Worker inherit this exact environment.
	args := append([]string{"--hostname=127.0.0.1", "--port=0"}, plan.Args...)
	child := exec.Command(path, args...)
	child.Env = environment
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = child.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err = <-done:
		return err
	case <-ctx.Done():
		stop := child.Process.Signal(syscall.SIGTERM)
		if errors.Is(stop, os.ErrProcessDone) {
			stop = nil
		}
		return errors.Join(stop, <-done)
	}
}
