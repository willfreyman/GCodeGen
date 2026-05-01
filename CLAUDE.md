# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

Three apps live here. The **modern viewer** (`gcode_viewer_v2/`) is the active development target; the two Tkinter scripts at root are kept working but are not where new features should land unless asked.

- `gcodegen.py` — **Editor.** Tkinter sketchpad: draw freehand toolpaths, set per-stroke depths + machine settings, export `.nc`. Stdlib-only. Has its own in-app simulator and finished-product preview windows.
- `gcode_preview.py` — **Legacy Tk viewer.** Earlier Tkinter-based G-code viewer with a hand-rolled 3D projection on a 2D Canvas. Performance-fixed in Stage 1 of the v2 refactor (see Stage-1 notes below). Kept around for users who can't run VTK.
- `gcode_viewer_v2/` — **Modern viewer.** VTK + PyQt5 rewrite with real 3D, material-removal heightmap simulation, splash, debug window, view cube, etc. Ships as `GcodeSimV1.exe` via PyInstaller. **This is where new viewer work should land.**
- `exe/gcodegenV1.0.exe` — bundled editor binary.

There is no shared library between root-level scripts and v2; the v2 viewer's `parser.py` is a clean port of the root scripts' parsing logic with no UI deps.

## Running

```
python gcodegen.py                    # editor (Tk, stdlib-only)
python gcode_preview.py               # legacy Tk viewer
python -m gcode_viewer_v2.app         # modern VTK viewer (run from project root)
```

The editor auto-opens the Finished Product Preview window 120ms after launch (`window.after(120, open_preview)` in `gcodegen.py`). That window is intentionally non-closable (`WM_DELETE_WINDOW` is bound to a no-op) — it deiconifies/lifts on subsequent calls instead of being recreated.

## Architecture notes worth knowing before editing

### `gcodegen.py` — module-level state, not a class

All app state lives in module-level globals: `strokes`, `current_stroke_pts`, `perim`, `origin`, `drag_state`, `hover_stroke_idx`, etc. The Tk widgets are also module-level (`canvas`, `window`, the `*_entry` widgets). When adding behavior, follow this convention rather than introducing a class — the existing code reads and mutates these globals directly across functions.

**Coordinate system.** The canvas is in pixels with Y inverted (top-left origin). The user-draggable orange dot defines the machine origin, and the perimeter rectangle defines the work area. `px_to_mm` / `mm_to_px` convert between canvas pixels and machine millimeters using `perim_w_entry`/`perim_h_entry` as the mm extents of the perimeter. Every G-code export and material preview goes through these.

**Strokes vs ops.** Freehand strokes are stored raw in `strokes` (pixel points). Two helpers normalize them for downstream use:
- `_ops_mm()` — converts to mm, optionally prepends a perimeter rectangle op. Used by `generate_gcode`.
- `_ops_px()` / `_all_strokes_px()` — keep pixel coords, optionally prepend perimeter. Used by the simulation and preview windows.

**Material presets.** Two dictionaries exist: `MAT_MACHINE_PRESETS` (currently used — populates feed XY/Z and RPM entries) and `NEW_MAT_MACHINE_PRESETS` (richer, includes step-over/step-down, **not yet wired up**). If you're asked to add step-over support, switch `_apply_material_preset` to read from the NEW dict.

**G-code dialect.** `generate_gcode` emits metric absolute (`G21 G90 G17`), spindle on (`M3 S<rpm>`), per-op rapid to safe Z then plunge then linear cuts, end with `M5 M30`. No arcs, no tool-radius compensation, no bounds checking — the README explicitly calls these out as known limitations.

### `gcode_preview.py` — class-based, performance-fixed Tk viewer

`Viewer` owns everything. The render path was overhauled in Stage 1 of the v2 effort to drop UI lag from 137 ms/rotate-event down to a coalesced redraw (~50× perceived improvement) and to eliminate a 3.3-second freeze when typing into the bit-width entry. Key fixes already landed:

- **Bit-width entry uses `<Return>` / `<FocusOut>` bindings**, not `trace_add("write", ...)` — typing no longer fires a redraw per keystroke.
- **Rotate / `<Configure>` redraws are coalesced** via `_request_redraw()` + `after_idle` so multiple events collapse into one render.
- **Live cut chain is incrementally appended** in `animate()` — one `create_line` per new sample point, not a full chain re-render.
- **Z-bar and tool actor use `coords` / `itemconfig`** to update in place (no delete+create per frame).
- **`_render_move_path` batches** consecutive same-style segments into single multi-point `create_line` calls (big win on arc-heavy files; no-op for single-segment moves).
- **Dead code removed**: `_chain_segments`, `_render_segments`, `_render_bit_cylinder` (abandoned 3D-cylinder attempt).

Static-vs-dynamic split: `_view_dirty` flag controls full redraw vs. updating only the `"dynamic"`-tagged items (tool, Z bar, HUD text). Pan/zoom use `canvas.move`/`canvas.scale` for cheap transforms; only rotation triggers `_view_dirty=True`.

**Projection cache.** `_update_proj_cache()` precomputes `cos`/`sin` of `rot_x` and `rot_z`. `project()` is on the hot path — call it many times per frame. Don't add per-call trig there.

**Parser.** `parse()` is a single-pass line-by-line state machine tracking modal G-code: position (x/y/z), feed, spindle (M3/M5), distance mode (G90/G91), and current motion mode (G0/G1/G2/G3). Arcs (G2/G3) are linearized in `arc_points` using I/J center offsets — R-form arcs are not handled. Comments in `(...)` and after `;` are stripped in `clean()`.

**Overlap heatmap.** `_compute_overlap_grid` rasterizes cutting moves into a 1mm grid, counting how many bit-radius-thick segments touch each cell. Only cells where Z<0 (in material) count. The result is cached in `_overlap_grid_cache` and invalidated when bit width changes or a new file is loaded.

**`min_cut_z`.** Set after load to the deepest cutting Z (or -1 fallback). `depth_color()` uses it to scale the green→yellow→orange→red→purple gradient — the deepest cut always shows as purple regardless of absolute depth.

### `gcode_viewer_v2/` — VTK + PyQt5, the active viewer

```
gcode_viewer_v2/
├── app.py              ← entry point with two-phase splash
├── parser.py           ← G-code parser (clean port of gcode_preview.py's logic, no UI)
├── scene/              ← VTK actors / pipelines
│   ├── path.py         ← cut + rapid line-plot polydata
│   ├── stock.py        ← stock-block outline (translucent vtkCubeSource)
│   ├── tool.py         ← realistic CNC bit (5-part vtkAssembly: flute + helix + band + shank + LED)
│   ├── removal.py      ← heightmap material-removal sim + overlap heatmap
│   └── view_cube.py    ← interactive labeled view cube (chamfered corners → iso-snap)
├── ui/
│   ├── main_window.py  ← QMainWindow + QVTK widget + animation timer
│   ├── controls.py     ← custom Z-axis indicator widget
│   └── debug_window.py ← floating debug pane (FPS, system info, captured stdio)
├── assets/splash.png
├── bench/              ← FPS benchmark suite (13 benchmarks + runner) — dev only
├── requirements.txt    ← vtk, PyQt5, numpy
└── pyinstaller.spec    ← builds GcodeSimV1.exe
```

