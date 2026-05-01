"""Tool / bit representation as a 3D CNC end-mill following the toolpath.

Bit anatomy (bottom up):

    z = SHAFT_HEIGHT  ┌─────┐  ← top of visible shank
                      │     │
                      │  ▓  │  ← LED ring (orange idle / green spinning)
                      │     │
                      │     │  ← polished shank section (smooth steel)
                      │     │
    z = FLUTE_HEIGHT  ├─────┤  ← flute terminus (thin dark band)
                      │ ╱╲╱ │
                      │╲╱╲╱ │  ← cutting flutes (dark gunmetal + 2 helical edges)
                      │╱╲╱╲ │
                      │     │
    z = 0             └─────┘  ← tool tip (flat — end mill)

The whole assembly is wrapped in a vtkAssembly so a single SetUserTransform
on the assembly translates every part together. Position the tip at the
desired world (x, y, z) by translating the assembly.
"""

import math
import vtk


# ── Bit geometry (mm) ────────────────────────────────────────────────────────
SHAFT_HEIGHT = 30.0          # total visible bit height (tip → top of shank)
FLUTE_HEIGHT = 12.0          # length of the flute / cutting section
LED_HEIGHT = 1.6             # spindle-state indicator ring height
LED_GAP = 2.0                # mm below shank top where the LED sits
DEFAULT_BIT_DIA = 3.175

# ── Material colors ─────────────────────────────────────────────────────────
# Tuned for the dark VTK background — pick saturated values that still look
# metallic under default lighting.
COLOR_FLUTE = (0.42, 0.40, 0.39)    # dark gunmetal — tool-steel cutting flute
COLOR_FLUTE_EDGE = (0.12, 0.12, 0.13)  # near-black — visible flute helix
COLOR_BAND = (0.18, 0.18, 0.20)     # transition band between flute and shank
COLOR_SHANK = (0.84, 0.85, 0.88)    # bright polished steel — smooth shank
COLOR_LED_OFF = (1.00, 0.55, 0.10)  # warm orange — spindle idle
COLOR_LED_ON = (0.20, 0.85, 0.35)   # bright green — spindle spinning


def _make_cylinder_actor(radius, z_bot, z_top, color, resolution=32,
                         specular=0.35, spec_power=40, ambient=0.2):
    """Build a single cylindrical-section actor along +Z, tip at z=z_bot."""
    cyl = vtk.vtkCylinderSource()
    cyl.SetRadius(radius)
    cyl.SetHeight(z_top - z_bot)
    cyl.SetResolution(resolution)
    # vtkCylinderSource axis is Y; we'll rotate to Z-up below.
    cyl.SetCenter(0.0, (z_top + z_bot) / 2.0, 0.0)

    rotate = vtk.vtkTransform()
    rotate.RotateX(90)
    tf = vtk.vtkTransformPolyDataFilter()
    tf.SetInputConnection(cyl.GetOutputPort())
    tf.SetTransform(rotate)
    tf.Update()

    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputConnection(tf.GetOutputPort())

    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    p = actor.GetProperty()
    p.SetColor(*color)
    p.SetAmbient(ambient)
    p.SetDiffuse(0.7)
    p.SetSpecular(specular)
    p.SetSpecularPower(spec_power)

    # Stash the source so we can resize without rebuilding the pipeline.
    actor._cyl_source = cyl
    actor._z_bot = z_bot
    actor._z_top = z_top
    return actor


def _make_flute_helix_actor(radius, z_bot, z_top, n_flutes=2, helix_angle_deg=30,
                            n_segments=64):
    """Visible helical edge lines down the flute section.

    Real end mills have spiral cutting edges; rendering 2 line-strips at
    ±180° offset reads convincingly as flutes without a heavy mesh rebuild.
    """
    height = z_top - z_bot
    # Axial advance per revolution from the flute helix angle. 30° from the
    # axis is typical for general-purpose end mills.
    pitch = height * math.tan(math.radians(90 - helix_angle_deg))
    n_revs = max(0.5, height / max(pitch, 1e-3))

    points = vtk.vtkPoints()
    lines = vtk.vtkCellArray()
    for f in range(n_flutes):
        phase_offset = 2 * math.pi * f / n_flutes
        line = vtk.vtkPolyLine()
        line.GetPointIds().SetNumberOfIds(n_segments + 1)
        for i in range(n_segments + 1):
            t = i / n_segments
            a = phase_offset + 2 * math.pi * n_revs * t
            # Slightly outside the cylinder radius so the line isn't z-fighting
            # with the cylinder surface — tiny offset, invisible to the eye.
            r = radius * 1.005
            x = r * math.cos(a)
            y = r * math.sin(a)
            z = z_bot + t * height
            pt_id = points.InsertNextPoint(x, y, z)
            line.GetPointIds().SetId(i, pt_id)
        lines.InsertNextCell(line)

    pd = vtk.vtkPolyData()
    pd.SetPoints(points)
    pd.SetLines(lines)

    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputData(pd)

    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    p = actor.GetProperty()
    p.SetColor(*COLOR_FLUTE_EDGE)
    p.SetLineWidth(1.8)
    p.SetLighting(False)  # lines look flat anyway; skip lighting overhead

    # Stash params so update_tool_diameter() can regenerate at a new radius
    actor._helix_params = {
        "z_bot": z_bot, "z_top": z_top,
        "n_flutes": n_flutes, "helix_angle_deg": helix_angle_deg,
        "n_segments": n_segments,
    }
    return actor


