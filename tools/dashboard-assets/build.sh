#!/bin/bash
set -euo pipefail

# Asset build pipeline for the motor-autonomo v2 dashboard.
# Usage: ./tools/dashboard-assets/build.sh
# Prereqs: templ binary at $HOME/go/bin/templ, tailwindcss standalone in this directory.
#
# Layout:
#   tools/dashboard-assets/{htmx,alpine}.min.js  — vendored JS, copied as-is
#   tools/dashboard-assets/src/app.css           — Tailwind input
#   internal/dashboard/assets/                   — generated output, embedded via go:embed

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
TEMPL_BIN="${TEMPL_BIN:-$HOME/go/bin/templ}"
TAILWIND_BIN="${TAILWIND_BIN:-$HERE/tailwindcss}"
OUT_DIR="$REPO_ROOT/internal/dashboard/assets"

[ -x "$TEMPL_BIN" ]    || { echo "ERROR: templ binary not found at $TEMPL_BIN"; exit 1; }
[ -x "$TAILWIND_BIN" ] || { echo "ERROR: tailwindcss not found at $TAILWIND_BIN"; exit 1; }

mkdir -p "$OUT_DIR"

# 1) Generate CSS from Tailwind, scanning templ files for class usage.
"$TAILWIND_BIN" \
  --input "$HERE/src/app.css" \
  --output "$OUT_DIR/app.css" \
  --content "$REPO_ROOT/internal/dashboard/views/*.templ" \
  --minify
echo "CSS built → $OUT_DIR/app.css"

# 2) Copy vendored JS (already minified).
cp "$HERE/htmx.min.js" "$HERE/alpine.min.js" "$OUT_DIR/"
echo "JS copied → $OUT_DIR/"

# 3) Generate Go code from templ templates.
(cd "$REPO_ROOT" && "$TEMPL_BIN" generate)
echo "Templ generated → internal/dashboard/views/"

echo "OK"
