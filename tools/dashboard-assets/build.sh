#!/bin/bash
set -euo pipefail

# Asset build pipeline for the motor-autonomo v2 dashboard.
# Usage: ./tools/dashboard-assets/build.sh
# Prereqs: templ binary at ~/go/bin/templ, tailwindcss standalone at this directory.

PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"
TEMPL_BIN="${TEMPL_BIN:-$HOME/go/bin/templ}"
TAILWIND_BIN="${TAILWIND_BIN:-$PROJECT_ROOT/tailwindcss}"
DIST_DIR="$PROJECT_ROOT/../../internal/dashboard/assets/dist"
SRC_DIR="$PROJECT_ROOT/../../internal/dashboard/assets/src"

# Check binaries exist.
[ -x "$TEMPL_BIN" ] || { echo "ERROR: templ binary not found at $TEMPL_BIN"; exit 1; }
[ -x "$TAILWIND_BIN" ] || { echo "ERROR: tailwindcss not found at $TAILWIND_BIN"; exit 1; }

mkdir -p "$DIST_DIR"

# 1) Generate CSS.
"$TAILWIND_BIN" \
  --input "$SRC_DIR/app.css" \
  --output "$DIST_DIR/app.css" \
  --content "$PROJECT_ROOT/../../internal/dashboard/views/*.templ" \
  --minify

echo "CSS built → $DIST_DIR/app.css"

# 2) Copy JS assets (already minified).
cp "$PROJECT_ROOT/htmx.min.js" "$DIST_DIR/"
cp "$PROJECT_ROOT/alpine.min.js" "$DIST_DIR/"
echo "JS copied → $DIST_DIR/"

# 3) Generate Templ Go code.
(cd "$PROJECT_ROOT/../.." && "$TEMPL_BIN" generate)
echo "Templ generated → internal/dashboard/views/"

echo "OK"
