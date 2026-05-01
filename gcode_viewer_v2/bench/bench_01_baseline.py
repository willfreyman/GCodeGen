"""01 — BASELINE: empty scene with one trivial actor.

Establishes the GPU/driver floor. Your hardware can't render faster than this.
Every subsequent bench adds something — drops vs this one tell you the cost.
"""

if __name__ == "__main__":
    import vtk
    from _common import (
        make_render_window, configure_camera, benchmark, report, format_header,
    )

    ren = vtk.vtkRenderer()
    ren.SetBackground(0.08, 0.08, 0.10)

    # A small sphere so the scene isn't empty (some drivers short-circuit
    # rendering of empty scenes which would skew the comparison).
    src = vtk.vtkSphereSource()
    src.SetRadius(2.0)
    src.SetPhiResolution(16)
    src.SetThetaResolution(16)
    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputConnection(src.GetOutputPort())
    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    ren.AddActor(actor)

    rw = make_render_window()
    rw.AddRenderer(ren)
    configure_camera(ren)

    format_header()
    result = benchmark(rw)
    report("01 baseline (one sphere, single layer)", result)
