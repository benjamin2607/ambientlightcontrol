#!/usr/bin/env bash
# Runs the server locally for development. Reads GOVEE_API_KEY from .env and keeps its state in .data/.
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi
export CONFIG_DIR="${CONFIG_DIR:-$PWD/.data}"
export LISTEN_ADDR="${LISTEN_ADDR:-127.0.0.1:8787}"
cd server
exec go run -ldflags "-X main.version=$(tr -d '[:space:]' < ../VERSION)-dev" ./cmd/ambientlightd
