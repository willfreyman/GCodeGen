package ui

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"

	"github.com/g3n/engine/window"
	"golang.org/x/image/draw"
)

// icon.png is the in-app window icon for the title bar. It is the
// single source of truth for the runtime icon — we used to also keep
// copies in windows/ and mac/, but they were unused (build scripts read
// icon.ico, not icon.png) and the duplicates inevitably drifted out of
// sync when someone updated only one copy.
//
// To change the title-bar icon: replace this file. To change the
// taskbar / Dock icon, also update windows/icon.ico and mac/icon.ico
// (those are read by goversioninfo and sips respectively at build time).
//
// GLFW does not pick up the embedded .exe icon for the title bar
// automatically; we have to hand it the image at runtime via SetIcon.
//
//go:embed icon.png
var iconPNG []byte

// Sizes we generate from the source for SetIcon. GLFW picks the closest
// match per context: ~16 for the title bar (24 on hi-DPI), ~32 for the
// taskbar preview, ~48 for Alt-Tab, larger for full-size renderings.
// Pre-scaling each size with a high-quality filter (CatmullRom) gives
// dramatically cleaner small-icon results than letting GLFW downscale
// a single 1024×1024 source straight to 16×16 — the brute-force
// single-step scale aliases fine details to noise.
//
// If even the 16×16 generated here looks choppy, the artwork itself
// is too detailed for that pixel count and a hand-tuned 16×16 PNG is
// the only real fix. See window_icon.go header comment for guidance.
var iconSizes = []int{16, 24, 32, 48, 64, 128, 256}

// setWindowIcon installs the embedded PNG (downscaled to several common
// icon sizes) as the title-bar / taskbar icon. Works on Windows + Linux.
// Documented no-op on macOS — the OS uses the .app bundle's icns and
// has no title-bar icon convention.
//
// Silent on decode failure (binary still runs with GLFW's default
// icon) so a corrupt embed doesn't take down the whole app.
func setWindowIcon(win *window.GlfwWindow) {
	src, _, err := image.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		return
	}

	// Generate all the standard sizes. CatmullRom is a high-quality
	// resampler — sharper than bilinear, less ringing than Lanczos
	// at icon sizes. draw.Over preserves the source alpha.
	images := make([]image.Image, 0, len(iconSizes))
	for _, sz := range iconSizes {
		dst := image.NewRGBA(image.Rect(0, 0, sz, sz))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		images = append(images, dst)
	}

	// GlfwWindow embeds *glfw.Window, so SetIcon is reachable directly.
	win.SetIcon(images)
}
