# CNC G-Code Toolchain

Two-app toolchain for hobbyist CNC routers:

1. **`gcodegen.py`** — a Tkinter sketchpad that turns freehand canvas strokes
   into a `.nc` G-code program. Set per-stroke depth, feeds, RPM, safe-Z;
   export and run.
2. **`gcode_viewer_v2/`** (distributed as **`GcodeSimV1.exe`**) — a 3D
   simulator that loads any `.nc` file, plays the toolpath through an
   animated end-mill, and removes material from a stock block in real time
   so you can sanity-check a program before sending it to a real machine.

## Quick start

### Editor — sketch a program (Tkinter, stdlib-only)

```cmd
python gcodegen.py
```

Click + drag to draw toolpaths. Drag the orange dot to set machine origin,
the corner handles to resize the perimeter. Pick depths, feeds, RPM, and
hit **Generate G-code** to export.

### Viewer — simulate a `.nc` file (VTK 3D)

```cmd
cd gcode_viewer_v2
pip install -r requirements.txt
python -m gcode_viewer_v2.app
```

`File → Open .nc`, then `Play`. Rotate with left-drag, pan with
shift-left-drag, zoom with the wheel. The labeled view-cube in the
top-right snaps the camera to standard views; click a corner of the cube
for an isometric.

A stripped-down legacy Tk-based viewer (`gcode_preview.py`) is kept for
users with older machines that can't run VTK; same file format.

## Get the viewer

Two paths, pick whichever's easier:

**Pre-built binary** (recommended for end users) — download
`GcodeSimV1.exe` from the latest [GitHub Release][releases]. Double-click
to run. Nothing else to install.

**Build from source** (one command, takes ~2 min):

```cmd
build.bat
```

Requires Python 3.10+ on `PATH`. The script installs `vtk`, `PyQt5`,
`numpy`, and `pyinstaller` into the active Python environment, runs
PyInstaller against `gcode_viewer_v2/pyinstaller.spec`, and copies the
output to `exe\GcodeSimV1.exe` for distribution.

Use a virtual env if you want isolation:

```cmd
python -m venv .venv
.venv\Scripts\activate
build.bat
```

The editor binary `exe/gcodegenV1.0.exe` is committed to the repo
directly (only ~10 MB). The viewer binary is excluded — it's 130 MB,
over GitHub's per-file limit, and rebuilds deterministically from source.

[releases]: https://github.com/willfreyman/GCodeGen/releases

## Repository layout

```
GCodeGen/
├── README.md             ← you are here
├── CLAUDE.md             ← Claude Code guidance (architecture notes)
├── .gitignore
├── icon.ico              ← shared app icon
│
├── gcodegen.py           ← Tkinter sketch-to-G-code editor
├── gcode_preview.py      ← legacy Tkinter viewer (Stage-1 perf-fixed)
│
├── gcode_viewer_v2/      ← the modern VTK + PyQt5 viewer (becomes GcodeSimV1.exe)
│   ├── app.py            ← entry point with splash
│   ├── parser.py         ← G-code state-machine parser (pure Python)
│   ├── scene/            ← VTK scene actors
│   │   ├── path.py       ← toolpath line plot
│   │   ├── stock.py      ← stock-block outline
│   │   ├── tool.py       ← realistic CNC bit (multi-part assembly)
│   │   ├── removal.py    ← heightmap material-removal sim
│   │   └── view_cube.py  ← interactive labeled view cube (chamfered)
│   ├── ui/
│   │   ├── main_window.py
│   │   ├── controls.py   ← Z-axis indicator widget
│   │   └── debug_window.py ← floating debug window (FPS, logs, system info)
│   ├── assets/splash.png
│   ├── bench/            ← FPS benchmark suite (dev only)
│   ├── requirements.txt  ← vtk, PyQt5, numpy
│   ├── pyinstaller.spec  ← build recipe
│   └── README.md         ← v2-specific notes
│
├── build.bat             ← one-command build of GcodeSimV1.exe
└── exe/
    ├── GcodeSimV1.exe    ← built locally by build.bat (gitignored — 130 MB)
    └── gcodegenV1.0.exe  ← bundled editor (committed, ~10 MB)
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
