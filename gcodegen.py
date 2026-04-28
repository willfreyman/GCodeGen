import tkinter as tk
from tkinter import filedialog, messagebox, simpledialog
import math

CANVAS_W = 700
CANVAS_H = 580

SAFE_Z      = 5.0
FEED_XY     = 300
FEED_Z      = 100
SPINDLE_RPM = 12000

# ── State ──────────────────────────────────────────────────────────────────────
strokes            = []   # [{points, name, depth, color}]
current_stroke_pts = []
drawing_active     = False
hover_stroke_idx   = None
stroke_rows        = []
preview_render_fn  = None
preview_win        = None

perim  = {"x0": 100, "y0": 480, "x1": 580, "y1": 80}
origin = {"x": 100,  "y": 480}

drag_state = {"what": None, "sx": 0, "sy": 0, "orig": {}}

COLORS    = ["#e63946","#2a9d8f","#e9c46a","#264653","#f4a261","#a8dadc","#457b9d"]
color_idx = [0]

# ── Coord helpers ──────────────────────────────────────────────────────────────

def _perim_wh():
    return perim["x1"]-perim["x0"], perim["y0"]-perim["y1"]  # py inverted

def px_to_mm(cx, cy):
    pw, ph = _perim_wh()
    if pw == 0 or ph == 0: return 0.0, 0.0
    wm = get_float(perim_w_entry, 50)
    hm = get_float(perim_h_entry, 50)
    return round((cx - origin["x"]) * (wm/pw), 3), \
           round((origin["y"] - cy) * (hm/ph), 3)

def mm_to_px(xm, ym):
    pw, ph = _perim_wh()
    if pw == 0 or ph == 0: return origin["x"], origin["y"]
    wm = get_float(perim_w_entry, 50)
    hm = get_float(perim_h_entry, 50)
    return int(origin["x"] + xm*(pw/wm)), int(origin["y"] - ym*(ph/hm))

def get_float(widget, default):
    try:    return float(widget.get())
    except: return default

# ── Redraw ─────────────────────────────────────────────────────────────────────

def redraw_all():
    canvas.delete("all")
    _grid()
    _draw_perim()
    _draw_origin()
    for i, s in enumerate(strokes):
        if i == hover_stroke_idx:
            _stroke_on(canvas, s, width=4)
        else:
            _stroke_on(canvas, s)

def _grid():
    for x in range(0, CANVAS_W, 20):
        canvas.create_line(x, 0, x, CANVAS_H, fill="#f0f0f0")
    for y in range(0, CANVAS_H, 20):
        canvas.create_line(0, y, CANVAS_W, y, fill="#f0f0f0")

def _draw_perim():
    x0,y0,x1,y1 = perim["x0"],perim["y0"],perim["x1"],perim["y1"]
    dash = (6,4) if not cut_perim_var.get() else ()
    canvas.create_rectangle(x0,y1,x1,y0, outline="#888", width=2, dash=dash)
    hw = 8
    for hx,hy in [(x0,y0),(x1,y0),(x0,y1),(x1,y1)]:
        canvas.create_rectangle(hx-hw,hy-hw,hx+hw,hy+hw,
                                fill="#4a90d9", outline="white", width=1)

def _draw_origin():
    ox,oy = origin["x"],origin["y"]
    r = 10
    canvas.create_oval(ox-r,oy-r,ox+r,oy+r, fill="#ff5500", outline="white", width=2)
    canvas.create_text(ox+14, oy-14, text="0,0", fill="#ff5500",
                       font=("Helvetica",9,"bold"))

def _stroke_on(c, s, width=2):
    pts = s["points"]
    for i in range(len(pts)-1):
        c.create_line(pts[i][0],pts[i][1],pts[i+1][0],pts[i+1][1],
                      fill=s["color"], width=width)

# ── Hit detection ──────────────────────────────────────────────────────────────
HR = 12

def _hit_origin(x, y):
    return math.hypot(x-origin["x"], y-origin["y"]) < HR

def _hit_handle(x, y):
    for tag,(hx,hy) in {
        "bl":(perim["x0"],perim["y0"]), "br":(perim["x1"],perim["y0"]),
        "tl":(perim["x0"],perim["y1"]), "tr":(perim["x1"],perim["y1"]),
    }.items():
        if math.hypot(x-hx,y-hy) < HR: return tag
    return None

def _hit_edge(x, y):
    x0,y0,x1,y1 = perim["x0"],perim["y0"],perim["x1"],perim["y1"]
    xmin,xmax = min(x0,x1),max(x0,x1); ymin,ymax = min(y0,y1),max(y0,y1)
    t = 8
    return ((xmin<=x<=xmax and (abs(y-y0)<t or abs(y-y1)<t)) or
            (ymin<=y<=ymax and (abs(x-x0)<t or abs(x-x1)<t)))

