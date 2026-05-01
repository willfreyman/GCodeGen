"""Interactive view-cube widget — hover highlights a face, click snaps the
main camera to that face's orthographic view.

Replaces the previous static vtkAnnotatedCubeActor + vtkOrientationMarkerWidget
combo. Building the cube as 6 distinct quad actors lets us pick which face
is under the cursor (vtkPropPicker returns a specific actor) and re-color
it independently. Labels are vtkVectorText oriented per-face so they read
upright when the user looks at that face head-on.

Layout: dedicated renderer in the top-right corner viewport, on its own
layer above the main scene + highlight overlay. The cube's camera mirrors
the main camera's direction-of-view so the cube re-orients in lock-step.
"""

import math
import vtk


# Each main face: (outward_normal, view_up_when_looking_at_face_from_outside)
FACES = {
    "TOP":    ((0.0,  0.0,  1.0), (0.0,  1.0,  0.0)),
    "BOTTOM": ((0.0,  0.0, -1.0), (0.0,  1.0,  0.0)),
    "FRONT":  ((0.0, -1.0,  0.0), (0.0,  0.0,  1.0)),
    "BACK":   ((0.0,  1.0,  0.0), (0.0,  0.0,  1.0)),
    "RIGHT":  ((1.0,  0.0,  0.0), (0.0,  0.0,  1.0)),
    "LEFT":   ((-1.0, 0.0,  0.0), (0.0,  0.0,  1.0)),
}

# 8 corners — chamfered into small triangular click-targets that snap to iso
# views. Each corner is identified by the sign-vector of its position.
# The view_up is +Z for every corner so all iso views show TOP at top.
CORNER_VIEW_UP = (0.0, 0.0, 1.0)
CORNERS = [(sx, sy, sz) for sx in (+1, -1) for sy in (+1, -1) for sz in (+1, -1)]


def _corner_name(sx, sy, sz):
    """Stable key for the 8 corner faces (used for actor lookup + snap dispatch)."""
    return f"CORNER_{sx:+d}_{sy:+d}_{sz:+d}"


CUBE_BODY_COLOR = (0.32, 0.36, 0.42)  # cool gray (matches the previous look)
HIGHLIGHT_COLOR = (0.55, 0.70, 0.95)  # soft blue when hovered
TEXT_COLOR      = (0.95, 0.96, 1.00)
EDGE_COLOR      = (0.18, 0.20, 0.24)  # darker — only used to outline polygons
TEXT_HEIGHT     = 0.30  # text height as fraction of face dimension
CHAMFER_DEPTH   = 0.15  # how much of each half-edge to shave at corners (0..0.5)


# ── Tiny vector helpers (avoids pulling numpy into this module) ─────────────

def _norm(v):
    l = math.sqrt(v[0] ** 2 + v[1] ** 2 + v[2] ** 2)
    return (v[0] / l, v[1] / l, v[2] / l) if l > 1e-12 else (0.0, 0.0, 0.0)


def _cross(a, b):
    return (
        a[1] * b[2] - a[2] * b[1],
        a[2] * b[0] - a[0] * b[2],
        a[0] * b[1] - a[1] * b[0],
    )


def _add(a, b):
    return (a[0] + b[0], a[1] + b[1], a[2] + b[2])


def _sub(a, b):
    return (a[0] - b[0], a[1] - b[1], a[2] - b[2])


def _scale(v, s):
    return (v[0] * s, v[1] * s, v[2] * s)


