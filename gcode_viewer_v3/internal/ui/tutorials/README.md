# GcodeSim Tutorials

Six small `.nc` files designed to walk a new user through the
simulator's features, from straight-line cuts to multi-operation
programs. **Bundled into the binary** via `//go:embed tutorials/*.nc`
in `../tutorials.go` — end users open them via the **Tutorials ▾**
dropdown in the toolbar, no disk hunt required.

Read the comments at the top of each file before clicking **Play** —
they explain what to look for and suggest bit-diameter / material-
thickness settings.

The files are real, runnable G-code (G21 metric, G90 absolute, M3/M5
spindle on/off, M30 program-end) — small enough that they could even
go on a real CNC if you've zeroed your workpiece appropriately.

> **For developers:** to add a tutorial, drop the `.nc` file in this
> folder AND add an entry to the `Tutorials` slice in `../tutorials.go`
> with the display name and file path. Rebuild — the embed pattern
> `tutorials/*.nc` picks it up automatically, but the dropdown only
> shows entries listed in the slice.

## The tutorials

| # | File | What it teaches |
|---|---|---|
| 1 | `01_basic_square.nc`     | Toolbar basics, depth-graded cuts, spindle indicator |
| 2 | `02_pocket_clear.nc`     | Heightmap material removal on a parallel-pass pocket |
| 3 | `03_arc_circle.nc`       | G2/G3 arc rendering and arc-following cuts |
| 4 | `04_layered_pyramid.nc`  | Multiple Z-depth passes, depth color gradient |
| 5 | `05_through_cutout.nc`   | The "Material thickness" option and through-cut behavior |
| 6 | `06_complex_motion.nc`   | Mixed operations: outline + pocket + drill + slot |

## Suggested workflow per tutorial

1. **Click Tutorials ▾** in the toolbar and pick the file. (Or
   `Ctrl+O` / `Cmd+O` if you're loading the source copy from disk.)
2. **Read the header** of the file in this folder for the "what to
   watch for" notes.
3. **Adjust settings** as suggested in the file header (bit diameter,
   material thickness if applicable).
4. **Hit Reset** to make sure the heightmap starts un-carved.
5. **Click Play** and watch the tool path through the toolbar
   playback.
6. **Try the speed slider** (drag toward the right for fast playback;
   the simulator handles up to 50× without phantom cuts).
7. **Drag the progress bar** to scrub forward and back — cuts replay
   correctly in either direction.
8. **Use the view cube** (top-right) to snap the camera to top, side,
   and isometric views. Click a corner of the cube for an iso shot.

## Going further

After working through these, try loading your own programs (.nc,
.gcode, .ngc, .tap, .cnc, .gco, .g, .mpf, .nci, .tab, .eia, .dnc all
recognized) and configuring the bit diameter to match your real
endmill — the heightmap resolution and through-cut behavior depend on
the diameter you set in the toolbar.

For a complete reference of every control, see the
[user manual](../../../../docs/MANUAL.md).
