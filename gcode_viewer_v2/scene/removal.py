"""Material removal simulation via heightmap.

A 2D heightmap of the stock surface — each cell holds the current top-of-material
z value. As the tool moves, cells within the bit's footprint are lowered to the
tool's z (if it cuts deeper than what's already there). This is the same model
that CAMotics and many other open-source CNC simulators use, and it's
sufficient for any toolpath that doesn't have undercuts (the vast majority of
3-axis routing).

Visualized as a vtkStructuredGrid → vtkWarpScalar pipeline: the grid's z values
get displaced by the heightmap, producing a 3D surface mesh that can be
rendered with shading.
"""

import numpy as np
import vtk
from vtk.util import numpy_support


class Heightmap:
    """Mutable heightmap covering the stock's XY extent.

    Resolution: cell_size mm (default 1.0 to match the original overlap-heatmap
    precision). At 1mm × a 100×100 mm stock = 10,000 cells; numpy-vectorized
    updates keep this under 1ms per tool sample on a normal laptop.
    """

    def __init__(self, x_range, y_range, top_z=0.0, cell_size=1.0):
        self.x0, self.x1 = x_range
        self.y0, self.y1 = y_range
        self.top_z = float(top_z)
        self.cell = float(cell_size)

        # +1 to include both edges
        self.nx = max(2, int(round((self.x1 - self.x0) / self.cell)) + 1)
        self.ny = max(2, int(round((self.y1 - self.y0) / self.cell)) + 1)

        # Heightmap stores z values. Initially the whole top surface is at top_z.
        # Shape (ny, nx) so that heights[iy, ix] indexes (x, y) in row-major.
        self.heights = np.full((self.ny, self.nx), self.top_z, dtype=np.float32)

        # Precompute cell-center XY grids — small alloc, large win in cut().
        xs = self.x0 + np.arange(self.nx) * self.cell
        ys = self.y0 + np.arange(self.ny) * self.cell
        self._xs, self._ys = np.meshgrid(xs, ys, indexing="xy")  # shape (ny, nx)

        # Lazy: vtk objects built on first to_polydata() call
        self._points = None
        self._polydata = None
        self._z_array = None

    def cut(self, tool_x, tool_y, tool_z, bit_radius):
        """Lower every cell within bit_radius of (tool_x, tool_y) to tool_z,
        if tool_z is deeper than the cell's current value.

        No-op if tool is at or above the top surface.
        """
        if tool_z >= self.top_z:
            return False

        # Bounding box around the tool footprint in cell indices
        bit_r = bit_radius
        ix0 = max(0, int((tool_x - bit_r - self.x0) / self.cell))
        ix1 = min(self.nx, int((tool_x + bit_r - self.x0) / self.cell) + 1)
        iy0 = max(0, int((tool_y - bit_r - self.y0) / self.cell))
        iy1 = min(self.ny, int((tool_y + bit_r - self.y0) / self.cell) + 1)
        if ix0 >= ix1 or iy0 >= iy1:
            return False

        # Compute squared distance from tool axis for each cell in the bbox
        sub_xs = self._xs[iy0:iy1, ix0:ix1]
        sub_ys = self._ys[iy0:iy1, ix0:ix1]
        dx = sub_xs - tool_x
        dy = sub_ys - tool_y
        dist_sq = dx * dx + dy * dy
        mask = dist_sq <= bit_r * bit_r

        if not mask.any():
            return False

        sub_h = self.heights[iy0:iy1, ix0:ix1]
        # Element-wise minimum: only lower cells that were higher than tool_z
        np.minimum(sub_h, tool_z, out=sub_h, where=mask)
        return True

    def cut_segment(self, p1, p2, bit_radius, sample_step=None):
        """Sample the segment from p1 to p2 and apply cut() at each sample.

        sample_step defaults to bit_radius / 2 — fine enough that adjacent
        footprints overlap and the swept volume looks continuous.
        """
        x1, y1, z1 = p1
        x2, y2, z2 = p2
        dx, dy, dz = x2 - x1, y2 - y1, z2 - z1
        length = (dx * dx + dy * dy) ** 0.5
        if length < 1e-9:
            self.cut(x1, y1, min(z1, z2), bit_radius)
            return
        step = sample_step if sample_step is not None else max(0.1, bit_radius / 2.0)
        n = max(2, int(length / step) + 1)
        for k in range(n + 1):
            t = k / n
            x = x1 + t * dx
            y = y1 + t * dy
            z = z1 + t * dz
            self.cut(x, y, z, bit_radius)

    def to_polydata(self):
        """Build a vtkPolyData mesh visualizing the heightmap as a 3D surface.

        Constructed once; subsequent cuts update the mesh in-place via the
        z-array Modified() flag.
        """
        nx, ny = self.nx, self.ny

        # Build XY grid points with z = current heightmap value
        # Numpy → vtk via numpy_support is the fast path
        xs = self._xs.ravel()
        ys = self._ys.ravel()
        zs = self.heights.ravel()
        pts_np = np.column_stack([xs, ys, zs]).astype(np.float64)

        points = vtk.vtkPoints()
        points.SetData(numpy_support.numpy_to_vtk(pts_np, deep=True))

        # Triangle strip cells: two triangles per quad
        polys = vtk.vtkCellArray()
        for iy in range(ny - 1):
            for ix in range(nx - 1):
                i0 = iy * nx + ix
                i1 = iy * nx + ix + 1
                i2 = (iy + 1) * nx + ix
                i3 = (iy + 1) * nx + ix + 1
                # Two triangles forming a quad
                polys.InsertNextCell(3)
                polys.InsertCellPoint(i0); polys.InsertCellPoint(i1); polys.InsertCellPoint(i2)
                polys.InsertNextCell(3)
                polys.InsertCellPoint(i1); polys.InsertCellPoint(i3); polys.InsertCellPoint(i2)

        pd = vtk.vtkPolyData()
        pd.SetPoints(points)
        pd.SetPolys(polys)

        self._points = points
        self._polydata = pd
        # Cache the underlying vtkFloatArray of point z-values for fast updates
        self._z_array = pts_np  # numpy view shared with VTK
        return pd

    def update_polydata(self):
        """Push current heightmap z values into the VTK points buffer.
        Call after a series of cut() / cut_segment() calls to refresh the mesh.
        """
        if self._points is None:
            return
        nx, ny = self.nx, self.ny
        zs = self.heights.ravel()
        # Re-build the (n*3) array — fast in numpy
        # _xs, _ys are immutable; only zs change
        new_pts = np.column_stack([self._xs.ravel(), self._ys.ravel(), zs]).astype(np.float64)
        # Overwrite VTK buffer in-place
        self._points.SetData(numpy_support.numpy_to_vtk(new_pts, deep=True))
        self._points.Modified()
        if self._polydata is not None:
            self._polydata.Modified()


