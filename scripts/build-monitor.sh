#!/bin/sh
# Assemble the separately licensed Antigravity quota helper from pinned source.
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
git -c advice.detachedHead=false clone --quiet --depth 1 --branch v0.5.3 https://github.com/fikrilal/agy-auth.git "$tmp/source"
cd "$tmp/source"
test "$(git rev-parse HEAD)" = c3aefae1ca1deee060100e4c369f7f09d261c74a
git apply --check "$root/scripts/agy-monitor.patch"
git apply "$root/scripts/agy-monitor.patch"
os=${GOOS:-$(go env GOOS)}
arch=${GOARCH:-$(go env GOARCH)}
out="${BALANCE_HUD_MONITOR_OUT:-$root/build/$os-$arch}"
mkdir -p "$out"
name=agy-auth-monitor
if [ "$os" = windows ]; then name="$name.exe"; fi
CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -mod=readonly -trimpath -ldflags="-s -w" -o "$out/$name" ./cmd/agy-auth
cp LICENSE "$out/LICENSE.agy-auth"
printf '%s\n' "$out/$name"
