# CNC G-Code Toolchain

A two-piece toolchain for hobbyist CNC routers:

1. **`gcodegen.py`** — a Tkinter sketchpad that turns freehand canvas strokes
   into a `.nc` G-code program. Set per-stroke depth, feeds, RPM, safe-Z;
   export and run.
2. **`gcode_viewer_v3/`** (distributed as **`GcodeSimV3` / `gcodesim.exe`**) — a
   pure-Go 3D simulator that loads any `.nc` file, plays the toolpath through
   an animated end-mill, and removes material from a stock block in real time
   (with optional through-cut when material thickness is set) so you can
   sanity-check a program before sending it to a real machine.

The earlier Python+VTK viewer (`gcode_viewer_v2/` → `GcodeSimV1.exe`) is kept
as a reference implementation; the Go viewer (v3) is the active development
target. v3 ships as a **single ~5 MB binary** vs ~200 MB for v2.

## Documentation

* **[`docs/MANUAL.md`](docs/MANUAL.md)** — full user manual: every
  toolbar button, the material-removal heightmap, through-cut, view
  cube, mouse + keyboard shortcuts, file formats, settings, and
  troubleshooting.
* **Tutorials are bundled into the binary** — open `gcodesim.exe` /
  `GcodeSimV3.app` and click **Tutorials ▾** in the toolbar for a
  curriculum of six `.nc` programs (basic outline → pocket → arcs →
  pyramid → through-cut → multi-op). Each file has an in-line comment
  header explaining what to look for. Source lives at
  [`gcode_viewer_v3/internal/ui/tutorials/`](gcode_viewer_v3/internal/ui/tutorials/).
* **[`CLAUDE.md`](CLAUDE.md)** — architecture notes for developers
  working on the source.

## Quick start

### Editor — sketch a program (Tkinter, stdlib-only)

```cmd
python gcodegen.py
```

Click + drag to draw toolpaths. Drag the orange dot to set machine origin,
the corner handles to resize the perimeter. Pick depths, feeds, RPM, and
hit **Generate G-code** to export.

### Viewer (v3, Go) — Windows

```cmd
cd gcode_viewer_v3\windows
.\build.bat                 :: produces ..\gcodesim.exe (~5 MB)
..\gcodesim.exe             :: opens the viewer, then Open file
```

Requires **Go 1.22+** and a C compiler (TDM-GCC or MSYS2 mingw-w64) for CGo.

### Viewer (v3, Go) — macOS

```sh
cd gcode_viewer_v3/mac
./build.sh                  # produces ../GcodeSimV3.app (universal arm64+amd64)
open ../GcodeSimV3.app
```

Requires **Go 1.22+** and Xcode CLI tools (`xcode-select --install`).
First launch may show "unidentified developer" — right-click → Open once
to bypass Gatekeeper.

> **Single source tree, two platform folders.** All Go source lives in
> `gcode_viewer_v3/cmd/` and `gcode_viewer_v3/internal/` and compiles
> on both platforms (build-tagged `*_darwin.go` / `*_windows.go` files
> split the OS-specific bits). The platform-specific build scripts and
> resources live in `gcode_viewer_v3/windows/` (`build.ps1`,
> `build.bat`, `versioninfo.json`, `icon.ico`) and
> `gcode_viewer_v3/mac/` (`build.sh`, `Info.plist`, `icon.ico`).
> Build outputs (`gcodesim.exe`, `GcodeSimV3.app`) land in the parent
> `gcode_viewer_v3/` folder.

### Controls (both platforms)

* **Open** / **Ctrl+O** — load a `.nc` / `.gcode` / `.tap` file
* **Play / Pause** / **Spacebar** — animate the toolpath
* **Reset** — restart playback and rebuild the carved surface
* **Speed slider** — 0.5× to 50× playback speed
* **Bit dia** — type a value (mm) and press Enter or **Set** to rebuild the tool
* **Options ▾** — material thickness for through-cut (accepts `19.05`, `0.75in`, or `0.75"`)
* **Progress bar** — slidable (drag to scrub through the program)
* **Mouse**: left-drag orbit, right-drag pan, scroll zoom
* **View cube**: hover any face → highlights, click → snaps the main camera

### Legacy viewers (kept for reference)

```cmd
python gcode_preview.py                  :: Tkinter+stdlib, no GPU needed
cd gcode_viewer_v2 && python -m gcode_viewer_v2.app   :: VTK + PyQt5
```

