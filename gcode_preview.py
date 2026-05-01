import tkinter as tk
from tkinter import filedialog, messagebox
import math
import re
import time

SAFE_Z = 0.0
DEFAULT_FEED = 500.0
RAPID_FEED = 2500.0


class Move:
    def __init__(self, kind, sx, sy, sz, ex, ey, ez, feed, spindle, points=None):
        self.kind = kind
        self.sx = sx
        self.sy = sy
        self.sz = sz
        self.ex = ex
        self.ey = ey
        self.ez = ez
        self.feed = feed if feed > 0 else DEFAULT_FEED
        self.spindle = spindle
        self.points = points or [(sx, sy, sz), (ex, ey, ez)]

        self.length = 0
        for i in range(1, len(self.points)):
            self.length += math.dist(self.points[i - 1], self.points[i])

        used_feed = RAPID_FEED if kind == "G0" else self.feed
        self.duration = self.length / (used_feed / 60.0) if used_feed > 0 else 0.01


class Viewer:
    def __init__(self, root):
        self.root = root
        self.root.title("CNC G-Code Viewer | Nightbots | 416aab")

        self.moves = []
        self.move_index = 0
        self.move_t = 0.0
        self.running = False
        self.last_time = None

        self.cut_chains = []
        self.current_chain = []

        self._view_dirty = True

        self.show_overlaps = tk.BooleanVar(value=False)
        self._overlap_grid_cache = None

        self.min_cut_z = -1.0

        self.bit_width = tk.DoubleVar(value=3.175)
        self._last_bit_width = 3.175
        self.speed_mult = tk.DoubleVar(value=1.0)

        self.zoom = 1
        self.pan_x = 550
        self.pan_y = 420
        self.rot_x = 55
        self.rot_z = -35
        self.last_mouse = None

        self._cos_rz = 1.0
        self._sin_rz = 0.0
        self._cos_rx = 1.0
        self._sin_rx = 0.0
        self._update_proj_cache()

        self._last_live_render = 0.0

        self._redraw_pending = False

        # Persistent dynamic item IDs (created on first draw, updated each frame)
        self._tool_line_id = None
        self._tool_oval_id = None
        self._tool_text_id = None
        self._hud_text_id = None
        self._zbar_ids = None  # dict of named item IDs once created

        self.build_ui()

    def build_ui(self):
        top = tk.Frame(self.root)
        top.pack(fill="x")

        tk.Button(top, text="Upload / Select .nc", command=self.load_file).pack(side="left", padx=5, pady=5)

        tk.Label(top, text="Bit width mm:").pack(side="left")
        bit_entry = tk.Entry(top, textvariable=self.bit_width, width=8)
        bit_entry.pack(side="left", padx=5)
        bit_entry.bind("<Return>", self._on_bit_width_change)
        bit_entry.bind("<FocusOut>", self._on_bit_width_change)

        tk.Label(top, text="Speed:").pack(side="left")
        tk.Scale(
            top,
            variable=self.speed_mult,
            from_=0.1,
            to=10,
            resolution=0.1,
            orient="horizontal",
            length=160
        ).pack(side="left")

        tk.Button(top, text="Play", command=self.play).pack(side="left", padx=5)
        tk.Button(top, text="Pause", command=self.pause).pack(side="left", padx=5)
        tk.Button(top, text="Reset Instructions", command=self.reset_instructions).pack(side="left", padx=5)
        tk.Button(top, text="Reset View", command=self.reset_view).pack(side="left", padx=5)

        tk.Checkbutton(
            top,
            text="Highlight overlaps",
            variable=self.show_overlaps,
            command=self._on_show_overlaps_change,
        ).pack(side="left", padx=5)

        self.info = tk.Label(top, text="No file loaded")
        self.info.pack(side="left", padx=15)

        paste_frame = tk.Frame(self.root)
        paste_frame.pack(fill="x", padx=5, pady=3)

        tk.Label(paste_frame, text="Paste G-code:").pack(side="left", padx=(0, 5))
        tk.Button(paste_frame, text="Load Pasted", command=self.load_pasted).pack(side="right", padx=5)
        tk.Button(paste_frame, text="Clear", command=self.clear_pasted).pack(side="right", padx=5)

        self.paste_text = tk.Text(paste_frame, height=3, wrap="none", bg="#1f1f1f", fg="#dddddd", insertbackground="white")
        self.paste_text.pack(side="left", fill="x", expand=True)

        self.canvas = tk.Canvas(self.root, bg="#151515", width=1100, height=760)
        self.canvas.pack(fill="both", expand=True)

        self.canvas.bind("<ButtonPress-1>", self.mouse_down)
        self.canvas.bind("<B1-Motion>", self.pan)

        self.canvas.bind("<ButtonPress-3>", self.mouse_down)
        self.canvas.bind("<B3-Motion>", self.rotate)

        self.canvas.bind("<MouseWheel>", self.zoom_mouse)
        self.canvas.bind("<Configure>", self._on_configure)

    def _bit_width(self):
        try:
            v = float(self.bit_width.get())
        except (tk.TclError, ValueError):
            return self._last_bit_width
        if v <= 0:
            return self._last_bit_width
        self._last_bit_width = v
        return v

    def _on_bit_width_change(self, *_):
        try:
            v = float(self.bit_width.get())
        except (tk.TclError, ValueError):
            return
        if v <= 0:
            return
        self._last_bit_width = v
        self._overlap_grid_cache = None
        self._view_dirty = True
        self.draw()

    def _on_configure(self, _event):
        self._view_dirty = True
        self._request_redraw()

    def _request_redraw(self):
        if self._redraw_pending:
            return
        self._redraw_pending = True
        self.root.after_idle(self._do_pending_redraw)

    def _do_pending_redraw(self):
        self._redraw_pending = False
        self.draw()

    def _on_show_overlaps_change(self):
        self._view_dirty = True
        self.draw()

    def _overlap_color(self, count):
        if count >= 4:
            return "#ff00ff"
        if count == 3:
            return "#ff60d0"
        return "#ff90c0"

    def _compute_overlap_grid(self):
        if self._overlap_grid_cache is not None:
            return self._overlap_grid_cache

        cell = 1.0
        bit_r = self._bit_width() / 2

        if bit_r < 0.1 or not self.moves:
            self._overlap_grid_cache = ({}, cell)
            return self._overlap_grid_cache

        grid = {}
        bit_r_sq = bit_r * bit_r

        for move in self.moves:
            if not (move.spindle and move.kind != "G0"):
                continue

            for i in range(1, len(move.points)):
                p1 = move.points[i - 1]
                p2 = move.points[i]
                z1 = p1[2]
                z2 = p2[2]

                if z1 >= 0 and z2 >= 0:
                    continue

                if z1 < 0 and z2 < 0:
                    x1, y1, x2, y2 = p1[0], p1[1], p2[0], p2[1]
                else:
                    t = -z1 / (z2 - z1)
                    cx = p1[0] + t * (p2[0] - p1[0])
                    cy = p1[1] + t * (p2[1] - p1[1])
                    if z1 < 0:
                        x1, y1, x2, y2 = p1[0], p1[1], cx, cy
                    else:
                        x1, y1, x2, y2 = cx, cy, p2[0], p2[1]

                min_x = min(x1, x2) - bit_r
                max_x = max(x1, x2) + bit_r
                min_y = min(y1, y2) - bit_r
                max_y = max(y1, y2) + bit_r

                ix0 = int(math.floor(min_x / cell))
                ix1 = int(math.ceil(max_x / cell))
                iy0 = int(math.floor(min_y / cell))
                iy1 = int(math.ceil(max_y / cell))

                dx = x2 - x1
                dy = y2 - y1
                seg_len_sq = dx * dx + dy * dy

                for ix in range(ix0, ix1 + 1):
                    for iy in range(iy0, iy1 + 1):
                        qx = ix * cell + cell / 2
                        qy = iy * cell + cell / 2

                        if seg_len_sq < 1e-6:
                            ddx = qx - x1
                            ddy = qy - y1
                            dist_sq = ddx * ddx + ddy * ddy
                        else:
                            tt = ((qx - x1) * dx + (qy - y1) * dy) / seg_len_sq
                            if tt < 0:
                                tt = 0
                            elif tt > 1:
                                tt = 1
                            px = x1 + tt * dx
                            py = y1 + tt * dy
                            ddx = qx - px
                            ddy = qy - py
                            dist_sq = ddx * ddx + ddy * ddy

                        if dist_sq <= bit_r_sq:
                            grid[(ix, iy)] = grid.get((ix, iy), 0) + 1

        self._overlap_grid_cache = (grid, cell)
        return self._overlap_grid_cache

    def _render_overlaps(self, tag):
        grid, cell = self._compute_overlap_grid()

        for (ix, iy), count in grid.items():
            if count <= 1:
                continue

            x0 = ix * cell
            y0 = iy * cell
            x1 = x0 + cell
            y1 = y0 + cell

            p1 = self.project(x0, y0, 0)
            p2 = self.project(x1, y0, 0)
            p3 = self.project(x1, y1, 0)
            p4 = self.project(x0, y1, 0)

            color = self._overlap_color(count)

            self.canvas.create_polygon(
                p1[0], p1[1],
                p2[0], p2[1],
                p3[0], p3[1],
                p4[0], p4[1],
                fill=color,
                outline="",
                tags=tag,
            )

    def clean(self, line):
        line = re.sub(r"\(.*?\)", "", line)
        line = line.split(";")[0]
        return line.strip().upper()

    def val(self, line, letter):
        m = re.search(letter + r"(-?\d+\.?\d*)", line)
        return float(m.group(1)) if m else None

    def load_file(self):
        path = filedialog.askopenfilename(
            filetypes=[("G-code", "*.nc *.gcode *.tap *.txt"), ("All files", "*.*")]
        )

        if not path:
            return

        try:
            with open(path, "r", errors="ignore") as f:
                text = f.read()
        except Exception as e:
            messagebox.showerror("Error", str(e))
            return

        self._load_text(text, "file")

    def load_pasted(self):
        text = self.paste_text.get("1.0", "end")

        if not text.strip():
            messagebox.showwarning("Empty", "Paste some G-code first.")
            return

        self._load_text(text, "pasted")

    def clear_pasted(self):
        self.paste_text.delete("1.0", "end")

    def _load_text(self, text, source):
        self.moves = self.parse(text)

        if not self.moves:
            messagebox.showwarning("No moves", "No usable G0/G1/G2/G3 moves found.")
            return

        deepest = 0.0
        for m in self.moves:
            if m.kind == "G0":
                continue
            for p in m.points:
                if p[2] < deepest:
                    deepest = p[2]
        self.min_cut_z = deepest if deepest < 0 else -1.0

        self.reset_instructions()
        self.fit_view()
        self._overlap_grid_cache = None
        self._view_dirty = True
        self.draw()

        self.info.config(text=f"{len(self.moves)} moves loaded ({source})")

    def parse(self, text):
        x = y = z = 0.0
        feed = DEFAULT_FEED
        spindle = False
        absolute = True
        mode = "G0"

        moves = []

        for raw in text.splitlines():
            line = self.clean(raw)

            if not line:
                continue

            if "G90" in line:
                absolute = True

            if "G91" in line:
                absolute = False

            if "M3" in line or "M03" in line:
                spindle = True

            if "M5" in line or "M05" in line:
                spindle = False

            if "G0" in line or "G00" in line:
                mode = "G0"
            elif "G1" in line or "G01" in line:
                mode = "G1"
            elif "G2" in line or "G02" in line:
                mode = "G2"
            elif "G3" in line or "G03" in line:
                mode = "G3"

            nf = self.val(line, "F")
            if nf is not None:
                feed = nf

            nx = self.val(line, "X")
            ny = self.val(line, "Y")
            nz = self.val(line, "Z")

            if nx is None and ny is None and nz is None:
                continue

            sx, sy, sz = x, y, z

            ex = x if nx is None else (nx if absolute else x + nx)
            ey = y if ny is None else (ny if absolute else y + ny)
            ez = z if nz is None else (nz if absolute else z + nz)

            if mode in ["G2", "G3"]:
                i = self.val(line, "I") or 0
                j = self.val(line, "J") or 0
                points = self.arc_points(sx, sy, sz, ex, ey, ez, i, j, mode == "G2")
            else:
                points = [(sx, sy, sz), (ex, ey, ez)]

            moves.append(Move(mode, sx, sy, sz, ex, ey, ez, feed, spindle, points))

            x, y, z = ex, ey, ez

        return moves

    def arc_points(self, sx, sy, sz, ex, ey, ez, i, j, clockwise):
        cx = sx + i
        cy = sy + j
        r = math.hypot(sx - cx, sy - cy)

        a1 = math.atan2(sy - cy, sx - cx)
        a2 = math.atan2(ey - cy, ex - cx)

        if clockwise:
            if a2 > a1:
                a2 -= math.tau
        else:
            if a2 < a1:
                a2 += math.tau

        steps = max(16, int(abs(a2 - a1) * r / 1.5))
        pts = []

        for n in range(steps + 1):
            t = n / steps
            a = a1 + (a2 - a1) * t

            x = cx + math.cos(a) * r
            y = cy + math.sin(a) * r
            z = sz + (ez - sz) * t

            pts.append((x, y, z))

        return pts

    def _update_proj_cache(self):
        rz = math.radians(self.rot_z)
        rx = math.radians(self.rot_x)
        self._cos_rz = math.cos(rz)
        self._sin_rz = math.sin(rz)
        self._cos_rx = math.cos(rx)
        self._sin_rx = math.sin(rx)

    def project(self, x, y, z):
        cos_rz = self._cos_rz
        sin_rz = self._sin_rz
        cos_rx = self._cos_rx
        sin_rx = self._sin_rx
        zoom = self.zoom

        x2 = x * cos_rz - y * sin_rz
        y2 = x * sin_rz + y * cos_rz
        y3 = y2 * cos_rx - z * sin_rx

        return x2 * zoom + self.pan_x, y3 * zoom + self.pan_y

    def point_on_move(self, move, t):
        if t <= 0:
            return move.points[0]

        if t >= 1:
            return move.points[-1]

        target = move.length * t
        traveled = 0

        for i in range(1, len(move.points)):
            p1 = move.points[i - 1]
            p2 = move.points[i]
            seg = math.dist(p1, p2)

            if traveled + seg >= target:
                local = (target - traveled) / seg if seg else 0

                return (
                    p1[0] + (p2[0] - p1[0]) * local,
                    p1[1] + (p2[1] - p1[1]) * local,
                    p1[2] + (p2[2] - p1[2]) * local,
                )

            traveled += seg

        return move.points[-1]

    def remaining_points_for_move(self, move, t):
        cur = self.point_on_move(move, t)
        pts = [cur]

        target = move.length * t
        traveled = 0

        for i in range(1, len(move.points)):
            p1 = move.points[i - 1]
            p2 = move.points[i]
            seg = math.dist(p1, p2)

            if traveled + seg >= target:
                pts.extend(move.points[i:])
                break

            traveled += seg

        return pts

    def play(self):
        if not self.moves:
            return

        self.running = True
        self.last_time = time.time()
        self.animate()

    def pause(self):
        self.running = False

    def reset_instructions(self):
        self.running = False
        self.move_index = 0
        self.move_t = 0.0
        self.last_time = None
        self.cut_chains = []
        self.current_chain = []
        self._view_dirty = True
        self.draw()

    def animate(self):
        if not self.running or self.move_index >= len(self.moves):
            return

        now = time.time()
        dt = now - self.last_time
        self.last_time = now

        move = self.moves[self.move_index]

        if move.duration > 0:
            self.move_t += (dt * self.speed_mult.get()) / move.duration
        else:
            self.move_t = 1

        x, y, z = self.point_on_move(move, min(self.move_t, 1))

        cutting = move.spindle and z < SAFE_Z and move.kind != "G0"

        if cutting:
            if not self.current_chain or math.dist(self.current_chain[-1], (x, y, z)) >= 0.3:
                prev = self.current_chain[-1] if self.current_chain else None
                self.current_chain.append((x, y, z))
                if prev is not None:
                    self._append_live_segment(prev, (x, y, z))

            if len(self.current_chain) > 500:
                self._commit_current_chain(keep_last=True)
        else:
            if len(self.current_chain) > 1:
                self._commit_current_chain(keep_last=False)
            else:
                self.current_chain = []
                self.canvas.delete("live")

        if self.move_t >= 1:
            old_idx = self.move_index
            self.move_index += 1
            self.move_t = 0
            self.canvas.delete(f"move_{old_idx}")

        self.draw(current=(x, y, z, move.spindle, move.feed, move.kind))

        self.root.after(16, self.animate)

    def fit_view(self):
        xs = []
        ys = []

        for m in self.moves:
            for x, y, z in m.points:
                xs.append(x)
                ys.append(y)

        if not xs:
            return

        min_x = min(xs)
        max_x = max(xs)
        min_y = min(ys)
        max_y = max(ys)

        w = max(max_x - min_x, 1)
        h = max(max_y - min_y, 1)

        self.zoom = min(850 / w, 550 / h) * 0.8
        self.pan_x = 550
        self.pan_y = 450

    def draw(self, current=None):
        if self._view_dirty:
            self.canvas.delete("all")
            # canvas.delete("all") wipes our persistent dynamic items too — reset refs.
            self._tool_line_id = None
            self._tool_oval_id = None
            self._tool_text_id = None
            self._hud_text_id = None
            self._zbar_ids = None

            self.draw_grid(tag="static")
            self._render_future_moves()

            for chain in self.cut_chains:
                self._render_chain(chain, tag="static")

            if len(self.current_chain) >= 2:
                self._render_chain(self.current_chain, tag="live")

            if self.show_overlaps.get():
                self._render_overlaps(tag="static")

            self._view_dirty = False

        if current:
            x, y, z, spindle, feed, kind = current
        elif self.moves:
            m = self.moves[min(self.move_index, len(self.moves) - 1)]
            x, y, z = m.sx, m.sy, m.sz
            spindle = m.spindle
            feed = m.feed
            kind = m.kind
        else:
            return

        self.draw_tool(x, y, z, spindle)
        self.draw_z_bar(z)
        self._update_hud(x, y, z, spindle, feed, kind)

    def _update_hud(self, x, y, z, spindle, feed, kind):
        text = (
            f"Move: {self.move_index}/{len(self.moves)}  "
            f"{kind}  "
            f"X:{x:.3f} Y:{y:.3f} Z:{z:.3f}  "
            f"Feed:{feed:.1f} mm/min  "
            f"Speed:{self.speed_mult.get():.1f}x  "
            f"Spindle:{'ON' if spindle else 'OFF'}"
        )
        if self._hud_text_id is None:
            self._hud_text_id = self.canvas.create_text(
                15, 15, anchor="nw", fill="white",
                font=("Consolas", 12), text=text,
            )
        else:
            self.canvas.itemconfig(self._hud_text_id, text=text)

    def draw_grid(self, tag="static"):
        step = 25
        size = 1000

        for x in range(-size, size + 1, step):
            self.canvas.create_line(
                *self.project(x, -size, 0),
                *self.project(x, size, 0),
                fill="#222222",
                tags=tag,
            )

        for y in range(-size, size + 1, step):
            self.canvas.create_line(
                *self.project(-size, y, 0),
                *self.project(size, y, 0),
                fill="#222222",
                tags=tag,
            )

        self.canvas.create_line(*self.project(0, 0, 0), *self.project(100, 0, 0), fill="red", width=2, tags=tag)
        self.canvas.create_line(*self.project(0, 0, 0), *self.project(0, 100, 0), fill="#4da3ff", width=2, tags=tag)
        self.canvas.create_line(*self.project(0, 0, 0), *self.project(0, 0, 50), fill="yellow", width=2, tags=tag)

    def _render_move_path(self, move, pts, tag):
        n = len(pts)
        if n < 2:
            return

        is_cutting_move = move.spindle and move.kind != "G0"
        thick = max(1, int(self._bit_width() * self.zoom))
        project = self.project

        # Fast path: single 2-point move with no z=0 crossing.
        # This is the dominant case in G1-only files (one Move per G-code line).
        if n == 2:
            p1, p2 = pts[0], pts[1]
            z1, z2 = p1[2], p2[2]
            if not (is_cutting_move and z1 * z2 < 0):
                if is_cutting_move and min(z1, z2) < 0:
                    color, width, dash = "#333333", thick, None
                elif is_cutting_move:
                    color, width, dash = "#666666", 1, (2, 4)
                else:
                    color, width, dash = "#666666", 1, None
                kwargs = {
                    "fill": color, "width": width,
                    "capstyle": tk.ROUND, "joinstyle": tk.ROUND,
                    "tags": tag,
                }
                if dash is not None:
                    kwargs["dash"] = dash
                sax, say = project(*p1)
                sbx, sby = project(*p2)
                self.canvas.create_line(sax, say, sbx, sby, **kwargs)
                return

        # General path: multi-point moves (arcs) or z=0-crossing splits — batch by style.
        sub_segs = []
        for i in range(1, n):
            p1 = pts[i - 1]
            p2 = pts[i]
            if is_cutting_move and p1[2] * p2[2] < 0:
                t = -p1[2] / (p2[2] - p1[2])
                cross = (
                    p1[0] + t * (p2[0] - p1[0]),
                    p1[1] + t * (p2[1] - p1[1]),
                    0,
                )
                sub_segs.append((p1, cross))
                sub_segs.append((cross, p2))
            else:
                sub_segs.append((p1, p2))

        if not sub_segs:
            return

        STYLE_IN_MAT = ("#333333", thick, None)
        STYLE_AIR_CUT = ("#666666", 1, (2, 4))
        STYLE_RAPID = ("#666666", 1, None)

        def style_for(sa, sb):
            if is_cutting_move and min(sa[2], sb[2]) < 0:
                return STYLE_IN_MAT
            if is_cutting_move:
                return STYLE_AIR_CUT
            return STYLE_RAPID

        cur_style = style_for(*sub_segs[0])
        run_start = 0
        for i in range(1, len(sub_segs)):
            seg_style = style_for(*sub_segs[i])
            if seg_style != cur_style:
                self._emit_polyline(sub_segs, run_start, i - 1, cur_style, tag, project)
                cur_style = seg_style
                run_start = i
        self._emit_polyline(sub_segs, run_start, len(sub_segs) - 1, cur_style, tag, project)

    def _emit_polyline(self, segments, start, end, style, tag, project):
        coords = []
        sa, _sb = segments[start]
        sax, say = project(*sa)
        coords.extend([sax, say])
        for i in range(start, end + 1):
            _sa, sb = segments[i]
            sbx, sby = project(*sb)
            coords.extend([sbx, sby])

        color, width, dash = style
        kwargs = {
            "fill": color,
            "width": width,
            "capstyle": tk.ROUND,
            "joinstyle": tk.ROUND,
            "tags": tag,
        }
        if dash is not None:
            kwargs["dash"] = dash
        self.canvas.create_line(*coords, **kwargs)

    def _render_future_moves(self):
        if not self.moves:
            return

        for idx in range(self.move_index, len(self.moves)):
            move = self.moves[idx]
            self._render_move_path(move, move.points, tag=("static", f"move_{idx}"))

    def depth_color(self, z):
        min_z = self.min_cut_z if self.min_cut_z < 0 else -1.0

        if z >= 0:
            t = 0.0
        else:
            t = max(0.0, min(1.0, z / min_z))

        stops = [
            (0.00, (0x6f, 0xff, 0xa0)),
            (0.25, (0xff, 0xd9, 0x3d)),
            (0.55, (0xff, 0x7a, 0x1f)),
            (0.85, (0xd6, 0x1a, 0x1a)),
            (1.00, (0x4a, 0x00, 0x40)),
        ]

        for i in range(1, len(stops)):
            t2, c2 = stops[i]
            if t <= t2:
                t1, c1 = stops[i - 1]
                span = t2 - t1
                local = (t - t1) / span if span else 0
                r = int(c1[0] + (c2[0] - c1[0]) * local)
                g = int(c1[1] + (c2[1] - c1[1]) * local)
                b = int(c1[2] + (c2[2] - c1[2]) * local)
                return f"#{r:02x}{g:02x}{b:02x}"

        r, g, b = stops[-1][1]
        return f"#{r:02x}{g:02x}{b:02x}"

    def shade_color(self, hex_color, factor):
        r = int(hex_color[1:3], 16)
        g = int(hex_color[3:5], 16)
        b = int(hex_color[5:7], 16)
        r = max(0, min(255, int(r * factor)))
        g = max(0, min(255, int(g * factor)))
        b = max(0, min(255, int(b * factor)))
        return f"#{r:02x}{g:02x}{b:02x}"

    def _append_live_segment(self, p1, p2):
        width = max(1, int(self._bit_width() * self.zoom))
        seg_z = p1[2] if p1[2] < p2[2] else p2[2]
        color = self.depth_color(seg_z)
        sax, say = self.project(*p1)
        sbx, sby = self.project(*p2)
        self.canvas.create_line(
            sax, say, sbx, sby,
            fill=color,
            width=width,
            capstyle=tk.ROUND,
            joinstyle=tk.ROUND,
            tags="live",
        )

    def _render_chain(self, chain, tag):
        if len(chain) < 2:
            return

        width = max(1, int(self._bit_width() * self.zoom))
        project = self.project
        create_line = self.canvas.create_line
        depth_color = self.depth_color

        p1 = chain[0]
        sax, say = project(*p1)

        for i in range(1, len(chain)):
            p2 = chain[i]
            sbx, sby = project(*p2)

            seg_z = p1[2] if p1[2] < p2[2] else p2[2]
            color = depth_color(seg_z)

            create_line(
                sax, say, sbx, sby,
                fill=color,
                width=width,
                capstyle=tk.ROUND,
                joinstyle=tk.ROUND,
                tags=tag,
            )

            p1 = p2
            sax, say = sbx, sby

    def _commit_current_chain(self, keep_last):
        if len(self.current_chain) < 2:
            if not keep_last:
                self.current_chain = []
            return

        self.cut_chains.append(self.current_chain)

        self.canvas.delete("live")
        self._render_chain(self.current_chain, tag="static")

        if keep_last:
            self.current_chain = [self.current_chain[-1]]
        else:
            self.current_chain = []

    def draw_tool(self, x, y, z, spindle):
        tip = self.project(x, y, z)
        top = self.project(x, y, z + 40)
        r = max(4, self._bit_width() * self.zoom / 2)
        spindle_text = "SPINDLE ON" if spindle else "SPINDLE OFF"
        spindle_color = "#31d158" if spindle else "#ff453a"

        if self._tool_line_id is None:
            self._tool_line_id = self.canvas.create_line(
                *top, *tip, fill="orange", width=5,
            )
            self._tool_oval_id = self.canvas.create_oval(
                tip[0] - r, tip[1] - r, tip[0] + r, tip[1] + r,
                outline="orange", width=2,
            )
            self._tool_text_id = self.canvas.create_text(
                tip[0] + 25, tip[1] - 20,
                text=spindle_text, fill=spindle_color,
                font=("Consolas", 10),
            )
        else:
            self.canvas.coords(self._tool_line_id, *top, *tip)
            self.canvas.coords(self._tool_oval_id,
                               tip[0] - r, tip[1] - r,
                               tip[0] + r, tip[1] + r)
            self.canvas.coords(self._tool_text_id, tip[0] + 25, tip[1] - 20)
            self.canvas.itemconfig(self._tool_text_id,
                                   text=spindle_text, fill=spindle_color)

    def draw_z_bar(self, z):
        canvas_w = self.canvas.winfo_width()
        if canvas_w < 100:
            canvas_w = int(self.canvas["width"])

        x = canvas_w - 70
        y = 80
        h = 260
        w = 28
        z_min, z_max = -10, 10

        z_clamped = max(z_min, min(z_max, z))
        zero_y = y + (z_max / (z_max - z_min)) * h
        z_y = y + ((z_max - z_clamped) / (z_max - z_min)) * h
        fill_color = self.depth_color(z) if z < 0 else "#4da3ff"

        if self._zbar_ids is None:
            ids = {}
            ids["title"] = self.canvas.create_text(
                x, y - 35, text="Z Axis", fill="white",
                font=("Consolas", 12, "bold"),
            )
            ids["value"] = self.canvas.create_text(
                x, y - 15, text=f"{z:+.3f} mm", fill="white",
                font=("Consolas", 11),
            )
            ids["frame"] = self.canvas.create_rectangle(
                x - w // 2, y, x + w // 2, y + h,
                outline="white", width=2,
            )
            ids["zero_line"] = self.canvas.create_line(
                x - 22, zero_y, x + 22, zero_y, fill="yellow", width=2,
            )
            ids["zero_label"] = self.canvas.create_text(
                x + 45, zero_y, text="0", fill="yellow",
                font=("Consolas", 10),
            )
            ids["fill"] = self.canvas.create_rectangle(
                x - w // 2 + 3, min(z_y, zero_y),
                x + w // 2 - 3, max(z_y, zero_y),
                fill=fill_color, outline="",
            )
            ids["ticks"] = []
            for mark in [-10, -5, 0, 5, 10]:
                my = y + ((z_max - mark) / (z_max - z_min)) * h
                tick_id = self.canvas.create_line(
                    x - 18, my, x - 12, my, fill="white",
                )
                label_id = self.canvas.create_text(
                    x - 42, my, text=f"{mark:+}", fill="white",
                    font=("Consolas", 8),
                )
                ids["ticks"].append((tick_id, label_id, mark))
            self._zbar_ids = ids
        else:
            ids = self._zbar_ids
            self.canvas.itemconfig(ids["value"], text=f"{z:+.3f} mm")
            self.canvas.coords(ids["fill"],
                               x - w // 2 + 3, min(z_y, zero_y),
                               x + w // 2 - 3, max(z_y, zero_y))
            self.canvas.itemconfig(ids["fill"], fill=fill_color)
            # Title, frame, zero line/label, and ticks are static-positioned —
            # no per-frame update needed unless canvas resized. The dirty path
            # handles resize by re-creating everything from scratch.

    def reset_view(self):
        self.rot_x = 55
        self.rot_z = -35
        self._update_proj_cache()
        self.fit_view()
        self._view_dirty = True
        self.draw()

    def mouse_down(self, event):
        self.last_mouse = (event.x, event.y)

    def pan(self, event):
        if not self.last_mouse:
            return

        dx = event.x - self.last_mouse[0]
        dy = event.y - self.last_mouse[1]

        self.pan_x += dx
        self.pan_y += dy
        self.last_mouse = (event.x, event.y)

        self.canvas.move("static", dx, dy)
        self.canvas.move("live", dx, dy)
        self.draw()

    def rotate(self, event):
        if not self.last_mouse:
            return

        dx = event.x - self.last_mouse[0]
        dy = event.y - self.last_mouse[1]

        self.rot_z += dx * 0.4
        self.rot_x += dy * 0.4
        self.rot_x = max(5, min(85, self.rot_x))
        self._update_proj_cache()

        self.last_mouse = (event.x, event.y)
        self._view_dirty = True
        self._request_redraw()

    def zoom_mouse(self, event):
        factor = 1.1 if event.delta > 0 else 1 / 1.1
        cx, cy = event.x, event.y

        self.zoom *= factor
        self.pan_x = (self.pan_x - cx) * factor + cx
        self.pan_y = (self.pan_y - cy) * factor + cy

        self.canvas.scale("static", cx, cy, factor, factor)
        self.canvas.scale("live", cx, cy, factor, factor)
        self.draw()


if __name__ == "__main__":
    root = tk.Tk()
    app = Viewer(root)
    root.mainloop()