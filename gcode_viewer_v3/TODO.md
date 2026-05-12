# TODO — gcode_viewer_v3 Through-Cut Transparency

## Goal
When the 3D stock model is cut, the current renderer appears to push the top surface down to the commanded cut depth. Add a visual through-cut signal: any surface/cell/area whose absolute depth exceeds the configured material thickness should become transparent, showing that the cut has passed fully through the material.

Rule:

```text
if abs(depth) > materialThickness:
    render that specific area transparent
```

The `materialThickness` value must come from the existing **Options ▾ → Material thickness** setting.

## Existing context
- The Options dropdown already exposes **Material thickness**.
- The setting is wired through:
  - `toolbar.go` Apply button
  - `commitMaterialThickness()`
  - `OnMaterialThicknessApplied(mm)`
  - `state.setMaterialThickness(mm)`
  - `heightmap.SetMaterialThickness(mm)`
  - `RefreshMesh()`
- Current through-cut handling may drop quads / create holes, but the desired behavior is more explicit: transparent areas where depth exceeds material thickness.

## Implementation tasks

### 1. Locate actual depth source
Find where each heightmap cell / mesh vertex / quad stores the cut depth.

Likely files/areas to inspect:
- `removal.go`
- heightmap data structure
- mesh rebuild path called by `RefreshMesh()`
- any code that creates the flat-shaded stock mesh through vertex duplication

Determine whether depth is stored as:
- negative Z values
- positive cut depths
- cell min/max Z
- vertex positions only
- a separate `through[]` marker array

### 2. Define through-cut condition cleanly
Use the material thickness option as the cutoff.

Suggested logic:

```go
throughCut := materialThickness > 0 && math.Abs(depth) > materialThickness
```

Important edge cases:
- If `materialThickness == 0`, disable this feature.
- If depth is stored as negative Z, `abs(depth)` is probably correct.
- If depth is already positive cut depth, do not double-convert it.
- Decide whether equality should count as through-cut:
  - user requested `|depth| > material thickness`
  - existing code may use `>=`
  - follow the requested behavior unless there is a strong reason not to.

### 3. Do not make the entire stock transparent
Transparency must be local to the specific cut-through area only.

Bad implementation:

```go
if anyThroughCut {
    wholeMeshMaterial.Transparent = true
}
```

Correct direction:
- split through-cut geometry from normal stock geometry, or
- assign per-vertex/per-face alpha if the renderer supports it, or
- generate a second mesh for transparent through-cut faces.

### 4. Choose rendering approach
Preferred approach:

#### Option A — split mesh into opaque and transparent geometry
During mesh generation:
- normal cells/quads go into the normal opaque mesh
- through-cut cells/quads go into a transparent mesh

This is probably the cleanest and least fragile approach because many renderers/material systems handle transparency per material, not per face.

Expected result:
- Opaque stock remains normal.
- Through-cut areas use transparent material.
- No global transparency bug.

#### Option B — per-face/per-vertex alpha
Only use this if the current renderer supports vertex colors with alpha and the material respects alpha.

Risks:
- alpha may be ignored by `Standard` material unless configured correctly
- sorting artifacts
- harder to debug

### 5. Material requirements
Transparent material likely needs:
- alpha less than 1.0
- blend/transparency enabled
- depth-write reviewed
- same lighting model as current stock material if possible

Avoid switching back to a weaker material path if the project intentionally uses Standard material instead of Basic material.

Suggested visual:
- transparent enough to clearly signal through-cut
- still faintly visible so the user can see where the cut occurred

### 6. Preserve current behavior where useful
If current through-cut logic drops quads / creates real holes, decide whether to:
- replace holes with transparent patches, or
- keep holes and add transparent side/bottom indicators, or
- make this an option later

For this specific task, prioritize the requested behavior:

```text
specific fully-through areas become transparent
```

Do not silently keep behavior that makes the feature look like it does nothing.

### 7. Connect to Options dropdown value
Verify the transparent logic uses the same `materialThickness` value set from Options.

Test values:
- `0` → transparency disabled
- `1` → shallow cuts deeper than 1 mm become transparent
- `19.05` → cuts deeper than 19.05 mm become transparent
- `0.75in` or `0.75"` → converts to 19.05 mm and works the same

### 8. Add debug verification
Temporarily log or expose counts during mesh rebuild:

```text
materialThickness = X
normal quads = Y
through transparent quads = Z
```

This prevents the fake-success failure mode where the UI is wired but no geometry changes.

Remove or gate debug logs before final cleanup.

### 9. Test with controlled sample G-code
Create or load a small test file with known depths:

Example intent:
- stock/material thickness: `5 mm`
- cut A: `Z-2` → opaque/depressed only
- cut B: `Z-5` → not transparent if using strict `>`
- cut C: `Z-6` → transparent

This directly tests the requested rule.

### 10. Watch for likely failure points
Likely ways this will fail:
- comparing signed negative Z directly instead of `abs(depth)`
- applying transparency to the whole material
- mesh generator drops through-cut quads before they can be rendered transparent
- material alpha is set but blending is not enabled
- transparent geometry is hidden by depth-write / draw order
- inches parsing works in toolbar but not in downstream heightmap logic
- `RefreshMesh()` is not called after changing thickness

## Acceptance criteria
- Changing **Options ▾ → Material thickness** visibly changes the through-cut transparency behavior.
- Only areas deeper than the material thickness become transparent.
- Areas shallower than or equal to material thickness remain normal.
- `0 = off` disables the transparent-through-cut behavior.
- The whole stock mesh does not become transparent.
- The behavior survives camera rotation and normal model interaction.

## Suggested commit message

```text
Add local transparent rendering for through-depth cuts
```
