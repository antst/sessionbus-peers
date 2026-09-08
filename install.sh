#!/bin/sh
# SPDX-License-Identifier: MIT

set -eu

repository="antst/sessionbus-peers"
binaries="claude-peer codex-peer grok-peer qwen-peer opencode-peer"
selected_products=
selected_binaries=
requested=${SESSIONBUS_PEERS_VERSION:-latest}
temporary=
stage=
replaced=

fail() {
	printf '%s\n' "sessionbus-peers install: $*" >&2
	exit 1
}

if [ "$#" -eq 0 ]; then
	set -- claude codex grok qwen opencode
fi
for product do
	case $product in
		claude) binary=claude-peer ;;
		codex) binary=codex-peer ;;
		grok) binary=grok-peer ;;
		qwen) binary=qwen-peer ;;
		opencode) binary=opencode-peer ;;
		*) fail "unknown product: $product" ;;
	esac
	case " $selected_products " in
		*" $product "*) fail "duplicate product: $product" ;;
	esac
	selected_products="$selected_products $product"
	selected_binaries="$selected_binaries $binary"
done

if [ -n "${SESSIONBUS_PREFIX:-}" ]; then
	prefix=$SESSIONBUS_PREFIX
else
	[ -n "${HOME:-}" ] || fail "HOME is unset and SESSIONBUS_PREFIX was not provided"
	prefix="$HOME/.local/bin"
fi

install_fail() {
	item=$1
	if [ -n "$replaced" ]; then
		printf '%s\n' "sessionbus-peers install: failed replacing $item; replaced: $replaced" >&2
	else
		printf '%s\n' "sessionbus-peers install: failed replacing $item; replaced: none" >&2
	fi
	exit 1
}

cleanup() {
	if [ -n "$stage" ]; then
		rm -f "$stage"
	fi
	if [ -n "$temporary" ]; then
		rm -rf "$temporary"
	fi
}

trap cleanup 0
trap 'exit 1' 1 2 3 15

case $(uname -s 2>/dev/null) in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "unsupported operating system: $(uname -s 2>/dev/null || printf unknown)" ;;
esac

case $(uname -m 2>/dev/null) in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "unsupported architecture: $(uname -m 2>/dev/null || printf unknown)" ;;
esac

[ -n "$prefix" ] || fail "install prefix is empty"
umask 077
temporary=$(mktemp -d "${TMPDIR:-/tmp}/sessionbus-peers.XXXXXX") || fail "cannot create a private temporary directory"

download() {
	url=$1
	destination=$2
	if ! curl -fsSL "$url" -o "$destination"; then
		fail "download failed: $url"
	fi
}

if [ "$requested" = latest ]; then
	metadata="$temporary/latest.json"
	download "https://api.github.com/repos/$repository/releases/latest" "$metadata"
	tag_count=$(sed -n 's/^[[:space:]]*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)"[,[:space:]]*$/\1/p' "$metadata" | wc -l | tr -d ' ')
	[ "$tag_count" = 1 ] || fail "latest release metadata contains no unique tag"
	tag=$(sed -n 's/^[[:space:]]*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)"[,[:space:]]*$/\1/p' "$metadata")
else
	case $requested in
		v*) tag=$requested ;;
		*) tag="v$requested" ;;
	esac
fi

if ! printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$'; then
	fail "malformed version: $requested"
fi
version=${tag#v}
archive="sessionbus-peers_${version}_${os}_${arch}.tar.gz"
release_url="https://github.com/$repository/releases/download/$tag"
archive_path="$temporary/$archive"
sums_path="$temporary/SHA256SUMS"
download "$release_url/$archive" "$archive_path"
download "$release_url/SHA256SUMS" "$sums_path"

checksum=$(awk -v name="$archive" '$2 == name { count++; value = $1; if (NF != 2) invalid = 1 } END { if (count == 1 && !invalid) print value; else exit 1 }' "$sums_path") || fail "SHA256SUMS does not contain exactly one entry for $archive"
if ! printf '%s\n' "$checksum" | grep -Eq '^[0-9A-Fa-f]{64}$'; then
	fail "SHA256SUMS contains an invalid checksum for $archive"
fi
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$archive_path" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$archive_path" | awk '{print $1}')
else
	fail "neither sha256sum nor shasum is available"
fi
expected=$(printf '%s' "$checksum" | tr 'A-F' 'a-f')
actual=$(printf '%s' "$actual" | tr 'A-F' 'a-f')
[ "$actual" = "$expected" ] || fail "checksum mismatch for $archive"

members="$temporary/members"
listing="$temporary/listing"
if ! tar -tzf "$archive_path" >"$members" 2>/dev/null; then
	fail "cannot read $archive"
fi
if ! awk '
	BEGIN {
		allowed["LICENSE"] = 1
		allowed["claude-peer"] = 1
		allowed["codex-peer"] = 1
		allowed["grok-peer"] = 1
		allowed["qwen-peer"] = 1
		allowed["opencode-peer"] = 1
	}
	!($0 in allowed) || seen[$0]++ { invalid = 1 }
	END { if (NR != 6 || invalid) exit 1 }
' "$members"; then
	fail "archive contains an unsafe, duplicate, or unexpected member"
fi
if ! tar -tvzf "$archive_path" >"$listing" 2>/dev/null || ! awk 'substr($0, 1, 1) != "-" { invalid = 1 } END { if (NR != 6 || invalid) exit 1 }' "$listing"; then
	fail "archive contains a non-regular member"
fi

extracted="$temporary/extracted"
mkdir "$extracted" || fail "cannot create the extraction directory"
if ! tar -xzf "$archive_path" -C "$extracted" $selected_binaries; then
	fail "cannot extract $archive"
fi
for binary in $selected_binaries; do
	[ -f "$extracted/$binary" ] && [ ! -L "$extracted/$binary" ] || fail "archive member is not a regular file: $binary"
done

if ! mkdir -p "$prefix"; then
	fail "cannot create install prefix: $prefix"
fi
for binary in $selected_binaries; do
	target="$prefix/$binary"
	[ ! -d "$target" ] || install_fail "$binary"
	stage=$(mktemp "$prefix/.${binary}.sessionbus.XXXXXX") || install_fail "$binary"
	cp "$extracted/$binary" "$stage" || install_fail "$binary"
	chmod 0755 "$stage" || install_fail "$binary"
	mv -f "$stage" "$target" || install_fail "$binary"
	stage=
	if [ -n "$replaced" ]; then
		replaced="$replaced,$binary"
	else
		replaced=$binary
	fi
	printf '%s\n' "installed $target"
done
printf '%s\n' "sessionbus-peers version $tag"
for product in $selected_products; do
	case $product in
		claude) printf '%s\n' 'Claude hookup: claude plugin marketplace add https://github.com/antst/sessionbus-peers.git && claude plugin install sessionbus@sessionbus' ;;
		codex) printf '%s\n' 'Codex hookup: configure the installed codex-peer with scripts/codex-mcp' ;;
		grok) printf '%s\n' 'Grok hookup: install or register the grok/ plugin directory' ;;
		qwen) printf '%s\n' 'Qwen hookup: install or register the qwen/ plugin directory' ;;
		opencode) printf '%s\n' 'OpenCode hookup: install the opencode/ package and run sessionbus-opencode-install' ;;
	esac
done
