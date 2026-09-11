// SPDX-License-Identifier: MIT

package opencodefamily

import (
	"errors"
	"net/url"
	"path/filepath"
)

// InstallOpenCodePlugin is the same-binary maintenance entry used by the archive's
// installer. It never launches native OpenCode or creates a Sessionbus owner.
func InstallOpenCodePlugin(arguments []string) error {
	return installPluginFor(openCodeNative, arguments)
}

func InstallKiloPlugin(arguments []string) error {
	return installPluginFor(kiloNative, arguments)
}

func installPluginFor(kind nativeKind, arguments []string) error {
	options := InstallOptions{}
	switch {
	case len(arguments) == 1 && arguments[0] == "--remove":
		options.Remove = true
	case len(arguments) == 2 && arguments[0] == "--specifier":
		options.Specifier = arguments[1]
	case len(arguments) == 2 && arguments[0] == "--plugin-dir":
		if !filepath.IsAbs(arguments[1]) {
			return errors.New("--plugin-dir requires the absolute permanent package directory")
		}
		options.Specifier = (&url.URL{Scheme: "file", Path: arguments[1]}).String()
	default:
		return errors.New("usage: " + kind.name() + "-peer --sessionbus-install (--plugin-dir ABSOLUTE_PATH | --specifier SPECIFIER | --remove)")
	}
	_, err := configurePluginFor(kind, options, commitConfigChanges)
	return err
}
