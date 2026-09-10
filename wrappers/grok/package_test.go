// SPDX-License-Identifier: MIT
package grok

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPackagedPrivateEntryIsInertAndOnlyGenericSkillIsInstalled(t *testing.T) {
	root, err := filepath.Abs("../..")
	must(t, err)
	stage := t.TempDir()
	build := exec.Command("sh", "scripts/package-product", "grok", stage)
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("package: %v %s", err, out)
	}
	archive := filepath.Join(stage, "grok-peer-"+runtime.GOOS+"-"+runtime.GOARCH+".tar.gz")
	unpack := exec.Command("tar", "-xzf", archive, "-C", stage)
	if out, err := unpack.CombinedOutput(); err != nil {
		t.Fatalf("unpack: %v %s", err, out)
	}
	target, err := os.Readlink(filepath.Join(stage, PrivateAlias))
	must(t, err)
	check(t, target == Product, "alias target=%s", target)
	skills, err := os.ReadDir(filepath.Join(stage, "plugin/skills"))
	must(t, err)
	check(t, len(skills) == 1 && skills[0].Name() == "sessionbus", "active skills=%v", skills)
	// Execute the actual shipped native entry against its permanent-layout alias.
	home := filepath.Join(stage, "home")
	permanent := filepath.Join(home, ".local/libexec/sessionbus/grok")
	must(t, os.MkdirAll(permanent, 0700))
	must(t, os.Symlink(filepath.Join(stage, Product), filepath.Join(permanent, PrivateAlias)))
	cmd := exec.Command(filepath.Join(stage, "plugin/scripts/native-entry"))
	cmd.Env = append(os.Environ(), "HOME="+home, ManagedEnv+"=", "SESSIONBUS_LANE_SOCKET=", "SESSIONBUS_LANE_TOKEN=", "GROK_SESSION_ID=", "GROK_LEADER_SOCKET=")
	in, err := cmd.StdinPipe()
	must(t, err)
	out, err := cmd.StdoutPipe()
	must(t, err)
	cmd.Stderr = os.Stderr
	must(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	enc, dec := json.NewEncoder(in), json.NewDecoder(out)
	for _, request := range []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}},
		{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}},
	} {
		must(t, enc.Encode(request))
		var reply struct {
			ID     int `json:"id"`
			Result struct {
				Tools []json.RawMessage `json:"tools"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		}
		must(t, dec.Decode(&reply))
		check(t, reply.ID == request["id"] && len(reply.Error) == 0, "unexpected reply=%+v", reply)
		if reply.ID == 2 {
			check(t, reply.Result.Tools != nil && len(reply.Result.Tools) == 0, "ordinary tools=%s", reply.Result.Tools)
		}
	}
	must(t, in.Close())
	must(t, cmd.Wait())
}