def _pt_seg_dist(px, py, x0, y0, x1, y1):
    dx, dy = x1 - x0, y1 - y0
    if dx == 0 and dy == 0:
        return math.hypot(px - x0, py - y0)
    t = ((px - x0) * dx + (py - y0) * dy) / float(dx * dx + dy * dy)
    t = max(0.0, min(1.0, t))
    qx, qy = x0 + t * dx, y0 + t * dy
    return math.hypot(px - qx, py - qy)

def _nearest_stroke_idx(x, y, threshold=7):
    best_i = None
    best_d = threshold
    for i, s in enumerate(strokes):
        pts = s["points"]
        for j in range(len(pts)-1):
            d = _pt_seg_dist(x, y, pts[j][0], pts[j][1], pts[j+1][0], pts[j+1][1])
            if d <= best_d:
                best_i, best_d = i, d
    return best_i

def _set_hover_stroke(idx):
    global hover_stroke_idx
    if hover_stroke_idx == idx:
        return
    hover_stroke_idx = idx
    redraw_all()
    _refresh_list()

def on_motion(e):
    if drawing_active or drag_state["what"] in ("draw", "origin", "perim", "bl", "br", "tl", "tr"):
        return
    _set_hover_stroke(_nearest_stroke_idx(e.x, e.y))

def on_leave(_e):
    _set_hover_stroke(None)

# ── Canvas events ──────────────────────────────────────────────────────────────

def on_press(e):
    global drawing_active
    x,y = e.x,e.y
    if _hit_origin(x,y):
        drag_state.update(what="origin",sx=x,sy=y,orig=dict(origin)); return
    h = _hit_handle(x,y)
    if h:
        drag_state.update(what=h,sx=x,sy=y,orig=dict(perim)); return
    if _hit_edge(x,y):
        drag_state.update(what="perim",sx=x,sy=y,orig=dict(perim)); return
    drawing_active = True
    drag_state["what"] = "draw"
    current_stroke_pts.clear()
    current_stroke_pts.append((x,y))

def on_drag(e):
    x,y = e.x,e.y
    w = drag_state["what"]
    if w == "origin":
        dx,dy = x-drag_state["sx"],y-drag_state["sy"]
        origin["x"] = drag_state["orig"]["x"]+dx
        origin["y"] = drag_state["orig"]["y"]+dy
        redraw_all(); _update_preview(); return
    if w in ("bl","br","tl","tr"):
        dx,dy = x-drag_state["sx"],y-drag_state["sy"]
        o = drag_state["orig"]
        perim["x0"] = o["x0"]+dx if "l" in w else o["x0"]
        perim["x1"] = o["x1"]+dx if "r" in w else o["x1"]
        perim["y0"] = o["y0"]+dy if "b" in w else o["y0"]
        perim["y1"] = o["y1"]+dy if "t" in w else o["y1"]
        redraw_all(); _update_preview(); return
    if w == "perim":
        dx,dy = x-drag_state["sx"],y-drag_state["sy"]
        o = drag_state["orig"]
        for k in ("x0","x1"): perim[k] = o[k]+dx
        for k in ("y0","y1"): perim[k] = o[k]+dy
        redraw_all(); _update_preview(); return
    if w == "draw" and drawing_active and current_stroke_pts:
        ox,oy = current_stroke_pts[-1]
        canvas.create_line(ox,oy,x,y, fill=_cur_color(), width=2, tags="preview")
        current_stroke_pts.append((x,y))

def on_release(e):
    global drawing_active
    if drag_state["what"]=="draw" and drawing_active and len(current_stroke_pts)>1:
        _finalize()
    drawing_active = False
    drag_state["what"] = None

def _finalize():
    s = {"points": list(current_stroke_pts),
         "name":   op_name_entry.get().strip() or f"Cut {len(strokes)+1}",
         "depth":  get_float(depth_entry,-1.0),
         "color":  _cur_color()}
    strokes.append(s)
    color_idx[0] = (color_idx[0]+1) % len(COLORS)
    current_stroke_pts.clear()
    canvas.delete("preview")
    _stroke_on(canvas, s)
    _refresh_list()
    _update_preview()

def _cur_color(): return COLORS[color_idx[0] % len(COLORS)]

# ── Snap origin ────────────────────────────────────────────────────────────────

def snap_origin():
    origin["x"] = min(perim["x0"],perim["x1"])
    origin["y"] = max(perim["y0"],perim["y1"])
    redraw_all()
    _update_preview()