def _make_polygon_actor(verts):
    """Build a single-cell polygon actor (octagon, triangle, etc.).

    Edges are drawn via the actor's edge-visibility property — saves us from
    maintaining a separate vtkExtractEdges actor that would have to track
    the chamfered geometry. Flat shading on each face makes the chamfer
    transitions read clearly under normal lighting.
    """
    points = vtk.vtkPoints()
    for v in verts:
        points.InsertNextPoint(*v)

    polygon = vtk.vtkPolygon()
    polygon.GetPointIds().SetNumberOfIds(len(verts))
    for i in range(len(verts)):
        polygon.GetPointIds().SetId(i, i)

    cells = vtk.vtkCellArray()
    cells.InsertNextCell(polygon)

    pd = vtk.vtkPolyData()
    pd.SetPoints(points)
    pd.SetPolys(cells)

    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputData(pd)

    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    p = actor.GetProperty()
    p.SetColor(*CUBE_BODY_COLOR)
    p.SetInterpolationToFlat()  # show chamfer facets distinctly
    p.SetAmbient(0.45)
    p.SetDiffuse(0.55)
    p.SetSpecular(0.0)
    p.SetEdgeVisibility(True)
    p.SetEdgeColor(*EDGE_COLOR)
    p.SetLineWidth(1.0)
    return actor


# ── ViewCube ────────────────────────────────────────────────────────────────

