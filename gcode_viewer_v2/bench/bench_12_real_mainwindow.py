"""12 — Drive the *actual* MainWindow class with synthetic data.

Bench 11 builds a stripped-down BenchWindow with the same VTK scene; it
reaches ~48 fps. The real app reaches ~3.8 fps with the same workload.
The gap MUST be inside MainWindow itself — its toolbar widgets, its custom
ZBar paintEvent, its menu, its actor wiring, something the bench replica
doesn't have.

This bench imports MainWindow as-is, calls load_moves with a synthetic
2,000-move toolpath, then calls play() and measures FPS via a render
observer for 5 seconds. If THIS bench drops to ~3-5 fps, the slowdown is
reproducible with synthetic data and we can git-bisect down to the line
that costs us. If it stays at ~40-50 fps, the cost depends on the user's
specific file (move count, arc density, stock size) and we'll need to
reproduce with their real data.
"""

if __name__ == "__main__":
    import sys
    import time

    from PyQt5 import QtCore, QtWidgets

    from _common import make_synthetic_moves
    from gcode_viewer_v2.ui.main_window import MainWindow

    DURATION_SEC = 5.0


    class Harness(QtCore.QObject):
        def __init__(self, win):
            super().__init__()
            self.win = win
            self.frame_count = 0
            self.start_time = None
            self.obs_id = win.vtk_widget.GetRenderWindow().AddObserver(
                "EndEvent", lambda obj, ev: self._on_render()
            )

        def _on_render(self):
            if self.start_time is not None:
                self.frame_count += 1

        def begin(self):
            self.win.play()
            self.start_time = time.perf_counter()
            QtCore.QTimer.singleShot(int(DURATION_SEC * 1000), self.end)

        def end(self):
            elapsed = time.perf_counter() - self.start_time
            fps = self.frame_count / elapsed if elapsed > 0 else 0.0
            ms = (elapsed / self.frame_count) * 1000 if self.frame_count else 0.0
            print(f"{'12 Real MainWindow class + play()':<46}"
                  f"{fps:>8.1f} fps{ms:>9.2f} ms/frame")
            QtWidgets.QApplication.instance().quit()


    app = QtWidgets.QApplication(sys.argv)

    win = MainWindow()
    win.show_and_initialize()

    # Same synthetic toolpath as bench_09 / bench_10 / bench_11
    moves = make_synthetic_moves(n=2000)
    win.load_moves(moves)

    harness = Harness(win)
    # Give the window a beat to settle before starting the timer
    QtCore.QTimer.singleShot(200, harness.begin)

    print(f"{'Benchmark':<46}{'FPS':>8}{'  ms/frame':>15}")
    print("-" * 80)
    app.exec_()
