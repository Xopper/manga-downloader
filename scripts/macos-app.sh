#!/bin/bash
# Wraps the desktop app's binary into a macOS app bundle, the way
# `wails build` does for a release. macOS opens its system panels, the
# output folder picker among them, only for an app bundle: run bare, the
# picker closes at once and Browse… does nothing.
#
# usage: scripts/macos-app.sh BINARY BUNDLE ICON.png
set -euo pipefail
bin=$1 app=$2 icon=$3

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$bin" "$app/Contents/MacOS/manga-downloader-gui"

# the icon: every size iconutil wants, from the 1024 tile
set_dir=$(mktemp -d)/iconfile.iconset
mkdir -p "$set_dir"
for size in 16 32 128 256 512; do
  sips -z $size $size "$icon" --out "$set_dir/icon_${size}x${size}.png" >/dev/null
  sips -z $((size * 2)) $((size * 2)) "$icon" --out "$set_dir/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$set_dir" -o "$app/Contents/Resources/iconfile.icns"
rm -rf "$(dirname "$set_dir")"

cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleName</key>
    <string>Manga Downloader</string>
    <key>CFBundleExecutable</key>
    <string>manga-downloader-gui</string>
    <key>CFBundleIdentifier</key>
    <string>com.github.elboletaire.mangadownloader.dev</string>
    <key>CFBundleVersion</key>
    <string>0.0.0</string>
    <key>CFBundleShortVersionString</key>
    <string>0.0.0</string>
    <key>CFBundleIconFile</key>
    <string>iconfile</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.13.0</string>
    <key>NSHighResolutionCapable</key>
    <string>true</string>
</dict>
</plist>
PLIST
