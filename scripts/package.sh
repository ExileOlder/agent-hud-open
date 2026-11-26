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
  CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags="-s -w -H windowsgui" -o "$stage/$binary" ./cmd/balance-hud
else
  binary=balance-hud
  helper=agy-auth-monitor
  archive=BalanceHUD-ubuntu-x64.tar.gz
  python=python3
  CGO_ENABLED=1 go build -mod=readonly -trimpath -ldflags="-s -w" -o "$stage/$binary" ./cmd/balance-hud
fi
BALANCE_HUD_MONITOR_OUT="$stage" bash scripts/build-monitor.sh
cp README.md LICENSE cmd/balance-hud/artwork/LICENSE-Go-image-font.txt "$stage/"
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
         "LICENSE-Go-image-font.txt", "ARTWORK-NOTICE.md")
members = [f"{stage.name}/{name}" for name in files]
if target.suffix == ".zip":
    with zipfile.ZipFile(target, "w", compression=zipfile.ZIP_DEFLATED,
                         compresslevel=9) as bundle:
        for n, m in zip(files, members):
            bundle.write(stage / n, m)
else:
    with tarfile.open(target, "w:gz") as bundle:
        for n, m in zip(files, members):
            bundle.add(stage / n, arcname=m)
PY
mv -f "$tmp/$archive" "$root/build/$archive"
printf 'Package: %s\n' "$root/build/$archive"
