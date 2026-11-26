#!/bin/sh
# Prepare the same pinned implementation for both the portable and standalone builds.
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
source_dir=$1
git -c advice.detachedHead=false clone --quiet --depth 1 --branch v0.5.3 https://github.com/fikrilal/agy-auth.git "$source_dir"
cd "$source_dir"
test "$(git rev-parse HEAD)" = c3aefae1ca1deee060100e4c369f7f09d261c74a
git apply "$root/scripts/agy-monitor.patch"
if [ "$(go env GOOS)/$(go env GOARCH)" = "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" ]; then
  CGO_ENABLED=0 go test -mod=readonly ./internal/cli -run '^TestMonitor' >&2
fi