# ── Op list ────────────────────────────────────────────────────────────────────

def _refresh_list():
    stroke_rows.clear()
    for w in stroke_list_frame.winfo_children(): w.destroy()
    for i,s in enumerate(strokes):
        is_hover = (i == hover_stroke_idx)
        row_bg = "#e7f1ff" if is_hover else "#fff"
        row = tk.Frame(stroke_list_frame, bg=row_bg)
        row.pack(fill="x", pady=1)
        stroke_rows.append(row)
        swatch = tk.Label(row, bg=s["color"], width=2)
        swatch.pack(side="left", padx=(2,4))
        txt = tk.Label(row, text=f"{s['name']}  z={s['depth']}mm",
                 anchor="w", bg=row_bg, font=("Helvetica",9))
        txt.pack(side="left",fill="x",expand=True)
        tk.Button(row, text="✕", command=lambda i=i: _del(i),
                  font=("Helvetica",8), relief="flat", bg=row_bg,
                  fg="#c00", cursor="hand2").pack(side="right")
        row.bind("<Button-1>", lambda _e, i=i: _edit_depth(i))
        swatch.bind("<Button-1>", lambda _e, i=i: _edit_depth(i))
        txt.bind("<Button-1>", lambda _e, i=i: _edit_depth(i))
    _scroll_hover_row_into_view()

def _edit_depth(i):
    if i < 0 or i >= len(strokes):
        return
    cur = strokes[i]["depth"]
    val = simpledialog.askfloat("Edit depth",
                                f"Set depth for '{strokes[i]['name']}' (mm):",
                                initialvalue=cur,
                                parent=window)
    if val is None:
        return
    strokes[i]["depth"] = round(val, 3)
    _refresh_list()
    _update_preview()

def _scroll_hover_row_into_view():
    if hover_stroke_idx is None:
        return
    if hover_stroke_idx < 0 or hover_stroke_idx >= len(stroke_rows):
        return
    window.update_idletasks()
    row = stroke_rows[hover_stroke_idx]
    top = row.winfo_y()
    bottom = top + row.winfo_height()
    viewport = stroke_list_canvas.winfo_height()
    total = max(1, stroke_list_frame.winfo_height())
    y0, y1 = stroke_list_canvas.yview()
    cur_top = y0 * total
    cur_bottom = y1 * total
    if top < cur_top:
        stroke_list_canvas.yview_moveto(top / total)
    elif bottom > cur_bottom:
        stroke_list_canvas.yview_moveto(max(0.0, (bottom - viewport) / total))

def _del(i):
    strokes.pop(i); redraw_all(); _refresh_list()
    _update_preview()

def clear_all():
    strokes.clear(); redraw_all(); _refresh_list()
    _update_preview()

# ── Build ops ──────────────────────────────────────────────────────────────────

def _ops_mm():
    ops = []
    if cut_perim_var.get():
        w = get_float(perim_w_entry,50); h = get_float(perim_h_entry,50)
        ops.append({"name":"Perimeter",
                    "pts":[(0,0),(w,0),(w,h),(0,h),(0,0)],
                    "depth":get_float(perim_depth_entry,-1.0)})
    for s in strokes:
        ops.append({"name":s["name"],
                    "pts":[px_to_mm(px,py) for px,py in s["points"]],
                    "depth":s["depth"]})
    return ops

def _ops_px():
    ops = []
    if cut_perim_var.get():
        lx,rx = min(perim["x0"],perim["x1"]),max(perim["x0"],perim["x1"])
        by_,ty = max(perim["y0"],perim["y1"]),min(perim["y0"],perim["y1"])
        ops.append({"name":"Perimeter","color":"#888888",
                    "pts":[(lx,by_),(rx,by_),(rx,ty),(lx,ty),(lx,by_)],
                    "depth":get_float(perim_depth_entry,-1.0)})
    for s in strokes:
        ops.append({"name":s["name"],"color":s["color"],
                    "pts":s["points"],"depth":s["depth"]})
    return ops

def _all_strokes_px():
    result = []
    if cut_perim_var.get():
        lx,rx = min(perim["x0"],perim["x1"]),max(perim["x0"],perim["x1"])
        by_,ty = max(perim["y0"],perim["y1"]),min(perim["y0"],perim["y1"])
        result.append({"points":[(lx,by_),(rx,by_),(rx,ty),(lx,ty),(lx,by_)],
                       "color":"#555555","depth":-1.0,"name":"Perimeter"})
    result.extend(strokes)
    return result

def _set_entry(entry, value):
    entry.delete(0, tk.END)
    entry.insert(0, str(value))

