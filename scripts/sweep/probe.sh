#!/usr/bin/env bash
# Pre-deploy model probe: validates availability of manifest-declared models
# against provider endpoints. Detects 404/410 deprecation, geo-blocks, and
# transport failures BEFORE running a sweep slice.
set -euo pipefail
cd "$(dirname "$0")/../.."
set -a; . ./.provider-secrets.env; set +a
exec python3 "$(dirname "$0")/probe.py"