class ViewCube:
    """Interactive cube widget pinned to the top-right corner of a render window.

    Construct, then call enable() to hook mouse events. Disposal is implicit
    when the render window is destroyed; if you need to detach explicitly,
    call disable().
    """

    def __init__(self, render_window, main_renderer, interactor):
        self.render_window = render_window
        self.main_renderer = main_renderer
        self.main_camera = main_renderer.GetActiveCamera()
        self.interactor = interactor

        # Make sure there's a layer for us above main + overlay
        if render_window.GetNumberOfLayers() < 3:
            render_window.SetNumberOfLayers(3)

        # Dedicated renderer for the cube in the top-right corner
        self.cube_renderer = vtk.vtkRenderer()
        self.cube_renderer.SetLayer(2)
        self.cube_renderer.SetViewport(0.80, 0.78, 1.00, 0.99)
        self.cube_renderer.SetBackgroundAlpha(0.0)
        self.cube_renderer.SetInteractive(False)
        self.cube_renderer.GetActiveCamera().ParallelProjectionOn()  # ortho — looks more stable
        render_window.AddRenderer(self.cube_renderer)

        # Build the cube: 6 face actors, 6 text labels, 1 edges actor
        self._face_actors = {}
        self._build_cube()

        # Sync cube camera to main camera
        self._sync_camera()
        self._cam_observer = self.main_camera.AddObserver(
            "ModifiedEvent", lambda obj, evt: self._sync_camera()
        )

        # Mouse state
        self._hovered = None
        self._mouse_observer = None
        self._click_observer = None
        self._picker = vtk.vtkPropPicker()

    # ── Public API ──────────────────────────────────────────────────────────

    def enable(self):
        if self._mouse_observer is None:
            self._mouse_observer = self.interactor.AddObserver(
                "MouseMoveEvent", self._on_mouse_move
            )
        if self._click_observer is None:
            self._click_observer = self.interactor.AddObserver(
                "LeftButtonPressEvent", self._on_click
            )

    def disable(self):
        if self._mouse_observer is not None:
            self.interactor.RemoveObserver(self._mouse_observer)
            self._mouse_observer = None
        if self._click_observer is not None:
            self.interactor.RemoveObserver(self._click_observer)
            self._click_observer = None

    # ── Cube construction ──────────────────────────────────────────────────

    def _build_cube(self):
        # 6 main faces — each is an octagon (the cube's chamfered cube faces
        # lose 4 corners and gain 4 chamfer edges, becoming 8-sided)
        for name, (normal, view_up) in FACES.items():
            face = self._build_octagon_face(normal, view_up)
            face._face_name = name
            self.cube_renderer.AddActor(face)
            self._face_actors[name] = face

            label = self._build_label(name, normal, view_up)
            label.PickableOff()
            self.cube_renderer.AddActor(label)

        # 8 corner faces — small triangles, one per chamfered corner
        for (sx, sy, sz) in CORNERS:
            cname = _corner_name(sx, sy, sz)
            corner_actor = self._build_corner_triangle(sx, sy, sz)
            corner_actor._face_name = cname
            self.cube_renderer.AddActor(corner_actor)
            self._face_actors[cname] = corner_actor

    def _build_octagon_face(self, normal, view_up):
        """A single octagonal face — a cube face with its 4 corners chamfered.

        Vertices are emitted in CCW order (viewed from +n) so the polygon's
        outward normal is correct without manual normal management.
        """
        n = _norm(normal)
        vu = _norm(view_up)
        right = _cross(vu, n)

        center = _scale(n, 0.5)
        h = 0.5
        d = CHAMFER_DEPTH

        # 8 octagon vertices (u, v) coefficients in CCW order from +n
        uv = [
            ( h - d,  h),
            (-(h - d),  h),
            (-h,  h - d),
            (-h, -(h - d)),
            (-(h - d), -h),
            ( h - d, -h),
            ( h, -(h - d)),
            ( h,  h - d),
        ]
        verts = [
            _add(center, _add(_scale(right, u), _scale(vu, v)))
            for (u, v) in uv
        ]
        return _make_polygon_actor(verts)

    def _build_corner_triangle(self, sx, sy, sz):
        """One of the 8 chamfered-corner triangles. Outward normal is the
        corner's diagonal direction (sx, sy, sz)/√3.
        """
        h = 0.5
        d = CHAMFER_DEPTH
        # 3 vertices on the 3 cube edges meeting at this corner, each shifted
        # inward by `d` from the corner.
        v_along_x = (sx * (h - d), sy * h, sz * h)
        v_along_y = (sx * h, sy * (h - d), sz * h)
        v_along_z = (sx * h, sy * h, sz * (h - d))

        # CCW-from-outside winding flips with the parity of the sign product.
        # See cross-product derivation in the module docstring.
        if sx * sy * sz > 0:
            verts = [v_along_x, v_along_y, v_along_z]
        else:
            verts = [v_along_x, v_along_z, v_along_y]
        return _make_polygon_actor(verts)

    def _build_label(self, name, normal, view_up):
        """Text label that lies flat on a face and reads upright when viewed
        from outside the face."""
        text = vtk.vtkVectorText()
        text.SetText(name)
        text.Update()
        bounds = text.GetOutput().GetBounds()
        text_w = bounds[1] - bounds[0]
        text_h = bounds[3] - bounds[2]

        # Scale so the text height fits TEXT_HEIGHT of a face dimension,
        # but never let the width exceed 0.85 of face dimension.
        max_w = 0.85
        if text_h > 0 and text_w > 0:
            scale = min(TEXT_HEIGHT / text_h, max_w / text_w)
        else:
            scale = TEXT_HEIGHT

        n = _norm(normal)
        vu = _norm(view_up)
        right = _cross(vu, n)

        # Rotation matrix mapping vtkVectorText's local axes to the face's
        # axes: text-X → right, text-Y → vu, text-Z → n.
        rot = vtk.vtkMatrix4x4()
        rot.Identity()
        for i in range(3):
            rot.SetElement(i, 0, right[i])
            rot.SetElement(i, 1, vu[i])
            rot.SetElement(i, 2, n[i])

        # Build the transform in PostMultiply mode so calls apply in order.
        # 1) Center the text on its own origin
        # 2) Scale to fit
        # 3) Rotate to face orientation
        # 4) Translate just outside the face surface (avoid z-fighting)
        offset = 0.005
        world_pos = _scale(n, 0.5 + offset)

        transform = vtk.vtkTransform()
        transform.PostMultiply()
        transform.Translate(-text_w / 2.0, -text_h / 2.0, 0.0)
        transform.Scale(scale, scale, scale)
        transform.Concatenate(rot)
        transform.Translate(*world_pos)

        tf = vtk.vtkTransformPolyDataFilter()
        tf.SetInputConnection(text.GetOutputPort())
        tf.SetTransform(transform)
        tf.Update()

        mapper = vtk.vtkPolyDataMapper()
        mapper.SetInputConnection(tf.GetOutputPort())

        actor = vtk.vtkActor()
        actor.SetMapper(mapper)
        actor.GetProperty().SetColor(*TEXT_COLOR)
        actor.GetProperty().SetLighting(False)
        return actor

    # ── Camera sync ────────────────────────────────────────────────────────

    def _sync_camera(self):
        """Mirror the main camera's view direction onto the cube's camera so
        the cube's orientation reflects what the main scene is showing."""
        cube_cam = self.cube_renderer.GetActiveCamera()
        mp = self.main_camera.GetPosition()
        mf = self.main_camera.GetFocalPoint()
        d = _sub(mp, mf)
        l = math.sqrt(d[0] ** 2 + d[1] ** 2 + d[2] ** 2)
        if l < 1e-9:
            return
        # Place cube cam at unit-distance in the same direction. The cube is
        # ~1 unit across; 4 units of distance gives a comfortable framing.
        n = (d[0] / l, d[1] / l, d[2] / l)
        cube_cam.SetPosition(n[0] * 4.0, n[1] * 4.0, n[2] * 4.0)
        cube_cam.SetFocalPoint(0.0, 0.0, 0.0)
        cube_cam.SetViewUp(*self.main_camera.GetViewUp())
        # Parallel-projection scale: the cube spans 1 unit, half-width is 0.5;
        # at iso, the visible diagonal extends to ~0.87. Use 0.95 to leave a
        # comfortable margin on every edge regardless of camera angle.
        cube_cam.SetParallelScale(0.95)
        self.cube_renderer.ResetCameraClippingRange()

    # ── Mouse handling ─────────────────────────────────────────────────────

    def _is_in_viewport(self, x, y):
        """Is screen position (x, y) inside the cube's corner viewport?"""
        size = self.render_window.GetSize()
        if size[0] == 0 or size[1] == 0:
            return False
        nx = x / size[0]
        ny = y / size[1]
        vp = self.cube_renderer.GetViewport()
        return vp[0] <= nx <= vp[2] and vp[1] <= ny <= vp[3]

    def _pick_face(self, x, y):
        if not self._is_in_viewport(x, y):
            return None
        self._picker.Pick(x, y, 0, self.cube_renderer)
        prop = self._picker.GetActor()
        if prop is None:
            return None
        return getattr(prop, "_face_name", None)

    def _on_mouse_move(self, iren, event):
        x, y = iren.GetEventPosition()
        face = self._pick_face(x, y)
        if face != self._hovered:
            if self._hovered is not None:
                self._face_actors[self._hovered].GetProperty().SetColor(*CUBE_BODY_COLOR)
            if face is not None:
                self._face_actors[face].GetProperty().SetColor(*HIGHLIGHT_COLOR)
            self._hovered = face
            self.render_window.Render()

    def _on_click(self, iren, event):
        x, y = iren.GetEventPosition()
        face = self._pick_face(x, y)
        if face is not None:
            self._snap_to_face(face)

    def _snap_to_face(self, face_name):
        """Move the main camera so it views the named face dead-on (or, for a
        chamfered corner, looks along the corner's diagonal so the three
        adjacent main faces are all visible). Preserves focal point and
        distance — only direction changes."""
        if face_name in FACES:
            normal, view_up = FACES[face_name]
            normal = _norm(normal)
        elif face_name.startswith("CORNER_"):
            # Parse "CORNER_+1_-1_+1" → (1, -1, 1)
            parts = face_name.split("_")
            sx, sy, sz = int(parts[1]), int(parts[2]), int(parts[3])
            normal = _norm((float(sx), float(sy), float(sz)))
            view_up = CORNER_VIEW_UP
        else:
            return  # unknown name — nothing to snap to

        focal = self.main_camera.GetFocalPoint()
        pos = self.main_camera.GetPosition()
        d = _sub(pos, focal)
        dist = math.sqrt(d[0] ** 2 + d[1] ** 2 + d[2] ** 2)
        if dist < 1e-9:
            dist = 100.0
        new_pos = _add(focal, _scale(normal, dist))
        self.main_camera.SetPosition(*new_pos)
        self.main_camera.SetViewUp(*view_up)
        self.main_renderer.ResetCameraClippingRange()
        self.render_window.Render()
