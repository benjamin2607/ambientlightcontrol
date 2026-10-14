#!/usr/bin/env bash
# Regenerates app/Resources/AppIcon.icns from scripts/make-icon.swift.
set -euo pipefail
cd "$(dirname "$0")/.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

swiftc -parse-as-library -o "$work/make-icon" scripts/make-icon.swift
"$work/make-icon" "$work/icon-1024.png"

iconset="$work/AppIcon.iconset"
mkdir "$iconset"
for size in 16 32 128 256 512; do
  sips -z $size $size "$work/icon-1024.png" --out "$iconset/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z $double $double "$work/icon-1024.png" --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done

mkdir -p app/Resources
iconutil -c icns "$iconset" -o app/Resources/AppIcon.icns
echo "Wrote app/Resources/AppIcon.icns"
