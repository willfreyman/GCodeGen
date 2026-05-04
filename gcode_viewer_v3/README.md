# gcode_viewer_v3

Go port of `gcode_viewer_v2/` (Python + VTK + PyQt5) using the
[g3n](https://github.com/g3n/engine) engine. Single ~5 MB binary vs
~180-220 MB for the PyInstaller build of v2.

**Single source tree** for both Windows and macOS — the build-tagged
`*_darwin.go` / `*_windows.go` files split platform-specific logic.
`build.ps1` / `build.bat` builds for Windows; `build.sh` builds the
universal-binary `.app` for macOS.

## Build

Requires Go 1.22+ and a C compiler (TDM-GCC or MSYS2 mingw-w64 on
Windows; Xcode CLI tools on macOS).

```
# Windows
.\build.bat              # → gcodesim.exe (with icon, version info)

# macOS
./build.sh               # → GcodeSimV3.app + GcodeSimV3.app.zip
open ./GcodeSimV3.app
```

For dev iteration without the full build pipeline:

```
go run ./cmd/gcodesim                 # quick run (no icon, no version stamp)
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
├── build.ps1 / build.bat       Windows build
├── build.sh                    macOS build (universal arm64+amd64 .app)
├── Info.plist                  macOS bundle metadata + .nc file association
├── icon.ico                    Windows icon; converted to .icns by build.sh
├── cmd/gcodesim/
│   ├── main.go                 entry — calls ui.Run()
│   └── versioninfo.json        goversioninfo input → resource_windows_*.syso
└── internal/
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

## Known parity quirks (preserved from Python)

- `G0`/`G1`/`G2`/`G3` mode detection uses substring match with an `elif`
  chain, so a line written as `G01 X10` is treated as a `G0` rapid (because
  `"G0"` is a substring of `"G01"`). Files emitted by `gcodegen.py` use bare
  `G0`/`G1` notation where this never triggers, so the behaviour matches v2
  in practice. Fix in both implementations together if real-world files
  with `G01`/`G02`/`G03` notation start showing up.
- R-form arcs (`G2 X.. Y.. R..`) are silently treated as degenerate moves —
  same as Python.
