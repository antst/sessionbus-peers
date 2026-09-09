// SPDX-License-Identifier: MIT

package main

import (
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const legacyPlugin = "agent-sessions@agent-sessions"

func legacyCommand(command string, args []string) bool {
	switch filepath.Base(command) {
	case "agent-sessions", "agentbus", "agentbus-mcp":
		return true
	case "codex-peer", "claude-peer":
		return len(args) > 0 && args[0] == "mcp"
	}
	return false
}

func oldGoBinary(path string) bool {
	b, err := buildinfo.ReadFile(path)
	if err != nil {
		return false
	}
	return b.Main.Path == "github.com/antst/agent-sessions" || b.Main.Path == "github.com/antst/agentbus"
}

func namedManifest(path string) bool {
	d, err := readObject(path)
	return err == nil && string(d["name"]) == `"agent-sessions"`
}

func (c *cleaner) scan() error {
	if err := c.safeParent(filepath.Join(c.config, "placeholder")); err != nil {
		return err
	}
	if err := c.codex(); err != nil {
		return err
	}
	if err := c.claude(); err != nil {
		return err
	}
	if err := c.otherProducts(); err != nil {
		return err
	}
	if err := c.services(); err != nil {
		return err
	}
	legacyRoot := filepath.Join(c.home, ".local/libexec/agent-sessions")
	entries, err := os.ReadDir(filepath.Join(c.home, ".local/bin"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		path := filepath.Join(c.home, ".local/bin", e.Name())
		link, err := os.Readlink(path)
		if err == nil {
			if !filepath.IsAbs(link) {
				link = filepath.Join(filepath.Dir(path), link)
			}
			if within(filepath.Clean(link), legacyRoot) {
				if err := c.move(path, "symlink targets the legacy installation"); err != nil {
					return err
				}
			}
			continue
		}
		if e.Name() == "agentbus" || e.Name() == "agentbus-call" || e.Name() == "agent-sessions" || strings.Contains(e.Name(), "-peer") {
			if oldGoBinary(path) {
				if err := noLiveExecutable(path); err != nil {
					return err
				}
				if err := c.move(path, "Go build metadata identifies Agent Sessions/Agentbus"); err != nil {
					return err
				}
			} else if e.Name() == "agentbus" || e.Name() == "agentbus-call" || strings.Contains(e.Name(), ".pre-") {
				c.notes = append(c.notes, path+" (unrecognized binary; not removed by name alone)")
			}
		}
	}
	if exists(legacyRoot) {
		if !namedManifest(filepath.Join(legacyRoot, ".codex-plugin/plugin.json")) && !oldGoBinary(filepath.Join(legacyRoot, "host/current/bin/agent-sessions")) {
			return fmt.Errorf("legacy-looking root has no recognized manifest or Go identity: %s", legacyRoot)
		}
		if err := noLiveExecutable(legacyRoot); err != nil {
			return err
		}
		if err := c.move(legacyRoot, "recognized legacy payload, including retained development releases"); err != nil {
			return err
		}
	}
	for _, product := range []string{"codex", "claude"} {
		for _, leaf := range []string{"cache/agent-sessions", "marketplaces/agent-sessions"} {
			path := filepath.Join(c.home, "."+product, "plugins", leaf)
			// These exact namespaces were assigned to the old marketplace by its
			// installer. Never touch sessionbus-peers or a product's whole cache.
			if err := c.move(path, "legacy marketplace namespace"); err != nil {
				return err
			}
		}
	}
	if err := c.move(filepath.Join(c.home, ".local/share/agent-sessions/claude-marketplaces"), "old installer-generated Claude marketplaces"); err != nil {
		return err
	}
	// npm global folder installs are symlinks. Removing the module link directly
	// avoids npm uninstall unlinking a claude-peer bin already owned by Go.
	module := filepath.Join(c.home, ".local/lib/node_modules/@sessionbus/claude")
	if exists(module) {
		d, err := readObject(filepath.Join(module, "package.json"))
		if err != nil {
			return err
		}
		if string(d["name"]) != `"@sessionbus/claude"` {
			return fmt.Errorf("unexpected old npm package: %s", module)
		}
		if err := c.move(module, "retired Node Claude package; current public bin preserved"); err != nil {
			return err
		}
	}
	c.notes = append(c.notes, "Sessionbus installations/services, native products, transcripts, source/evidence, and old unified state/config")
	return nil
}

func (c *cleaner) codex() error {
	bin := c.product("codex")
	if bin == "" {
		if exists(filepath.Join(c.home, ".codex/config.toml")) {
			return fmt.Errorf("Codex config exists but codex is unavailable; native removal is required")
		}
		return nil
	}
	config := filepath.Join(c.home, ".codex/config.toml")
	b, err := c.run(bin, "mcp", "list", "--json")
	if err != nil {
		return err
	}
	var servers []struct {
		Name      string `json:"name"`
		Transport struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(b, &servers); err != nil {
		return err
	}
	for _, s := range servers {
		if s.Name != "agent_sessions" && s.Name != "agent-sessions" && s.Name != "agentbus" {
			continue
		}
		if !legacyCommand(s.Transport.Command, s.Transport.Args) {
			return fmt.Errorf("MCP %s has an unrecognized command; review it instead of removing by name", s.Name)
		}
		c.native(bin, []string{"mcp", "remove", s.Name}, config)
	}
	b, err = c.run(bin, "plugin", "list", "--json")
	if err != nil {
		return err
	}
	var plugins struct {
		Installed []struct {
			ID string `json:"pluginId"`
		} `json:"installed"`
	}
	if err := json.Unmarshal(b, &plugins); err != nil {
		return err
	}
	for _, p := range plugins.Installed {
		if p.ID == legacyPlugin {
			c.native(bin, []string{"plugin", "remove", p.ID}, config)
		}
	}
	b, err = c.run(bin, "plugin", "marketplace", "list", "--json")
	if err != nil {
		return err
	}
	var markets struct {
		Marketplaces []struct {
			Name string `json:"name"`
		} `json:"marketplaces"`
	}
	if err := json.Unmarshal(b, &markets); err != nil {
		return err
	}
	for _, m := range markets.Marketplaces {
		if m.Name == "agent-sessions" {
			c.native(bin, []string{"plugin", "marketplace", "remove", m.Name}, config)
		}
	}
	return nil
}

func (c *cleaner) claude() error {
	root := filepath.Join(c.home, ".claude")
	installed := filepath.Join(root, "plugins/installed_plugins.json")
	markets := filepath.Join(root, "plugins/known_marketplaces.json")
	settings := filepath.Join(root, "settings.json")
	d, err := readObject(installed)
	if err != nil {
		return err
	}
	var plugins map[string][]struct {
		Scope string `json:"scope"`
	}
	if d["plugins"] != nil {
		if err := json.Unmarshal(d["plugins"], &plugins); err != nil {
			return err
		}
	}
	bin := c.product("claude")
	for _, p := range plugins[legacyPlugin] {
		if p.Scope != "user" {
			return fmt.Errorf("legacy Claude plugin registered at %s scope; remove that scope first", p.Scope)
		}
		if bin == "" {
			return fmt.Errorf("Claude plugin is installed but native claude is unavailable")
		}
		c.native(bin, []string{"plugin", "uninstall", "--scope", "user", "--keep-data", legacyPlugin}, installed, markets, settings)
		break
	}
	d, err = readObject(markets)
	if err != nil {
		return err
	}
	if d["agent-sessions"] != nil {
		if bin == "" {
			return fmt.Errorf("Claude marketplace is registered but native claude is unavailable")
		}
		c.native(bin, []string{"plugin", "marketplace", "remove", "--scope", "user", "agent-sessions"}, installed, markets, settings)
	}
	if err := c.removeKey(settings, "enabledPlugins", legacyPlugin); err != nil {
		return err
	}
	// Older development installs sometimes used a direct user MCP registration.
	path := filepath.Join(c.home, ".claude.json")
	d, err = readObject(path)
	if err != nil {
		return err
	}
	var servers map[string]struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if d["mcpServers"] != nil {
		if err := json.Unmarshal(d["mcpServers"], &servers); err != nil {
			return err
		}
	}
	for name, s := range servers {
		if (name == "agent_sessions" || name == "agent-sessions") && legacyCommand(s.Command, s.Args) {
			if bin == "" {
				return fmt.Errorf("Claude MCP is registered but native claude is unavailable")
			}
			c.native(bin, []string{"mcp", "remove", "--scope", "user", name}, path)
		}
	}
	return nil
}

func (c *cleaner) otherProducts() error {
	root := filepath.Join(c.home, ".qwen/extensions/agent-sessions")
	if exists(root) {
		if !namedManifest(filepath.Join(root, "plugin.json")) && !namedManifest(filepath.Join(root, "qwen-extension.json")) {
			return fmt.Errorf("unrecognized Qwen extension: %s", root)
		}
		// Qwen owns its asset and enablement/preferences removal. Snapshot the
		// registration files; don't rewrite its product-specific format.
		bin := c.product("qwen")
		if bin == "" {
			return fmt.Errorf("legacy Qwen extension exists but qwen is unavailable")
		}
		c.native(bin, []string{"extensions", "uninstall", "agent-sessions"}, filepath.Join(c.home, ".qwen/extensions/extension-enablement.json"), filepath.Join(c.home, ".qwen/extensions/extension-preferences.json"))
	}
	root = filepath.Join(c.home, ".grok/plugins/agent-sessions")
	if exists(root) {
		if !namedManifest(filepath.Join(root, ".grok-plugin/plugin.json")) {
			return fmt.Errorf("unrecognized Grok plugin: %s", root)
		}
		bin := c.product("grok")
		if bin == "" {
			return fmt.Errorf("legacy Grok plugin exists but grok is unavailable")
		}
		c.native(bin, []string{"plugin", "uninstall", "agent-sessions", "--keep-data"})
	}
	for _, dir := range []string{"plugins", "plugin"} {
		p := filepath.Join(c.config, "opencode", dir, "agent-sessions.js")
		if !exists(p) {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "createLiveSessionClient") || !strings.Contains(string(b), "lane.start") {
			return fmt.Errorf("unrecognized OpenCode plugin: %s", p)
		}
		if err := c.move(p, "legacy auto-loaded OpenCode integration"); err != nil {
			return err
		}
	}
	return nil
}

func (c *cleaner) services() error {
	if runtime.GOOS != "linux" {
		for _, name := range []string{"net.antst.agent-sessions", "net.antst.agent-sessions-hub"} {
			if exists(filepath.Join(c.home, "Library/LaunchAgents", name+".plist")) {
				return fmt.Errorf("unload/remove legacy launchd service %s before cleanup; automated service removal is Linux-only", name)
			}
		}
		return nil
	}
	for _, name := range []string{"agent-sessions", "agent-sessions-hub", "agentbus"} {
		path := filepath.Join(c.config, "systemd/user", name+".service")
		if !exists(path) {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "/agent-sessions") && !strings.Contains(string(b), "/agentbus") {
			return fmt.Errorf("unrecognized legacy-named service: %s", path)
		}
		state, err := c.run("systemctl", "--user", "show", name+".service", "--property=ActiveState", "--value")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(state)) != "inactive" && strings.TrimSpace(string(state)) != "failed" {
			return fmt.Errorf("legacy service %s is %s; finish its sessions and stop it explicitly first", name, strings.TrimSpace(string(state)))
		}
		c.native("systemctl", []string{"--user", "disable", name + ".service"})
		if err := c.move(path, "inactive legacy service"); err != nil {
			return err
		}
		if err := c.move(path+".d", "legacy service overrides"); err != nil {
			return err
		}
		c.native("systemctl", []string{"--user", "daemon-reload"})
	}
	return nil
}

func noLiveExecutable(root string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("cannot prove legacy binaries inactive on this OS: %s (automatic binary retirement is Linux-only)", root)
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, e := range entries {
		path, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe"))
		if err != nil {
			continue
		}
		path = strings.TrimSuffix(path, " (deleted)")
		if within(path, root) {
			return fmt.Errorf("legacy process %s still uses %s; finish it before cleanup", e.Name(), path)
		}
	}
	return nil
}
