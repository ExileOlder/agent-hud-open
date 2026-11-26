#!/usr/bin/env bash
# Build a self-contained delivery archive on the intended x64 platform.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
os="$(go env GOOS)"
arch="$(go env GOARCH)"
if [[ "$arch" != amd64 || ("$os" != linux && "$os" != windows) ]]; then
  echo "Packaging is supported for Ubuntu/X11 and Windows x64." >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
stage="$tmp/$os-$arch"
mkdir -p "$stage" "$root/build"
if [[ "$os" == windows ]]; then
  binary=BalanceHUD.exe
  helper=agy-auth-monitor.exe
  archive=BalanceHUD-windows-x64.zip
  python=python
  cgo=0
  linker="-s -w -H windowsgui"
else
  binary=balance-hud
  helper=agy-auth-monitor
  case "${BALANCE_HUD_LINUX_ARCHIVE:-gzip}" in
    gzip) archive=BalanceHUD-ubuntu-x64.tar.gz ;;
    xz) archive=BalanceHUD-ubuntu-x64.tar.xz ;;
    *) echo "BALANCE_HUD_LINUX_ARCHIVE must be gzip or xz." >&2; exit 2 ;;
  esac
  python=python3
  cgo=1
  linker="-s -w"
fi
# Only this staged build links the helper. Local source builds and explicitly
# selected external monitors keep their original entry points.
sh scripts/prepare-monitor.sh "$tmp/monitor"
cp go.mod "$tmp/bundle.mod"
cat go.sum "$tmp/monitor/go.sum" | sort -u > "$tmp/bundle.sum"
native_tmp=$("$python" - "$tmp" <<'PYBUILD'
import json
from pathlib import Path
import sys

root = Path(sys.argv[1]).resolve()
source = root / "bundled_monitor.go"
source.write_text("""package main
import "github.com/fikrilal/agy-auth/quotahelper"
func init() { bundledMonitor = quotahelper.Run }
""", encoding="utf-8")
overlay = Path.cwd() / "cmd/balance-hud/zz_bundled_monitor.go"
(root / "overlay.json").write_text(
    json.dumps({"Replace": {overlay.as_posix(): source.as_posix()}}),
    encoding="utf-8")
print(root.as_posix())
PYBUILD
)
go mod edit -modfile="$native_tmp/bundle.mod" \
  -require=github.com/fikrilal/agy-auth@v0.5.3 \
  "-replace=github.com/fikrilal/agy-auth=$native_tmp/monitor"
CGO_ENABLED="$cgo" go build -mod=mod -modfile="$native_tmp/bundle.mod" \
  -overlay="$native_tmp/overlay.json" -trimpath -ldflags="$linker" \
  -o "$stage/$binary" ./cmd/balance-hud
if [[ "$os" == windows ]]; then
  CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags="-s -w" \
    -o "$stage/$helper" ./cmd/agy-auth-monitor
else
  cat > "$stage/$helper" <<'SHHELPER'
#!/bin/sh
set -eu
dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
exec "$dir/balance-hud" --antigravity-helper "$@"
SHHELPER
  chmod +x "$stage/$helper"
fi
if [[ "$os/$arch" == "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" ]]; then
  "$python" scripts/check-packaged-monitor.py "$stage/$helper"
fi
cp "$tmp/monitor/LICENSE" "$stage/LICENSE.agy-auth"
cp README.md LICENSE LICENSE-klauspost-compress.txt cmd/balance-hud/artwork/LICENSE-Go-image-font.txt "$stage/"
cp cmd/balance-hud/artwork/NOTICE.md "$stage/ARTWORK-NOTICE.md"

# An explicit allowlist keeps both archives independent of user config,
# build caches and any unrelated files in a previous build directory.
"$python" - "$stage" "$tmp/$archive" "$binary" "$helper" <<'PY'
import pathlib
import sys
import tarfile
import zipfile

stage = pathlib.Path(sys.argv[1])
target = pathlib.Path(sys.argv[2])
files = (*sys.argv[3:], "README.md", "LICENSE", "LICENSE.agy-auth",
         "LICENSE-Go-image-font.txt", "LICENSE-klauspost-compress.txt", "ARTWORK-NOTICE.md")
members = [f"{stage.name}/{name}" for name in files]
if target.suffix == ".zip":
    with zipfile.ZipFile(target, "w", compression=zipfile.ZIP_DEFLATED,
                         compresslevel=9) as bundle:
        for n, m in zip(files, members):
            bundle.write(stage / n, m)
else:
    mode = "w:xz" if target.suffix == ".xz" else "w:gz"
    with tarfile.open(target, mode) as bundle:
        for n, m in zip(files, members):
            bundle.add(stage / n, arcname=m)
PY
mv -f "$tmp/$archive" "$root/build/$archive"
printf 'Package: %s\n' "$root/build/$archive"
