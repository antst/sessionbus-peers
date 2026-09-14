// SPDX-License-Identifier: MIT
package qwen

func setFixtureRegistryProcess(row *nativeRegistry, _ nativeProcessIdentity) {
	row.ProcStart = nil
	row.PIDNS = nil
}
