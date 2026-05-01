"""Stock block — the workpiece the bit is cutting into.

For Day 2 this is a static translucent box sized to the toolpath's XY bounds
plus a margin, with depth equal to the deepest cut. Day 3 will turn this
into an editable mesh that has material removed as the tool moves.
"""

import vtk


def make_stock_actor(bounds, margin=10.0, top_z=0.0, fill_color=(0.78, 0.62, 0.42)):
    """Build a translucent stock block actor.

    bounds: ((min_x, min_y, min_z), (max_x, max_y, max_z)) from parser.bounds()
    margin: extra mm added on every side beyond the toolpath XY extent
    top_z: top surface of the stock (default 0 — assumes Z=0 is workpiece top)
    fill_color: warm wood-ish default; caller can override per material
    """
    (mn, mx) = bounds
    x0, y0, z0 = mn[0] - margin, mn[1] - margin, mn[2]
    x1, y1, z1 = mx[0] + margin, mx[1] + margin, top_z

    # Ensure non-zero thickness even if all moves are at z=0
    if z1 - z0 < 0.5:
        z0 = z1 - 5.0

    cube = vtk.vtkCubeSource()
    cube.SetBounds(x0, x1, y0, y1, z0, z1)
    cube.Update()

    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputConnection(cube.GetOutputPort())

    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    p = actor.GetProperty()
    p.SetColor(*fill_color)
    p.SetOpacity(0.25)
    p.SetEdgeVisibility(True)
    p.SetEdgeColor(0.5, 0.4, 0.3)
    p.SetLineWidth(1.5)
    return actor


def stock_dimensions(bounds, margin=10.0):
    """Return ((x0, x1), (y0, y1), (z0, z1)) for a stock block sized to the toolpath."""
    (mn, mx) = bounds
    x0, y0, z0 = mn[0] - margin, mn[1] - margin, mn[2]
    x1, y1, z1 = mx[0] + margin, mx[1] + margin, 0.0
    if z1 - z0 < 0.5:
        z0 = z1 - 5.0
    return (x0, x1), (y0, y1), (z0, z1)
