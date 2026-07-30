# gcode_viewer_v3

Go port of `gcode_viewer_v2/` (Python + VTK + PyQt5) using the
[g3n](https://github.com/g3n/engine) engine. Single ~10 MB binary vs
~180-220 MB for the PyInstaller build of v2.

**Shared Go source, two platform folders.** All Go source lives in
`cmd/` + `internal/` and compiles on both Windows and macOS (build-tagged
`*_darwin.go` / `*_windows.go` files split the OS-specific bits). Build
scripts + per-platform resources live in dedicated `windows/` and `mac/`
folders. Each platform's folder is self-contained — edit the .go files
once and both platforms pick up the change.

## Build

Requires Go 1.22+ and a C compiler (TDM-GCC or MSYS2 mingw-w64 on
Windows; Xcode CLI tools on macOS).

```
# Windows
cd windows
.\build.bat              # → ../gcodesim.exe (with icon, version info)

# macOS
cd mac
./build.sh               # → ../GcodeSimV3.app + ../GcodeSimV3.app.zip
open ../GcodeSimV3.app
```

For dev iteration without the full build pipeline (no icon, no version
stamp, no .syso resources):

```
go run ./cmd/gcodesim                 # quick run from the project root
go run ./cmd/gcodesim path/to/file.nc # load a file at startup
```

## HoleGen — hole-grid generator

The **HoleGen** toolbar button opens a draggable in-window panel that generates
G-code for a grid of round holes helically bored into metal tube (FRC /
MAXTube stock). Implemented to [`HoleGen_SPEC.md`](HoleGen_SPEC.md).

- 12 parameters, each accepting an optional `in` / `"` / `mm` suffix.
- Named target-diameter quick-fill, live run-time estimate, and named
  presets persisted to `~/.holegen_presets.json`.
- **Preview in viewer** renders the program in the 3D scene without touching
  the disk; **Generate .nc File** runs the full save flow and then previews.
  Either way the program's own bit diameter and tube thickness are pushed
  into the viewer so the end-mill actor and the through-cut threshold match
  what was generated.

`internal/holegen/` is pure stdlib with no UI imports, so it is testable
headless — including a byte-for-byte assertion against the spec's §7.7
reference program.

## Tests

```
go test ./internal/parser/... ./internal/holegen/...
```

Asserts byte-exact parity against canonical output from the Python v2
reference parser. Any failure means the Go parser has drifted from
`gcode_viewer_v2/parser.py`.

## Module layout

```
gcode_viewer_v3/
├── go.mod / go.sum             module gcodegen.local/viewer
├── windows/                    Windows-only build assets
│   ├── build.ps1               PowerShell build script
│   ├── build.bat               exec-policy wrapper around build.ps1
│   ├── versioninfo.json        goversioninfo input → resource_windows_*.syso
│   └── icon.ico                embedded into the .exe by goversioninfo
├── mac/                        macOS-only build assets
│   ├── build.sh                bash build script (universal arm64+amd64)
│   ├── Info.plist              bundle metadata + .nc file association
│   └── icon.ico                converted to .icns by build.sh, copied into .app
├── cmd/gcodesim/
│   └── main.go                 entry — calls ui.Run()
└── internal/                   shared Go source, all platforms
    ├── parser/                 G-code parser (1:1 port of v2's parser.py)
    ├── holegen/                hole-grid G-code generator (pure stdlib logic)
    ├── scene/                  actors: path, stock, tool, view cube, heightmap, axes
    └── ui/
        ├── window.go           App, scene, camera, animation tick
        ├── orbiter.go          Z-up orbit controller (replaces g3n's Y-up one)
        ├── toolbar.go          two-row toolbar + Options/Tutorials dropdowns
        ├── holegen_panel.go    HoleGen overlay (12 fields, presets, preview)
        ├── tutorials.go        //go:embed tutorials/*.nc
        ├── tutorials/*.nc      6 starter programs baked into the binary
        ├── window_icon.go      //go:embed icon.png + GLFW SetIcon
        ├── icon.png            title-bar icon source (single-source-of-truth)
        ├── settings.go         persisted skipped-update-versions
        ├── update_prompt.go    GitHub-API check + native Yes/No dialogs
        ├── openfile_*.go       darwin: Apple Event handler for double-click
        ├── register_*.go       windows: HKCU file association
        └── dialogs.go          sqweek/dialog file open + error message
```

Build outputs (`gcodesim.exe`, `GcodeSimV3.app`, `GcodeSimV3.app.zip`)
land at the project root (`gcode_viewer_v3/`), not inside the platform
folders, so the `gh release upload` recipe in the top-level README is
unaffected.

## Roadmap — designed but not implemented

- **Polygon-union material removal renderer.** Replaces the
  heightmap's discretized cut display with mathematically exact
  swept-stadium polygons — real bit-radius fillets at internal
  corners, real roundings at external corners. Heightmap stays for
  Z-varying moves (ramps, 3D surfacing). Full design at
  [`docs/POLYGON_RENDERER.md`](../docs/POLYGON_RENDERER.md). 7
  end-to-end-testable milestones, ~1000 LOC. Don't re-derive — that
  doc covers root-cause analysis, alternatives, algorithm details,
  testing, performance, and open questions.

## Known parity quirks (preserved from Python)

- R-form arcs (`G2 X.. Y.. R..`) are silently treated as degenerate moves —
  same as Python.

## Fixed parity bugs (changed in BOTH implementations)

Both of these were found by feeding the viewer its own HoleGen output, and
both are fixed in `internal/parser/` **and** `gcode_viewer_v2/parser.py` so
the two stay in lockstep. `testdata/sample.nc` results are unchanged, so the
golden test still pins Go↔Python parity.

- **G-word matching is by whole number, not substring.** Mode detection used
  to be a `strings.Contains` / `in` chain, which misread any G-word that
  merely contained those two characters: `G01`/`G03` were read as `G0`
  rapids, `G17` set G1 mode, and `G21` left the parser in *arc* mode.
  Bare-notation files never tripped it, which is why it survived so long —
  HoleGen output hits all of it. Now `motionMode` / `motion_mode` extracts
  each G-word and compares the integer.
- **Full-circle arcs sweep 360° instead of collapsing to a point.** When an
  arc's end XY equals its start XY — the standard I/J way to program a full
  circle, and what every helical boring pass emits — the sweep computed as
  zero, so a bored hole rendered as nothing but a Z descent. `ArcPoints` /
  `arc_points` now detect the closed case and wind a full revolution in the
  commanded direction.
