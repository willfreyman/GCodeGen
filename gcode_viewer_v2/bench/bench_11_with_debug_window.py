"""11 — Same as bench_10 but with the Debug Window open (stdio tee active).

Reproduces the real-app condition: the user is observing FPS via the
debug window, which means the debug window is open when they see 3.8 fps.

If THIS bench drops dramatically vs bench_10, the debug window's stdio
tee + queued appendPlainText is the GUI-thread bottleneck. That'd suggest
something is being written to stdout/stderr per render (a VTK warning
firing every frame, or an internal log message we missed) — the tee
intercepts it, queues a signal, the GUI thread processes it, and the
QPlainTextEdit append/scroll/relayout cost serializes against the renders.

Runs for 5 seconds of timer-driven animation, then prints FPS and exits.
"""

if __name__ == "__main__":
    import sys
    import time

    from PyQt5 import QtCore, QtWidgets
    from vtkmodules.qt.QVTKRenderWindowInteractor import QVTKRenderWindowInteractor
    import vtk

    from _common import (
        configure_camera, format_header, make_synthetic_moves, report,
    )
    from gcode_viewer_v2 import parser
    from gcode_viewer_v2.scene import path as scene_path
    from gcode_viewer_v2.scene import stock as scene_stock
    from gcode_viewer_v2.scene import tool as scene_tool
    from gcode_viewer_v2.scene import removal as scene_removal
    from gcode_viewer_v2.ui import debug_window as dbg

    # Replace VTK's output window with the same stderr-routing instance the
    # real app installs at module import — so any per-render VTK warning
    # gets caught the same way.
    _stderr_output = vtk.vtkOutputWindow()
    _stderr_output.SetDisplayModeToAlwaysStdErr()
    vtk.vtkOutputWindow.SetInstance(_stderr_output)

    DURATION_SEC = 5.0


    class BenchWindow(QtWidgets.QMainWindow):
        def __init__(self):
            super().__init__()
            self.setWindowTitle("bench_11 with Debug window open")
            self.resize(1200, 800)

            self.statusBar().showMessage("starting…")
            self._zbar = QtWidgets.QProgressBar(); self._zbar.setOrientation(QtCore.Qt.Vertical)
            self._zbar.setRange(-3000, 0); self._zbar.setValue(0)
            d = QtWidgets.QDockWidget("Z", self); d.setWidget(self._zbar)
            self.addDockWidget(QtCore.Qt.RightDockWidgetArea, d)

            central = QtWidgets.QWidget(); layout = QtWidgets.QVBoxLayout(central)
            layout.setContentsMargins(0, 0, 0, 0)
            self.vtk_widget = QVTKRenderWindowInteractor(central)
            layout.addWidget(self.vtk_widget)
            self.setCentralWidget(central)

            rw = self.vtk_widget.GetRenderWindow()
            rw.SetNumberOfLayers(2); rw.SetAlphaBitPlanes(1); rw.SetMultiSamples(0)

            self.ren = vtk.vtkRenderer(); self.ren.SetBackground(0.08, 0.08, 0.10)
            self.ren.SetLayer(0); rw.AddRenderer(self.ren)
            overlay = vtk.vtkRenderer(); overlay.SetLayer(1)
            overlay.SetBackgroundAlpha(0.0)
            overlay.SetActiveCamera(self.ren.GetActiveCamera())
            overlay.SetInteractive(False)
            rw.AddRenderer(overlay)

            self.moves = make_synthetic_moves(n=2000)
            min_z = parser.deepest_cut_z(self.moves)
            b = parser.bounds(self.moves)

            cut_actor, _ = scene_path.make_cut_actor(self.moves, min_z)
            rapid_actor, _ = scene_path.make_rapid_actor(self.moves)
            stock_outline = scene_stock.make_stock_actor(b, margin=10)
            x_range, y_range, _z = scene_stock.stock_dimensions(b, margin=10)
            self.hm = scene_removal.Heightmap(x_range, y_range, top_z=0.0, cell_size=1.0)
            surf, self.normals = scene_removal.make_stock_surface_actor(self.hm)
            self.bit = scene_tool.make_tool_actor(3.175)
            for a in (cut_actor, rapid_actor, stock_outline, surf, self.bit):
                self.ren.AddActor(a)
            configure_camera(self.ren)

            cube = vtk.vtkAnnotatedCubeActor()
            cube.SetXPlusFaceText("RIGHT"); cube.SetXMinusFaceText("LEFT")
            cube.SetYPlusFaceText("BACK"); cube.SetYMinusFaceText("FRONT")
            cube.SetZPlusFaceText("TOP"); cube.SetZMinusFaceText("BOTTOM")
            cube.SetFaceTextScale(0.10); cube.SetTextEdgesVisibility(0)
            self.iren = rw.GetInteractor()
            self.cube_widget = vtk.vtkOrientationMarkerWidget()
            self.cube_widget.SetOrientationMarker(cube)
            self.cube_widget.SetInteractor(self.iren)
            self.cube_widget.SetViewport(0.80, 0.78, 1.00, 0.99)

            # ─── KEY DIFFERENCE FROM bench_10 ──────────────────────────────
            # Open the debug window and install the stdio tee + render observer,
            # mirroring what _on_open_debug_window does in MainWindow.
            self.debug_window = dbg.DebugWindow(self)
            self.debug_window.populate_specs(
                vtk_version=vtk.vtkVersion.GetVTKVersion(),
                qt_version=QtCore.QT_VERSION_STR,
                gl_info=dbg.gather_gl_info(rw),
            )
            self._dbg_orig_stdout, self._dbg_orig_stderr = (
                dbg.install_stdio_tee(self.debug_window)
            )
            self._render_observer_id = rw.AddObserver(
                "EndEvent", lambda obj, ev: self.debug_window.note_render()
            )
            self.debug_window.show()
            # ───────────────────────────────────────────────────────────────

            self._frame_count = 0
            self._prev_pos = (0.0, 0.0, 0.0)
            self._refresh_ctr = 0
            self._start = None

            self._timer = QtCore.QTimer(self)
            self._timer.setInterval(16)
            self._timer.timeout.connect(self._tick)

        def show_and_initialize(self):
            self.show()
            self.iren.Initialize()
            self.cube_widget.SetEnabled(1); self.cube_widget.InteractiveOff()

        def start_animation(self):
            self._start = time.perf_counter()
            self._frame_count = 0
            self._timer.start()
            QtCore.QTimer.singleShot(int(DURATION_SEC * 1000), self._stop)

        def _stop(self):
            self._timer.stop()
            elapsed = time.perf_counter() - self._start
            fps = self._frame_count / elapsed if elapsed > 0 else 0.0
            ms = (elapsed / self._frame_count) * 1000 if self._frame_count else 0.0
            # Restore stdio so this print doesn't get tee'd into the dying window
            dbg.restore_stdio(self._dbg_orig_stdout, self._dbg_orig_stderr)
            print(f"{'11 Qt+QVTK+QTimer + Debug Window OPEN':<46}"
                  f"{fps:>8.1f} fps{ms:>9.2f} ms/frame")
            QtWidgets.QApplication.instance().quit()

        def _tick(self):
            i = self._frame_count
            m = self.moves[(i // 10) % len(self.moves)]
            t = (i % 10) / 10.0
            x = m.sx + (m.ex - m.sx) * t
            y = m.sy + (m.ey - m.sy) * t
            z = m.sz + (m.ez - m.sz) * t
            scene_tool.update_tool_position(self.bit, x, y, z, m.spindle)
            self._zbar.setValue(int(z * 1000))
            if m.spindle and m.kind != "G0":
                self.hm.cut_segment(self._prev_pos, (x, y, z), 1.5875)
            self._prev_pos = (x, y, z)
            self._refresh_ctr += 1
            if self._refresh_ctr >= 4:
                self.hm.update_polydata()
                if self.normals is not None:
                    self.normals.Update()
                self._refresh_ctr = 0
            self.statusBar().showMessage(
                f"Move {(i // 10) % len(self.moves)}/{len(self.moves)}  "
                f"X:{x:.2f} Y:{y:.2f} Z:{z:.2f}"
            )
            self.vtk_widget.GetRenderWindow().Render()
            self._frame_count += 1


    app = QtWidgets.QApplication(sys.argv)
    win = BenchWindow()
    win.show_and_initialize()
    win.start_animation()

    format_header()
    app.exec_()