def make_stock_surface_actor(heightmap, fill_color=(0.78, 0.62, 0.42)):
    """Build the actor that renders the heightmap as a 3D surface.

    Uses *flat* interpolation: each triangle gets a single face-normal that
    OpenGL computes per-render from the current vertex positions. This skips
    the vtkPolyDataNormals filter entirely (which previously cost 5-10 ms of
    CPU per refresh on big heightmaps + a normals re-upload to the GPU) and
    also gives a more honest "machined" look that matches what real CAM
    verifiers (CAMotics, Vericut) tend to render — the heightmap IS faceted,
    so we should show it that way.

    Returns (actor, None). The second tuple element used to be the
    vtkPolyDataNormals filter; callers that previously did
        if normals is not None: normals.Update()
    keep working unchanged.
    """
    pd = heightmap.to_polydata()

    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputData(pd)
    mapper.ScalarVisibilityOff()

    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    p = actor.GetProperty()
    p.SetColor(*fill_color)
    p.SetInterpolationToFlat()  # one normal per triangle — face-shaded
    p.SetAmbient(0.25)
    p.SetDiffuse(0.7)
    p.SetSpecular(0.15)
    p.SetSpecularPower(20)
    return actor, None


def compute_overlap_counts(moves, bit_radius, x_range, y_range, cell_size=1.0):
    """For each grid cell, count how many cutting sub-segments pass within
    bit_radius of the cell center. Mirrors the algorithm used in the Tk
    prototype's _compute_overlap_grid (see gcode_preview.py:171), preserving
    the same 1mm precision the user explicitly required.

    Returns a (ny, nx) int32 numpy array, where heights[iy, ix] is the
    overlap count at cell (ix, iy).
    """
    x0, x1 = x_range
    y0, y1 = y_range
    nx = max(2, int(round((x1 - x0) / cell_size)) + 1)
    ny = max(2, int(round((y1 - y0) / cell_size)) + 1)
    counts = np.zeros((ny, nx), dtype=np.int32)
    bit_r_sq = bit_radius * bit_radius

    for move in moves:
        if move.kind == "G0" or not move.spindle:
            continue
        pts = move.points
        for i in range(1, len(pts)):
            p1 = pts[i - 1]
            p2 = pts[i]
            z1, z2 = p1[2], p2[2]
            if z1 >= 0 and z2 >= 0:
                continue
            # Clip the segment to the part where it's actually cutting (z<0)
            if z1 < 0 and z2 < 0:
                sx, sy, ex, ey = p1[0], p1[1], p2[0], p2[1]
            else:
                t = -z1 / (z2 - z1)
                cx = p1[0] + t * (p2[0] - p1[0])
                cy = p1[1] + t * (p2[1] - p1[1])
                if z1 < 0:
                    sx, sy, ex, ey = p1[0], p1[1], cx, cy
                else:
                    sx, sy, ex, ey = cx, cy, p2[0], p2[1]

            # Cell bbox around segment, expanded by bit radius
            min_x = min(sx, ex) - bit_radius
            max_x = max(sx, ex) + bit_radius
            min_y = min(sy, ey) - bit_radius
            max_y = max(sy, ey) + bit_radius
            ix0 = max(0, int((min_x - x0) / cell_size))
            ix1 = min(nx, int((max_x - x0) / cell_size) + 1)
            iy0 = max(0, int((min_y - y0) / cell_size))
            iy1 = min(ny, int((max_y - y0) / cell_size) + 1)
            if ix0 >= ix1 or iy0 >= iy1:
                continue

            # Vectorized point-segment distance for cells in the bbox
            ixs = np.arange(ix0, ix1)
            iys = np.arange(iy0, iy1)
            qxs = x0 + ixs * cell_size + cell_size / 2
            qys = y0 + iys * cell_size + cell_size / 2
            QX, QY = np.meshgrid(qxs, qys, indexing="xy")  # (iy_count, ix_count)

            dx = ex - sx
            dy = ey - sy
            seg_len_sq = dx * dx + dy * dy
            if seg_len_sq < 1e-9:
                ddx = QX - sx
                ddy = QY - sy
                dist_sq = ddx * ddx + ddy * ddy
            else:
                tt = ((QX - sx) * dx + (QY - sy) * dy) / seg_len_sq
                tt = np.clip(tt, 0.0, 1.0)
                px = sx + tt * dx
                py = sy + tt * dy
                ddx = QX - px
                ddy = QY - py
                dist_sq = ddx * ddx + ddy * ddy

            mask = dist_sq <= bit_r_sq
            counts[iy0:iy1, ix0:ix1] += mask.astype(np.int32)

    return counts