# Source: Bantam Tools material guides (Acrylic/Aluminum/Brass/Machinable Foam),
# using conservative starter values; wood/MDF/stone are inferred fallback presets.
MAT_MACHINE_PRESETS = {
    "Wood":      {"feed_xy": 900, "feed_z": 60, "rpm": 12000},
    "MDF":       {"feed_xy": 800, "feed_z": 50, "rpm": 12000},
    "Aluminium": {"feed_xy": 180, "feed_z": 15, "rpm": 12000},
    "Acrylic":   {"feed_xy": 600, "feed_z": 40, "rpm": 12000},
    "Foam":      {"feed_xy": 800, "feed_z": 40, "rpm": 12000},
    "Brass":     {"feed_xy": 200, "feed_z": 17, "rpm": 12000},
    "Stone":     {"feed_xy": 120, "feed_z": 10, "rpm": 10000},
}

def _apply_material_preset(material):
    p = MAT_MACHINE_PRESETS.get(material)
    if not p:
        return
    _set_entry(feed_xy_entry, p["feed_xy"])
    _set_entry(feed_z_entry, p["feed_z"])
    _set_entry(rpm_entry, p["rpm"])

def _update_preview():
    if preview_render_fn:
        preview_render_fn()

# ── G-code export ──────────────────────────────────────────────────────────────

def generate_gcode():
    ops = _ops_mm()
    if not ops:
        messagebox.showwarning("Nothing","Add strokes or enable perimeter cutting."); return
    path = filedialog.asksaveasfilename(defaultextension=".nc",
           filetypes=[("G-code","*.nc"),("Text","*.txt"),("All","*.*")])
    if not path: return
    safe_z = get_float(safe_z_entry,SAFE_Z)
    fxy    = get_float(feed_xy_entry,FEED_XY)
    fz     = get_float(feed_z_entry,FEED_Z)
    rpm    = get_float(rpm_entry,SPINDLE_RPM)
    L = ["; G-code — Draw-to-Gcode",""]
    for op in ops: L.append(f";  {op['name']}  z={op['depth']}mm")
    L += ["","G21","G90","G17",f"G0 Z{safe_z:.3f}",f"M3 S{int(rpm)}",""]
    for op in ops:
        pts = op["pts"]
        if len(pts)<2: continue
        L += [f"; --- {op['name']} ---",
              f"G0 Z{safe_z:.3f}",
              f"G0 X{pts[0][0]:.3f} Y{pts[0][1]:.3f}",
              f"G1 Z{op['depth']:.3f} F{int(fz)}"]
        for x,y in pts[1:]: L.append(f"G1 X{x:.3f} Y{y:.3f} F{int(fxy)}")
        L += [f"G0 Z{safe_z:.3f}",""]
    L += ["M5","M30",""]
    with open(path,"w") as f: f.write("\n".join(L))
    messagebox.showinfo("Saved",f"Saved to:\n{path}")

# ══════════════════════════════════════════════════════════════════════════════
#  TOOLPATH SIMULATION
# ══════════════════════════════════════════════════════════════════════════════

