# Polygon-Union Material Removal Renderer (Design Doc)

**Status:** Designed, not implemented. Deferred from the v3.0.4 release.
**Audience:** anyone (Claude Code session, human dev, future-self) picking
this up to actually build it.
**Last updated:** 2026-05-03 by Claude Code (Opus 4.7).

---

## TL;DR

The current material-removal display uses a heightmap — a regular XY grid
where each cell stores the current top-of-material Z. As the bit moves,
cells inside the bit's circular footprint are lowered to tool Z. This is
fast, simple, and handles arbitrary 3D geometry (ramps, surfacing) — but
the cell discretization makes circular bit features (corner fillets,
external roundings) look jagged on close inspection.

This doc proposes a second renderer that runs alongside the heightmap and
handles the common case (constant-Z 2.5D pocketing) with **mathematically
exact** geometry — every cutting move sweeps a *stadium* shape (rectangle
+ two semicircles), all stadiums per Z layer are unioned into a real 2D
polygon, the polygon is triangulated for the floor, and its boundary
edges are extruded into vertical wall quads. Bit fillets at internal
corners and roundings at external corners are real geometry, not
discretized blobs.

The heightmap stays in the codebase as a fallback for Z-varying moves
(plunge ramps, 3D surfacing) where the stadium model doesn't apply.

---

## 1. The problem

User complaint, May 2026: "The 3D viewer leaves jagged edges around
internal corners and does not round over external corners when they
should be."

Root cause is in [`internal/scene/removal.go`](../gcode_viewer_v3/internal/scene/removal.go):

```go
func HeightmapCellSize(bitDiameter float64) float64 {
    c := bitDiameter / 8.0
    if c < 0.4 { return 0.4 }
    return c
}
```

For a typical 6 mm bit, cell size = **0.75 mm**. That gives only ~4 cells
per bit radius. The bit's circular footprint is rasterized into a chunky
~50-cell octagon.

```
What it should look like at an external corner of an island:

    ··················           ←  uncut stock
    ····██████████████              cut region
    ····█┐         ███
    ····█│ smooth  ███           ← bit_radius fillet
    ····█│ ⌒  ⌒  ⌒  ███             (real curve)
    ····█│         ███
    ····█│         ███

What it actually looks like at the same corner today:

    ··················
    ····██████████████
    ····█┐  ┐ ┐      ███         ← stair-stepped
    ····█│  │ │      ███             discretization
    ····█│ ┘ ┘       ███
    ····█│┘┘         ███
    ····█│           ███
```

**Why internal corners are jagged:** an internal corner of a pocket
should show a smooth fillet of radius = bit_radius. With 4 cells per
radius, that fillet is 4 stair-steps. Looks like a Minecraft corner.

**Why external corners look square:** the algorithm cuts cells whose
*centers* fall within `bit_radius` of the tool. Cells whose centers sit
just outside the radius (but whose bodies cross into it) are NOT cut,
so the actual cut region is slightly smaller than the bit's true
footprint. That leaves a half-cell-thick rim of material at the tip of
external corners — which reads as "the corner isn't rounded."

**Flat shading magnifies it.** The heightmap's flat-shaded triangulation
gives every quad its own face normal. Stair-step Z transitions become
visually distinct triangles instead of being smoothed by interpolation,
so jaggies pop visually instead of fading into a continuous shading
gradient.

---

## 2. Why not just shrink cell size

Tightening the formula (e.g., `bit_diameter / 16`, floored at 0.25 mm)
would help — but at significant memory cost. Cells per dimension scale
with `1 / cell_size`, total cell count scales with `1 / cell_size²`,
mesh vertex count is 6× cells (flat-shaded), VBO memory scales the same
way.