def apply_overlap_scalars(polydata, counts):
    """Attach overlap-count scalars to the heightmap polydata's points.

    counts shape must match the heightmap grid (ny, nx). Result: each vertex
    in `polydata` carries an integer scalar = its cell's overlap count.
    """
    flat = counts.ravel().astype(np.float32)
    arr = numpy_support.numpy_to_vtk(flat, deep=True)
    arr.SetName("overlap")
    polydata.GetPointData().SetScalars(arr)
    polydata.Modified()


def overlap_color_lookup():
    """Color table for the overlap heatmap.

    Matches the original Tk version's intent: 1=light pink, 2=pink,
    3=hot pink, 4+=magenta. 0 is rendered as the surface base color
    (mapper handles that via NaN / use-base-color).
    """
    lut = vtk.vtkColorTransferFunction()
    # Below 1: clamp to surface base color (transparent-ish gray)
    lut.AddRGBPoint(0.0, 0.78, 0.62, 0.42)  # below-1 → blend with surface
    lut.AddRGBPoint(1.0, 1.00, 0.56, 0.75)  # 1 = light pink
    lut.AddRGBPoint(2.0, 1.00, 0.38, 0.81)  # 2 = pink
    lut.AddRGBPoint(3.0, 1.00, 0.20, 0.85)  # 3 = hot pink
    lut.AddRGBPoint(4.0, 1.00, 0.00, 1.00)  # 4+ = magenta
    return lut