## Get the viewer (pre-built)

Pre-built binaries for the v3 viewer are published on the
[GitHub Release][releases] page when there's a tagged release —
`gcodesim.exe` for Windows and `GcodeSimV3.app.zip` for macOS. Otherwise,
build from source as shown above (one command).

The editor binary `exe/gcodegenV1.0.exe` is committed directly (only
~10 MB). All viewer binaries are gitignored — they rebuild from source.

[releases]: https://github.com/willfreyman/GCodeGen/releases

## Publishing a new release

After building both binaries on their native platforms, upload as release
assets so teammates can download instead of building from source.
Requires `gh` CLI (`brew install gh` / `winget install GitHub.cli`,
then `gh auth login` once).

```sh
# Pick a version tag
TAG=v3.0.1

# Create the release (do this once per version)
gh release create $TAG --title "GcodeSimV3 $TAG" --notes "What changed: ..."

# Upload Windows binary (run after gcode_viewer_v3\windows\build.bat)
gh release upload $TAG gcode_viewer_v3/gcodesim.exe --clobber

# Upload macOS bundle (run after gcode_viewer_v3/mac/build.sh)
gh release upload $TAG gcode_viewer_v3/GcodeSimV3.app.zip --clobber
```

`--clobber` lets you re-upload if you build a new version of the same
asset; drop it if you want uploads to fail on duplicate.

## Repository layout

```
GCodeGen/
├── README.md                ← you are here
├── CLAUDE.md                ← Claude Code guidance (architecture notes)
├── .gitignore
├── icon.ico                 ← shared app icon
│
├── gcodegen.py              ← Tkinter sketch-to-G-code editor
├── gcode_preview.py         ← legacy Tkinter viewer
│
├── gcode_viewer_v3/         ← active viewer (Go + g3n) — shared cross-platform tree
│   ├── go.mod
│   ├── windows/             ← Windows-only: build.ps1, build.bat, versioninfo.json, icon.ico
│   ├── mac/                 ← macOS-only: build.sh, Info.plist, icon.ico
│   ├── cmd/gcodesim/        ← entry point (shared)
│   └── internal/            ← shared Go source (build-tagged *_darwin.go / *_windows.go)
│       ├── parser/          ← G-code parser (1:1 port of v2's parser.py)
│       ├── scene/           ← actors: path, stock, tool, view cube, heightmap
│       └── ui/              ← window, toolbar, orbiter, dialogs
│           └── tutorials/   ← 6 .nc files baked into the binary via go:embed
│
├── gcode_viewer_v2/         ← reference viewer (Python + VTK + PyQt5)
│   ├── app.py
│   ├── parser.py
│   ├── scene/{path, stock, tool, removal, view_cube}.py
│   ├── ui/{main_window, controls, debug_window}.py
│   ├── bench/               ← FPS benchmark suite (dev only)
│   └── pyinstaller.spec     ← v2 build recipe
│
├── build.bat                ← one-command build of v2's GcodeSimV1.exe
└── exe/
    ├── GcodeSimV1.exe       ← v2 PyInstaller build (gitignored — 200 MB)
    └── gcodegenV1.0.exe     ← bundled editor (committed, ~10 MB)
```

## What's deliberately out of scope

The viewer is for **visual verification**, not CAM-grade post-processing.
It does not:
- emit G2/G3 arcs (the editor linearizes everything to G1)
- perform tool-radius compensation
- check for collisions
- validate against your specific CNC controller
- handle undercuts in material-removal sim (heightmap-based — fine for
  3-axis routing, doesn't model 5-axis)

If you run a generated program blindly on a real machine, you can crash a
tool. Always dry-run with the spindle off first.

## Limitations / known issues

- v2 viewer uses a heightmap with cell size scaled to bit diameter
  (`max(1.0, bit_dia/4.0)` mm). Bigger bits → coarser cells → faster but
  visibly stepped. Adjust in `scene/removal.py` if needed.
- vtkPolyDataNormals is intentionally **not** used on the heightmap surface
  (flat shading instead). This is a perf optimization that also reads as
  more "machined" and matches CAMotics / Vericut style.
- WSL renders via Mesa software OpenGL — performance numbers from there
  aren't representative. Build and test on native Windows.

## License

Use at your own risk. See per-file headers for any specific notices.
