#!/usr/bin/env bash
# Builds the release archive for the OS/arch this runs on into dist/.
# Wails needs cgo on Linux and macOS, so each platform builds on its own runner.
# Each archive holds exactly one entry (binary or .app): the in-app updater requires that,
# and finds the asset by the GOOS and GOARCH in its name.
set -euo pipefail

VERSION="${VERSION:?set VERSION, e.g. 1.2.3}"
GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
NAME="do-not-forget"
ASSET="${NAME}_${GOOS}_${GOARCH}"
# Google OAuth client of the app (CI secrets, or .env locally). Not truly secret for a
# desktop app, but kept out of the repo so forks use their own. Empty = user enters their own.
# production: without it Wails builds its debug variant (devtools, FRONTEND_DEVSERVER_URL).
LDFLAGS="-s -w -X main.version=${VERSION} -X main.googleClientID=${GOOGLE_CLIENT_ID:-} -X main.googleClientSecret=${GOOGLE_CLIENT_SECRET:-}"

rm -rf build dist
mkdir -p build dist

case "$GOOS" in
linux)
	go build -trimpath -tags production -ldflags "$LDFLAGS" -o "build/$NAME" .
	tar -czf "dist/$ASSET.tar.gz" -C build "$NAME"
	;;
darwin)
	app="build/$NAME.app"
	mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
	cp assets/AppIcon.icns "$app/Contents/Resources/" # go run scripts/appicon.go
	go build -trimpath -tags production -ldflags "$LDFLAGS" -o "$app/Contents/MacOS/$NAME" .
	# LSUIElement: tray-only app, no Dock icon. A bundle id is needed for notifications.
	cat >"$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>$NAME</string>
	<key>CFBundleIconFile</key><string>AppIcon</string>
	<key>CFBundleIdentifier</key><string>io.github.mkelcik.do-not-forget</string>
	<key>CFBundleName</key><string>Do Not Forget</string>
	<key>CFBundleDisplayName</key><string>Do Not Forget</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>$VERSION</string>
	<key>CFBundleVersion</key><string>$VERSION</string>
	<key>LSMinimumSystemVersion</key><string>11.0</string>
	<key>LSUIElement</key><true/>
</dict>
</plist>
PLIST
	# Ad-hoc signature: required to run on Apple Silicon. Not notarized, so Gatekeeper
	# still asks on first launch (right-click > Open).
	codesign --force --deep --sign - "$app"
	ditto -c -k --keepParent "$app" "dist/$ASSET.zip"
	;;
windows)
	# windowsgui: no console window behind the tray app.
	go build -trimpath -tags production -ldflags "$LDFLAGS -H windowsgui" -o "build/$NAME.exe" .
	(cd build && 7z a -tzip "../dist/$ASSET.zip" "$NAME.exe" >/dev/null)
	;;
*)
	echo "unsupported GOOS $GOOS" >&2
	exit 1
	;;
esac

ls -l dist
