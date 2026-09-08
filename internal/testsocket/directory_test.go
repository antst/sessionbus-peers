// SPDX-License-Identifier: MIT

package testsocket

import (
	"path/filepath"
	"testing"

	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestDirectoryUsesDefaultSessionSocketParent(t *testing.T) {
	for _, xdg := range []string{"", t.TempDir()} {
		t.Run(map[bool]string{false: "fallback", true: "xdg"}[xdg != ""], func(t *testing.T) {
			t.Setenv("SESSIONBUS_SOCKET", "")
			t.Setenv("XDG_RUNTIME_DIR", xdg)
			directory := Directory(t)
			if filepath.Dir(directory) != filepath.Dir(sessionkit.Socket()) {
				t.Fatalf("test socket parent %q differs from production parent %q", filepath.Dir(directory), filepath.Dir(sessionkit.Socket()))
			}
		})
	}
}
