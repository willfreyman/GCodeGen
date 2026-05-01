"""Shared utilities for the rendering benchmarks.

Each bench script builds a specific scene, warms up the GPU, then measures
FPS over a fixed number of frames. The harness keeps the window size, camera,
and frame count identical across tests so results are comparable.

Goal: isolate which part of the v2 viewer is bottlenecking your hardware.
Run each bench, compare the FPS — large drops between adjacent benches point
at the feature added in the second one.
"""

import math
import random
import sys
import time
from pathlib import Path


# ── Standard test config ─────────────────────────────────────────────────────
WINDOW_W = 1200
WINDOW_H = 800
WARMUP_FRAMES = 10
MEASURE_FRAMES = 200


# Make 'from gcode_viewer_v2 import …' work whether you run a bench as a
# module (`python -m gcode_viewer_v2.bench.bench_xx`) or as a script
# (`python gcode_viewer_v2/bench/bench_xx.py`).
_PROJECT_ROOT = Path(__file__).resolve().parents[2]
if str(_PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(_PROJECT_ROOT))


def configure_camera(ren):
    """Standard 3D camera so all benches frame the scene the same way."""
    cam = ren.GetActiveCamera()
    cam.SetPosition(80, -80, 60)
    cam.SetFocalPoint(0, 0, 0)
    cam.SetViewUp(0, 0, 1)
    ren.ResetCamera()
    cam.Zoom(1.2)
    ren.ResetCameraClippingRange()


def make_render_window(layered=False, alpha=False):
    """Build a vtkRenderWindow with the standard size. Optional layered/alpha
    setup matches what main_window.py does for the highlight-path overlay."""
    import vtk
    rw = vtk.vtkRenderWindow()
    rw.SetSize(WINDOW_W, WINDOW_H)
    if layered:
        rw.SetNumberOfLayers(2)
    if alpha:
        rw.SetAlphaBitPlanes(1)
        rw.SetMultiSamples(0)
    return rw


def make_synthetic_moves(n=2000, seed=0):
    """Generate a credible CNC toolpath: short cutting segments at random
    z-depths between -2.5 and -0.1mm, scattered across a 100x100 area."""
    from gcode_viewer_v2 import parser
    rng = random.Random(seed)
    moves = []
    for _ in range(n):
        x0 = rng.uniform(-50, 50)
        y0 = rng.uniform(-50, 50)
        ang = rng.uniform(0, math.tau)
        x1 = x0 + math.cos(ang) * 5
        y1 = y0 + math.sin(ang) * 5
        z = rng.uniform(-2.5, -0.1)
        moves.append(parser.Move("G1", x0, y0, z, x1, y1, z, 500, True))
    return moves


def benchmark(rw, on_frame=None, frames=MEASURE_FRAMES, warmup=WARMUP_FRAMES):
    """Render `warmup` frames (discarded), then `frames` frames timed.

    `on_frame(i)` is invoked before each timed render — use it to mutate the
    scene (move the bit, update the heightmap, etc.) so we measure both the
    work done during animation AND the render itself, like the real app.
    """
    # Force the GL context to initialize before timing
    rw.Render()
    for _ in range(warmup):
        if on_frame:
            on_frame(-1)
        rw.Render()

    t0 = time.perf_counter()
    for i in range(frames):
        if on_frame:
            on_frame(i)
        rw.Render()
    elapsed = time.perf_counter() - t0

    return {
        "frames": frames,
        "elapsed_s": elapsed,
        "fps": frames / elapsed if elapsed > 0 else 0.0,
        "ms_per_frame": (elapsed / frames) * 1000 if frames > 0 else 0.0,
    }


def report(name, result, baseline_fps=None):
    """Print one line of result. If baseline_fps given, append the % change."""
    delta = ""
    if baseline_fps and baseline_fps > 0:
        ratio = (result["fps"] / baseline_fps - 1) * 100
        delta = f"  ({ratio:+.0f}% vs baseline)"
    print(
        f"{name:<46}"
        f"{result['fps']:>8.1f} fps"
        f"{result['ms_per_frame']:>9.2f} ms/frame"
        f"{delta}"
    )


def format_header():
    # ASCII-only divider — Windows subprocesses default to cp1252 stdout
    # encoding which can't encode Unicode box-drawing chars like U+2500.
    print(f"{'Benchmark':<46}{'FPS':>8}{'  ms/frame':>15}")
    print("-" * 80)
