#!/bin/sh
# Assemble the separately licensed Antigravity quota helper from pinned source.
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
sh "$root/scripts/prepare-monitor.sh" "$tmp/source"
cd "$tmp/source"
os=${GOOS:-$(go env GOOS)}
arch=${GOARCH:-$(go env GOARCH)}
out="${BALANCE_HUD_MONITOR_OUT:-$root/build/$os-$arch}"
mkdir -p "$out"
name=agy-auth-monitor
if [ "$os" = windows ]; then name="$name.exe"; fi
CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -mod=readonly -trimpath -ldflags="-s -w" -o "$out/$name" ./cmd/agy-auth-monitor
cp LICENSE "$out/LICENSE.agy-auth"
printf '%s\n' "$out/$name"
