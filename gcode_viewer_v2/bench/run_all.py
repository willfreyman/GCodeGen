"""Run every bench_*.py in this directory in sequence and print one line of
results per benchmark. Each bench runs in its own subprocess so VTK state
is fresh — no leftover render-window pollution from prior tests.

Usage (from project root):
    python -m gcode_viewer_v2.bench.run_all

If you'd rather see each window pop up and close one at a time, run any
single bench directly:
    python gcode_viewer_v2/bench/bench_07_layered_alpha.py

What to look for in the output:
  • Compare bench_02 → bench_03: cost of multi-part bit
  • Compare bench_04 → bench_05: cost of per-frame cuts + normals refresh
  • Compare bench_01 → bench_07: cost of layered rendering + alpha bit-planes
  • Compare bench_08 → bench_09: cost of layering ON TOP OF the full scene
    (this one is the most direct read on the highlight-path overlay's cost)
"""

import os
import subprocess
import sys
from pathlib import Path


def main():
    bench_dir = Path(__file__).parent
    benches = sorted(p for p in bench_dir.glob("bench_*.py") if p.name != "_common.py")

    print()
    print("=" * 80)
    print("  gcode_viewer_v2 rendering benchmarks")
    print("  Each bench renders 200 frames in a 1200x800 window after a 10-frame warmup.")
    print("  Higher FPS / lower ms-per-frame = better.")
    print("=" * 80)
    print()
    print(f"{'Benchmark file':<46}{'FPS':>8}{'  ms/frame':>15}")
    print("-" * 80)

    # Force the subprocesses to use UTF-8 for stdout/stderr regardless of
    # the system codepage — otherwise Windows hits a UnicodeEncodeError on
    # any non-ASCII character before our ASCII-only switch landed.
    sub_env = os.environ.copy()
    sub_env["PYTHONIOENCODING"] = "utf-8"

    for b in benches:
        # Run each bench as its own process so each gets a clean VTK state.
        try:
            result = subprocess.run(
                [sys.executable, str(b)],
                capture_output=True,
                text=True,
                encoding="utf-8",
                errors="replace",
                timeout=120,
                cwd=str(bench_dir),
                env=sub_env,
            )
        except subprocess.TimeoutExpired:
            print(f"{b.name:<46}  TIMEOUT (>120s)")
            continue

        # The bench scripts print their own header line and a result line.
        # We want only the result line (skip the header lines we'd duplicate).
        for line in result.stdout.splitlines():
            line = line.strip()
            if not line or line.startswith("Benchmark") or set(line) <= {"-"}:
                continue
            print(line)

        if result.returncode != 0:
            print(f"  -> {b.name} exited with code {result.returncode}")
            if result.stderr.strip():
                # Print full stderr so any future error is fully visible
                for line in result.stderr.splitlines():
                    print(f"    stderr: {line}")

    print()
    print("=" * 80)
    print("  Send the results above so we can pinpoint the exact cause.")
    print("=" * 80)


if __name__ == "__main__":
    main()
