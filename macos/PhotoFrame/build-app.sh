#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
swift build -c release
app="$PWD/dist/照片相框.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp .build/release/PhotoFrame "$app/Contents/MacOS/PhotoFrame"
cp Info.plist "$app/Contents/Info.plist"
swift tools/make-icon.swift "$app/Contents/Resources/AppIcon.icns"
codesign --force --sign - "$app"
printf '%s\n' "$app"
