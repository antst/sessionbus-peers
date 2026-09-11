// SPDX-License-Identifier: MIT

package kilo

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// NativeExecutable is a direct native process, never the npm Node launcher.
// Resolution reads metadata only; it does not launch a version probe.
type NativeExecutable struct {
	Path              string
	resourceDirectory string
}

// ResolveNativeExecutable honors Kilo's explicit native override, a direct PATH
// executable, or the recognized npm installation's already-selected cache.
// It deliberately does not implement native CPU/libc package fallback selection.
func ResolveNativeExecutable(frontDoor string) (NativeExecutable, error) {
	if override := os.Getenv("KILO_BIN_PATH"); override != "" {
		path, err := executablePath(override)
		if err != nil {
			return NativeExecutable{}, fmt.Errorf("KILO_BIN_PATH: %w", err)
		}
		if err := nativeShape(path); err != nil {
			return NativeExecutable{}, fmt.Errorf("KILO_BIN_PATH: %w", err)
		}
		// The native shim computes resources from the override as supplied, before
		// spawn performs PATH lookup. Preserve that distinction for a bare override.
		return nativeExecutable(path, filepath.Dir(override)), nil
	}
	path, err := executablePath(frontDoor)
	if err != nil {
		return NativeExecutable{}, err
	}
	if err := nativeShape(path); err == nil {
		return nativeExecutable(path, filepath.Dir(path)), nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return NativeExecutable{}, err
	}
	if filepath.Base(resolved) != "kilo" || filepath.Base(filepath.Dir(resolved)) != "bin" {
		return NativeExecutable{}, errors.New("unsupported Kilo executable layout: use a direct native KILO_BIN_PATH")
	}
	root := filepath.Dir(filepath.Dir(resolved))
	body, err := regularPrefix(filepath.Join(root, "package.json"), 64*1024+1)
	if err != nil || len(body) > 64*1024 {
		return NativeExecutable{}, errors.New("unsupported Kilo npm installation metadata")
	}
	var manifest struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Bin     map[string]string `json:"bin"`
	}
	if json.Unmarshal(body, &manifest) != nil || manifest.Name != "@kilocode/cli" || manifest.Version == "" || (manifest.Bin["kilo"] != "./bin/kilo" && manifest.Bin["kilo"] != "bin/kilo") {
		return NativeExecutable{}, errors.New("unsupported Kilo npm installation metadata")
	}
	cache := filepath.Join(root, "bin", ".kilo")
	// Native postinstall selects this regular file; do not reinterpret a new
	// symlink layout or fall back to another optional platform package ourselves.
	info, err := os.Lstat(cache)
	if err != nil || !info.Mode().IsRegular() {
		return NativeExecutable{}, errors.New("Kilo native bin/.kilo cache is unavailable; supply a direct native KILO_BIN_PATH")
	}
	if err := nativeShape(cache); err != nil {
		return NativeExecutable{}, fmt.Errorf("Kilo native cache: %w", err)
	}
	return nativeExecutable(cache, filepath.Dir(cache)), nil
}

func executablePath(value string) (string, error) {
	path, err := exec.LookPath(value)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func nativeExecutable(path, directory string) NativeExecutable {
	result := NativeExecutable{Path: path}
	resource := filepath.Join(directory, "tree-sitter")
	info, err := os.Stat(filepath.Join(resource, "tree-sitter.wasm"))
	if err == nil && info.Mode().IsRegular() {
		// Environment paths must retain their meaning when the lane changes cwd.
		result.resourceDirectory, _ = filepath.Abs(resource)
	}
	return result
}

// Environment preserves explicit native resource settings. Otherwise it applies
// only the same co-located WASM directory selection as Kilo's npm launcher.
func (n NativeExecutable) Environment(environment []string) []string {
	result := append([]string(nil), environment...)
	if n.resourceDirectory == "" {
		return result
	}
	const prefix = "KILO_TREE_SITTER_WASM_DIR="
	var current string
	for _, entry := range result {
		if strings.HasPrefix(entry, prefix) {
			current = strings.TrimPrefix(entry, prefix)
		}
	}
	if current != "" {
		return result
	}
	filtered := result[:0]
	for _, entry := range result {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+n.resourceDirectory)
}

// Check a bounded native executable header, not a shebang/shim. This establishes
// layout/format, not a version, successful launch, or CPU/libc compatibility.
func nativeShape(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("native executable must be a regular executable file")
	}
	header, err := regularPrefix(path, 64)
	if err != nil {
		return err
	}
	if len(header) < 32 {
		return errors.New("native executable header is truncated")
	}
	if string(header[:4]) == "\x7fELF" {
		if (header[4] != 1 && header[4] != 2) || (header[5] != 1 && header[5] != 2) || header[6] != 1 {
			return errors.New("invalid native ELF header")
		}
		var order binary.ByteOrder = binary.LittleEndian
		if header[5] == 2 {
			order = binary.BigEndian
		}
		if (header[4] == 1 && len(header) < 52) || (header[4] == 2 && len(header) < 64) {
			return errors.New("native ELF header is truncated")
		}
		kind := order.Uint16(header[16:18])
		if kind == 2 || kind == 3 {
			return nil
		}
	}
	switch binary.BigEndian.Uint32(header[:4]) {
	case 0xfeedface, 0xfeedfacf:
		if binary.BigEndian.Uint32(header[12:16]) == 2 {
			return nil
		}
	case 0xcefaedfe, 0xcffaedfe:
		if binary.LittleEndian.Uint32(header[12:16]) == 2 {
			return nil
		}
	case 0xcafebabe, 0xbebafeca, 0xcafebabf, 0xbfbafeca:
		var order binary.ByteOrder = binary.BigEndian
		if binary.BigEndian.Uint32(header[:4]) == 0xbebafeca || binary.BigEndian.Uint32(header[:4]) == 0xbfbafeca {
			order = binary.LittleEndian
		}
		if count := order.Uint32(header[4:8]); count > 0 && count <= 64 {
			return nil
		} // Architecture selection remains the OS loader's job.
	}
	return errors.New("expected a direct ELF or Mach-O native executable, not a script or Node launcher")
}

func regularPrefix(path string, limit int64) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("cannot own native metadata descriptor")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("native metadata descriptor is not a regular file")
	}
	return io.ReadAll(io.LimitReader(file, limit))
}
