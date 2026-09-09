// SPDX-License-Identifier: MIT

// cleanup-legacy retires recognized Agent Sessions installations, not Sessionbus.
// It intentionally has no product runtime or third-party dependencies.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type action struct {
	Description string `json:"description"`
	do          func() error
}

type cleaner struct {
	home, config, backup string
	actions              []action
	notes                []string
	run                  func(string, ...string) ([]byte, error)
	out                  io.Writer
}

func command(name string, args ...string) ([]byte, error) {
	b, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w: %s", name, args, err, b)
	}
	return b, nil
}

func main() {
	apply := flag.Bool("apply", false, "apply the printed cleanup; default is inspection only")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: cleanup-legacy [--apply]")
		os.Exit(2)
	}
	home, err := os.UserHomeDir()
	if err == nil {
		home, err = filepath.EvalSymlinks(home)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	// Product-specific alternate homes are not silently substituted for the real
	// user installation. They need their own explicitly reviewed migration.
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR"} {
		if os.Getenv(key) != "" {
			fmt.Fprintf(os.Stderr, "unset %s before cleaning the real user installation\n", key)
			os.Exit(1)
		}
	}
	c := &cleaner{home: home, config: config, run: command, out: os.Stdout}
	if err := c.scan(); err != nil {
		fmt.Fprintln(os.Stderr, "No cleanup performed:", err)
		os.Exit(1)
	}
	if err := c.execute(*apply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (c *cleaner) execute(apply bool) error {
	for _, note := range c.notes {
		fmt.Fprintln(c.out, "KEEP:", note)
	}
	for _, a := range c.actions {
		fmt.Fprintln(c.out, "CLEAN:", a.Description)
	}
	if !apply || len(c.actions) == 0 {
		fmt.Fprintf(c.out, "%d cleanup actions. No changes made. Use --apply to execute.\n", len(c.actions))
		return nil
	}
	root := filepath.Join(c.home, ".local/state/sessionbus/legacy-cleanup")
	if err := c.safeParent(filepath.Join(root, "placeholder")); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	var err error
	c.backup, err = os.MkdirTemp(root, time.Now().UTC().Format("20060102T150405Z-"))
	if err != nil {
		return err
	}
	plan, err := json.MarshalIndent(c.actions, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.backup, "plan.json"), plan, 0600); err != nil {
		return err
	}
	for i, a := range c.actions {
		if err := a.do(); err != nil {
			return fmt.Errorf("cleanup stopped at %q: %w; earlier changes and backups: %s", a.Description, err, c.backup)
		}
		if err := os.WriteFile(filepath.Join(c.backup, "completed.txt"), []byte(fmt.Sprint(i+1)), 0600); err != nil {
			return err
		}
	}
	fmt.Fprintln(c.out, "Cleanup complete. Files and config snapshots:", c.backup)
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// A leaf symlink may be quarantined, but its parents may not redirect a
// destructive operation outside the inspected installation.
func (c *cleaner) safeParent(path string) error {
	if !within(path, c.home) || path == c.home {
		return fmt.Errorf("path outside the user home: %s", path)
	}
	for p := filepath.Dir(path); p != c.home; p = filepath.Dir(p) {
		st, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("indirect installation directory requires manual review: %s", p)
		}
	}
	return nil
}

func (c *cleaner) move(path, reason string) error {
	if !exists(path) {
		return nil
	}
	if err := c.safeParent(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	link, _ := os.Readlink(path)
	c.actions = append(c.actions, action{Description: "quarantine " + path + " (" + reason + ")", do: func() error {
		now, err := os.Lstat(path)
		if os.IsNotExist(err) { // A native uninstall may already have removed it.
			return nil
		}
		if err != nil {
			return err
		}
		currentLink, _ := os.Readlink(path)
		if !os.SameFile(info, now) || currentLink != link {
			return fmt.Errorf("installation changed since inspection: %s", path)
		}
		if err := c.safeParent(path); err != nil {
			return err
		}
		rel, _ := filepath.Rel(c.home, path)
		target := filepath.Join(c.backup, "files", rel)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		return os.Rename(path, target)
	}})
	return nil
}

func readObject(path string) (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	var d map[string]json.RawMessage
	if err := json.Unmarshal(b, &d); err != nil || d == nil {
		return nil, fmt.Errorf("not a JSON object: %s", path)
	}
	return d, nil
}

func (c *cleaner) snapshot(path string) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(c.home, path)
	if err != nil || !within(path, c.home) {
		return fmt.Errorf("snapshot outside home: %s", path)
	}
	target := filepath.Join(c.backup, "config", rel)
	if exists(target) {
		return nil // Keep the original, not an intermediate native edit.
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	return os.WriteFile(target, b, 0600)
}

func (c *cleaner) removeKey(path, object, key string) error {
	d, err := readObject(path)
	if err != nil {
		return err
	}
	var inner map[string]json.RawMessage
	if b := d[object]; b != nil {
		if err := json.Unmarshal(b, &inner); err != nil {
			return err
		}
	}
	if _, ok := inner[key]; !ok {
		return nil
	}
	c.actions = append(c.actions, action{Description: "remove " + object + "." + key + " from " + path, do: func() error {
		// Reread after native uninstall; never write an old whole-config snapshot.
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		if err := c.safeParent(realPath); err != nil {
			return err
		}
		before, err := os.ReadFile(realPath)
		if err != nil {
			return err
		}
		d, err := readObject(realPath)
		if err != nil {
			return err
		}
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(d[object], &inner); err != nil {
			if d[object] == nil {
				return nil
			}
			return err
		}
		if _, ok := inner[key]; !ok {
			return nil
		}
		delete(inner, key)
		d[object], err = json.Marshal(inner)
		if err != nil {
			return err
		}
		after, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			return err
		}
		if err := c.snapshot(path); err != nil {
			return err
		}
		st, err := os.Stat(realPath)
		if err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(realPath), ".legacy-cleanup-")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		err = errors.Join(f.Chmod(st.Mode().Perm()), writeAll(f, append(after, '\n')))
		err = errors.Join(err, f.Close())
		if err != nil {
			return err
		}
		current, err := os.ReadFile(realPath)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, before) {
			return fmt.Errorf("configuration changed during cleanup: %s", path)
		}
		return os.Rename(f.Name(), realPath)
	}})
	return nil
}

func writeAll(w io.Writer, b []byte) error { _, err := w.Write(b); return err }

func (c *cleaner) native(bin string, args []string, configs ...string) {
	c.actions = append(c.actions, action{Description: bin + " " + strings.Join(args, " "), do: func() error {
		for _, path := range configs {
			if err := c.snapshot(path); err != nil {
				return err
			}
		}
		b, err := c.run(bin, args...)
		if err != nil {
			return err
		}
		fmt.Fprint(c.out, string(b))
		return nil
	}})
}

func (c *cleaner) product(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	p := filepath.Join(c.home, ".local/bin", name)
	if st, err := os.Stat(p); err == nil && st.Mode().Perm()&0111 != 0 {
		return p
	}
	return ""
}
