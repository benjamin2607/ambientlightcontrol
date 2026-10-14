#!/usr/bin/env bash
# Development run via `swift run` (no app bundle, so no launch-at-login).
#   --demo   sample data, no server needed
set -euo pipefail
cd "$(dirname "$0")/../app"
if [[ "${1:-}" == "--demo" ]]; then export AMBIENT_DEMO=1; fi
exec swift run AmbientLight
