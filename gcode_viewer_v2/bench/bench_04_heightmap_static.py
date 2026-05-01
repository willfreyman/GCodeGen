"""04 — Static heightmap surface (uncut workpiece, no per-frame updates).

A 100x100 mm stock surface at 1 mm cell size = 10,201 vertices, 20,000 tris,
plus the vtkPolyDataNormals filter. The mesh is built once and rendered
without modification — this measures the steady-state cost of the heightmap
visualization, separate from the per-cut update cost (bench_05).
"""

if __name__ == "__main__":
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

    format_header()
    result = benchmark(rw)
    report("04 heightmap static (10k verts, no updates)", result)
