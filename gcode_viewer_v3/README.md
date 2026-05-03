# gcode_viewer_v3

Go port of `gcode_viewer_v2/` (Python + VTK + PyQt5) using the
[g3n](https://github.com/g3n/engine) engine. Targets a much smaller
distributable `.exe` (~16-20 MB stripped vs ~180-220 MB for the PyInstaller
build of v2).

Status: **M2 — first 3D window, toolpath / stock / tool rendering, file open**.
Animation lands in M3 (heightmap removal sim). See
`/home/will/.claude/plans/i-want-to-turn-zippy-stroustrup.md` for the full plan.

## Build & run

Requires Go 1.22+ and a C compiler (TDM-GCC or MSYS2 mingw-w64 on Windows).

First-time setup:
```
cd gcode_viewer_v3
go mod tidy            # downloads g3n, sqweek/dialog, transitive deps
```

Build & run:
```
go build .\cmd\gcodesim
gcodesim.exe              # empty window; press Ctrl+O to load a file
gcodesim.exe path\to\file.nc   # loads on startup
```

Keys:
- **Ctrl+O** — open a `.nc` / `.gcode` / `.tap` / `.txt` file
- **R** — reframe the camera to fit the model
- **Esc** — quit

Mouse: orbit (left-drag), pan (right-drag), zoom (wheel).

## Tests

```
go test ./internal/parser/...
```

The parser tests assert byte-exact parity against canonical output from the
Python v2 reference parser, computed once from
`internal/parser/testdata/sample.nc`. Any test failure means the Go parser
has drifted from `gcode_viewer_v2/parser.py`.

## Module layout

```
gcode_viewer_v3/
├── go.mod
├── README.md
├── cmd/
│   ├── gcodesim/main.go        production entry point — opens g3n window
│   └── g3n_smoke/main.go       reference single-file API skeleton (kept as
│                               living docs; not part of the shipped binary)
└── internal/
    ├── parser/                 (M1) G-code parser + tests
    ├── scene/
    │   ├── colors.go           5-stop depth gradient
    │   ├── path.go             toolpath: cut LineStrips + dashed rapid Lines
    │   ├── stock.go            translucent stock box + edge wireframe
    │   └── tool.go             5-part end-mill assembly under a core.Node
    └── ui/
        ├── dialogs.go          sqweek/dialog file-open + error message
        └── window.go           App, scene, camera, key handlers, file loader
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

## Build environment (eventual M2+ requirements, not needed for M1 CLI)

Once g3n lands in M2, the build needs:

- Go 1.22+ on PATH
- TDM-GCC or MSYS2 mingw-w64 (g3n requires CGo for OpenGL via go-gl/glfw)
- `CGO_ENABLED=1`, `CC=gcc`

The M1 parser package has no CGo deps and builds with plain `go build`.
