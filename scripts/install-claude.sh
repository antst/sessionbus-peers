#!/bin/sh
# SPDX-License-Identifier: MIT
# Download a checksummed release; the archive owns the installation recipe.
set -eu
role=claude
# Compatibility entrypoint for links published before the repository split.
# New product releases belong to the destination repository shown below.
version=${SESSIONBUS_VERSION:-v0.5.3}
[ "$version" != latest ] || version=v0.5.3
printf '%s\n' 'Legacy claude installer: default pinned to the final combined release v0.5.3. New releases: https://github.com/sessionbus/claude-peer .' >&2
case "$version" in ''|*[!A-Za-z0-9._-]*) echo 'Invalid SESSIONBUS_VERSION' >&2; exit 1;; esac
case "$(uname -s)" in Linux) platform=linux;; Darwin) platform=darwin;; *) echo 'Supported systems: Linux and macOS' >&2; exit 1;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo 'Supported architectures: amd64 and arm64' >&2; exit 1;; esac
for cmd in curl tar awk mktemp; do command -v "$cmd" >/dev/null || { echo "Required command: $cmd" >&2; exit 1; }; done
asset="$role-peer-$platform-$arch.tar.gz"
if [ -n "${SESSIONBUS_DOWNLOAD_ROOT:-}" ]; then
 base=$SESSIONBUS_DOWNLOAD_ROOT

else
 base=https://github.com/sessionbus/codex-peer/releases/download/$version
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' 0
trap 'exit 1' HUP INT TERM
curl -fLsS --retry 2 "$base/$asset" -o "$tmp/$asset"
curl -fLsS --retry 2 "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
expected=$(awk -v file="$asset" '$2 == file {print $1}' "$tmp/SHA256SUMS")
case "$expected" in ''|*[!0-9a-f]*) echo 'Missing or invalid checksum' >&2; exit 1;; esac
[ "${#expected}" -eq 64 ] || { echo 'Ambiguous checksum' >&2; exit 1; }
if command -v sha256sum >/dev/null; then actual=$(sha256sum "$tmp/$asset" | awk '{print $1}'); else actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}'); fi
[ "$actual" = "$expected" ] || { echo 'Release checksum mismatch' >&2; exit 1; }
mkdir "$tmp/payload"
tar -tzf "$tmp/$asset" > "$tmp/members"
if awk '/^\// || /(^|\/)\.\.(\/|$)/ {bad=1} END {exit !bad}' "$tmp/members"; then echo 'Unsafe archive member' >&2; exit 1; fi
tar -xzf "$tmp/$asset" -C "$tmp/payload"
[ "$(cat "$tmp/payload/ROLE")" = "$role" ] || { echo 'Wrong release role' >&2; exit 1; }
sh "$tmp/payload/install" </dev/null
