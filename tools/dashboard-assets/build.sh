#!/bin/bash
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
TEMPL_BIN="${TEMPL_BIN:-$HOME/go/bin/templ}"
TAILWIND_BIN="${TAILWIND_BIN:-$HERE/tailwindcss}"
OUT_DIR="$REPO_ROOT/internal/dashboard/assets"

[ -x "$TEMPL_BIN" ]    || { echo "ERROR: templ binary not found at $TEMPL_BIN"; exit 1; }
[ -x "$TAILWIND_BIN" ] || { echo "ERROR: tailwindcss not found at $TAILWIND_BIN"; exit 1; }

mkdir -p "$OUT_DIR"

# 1) Generate Go code from templ templates FIRST, so Tailwind can scan the generated Go files if needed,
#    or at least ensure the files are in sync.
(cd "$REPO_ROOT/internal/dashboard" && "$TEMPL_BIN" generate)
echo "Templ generated → internal/dashboard/views/"

# 2) Generate CSS from Tailwind. Tailwind v4 uses @source inside the CSS file, 
#    but we can also try explicit paths from the root.
cd "$REPO_ROOT"
"$TAILWIND_BIN" \
  --input "$HERE/src/app.css" \
  --output "$OUT_DIR/app.css" \
  --minify
echo "CSS built → $OUT_DIR/app.css"

# 3) Copy vendored JS.
cp "$HERE/htmx.min.js" "$HERE/alpine.min.js" "$OUT_DIR/"
echo "JS copied → $OUT_DIR/"

echo "OK"