def open_simulation():
    ops = _ops_px()
    if not ops:
        messagebox.showwarning("Nothing","Draw something first."); return

    win = tk.Toplevel(window)
    win.title("Toolpath Simulation")
    win.configure(bg="#0d0d1a")
    win.resizable(False, False)

    SW, SH = 740, 580

    # header
    hdr = tk.Frame(win, bg="#0d0d1a"); hdr.pack(fill="x", padx=12, pady=(10,4))
    tk.Label(hdr, text="TOOLPATH SIMULATION", bg="#0d0d1a", fg="#00d4ff",
             font=("Courier",12,"bold")).pack(side="left")

    speed_var = tk.IntVar(value=6)
    tk.Label(hdr, text="Speed:", bg="#0d0d1a", fg="#888",
             font=("Helvetica",9)).pack(side="right", padx=(0,4))
    tk.Scale(hdr, from_=1, to=30, orient="horizontal", variable=speed_var,
             bg="#0d0d1a", fg="#00d4ff", troughcolor="#222",
             highlightthickness=0, length=140).pack(side="right")

    # sim canvas
    sc = tk.Canvas(win, width=SW, height=SH, bg="#08081a", highlightthickness=0)
    sc.pack(padx=12)

    # faint grid
    for x in range(0,SW,20): sc.create_line(x,0,x,SH,fill="#13132a")
    for y in range(0,SH,20): sc.create_line(0,y,SW,y,fill="#13132a")

    # ghost paths
    for op in ops:
        pts = op["pts"]
        for i in range(len(pts)-1):
            sc.create_line(pts[i][0],pts[i][1],pts[i+1][0],pts[i+1][1],
                           fill="#1e1e44", width=1)

    # perimeter outline
    lx,rx = min(perim["x0"],perim["x1"]),max(perim["x0"],perim["x1"])
    by_,ty = max(perim["y0"],perim["y1"]),min(perim["y0"],perim["y1"])
    sc.create_rectangle(lx,ty,rx,by_, outline="#2a2a55", width=1, dash=(4,4))

    # status / controls
    status = tk.Label(win, text="Press  ▶ Play  to start",
                      bg="#0d0d1a", fg="#556", font=("Courier",9))
    status.pack(pady=(5,2))

    progress_var = tk.DoubleVar(value=0)
    prog_bar = tk.Scale(win, variable=progress_var, from_=0, to=100,
                        orient="horizontal", length=SW-24,
                        bg="#0d0d1a", fg="#00d4ff", troughcolor="#1a1a3a",
                        highlightthickness=0, showvalue=False, state="disabled")
    prog_bar.pack(padx=12)

    bf = tk.Frame(win, bg="#0d0d1a"); bf.pack(pady=(4,10))

    # --- simulation engine ---
    total_pts = sum(len(op["pts"]) for op in ops)

    st = {"running":False, "after":None, "oi":0, "pi":0, "done":False,
          "head":None, "pts_done":0}

    def _head(x, y, cutting=True):
        if st["head"]: sc.delete(st["head"])
        col = "#00d4ff" if cutting else "#ff8800"
        r = 7
        st["head"] = sc.create_oval(x-r,y-r,x+r,y+r,
                                    fill=col, outline="#ffffff", width=1)
        # crosshair
        sc.create_line(x-12,y,x+12,y, fill=col, width=1, tags="head_cross")
        sc.create_line(x,y-12,x,y+12, fill=col, width=1, tags="head_cross")

    def _step():
        if not st["running"]: return
        # delete old crosshairs (redrawn each frame)
        sc.delete("head_cross")

        n = max(1, speed_var.get())
        for _ in range(n):
            oi,pi = st["oi"],st["pi"]
            if oi >= len(ops):
                st["running"]=False; st["done"]=True
                status.config(text="✓  Complete — all paths machined", fg="#00ff88")
                play_btn.config(text="↺  Restart"); return

            op  = ops[oi]
            pts = op["pts"]
            status.config(
                text=f"Op {oi+1}/{len(ops)}: {op['name']}   "
                     f"pt {pi}/{len(pts)-1}   z={op['depth']}mm",
                fg="#00d4ff")
            pct = (st["pts_done"] / max(1, total_pts)) * 100
            progress_var.set(pct)

            if pi == 0:
                # rapid travel
                prev = ops[oi-1]["pts"][-1] if oi>0 else (origin["x"],origin["y"])
                sx,sy = pts[0]
                sc.create_line(prev[0],prev[1],sx,sy,
                               fill="#ff8800", width=1, dash=(2,6))
                _head(sx,sy, cutting=False)
                st["pi"] = 1
            elif pi < len(pts):
                x0,y0 = pts[pi-1]; x1,y1 = pts[pi]
                sc.create_line(x0,y0,x1,y1, fill=op["color"], width=2)
                _head(x1,y1, cutting=True)
                st["pi"]+=1; st["pts_done"]+=1
            else:
                st["oi"]+=1; st["pi"]=0

        st["after"] = win.after(16, _step)

    def toggle():
        if st["done"]: _reset(); return
        if st["running"]:
            st["running"]=False
            if st["after"]: win.after_cancel(st["after"])
            play_btn.config(text="▶  Resume")
        else:
            st["running"]=True
            play_btn.config(text="⏸  Pause")
            _step()

    def _reset():
        if st["after"]: win.after_cancel(st["after"])
        sc.delete("trail"); sc.delete("rapid"); sc.delete("head_cross")
        if st["head"]: sc.delete(st["head"]); st["head"]=None
        # redraw ghost
        for op in ops:
            pts = op["pts"]
            for i in range(len(pts)-1):
                sc.create_line(pts[i][0],pts[i][1],pts[i+1][0],pts[i+1][1],
                               fill="#1e1e44", width=1)
        st.update(running=False,oi=0,pi=0,done=False,pts_done=0)
        progress_var.set(0)
        status.config(text="Press  ▶ Play  to start", fg="#556")
        play_btn.config(text="▶  Play")

    play_btn = tk.Button(bf, text="▶  Play", command=toggle,
                         font=("Helvetica",10,"bold"), bg="#00d4ff", fg="#000",
                         relief="flat", padx=14, pady=4, cursor="hand2")
    play_btn.pack(side="left", padx=6)

    tk.Button(bf, text="↺  Reset", command=_reset,
              font=("Helvetica",10), bg="#333", fg="#fff",
              relief="flat", padx=10, pady=4, cursor="hand2").pack(side="left",padx=4)

    win.protocol("WM_DELETE_WINDOW", lambda: [
        st["after"] and win.after_cancel(st["after"]), win.destroy()])

