#!/usr/bin/env bash
# Builds build/AmbientLight.app in release mode.
#   --universal   arm64 + x86_64 binary (needs a full Xcode installation)
set -euo pipefail
cd "$(dirname "$0")/.."

version="$(tr -d '[:space:]' < VERSION)"
build_number="$(git rev-list --count HEAD 2>/dev/null || echo 1)"
swift_flags=(--package-path app -c release)
if [[ "${1:-}" == "--universal" ]]; then
  swift_flags+=(--arch arm64 --arch x86_64)
fi

swift build "${swift_flags[@]}"
bin_dir="$(swift build "${swift_flags[@]}" --show-bin-path)"

app="build/AmbientLight.app"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$bin_dir/AmbientLight" "$app/Contents/MacOS/AmbientLight"
cp app/Resources/AppIcon.icns "$app/Contents/Resources/AppIcon.icns"
sed -e "s/__VERSION__/$version/" -e "s/__BUILD__/$build_number/" app/Resources/Info.plist > "$app/Contents/Info.plist"
plutil -lint -s "$app/Contents/Info.plist"

# Ad-hoc signature with hardened runtime. Not notarized: see README › Installation.
codesign --force --options runtime --timestamp=none --sign - "$app"
codesign --verify --strict "$app"
echo "Built $app ($version, build $build_number)"
