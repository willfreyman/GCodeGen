"""06 — Same scene as bench_01 plus the labeled view-cube widget.

The orientation widget adds a third internal renderer (its own viewport in
the corner). If FPS drops noticeably vs bench_01 the widget is non-trivial
on your hardware.
"""

if __name__ == "__main__":
    import vtk
    from _common import (
        make_render_window, configure_camera, benchmark, report, format_header,
    )

    ren = vtk.vtkRenderer()
    ren.SetBackground(0.08, 0.08, 0.10)
    src = vtk.vtkSphereSource(); src.SetRadius(2.0)
    src.SetPhiResolution(16); src.SetThetaResolution(16)
    mapper = vtk.vtkPolyDataMapper(); mapper.SetInputConnection(src.GetOutputPort())
    actor = vtk.vtkActor(); actor.SetMapper(mapper)
    ren.AddActor(actor)

    rw = make_render_window()
    rw.AddRenderer(ren)
    configure_camera(ren)

    # Add the orientation cube widget — same setup as MainWindow
    cube = vtk.vtkAnnotatedCubeActor()
    cube.SetXPlusFaceText("RIGHT"); cube.SetXMinusFaceText("LEFT")
    cube.SetYPlusFaceText("BACK"); cube.SetYMinusFaceText("FRONT")
    cube.SetZPlusFaceText("TOP"); cube.SetZMinusFaceText("BOTTOM")
    cube.SetFaceTextScale(0.10)
    cube.GetCubeProperty().SetColor(0.32, 0.36, 0.42)
    cube.SetTextEdgesVisibility(0)

    iren = vtk.vtkRenderWindowInteractor()
    iren.SetRenderWindow(rw)
    iren.Initialize()

    widget = vtk.vtkOrientationMarkerWidget()
    widget.SetOrientationMarker(cube)
    widget.SetInteractor(iren)
    widget.SetViewport(0.80, 0.78, 1.00, 0.99)
    widget.SetEnabled(1)
    widget.InteractiveOff()

    format_header()
    result = benchmark(rw)
    report("06 + orientation cube widget", result)
