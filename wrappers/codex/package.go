// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const PluginID = "codex@sessionbus-peers"
const MCPAlias = "codex-peer-mcp"
const BrokerAlias = "codex-peer-broker"
const InstallAlias = "codex-peer-install"

func ActivationArguments() []string {
	return []string{"-c", "features.plugins=true", "-c", `plugins."codex@sessionbus-peers".enabled=true`}
}
func packageRoot() (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(binary)), nil
}

// Native add returns the path; callers never guess a cache/version directory.
func InstalledCodexPlugin() (string, error) {
	root, err := packageRoot()
	if err != nil {
		return "", err
	}
	return installedCodexPlugin(root)
}

func installedCodexPlugin(root string) (string, error) {
	body, err := os.ReadFile(filepath.Join(root, "installed.json"))
	if err != nil {
		return "", fmt.Errorf("Codex plugin installation record: %w", err)
	}
	var record struct{ PluginID, InstalledPath string }
	if json.Unmarshal(body, &record) != nil || record.PluginID != PluginID || !filepath.IsAbs(record.InstalledPath) {
		return "", errors.New("invalid Codex plugin installation record")
	}
	if _, err = os.Stat(filepath.Join(record.InstalledPath, ".codex-plugin", "plugin.json")); err != nil {
		return "", err
	}
	return record.InstalledPath, nil
}

var installNativeCommand = exec.CommandContext

// RegisterPlugin follows the real native add commands, then changes only our
// plugin's enabled key through Codex's own user-layer config editor.
func RegisterPlugin(ctx context.Context, marketplace string, output io.Writer) error {
	root, err := packageRoot()
	if err != nil {
		return err
	}
	return registerPlugin(ctx, root, marketplace, output)
}

func registerPlugin(ctx context.Context, root, marketplace string, output io.Writer) error {
	marketplace, err := filepath.Abs(marketplace)
	if err != nil {
		return err
	}
	config := map[string]any{"mcpServers": map[string]any{"sessionbus": map[string]any{
		"command": filepath.Join(root, "bin", MCPAlias), "env_vars": []string{"SESSIONBUS_GROUPS", "SESSIONBUS_SOCKET", EndpointEnv},
	}}}
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(marketplace, "codex", ".mcp.json"), append(body, '\n'), 0644); err != nil {
		return err
	}
	addMarket := installNativeCommand(ctx, "codex", "plugin", "marketplace", "add", marketplace)
	addMarket.Stdin, addMarket.Stdout, addMarket.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err = addMarket.Run(); err != nil {
		return fmt.Errorf("native marketplace add: %w", err)
	}
	add := installNativeCommand(ctx, "codex", "plugin", "add", PluginID, "--json")
	add.Stdin, add.Stderr = os.Stdin, os.Stderr
	raw, err := add.Output()
	if err != nil {
		return fmt.Errorf("native plugin add: %w", err)
	}
	// Even malformed add output must not leave a successful add globally enabled.
	disabled, disableErr := disableInstalledPlugin(ctx)
	var installed struct{ PluginID, InstalledPath string }
	decodeErr := json.Unmarshal(raw, &installed)
	if decodeErr != nil || installed.PluginID != PluginID || !filepath.IsAbs(installed.InstalledPath) {
		return errors.Join(errors.New("native add did not report the expected installed plugin path"), decodeErr, disableErr)
	}
	if disableErr != nil {
		return disableErr
	}
	record := map[string]any{"pluginId": installed.PluginID, "installedPath": installed.InstalledPath, "marketplace": marketplace, "configWrite": disabled}
	body, err = json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "installed.json"), append(body, '\n'), 0644); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, string(body))
	return err
}

type configWriteResult struct{ Status, FilePath, Version string }

func disableInstalledPlugin(ctx context.Context) (result configWriteResult, err error) {
	command := laneCommand("codex", "app-server", "--stdio")
	command.Stderr = os.Stderr
	child, input, output, err := startNative(command)
	if err != nil {
		return result, err
	}
	stop := context.AfterFunc(ctx, child.abort)
	defer stop()
	app := newAppClient(input, output, nil, nil)
	defer func() {
		closeErr := app.close()
		exitErr := child.Wait()
		<-app.done
		app.mu.Lock()
		drainErr := app.failed
		app.mu.Unlock()
		if errors.Is(drainErr, io.EOF) {
			drainErr = nil
		}
		err = errors.Join(err, closeErr, exitErr, drainErr)
	}()
	if err = app.initialize(ctx, "Sessionbus plugin installer"); err != nil {
		child.abort()
		return result, err
	}
	if err = app.call(ctx, "config/value/write", map[string]any{"keyPath": `plugins."codex@sessionbus-peers".enabled`, "value": false, "mergeStrategy": "replace"}, &result); err != nil {
		child.abort()
		return result, err
	}
	if result.Status != "ok" {
		return result, fmt.Errorf("native plugin disable status %q (may be overridden)", result.Status)
	}
	return result, nil
}