def _rebuild_helix_geometry(helix_actor, radius):
    """Regenerate the helix polydata for a new bit radius (used by diameter
    changes)."""
    p = helix_actor._helix_params
    height = p["z_top"] - p["z_bot"]
    pitch = height * math.tan(math.radians(90 - p["helix_angle_deg"]))
    n_revs = max(0.5, height / max(pitch, 1e-3))

    points = vtk.vtkPoints()
    lines = vtk.vtkCellArray()
    for f in range(p["n_flutes"]):
        phase_offset = 2 * math.pi * f / p["n_flutes"]
        line = vtk.vtkPolyLine()
        line.GetPointIds().SetNumberOfIds(p["n_segments"] + 1)
        for i in range(p["n_segments"] + 1):
            t = i / p["n_segments"]
            a = phase_offset + 2 * math.pi * n_revs * t
            r = radius * 1.005
            x = r * math.cos(a)
            y = r * math.sin(a)
            z = p["z_bot"] + t * height
            pt_id = points.InsertNextPoint(x, y, z)
            line.GetPointIds().SetId(i, pt_id)
        lines.InsertNextCell(line)

    pd = vtk.vtkPolyData()
    pd.SetPoints(points)
    pd.SetLines(lines)
    helix_actor.GetMapper().SetInputData(pd)


def make_tool_actor(bit_diameter=DEFAULT_BIT_DIA):
    """Build a realistic-looking end-mill assembly with tip at local (0,0,0).

    Position by calling update_tool_position(actor, x, y, z, spindle_on),
    which translates the entire assembly so the tip is at world (x, y, z).
    """
    radius = bit_diameter / 2.0

    # Cutting flute section (bottom) — dark gunmetal cylinder
    flute = _make_cylinder_actor(
        radius, 0.0, FLUTE_HEIGHT, COLOR_FLUTE,
        specular=0.25, spec_power=20, ambient=0.25,
    )
    # Helical edges over the flute section — sells the "real bit" look
    helix = _make_flute_helix_actor(radius, 0.0, FLUTE_HEIGHT)
    # Thin transition band between flute and shank
    band = _make_cylinder_actor(
        radius * 1.02, FLUTE_HEIGHT - 0.4, FLUTE_HEIGHT + 0.4, COLOR_BAND,
        specular=0.05, spec_power=10,
    )
    # Smooth polished shank above the flute
    shank = _make_cylinder_actor(
        radius, FLUTE_HEIGHT, SHAFT_HEIGHT, COLOR_SHANK,
        specular=0.8, spec_power=80, ambient=0.3,
    )
    # Spindle status LED — slightly wider than the shank, near the top
    led = _make_cylinder_actor(
        radius * 1.18,
        SHAFT_HEIGHT - LED_GAP - LED_HEIGHT, SHAFT_HEIGHT - LED_GAP,
        COLOR_LED_OFF, specular=0.1, spec_power=10, ambient=0.7,
    )

    assembly = vtk.vtkAssembly()
    assembly.AddPart(flute)
    assembly.AddPart(helix)
    assembly.AddPart(band)
    assembly.AddPart(shank)
    assembly.AddPart(led)

    # Stash references for later updates
    assembly._flute = flute
    assembly._helix = helix
    assembly._band = band
    assembly._shank = shank
    assembly._led = led
    return assembly


def update_tool_position(actor, x, y, z, spindle_on):
    """Translate the bit so its tip lands at world (x, y, z). LED ring color
    reflects spindle state — orange when idle, green when M3 is active.

    The bit itself does not rotate during animation. Visual rotation looked
    cool but cost us framerate on integrated-graphics laptops; the LED is a
    lighter-weight cue for the same information.
    """
    t = vtk.vtkTransform()
    t.Translate(x, y, z)
    actor.SetUserTransform(t)
    if hasattr(actor, "_led"):
        c = COLOR_LED_ON if spindle_on else COLOR_LED_OFF
        actor._led.GetProperty().SetColor(*c)


def update_tool_diameter(actor, bit_diameter):
    """Resize the bit for a new diameter. Cheap (~1 ms): updates the cylinder
    radii in place and regenerates the helix line points."""
    radius = bit_diameter / 2.0
    if hasattr(actor, "_flute"):
        actor._flute._cyl_source.SetRadius(radius)
        actor._flute._cyl_source.Update()
    if hasattr(actor, "_band"):
        actor._band._cyl_source.SetRadius(radius * 1.02)
        actor._band._cyl_source.Update()
    if hasattr(actor, "_shank"):
        actor._shank._cyl_source.SetRadius(radius)
        actor._shank._cyl_source.Update()
    if hasattr(actor, "_led"):
        actor._led._cyl_source.SetRadius(radius * 1.18)
        actor._led._cyl_source.Update()
    if hasattr(actor, "_helix"):
        _rebuild_helix_geometry(actor._helix, radius)
