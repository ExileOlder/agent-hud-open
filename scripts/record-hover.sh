#!/usr/bin/env bash
# Usage: ESBUILD=esbuild BROWSER=chromium ./scripts/record-hover.sh /path/to/upstream/src
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
assets="$root/cmd/balance-hud/artwork"
cd "$root"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
upstream="$1"
shift
if [[ $# == 0 ]]; then set -- logos hover idle sleep claude-hover claude-idle work claude-work tool compact cue-question cue-permission cue-done; fi
cp -r "$upstream/packages/hud/src/bots" "$tmp/bots"
cp "$root/scripts/record-hover.ts" "$tmp/record-hover.ts"
"${ESBUILD:-esbuild}" "$tmp/record-hover.ts" --bundle "--outfile=$tmp/hover.js"
python3 - "$assets" > "$tmp/hover.html" <<'PY'
import base64, pathlib, sys
root = pathlib.Path(sys.argv[1])
print('<!doctype html><body>')
for name in ("chatgpt", "antigravity", "deepseek", "claude"):
    data = base64.b64encode((root / f"{name}.png").read_bytes()).decode()
    print(f'<img hidden src="data:image/png;base64,{data}">')
print('<script src="hover.js"></script>')
PY
for mode in "$@"; do
  query=""
  providers=3
  case "$mode" in
    logos) query="?logos"; providers=4;;
    hover) :;;
    compact) query="?compact"; providers=3;;
    tool) query="?tool=tool-read"; providers=4;;
    cue-question|cue-permission|cue-done) query="?cue=${mode#cue-}"; providers=4;;
    idle) query="?idle";;
    sleep) query="?sleep"; providers=4;;
    work) query="?work";;
    claude-work) query="?claude&work"; providers=1;;
    claude-hover) query="?claude"; providers=1;;
    claude-idle) query="?claude&idle"; providers=1;;
    *) echo "Unknown animation atlas: $mode" >&2; exit 2;;
  esac
  "${BROWSER:-chromium}" --headless=new --disable-gpu --no-first-run --user-data-dir="$tmp/profile" \
    --virtual-time-budget=30000 --dump-dom "file://$tmp/hover.html$query" > "$tmp/dom.html"
  python3 - "$tmp/dom.html" "$tmp/recorded.png" <<'PY'
import base64, pathlib, re, sys
page = pathlib.Path(sys.argv[1]).read_text()
match = re.search(r'<body>data:image/png;base64,([A-Za-z0-9+/=]+)</body>', page)
if not match:
    sys.exit(page[:1000])
pathlib.Path(sys.argv[2]).write_bytes(base64.b64decode(match[1], validate=True))
PY
  go run ./scripts/compact-atlas.go "$tmp/recorded.png" "$tmp/compact.atlas" "$providers"
  mv -f "$tmp/compact.atlas" "$assets/$mode.atlas"
done