| Bit | Stock | Current cells (bit/8) | Proposed cells (bit/16) | VBO @ proposed |
|---|---|---|---|---|
| 6 mm | 100 × 100 | 134² ≈ 18 K | 268² ≈ 72 K | ~10 MB |
| 3 mm | 100 × 100 | 250² ≈ 62 K (floored) | 500² ≈ 250 K | ~36 MB |
| 1 mm | 100 × 100 | 250² ≈ 62 K (floored) | 500² ≈ 250 K (floored) | ~36 MB |
| 6 mm | 300 × 300 | 401² ≈ 160 K | 800² ≈ 640 K | ~93 MB |

Even with the cap, sub-mm bits on big stocks become VBO-heavy. And the
underlying problem doesn't actually go away — at any finite cell size,
the bit's circle is still a polygon, just a higher-resolution one.

The polygon-union approach is the **correct geometric fix** — bit_radius
is encoded directly in the rendered geometry as a real arc, not as a
rasterization of a circle.

---

## 3. Goals and non-goals

### Goals

- **G1:** Internal corners of pockets show clean circular fillets of
  radius = bit_radius, not stair-steps.
- **G2:** External corners of islands show clean circular roundings of
  radius = bit_radius, not square cut-offs.
- **G3:** The bit's swept footprint along straight cuts shows a clean
  stadium boundary, not a stair-stepped strip.
