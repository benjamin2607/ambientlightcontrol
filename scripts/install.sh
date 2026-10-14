#!/usr/bin/env bash
# Builds AmbientLight.app, installs it to /Applications (or ~/Applications) and starts it.
# On first start the app registers itself to launch at login (can be turned off in Settings).
set -euo pipefail
cd "$(dirname "$0")/.."

scripts/build-app.sh

dest="/Applications"
if [[ ! -w "$dest" ]]; then
  dest="$HOME/Applications"
  mkdir -p "$dest"
fi

# Quit a running copy so the new build replaces it cleanly.
osascript -e 'tell application id "io.github.benjamin2607.ambientlight" to quit' >/dev/null 2>&1 || true
sleep 1

rm -rf "$dest/AmbientLight.app"
cp -R build/AmbientLight.app "$dest/"
open "$dest/AmbientLight.app"
echo "Installed to $dest/AmbientLight.app"
