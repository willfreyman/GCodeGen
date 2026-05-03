# gcode_viewer_v3_mac

macOS port of `GcodeSimV3` — same Go source as `gcode_viewer_v3/`, packaged
as a double-clickable `.app` for Mac.

Why a separate folder? The Go source is fully cross-platform; the only
macOS-specific pieces are the build script, the `Info.plist`, and the
`.app` bundle layout. Keeping them in their own directory means the
Windows tree stays clean and the Mac tree stays self-documenting.

## Building (on a Mac)

You need **a Mac** to build this — Go can't reliably cross-compile CGo
(GLFW + OpenGL bindings) from Windows or Linux. You can rent a cloud Mac
(MacStadium, MacInCloud) for an hour if you don't have one.

Prerequisites on the Mac:

```sh
xcode-select --install         # Apple's CLI dev tools (provides clang for CGo)
brew install go                # Go 1.22+
```

Build + bundle in one shot:

```sh
cd /path/to/gcode_viewer_v3_mac
./build.sh
```

Output: `GcodeSimV3.app` in the current directory.

The script:
1. Compiles a **universal binary** (arm64 + amd64) so one `.app` runs on
   both Apple Silicon and Intel Macs (falls back to host-arch if `lipo` is
   missing).
2. Converts `icon.ico` → `icon.icns` using macOS's built-in `sips` +
   `iconutil` (no extra dependencies needed).
3. Assembles the `.app` bundle structure with `Contents/MacOS/gcodesim`,
   `Contents/Info.plist`, and `Contents/Resources/icon.icns`.

## Running

Double-click `GcodeSimV3.app` in Finder, or from a terminal:

```sh
open ./GcodeSimV3.app
```

**First launch** will show the "App can't be opened because it is from an
unidentified developer" warning — we're unsigned. Bypass once with:

* Right-click `GcodeSimV3.app` → **Open** → confirm.
* From then on it opens normally.

To distribute publicly (and avoid the warning entirely), the `.app` would
need to be code-signed with an Apple Developer ID and notarized. Costs $99/yr
plus 5 minutes per build to notarize. Not needed for in-house use.

## Same controls as Windows

* **Open .nc...** / **Ctrl+O** — load G-code (Cmd+O may not work yet —
  g3n's Edit/shortcut handling is Ctrl-based; we can map Cmd → Ctrl in a
  later iteration if the team prefers Mac-native conventions)
* **Play / Pause / Reset / Reframe** — toolbar buttons or **Spacebar / R**
* **Speed / Bit / Material thickness** — same UI as Windows
* **Mouse**: left-drag orbit, right-drag pan, scroll zoom
* **View cube**: hover any face → highlights, click → snaps to that view

## Source code

The Go source under `cmd/` and `internal/` is a copy of `gcode_viewer_v3/`.
For now, fixes need to be made in both trees by hand. Future improvement:
collapse to a single source tree with two build scripts (one PowerShell,
one bash) sharing the same module.

## Distribution checklist

Before sending the `.app` to a teammate:
- [ ] Test on a clean Mac (no Go, no Xcode) to confirm self-containment
- [ ] Right-click → Open once to clear Gatekeeper for them
- [ ] Bundle size should be ~25-30 MB (universal arm64+amd64) — much
      smaller if you build host-arch only
- [ ] Drag the `.app` to `~/Applications/` if you want it permanently
      installed; otherwise it can run from any folder
