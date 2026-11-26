#!/usr/bin/env bash
# Usage: ESBUILD=esbuild BROWSER=chromium ./scripts/record-hover.sh /path/to/upstream/src
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
assets="$root/cmd/balance-hud/artwork"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp -r "$1/packages/hud/src/bots" "$tmp/bots"
cp "$root/scripts/record-hover.ts" "$tmp/record-hover.ts"
marks=$(python3 - "$assets" <<'PY'
import base64, json, pathlib, sys
root = pathlib.Path(sys.argv[1])
print(json.dumps(["data:image/png;base64," + base64.b64encode((root / f"{name}.png").read_bytes()).decode()
                  for name in ("chatgpt", "antigravity", "deepseek")]))
PY
)
"${ESBUILD:-esbuild}" "$tmp/record-hover.ts" --bundle "--define:MARKS=$marks" "--outfile=$tmp/hover.js"
printf '<!doctype html><body><script src="hover.js"></script>\n' > "$tmp/hover.html"
"${BROWSER:-chromium}" --headless=new --disable-gpu --no-first-run --user-data-dir="$tmp/profile" \
  --virtual-time-budget=6000 --dump-dom "file://$tmp/hover.html" > "$tmp/dom.html"
python3 - "$tmp/dom.html" "$assets/hover.png" <<'PY'
import base64, pathlib, re, sys
page = pathlib.Path(sys.argv[1]).read_text()
match = re.search(r'<body>data:image/png;base64,([A-Za-z0-9+/=]+)</body>', page)
if not match:
    sys.exit(page[:1000])
pathlib.Path(sys.argv[2]).write_bytes(base64.b64decode(match[1], validate=True))
PY
