"""Main application window for the v2 viewer.

Hosts a QVTKRenderWindowInteractor as the central widget, with a top
toolbar (file open, transport controls, speed/bit settings) and a
right-side dock holding the Z-axis indicator.

Animation is driven by a QTimer at 60 Hz that advances along the move
list, updates the tool actor's transform, and triggers a VTK render.
"""

import math
import time

from PyQt5 import QtCore, QtGui, QtWidgets
from vtkmodules.qt.QVTKRenderWindowInteractor import QVTKRenderWindowInteractor
import vtk

# Suppress VTK's native popup window on Windows.
#
# Background: VTK's default output-window singleton on Windows is a
# vtkWin32OutputWindow subclass that opens a native dialog whenever any
# warning fires (and a stale "vtkOutputWindow" empty-frame appears on import
# even before warnings exist). SetDisplayMode on that subclass is unreliable
# — the right fix is to *replace* the singleton with the base vtkOutputWindow
# class, which writes to stderr cross-platform and never opens a window.
#
# Done at module import so it takes effect before any VTK object emits.
_stderr_output = vtk.vtkOutputWindow()
_stderr_output.SetDisplayModeToAlwaysStdErr()
vtk.vtkOutputWindow.SetInstance(_stderr_output)

from .. import parser
from ..scene import path as scene_path
from ..scene import stock as scene_stock
from ..scene import tool as scene_tool
from ..scene import removal as scene_removal
from ..scene import view_cube as scene_view_cube
from .controls import ZBar
from . import debug_window as dbg


SAFE_Z = 0.0
TICK_MS = 16  # ~60 fps animation cadence
SURFACE_REFRESH_EVERY = 4  # update heightmap visualization every N ticks


