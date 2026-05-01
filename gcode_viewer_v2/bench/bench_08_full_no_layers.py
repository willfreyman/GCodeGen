"""08 — FULL real-app scene MINUS layered rendering.

Everything the real app does — toolpath actors, stock surface, multi-part
bit, orientation widget, animation — but with single-layer rendering (no
alpha bit-planes, no overlay renderer).

Compare to bench_09 (which adds layered rendering on top of the same
scene). The delta is the cost of the highlight-path overlay system.
"""

if __name__ == "__main__":
    import vtk
    from _common import (
        make_render_window, configure_camera, benchmark, report, format_header,
        make_synthetic_moves,
    )
    from gcode_viewer_v2 import parser
    from gcode_viewer_v2.scene import path as scene_path
    from gcode_viewer_v2.scene import stock as scene_stock
    from gcode_viewer_v2.scene import tool as scene_tool
    from gcode_viewer_v2.scene import removal as scene_removal

    moves = make_synthetic_moves(n=2000)
    min_z = parser.deepest_cut_z(moves)
    b = parser.bounds(moves)

    ren = vtk.vtkRenderer()
    ren.SetBackground(0.08, 0.08, 0.10)

    cut_actor, _ = scene_path.make_cut_actor(moves, min_z)
    rapid_actor, _ = scene_path.make_rapid_actor(moves)
    stock_outline = scene_stock.make_stock_actor(b, margin=10)
    x_range, y_range, _z_range = scene_stock.stock_dimensions(b, margin=10)
    hm = scene_removal.Heightmap(x_range, y_range, top_z=0.0, cell_size=1.0)
    surf, normals = scene_removal.make_stock_surface_actor(hm)
    bit = scene_tool.make_tool_actor(3.175)

    for a in (cut_actor, rapid_actor, stock_outline, surf, bit):
        ren.AddActor(a)

    rw = make_render_window()  # NO layers, NO alpha
    rw.AddRenderer(ren)
    configure_camera(ren)

    # Orientation widget
    cube = vtk.vtkAnnotatedCubeActor()
    cube.SetXPlusFaceText("RIGHT"); cube.SetXMinusFaceText("LEFT")
    cube.SetYPlusFaceText("BACK"); cube.SetYMinusFaceText("FRONT")
    cube.SetZPlusFaceText("TOP"); cube.SetZMinusFaceText("BOTTOM")
    cube.SetFaceTextScale(0.10)
    cube.SetTextEdgesVisibility(0)
    iren = vtk.vtkRenderWindowInteractor(); iren.SetRenderWindow(rw); iren.Initialize()
    widget = vtk.vtkOrientationMarkerWidget()
    widget.SetOrientationMarker(cube); widget.SetInteractor(iren)
    widget.SetViewport(0.80, 0.78, 1.00, 0.99)
    widget.SetEnabled(1); widget.InteractiveOff()

    # Per-frame: animate the bit + cut the heightmap (real animation pattern)
    import random
    rng = random.Random(0)
    prev_pos = [0.0, 0.0, 0.0]

    def on_frame(i):
        # Walk the bit along synthesized moves; sample one per ~10 frames so
        # the bit visibly traverses the scene during the bench.
        m = moves[(i // 10) % len(moves)]
        t = (i % 10) / 10.0
        x = m.sx + (m.ex - m.sx) * t
        y = m.sy + (m.ey - m.sy) * t
        z = m.sz + (m.ez - m.sz) * t
        scene_tool.update_tool_position(bit, x, y, z, m.spindle)
        if m.spindle and m.kind != "G0":
            hm.cut_segment((prev_pos[0], prev_pos[1], prev_pos[2]),
                           (x, y, z), 1.5875)
        prev_pos[0], prev_pos[1], prev_pos[2] = x, y, z
        if i % 4 == 0:
            hm.update_polydata()
            if normals is not None: normals.Update()

    format_header()
    result = benchmark(rw, on_frame=on_frame)
    report("08 full scene, NO layered rendering", result)
