#!/bin/sh
# SPDX-License-Identifier: MIT

# Sourced by archive builders. Call peer_version_init from the repository root
# before peer_go_build.
peer_version_init() {
	peer_version_root=$1
	peer_base_version=$(cat "$peer_version_root/RELEASE_VERSION")
	if ! printf '%s\n' "$peer_base_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
		echo "Invalid RELEASE_VERSION: $peer_base_version" >&2
		return 1
	fi
	peer_claude_version=$(awk -F'"' '$2 == "version" { print $4 }' "$peer_version_root/claude/.claude-plugin/plugin.json")
	peer_codex_version=$(awk -F'"' '$2 == "version" { print $4 }' "$peer_version_root/codex/marketplace/codex/.codex-plugin/plugin.json")
	if [ "$peer_claude_version" != "$peer_base_version" ]; then
		echo "Claude manifest version $peer_claude_version does not match RELEASE_VERSION $peer_base_version" >&2
		return 1
	fi
	if [ "$peer_codex_version" != "$peer_base_version-codex.0" ]; then
		echo "Codex base version $peer_codex_version does not match RELEASE_VERSION $peer_base_version" >&2
		return 1
	fi
	peer_release=${SESSIONBUS_PEERS_RELEASE:-development}
	if [ "$peer_release" != development ] && [ "$peer_release" != "v$peer_base_version" ]; then
		echo "Stable peer release $peer_release does not match RELEASE_VERSION v$peer_base_version" >&2
		return 1
	fi
	peer_revision=${SESSIONBUS_PEERS_REVISION:-$(git -C "$peer_version_root" rev-parse HEAD)}
	case "$peer_revision" in
		*[!0-9a-f]*) echo "Invalid peer revision: $peer_revision" >&2; return 1 ;;
	esac
	if [ "${#peer_revision}" -ne 40 ]; then
		echo "Invalid peer revision length: $peer_revision" >&2
		return 1
	fi
}

peer_go_build() {
	peer_build_output=$1
	peer_build_package=$2
	go build -trimpath \
		-ldflags="-s -w -X github.com/antst/sessionbus-peers/internal/peerversion.Release=$peer_release -X github.com/antst/sessionbus-peers/internal/peerversion.Revision=$peer_revision" \
		-o "$peer_build_output" "$peer_build_package"
}
