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

        self.bit_width = tk.DoubleVar(value=3.175)
        self.speed_mult = tk.DoubleVar(value=1.0)

        self.zoom = 1
        self.pan_x = 550
        self.pan_y = 420
        self.rot_x = 55
        self.rot_z = -35
        self.last_mouse = None

        self.build_ui()

    def build_ui(self):
        top = tk.Frame(self.root)
        top.pack(fill="x")

        tk.Button(top, text="Upload / Select .nc", command=self.load_file).pack(side="left", padx=5, pady=5)

        tk.Label(top, text="Bit width mm:").pack(side="left")
        tk.Entry(top, textvariable=self.bit_width, width=8).pack(side="left", padx=5)

        tk.Label(top, text="Speed:").pack(side="left")
        tk.Scale(top, variable=self.speed_mult, from_=0.1, to=10, resolution=0.1,
                 orient="horizontal", length=160).pack(side="left")

        tk.Button(top, text="Play", command=self.play).pack(side="left", padx=5)
        tk.Button(top, text="Pause", command=self.pause).pack(side="left", padx=5)
        tk.Button(top, text="Reset Instructions", command=self.reset_instructions).pack(side="left", padx=5)
        tk.Button(top, text="Reset View", command=self.reset_view).pack(side="left", padx=5)

        self.info = tk.Label(top, text="No file loaded")
        self.info.pack(side="left", padx=15)

        self.canvas = tk.Canvas(self.root, bg="#151515", width=1100, height=760)
        self.canvas.pack(fill="both", expand=True)

        self.canvas.bind("<ButtonPress-1>", self.mouse_down)
        self.canvas.bind("<B1-Motion>", self.pan)

        self.canvas.bind("<ButtonPress-3>", self.mouse_down)
        self.canvas.bind("<B3-Motion>", self.rotate)

        self.canvas.bind("<MouseWheel>", self.zoom_mouse)

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

        self.moves = self.parse(text)

        if not self.moves:
            messagebox.showwarning("No moves", "No usable G0/G1/G2/G3 moves found.")
            return

        self.reset_instructions()
        self.fit_view()
        self.draw()

        self.info.config(text=f"{len(self.moves)} moves loaded")

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

    def project(self, x, y, z):
        rz = math.radians(self.rot_z)
        rx = math.radians(self.rot_x)

        x2 = x * math.cos(rz) - y * math.sin(rz)
        y2 = x * math.sin(rz) + y * math.cos(rz)

        y3 = y2 * math.cos(rx) - z * math.sin(rx)

        return x2 * self.zoom + self.pan_x, y3 * self.zoom + self.pan_y

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
            self.current_chain.append((x, y, z))
        else:
            if len(self.current_chain) > 1:
                self.cut_chains.append(self.current_chain)
            self.current_chain = []

        if self.move_t >= 1:
            if not cutting and len(self.current_chain) > 1:
                self.cut_chains.append(self.current_chain)
                self.current_chain = []

            self.move_index += 1
            self.move_t = 0

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
        self.canvas.delete("all")

        self.draw_grid()
        self.draw_remaining_path()
        self.draw_cut_trace()

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

        self.canvas.create_text(
            15, 15,
            anchor="nw",
            fill="white",
            font=("Consolas", 12),
            text=(
                f"Move: {self.move_index}/{len(self.moves)}  "
                f"{kind}  "
                f"X:{x:.3f} Y:{y:.3f} Z:{z:.3f}  "
                f"Feed:{feed:.1f} mm/min  "
                f"Speed:{self.speed_mult.get():.1f}x  "
                f"Spindle:{'ON' if spindle else 'OFF'}"
            )
        )

    def draw_grid(self):
        step = 25
        size = 1000

        for x in range(-size, size + 1, step):
            self.canvas.create_line(*self.project(x, -size, 0), *self.project(x, size, 0), fill="#222222")

        for y in range(-size, size + 1, step):
            self.canvas.create_line(*self.project(-size, y, 0), *self.project(size, y, 0), fill="#222222")

        self.canvas.create_line(*self.project(0, 0, 0), *self.project(100, 0, 0), fill="red", width=2)
        self.canvas.create_line(*self.project(0, 0, 0), *self.project(0, 100, 0), fill="#4da3ff", width=2)
        self.canvas.create_line(*self.project(0, 0, 0), *self.project(0, 0, 50), fill="yellow", width=2)

    def draw_remaining_path(self):
        if not self.moves or self.move_index >= len(self.moves):
            return

        for idx in range(self.move_index, len(self.moves)):
            move = self.moves[idx]

            if idx == self.move_index:
                pts = self.remaining_points_for_move(move, self.move_t)
            else:
                pts = move.points

            if len(pts) < 2:
                continue

            flat = []
            for p in pts:
                flat.extend(self.project(*p))

            if move.spindle and move.kind != "G0":
                color = "#333333"
                width = max(1, int(self.bit_width.get() * self.zoom))
            else:
                color = "#666666"
                width = 1

            self.canvas.create_line(
                *flat,
                fill=color,
                width=width,
                capstyle=tk.ROUND,
                joinstyle=tk.ROUND
            )

    def draw_cut_trace(self):
        width = max(1, int(self.bit_width.get() * self.zoom))

        all_chains = self.cut_chains[:]

        if len(self.current_chain) > 1:
            all_chains.append(self.current_chain)

        for chain in all_chains:
            flat = []
            for p in chain:
                flat.extend(self.project(*p))

            if len(flat) >= 4:
                self.canvas.create_line(
                    *flat,
                    fill="#31d158",
                    width=width,
                    capstyle=tk.ROUND,
                    joinstyle=tk.ROUND
                )

    def draw_tool(self, x, y, z, spindle):
        tip = self.project(x, y, z)
        top = self.project(x, y, z + 40)

        self.canvas.create_line(*top, *tip, fill="orange", width=5)

        r = max(4, self.bit_width.get() * self.zoom / 2)
        self.canvas.create_oval(
            tip[0] - r,
            tip[1] - r,
            tip[0] + r,
            tip[1] + r,
            outline="orange",
            width=2
        )

        self.canvas.create_text(
            tip[0] + 25,
            tip[1] - 20,
            text="SPINDLE ON" if spindle else "SPINDLE OFF",
            fill="#31d158" if spindle else "#ff453a",
            font=("Consolas", 10)
        )

    def reset_view(self):
        self.rot_x = 55
        self.rot_z = -35
        self.fit_view()
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
        self.draw()

    def rotate(self, event):
        if not self.last_mouse:
            return

        dx = event.x - self.last_mouse[0]
        dy = event.y - self.last_mouse[1]

        self.rot_z += dx * 0.4
        self.rot_x += dy * 0.4
        self.rot_x = max(5, min(85, self.rot_x))

        self.last_mouse = (event.x, event.y)
        self.draw()

    def zoom_mouse(self, event):
        if event.delta > 0:
            self.zoom *= 1.1
        else:
            self.zoom /= 1.1

        self.draw()


if __name__ == "__main__":
    root = tk.Tk()
    app = Viewer(root)
    root.mainloop()