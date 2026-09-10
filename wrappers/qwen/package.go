// SPDX-License-Identifier: MIT
package qwen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// InstalledMCPExecutable returns the private alias beside the actual binary,
// even when the public entry was invoked through a login-PATH symlink.
func InstalledMCPExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return installedMCPExecutable(executable)
}

func installedMCPExecutable(executable string) (string, error) {
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	binary, err := os.Stat(executable)
	if err != nil {
		return "", err
	}
	alias := filepath.Join(filepath.Dir(executable), PrivateAlias)
	info, err := os.Stat(alias)
	if err != nil {
		return "", fmt.Errorf("installed Qwen MCP alias: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || !os.SameFile(binary, info) {
		return "", errors.New("installed Qwen MCP alias must name the same executable")
	}
	// Keep the alias basename: it selects the private entry in the same binary.
	return alias, nil
}
