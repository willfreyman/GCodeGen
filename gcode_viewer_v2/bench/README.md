# gcode_viewer_v2 rendering benchmarks

Nine standalone scripts that isolate one rendering variable each. Run them
on your Windows machine with the same Python environment that runs the app,
then send back the numbers — the FPS deltas tell us exactly which feature
is the bottleneck.

## Run all of them at once

From the project root (`C:\Users\WillFreyman\Desktop\CNC\GCodeGen`):

```cmd
python -m gcode_viewer_v2.bench.run_all
```

This pops up nine 1200×800 windows in sequence (each closes after 200 frames)
and prints a comparison table at the end.

## Run one at a time

```cmd
cd gcode_viewer_v2\bench
python bench_07_layered_alpha.py
```

## What each one measures

| # | File | What it measures |
|---|---|---|
| 01 | `bench_01_baseline.py` | Empty scene + one sphere. Your hardware's floor. |
| 02 | `bench_02_bit_single.py` | One cylinder bit (the **old** representation). |
| 03 | `bench_03_bit_multi.py` | The **new** 5-part CNC bit assembly. |
| 04 | `bench_04_heightmap_static.py` | 10k-vertex stock surface, no updates. |
| 05 | `bench_05_heightmap_animated.py` | Same surface, with per-frame cuts + 15 Hz normals refresh. |
| 06 | `bench_06_orientation_widget.py` | Baseline + the corner cube widget. |
| 07 | `bench_07_layered_alpha.py` | Baseline + the layered renderer & alpha bit-planes (highlight-path setup). |
| 08 | `bench_08_full_no_layers.py` | Full real-app scene **without** layered rendering. |
| 09 | `bench_09_full_with_layers.py` | Full real-app scene **with** layered rendering. |

## What the comparisons mean

| Pair | Tells us |
|---|---|
| **02 vs 03** | Cost of the multi-part bit assembly (vs. one cylinder). |
| **04 vs 05** | Cost of per-frame heightmap cuts + normals recomputation during animation. |
| **01 vs 07** | Cost of the layered-rendering / alpha-bit-planes setup in isolation. |
| **08 vs 09** | The real, on-actual-scene cost of the highlight-path overlay system. |

If `09` is much slower than `08`, the layered rendering is the dominant cost
and we should rewrite the highlight-path feature using polygon offset.
If `05` is much slower than `04`, the per-cut heightmap update is the issue.
If `03` is much slower than `02`, the multi-part bit needs simplifying.

The comparisons are independent — you can identify multiple causes from one run.
