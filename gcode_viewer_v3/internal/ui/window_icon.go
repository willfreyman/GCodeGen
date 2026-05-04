package ui

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"

	"github.com/g3n/engine/window"
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

// setWindowIcon installs the embedded PNG as the title-bar / taskbar
// icon. Works on Windows + Linux. Documented no-op on macOS — the OS
// uses the .app bundle's icns and has no title-bar icon convention.
//
// Silent on decode failure (binary still runs with GLFW's default
// icon) so a corrupt embed doesn't take down the whole app.
func setWindowIcon(win *window.GlfwWindow) {
	img, _, err := image.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		return
	}
	// GlfwWindow embeds *glfw.Window, so SetIcon is reachable
	// directly. GLFW picks the closest size from the slice for each
	// usage context (16×16 for the title bar, 32×32 for the taskbar
	// preview, etc.) — passing a single 256× source is fine; it
	// downscales reasonably for our purposes.
	win.SetIcon([]image.Image{img})
}
