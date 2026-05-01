"""G-code parser. Pure logic — no UI deps.

Ported from gcode_preview.py. The state machine handles modal G-code:
G0/G1/G2/G3 motion modes, G90/G91 distance modes, M3/M5 spindle, and per-axis
position carry-over (X/Y/Z hold their previous value when omitted).

Arcs (G2/G3) are linearized via arc_points() using I/J center-offset form.
R-form arcs are not currently supported (silently produce a degenerate move).
"""

import math
import re

SAFE_Z = 0.0
DEFAULT_FEED = 500.0
RAPID_FEED = 2500.0


class Move:
    """One toolpath segment (or arc, expanded into multiple points).

    kind: "G0" | "G1" | "G2" | "G3"
    sx, sy, sz: start position
    ex, ey, ez: end position
    feed: F value at the time of this move (mm/min)
    spindle: True if M3 active
    points: list of (x, y, z) tuples — for straight moves, [start, end];
            for arcs, multiple linearized points
    length: total path length in mm
    duration: elapsed time in seconds at the move's feed (rapids use RAPID_FEED)
    """

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

        self.length = 0.0
        for i in range(1, len(self.points)):
            self.length += math.dist(self.points[i - 1], self.points[i])

        used_feed = RAPID_FEED if kind == "G0" else self.feed
        self.duration = self.length / (used_feed / 60.0) if used_feed > 0 else 0.01


def clean(line):
    """Strip parenthesized comments and ;-comments, uppercase, and trim."""
    line = re.sub(r"\(.*?\)", "", line)
    line = line.split(";")[0]
    return line.strip().upper()


def val(line, letter):
    """Extract a numeric value following the given letter (e.g. 'X12.5').
    Returns None if the letter is not present.
    """
    m = re.search(letter + r"(-?\d+\.?\d*)", line)
    return float(m.group(1)) if m else None


def arc_points(sx, sy, sz, ex, ey, ez, i, j, clockwise):
    """Linearize a G2/G3 arc from (sx,sy) to (ex,ey) about center offsets (i,j).

    The center is at (sx+i, sy+j). Z linearly interpolates from sz to ez along
    the arc parameter. Step count scales with arc length to keep facets ~1.5mm.
    """
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


def parse(text):
    """Parse a G-code program (string) into a list of Move objects.

    Modal state tracked: position (x/y/z), feed, spindle, distance mode
    (G90 abs / G91 rel), and current motion mode (G0/G1/G2/G3 — sticky
    across lines).

    Lines with no XYZ change are skipped (no Move emitted) — feed-only
    lines like "F500" update state but don't appear in the output.
    """
    x = y = z = 0.0
    feed = DEFAULT_FEED
    spindle = False
    absolute = True
    mode = "G0"

    moves = []

    for raw in text.splitlines():
        line = clean(raw)
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

        nf = val(line, "F")
        if nf is not None:
            feed = nf

        nx = val(line, "X")
        ny = val(line, "Y")
        nz = val(line, "Z")

        if nx is None and ny is None and nz is None:
            continue

        sx, sy, sz = x, y, z
        ex = x if nx is None else (nx if absolute else x + nx)
        ey = y if ny is None else (ny if absolute else y + ny)
        ez = z if nz is None else (nz if absolute else z + nz)

        if mode in ("G2", "G3"):
            i = val(line, "I") or 0
            j = val(line, "J") or 0
            points = arc_points(sx, sy, sz, ex, ey, ez, i, j, mode == "G2")
        else:
            points = [(sx, sy, sz), (ex, ey, ez)]

        moves.append(Move(mode, sx, sy, sz, ex, ey, ez, feed, spindle, points))
        x, y, z = ex, ey, ez

    return moves


def parse_file(path):
    """Convenience wrapper: read a .nc / .gcode file from disk and parse it."""
    with open(path, "r", errors="ignore") as f:
        return parse(f.read())


def bounds(moves):
    """Return ((min_x, min_y, min_z), (max_x, max_y, max_z)) over all move points,
    or None if the moves list is empty.
    """
    if not moves:
        return None
    xs = []
    ys = []
    zs = []
    for m in moves:
        for px, py, pz in m.points:
            xs.append(px)
            ys.append(py)
            zs.append(pz)
    return (min(xs), min(ys), min(zs)), (max(xs), max(ys), max(zs))


def deepest_cut_z(moves):
    """Return the most-negative Z reached during any cutting (non-G0) move,
    or -1.0 as a fallback if no cut goes below zero.
    """
    deepest = 0.0
    for m in moves:
        if m.kind == "G0":
            continue
        for p in m.points:
            if p[2] < deepest:
                deepest = p[2]
    return deepest if deepest < 0 else -1.0
