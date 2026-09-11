// SPDX-License-Identifier: MIT

package opencodefamily

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type stagedConfig struct {
	change              configChange
	temporary, backup   string
	backedUp, installed bool
}

func commitConfigChanges(changes []configChange) error {
	return commitConfigChangesWithHook(changes, nil)
}

// The test hook observes an actual completed rename, never substitutes fake I/O.
func commitConfigChangesWithHook(changes []configChange, afterInstall func(int) error) (result error) {
	var staged []*stagedConfig
	finished := false
	defer func() {
		for i := len(staged) - 1; i >= 0; i-- {
			item := staged[i]
			if !finished {
				switch {
				case item.backedUp:
					if err := os.Rename(item.backup, item.change.doc.physical); err != nil {
						result = errors.Join(result, fmt.Errorf("restore %s (backup retained at %s): %w", item.change.doc.file, item.backup, err))
						item.backup = "" // retain original bytes for diagnosis/recovery
					}
				case item.installed:
					result = errors.Join(result, removeConfigFile(item.change.doc.physical))
				}
			}
			result = errors.Join(result, removeConfigFile(item.temporary), removeConfigFile(item.backup))
		}
	}()
	for _, change := range changes {
		dir := filepath.Dir(change.doc.physical)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		item := &stagedConfig{change: change}
		staged = append(staged, item)
		if !change.removeFile {
			f, err := os.CreateTemp(dir, ".sessionbus-*.new")
			if err != nil {
				return err
			}
			item.temporary = f.Name()
			if err := f.Chmod(change.doc.mode); err != nil {
				return errors.Join(err, f.Close())
			}
			_, writeErr := f.Write(change.body)
			syncErr := f.Sync()
			if err := errors.Join(writeErr, syncErr, f.Close()); err != nil {
				return err
			}
		}
		if change.doc.info != nil {
			backup, err := os.CreateTemp(dir, ".sessionbus-*.old")
			if err != nil {
				return err
			}
			item.backup = backup.Name()
			if err := backup.Close(); err != nil {
				return err
			}
		}
	}
	// Refuse changed snapshots before beginning the multi-file mutation. This is
	// not a claim of locking out native/user writers during subsequent renames.
	for _, item := range staged {
		doc := item.change.doc
		if item.change.removeFile {
			if err := validateRemovedConfig(doc); err != nil {
				return err
			}
		}
		if doc.info == nil {
			if _, err := os.Lstat(doc.physical); !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s: configuration appeared during installation", doc.file)
			}
			continue
		}
		body, info, err := readInstallFile(doc.file)
		if err != nil || !os.SameFile(doc.info, info) || !bytes.Equal(doc.body, body) {
			return fmt.Errorf("%s: configuration changed during installation", doc.file)
		}
	}
	for i, item := range staged {
		if item.change.removeFile {
			if err := validateRemovedConfig(item.change.doc); err != nil {
				return err
			}
		}
		if item.backup != "" {
			if err := os.Rename(item.change.doc.physical, item.backup); err != nil {
				return err
			}
			item.backedUp = true
		}
		if !item.change.removeFile {
			if err := os.Rename(item.temporary, item.change.doc.physical); err != nil {
				return err
			}
		}
		item.installed = true
		if afterInstall != nil {
			if err := afterInstall(i + 1); err != nil {
				return err
			}
		}
	}
	finished = true
	return nil
}

func removeConfigFile(file string) error {
	if file == "" {
		return nil
	}
	err := os.Remove(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
