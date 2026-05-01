# gcode_viewer_v2

VTK + PyQt5 rewrite of the G-code viewer. Replaces the Tkinter prototype
(`gcode_preview.py`) with a real 3D rendering backend.

## Setup

```
python -m venv .venv
.venv/bin/pip install -r requirements.txt    # Linux/macOS
.venv\Scripts\pip install -r requirements.txt # Windows
```

## Running

```
python -m gcode_viewer_v2.app
```

or directly:

```
python gcode_viewer_v2/app.py
```

## Layout

- `parser.py` — G-code state-machine parser. Pure logic, no UI deps.
  Ported verbatim from the Tk prototype's `parse()` / `arc_points()` / `Move`.
- `scene/path.py` — `vtkPolyData` builders for cut and rapid toolpaths; depth-color lookup table.
- `scene/stock.py` — translucent stock block outline (vtkCubeSource).
- `scene/tool.py` — bit cylinder mesh that follows the path with vtkTransform.
- `scene/removal.py` — heightmap-based material removal sim and overlap heatmap.
- `ui/main_window.py` — `QMainWindow` hosting a `QVTKRenderWindowInteractor`,
  toolbar (Open / Play / Pause / Reset / Speed / Bit dia / Heatmap toggle / Frame).
- `ui/controls.py` — custom Qt widgets, currently the Z-axis indicator.
- `app.py` — entry point.
- `pyinstaller.spec` — build spec with VTK/Qt module excludes for size trim.

## Notes on benchmarking from WSL

VTK on WSL falls back to Mesa software OpenGL — no GPU acceleration. Frame
rates measured under WSL are CPU-bound and unrepresentative. On native
Windows with hardware OpenGL (any integrated GPU from the last decade),
the same workload renders at 60+ fps.

## Building GcodeSimV1.exe (Windows)

From the project root in PowerShell:

```cmd
cd gcode_viewer_v2
pip install -r requirements.txt
pip install pyinstaller
pyinstaller pyinstaller.spec --noconfirm --clean
```

The .exe will land in `gcode_viewer_v2/dist/GcodeSimV1.exe`. Expect
**~180-220 MB** after the spec's VTK/Qt module excludes have been applied.
First-launch from `--onefile` mode takes 2-4 sec while the bundle
self-extracts; the bundled splash image shows during that phase so users
see immediate feedback.

Distribute by sending `GcodeSimV1.exe` directly. End users need nothing
installed beyond Windows itself + a recent OpenGL driver (ships with every
GPU driver from the last decade). The Microsoft Visual C++ 2015-2022
Redistributable is the only edge case — it's pre-installed on most Windows
machines but missing on some clean installs; if a tester reports
"won't launch with no error message", that's the first thing to check.

If the .exe fails to launch on a clean Windows VM, the most common cause
is an over-zealous exclude. Comment out blocks of `VTK_EXCLUDES` /
`QT_EXCLUDES` in `pyinstaller.spec` until it works, then reintroduce them
one block at a time to find the offender.