# ══════════════════════════════════════════════════════════════════════════════
#  FINISHED PRODUCT PREVIEW
# ══════════════════════════════════════════════════════════════════════════════

def open_preview():
    global preview_render_fn, preview_win
    if preview_win and preview_win.winfo_exists():
        preview_win.deiconify()
        preview_win.lift()
        return

    win = tk.Toplevel(window)
    preview_win = win
    win.title("Finished Product Preview")
    win.configure(bg="#222")
    win.resizable(False, False)

    PW, PH = 580, 520

    hdr = tk.Frame(win, bg="#222"); hdr.pack(fill="x", padx=10, pady=(10,2))
    tk.Label(hdr, text="FINISHED PRODUCT PREVIEW", bg="#222", fg="#f0c040",
             font=("Courier",12,"bold")).pack(side="left")

    # controls row
    ctrl = tk.Frame(win, bg="#222"); ctrl.pack(fill="x", padx=10, pady=(0,4))

    def _lbl(text):
        tk.Label(ctrl, text=text, bg="#222", fg="#aaa",
                 font=("Helvetica",9)).pack(side="left", padx=(8,2))

    _lbl("Bit diameter (mm):")
    bit_var = tk.DoubleVar(value=1.0)
    bit_scale = tk.Scale(ctrl, from_=0.1, to=12.0, resolution=0.1,
                         orient="horizontal", variable=bit_var, length=180,
                         bg="#222", fg="#f0c040", troughcolor="#444",
                         highlightthickness=0, showvalue=True)
    bit_scale.pack(side="left")

    _lbl("Material:")
    mat_var = tk.StringVar(value="Wood")
    MATS = ["Wood","MDF","Aluminium","Acrylic","Foam","Brass","Stone"]
    mat_menu = tk.OptionMenu(ctrl, mat_var, *MATS)
    mat_menu.config(bg="#444", fg="#fff", relief="flat",
                    activebackground="#555", font=("Helvetica",9), width=9)
    mat_menu.pack(side="left")

    # colour palette per material: (surface, cut groove, shadow edge)
    MAT_PAL = {
        "Wood":      ("#c8a06a","#5c3010","#9a7040"),
        "MDF":       ("#c0a882","#6b4a2a","#9a7a52"),
        "Aluminium": ("#b8c0cc","#50606e","#8898a8"),
        "Acrylic":   ("#cce8ff","#1a55aa","#7ab0e0"),
        "Foam":      ("#e8e8e8","#909090","#c0c0c0"),
        "Brass":     ("#d4a840","#7a5800","#a07820"),
        "Stone":     ("#a0a098","#505048","#787870"),
    }

    pc = tk.Canvas(win, width=PW, height=PH, bg="#111", highlightthickness=0)
    pc.pack(padx=10, pady=(0,4))

    info = tk.Label(win, text="", bg="#222", fg="#888", font=("Courier",8))
    info.pack(pady=(0,8))

    def render(*_):
        pc.delete("all")
        all_s = _all_strokes_px()
        if not all_s:
            pc.create_rectangle(0,0,PW,PH, fill="#2a2a2a", outline="")
            pc.create_text(PW//2, PH//2, text="Draw something to preview",
                           fill="#aaa", font=("Helvetica",11,"bold"))
            info.config(text="No operations yet.", fg="#888")
            return

        mat = mat_var.get()
        surf, groove, shadow = MAT_PAL.get(mat, ("#c8a06a","#5c3010","#9a7040"))

        # fill surface
        pc.create_rectangle(0,0,PW,PH, fill=surf, outline="")

        # texture lines for organic materials
        import random
        rng = random.Random(99)
        if mat in ("Wood","MDF"):
            for _ in range(40):
                yg = rng.randint(0,PH)
                dy = rng.randint(-12,12)
                shade = surf if rng.random()>0.35 else shadow
                pc.create_line(0,yg,PW,yg+dy, fill=shade, width=1)
        elif mat == "Stone":
            for _ in range(60):
                x1s=rng.randint(0,PW); y1s=rng.randint(0,PH)
                x2s=x1s+rng.randint(-60,60); y2s=y1s+rng.randint(-40,40)
                pc.create_line(x1s,y1s,x2s,y2s, fill=shadow, width=1)
        elif mat == "Brass":
            for _ in range(25):
                yg = rng.randint(0,PH)
                pc.create_line(0,yg,PW,yg, fill=shadow, width=1)

        # bit radius in pixels
        pw_mm = get_float(perim_w_entry,50)
        pw_px = abs(perim["x1"]-perim["x0"])
        if pw_mm <= 0 or pw_px <= 0:
            info.config(text="Set perimeter size first"); return
        pxmm   = pw_px / pw_mm
        bit_mm = bit_var.get()
        br     = max(1.0, (bit_mm/2) * pxmm)

        # draw strokes as continuous grooves (avoid visible circle footprint edges)
        groove_w = max(2, int(round(br * 2)))
        for s in all_s:
            pts = s["points"]
            for i in range(len(pts)-1):
                x0,y0 = pts[i]; x1,y1 = pts[i+1]
                pc.create_line(x0,y0,x1,y1, fill=shadow, width=groove_w+1,
                               capstyle=tk.ROUND, joinstyle=tk.ROUND)
                pc.create_line(x0,y0,x1,y1, fill=groove, width=groove_w,
                               capstyle=tk.ROUND, joinstyle=tk.ROUND)

        # perimeter boundary overlay
        lx = min(perim["x0"],perim["x1"]); rx = max(perim["x0"],perim["x1"])
        ty = min(perim["y0"],perim["y1"]); by_ = max(perim["y0"],perim["y1"])
        pc.create_rectangle(lx,ty,rx,by_, outline="#ffffff", width=1, dash=(4,4))

        # quality indicator
        area_mm2 = get_float(perim_w_entry,50)*get_float(perim_h_entry,50)
        bit_area = math.pi*(bit_mm/2)**2
        ratio    = bit_area/area_mm2 if area_mm2>0 else 0
        if ratio < 0.002:   quality,qcol = "Excellent detail","#00ff88"
        elif ratio < 0.008: quality,qcol = "Good detail",     "#88ff00"
        elif ratio < 0.02:  quality,qcol = "Moderate detail", "#f0c040"
        elif ratio < 0.06:  quality,qcol = "Low detail",      "#ff8800"
        else:               quality,qcol = "Very coarse",     "#ff3300"

        pc.create_text(PW-8,PH-8, anchor="se",
                       text=f"Bit {bit_mm:.1f}mm  ▸  {quality}",
                       fill=qcol, font=("Courier",9,"bold"))

        info.config(
            text=f"Bit ⌀ {bit_mm:.1f}mm  |  Material: {mat}  |  "
                 f"Scale: {pxmm:.1f}px/mm  |  "
                 f"Min feature ≈ {bit_mm:.1f}mm",
            fg="#888")

    def _material_changed(*_):
        _apply_material_preset(mat_var.get())
        render()

    bit_scale.config(command=lambda _: render())
    mat_var.trace_add("write", _material_changed)
    preview_render_fn = render
    render()
    _apply_material_preset(mat_var.get())
    render()
    win.protocol("WM_DELETE_WINDOW", lambda: None)

