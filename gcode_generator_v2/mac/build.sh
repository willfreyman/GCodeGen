#!/usr/bin/env bash
# Build the macOS version of GcodeGenV1 (the Tk-port editor) and bundle
# it as a .app. Parallel of gcode_viewer_v3/mac/build.sh but builds
# from a separate Go module — no file-association steps (the editor
# exports files, doesn't open them).
#
# Run from gcode_generator_v2/mac/:
#   ./build.sh
#
# Output: GcodeGenV1.app + GcodeGenV1.app.zip at the parent
# gcode_generator_v2/ folder.

set -euo pipefail

MAC_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$MAC_DIR/.."

APP_NAME="GcodeGenV1"
BIN_NAME="gcodegen"
APP_DIR="${APP_NAME}.app"

# ---------------------------------------------------------------------- 1. Build
echo "→ Building $BIN_NAME (CGO + universal-binary if possible)..."
go mod tidy

GIT_VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")
LDFLAGS="-s -w -X gcodegen.local/generator/internal/version.Version=${GIT_VERSION}"
echo "  (version: ${GIT_VERSION})"

ARCH_BINS=()
build_arch() {
    local arch="$1"
    local out="${BIN_NAME}-${arch}"
    echo "  · ${arch}"
    GOARCH="$arch" CGO_ENABLED=1 \
        go build -ldflags="${LDFLAGS}" -trimpath -o "$out" ./cmd/gcodegen
    ARCH_BINS+=("$out")
}

if command -v lipo >/dev/null 2>&1; then
    build_arch arm64
    build_arch amd64
    echo "  · merging into universal binary"
    lipo -create -output "$BIN_NAME" "${ARCH_BINS[@]}"
    rm -f "${ARCH_BINS[@]}"
else
    echo "  (lipo missing — building host-arch only)"
    CGO_ENABLED=1 go build -ldflags="${LDFLAGS}" -trimpath -o "$BIN_NAME" ./cmd/gcodegen
fi

# ----------------------------------------------------------------- 2. Icon
if [ ! -f "$MAC_DIR/icon.icns" ] && [ -f "$MAC_DIR/icon.ico" ]; then
    echo "→ Converting icon.ico → icon.icns..."
    TMP_DIR=$(mktemp -d)
    if sips -s format png "$MAC_DIR/icon.ico" --out "$TMP_DIR/icon.png" >/dev/null 2>&1; then
        ICONSET="$TMP_DIR/icon.iconset"
        mkdir -p "$ICONSET"
        for SIZE in 16 32 128 256 512; do
            sips -z $SIZE $SIZE "$TMP_DIR/icon.png" --out "$ICONSET/icon_${SIZE}x${SIZE}.png" >/dev/null
            sips -z $((SIZE*2)) $((SIZE*2)) "$TMP_DIR/icon.png" --out "$ICONSET/icon_${SIZE}x${SIZE}@2x.png" >/dev/null
        done
        iconutil -c icns -o "$MAC_DIR/icon.icns" "$ICONSET"
        echo "  · icon.icns created"
    else
        echo "  · sips couldn't read icon.ico — skipping (.app will use generic icon)"
    fi
    rm -rf "$TMP_DIR"
fi

# --------------------------------------------------------------- 3. Bundle
echo "→ Bundling $APP_DIR..."
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS"
mkdir -p "$APP_DIR/Contents/Resources"

cp "$BIN_NAME" "$APP_DIR/Contents/MacOS/$BIN_NAME"
cp "$MAC_DIR/Info.plist" "$APP_DIR/Contents/Info.plist"
[ -f "$MAC_DIR/icon.icns" ] && cp "$MAC_DIR/icon.icns" "$APP_DIR/Contents/Resources/"

chmod +x "$APP_DIR/Contents/MacOS/$BIN_NAME"

SIZE=$(du -sh "$APP_DIR" | cut -f1)
echo
echo "✓ Built $APP_DIR ($SIZE)"

# ----------------------------------------- 3.5 Zip for safe transfer
ZIP_NAME="${APP_NAME}.app.zip"
echo "→ Packaging $ZIP_NAME for distribution..."
rm -f "$ZIP_NAME"
zip -qry "$ZIP_NAME" "$APP_DIR"
echo "  · $ZIP_NAME ready ($(du -h "$ZIP_NAME" | cut -f1)) — share this, not the raw .app folder"

echo
echo "  Run:    open ../$APP_DIR        (from this mac-gen/ folder)"
echo "  Or:     double-click in Finder."
echo
echo "First launch may show 'unidentified developer' (we're unsigned)."
echo "Right-click → Open the first time to bypass Gatekeeper."
