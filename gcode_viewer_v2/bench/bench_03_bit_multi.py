"""03 — Multi-part CNC bit (the NEW tool representation).

5 actors in a vtkAssembly: flute + helix lines + transition band + shank + LED.
If FPS here is much lower than bench_02, the multi-part bit is part of the
slowdown and we'd want to consider merging the parts.
"""

if __name__ == "__main__":
    import vtk
    from _common import (
        make_render_window, configure_camera, benchmark, report, format_header,
    )
    from gcode_viewer_v2.scene import tool as scene_tool

    ren = vtk.vtkRenderer()
    ren.SetBackground(0.08, 0.08, 0.10)

    bit = scene_tool.make_tool_actor(3.175)
    ren.AddActor(bit)

    rw = make_render_window()
    rw.AddRenderer(ren)
    configure_camera(ren)

    def on_frame(i):
        x = (i % 50) - 25
        scene_tool.update_tool_position(bit, x, 0, 0, i % 2 == 0)

    format_header()
    result = benchmark(rw, on_frame=on_frame)
    report("03 multi-part bit (5-actor assembly)", result)
