"""Toolpath geometry as a vtkPolyData line plot.

Converts a list of parser.Move objects into vtkPolyData with one polyline cell
per move. Cuts (G1/G2/G3) and rapids (G0) are returned as two separate actors
so they can be styled differently (color, line width, dash).

Coloring strategy: per-point scalar = z value, mapped through a vtkColorTransferFunction
that reproduces the original tool's "depth gradient" — green at z≈0, fading
through yellow/orange/red/purple as z goes more negative.
"""

import vtk
import numpy as np


def _depth_lookup_table(min_z):
    """5-stop gradient from the original Tk viewer (green → yellow → orange → red → purple),
    keyed off the deepest cut z (clamped to -1 for files without negative z).
    """
    min_z = min(min_z, -1.0)  # ensure at least one stop range
    ctf = vtk.vtkColorTransferFunction()
    # stops as (relative-t, r, g, b) — t=0 at z=0 (surface), t=1 at min_z (deepest)
    stops = [
        (0.00, 0x6f / 255, 0xff / 255, 0xa0 / 255),
        (0.25, 0xff / 255, 0xd9 / 255, 0x3d / 255),
        (0.55, 0xff / 255, 0x7a / 255, 0x1f / 255),
        (0.85, 0xd6 / 255, 0x1a / 255, 0x1a / 255),
        (1.00, 0x4a / 255, 0x00 / 255, 0x40 / 255),
    ]
    for t, r, g, b in stops:
        z = t * min_z
        ctf.AddRGBPoint(z, r, g, b)
    # Add a stop at z=+1 so anything above the surface clamps to the green end
    ctf.AddRGBPoint(1.0, 0x6f / 255, 0xff / 255, 0xa0 / 255)
    return ctf


def build_polydata(moves, kinds):
    """Build a vtkPolyData with all points from the given moves merged into a
    single vtkPoints, and one polyline cell per move (only for moves whose
    kind is in `kinds`).

    Returns (polydata, point_count) — point_count helps callers decide on
    rendering strategy (e.g., whether to enable point smoothing).
    """
    pts = vtk.vtkPoints()
    lines = vtk.vtkCellArray()
    z_values = vtk.vtkFloatArray()
    z_values.SetName("z")

    pt_idx = 0
    for m in moves:
        if m.kind not in kinds:
            continue
        n = len(m.points)
        if n < 2:
            continue
        # Insert points
        start_id = pt_idx
        for px, py, pz in m.points:
            pts.InsertNextPoint(px, py, pz)
            z_values.InsertNextValue(pz)
            pt_idx += 1
        # Polyline cell referencing those points
        line = vtk.vtkPolyLine()
        line.GetPointIds().SetNumberOfIds(n)
        for i in range(n):
            line.GetPointIds().SetId(i, start_id + i)
        lines.InsertNextCell(line)

    pd = vtk.vtkPolyData()
    pd.SetPoints(pts)
    pd.SetLines(lines)
    pd.GetPointData().SetScalars(z_values)
    return pd, pt_idx


def make_cut_actor(moves, min_z):
    """Actor for cutting moves (G1/G2/G3), colored by depth gradient."""
    pd, n = build_polydata(moves, kinds={"G1", "G2", "G3"})

    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputData(pd)
    mapper.SetLookupTable(_depth_lookup_table(min_z))
    mapper.SetScalarRange(min_z, 0.0)
    mapper.SetScalarModeToUsePointData()
    mapper.SetColorModeToMapScalars()

    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    actor.GetProperty().SetLineWidth(2.0)
    return actor, n


def make_rapid_actor(moves):
    """Actor for rapid moves (G0), drawn as thin dashed yellow lines."""
    pd, n = build_polydata(moves, kinds={"G0"})

    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputData(pd)
    mapper.ScalarVisibilityOff()

    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    p = actor.GetProperty()
    p.SetColor(0.6, 0.6, 0.3)
    p.SetLineWidth(1.0)
    p.SetLineStipplePattern(0xF0F0)  # dashed (legacy GL — ignored on modern OpenGL renderers)
    p.SetLineStippleRepeatFactor(1)
    return actor, n


