#!/bin/sh
# SPDX-License-Identifier: MIT

set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
temporary=$(mktemp -d "${TMPDIR:-/tmp}/sessionbus-peers-install-test.XXXXXX")
trap 'rm -rf "$temporary"' 0

fail() {
	printf '%s\n' "test-install: $*" >&2
	exit 1
}

case $(uname -s) in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "unsupported test operating system" ;;
esac
case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "unsupported test architecture" ;;
esac

binaries="claude-peer codex-peer grok-peer qwen-peer opencode-peer"
archive="sessionbus-peers_0.5.0_${os}_${arch}.tar.gz"
payload="$temporary/payload"
release="$temporary/release"
fakebin="$temporary/fakebin"
mkdir "$payload" "$release" "$fakebin"
for binary in $binaries; do
	printf '%s\n' '#!/bin/sh' 'printf "%s\n" v0.5.0' >"$payload/$binary"
	chmod 0755 "$payload/$binary"
done
cp "$root/LICENSE" "$payload/LICENSE"
tar -czf "$release/$archive" -C "$payload" LICENSE $binaries
if command -v sha256sum >/dev/null 2>&1; then
	hash=$(sha256sum "$release/$archive" | awk '{print $1}')
else
	hash=$(shasum -a 256 "$release/$archive" | awk '{print $1}')
fi
printf '%s  %s\n' "$hash" "$archive" >"$release/SHA256SUMS"
printf '%s\n' '{' '  "tag_name": "v0.5.0"' '}' >"$release/latest.json"

cat >"$fakebin/curl" <<'EOF'
#!/bin/sh
set -eu
output=
url=
while [ "$#" -gt 0 ]; do
	case $1 in
		-o) output=$2; shift 2 ;;
		-*) shift ;;
		*) url=$1; shift ;;
	esac
done
[ -n "$output" ] && [ -n "$url" ]
printf '%s\n' "$url" >>"$TEST_CURL_LOG"
case $url in
	*/releases/latest) source="$TEST_RELEASE_DIR/latest.json" ;;
	*) source="$TEST_RELEASE_DIR/${url##*/}" ;;
esac
cp "$source" "$output"
EOF
chmod 0755 "$fakebin/curl"

curl_log="$temporary/curl.log"
prefix="$temporary/success/bin"
PATH="$fakebin:$PATH" TEST_RELEASE_DIR="$release" TEST_CURL_LOG="$curl_log" SESSIONBUS_PREFIX="$prefix" sh "$root/install.sh" >"$temporary/success.out"
[ "$(grep -c '/releases/latest$' "$curl_log")" = 1 ] || fail "latest was not resolved exactly once"
[ "$(grep -c '/download/v0.5.0/' "$curl_log")" = 2 ] || fail "assets did not use the one resolved tag"
for binary in $binaries; do
	[ "$("$prefix/$binary" --version)" = v0.5.0 ] || fail "$binary was not installed"
	grep -F "installed $prefix/$binary" "$temporary/success.out" >/dev/null || fail "$binary path was not printed"
done
grep -F 'sessionbus-peers version v0.5.0' "$temporary/success.out" >/dev/null || fail "installed version was not printed"

bad_release="$temporary/bad-release"
bad_prefix="$temporary/bad-prefix"
mkdir "$bad_release" "$bad_prefix"
cp "$release/$archive" "$release/SHA256SUMS" "$release/latest.json" "$bad_release/"
printf 'tampered' >>"$bad_release/$archive"
for binary in $binaries; do
	printf '%s\n' "original-$binary" >"$bad_prefix/$binary"
done
if PATH="$fakebin:$PATH" TEST_RELEASE_DIR="$bad_release" TEST_CURL_LOG="$temporary/bad-curl.log" SESSIONBUS_PEERS_VERSION=v0.5.0 SESSIONBUS_PREFIX="$bad_prefix" sh "$root/install.sh" >"$temporary/bad.out" 2>"$temporary/bad.err"; then
	fail "checksum mismatch was accepted"
fi
grep -F "checksum mismatch for $archive" "$temporary/bad.err" >/dev/null || fail "checksum failure was not truthful"
for binary in $binaries; do
	[ "$(cat "$bad_prefix/$binary")" = "original-$binary" ] || fail "checksum failure replaced $binary"
done

duplicate_release="$temporary/duplicate-release"
mkdir "$duplicate_release"
cp "$release/$archive" "$release/latest.json" "$duplicate_release/"
cat "$release/SHA256SUMS" "$release/SHA256SUMS" >"$duplicate_release/SHA256SUMS"
if PATH="$fakebin:$PATH" TEST_RELEASE_DIR="$duplicate_release" TEST_CURL_LOG="$temporary/duplicate-curl.log" SESSIONBUS_PEERS_VERSION=0.5.0 SESSIONBUS_PREFIX="$temporary/duplicate-prefix" sh "$root/install.sh" >"$temporary/duplicate.out" 2>"$temporary/duplicate.err"; then
	fail "duplicate checksum entry was accepted"
fi
grep -F "SHA256SUMS does not contain exactly one entry for $archive" "$temporary/duplicate.err" >/dev/null || fail "duplicate checksum failure was not truthful"

partial_prefix="$temporary/partial-prefix"
mkdir "$partial_prefix" "$partial_prefix/grok-peer"
for binary in claude-peer codex-peer qwen-peer opencode-peer; do
	printf '%s\n' "original-$binary" >"$partial_prefix/$binary"
done
if PATH="$fakebin:$PATH" TEST_RELEASE_DIR="$release" TEST_CURL_LOG="$temporary/partial-curl.log" SESSIONBUS_PEERS_VERSION=v0.5.0 SESSIONBUS_PREFIX="$partial_prefix" sh "$root/install.sh" >"$temporary/partial.out" 2>"$temporary/partial.err"; then
	fail "partial replacement failure was accepted"
fi
grep -F 'failed replacing grok-peer; replaced: claude-peer,codex-peer' "$temporary/partial.err" >/dev/null || fail "partial replacement report was not exact"
[ "$("$partial_prefix/claude-peer" --version)" = v0.5.0 ] || fail "partial replacement omitted claude-peer"
[ "$("$partial_prefix/codex-peer" --version)" = v0.5.0 ] || fail "partial replacement omitted codex-peer"
for binary in qwen-peer opencode-peer; do
	[ "$(cat "$partial_prefix/$binary")" = "original-$binary" ] || fail "partial failure unexpectedly replaced $binary"
done

before=$(wc -l <"$curl_log" | tr -d ' ')
if PATH="$fakebin:$PATH" TEST_RELEASE_DIR="$release" TEST_CURL_LOG="$curl_log" SESSIONBUS_PEERS_VERSION='v0.5;bad' SESSIONBUS_PREFIX="$temporary/malformed-prefix" sh "$root/install.sh" >"$temporary/malformed.out" 2>"$temporary/malformed.err"; then
	fail "malformed version was accepted"
fi
after=$(wc -l <"$curl_log" | tr -d ' ')
[ "$before" = "$after" ] || fail "malformed version reached the network"
grep -F 'malformed version: v0.5;bad' "$temporary/malformed.err" >/dev/null || fail "malformed version failure was not truthful"

printf '%s\n' "install tests passed"
