#!/usr/bin/env bash
# Build the 开心十三张 binaries. Without arguments it compiles the current
# platform into bin/; with GOOS/GOARCH pairs it cross-compiles them all.
set -euo pipefail

cd "$(dirname "$0")"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X main.version=${VERSION}"
mkdir -p bin

build() {
	local goos="$1" goarch="$2" ext=""
	[ "$goos" = "windows" ] && ext=".exe"
	local out="bin/kuaile13-${goos}-${goarch}${ext}"
	echo "building ${out} (${VERSION})"
	GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
		go build -trimpath -ldflags "$LDFLAGS" -o "$out" .
}

if [ "$#" -gt 0 ]; then
	for target in "$@"; do
		build "${target%%-*}" "${target##*-}"
	done
else
	build "$(go env GOOS)" "$(go env GOARCH)"
fi
