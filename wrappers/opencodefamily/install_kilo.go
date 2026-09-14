// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (kind nativeKind) installPackage() string { return "@sessionbus/" + kind.name() }

func (kind nativeKind) installConfigNames() [][]string {
	if kind == kiloNative {
		return [][]string{{"kilo.jsonc", "kilo.json", "opencode.jsonc", "opencode.json", "config.json"}, {"tui.jsonc", "tui.json"}}
	}
	return [][]string{{"opencode.jsonc", "opencode.json", "config.json"}, {"tui.jsonc", "tui.json"}}
}

// This is the exact retained installed Sessionbus autoload entry, independently
// hash-bound in the Kilo 7.5.16 real-home record and current preflight. A filename
// or an identifying substring alone is not authority to remove a user file.
const kiloLegacyEntrySHA256 = "7c9fefae063f4ead326dfc5db6c0a8bb1a8bd3d09a842a9e287749eaf2eeb8ed"

func readKiloLegacyEntry(directory string) (*configDocument, error) {
	file := filepath.Join(directory, "plugins", "agent-sessions.js")
	entry, err := os.Lstat(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: legacy plugin must be a regular, non-symlink file", file)
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	physical, err := filepath.EvalSymlinks(file)
	if err != nil {
		return nil, err
	}
	physical, err = filepath.Abs(physical)
	if err != nil {
		return nil, err
	}
	// The selected legacy entry must actually reside in this config's plugins
	// directory, not a redirected external autoload tree.
	if physical != filepath.Join(root, "plugins", "agent-sessions.js") {
		return nil, fmt.Errorf("%s: legacy plugin path leaves its owned config location", file)
	}
	body, info, err := readInstallFile(file)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(entry, info) || fmt.Sprintf("%x", sha256.Sum256(body)) != kiloLegacyEntrySHA256 {
		return nil, fmt.Errorf("%s: unrecognized or changed legacy plugin; no files changed", file)
	}
	return &configDocument{kind: kiloNative, file: file, physical: physical, body: body, mode: info.Mode().Perm(), info: info}, nil
}

func validateRemovedConfig(doc *configDocument) error {
	entry, err := os.Lstat(doc.file)
	if err != nil || !entry.Mode().IsRegular() || !os.SameFile(doc.info, entry) {
		return fmt.Errorf("%s: removal target identity changed", doc.file)
	}
	physical, err := filepath.EvalSymlinks(doc.file)
	if err == nil {
		physical, err = filepath.Abs(physical)
	}
	if err != nil || physical != doc.physical {
		return fmt.Errorf("%s: removal target path changed", doc.file)
	}
	body, info, err := readInstallFile(doc.file)
	if err != nil || !os.SameFile(doc.info, info) || sha256.Sum256(body) != sha256.Sum256(doc.body) {
		return fmt.Errorf("%s: removal target content changed", doc.file)
	}
	return nil
}