# ══════════════════════════════════════════════════════════════════════════════
#  GUI LAYOUT
# ══════════════════════════════════════════════════════════════════════════════

window = tk.Tk()
window.title("Draw → G-code  |  CNC Path Editor")
window.configure(bg="#f0f0f0")
window.resizable(False, False)

main = tk.Frame(window, bg="#f0f0f0")
main.pack(fill="both", expand=True, padx=8, pady=8)

# ── Drawing canvas ─────────────────────────────────────────────────────────────
cf = tk.Frame(main, bg="#bbb", bd=1, relief="sunken")
cf.grid(row=0, column=0, rowspan=2, padx=(0,8))

canvas = tk.Canvas(cf, width=CANVAS_W, height=CANVAS_H, bg="white",
                   cursor="crosshair", highlightthickness=0)
canvas.pack()
canvas.bind("<ButtonPress-1>",   on_press)
canvas.bind("<B1-Motion>",       on_drag)
canvas.bind("<ButtonRelease-1>", on_release)
canvas.bind("<Motion>",          on_motion)
canvas.bind("<Leave>",           on_leave)

# ── Panel ──────────────────────────────────────────────────────────────────────
panel = tk.Frame(main, bg="#f0f0f0", width=252)
panel.grid(row=0, column=1, sticky="n")
panel.grid_propagate(False)

def _sec(title):
    f = tk.LabelFrame(panel, text=title, bg="#f0f0f0",
                      font=("Helvetica",9,"bold"), fg="#333", padx=6, pady=4)
    f.pack(fill="x", pady=(0,5))
    return f

