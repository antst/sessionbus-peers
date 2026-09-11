// SPDX-License-Identifier: MIT
package kilo

import "github.com/antst/sessionbus-peers/wrappers/opencodefamily"

type InstallOptions = opencodefamily.InstallOptions

func ConfigurePlugin(options InstallOptions) (bool, error) {
	return opencodefamily.ConfigureKiloPlugin(options)
}

func InstallPlugin(arguments []string) error {
	return opencodefamily.InstallKiloPlugin(arguments)
}
