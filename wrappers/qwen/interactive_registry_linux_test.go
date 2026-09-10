// SPDX-License-Identifier: MIT
package qwen

func setFixtureRegistryProcess(row *nativeRegistry, p nativeProcessIdentity) {
	row.ProcStart = &p.start
	namespace, err := nativePIDNamespace(p.pid)
	if err != nil {
		panic(err)
	}
	row.PIDNS = &namespace
}