def _row(parent, label, default, width=7):
    r = tk.Frame(parent, bg="#f0f0f0"); r.pack(fill="x", pady=1)
    tk.Label(r, text=label, bg="#f0f0f0", font=("Helvetica",9),
             width=14, anchor="w").pack(side="left")
    e = tk.Entry(r, width=width, font=("Helvetica",9))
    e.insert(0, str(default)); e.pack(side="left")
    return e

def _btn(parent, text, cmd, bg="#4a90d9", fg="white", pady=3):
    return tk.Button(parent, text=text, command=cmd,
                     font=("Helvetica",9,"bold"), bg=bg, fg=fg,
                     relief="flat", padx=6, pady=pady,
                     cursor="hand2")

# Perimeter
ps = _sec("Perimeter")
perim_w_entry     = _row(ps, "Width (mm)",    50)
perim_h_entry     = _row(ps, "Height (mm)",   50)
perim_depth_entry = _row(ps, "Cut depth (mm)", -1.0)
cut_perim_var = tk.BooleanVar(value=False)
tk.Checkbutton(ps, text="Cut perimeter", variable=cut_perim_var,
               bg="#f0f0f0", font=("Helvetica",9),
               command=redraw_all).pack(anchor="w", pady=2)
_btn(ps, "Snap 0,0 → perimeter lower-left", snap_origin).pack(fill="x", pady=(3,0))

# New op
os_ = _sec("New Operation")
op_name_entry = _row(os_, "Name",       "Cut 1", width=14)
depth_entry   = _row(os_, "Depth (mm)", -1.0)
tk.Label(os_, text="Draw on canvas with left mouse button",
         bg="#f0f0f0", fg="#777", font=("Helvetica",8),
         wraplength=210, justify="left").pack(anchor="w", pady=(3,0))

# Op list
ls = _sec("Operations")
stroke_list_outer = tk.Frame(ls, bg="#fff", bd=1, relief="sunken", height=110)
stroke_list_outer.pack(fill="both", expand=True)
stroke_list_outer.pack_propagate(False)
stroke_list_canvas = tk.Canvas(stroke_list_outer, bg="#fff", highlightthickness=0)
stroke_list_scroll = tk.Scrollbar(stroke_list_outer, orient="vertical",
                                  command=stroke_list_canvas.yview)
stroke_list_canvas.configure(yscrollcommand=stroke_list_scroll.set)
stroke_list_canvas.pack(side="left", fill="both", expand=True)
stroke_list_scroll.pack(side="right", fill="y")
stroke_list_frame = tk.Frame(stroke_list_canvas, bg="#fff")
stroke_list_canvas_window = stroke_list_canvas.create_window(
    (0, 0), window=stroke_list_frame, anchor="nw"
)

def _on_stroke_list_frame_cfg(_e):
    stroke_list_canvas.configure(scrollregion=stroke_list_canvas.bbox("all"))

def _on_stroke_list_canvas_cfg(e):
    stroke_list_canvas.itemconfig(stroke_list_canvas_window, width=e.width)

stroke_list_frame.bind("<Configure>", _on_stroke_list_frame_cfg)
stroke_list_canvas.bind("<Configure>", _on_stroke_list_canvas_cfg)
_btn(ls, "Clear All", clear_all, bg="#c0392b").pack(anchor="e", pady=(4,0))

# Machine
ms = _sec("Machine Settings")
safe_z_entry  = _row(ms, "Safe Z (mm)",       SAFE_Z)
feed_xy_entry = _row(ms, "Feed XY (mm/min)",  FEED_XY)
feed_z_entry  = _row(ms, "Feed Z (mm/min)",   FEED_Z)
rpm_entry     = _row(ms, "Spindle RPM",        SPINDLE_RPM)

# Actions
ab = tk.Frame(panel, bg="#f0f0f0"); ab.pack(fill="x", pady=(4,0))
_btn(ab, "▶  Simulate Toolpath",        open_simulation, bg="#1565c0", pady=5).pack(fill="x", pady=(0,3))
_btn(ab, "👁  Finished Product Preview", open_preview,    bg="#6a1b9a", pady=5).pack(fill="x", pady=(0,3))
_btn(ab, "⬇  Generate G-code",          generate_gcode,  bg="#2e7d32", pady=5).pack(fill="x")

# Status bar
tk.Label(window,
         text="Drag orange dot = move 0,0  │  Drag corners/edges = resize perimeter  │  Draw strokes on canvas",
         bg="#333", fg="#999", font=("Helvetica",8), anchor="w", padx=8
         ).pack(fill="x", side="bottom")

redraw_all()
_refresh_list()
window.after(120, open_preview)
window.mainloop()