**Active perf optimizations** (already applied; don't undo):

1. **Heightmap cell size scales with bit diameter** — `cell_size = max(1.0, bit_diameter / 4.0)` in `MainWindow.load_moves`. Caps mesh size on big-bit/big-stock workloads with no visible loss (a bit physically can't cut features finer than ~bit/4).
2. **Heightmap surface uses flat shading** (`SetInterpolationToFlat`) instead of `vtkPolyDataNormals` — saves 5-10 ms per refresh on big meshes and reads more honestly as a "machined" surface. `make_stock_surface_actor` returns `(actor, None)` (the second tuple element used to be a normals filter; callers `if normals is not None: ...` correctly).
3. **Two-layer rendering** with `SetAlphaBitPlanes(1) + SetMultiSamples(0)` — main scene at layer 0, "Highlight Path" overlay at layer 1, view-cube widget at layer 2. The MSAA-off side effect actually makes layered rendering FASTER than non-layered (verified in bench_09 vs bench_08).
4. **Tool actor is a `vtkAssembly`** of 5 named parts (`._flute`, `._helix`, `._band`, `._shank`, `._led`). `update_tool_position(actor, x, y, z, spindle_on)` translates the assembly via `SetUserTransform` (cheap) and flips the LED color. Bit does NOT spin visually — that experiment cost too many ms per frame on integrated GPUs.
5. **VTK output window replacement at module load** — `vtk.vtkOutputWindow.SetInstance(vtk.vtkOutputWindow())` in `main_window.py` swaps the platform-default Win32 popup for a stderr-routing instance. Don't remove this; the popup is annoying and unhelpful.

**View cube** (`scene/view_cube.py`): chamfered cube with 6 main octagonal faces + 8 corner triangles. Hover highlights a face/corner; click snaps the main camera to that view. Corner clicks snap to iso views (e.g., `CORNER_+1_-1_+1` snaps to TOP+FRONT+RIGHT iso). Lives in its own corner-viewport renderer (layer 2) with its camera mirrored from the main camera via a `ModifiedEvent` observer. **Don't replace with `vtkAnnotatedCubeActor`** — its faces aren't individually pickable.

**Debug window** (`ui/debug_window.py`): lazy-constructed (Debug → Open Debug Window…). Pipes stdout/stderr through a `_TeeStream` so VTK warnings + Python prints land in the log. Adds a render observer to count FPS. Verified not to be the FPS-drop cause; safe to leave open.

**Bench suite** (`bench/`): 13 standalone benchmarks isolating each rendering variable. `python -m gcode_viewer_v2.bench.run_all` runs the full set. Useful for validating perf claims before/after a change. Don't ship in the .exe (already excluded via PyInstaller's static analysis since it doesn't import them).

### Cross-cutting things that have already been investigated

- **3.8 fps → 62 fps fix**: was the heightmap normals filter + 1-mm-everywhere cell size on big stocks. The two perf fixes above (cell scaling + flat shading) brought the same workload to 62 fps in `bench_13`.
- **The debug window is NOT the cause** of FPS drops; bench_11 confirmed it adds <1 fps overhead.
- **Layered rendering is NOT slow** on this stack — the alpha-bit-planes + multisamples=0 combo is actually faster than single-layer (bench_09 vs bench_08).
- **The bit assembly is NOT slow** — bench_03 (5-actor assembly) is 148 fps vs bench_02 (single cylinder) at 148 fps. The flute helix lines are basically free.
- **Bit spin animation is dropped intentionally** — even at low rev/sec it cost too much per frame on the user's hardware. LED color is the spindle indicator.
- **Camera controls** are stock `vtkInteractorStyleTrackballCamera`: left = orbit, shift+left = pan, ctrl+left = roll, ctrl+shift+left = dolly, middle = pan, right = dolly, wheel = dolly. View-cube hover/click coexists.

## Building the .exe binaries

`gcode_viewer_v2/pyinstaller.spec` is the source of truth. From inside `gcode_viewer_v2/`:

```
pyinstaller pyinstaller.spec --noconfirm --clean
```

Output: `gcode_viewer_v2/dist/GcodeSimV1.exe` (~180-220 MB, single file, no end-user installs).

The spec uses `collect_submodules('gcode_viewer_v2')` to force-bundle the entire package. `pathex` includes both the spec dir AND its parent so `from gcode_viewer_v2.X import Y` resolves at analysis time. Don't change either of those without understanding the "No module named 'gcode_viewer_v2'" PyInstaller pitfall this guards against.

The bootstrap in `app.py` distinguishes source mode from frozen mode:
```python
if getattr(sys, "frozen", False) and hasattr(sys, "_MEIPASS"):
    pkg_root = sys._MEIPASS
else:
    pkg_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
```
That's load-bearing for the frozen build — don't simplify it back to the source-only path.

The legacy `exe/gcode_preview.exe` was a PyInstaller build of `gcode_preview.py` from before v2 existed. The editor `exe/gcodegenV1.0.exe` is similarly built from `gcodegen.py`. Both are kept because they're known-working snapshots of the Tk era.

## Things the README flags as deliberately out of scope

No tool radius compensation, no G2/G3 emission (input parsing only), no machine-specific post-processing, no bounds checking, no collision detection, no undercuts in the v2 heightmap (3-axis routing only). Don't add these speculatively — wait for the user to ask.
