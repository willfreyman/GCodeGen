#!/usr/bin/env bash
# Build the macOS version of GcodeSimV3 and bundle it as a .app.
#
# Run on a Mac (or in a Mac CI runner) — Go can't reliably cross-compile
# CGo from Windows. End users don't need anything; the .app is double-
# clickable and self-contained.
#
# Prerequisites:
#   * Go 1.22+   (brew install go)
#   * Xcode CLI tools  (xcode-select --install) — provides clang for CGo
#
# Output: GcodeSimV3.app in the current directory.

set -euo pipefail
cd "$(dirname "$0")"

APP_NAME="GcodeSimV3"
BIN_NAME="gcodesim"
APP_DIR="${APP_NAME}.app"

# ---------------------------------------------------------------------- 1. Build
echo "→ Building $BIN_NAME (CGO + universal-binary if possible)..."

# Build a universal binary (arm64 + amd64) so the same .app runs on Apple
# Silicon and Intel Macs. Requires both arch toolchains; falls back to a
# single-arch build if `lipo` or one of the targets isn't available.
# Sync module dependencies. Catches imports added since the last build
# (e.g. new top-level modules) — `go build` alone won't auto-fetch
# new top-level modules, only transitive ones already in go.sum.
go mod tidy

# Inject the most recent git tag so the running binary can compare itself
# against the latest GitHub release on startup. --abbrev=0 returns the
# nearest tag without the offset suffix, so a build one commit past
# v3.0.1 still stamps as "v3.0.1" instead of "v3.0.1-1-gabc1234". The
# offset form was tripping the update check into thinking the running
# build was older than the release it was actually built from.
GIT_VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")
LDFLAGS="-s -w -X gcodegen.local/viewer/internal/version.Version=${GIT_VERSION}"
echo "  (version: ${GIT_VERSION})"

ARCH_BINS=()
build_arch() {
    local arch="$1"
    local out="${BIN_NAME}-${arch}"
    echo "  · ${arch}"
    GOARCH="$arch" CGO_ENABLED=1 \
        go build -ldflags="${LDFLAGS}" -trimpath -o "$out" ./cmd/gcodesim
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
    CGO_ENABLED=1 go build -ldflags="${LDFLAGS}" -trimpath -o "$BIN_NAME" ./cmd/gcodesim
fi

# ----------------------------------------------------------------- 2. Icon
# Convert icon.ico → icon.icns if we don't already have icns. Uses macOS's
# built-in `sips` and `iconutil` so no extra deps are needed. If something
# goes wrong (e.g. the .ico has only small sizes) the .app still builds
# without an icon.
if [ ! -f icon.icns ] && [ -f icon.ico ]; then
    echo "→ Converting icon.ico → icon.icns..."
    TMP_DIR=$(mktemp -d)
    if sips -s format png icon.ico --out "$TMP_DIR/icon.png" >/dev/null 2>&1; then
        ICONSET="$TMP_DIR/icon.iconset"
        mkdir -p "$ICONSET"
        for SIZE in 16 32 128 256 512; do
            sips -z $SIZE $SIZE "$TMP_DIR/icon.png" --out "$ICONSET/icon_${SIZE}x${SIZE}.png" >/dev/null
            sips -z $((SIZE*2)) $((SIZE*2)) "$TMP_DIR/icon.png" --out "$ICONSET/icon_${SIZE}x${SIZE}@2x.png" >/dev/null
        done
        iconutil -c icns -o icon.icns "$ICONSET"
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
cp Info.plist "$APP_DIR/Contents/Info.plist"
[ -f icon.icns ] && cp icon.icns "$APP_DIR/Contents/Resources/"

# Mark the binary executable explicitly (sometimes lost on FAT/SMB shares).
chmod +x "$APP_DIR/Contents/MacOS/$BIN_NAME"

# Print the result.
SIZE=$(du -sh "$APP_DIR" | cut -f1)
echo
echo "✓ Built $APP_DIR ($SIZE)"

# ----------------------------------------- 3.5 Zip for safe transfer
# A .app is a directory; emailing/Slacking/Discord-ing the raw folder
# usually flattens it or strips the executable bit, leaving the receiver
# with a bundle that won't launch. Distribute the .zip instead — Finder
# unpacks it back to a proper .app on the other end.
ZIP_NAME="${APP_NAME}.app.zip"
echo "→ Packaging $ZIP_NAME for distribution..."
rm -f "$ZIP_NAME"
zip -qry "$ZIP_NAME" "$APP_DIR"
echo "  · $ZIP_NAME ready ($(du -h "$ZIP_NAME" | cut -f1)) — share this, not the raw .app folder"

# ----------------------------------------------- 4. Register with Launch Services
# Tell macOS our app exists and owns the .nc/.gcode/.tap UTIs. Without
# this, double-clicking a .nc opens the system's default text editor
# instead of GcodeSimV3 (the OS reads our Info.plist on registration,
# not on every file open).
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
if [ -x "$LSREGISTER" ]; then
    echo "→ Registering with Launch Services..."
    "$LSREGISTER" -f "$(pwd)/$APP_DIR" >/dev/null 2>&1 || true
    echo "  · done — .nc / .gcode / .tap files now associate with GcodeSimV3"
fi

echo
echo "  Run:    open ./$APP_DIR"
echo "  Or:     double-click in Finder."
echo
echo "First launch may show 'unidentified developer' (we're unsigned)."
echo "Right-click → Open the first time to bypass Gatekeeper."
echo
echo "If .nc files still open in TextEdit:"
echo "  Right-click any .nc → Get Info → Open with: GcodeSimV3 → Change All..."
