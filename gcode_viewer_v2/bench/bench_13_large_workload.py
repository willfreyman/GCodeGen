"""13 — Large workload through real MainWindow.

Same setup as bench_12 (real MainWindow class + play()), but with a larger
synthetic toolpath that includes G2/G3 arcs. Closer to a realistic file size.

Defaults: 10,000 moves (40% arcs), 200x200 mm stock at 1 mm cell.
   ~ 60,000 cut-actor vertices, ~40,000 heightmap vertices.

Tweak the constants below to match your typical file. If THIS bench
reproduces ~3-5 fps, the data scale is the cause and we know exactly
what to optimize (heightmap resolution, normals throttling, etc.).
"""

# ──────────────────────── tune these to match your file ────────────────────
N_MOVES = 10_000           # try 5_000, 20_000, 50_000 to bracket your file
ARC_FRACTION = 0.40        # 0.0 = all straights, 1.0 = all arcs
STOCK_WIDTH_MM = 200.0     # match your actual stock XY dimension
HEIGHTMAP_CELL_MM = 1.0    # the app uses 1 mm — keeps overlap precision
DURATION_SEC = 5.0
# ───────────────────────────────────────────────────────────────────────────


if __name__ == "__main__":
    import math
    import random
    import sys
    import time

    from PyQt5 import QtCore, QtWidgets

    # add project root to path before importing gcode_viewer_v2
    from _common import format_header  # noqa: F401  (triggers sys.path setup)
    from gcode_viewer_v2 import parser
    from gcode_viewer_v2.ui.main_window import MainWindow


    def make_realistic_moves(n, arc_frac, stock_w):
        """Generate a mix of straight cutting moves and G2/G3 arcs distributed
        over a stock-sized region. Arcs use the existing parser.arc_points
        linearizer so the move-point counts match what real .nc files produce.
        """
        rng = random.Random(0)
        half = stock_w / 2 - 5  # keep moves inside the stock with a margin
        moves = []
        for _ in range(n):
            if rng.random() < arc_frac:
                cx = rng.uniform(-half, half); cy = rng.uniform(-half, half)
                r = rng.uniform(2, 8)
                a1 = rng.uniform(0, math.tau)
                a2 = a1 + rng.uniform(0.5, 2.5) * (1 if rng.random() > 0.5 else -1)
                sx = cx + r * math.cos(a1); sy = cy + r * math.sin(a1)
                ex = cx + r * math.cos(a2); ey = cy + r * math.sin(a2)
                z = rng.uniform(-2.5, -0.1)
                pts = parser.arc_points(sx, sy, z, ex, ey, z,
                                        cx - sx, cy - sy, clockwise=(a2 < a1))
                m = parser.Move("G2", sx, sy, z, ex, ey, z, 500, True, points=pts)
            else:
                x0 = rng.uniform(-half, half); y0 = rng.uniform(-half, half)
                ang = rng.uniform(0, math.tau)
                length = rng.uniform(2, 8)
                x1 = x0 + math.cos(ang) * length; y1 = y0 + math.sin(ang) * length
                z = rng.uniform(-2.5, -0.1)
                m = parser.Move("G1", x0, y0, z, x1, y1, z, 500, True)
            moves.append(m)
        return moves


    class Harness(QtCore.QObject):
        def __init__(self, win):
            super().__init__()
            self.win = win
            self.frame_count = 0
            self.start_time = None
            win.vtk_widget.GetRenderWindow().AddObserver(
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

            # Print enough context to diagnose the workload
            total_pts = sum(len(m.points) for m in self.win.moves)
            hm = self.win._heightmap
            hm_verts = hm.heights.size if hm is not None else 0
            print(
                f"{'13 large workload (real MainWindow)':<46}"
                f"{fps:>8.1f} fps{ms:>9.2f} ms/frame"
            )
            print(
                f"     ↳ workload: {len(self.win.moves):,} moves "
                f"({total_pts:,} cut-actor pts), "
                f"heightmap {hm_verts:,} verts "
                f"({hm.nx}x{hm.ny} @ {hm.cell:.1f} mm)" if hm else ""
            )
            QtWidgets.QApplication.instance().quit()


    moves = make_realistic_moves(N_MOVES, ARC_FRACTION, STOCK_WIDTH_MM)
    print(f"  generated {len(moves):,} moves "
          f"({sum(len(m.points) for m in moves):,} total points)")

    app = QtWidgets.QApplication(sys.argv)
    win = MainWindow()
    win.show_and_initialize()
    win.load_moves(moves)

    harness = Harness(win)
    QtCore.QTimer.singleShot(500, harness.begin)
    print(f"{'Benchmark':<46}{'FPS':>8}{'  ms/frame':>15}")
    print("-" * 80)
    app.exec_()
