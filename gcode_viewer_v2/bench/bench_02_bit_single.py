"""02 — Single-cylinder bit (the OLD tool representation).

Compare to bench_03 (multi-part bit) to measure the cost of the new 5-actor
CNC bit assembly. Same transform-update-per-frame as the real animation tick.
"""

if __name__ == "__main__":
    import vtk
    from _common import (
        make_render_window, configure_camera, benchmark, report, format_header,
    )

    ren = vtk.vtkRenderer()
    ren.SetBackground(0.08, 0.08, 0.10)

    cyl = vtk.vtkCylinderSource()
    cyl.SetRadius(1.5875)
    cyl.SetHeight(30.0)
    cyl.SetResolution(32)
    cyl.SetCenter(0.0, 15.0, 0.0)
    rotate = vtk.vtkTransform()
    rotate.RotateX(90)
    tf = vtk.vtkTransformPolyDataFilter()
    tf.SetInputConnection(cyl.GetOutputPort())
    tf.SetTransform(rotate)
    tf.Update()
    mapper = vtk.vtkPolyDataMapper()
    mapper.SetInputConnection(tf.GetOutputPort())
    actor = vtk.vtkActor()
    actor.SetMapper(mapper)
    actor.GetProperty().SetColor(0.95, 0.55, 0.10)
    ren.AddActor(actor)

    rw = make_render_window()
    rw.AddRenderer(ren)
    configure_camera(ren)

    # Mimic per-tick transform update: move the bit through space each frame
    def on_frame(i):
        x = (i % 50) - 25
        t = vtk.vtkTransform()
        t.Translate(x, 0, 0)
        actor.SetUserTransform(t)

    format_header()
    result = benchmark(rw, on_frame=on_frame)
    report("02 single-cylinder bit (old tool)", result)
