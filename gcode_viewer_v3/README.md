# gcode_viewer_v3

Go port of `gcode_viewer_v2/` (Python + VTK + PyQt5) using the
[g3n](https://github.com/g3n/engine) engine. Single ~5 MB binary vs
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

## Tests

```
go test ./internal/parser/...
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
    ├── scene/                  actors: path, stock, tool, view cube, heightmap
    └── ui/
        ├── window.go           App, scene, camera, animation tick
        ├── orbiter.go          Z-up orbit controller (replaces g3n's Y-up one)
        ├── toolbar.go          two-row toolbar + Options/Tutorials dropdowns
        ├── tutorials.go        //go:embed tutorials/*.nc
        ├── tutorials/*.nc      6 starter programs baked into the binary
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

- `G0`/`G1`/`G2`/`G3` mode detection uses substring match with an `elif`
  chain, so a line written as `G01 X10` is treated as a `G0` rapid (because
  `"G0"` is a substring of `"G01"`). Files emitted by `gcodegen.py` use bare
  `G0`/`G1` notation where this never triggers, so the behaviour matches v2
  in practice. Fix in both implementations together if real-world files
  with `G01`/`G02`/`G03` notation start showing up.
- R-form arcs (`G2 X.. Y.. R..`) are silently treated as degenerate moves —
  same as Python.
