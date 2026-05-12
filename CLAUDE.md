# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

Five apps live here, in two generations of viewer + two generations of
editor. **`gcode_viewer_v3/`** and **`gcode_generator_v2/`** (both Go)
are the **active development targets** — separate modules so each ships
as its own self-contained binary with no shared dependency surface. The
older Python apps (v1 editor + v1/v2 viewers) are kept working but are
not where new features should land unless asked.

- `gcodegen.py` — **Editor v1.** Tkinter sketchpad: draw freehand toolpaths, set per-stroke depths + machine settings, export `.nc`. Stdlib-only. Has its own in-app simulator and finished-product preview windows. Reference implementation for v2 (Go).
- `gcode_preview.py` — **Legacy Tk viewer.** Earlier Tkinter-based G-code viewer with a hand-rolled 3D projection on a 2D Canvas. Performance-fixed in Stage 1 of the v2 refactor (see Stage-1 notes below). Kept around for users who can't run VTK.
- `gcode_viewer_v2/` — **Reference viewer (Python).** VTK + PyQt5 with real 3D, material-removal heightmap simulation, splash, debug window, view cube, etc. Ships as `GcodeSimV1.exe` via PyInstaller (~200 MB). The Go port (v3) is a port of this — when something is missing in v3, look at v2 for the spec.
- `gcode_viewer_v3/` — **Active viewer (Go).** Pure-Go rewrite using the [g3n](https://github.com/g3n/engine) engine. Same features as v2 (toolpath, stock, end-mill, view cube, material-removal heightmap with through-cut, playback controls, options dropdown, embedded tutorials), but ships as a single ~5 MB `.exe` (vs ~200 MB for v2). **Shared Go source under `cmd/` + `internal/`; platform-specific build scripts and resources live in dedicated `windows/` and `mac/` subfolders.** Module: `gcodegen.local/viewer`.
- `gcode_generator_v2/` — **Active editor (Go).** 1:1 port of `gcodegen.py` using **Ebiten + ebitenui** (chosen over g3n because the editor is fundamentally a 2D paint canvas with a heavy entry-form right panel — g3n's pixel-pushed `gui.*` widgets are the wrong shape for that workload). Same shape as v3: `cmd/` + `internal/` + `windows/` + `mac/`. Module: `gcodegen.local/generator`. See the dedicated section below.

There is no shared library between root-level scripts and v2; the v2 viewer's `parser.py` is a clean port of the root scripts' parsing logic with no UI deps. v3's parser is a 1:1 port of v2's.

## Running

```
python gcodegen.py                                              # editor (Tk, stdlib-only)
python gcode_preview.py                                         # legacy Tk viewer
python -m gcode_viewer_v2.app                                   # v2 VTK viewer (run from project root)
cd gcode_viewer_v3 && go run ./cmd/gcodesim                          # v3 Go viewer (dev — quick iteration)
cd gcode_viewer_v3 && go test ./internal/parser/...                  # v3 parser golden tests
cd gcode_viewer_v3/windows && .\build.bat                            # v3 viewer (Windows release → ../gcodesim.exe)
cd gcode_viewer_v3/mac && ./build.sh && open ../GcodeSimV3.app       # v3 viewer (macOS release → ../GcodeSimV3.app)
cd gcode_generator_v2 && go run ./cmd/gcodegen                       # v2 Go editor (dev)
cd gcode_generator_v2 && go test ./internal/gen/...                  # v2 emitter+model tests
cd gcode_generator_v2/windows && .\build.bat                         # v2 editor (Windows release → ../gcodegen.exe)
cd gcode_generator_v2/mac && ./build.sh && open ../GcodeGenV1.app    # v2 editor (macOS release → ../GcodeGenV1.app)
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

### `gcode_viewer_v3/` — Go + g3n, the active viewer

```
gcode_viewer_v3/
├── go.mod / go.sum             module gcodegen.local/viewer (local — not published)
├── windows/                    Windows-only build assets
│   ├── build.ps1               one-shot Windows build (go build -ldflags='-s -w -H windowsgui')
│   ├── build.bat               PowerShell exec-policy wrapper around build.ps1
│   ├── versioninfo.json        goversioninfo input → resource_windows_*.syso
│   └── icon.ico                embedded into the .exe by goversioninfo
├── mac/                        macOS-only build assets
│   ├── build.sh                one-shot macOS build → universal .app bundle (arm64+amd64)
│   ├── Info.plist              macOS bundle metadata + .nc/.gcode/.tap file association
│   └── icon.ico                converted to .icns by build.sh, copied into .app
├── cmd/
│   ├── gcodesim/main.go        production entry — calls ui.Run()
│   └── g3n_smoke/main.go       single-file API skeleton (build-tag `smoke`, excluded by default)
├── internal/
│   ├── parser/                 1:1 port of v2's parser.py + golden tests vs sample.nc
│   ├── scene/
│   │   ├── colors.go           5-stop depth gradient lerp
│   │   ├── path.go             toolpath: cut LineStrips (depth-graded) + dashed rapid Lines
│   │   ├── stock.go            stock outline (NewStockWireframe — translucent fill removed)
│   │   ├── tool.go             5-part end-mill assembly (flute / helix / band / shank / LED)
│   │   ├── view_cube.go        chamfered cube with edge outlines + text labels per face
│   │   ├── labels.go           bundled FreeSansBold.ttf + texture builder for face labels
│   │   ├── playback.go         time-driven Move advance with arc-length interpolation
│   │   ├── removal.go          heightmap material removal (flat-shaded triangulation)
│   │   └── FreeSansBold.ttf    embedded font (~400 KB) for view-cube labels
│   ├── ui/
│   │   ├── window.go           App, scene, camera, animation tick, key handlers
│   │   ├── orbiter.go          custom Z-up orbit controller (replaces g3n's Y-up OrbitControl)
│   │   ├── toolbar.go          two-row toolbar + Options + Tutorials dropdown panels
│   │   ├── tutorials.go        //go:embed tutorials/*.nc  → bundled tutorial dropdown
│   │   ├── tutorials/*.nc      6 runnable .nc files baked into the binary
│   │   ├── window_icon.go      //go:embed icon.png + GLFW SetIcon for title bar
│   │   ├── icon.png            single-source-of-truth title-bar icon (NOT in windows/ or mac/ — those would drift out of sync)
│   │   ├── settings.go         persisted skipped-update-versions (UserConfigDir)
│   │   ├── update_prompt.go    GitHub-Releases-API update check + native Yes/No dialogs
│   │   ├── openfile_*.go       darwin: Apple Event handler for double-clicked .nc; other: stub
│   │   ├── register_*.go       windows: HKCU\Software\Classes file association; other: stub
│   │   └── dialogs.go          sqweek/dialog file open + error message
└── (gcodesim.exe / GcodeSimV3.app  — gitignored build artifacts)
```

**Shared Go source, dedicated platform folders.** All Go source lives in `cmd/` + `internal/` and compiles on both platforms — build-tagged `*_darwin.go` / `*_windows.go` files split platform-specific logic. Build scripts and platform resources are isolated in `windows/` and `mac/`, so each platform's folder is self-contained. The build scripts `cd ..` to the project root before invoking `go build`/`go mod tidy`/`git describe`, and reference their inputs (icon, JSON, plist) via the script's own folder. Outputs (`gcodesim.exe`, `GcodeSimV3.app`, `GcodeSimV3.app.zip`) land in the parent `gcode_viewer_v3/` folder so the `gh release upload` recipe in the README still works.

**Embedded tutorials.** `internal/ui/tutorials.go` uses `//go:embed tutorials/*.nc` to bundle 6 starter programs into the binary. The toolbar has a "Tutorials ▾" dropdown that lists them; clicking one parses the embedded bytes through `loadBytes` (same code path as `loadFile`, just no disk I/O). The directory must live INSIDE the package using the embed directive — that's why `tutorials/` lives at `internal/ui/tutorials/` rather than the repo root.

**Key architectural choices that took real debugging to find:**

- **No `engine/app`.** `app.App()` transitively imports `engine/audio/al` and `engine/audio/vorbis`, which embed `cgo LDFLAGS: -lOpenAL32 -lvorbis`. The resulting binary needs `OpenAL32.dll` + `libvorbis.dll` at startup — Windows refuses to load it with "this app can't run on your PC". We bypass app and call `window.Init` + `renderer.NewRenderer` directly. Saves ~3 DLLs from the distribution. **Don't reintroduce app.App().**

- **Custom `Orbiter` (Z-up), not `camera.OrbitControl`.** g3n's OrbitControl has a private `up` field hard-coded to `(0, 1, 0)`. Every `Rotate` re-orients the camera with Y vertical, fighting our CNC-convention Z-up world. Symptoms: view-cube widget feels reversed, hover picks wrong faces. Our `Orbiter` uses (yaw, pitch, distance) about a target with fixed world-Z up. Same controls (left-drag rotate, right-drag pan, scroll zoom).

- **View cube uses perspective, not ortho.** g3n's `Raycaster.SetFromCamera` builds a single-origin convergent ray (correct for perspective, wrong for ortho — ortho needs parallel rays). Picking on an ortho cube cam returned wrong faces for off-center clicks. Switched cube cam to narrow-FOV (30°) perspective; visually still reads as orthographic but picking is correct.

- **Mouse events through `gui.Manager().SubscribeID`, not `win.Subscribe`.** gui.Manager only forwards mouse events to non-panel subscribers when no GUI panel is under the cursor. Subscribing the orbiter via gui.Manager means dragging the speed/progress sliders or clicking buttons doesn't also rotate the camera. `SetCursorFocus(o)` on mouse-down keeps cursor events flowing to the orbiter for the rest of a drag even if the cursor wanders over a panel.

- **Heightmap uses flat shading via vertex duplication.** Each triangle owns its own three vertex copies sharing one face normal — no averaging across seams. Cut walls render as crisp facets (the "rigid machined" look). Cost is ~6× more vertices but still fast on typical grids (~25–110K verts).

- **Material thickness ("through cut") via `through[]` markers + dropped quads.** `Cut()` flags cells whose Z reaches `-MaterialThickness`. `RefreshMesh` emits degenerate triangles for fully-through quads, which the GPU draws as nothing — produces real holes in the mesh, so cutout parts visually separate from the surrounding stock.

- **Toolbar dropdown panels are SIBLINGS of the toolbar (not children).** g3n clips child panels to parent bounds. The Options + Tutorials dropdowns extend below the 64-px toolbar so they'd be clipped to invisibility if parented. We expose `Toolbar.OptionsPanel` + `Toolbar.TutorialsPanel` and the caller adds them to `sceneRoot` separately. Toggle behavior: opening one closes the other (avoids visual overlap).

- **g3n texture FlipY is on by default.** The Standard vertex shader does `texcoord.y = 1.0 - texcoord.y` when the texture's FlipY flag is set. Our font.DrawText image already has row 0 at top, so we call `tex.SetFlipY(false)` on label textures. Without this, view-cube labels render upside down.

- **`material.Basic` IGNORES textures.** Its fragment shader is literally `FragColor = vec4(Color, 1.0)`. Any `mat.AddTexture` call on Basic is silently dropped. Use `material.Standard` for textured meshes (with vertex normals so lighting works).

### `gcode_generator_v2/` — Go + Ebiten, the active editor

1:1 port of `gcodegen.py` in its own Go module (`gcodegen.local/generator`), separate from the viewer (`gcodegen.local/viewer`). GUI stack is **Ebiten + ebitenui** rather than g3n — the editor is fundamentally a 2D paint canvas with a heavy entry-form right panel, and g3n's pixel-pushed `gui.*` widgets are the wrong shape for that workload.

```
gcode_generator_v2/
├── go.mod / go.sum             module gcodegen.local/generator
├── cmd/gcodegen/main.go        argv dispatch: editor | sim | preview
├── internal/
│   ├── assets/                 //go:embed FreeSansBold.ttf
│   ├── version/                build-time version stamp + GitHub release check
│   ├── gen/                    pure data model + G-code emitter (no UI deps)
│   │   ├── model.go            Editor struct + state methods
│   │   ├── coords.go           PxToMM/MMToPx + hit tests (1:1 of gcodegen.py:30-150)
│   │   ├── ops.go              OpsMM/OpsPx/AllStrokesPx (gcodegen.py:293-327)
│   │   ├── presets.go          MAT_MACHINE_PRESETS + 7-color palette
│   │   ├── emit.go             Emit(ops, machine) string — byte-identical to gcodegen.py:380-405
│   │   ├── ipc.go              UpdateMessage JSON wire format
│   │   └── *_test.go           25 unit tests; emit_test.go golden against testdata/*.nc
│   ├── editor/                 main editor Ebiten game
│   ├── sim/                    toolpath simulation subprocess (animated playback)
│   ├── preview/                finished-product preview subprocess (textured surface render)
│   └── shared/                 settings.go (~/UserConfigDir/GcodeGen/settings.json)
├── windows/                    build.ps1 + build.bat + versioninfo.json + icon.ico → ../gcodegen.exe
└── mac/                        build.sh + Info.plist + icon.ico → ../GcodeGenV1.app
```

**Single binary, three modes** (Ebiten is single-window — no `NewWindow` API; multi-window only via subprocess):
- `gcodegen` — main editor window (1024×600). Argv-less.
- `gcodegen sim` — toolpath simulation subprocess. Reads `UpdateMessage` JSON lines from stdin.
- `gcodegen preview` — finished-product preview subprocess. Same stdin protocol.

The editor spawns the two aux processes via `os/exec`, holds their stdin pipes, and broadcasts a snapshot every 3 frames (~20 Hz throttle). Preview is auto-spawned at editor startup (mirrors `window.after(120, open_preview)` in the Python). When an aux window is closed, `cmd.Wait()` in a goroutine clears the editor's pointer so the next click respawns.

**Decisions worth remembering before editing:**

- **Separate Go module**, not a sibling binary in v3. The two apps have completely different deps (g3n+OpenGL vs Ebiten+ebitenui+sqweek/dialog) and no shared internal API; isolating modules keeps each app's dep tree small. The font (FreeSansBold.ttf) is duplicated across `gcode_viewer_v3/internal/scene/` and `gcode_generator_v2/internal/assets/` — that's 800 KB total, acceptable cost for module independence.
- **Ebiten + ebitenui, not Fyne / Gio.** Ebiten is purpose-built for 2D drawing (freehand strokes are native via `vector.StrokeLine`); ebitenui supplies the form widgets the right panel needs (`Container`, `RowLayout`, `TextInput`, `Slider`, `Checkbox`, `ScrollContainer`, `ProgressBar`, `Button`).
- **Subprocess multi-window.** Ebiten v2 explicitly cannot call `RunGame` twice in one process. The user wanted aux windows as separate OS windows (not in-app overlays), so each "window" is a re-exec of the same binary in a different mode. Boot cost ~300-800 ms on first launch is acceptable for a desktop CAM tool.
- **Mouse polling, not event callbacks.** Ebiten only exposes pressed / just-pressed / just-released via `inpututil`. The press/drag/release/motion handlers in `internal/editor/input.go` look very different from gcodegen.py's Tk callbacks, but the state mutations they perform are identical.
- **Trails image vs ghosts image** in sim. Ghosts is rebuilt only when ops change; trails is appended to as playback advances. Avoids redrawing every cut every frame at 60 fps.
- **Format strings matter for `Emit`.** Python's `f"{depth}"` (default `__str__`) gives `"-1.0"` for -1.0; Go's `strconv.FormatFloat('g', -1)` gives `"-1"`. The `pyFloat` helper appends `.0` when the formatted value has no decimal/exponent. The golden tests pin byte-identity against `testdata/perim_only.nc`, `single_stroke.nc`, `three_strokes.nc`.
- **NineSlice solid colors** for button/slider/checkbox states. Avoids bundling 12 PNG assets just for flat fills.
- **No file association on macOS** for the editor (no `UTExportedTypeDeclarations` in `mac/Info.plist`). The editor exports `.nc`, never opens it; v3's viewer owns the .nc UTI. Distinct bundle IDs (`com.nightbots10686.gcodesimv3` vs `com.nightbots10686.gcodegenv1`) so both `.app`s can coexist on the same Mac.

### Deferred work — polygon-union renderer

**Don't re-derive this from scratch when a user complains about jagged corners.** A full design exists at [`docs/POLYGON_RENDERER.md`](docs/POLYGON_RENDERER.md) for replacing the heightmap's display geometry with mathematically exact swept-stadium polygons (real bit-radius fillets at internal corners, real roundings at external corners). The user has seen the design and chose to defer implementation, possibly after testing simpler fixes (cell-size shrink + finer toolpath sampling) first.

If a user complains about cut-edge quality:
1. Read `docs/POLYGON_RENDERER.md` — full root-cause analysis is in §1, alternatives ranked in §11.
2. The two simpler fixes that might be tried before the polygon renderer: tighten `HeightmapCellSize` to `bit_diameter/16` (floored at 0.25 mm, with a hard cell-count cap), and tighten `CutSegment`'s `step` from `bitRadius/2` to `bitRadius/4`. Both are local edits in `internal/scene/removal.go`.
3. If those don't satisfy, follow the M1–M7 milestones in the design doc.

The heightmap stays in the codebase regardless — it handles Z-varying moves (plunge ramps, 3D surfacing) that the swept-stadium model can't represent.

### Cross-cutting things that have already been investigated

- **Jagged cut corners + non-rounded external corners** are caused by heightmap cell discretization (bit_diameter/8 floored at 0.4 mm gives only ~4 cells per bit radius for a 6 mm bit). Combined with flat shading, the bit's circular footprint reads as a chunky octagon. Real fix is the polygon-union renderer in `docs/POLYGON_RENDERER.md`.
- **3.8 fps → 62 fps fix**: was the heightmap normals filter + 1-mm-everywhere cell size on big stocks. The two perf fixes above (cell scaling + flat shading) brought the same workload to 62 fps in `bench_13`.
- **The debug window is NOT the cause** of FPS drops; bench_11 confirmed it adds <1 fps overhead.
- **Layered rendering is NOT slow** on this stack — the alpha-bit-planes + multisamples=0 combo is actually faster than single-layer (bench_09 vs bench_08).
- **The bit assembly is NOT slow** — bench_03 (5-actor assembly) is 148 fps vs bench_02 (single cylinder) at 148 fps. The flute helix lines are basically free.
- **Bit spin animation is dropped intentionally** — even at low rev/sec it cost too much per frame on the user's hardware. LED color is the spindle indicator.
- **Camera controls** are stock `vtkInteractorStyleTrackballCamera`: left = orbit, shift+left = pan, ctrl+left = roll, ctrl+shift+left = dolly, middle = pan, right = dolly, wheel = dolly. View-cube hover/click coexists.

## Building the .exe binaries

The canonical build path is `build.bat` at the repo root — it `pip install`s deps, runs PyInstaller against the spec, and copies the result to `exe/GcodeSimV1.exe` for distribution. To build the viewer manually, from inside `gcode_viewer_v2/`:

```
pyinstaller pyinstaller.spec --noconfirm --clean
```

Output: `gcode_viewer_v2/dist/GcodeSimV1.exe` (~180-220 MB, single file, no end-user installs). `build.bat` publishes a copy to `exe/GcodeSimV1.exe`, which is **gitignored** — the viewer binary ships via GitHub Releases (it's over GitHub's per-file size limit). Only the smaller editor binary `exe/gcodegenV1.0.exe` is committed.

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
