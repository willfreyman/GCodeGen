"""07 — Layered rendering with alpha bit-planes (the highlight-path setup).

Same scene as bench_01 but with rw.SetAlphaBitPlanes(1) +
rw.SetNumberOfLayers(2) + a second empty renderer at layer 1. This is what
MainWindow does to support the "Highlight Path" overlay.

This is the prime suspect for the FPS drop. If FPS here is significantly
lower than bench_01 / bench_06, the alpha-bit-plane framebuffer config is
the culprit and we should switch the highlight-path feature to use polygon
offset instead of layered rendering.
"""

if __name__ == "__main__":
    import vtk
    from _common import (
        make_render_window, configure_camera, benchmark, report, format_header,
    )

    rw = make_render_window(layered=True, alpha=True)

    main_ren = vtk.vtkRenderer()
    main_ren.SetBackground(0.08, 0.08, 0.10)
    main_ren.SetLayer(0)
    rw.AddRenderer(main_ren)

    overlay_ren = vtk.vtkRenderer()
    overlay_ren.SetLayer(1)
    overlay_ren.SetBackgroundAlpha(0.0)
    overlay_ren.SetActiveCamera(main_ren.GetActiveCamera())
    overlay_ren.SetInteractive(False)
    rw.AddRenderer(overlay_ren)

    src = vtk.vtkSphereSource(); src.SetRadius(2.0)
    src.SetPhiResolution(16); src.SetThetaResolution(16)
    mapper = vtk.vtkPolyDataMapper(); mapper.SetInputConnection(src.GetOutputPort())
    actor = vtk.vtkActor(); actor.SetMapper(mapper)
    main_ren.AddActor(actor)

    configure_camera(main_ren)

    format_header()
    result = benchmark(rw)
    report("07 layered + alpha bit-planes (overlay empty)", result)
