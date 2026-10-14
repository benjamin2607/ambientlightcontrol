#!/usr/bin/env bash
# Runs all tests: Go server and Swift app.
# With only the Command Line Tools installed (no Xcode), Swift Testing needs its framework path passed explicitly.
set -euo pipefail
cd "$(dirname "$0")/.."

(cd server && go vet ./... && go test -race ./...)

clt=/Library/Developer/CommandLineTools/Library/Developer
if [[ "$(xcode-select -p)" == /Library/Developer/CommandLineTools* && -d "$clt/Frameworks/Testing.framework" ]]; then
  swift test --package-path app --disable-xctest \
    -Xswiftc -F -Xswiftc "$clt/Frameworks" \
    -Xlinker -F -Xlinker "$clt/Frameworks" \
    -Xlinker -rpath -Xlinker "$clt/Frameworks" \
    -Xlinker -rpath -Xlinker "$clt/usr/lib"
else
  swift test --package-path app
fi