class MainWindow(QtWidgets.QMainWindow):
    def __init__(self):
        super().__init__()
        self.setWindowTitle("CNC G-Code Viewer v2 | Nightbots | 416aab")
        self.resize(1280, 820)

        # ── Animation state ────────────────────────────────────────────────
        self.moves = []
        self.move_index = 0
        self.move_t = 0.0
        self.running = False
        self.last_time = None
        self.speed_mult = 1.0
        self.bit_diameter = scene_tool.DEFAULT_BIT_DIA

        # ── Scene actors ───────────────────────────────────────────────────
        self._cut_actor = None
        self._rapid_actor = None
        self._stock_actor = None
        self._stock_surface_actor = None
        self._stock_surface_normals = None  # vtkPolyDataNormals filter (so we can refresh)
        self._tool_actor = None
        self._heightmap = None
        self._tool_prev_pos = None
        self._surface_refresh_ctr = 0
        # Interactive view cube in the top-right corner — built in
        # _build_central, enabled in show_and_initialize once the VTK
        # interactor is up. Hover highlights a face; click snaps the
        # main camera to that face's orthographic view.
        self._view_cube = None
        # Debug window state — lazy-constructed on first open. Saves the
        # original stdio streams so we can restore them on close.
        self._debug_window = None
        self._debug_render_observer_id = None
        self._debug_orig_stdout = None
        self._debug_orig_stderr = None

        # ── UI ─────────────────────────────────────────────────────────────
        self._build_central()
        self._build_toolbar()
        self._build_dock()
        self._build_menu()
        self._build_status_bar()

        # Animation tick timer
        self._timer = QtCore.QTimer(self)
        self._timer.setInterval(TICK_MS)
        self._timer.timeout.connect(self._tick)

    # ─── UI construction ─────────────────────────────────────────────────────

    def _build_central(self):
        central = QtWidgets.QWidget()
        layout = QtWidgets.QVBoxLayout(central)
        layout.setContentsMargins(0, 0, 0, 0)

        self.vtk_widget = QVTKRenderWindowInteractor(central)
        layout.addWidget(self.vtk_widget)

        rw = self.vtk_widget.GetRenderWindow()

        # Two-layer rendering: layer 0 holds the scene (stock + paths + tool),
        # layer 1 is an "always on top" overlay used by the Highlight Path toggle.
        # Both layers share the active camera so orbit/pan/zoom stay in sync.
        # Alpha bit-planes are required so layer 1 composites with transparency
        # over layer 0 instead of overwriting it with a black clear.
        rw.SetNumberOfLayers(2)
        rw.SetAlphaBitPlanes(1)
        rw.SetMultiSamples(0)

        self.renderer = vtk.vtkRenderer()
        self.renderer.SetBackground(0.08, 0.08, 0.10)
        self.renderer.SetLayer(0)
        rw.AddRenderer(self.renderer)

        self.overlay_renderer = vtk.vtkRenderer()
        self.overlay_renderer.SetLayer(1)
        self.overlay_renderer.SetBackground(0, 0, 0)
        self.overlay_renderer.SetBackgroundAlpha(0.0)  # transparent — show layer 0
        self.overlay_renderer.SetActiveCamera(self.renderer.GetActiveCamera())
        self.overlay_renderer.SetInteractive(False)
        rw.AddRenderer(self.overlay_renderer)

        self.interactor = rw.GetInteractor()
        # TrackballCamera = orbit on left-drag, pan on shift+drag, zoom on wheel
        style = vtk.vtkInteractorStyleTrackballCamera()
        self.interactor.SetInteractorStyle(style)

        # ViewCube is built lazily in show_and_initialize — it needs the
        # interactor to be initialized before it can hook mouse events.

        self.setCentralWidget(central)

    def _build_toolbar(self):
        tb = QtWidgets.QToolBar("Main")
        tb.setMovable(False)
        tb.setIconSize(QtCore.QSize(16, 16))
        self.addToolBar(QtCore.Qt.TopToolBarArea, tb)

        open_act = QtWidgets.QAction("Open...", self)
        open_act.setShortcut("Ctrl+O")
        open_act.triggered.connect(self._on_open)
        tb.addAction(open_act)

        tb.addSeparator()

        self.play_act = QtWidgets.QAction("▶ Play", self)
        self.play_act.triggered.connect(self.play)
        tb.addAction(self.play_act)

        self.pause_act = QtWidgets.QAction("⏸ Pause", self)
        self.pause_act.triggered.connect(self.pause)
        tb.addAction(self.pause_act)

        reset_anim_act = QtWidgets.QAction("⏮ Reset", self)
        reset_anim_act.triggered.connect(self.reset_animation)
        tb.addAction(reset_anim_act)

        tb.addSeparator()

        # Speed slider with label
        tb.addWidget(QtWidgets.QLabel("  Speed:  "))
        self.speed_slider = QtWidgets.QSlider(QtCore.Qt.Horizontal)
        self.speed_slider.setMinimumWidth(140)
        self.speed_slider.setMinimum(1)
        self.speed_slider.setMaximum(100)
        self.speed_slider.setValue(10)  # 1.0x
        self.speed_slider.valueChanged.connect(self._on_speed_changed)
        tb.addWidget(self.speed_slider)
        self.speed_label = QtWidgets.QLabel(" 1.0× ")
        tb.addWidget(self.speed_label)

        tb.addSeparator()

        # Bit width entry (commits on Enter / focus-out — same UX as Stage 1 Tk fix)
        tb.addWidget(QtWidgets.QLabel("  Bit dia (mm):  "))
        self.bit_entry = QtWidgets.QLineEdit(f"{scene_tool.DEFAULT_BIT_DIA}")
        self.bit_entry.setMaximumWidth(70)
        self.bit_entry.setAlignment(QtCore.Qt.AlignRight)
        self.bit_entry.editingFinished.connect(self._on_bit_changed)
        tb.addWidget(self.bit_entry)

        tb.addSeparator()

        # Highlight path: render toolpath in the always-on-top overlay layer
        # so it stays visible through the stock surface.
        self.highlight_path_chk = QtWidgets.QCheckBox(" Highlight path ")
        self.highlight_path_chk.stateChanged.connect(self._on_highlight_path_toggled)
        tb.addWidget(self.highlight_path_chk)

        tb.addSeparator()

        # Overlap heatmap toggle (1mm precision matches the Tk prototype)
        self.overlap_chk = QtWidgets.QCheckBox(" Highlight overlaps ")
        self.overlap_chk.stateChanged.connect(self._on_overlap_toggled)
        tb.addWidget(self.overlap_chk)

        tb.addSeparator()

        # Reset camera (frame the scene)
        reset_cam_act = QtWidgets.QAction("Frame", self)
        reset_cam_act.setShortcut("R")
        reset_cam_act.triggered.connect(self._on_reset_camera)
        tb.addAction(reset_cam_act)

    def _build_dock(self):
        dock = QtWidgets.QDockWidget("Z Axis", self)
        dock.setFeatures(QtWidgets.QDockWidget.DockWidgetMovable |
                         QtWidgets.QDockWidget.DockWidgetFloatable)
        self.zbar = ZBar()
        dock.setWidget(self.zbar)
        self.addDockWidget(QtCore.Qt.RightDockWidgetArea, dock)

    def _build_menu(self):
        menu = self.menuBar()
        file_menu = menu.addMenu("&File")

        open_act = QtWidgets.QAction("&Open .nc...", self)
        open_act.setShortcut("Ctrl+O")
        open_act.triggered.connect(self._on_open)
        file_menu.addAction(open_act)

        file_menu.addSeparator()
        quit_act = QtWidgets.QAction("&Quit", self)
        quit_act.setShortcut("Ctrl+Q")
        quit_act.triggered.connect(self.close)
        file_menu.addAction(quit_act)

        view_menu = menu.addMenu("&View")
        reset_act = QtWidgets.QAction("&Reset camera", self)
        reset_act.setShortcut("R")
        reset_act.triggered.connect(self._on_reset_camera)
        view_menu.addAction(reset_act)

        debug_menu = menu.addMenu("&Debug")
        open_dbg_act = QtWidgets.QAction("&Open Debug Window…", self)
        open_dbg_act.setShortcut("Ctrl+D")
        open_dbg_act.triggered.connect(self._on_open_debug_window)
        debug_menu.addAction(open_dbg_act)

    def _build_status_bar(self):
        self.status = self.statusBar()
        self.status.showMessage("No file loaded")

    # ─── Lifecycle ───────────────────────────────────────────────────────────

    def show_and_initialize(self):
        """Must be called after show() — initializes the VTK interactor and
        builds the interactive view cube (which needs an initialized
        interactor before it can hook mouse events)."""
        self.show()
        self.interactor.Initialize()

        # Build the interactive view cube now that the interactor is live
        self._view_cube = scene_view_cube.ViewCube(
            self.vtk_widget.GetRenderWindow(),
            self.renderer,
            self.interactor,
        )
        self._view_cube.enable()

    # ─── Toolbar handlers ────────────────────────────────────────────────────

    def _on_open(self):
        path, _ = QtWidgets.QFileDialog.getOpenFileName(
            self, "Open G-code file", "",
            "G-code (*.nc *.gcode *.tap *.txt);;All files (*.*)",
        )
        if not path:
            return
        try:
            moves = parser.parse_file(path)
        except Exception as e:
            QtWidgets.QMessageBox.critical(self, "Parse error", str(e))
            return
        if not moves:
            QtWidgets.QMessageBox.warning(self, "No moves", "No usable G0/G1/G2/G3 moves found.")
            return
        self.load_moves(moves)
        self.status.showMessage(f"{len(moves)} moves loaded — {path}")

    def _on_speed_changed(self, value):
        # Slider 1..100 maps to 0.1×..10× logarithmically
        # Simplest: linear mapping 1->0.1, 10->1.0, 100->10.0 — three decade slider
        self.speed_mult = value / 10.0
        self.speed_label.setText(f" {self.speed_mult:.1f}× ")

    def _on_bit_changed(self):
        try:
            v = float(self.bit_entry.text())
        except ValueError:
            self.bit_entry.setText(f"{self.bit_diameter}")
            return
        if v <= 0:
            self.bit_entry.setText(f"{self.bit_diameter}")
            return
        self.bit_diameter = v
        if self._tool_actor is not None:
            scene_tool.update_tool_diameter(self._tool_actor, v)
            self.vtk_widget.GetRenderWindow().Render()

    def _on_reset_camera(self):
        self.renderer.ResetCamera()
        self.vtk_widget.GetRenderWindow().Render()

    def _on_open_debug_window(self):
        """Lazy-construct the debug window, populate specs, install stdio tees
        and a VTK render observer so logs and FPS land in the window."""
        if self._debug_window is None:
            from PyQt5.QtCore import QT_VERSION_STR
            self._debug_window = dbg.DebugWindow(self)
            self._debug_window.populate_specs(
                vtk_version=vtk.vtkVersion.GetVTKVersion(),
                qt_version=QT_VERSION_STR,
                gl_info=dbg.gather_gl_info(self.vtk_widget.GetRenderWindow()),
            )
            # Tee Python's stdout/stderr into the debug log. VTK warnings
            # already route to stderr (we replaced the singleton at the top
            # of this module), so they get captured automatically.
            self._debug_orig_stdout, self._debug_orig_stderr = (
                dbg.install_stdio_tee(self._debug_window)
            )
            # Hook VTK's per-render event so the FPS counter has data to chew on
            rw = self.vtk_widget.GetRenderWindow()
            self._debug_render_observer_id = rw.AddObserver(
                "EndEvent", lambda obj, event: self._debug_window.note_render()
            )
            print(f"[debug] window opened — {dbg.platform.python_implementation()}"
                  f" {dbg.sys.version.split()[0]}, VTK {vtk.vtkVersion.GetVTKVersion()}")

        self._debug_window.show()
        self._debug_window.raise_()
        self._debug_window.activateWindow()

    def closeEvent(self, event):
        """Clean up debug-window plumbing so we don't hold the renderer's
        observer or leak the stdio tee streams when the app exits."""
        if self._debug_window is not None:
            if self._debug_render_observer_id is not None:
                rw = self.vtk_widget.GetRenderWindow()
                rw.RemoveObserver(self._debug_render_observer_id)
                self._debug_render_observer_id = None
            if self._debug_orig_stdout is not None:
                dbg.restore_stdio(self._debug_orig_stdout, self._debug_orig_stderr)
                self._debug_orig_stdout = None
                self._debug_orig_stderr = None
            self._debug_window.close()
            self._debug_window = None
        super().closeEvent(event)

    def _on_highlight_path_toggled(self, state):
        """Move the cut + rapid actors between the main renderer (layer 0)
        and the overlay renderer (layer 1) which paints on top of everything.
        """
        if self._cut_actor is None or self._rapid_actor is None:
            return
        on = (state == QtCore.Qt.Checked)
        for actor in (self._cut_actor, self._rapid_actor):
            self.renderer.RemoveActor(actor)
            self.overlay_renderer.RemoveActor(actor)
            (self.overlay_renderer if on else self.renderer).AddActor(actor)
        # Slightly fatter line in highlight mode for visibility against the surface.
        if self._cut_actor is not None:
            self._cut_actor.GetProperty().SetLineWidth(3.5 if on else 2.0)
        if self._rapid_actor is not None:
            self._rapid_actor.GetProperty().SetLineWidth(2.0 if on else 1.0)
        self.vtk_widget.GetRenderWindow().Render()

    def _on_overlap_toggled(self, state):
        """Toggle the overlap heatmap colormap on the stock surface actor."""
        if self._heightmap is None or self._stock_surface_actor is None:
            return
        mapper = self._stock_surface_actor.GetMapper()
        if state == QtCore.Qt.Checked:
            counts = scene_removal.compute_overlap_counts(
                self.moves, self.bit_diameter / 2.0,
                (self._heightmap.x0, self._heightmap.x1),
                (self._heightmap.y0, self._heightmap.y1),
                cell_size=self._heightmap.cell,
            )
            scene_removal.apply_overlap_scalars(self._heightmap._polydata, counts)
            lut = scene_removal.overlap_color_lookup()
            mapper.SetLookupTable(lut)
            mapper.SetScalarRange(0, max(4, int(counts.max())))
            mapper.SetScalarModeToUsePointData()
            mapper.SetColorModeToMapScalars()
            mapper.ScalarVisibilityOn()
            self.status.showMessage(
                f"Overlap heatmap: max count = {int(counts.max())}, "
                f"{int((counts > 1).sum())} cells with multi-pass cuts"
            )
        else:
            mapper.ScalarVisibilityOff()
        self.vtk_widget.GetRenderWindow().Render()

    # ─── Scene management ────────────────────────────────────────────────────

    def load_moves(self, moves):
        """Replace the current scene with new moves."""
        self.moves = moves
        self.move_index = 0
        self.move_t = 0.0
        self.running = False
        self._tool_prev_pos = None
        self._surface_refresh_ctr = 0

        # Wipe previous actors from both renderer layers
        for actor in (self._cut_actor, self._rapid_actor,
                      self._stock_actor, self._stock_surface_actor,
                      self._tool_actor):
            if actor is not None:
                self.renderer.RemoveActor(actor)
                self.overlay_renderer.RemoveActor(actor)

        min_z = parser.deepest_cut_z(moves)
        self.zbar.set_min_cut_z(min_z)

        self._cut_actor, _ = scene_path.make_cut_actor(moves, min_z)
        self._rapid_actor, _ = scene_path.make_rapid_actor(moves)

        b = parser.bounds(moves)
        if b is not None:
            # Translucent outline of the original stock dimensions
            self._stock_actor = scene_stock.make_stock_actor(b, margin=10.0)
            self.renderer.AddActor(self._stock_actor)

            # Heightmap-driven cut surface (gets material removed during animation).
            #
            # Cell size is tied to the bit diameter rather than fixed at 1 mm:
            # the bit physically can't cut features finer than its own width,
            # so a heightmap cell smaller than ~bit_diameter/4 just shows
            # quantization noise from the cut sampler rather than real detail.
            # This keeps the mesh size bounded for big stocks + big bits while
            # preserving 1 mm cells for fine-detail bits ≤ 4 mm dia.
            x_range, y_range, _z_range = scene_stock.stock_dimensions(b, margin=10.0)
            cell_size = max(1.0, self.bit_diameter / 4.0)
            self._heightmap = scene_removal.Heightmap(
                x_range, y_range, top_z=0.0, cell_size=cell_size
            )
            self._stock_surface_actor, self._stock_surface_normals = (
                scene_removal.make_stock_surface_actor(self._heightmap)
            )
            self.renderer.AddActor(self._stock_surface_actor)

        self._tool_actor = scene_tool.make_tool_actor(self.bit_diameter)
        scene_tool.update_tool_position(
            self._tool_actor, moves[0].sx, moves[0].sy, moves[0].sz, moves[0].spindle
        )

        # Path actors go to the overlay layer if Highlight Path is on,
        # otherwise to the main renderer.
        path_renderer = (self.overlay_renderer
                         if self.highlight_path_chk.isChecked()
                         else self.renderer)
        path_renderer.AddActor(self._cut_actor)
        path_renderer.AddActor(self._rapid_actor)
        # Apply the matching line widths
        if self.highlight_path_chk.isChecked():
            self._cut_actor.GetProperty().SetLineWidth(3.5)
            self._rapid_actor.GetProperty().SetLineWidth(2.0)

        self.renderer.AddActor(self._tool_actor)

        # Initial Z-bar position
        self.zbar.set_z(moves[0].sz)

        self.renderer.ResetCamera()
        self.vtk_widget.GetRenderWindow().Render()

    # ─── Animation transport ─────────────────────────────────────────────────

    def play(self):
        if not self.moves:
            return
        if self.move_index >= len(self.moves):
            self.reset_animation()
        self.running = True
        self.last_time = time.perf_counter()
        self._timer.start()

    def pause(self):
        self.running = False
        self._timer.stop()

    def reset_animation(self):
        self.running = False
        self._timer.stop()
        self.move_index = 0
        self.move_t = 0.0
        self.last_time = None
        self._tool_prev_pos = None
        self._surface_refresh_ctr = 0

        # Re-initialize heightmap to top_z everywhere so the workpiece looks
        # uncut again. Cheap — just an array fill.
        if self._heightmap is not None:
            self._heightmap.heights[:, :] = self._heightmap.top_z
            self._heightmap.update_polydata()
            if self._stock_surface_normals is not None:
                self._stock_surface_normals.Update()

        if self.moves:
            m = self.moves[0]
            scene_tool.update_tool_position(
                self._tool_actor, m.sx, m.sy, m.sz, m.spindle
            )
            self.zbar.set_z(m.sz)
            self.vtk_widget.GetRenderWindow().Render()

    # ─── Animation tick ──────────────────────────────────────────────────────

    def _tick(self):
        if not self.running or self.move_index >= len(self.moves):
            self.running = False
            self._timer.stop()
            return

        now = time.perf_counter()
        dt = now - (self.last_time if self.last_time is not None else now)
        self.last_time = now

        move = self.moves[self.move_index]
        if move.duration > 0:
            self.move_t += (dt * self.speed_mult) / move.duration
        else:
            self.move_t = 1.0

        x, y, z = self._point_on_move(move, min(self.move_t, 1.0))

        # Material removal: walk the segment from prev tool pos to current along
        # the cutting move's path, lowering heightmap cells under the bit.
        if (self._heightmap is not None
                and self._tool_prev_pos is not None
                and move.spindle and move.kind != "G0"):
            self._heightmap.cut_segment(
                self._tool_prev_pos, (x, y, z), self.bit_diameter / 2.0
            )

        scene_tool.update_tool_position(self._tool_actor, x, y, z, move.spindle)
        self.zbar.set_z(z)
        self._tool_prev_pos = (x, y, z)

        if self.move_t >= 1.0:
            self.move_index += 1
            self.move_t = 0.0

        # Throttle the heightmap visual refresh — cuts are recorded every tick,
        # but rebuilding the polydata is amortized over several ticks.
        self._surface_refresh_ctr += 1
        if (self._heightmap is not None
                and self._surface_refresh_ctr >= SURFACE_REFRESH_EVERY):
            self._heightmap.update_polydata()
            if self._stock_surface_normals is not None:
                self._stock_surface_normals.Update()
            self._surface_refresh_ctr = 0

        # Update status with current move info
        self.status.showMessage(
            f"Move {min(self.move_index, len(self.moves))}/{len(self.moves)}  "
            f"X:{x:.2f} Y:{y:.2f} Z:{z:.2f}  "
            f"Feed:{move.feed:.0f} mm/min  "
            f"Spindle:{'ON' if move.spindle else 'OFF'}"
        )
        self.vtk_widget.GetRenderWindow().Render()

    @staticmethod
    def _point_on_move(move, t):
        """Interpolate the (x, y, z) position at fraction t along the move."""
        if t <= 0:
            return move.points[0]
        if t >= 1:
            return move.points[-1]
        target = move.length * t
        traveled = 0.0
        for i in range(1, len(move.points)):
            p1 = move.points[i - 1]
            p2 = move.points[i]
            seg = math.dist(p1, p2)
            if traveled + seg >= target:
                local = (target - traveled) / seg if seg else 0
                return (
                    p1[0] + (p2[0] - p1[0]) * local,
                    p1[1] + (p2[1] - p1[1]) * local,
                    p1[2] + (p2[2] - p1[2]) * local,
                )
            traveled += seg
        return move.points[-1]
