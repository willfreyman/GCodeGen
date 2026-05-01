"""05 — Heightmap with per-frame cutting + every-4-frames normals refresh.

Replicates what _tick does during a real cut: numpy heightmap.cut_segment
each frame, then heightmap.update_polydata + normals.Update every 4 frames.
This is where the per-frame cost during animation actually lives.

If bench_05 is much slower than bench_04, the normals filter and/or the
polydata re-upload is the culprit. If they're similar, the cutting itself
is cheap and rendering dominates.
"""

if __name__ == "__main__":
    import math, random
    import vtk
    from _common import (
        make_render_window, configure_camera, benchmark, report, format_header,
    )
    from gcode_viewer_v2.scene import removal as scene_removal

    ren = vtk.vtkRenderer()
    ren.SetBackground(0.08, 0.08, 0.10)

    hm = scene_removal.Heightmap((-50, 50), (-50, 50), top_z=0.0, cell_size=1.0)
    surf, normals = scene_removal.make_stock_surface_actor(hm)
    ren.AddActor(surf)

    rw = make_render_window()
    rw.AddRenderer(ren)
    configure_camera(ren)

    rng = random.Random(0)

    def on_frame(i):
        # cut a small line every frame, like _tick does
        x0 = rng.uniform(-40, 40); y0 = rng.uniform(-40, 40); z = -1.0
        x1 = x0 + rng.uniform(-2, 2); y1 = y0 + rng.uniform(-2, 2)
        hm.cut_segment((x0, y0, z), (x1, y1, z), 1.5875)
        # every 4 frames, push the heightmap to GPU + recompute normals
        if i % 4 == 0:
            hm.update_polydata()
            if normals is not None: normals.Update()

    format_header()
    result = benchmark(rw, on_frame=on_frame)
    report("05 heightmap animated (cuts + normals 15Hz)", result)
