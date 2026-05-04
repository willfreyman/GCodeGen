# GcodeSim — User Manual

A 3D simulator for CNC G-code programs. Loads any common `.nc` /
`.gcode` / `.ngc` / `.tap` etc. file, plays the toolpath through an
animated end-mill, and removes material from a stock block in real
time so you can sanity-check a program before sending it to a real
machine.

This manual covers everything in the running app — every button,
slider, mouse gesture, keyboard shortcut, and the math behind the
material-removal heightmap. For build / release / development docs
see [`CLAUDE.md`](../CLAUDE.md) at the repo root.

---

## Contents

1. [Getting started](#1-getting-started)
2. [Interface overview](#2-interface-overview)
3. [Toolbar — every control explained](#3-toolbar--every-control-explained)
4. [Options panel](#4-options-panel)
5. [Mouse + keyboard reference](#5-mouse--keyboard-reference)
6. [The view cube](#6-the-view-cube)
7. [Material removal — what's actually happening](#7-material-removal--whats-actually-happening)
8. [Through-cut (material thickness)](#8-through-cut-material-thickness)
9. [File formats](#9-file-formats)
10. [Update prompt](#10-update-prompt)
11. [Settings file](#11-settings-file)
12. [Troubleshooting](#12-troubleshooting)
13. [Known limitations](#13-known-limitations)

---

## 1. Getting started

Download the binary for your platform from the [Releases page][rel]:

* **Windows**: `gcodesim.exe` — double-click it.
* **macOS**: `GcodeSimV3.app.zip` — double-click to unzip, then
  right-click `GcodeSimV3.app` → **Open** (one-time Gatekeeper bypass
  since we're unsigned). Subsequent launches are normal double-clicks.

After launch you get an empty window with a dark background, a
two-row toolbar at top, and a small chamfered cube widget in the
top-right corner. Click **Open .nc...** (or press `Ctrl+O` /
`Cmd+O`) to load a G-code file.

Try the included [tutorials](../tutorials/) for a walk through the
features.

[rel]: https://github.com/willfreyman/GCodeGen/releases

---

## 2. Interface overview

```
┌──────────────────────────────────────────────────────────────────────┐
│ [Open .nc...] [Play] [Reset] [R] | Speed: ◯───◯ 1.0× | Bit: 6.35  ┤
│                                  | mm [Set] [Options ▾]             │
│ ◯─────────────────────────────────────────────────◯ 42%             │
├──────────────────────────────────────────────────────────────────────┤
│                                                          ┌────────┐ │
│                                                          │  TOP   │ │
│           [3D scene: stock outline, toolpath,            │FRONT |R│ │
│            tool, carved heightmap surface]               │   IGHT │ │
│                                                          └────────┘ │
│                                                                     │
│                                                                     │
│                                                                     │
└──────────────────────────────────────────────────────────────────────┘
```

* **Top toolbar (2 rows)** — file/playback/setup controls (row 1) and
  the slidable progress bar (row 2)
* **3D viewport** — orbits/pans/zooms with the mouse; shows stock
  outline, toolpath polylines, end-mill assembly, and the carved
  heightmap surface
* **View cube** — top-right corner, ~90 px square; click any face or
  corner to snap the camera to a standard view

---

## 3. Toolbar — every control explained

### Row 1

| Control | What it does |
|---|---|
| **Open .nc...** | Native file-open dialog. Filters for the common G-code extensions (see [§9](#9-file-formats)). Same as `Ctrl+O` / `Cmd+O`. |
| **Play / Pause** | Toggles playback. Same as `Spacebar`. Auto-pauses when the program reaches the end. |
| **Reset** | Returns playback to the very first move and rebuilds the heightmap (so the carved surface starts un-carved). Use this after changing the bit diameter mid-run if you want a properly-resolved fresh stock. |
| **R** | Reframes the camera to fit the model bounds. Useful after orbiting or panning far away. Same as the `R` keyboard shortcut. |
| **Speed slider** | Playback multiplier from **0.5× to 50×** on a logarithmic scale. The default position (~15% from the left) corresponds to 1×. The numeric label to the right shows the exact current value (e.g., `1.0x`, `5.0x`, `25.0x`). |
| **Bit edit + Set** | Tool diameter in mm. Type a number, press Enter or click **Set** (or click outside the box) to apply. The end-mill model rebuilds at the new size. The heightmap cell-size formula is `max(0.4 mm, bit_dia / 8)`, so smaller bits give you a finer mesh on the next Reset. |
| **Options ▾** | Drops down a panel below the toolbar with extra settings (currently: Material thickness — see [§4](#4-options-panel)). |

### Row 2

| Control | What it does |
|---|---|
| **Progress bar (full-width)** | Drag to scrub forward or backward through the program. The carved surface and tool position update live. Cuts replay correctly when scrubbing forward (re-cuts) AND backward (resets and replays from start). The percentage on the slider shows how far through the program you are. |

---

## 4. Options panel

Opens via the **Options ▾** button. Currently has one setting:

### Material thickness

Sets the bottom of the stock at `-thickness` (mm).

* **0 (default)** — through-cut disabled. Cuts deeper than 0 just
  carve a deeper depression in the heightmap surface.
* **Any positive value** — cuts that reach `-thickness` mark cells as
  "through". Fully-through quads are dropped from the rendered mesh,
  producing real holes in the surface where the part separates. This
  is the "cut a part out of stock" workflow.

**Input formats accepted:**
* `19.05` — interpreted as mm
* `19.05mm` — explicit mm
* `0.75in` or `0.75"` — inches, auto-converted to mm

The Apply button (or pressing Enter / clicking outside the box)
commits the value. Empty / `0` disables through-cut.

After changing material thickness, **hit Reset** if you want existing
cuts re-evaluated against the new bottom.

---

## 5. Mouse + keyboard reference

### Mouse (3D viewport)

| Gesture | Action |
|---|---|
| Left-drag | Orbit camera around the focal point |
| Right-drag | Pan the focal point (and camera with it) |
| Middle-drag | Pan (alias for right-drag) |
| Scroll wheel | Zoom in / out |
| Click on view cube face | Snap camera to that face's orthogonal view |
| Click on view cube corner | Snap camera to the iso view showing the 3 adjacent faces |
| Hover over view cube face | Highlights that face in soft-blue |

Dragging on the toolbar / sliders does NOT also rotate the camera —
events are routed via g3n's gui manager, which absorbs clicks that
land on a panel.

### Keyboard

| Key | Action |
|---|---|
| `Ctrl+O` (Win) / `Cmd+O` (Mac) | Open file |
| `Spacebar` | Play / Pause |
| `R` | Reframe camera |
| `Esc` | Quit |

(Mac `Cmd+O` may not work yet — g3n's modifier handling is Ctrl-based;
use `Ctrl+O` on Mac too if `Cmd+O` doesn't fire. Toolbar buttons
always work as fallback.)

---

## 6. The view cube

Small chamfered cube in the top-right corner of the window.
Six octagonal main faces (TOP, BOTTOM, FRONT, BACK, LEFT, RIGHT) plus
eight triangular corner faces. Used for camera navigation.

* **Hover** any face / corner → it highlights soft-blue.
* **Click** a main face → main camera snaps to look directly at that
  face. (E.g., click TOP → camera moves to look straight down.)
* **Click** a corner triangle → main camera snaps to the **isometric
  view** that shows the three adjacent main faces simultaneously
  (e.g., the +X+Y+Z corner gives the classic TOP+FRONT+RIGHT iso).

The cube continuously syncs with the main camera — as you orbit, the
cube rotates in lock-step so its orientation reflects what you're
actually looking at.

The cube doesn't have text labels yet — color is your guide:
- Blue shades = TOP/BOTTOM (Z axis)
- Green shades = BACK/FRONT (Y axis)
- Red shades = RIGHT/LEFT (X axis)

(Labels are TODO; see CLAUDE.md if you want to add them — needs a
bundled TTF and per-face texture-quad transforms.)

---

## 7. Material removal — what's actually happening

The carved surface is a **2D heightmap** — a grid of cells covering
the toolpath's XY footprint, where each cell stores the current
top-of-material Z value.

**Cell size** is derived from the bit diameter:
```
cell_size = max(0.4 mm, bit_diameter / 8)
```
So a 6 mm bit gives 0.75 mm cells; a 1 mm bit gives 0.4 mm cells (the
floor). Smaller cells = finer detail but more vertices to draw.

**Cutting:** every animation tick, the simulator walks every move the
playback advanced through (in arc-length order, respecting the
linearized polylines for G2/G3 arcs). For each cutting move (G1/G2/G3
with spindle on), it samples the segment at `bit_radius / 2` spacing
and at each sample point lowers every cell within `bit_radius` of the
tool axis to the tool's Z (if the cell was higher before).

**Rendering:** the heightmap surface uses **flat shading via vertex
duplication** — each triangle owns its own three vertex copies with a
single face normal. Adjacent triangles don't share normals, so cut
walls render as crisp facets rather than melted blobs. The trade-off
is ~6× more vertices for the same grid, but it stays well under a
millisecond per refresh on typical sizes.

**Refresh rate:** the heightmap mesh re-uploads to the GPU every 4
animation ticks (~15 Hz at 60 fps). Cuts are recorded every tick;
visual refresh is throttled to keep the GPU happy.

**Reset** rebuilds the entire heightmap and walks every cut from move
0 to the current playback position. Same path is used by the progress
slider scrub — drag the bar backward and the surface re-cuts from
scratch up to the new position (the heightmap doesn't store history,
just current state).

---

## 8. Through-cut (material thickness)

Set via the Options panel ([§4](#4-options-panel)). When non-zero:

* The "bottom of stock" is at Z = `-thickness`.
* Any cut that reaches the bottom marks the cell as **through** (a
  parallel boolean array tracks this per cell).
* During mesh refresh, quads where ALL FOUR corner cells are through
  are emitted as **degenerate triangles** (zero-area), which the GPU
  draws as nothing → real holes appear in the mesh.
* Quads on the BOUNDARY (some corners through, some not) render
  normally — this forms the side walls of the cutout.

Visual result: cut-out parts visually separate from the surrounding
stock, just like in the real CNC where the part falls out (or stays
in place via tabs you didn't add).

**Limitation:** the through-cut is determined per-cell, so cutout
edges are jagged at the cell-size resolution. Using a smaller bit
(finer cells) gives crisper cutout edges.

---

## 9. File formats

Recognized extensions, in priority order:

* `.nc` — universal CAM output
* `.gcode` — RepRap / 3D-printer slicers, also some CAM
* `.ngc` — LinuxCNC, GRBL
* `.tap` — Mach3 default
* `.cnc` — generic CAM
* `.gco` — Cura and other 3D-printer slicers
* `.g` — older / generic
* `.mpf` — Siemens controllers
* `.nci` — Mastercam intermediate
* `.tab` — Mach3 alternative
* `.eia` — paper-tape-era controllers
* `.dnc` — DNC transfers
* `.txt` — plain-text CAM dumps

The parser is **extension-agnostic** — only file content matters.
Extensions only affect what's listed in the file picker filter and
what double-click associates with on macOS.

### Supported G-code dialect

* **Motion**: G0 (rapid), G1 (linear), G2 (CW arc), G3 (CCW arc) —
  all in 3 axes (X/Y/Z)
* **Modes**: G90 (absolute), G91 (relative), G17 (XY plane), G21 (mm)
* **Spindle**: M3 (on, optionally with `S<rpm>`), M5 (off)
* **Program end**: M30
* **Feed**: F values (mm/min); rapids use a fixed 2500 mm/min for
  simulation timing
* **Comments**: `(parens)` and `;semicolon` to end of line
* **Arcs**: I/J center-offset form (NOT R-form — R-form arcs become
  silently degenerate)

### NOT supported (deliberately out of scope)

* G4 dwell, G28/G30 home, G92 work offsets — silently ignored
* Macros / variables (#1 = ...) — ignored
* M6 tool change — ignored (use the bit-diameter edit + Reset to
  manually swap bits)
* Probing cycles (G38.x) — ignored
* Subprograms (M98/M99) — ignored

---

## 10. Update prompt

On launch, the app asynchronously pings GitHub's Releases API to
check for a newer version (5-second timeout, silent fail on offline /
slow network). If a newer release is found:

1. **Title bar** updates immediately with `→ V3.0.X available`
   appended.
2. **Modal dialog** appears asking "Open the download page in your
   browser?"
   * **Yes** → default browser opens at the specific release tag's
     page on GitHub.
   * **No** → second dialog asks "Hide this update notice until a
     newer version is released?"
     * **Yes** → version is added to the persisted skip list (see
       [§11](#11-settings-file)). Won't prompt again for this version
       until a newer one ships.
     * **No** → no action; will re-prompt on next launch.

The update check is skipped entirely for `dev` builds (anything
without a `git tag`-derived version stamp). It also skips if the
running version exactly matches the latest release tag.

---

## 11. Settings file

Persisted JSON at:

* **Windows**: `%AppData%\GcodeSim\settings.json`
* **macOS**: `~/Library/Application Support/GcodeSim/settings.json`
* **Linux**: `~/.config/GcodeSim/settings.json`

### Schema

```json
{
  "skipped_versions": ["v3.0.3", "v3.0.5"]
}
```

### Fields

* **skipped_versions** — list of release tags the user dismissed via
  the "Hide this update?" follow-up. The next launch silently
  suppresses the prompt for any version in this list. Newer versions
  not in the list still prompt normally.

If the file is missing, empty, or corrupted, the app treats it as a
zero-valued `Settings{}` and continues — bad settings never block
launch. Manual edits are safe; the file rewrites itself on the next
"hide" action.

---

## 12. Troubleshooting

### Windows: `gcodesim.exe` won't launch ("This app can't run on your PC")

Most often a missing dependency or PE issue. Try:
1. Right-click `gcodesim.exe` → Properties → bottom of General tab.
   If there's an "Unblock" checkbox, check it and Apply.
2. From PowerShell: `.\gcodesim.exe` (with the `.\` prefix). If you
   get a different error, paste it.
3. Verify with `objdump -p gcodesim.exe | findstr "DLL"` — should
   only list standard Windows DLLs (KERNEL32, USER32, OPENGL32,
   GDI32, msvcrt, SHELL32). Any third-party DLL means the build is
   wrong; rebuild from source via `build.bat`.

### macOS: "App can't be opened — unidentified developer"

Right-click the `.app` → **Open** → confirm in the dialog. One-time
per machine. After that, double-click works normally.

### macOS: `.nc` opens in TextEdit instead of GcodeSim

Right-click any `.nc` → **Get Info** → in the **Open with:** section
pick `GcodeSimV3` → click **Change All...** → Continue.

This sets GcodeSim as the default for ALL `.nc` files going forward.

### Toolbar appears in the middle of the screen (Mac)

You're running an older binary (pre-Retina-fix). Rebuild via
`./build.sh` or download the latest release. The current build uses
`GetFramebufferSize()` for the GL viewport, which fixes the 2x scale
issue on Retina displays.

### Title bar shows "→ V3.0.X available" but I just rebuilt

Most likely your binary was built against a commit that's older than
the latest tag, OR you're running an older copy (downloaded `.exe`
sitting in your Downloads folder vs. the freshly-built one). Check:
1. Run `LastWriteTime` on the `.exe`/`.app` you're launching — should
   be just after your last build.
2. Make sure you ran `git pull` before `build.bat` / `build.sh`.

### Phantom cuts at high speed

Fixed in V3.0.2+ — the swept-path cut walker correctly skips G0
rapids and follows arc-linearized polylines even when one tick crosses
many moves. If you still see this, you're on an older build.

### Out of memory / slow at high bit diameters on big stock

Heightmap cell count = `(stock_X / cell) × (stock_Y / cell)`, where
`cell = max(0.4, bit_dia / 8)`. A 1 mm bit on a 200 mm stock gives
500×500 = 250K cells = 1.5M flat-shaded vertices. That's the upper
edge of comfort; can drop to 30 fps on integrated GPUs. Workarounds:
larger bit (coarser mesh), smaller stock bounds (the heightmap covers
toolpath bounds + 5 mm margin).

### App won't connect to GitHub for the update check

Likely network restriction or proxy. The check times out silently
after 5 seconds; the app launches normally. The title bar just won't
show the update hint. Worst case: you don't get notified about new
versions — manually check the [Releases page][rel] periodically.

---

## 13. Known limitations

* **Bit profile is flat-end only.** Ball-end and V-bit profiles
  aren't modeled — a 6 mm bit is simulated as a flat 6 mm cylinder.
  3D-pocketing CAM that depends on hemispherical scallops will look
  inaccurate.
* **No collision detection.** The simulator shows what the toolpath
  looks like but doesn't check if it would crash the spindle into the
  fixture or rapid-traverse through stock.
* **No tool-radius compensation (G40/G41/G42).** Cuts are at the bit
  centerline as written.
* **No undercuts.** The heightmap is 2.5D — every XY cell has exactly
  one Z. Geometry that requires undercuts (e.g., 5-axis machining)
  isn't modeled.
* **No tool change (M6).** Programs with M6 just keep the same bit
  diameter throughout.
* **No subprograms / macros.** M98/M99 + variables are ignored.
* **No file-association on Linux.** Windows + macOS do file types;
  Linux users use the file picker or command-line argument.
* **Mac double-click only fires for launch.** If the app is already
  running, double-clicking another `.nc` file activates the existing
  window without loading the new file.

If any of these matter for your workflow, file an issue on GitHub.

---

## See also

* [`tutorials/`](../tutorials/) — runnable example `.nc` files with
  in-line comments
* [`README.md`](../README.md) — install / build quick-start
* [`CLAUDE.md`](../CLAUDE.md) — architecture notes for developers