- **G4:** Multi-Z passes display as proper stratified terraces (each
  layer's floor + walls visible).
- **G5:** Through-cuts (cut deeper than `MaterialThickness`) render as
  real holes in the mesh, same as today.
- **G6:** Performance ≥ current heightmap on typical programs
  (≤ 1000 cutting moves). Acceptable degradation up to 10 K moves.
- **G7:** No visual aesthetic regression — keeps the "machined" look
  (flat shading on floors, hard edges between facets).

### Non-goals

- **NG1:** Z-varying moves (plunge ramps, 3D surfacing). These remain
  handled by the heightmap. Polygon renderer is for constant-Z passes.
- **NG2:** True 5-axis simulation. Same scope cap as the existing viewer.
- **NG3:** Tool radius compensation (bit-side allowance). Out of scope.
- **NG4:** Realtime cut animation at the bit-spinning level. Bit still
  doesn't visually spin (LED-color spindle indicator stays).
- **NG5:** Eliminating the heightmap. It stays as the fallback for
  Z-varying moves and as the un-cut surface model.

---

## 4. The geometric model

### 4.1 The stadium — one cutting move's footprint

A round bit moving from `(x1, y1)` to `(x2, y2)` at constant Z sweeps a
**stadium**: a rectangle of length `L` and width `2r` (where `L` is the
distance between centers and `r` is bit_radius), capped with a
semicircle of radius `r` at each end.

```
      ⌒ ⌒ ⌒                           ⌒ ⌒ ⌒
   ⌒          ⌒                    ⌒         ⌒
  ⌒            ⌒━━━━━━━━━━━━━━━━━━⌒           ⌒
 ⌒              •─── center ────────•           ⌒    ← bit center path
  ⌒            ⌒━━━━━━━━━━━━━━━━━━⌒           ⌒
   ⌒          ⌒                    ⌒         ⌒
      ⌒ ⌒ ⌒                           ⌒ ⌒ ⌒
      ←——r——→         L              ←——r——→
```

Polygon representation: 2 long edges + 2 arcs. Arcs are approximated by
N segments. Default N = 16 per full circle (8 per semicircle); adaptive
later if needed.

### 4.2 The union — combining all cuts at one Z

Multiple cutting moves at the same Z produce overlapping stadiums. The
**union** of all stadiums is the total cut region at that Z.

```
Two stadiums meeting at an external corner (90° turn):

  ┌──────────────┐
  │              │
  │   stadium 1  │
  │              │╲
  └──────────────┘ ╲
                    ╲
              ┌─────╲─────┐
              │      ╲    │
              │ stadium 2 │
              │           │
              └───────────┘

Union = the outer outline. The corner is auto-rounded to bit_radius
because the two semicircle ends overlap at the turn point.
```

Inside corners: when two stadiums approach a 90° internal corner from
the inside (e.g., a pocket boundary), their union leaves a circular
fillet of radius = bit_radius in the corner. Real geometry, no
discretization.

### 4.3 Multi-Z stratification

A program like `tutorials/04_layered_pyramid.nc` cuts at three depths
(-1, -2, -3 mm). Each depth has its own polygon union. Stacked, they
look like terraces — deeper polygons are typically inside shallower
ones (the rough → finishing pass pattern).

For each depth `Z_L` with polygon `P_L`:
- **Visible floor** at depth `Z_L` = `P_L` minus the union of polygons
  at deeper depths. (Don't draw a -2 mm floor where there's actually a
  -3 mm floor below.)
- **Visible walls** at depth `Z_L` = boundary edges of `P_L` not shared
  with shallower polygons. Walls go up from `Z_L` to the next-shallower
  `Z` (or to the stock top if `Z_L` is the shallowest layer).

### 4.4 Through-cuts

When a cut depth `Z_L ≤ -MaterialThickness`, mark the polygon (or part
of it) as "through". When generating the mesh:
- Don't emit a floor for through regions.
- Walls extend down to `-MaterialThickness` instead of `Z_L`.
- The result: a real hole in the mesh. Cutout pieces visually separate
  from the surrounding stock.

### 4.5 Why this gives real curves

The bit's circle isn't approximated by rasterization — it's approximated
by N segments per arc on the stadium itself, then carried through the
polygon union into the rendered mesh as actual N-segment arcs of vertical
walls. With N=16, the visual smoothness is comparable to a 32-faceted
cylinder, which reads as round at any reasonable viewing distance.

---

## 5. Code structure

### 5.1 New package: `internal/geom/`

Pure-Go geometry primitives, no g3n deps. Easy to unit-test.

```
internal/geom/
├── stadium.go        Stadium-polygon generation (rect + 2 semicircle arcs)
├── polygon.go        Polygon type, basic ops, simplification
├── boolean.go        Polygon boolean ops (union, difference) — Vatti or wrapper
├── triangulate.go    Ear-clipping triangulator for simple-with-holes polygons
└── *_test.go         Unit tests with golden polygons
```

`polygon.go` representation:

```go
type Point struct{ X, Y float64 }

type Polygon struct {
    // First contour is the outer boundary (CCW). Subsequent contours
    // are holes (CW). All in mm world coordinates.
    Contours [][]Point
}
```

### 5.2 New scene actor: `internal/scene/swept.go`

Mirrors the heightmap's API for drop-in coexistence.

```go
type SweptRenderer struct {
    BitRadius         float64
    StockBounds       [4]float64  // X0, X1, Y0, Y1
    StockTop          float64
    MaterialThickness float64

    // Per-Z-depth polygon union. Map key is the cut Z value
    // (rounded to a tolerance, e.g., 0.01 mm) so near-equal Zs bucket
    // together.
    layers map[int64]*polygonLayer

    actor    *graphic.Mesh
    posVBO   *gls.VBO
    normVBO  *gls.VBO
}

type polygonLayer struct {
    Z        float64
    Polygon  geom.Polygon  // running union of all cuts at this Z
    Through  bool          // Z <= -MaterialThickness
    Dirty    bool          // needs re-triangulation
}

// API mirrors Heightmap so the caller in window.go can swap.
func NewSweptRenderer(bounds [4]float64, topZ, bitRadius, materialThickness float64) *SweptRenderer
func (r *SweptRenderer) Cut(p1, p2 parser.Point, bitRadius float64)
func (r *SweptRenderer) Reset()
func (r *SweptRenderer) SetMaterialThickness(mm float64)
func (r *SweptRenderer) Actor(color math32.Color) *graphic.Mesh
func (r *SweptRenderer) RefreshMesh()
```

### 5.3 Changes to existing files

`internal/scene/removal.go` (heightmap):
- Keep as-is. Still handles Z-varying moves and the un-cut surface base.
- Optionally: add a `SkipConstantZ bool` flag to suppress cuts that the
  swept renderer is already handling, so the heightmap's Z values don't
  go below 0 in regions the polygon mesh covers. Avoids Z-fighting.

`internal/ui/window.go`:
- Add a `swept *scene.SweptRenderer` field on `sceneState`.
- In `installMoves()`, build both the heightmap AND the swept renderer.
- In `applyCutsBetween()` / `cutMoveSegment()`, classify the move:
  - **Constant-Z + cutting (Z < 0)** → feed to swept renderer
  - **Z-varying** (`|Z2 - Z1| > epsilon`) → feed to heightmap
  - **G0 or spindle off** → skip both (existing behavior)
- In `RefreshMesh()` cadence, refresh both renderers.

`internal/ui/toolbar.go`:
- Optional: add a "Smooth corners" toggle in the Options panel that
  enables/disables the swept renderer. Useful for debugging or for users
  who want the old behavior. Default = on.

### 5.4 No external Go deps if possible

The polygon boolean op is the load-bearing dependency. Two paths:

**Option A — vendor a port of Clipper:**
[`github.com/ctessum/polyclip-go`](https://github.com/ctessum/polyclip-go)
is a Go port of Vatti's algorithm. Stable, ~3 K LOC, no CGo. Pulled in
via `go get`. Risk: dependency on third-party code that may go
unmaintained.

**Option B — implement Greiner-Hormann ourselves:**
~500 LOC of pure Go. We own it. Numerical-precision edge cases need
care (use rational arithmetic or scaled integers for robustness). Risk:
implementation bugs, longer to ship.

**Recommendation:** Start with Option A for speed of prototyping. If
boundary precision turns out to be a problem on real programs, swap in
Option B.

---

## 6. Hybrid rendering integration

The swept renderer and the heightmap coexist in the scene graph. Both
render at every frame; the GPU's depth test resolves overlap.

### 6.1 Region ownership

Every move is classified once on parse:

```go
func isConstantZCut(m *parser.Move) bool {
    if m.Kind == "G0" || !m.Spindle {
        return false
    }
    // All points in the move must share the same Z (within tolerance)
    // and be below the stock top.
    z := m.Points[0].Z
    if z >= 0 {
        return false  // not cutting into the stock
    }
    for _, p := range m.Points[1:] {
        if math.Abs(p.Z-z) > 1e-6 {
            return false  // Z-varying — heightmap territory
        }
    }
    return true
}
```

Cuts from `isConstantZCut == true` go to the swept renderer. Other cuts
go to the heightmap.

### 6.2 The Z=0 seam

Where a constant-Z cut region meets uncut stock, the swept renderer's
walls extend up to `Z=0`. The heightmap surface is at `Z=0` in those
regions (it never received a cut). They meet exactly. No seam.

Where a Z-varying move (e.g., a ramp into material) ends at `Z=Z_low`
and a constant-Z cut continues at `Z=Z_low`, the heightmap's surface
ends at `Z_low` and the swept renderer's floor begins at `Z_low`. They
should meet cleanly if both classifications agree on the depth.

Edge case: arc moves (G2/G3) where the arc has constant Z but the
linearized polyline may show tiny floating-point Z drift. Handle by
rounding the cut Z to the nearest 0.01 mm bucket before classification.

### 6.3 Z-fighting risk

Where the swept renderer's floor and the heightmap's surface overlap (in
regions that should ONLY be swept-rendered but the heightmap has a
default flat surface there): GPU depth test will pick one. Visible
flicker possible.

Mitigation: when a swept-renderer cut happens at `(x, y, z<0)`, also
flag the corresponding heightmap cell(s) so the heightmap mesh skips
those quads (degenerate triangles). Same trick the through-cut
implementation already uses.

---

## 7. Algorithm details

### 7.1 Stadium generation

```go
func StadiumPolygon(p1, p2 geom.Point, radius float64, arcSegments int) geom.Polygon {
    dx, dy := p2.X-p1.X, p2.Y-p1.Y
    length := math.Sqrt(dx*dx + dy*dy)

    if length < 1e-9 {
        // Degenerate move — single circle
        return CirclePolygon(p1, radius, arcSegments*2)
    }

    // Unit vector along the move
    ux, uy := dx/length, dy/length
    // Perpendicular (left of motion)
    vx, vy := -uy, ux

    // Two long sides
    a := geom.Point{p1.X + vx*radius, p1.Y + vy*radius}  // start, left
    b := geom.Point{p2.X + vx*radius, p2.Y + vy*radius}  // end,   left
    c := geom.Point{p2.X - vx*radius, p2.Y - vy*radius}  // end,   right
    d := geom.Point{p1.X - vx*radius, p1.Y - vy*radius}  // start, right

    pts := []geom.Point{a}
    // Arc end at p2: from b around to c, going CCW (through "outside" of motion)
    appendArc(&pts, p2, radius, math.Atan2(vy, vx), math.Atan2(-vy, -vx), arcSegments)
    pts = append(pts, c, d)
    // Arc start at p1: from d around to a
    appendArc(&pts, p1, radius, math.Atan2(-vy, -vx), math.Atan2(vy, vx), arcSegments)

    return geom.Polygon{Contours: [][]geom.Point{pts}}
}
```

`appendArc` is N-segment circle approximation between two angles. CCW.

### 7.2 Incremental union

Naive: every `Cut()` call computes `union(layer.Polygon, newStadium)`.
This is O(n) per cut where n = current vertex count. For 10 K cuts with
union growing to ~10 K vertices, that's O(n²) = 100 M operations.
Sluggish.

Optimization: batch unions.
- Each `Cut()` appends the new stadium to a pending list.
- Once per frame (or every K cuts), union the pending list as a single
  multi-polygon batch and merge into the layer. Clipper-class libraries
  handle multi-polygon input natively in O((m+n) log(m+n)).

For incremental display, render the layer mesh from the last-merged
polygon; pending stadiums can render as separate meshes until merged.

### 7.3 Difference for stratification

When generating a layer's visible floor:

```go
visibleFloor := layer.Polygon.Difference(unionOfDeeperLayers)
```

`unionOfDeeperLayers` can be cached and updated incrementally as new
deeper layers appear.

### 7.4 Triangulation

Ear-clipping is the simplest robust algorithm for arbitrary
simple-with-holes polygons.

```go
// Returns triangle indices (3 per triangle) into the input vertex list.
func TriangulateEarClip(p geom.Polygon) []int
```

For polygons with holes, first cut bridges from the outer boundary to
each hole (turning a polygon-with-holes into a simple polygon with
duplicated edge), then ear-clip.

A 200-vertex polygon triangulates in <1 ms.

### 7.5 Wall extrusion

For each edge `(v_i, v_{i+1})` in `visibleFloor.Contours[outer]`:
```go
wall := []geom.Point{
    {v_i.X,    v_i.Y,    z_low},
    {v_{i+1}.X, v_{i+1}.Y, z_low},
    {v_{i+1}.X, v_{i+1}.Y, z_high},
    {v_i.X,    v_i.Y,    z_high},
}
emitQuad(wall)
```

Outward normal for the wall = perpendicular to the edge, in the XY
plane, pointing AWAY from the polygon interior.

`z_high` = next-shallower layer's Z (or 0 if topmost).
`z_low`  = this layer's Z (or `-MaterialThickness` if through-cut).

For inner contours (holes — i.e., islands inside the cut), wall normals
point toward the hole interior. Same logic, opposite sign.

---

## 8. Implementation phasing

Each milestone is end-to-end testable on its own.

### M1 — Stadium + single-layer union (proof of concept)

- New `internal/geom/` package with `Point`, `Polygon`, `Stadium`.
- Vendor or write polygon union.
- Standalone test program: hardcode 10 stadiums in a square pattern,
  compute their union, render the union outline as 2D wireframe in the
  3D viewer (just lines on the XY plane).
- **Acceptance:** the union outline visually matches what you'd draw by
  hand on paper.

### M2 — Floor triangulation

- Add ear-clipping triangulator.
- Convert the union polygon into a triangle mesh at constant Z.
- Render as an opaque mesh.
- **Acceptance:** running `tutorials/01_basic_square.nc` shows a clean
  square pocket floor with rounded corners.

### M3 — Walls

- For each polygon edge, emit a vertical quad from `z_cut` to `0`.
- Compute outward normals.
- **Acceptance:** the basic square pocket renders as a 3D pocket with
  smooth walls. View from the side via the cube.

### M4 — Multi-Z layers

- Bin moves by Z (with rounding tolerance).
- Per-layer polygon union.
- Stratification: deeper layers' polygons are drawn at their depth;
  shallower layers' floors are clipped to NOT cover deeper regions.
- Walls between layers transition at the right Z.
- **Acceptance:** `tutorials/04_layered_pyramid.nc` shows three crisp
  terraces, each with smooth corners.

### M5 — Through-cut

- Detect cuts at `Z ≤ -MaterialThickness`.
- Mark layer as through.
- Drop floor for through regions; extend walls to `-MaterialThickness`.
- **Acceptance:** `tutorials/05_through_cutout.nc` with material
  thickness = 6 mm shows the inner square as a real cutout (visible
  through the hole when looking from above).

### M6 — Hybrid integration

- Wire the swept renderer into `window.go` alongside the heightmap.
- Classify moves: constant-Z → swept, varying-Z → heightmap.
- Suppress heightmap cuts in swept-handled regions.
- **Acceptance:** `tutorials/06_complex_motion.nc` shows smooth
  pocketing AND correctly handles the drilled holes (Z-varying plunges
  use heightmap fallback).

### M7 — Performance + polish

- Spatial bucketing if 10 K-move programs are slow.
- Incremental union (batch pending cuts per frame).
- Mesh refresh throttle (mirror the heightmap's `SURFACE_REFRESH_EVERY = 4`
  cadence).
- Profile and bench.
- **Acceptance:** ≥ 45 fps on `tutorials/06_complex_motion.nc` at 50× speed.

### M8 (optional) — Polish

- Adaptive arc segment count (more segments for big bits).
- Optional Options-panel toggle ("Smooth corners on/off").
- Visual transition smoothing at swept ↔ heightmap boundaries.

---

## 9. Testing & validation

### 9.1 Unit tests (in `internal/geom/`)

- Stadium vertex positions for trivial moves (axis-aligned, 45°, zero-length).
- Polygon union: two squares overlapping; stadium + circle; two stadiums
  meeting at a corner.
- Difference: nested squares.
- Ear-clipping: convex polygon, polygon with hole, concave polygon,
  polygon with collinear vertices.

### 9.2 Visual regression (the 6 tutorials)

Each tutorial is a known input. After a milestone, render it and
compare against a reference screenshot. Save reference screenshots in
`docs/POLYGON_RENDERER_screenshots/` for diffing later.

| Tutorial | What it validates |
|---|---|
| `01_basic_square.nc` | Single-pass outline; 4 external corners rounded |
| `02_pocket_clear.nc` | Parallel-pass pocketing; no visible streaks between passes |
| `03_arc_circle.nc` | G2/G3 arcs render as smooth curves |
| `04_layered_pyramid.nc` | Three-tier stratification, clean step transitions |
| `05_through_cutout.nc` | Through-cut produces a real hole |
| `06_complex_motion.nc` | Hybrid integration: pocket smooth, drills via heightmap |

### 9.3 Performance benchmarks

In `bench/` (TBD): generate synthetic programs with N = 100, 1 K, 10 K
cutting moves. Measure:
- Cut-application time (ns per move)
- Mesh refresh time (ms per frame)
- VBO upload time (ms)
- Steady-state FPS

Target: ≥ current heightmap on N ≤ 1 K. Acceptable degradation up to N = 10 K.

---

## 10. Performance considerations

### 10.1 Where time goes

| Operation | Heightmap | Swept renderer |
|---|---|---|
| Per-cut cost | O(cells in bbox of bit) ≈ const | O(n) for incremental union, where n is current vertex count |
| Mesh refresh | O(Nx × Ny) full grid | O(boundary vertices + triangles) |
| VBO size | scales with stock area | scales with cut complexity |

Heightmap is roughly constant per cut and per frame. Swept renderer is
fast for typical programs but degrades on huge programs.

### 10.2 Memory comparison

Heightmap, 100×100 mm stock, 6 mm bit, 0.75 mm cells:
- 134 × 134 = 18 K cells → 108 K verts flat-shaded → ~2.5 MB VBO

Swept renderer, 1000-cut program, ~500 boundary vertices per layer,
3 layers:
- ~1.5 K verts → ~36 KB VBO

Swept WINS on big simple stocks. LOSES on tiny stocks with crazy cut
complexity (10 K+ moves with intricate paths).

### 10.3 Throttling

Mirror the heightmap's `SURFACE_REFRESH_EVERY = 4` (mesh refresh ~15 Hz
at 60 Hz tick). Cuts apply every tick; mesh rebuild is throttled.

For incremental union, batch pending stadiums each frame and union them
in one Clipper call (much faster than 60 individual unions per second).

---

## 11. Alternatives considered

| Approach | Why rejected |
|---|---|
| Shrink heightmap cell size (`bit/16`) | Memory cost is significant; sub-mm bits on big stocks blow up. Doesn't address external-corner issue (always discretized). |
| Coverage-fraction cuts on heightmap | Smooths edges but cut walls become "averaged" / sloped, losing crisp machined look. ~3× slower per Cut(). |
| Sample toolpath finer | Helps along-path jaggies but not the corner-radius issue. |
| Boundary-only smoothing pass | Modest visual win, complicates the mesh-build code. |
| Smooth (Gouraud) shading | Free fix BUT loses the deliberate flat/machined aesthetic. |
| Marching cubes on a 3D voxel grid | Smooth iso-surface but voxel storage is even heavier than heightmap. Overkill for 2.5D. |
| Pure SDF (signed distance field) ray marching | True smooth surfaces but completely different rendering pipeline, doesn't fit g3n. |
| CSG (constructive solid geometry) | Mathematically exact but notoriously hard to implement robustly and fast. Mesh quality issues at boolean intersections. |

Polygon-union is the right balance of correctness and tractability for
the 2.5D use case that dominates hobbyist CNC.

---

## 12. Open questions (decisions to make at implementation time)

### 12.1 Polygon library

- **A:** Vendor [`ctessum/polyclip-go`](https://github.com/ctessum/polyclip-go).
  Faster to ship, third-party risk.
- **B:** Implement Greiner-Hormann ourselves. ~500 LOC, more work, full
  control.

Recommendation: **A** for prototype, swap to **B** if precision issues
appear.

### 12.2 Arc segment count

How many segments per full circle for stadium semicircles?
- 8: visible faceting, fine for tiny on-screen bits
- 16: smooth at typical viewing distance
- 32: very smooth, more polygon complexity

Recommendation: **16** default. Make it adaptive later (more segments
for larger bits).

### 12.3 Heightmap retention

Three options:
- **A:** Keep heightmap as-is, swept renderer is purely additive.
  Heightmap renders the un-cut surface and Z-varying cuts; swept
  renderer renders constant-Z cuts. Hybrid.
- **B:** Remove the heightmap entirely. Use only the swept renderer +
  a flat plane for the un-cut surface. Loses Z-varying support.
- **C:** Keep heightmap as the base layer (always renders the un-cut
  flat surface), swept renderer overlays cut regions.

Recommendation: **A**. Heightmap stays for fallback; minimal disruption.

### 12.4 Toggle in the Options panel?

Should users be able to switch between "smooth (swept)" and "machined
look (heightmap)"?

- Pro: gives users control, lets them compare.
- Con: more UI, more state, users might pick the worse-looking option
  by accident.

Recommendation: **No toggle, swept-on by default**. If we want a
fallback, expose it via a hidden env var or config flag for debugging.

### 12.5 Z bucketing tolerance

Two cuts at Z = -2.0001 and Z = -2.0000 should land in the same layer.
Use rounding to `0.01 mm` (i.e., bucket key = `int64(z * 100)`)?
Or `0.001 mm`? Tighter risks splitting layers that the user means as one.

Recommendation: **0.01 mm** (bucket key = `int64(math.Round(z * 100))`).

### 12.6 Background goroutine for unions?

Could move the polygon union work to a background goroutine to avoid
stutter on the main render thread.

- Pro: smoother frame times under heavy cut load.
- Con: synchronization complexity; mesh updates need careful sequencing.

Recommendation: **No, defer.** Profile first; if main-thread refresh is
under throttled budget, no need.

---

## 13. Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Polygon library numerical precision issues (degenerate edges, self-intersections) | Med | Use scaled integers internally (1 unit = 1 µm), validate output, fall back to heightmap on union failure |
| Performance regression on huge programs | Med | Spatial bucketing + incremental union batching |
| Visible seam between swept and heightmap regions | Low-Med | Z-bias; suppress heightmap cells in swept regions |
| Implementation complexity blowup | High | Strict milestone phasing; each M is end-to-end testable |
| Aesthetic regression (loses "machined" look) | Low | Keep flat shading on swept-renderer floors and walls — same triangle-per-face approach as heightmap |
| Unhandled edge case: degenerate (zero-length) cuts | Low | Stadium-gen function falls back to circle for zero-length |
| Arc faceting visible on close zoom | Low | Adaptive segment count if user reports |

---

## 14. Out of scope (deliberate)

- **Bit shapes other than flat end-mill.** Ball-nose / V-bit / corner-rounding bits would need a 3D footprint, not a 2D circle. Hobbyist routers mostly use flat end-mills; defer.
- **Climb-vs-conventional milling visualization.** Direction of travel doesn't affect the swept polygon.
- **Surface roughness modeling.** The simulator shows the *cut envelope*, not the actual surface texture from the bit's individual flutes.
- **Bit deflection / chatter.** Real-machine artifacts; out of simulator scope.

---

## 15. References

- Vatti, B. R. (1992). "A generic solution to polygon clipping."
  *Communications of the ACM*, 35(7), 56–63.
- Greiner, G., & Hormann, K. (1998). "Efficient clipping of arbitrary
  polygons." *ACM Transactions on Graphics*, 17(2), 71–83.
- Held, M. (2001). "FIST: Fast Industrial-Strength Triangulation of
  Polygons." *Algorithmica*, 30(4), 563–596. (Ear-clipping reference.)
- [CAMotics](https://camotics.org) — open-source CNC simulator using
  similar polygon-union approach for 2.5D rendering.
- [Clipper2 documentation](http://www.angusj.com/clipper2/) — reference
  for the polygon boolean op semantics.

---

## 16. Status checkpoints

Update this section as work progresses. Format: date, milestone, who, notes.

- **2026-05-03** — Design doc written. No code yet. Deferred from v3.0.4
  release to allow user to evaluate simpler fixes (cell-size shrink +
  finer toolpath sampling) first.
