// SPDX-License-Identifier: MIT

package interactive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestInstalledArchiveExecAndPrivateMCP(t *testing.T) {
	// Resolve the baseline once: macOS /var and /private/var identify the same
	// directory. Production native cwd/argv are never normalized by the test.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err = os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(alias, "output with spaces")
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(repo, "scripts/package-claude"), output)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("archive: %v %s", err, out)
	}
	archive, err := os.Open(filepath.Join(output, "claude-peer-"+runtime.GOOS+"-"+runtime.GOARCH+".tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	gz, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	install := filepath.Join(root, "permanent path fixture")
	if err = os.MkdirAll(install, 0700); err != nil {
		t.Fatal(err)
	}
	r := tar.NewReader(gz)
	var skills []string
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(install, h.Name)
		if strings.Contains(h.Name, "node-reference") || strings.HasSuffix(h.Name, ".mjs") || strings.Contains(h.Name, "node_modules") {
			t.Fatal("Node payload in archive")
		}
		if strings.HasSuffix(h.Name, "SKILL.md") {
			skills = append(skills, h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, os.FileMode(h.Mode))
		case tar.TypeSymlink:
			err = os.Symlink(h.Linkname, path)
		case tar.TypeReg:
			var f *os.File
			f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.FileMode(h.Mode))
			if err == nil {
				_, err = io.Copy(f, r)
				_ = f.Close()
			}
		default:
			t.Fatalf("unexpected archive member %s", h.Name)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(skills, []string{"plugin/skills/sessionbus/SKILL.md"}) {
		t.Fatal(skills)
	}
	binary := filepath.Join(install, "claude-peer")
	private := filepath.Join(install, "plugin/bin/"+PrivateAlias)
	resolved, err := filepath.EvalSymlinks(private)
	if err != nil || resolved != binary {
		t.Fatalf("alias %s %v", resolved, err)
	}
	manifest, err := os.ReadFile(filepath.Join(install, "plugin/.mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifest, []byte("${CLAUDE_PLUGIN_ROOT}/bin/"+PrivateAlias)) {
		t.Fatal("manifest does not invoke private alias")
	}
	pub := filepath.Join(root, "claude-peer")
	if err = os.Symlink(binary, pub); err != nil {
		t.Fatal(err)
	}
	// This is a compiled fixture executable, never a real Claude launch on pdev.
	nativeDir := filepath.Join(root, "native fixture")
	if err = os.Mkdir(nativeDir, 0700); err != nil {
		t.Fatal(err)
	}
	source := `package main
import("encoding/json";"os";"io")
func main(){cwd,_:=os.Getwd();input,_:=io.ReadAll(os.Stdin);_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"pid":os.Getpid(),"args":os.Args[1:],"cwd":cwd,"input":string(input),"groups":os.Getenv("SESSIONBUS_GROUPS")})}`
	helper := filepath.Join(nativeDir, "main.go")
	if err = os.WriteFile(helper, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(nativeDir, "claude"), helper)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture build %v %s", err, out)
	}
	nativeArgs := []string{"-n", "", "--", "--unknown", "-g", "one,, two"}
	cmd = exec.Command(pub, nativeArgs...)
	cmd.Dir = alias
	env := Environment(os.Environ())
	env["PATH"] = nativeDir
	delete(env, "SESSIONBUS_LAUNCH_TOKEN")
	cmd.Env = nil
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdin = strings.NewReader("stdin preserved")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if err = cmd.Wait(); err != nil {
		t.Fatalf("exec %v %s", err, out.String())
	}
	var got struct {
		PID                int
		Args               []string
		Cwd, Input, Groups string
	}
	if err = json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := append([]string{"--allowedTools", PublicTool, "--plugin-dir", filepath.Join(install, "plugin")}, nativeArgs[:len(nativeArgs)-2]...)
	if got.PID != pid || got.Cwd != root || got.Input != "stdin preserved" || got.Groups != `["one",""," two"]` || !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("native fixture %#v want%q", got, want)
	}
	cmd = exec.Command(private)
	cmd.Env = append(cmd.Env, "PATH="+nativeDir)
	cmd.Stdin = strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-03-26\"}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}\n")
	response, err := cmd.CombinedOutput()
	if err != nil || !bytes.Contains(response, []byte(`"name":"sessionbus"`)) {
		t.Fatalf("private MCP %v %s", err, response)
	}
	if err = os.Remove(pub); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(install); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(nativeDir, "claude")); err != nil {
		t.Fatal("removal touched native fixture")
	}
}
