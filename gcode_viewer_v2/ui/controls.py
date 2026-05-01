"""Custom Qt widgets for the v2 viewer.

Currently:
- ZBar: vertical Z-axis indicator showing the tool's current Z height,
  color-coded by depth (matches the gradient used in the 3D scene).
"""

from PyQt5 import QtCore, QtGui, QtWidgets


# Same 5-stop gradient as scene/path.py — duplicated here so this widget
# is independent of VTK (it uses Qt painting, not VTK).
_DEPTH_STOPS = [
    (0.00, 0x6f, 0xff, 0xa0),
    (0.25, 0xff, 0xd9, 0x3d),
    (0.55, 0xff, 0x7a, 0x1f),
    (0.85, 0xd6, 0x1a, 0x1a),
    (1.00, 0x4a, 0x00, 0x40),
]


def _depth_color(z, min_z):
    """Return a QColor matching the depth gradient at given z (with deepest = min_z)."""
    if z >= 0:
        t = 0.0
    else:
        t = max(0.0, min(1.0, z / min(min_z, -1.0)))
    for i in range(1, len(_DEPTH_STOPS)):
        t2, r2, g2, b2 = _DEPTH_STOPS[i]
        if t <= t2:
            t1, r1, g1, b1 = _DEPTH_STOPS[i - 1]
            span = t2 - t1
            local = (t - t1) / span if span else 0
            r = int(r1 + (r2 - r1) * local)
            g = int(g1 + (g2 - g1) * local)
            b = int(b1 + (b2 - b1) * local)
            return QtGui.QColor(r, g, b)
    _, r, g, b = _DEPTH_STOPS[-1]
    return QtGui.QColor(r, g, b)


class ZBar(QtWidgets.QWidget):
    """Vertical Z-axis indicator widget.

    Range: ±10 mm by default. The fill rectangle stretches from z=0 down to
    the current z value (or up if z>0), color-coded by depth.
    """

    Z_MIN = -10.0
    Z_MAX = 10.0
    BAR_WIDTH = 28
    BAR_HEIGHT = 280

    # Match the VTK viewport background (0.08, 0.08, 0.10 in vtkRenderer's
    # 0..1 floats) so the dock and the 3D scene read as one continuous panel.
    BG_COLOR = QtGui.QColor(20, 20, 26)

    def __init__(self, parent=None):
        super().__init__(parent)
        self._z = 0.0
        self._min_cut_z = -1.0
        self.setMinimumSize(110, self.BAR_HEIGHT + 80)
        self.setMaximumWidth(110)
        # Tell Qt we paint our entire surface; skip the default light theme fill
        self.setAutoFillBackground(True)
        pal = self.palette()
        pal.setColor(QtGui.QPalette.Window, self.BG_COLOR)
        self.setPalette(pal)

    def set_z(self, z):
        self._z = float(z)
        self.update()  # schedule a repaint

    def set_min_cut_z(self, min_z):
        self._min_cut_z = float(min_z)
        self.update()

    def paintEvent(self, _event):
        p = QtGui.QPainter(self)
        p.setRenderHint(QtGui.QPainter.Antialiasing)

        # Paint our dark backdrop first — the rest of the widget is designed
        # for white-on-dark contrast (matches the VTK 3D viewport color).
        p.fillRect(self.rect(), self.BG_COLOR)

        w = self.width()
        h = self.height()

        # Layout: bar centered horizontally, vertical position with margin
        cx = w // 2
        bar_x = cx - self.BAR_WIDTH // 2
        bar_top = 50
        bar_h = self.BAR_HEIGHT

        # Title and current value
        p.setPen(QtGui.QColor(255, 255, 255))
        font = QtGui.QFont("Consolas", 10, QtGui.QFont.Bold)
        p.setFont(font)
        p.drawText(0, 0, w, 18, QtCore.Qt.AlignCenter, "Z Axis")

        font.setBold(False)
        p.setFont(font)
        p.drawText(0, 22, w, 18, QtCore.Qt.AlignCenter, f"{self._z:+.3f} mm")

        # Frame
        p.setPen(QtGui.QPen(QtGui.QColor(255, 255, 255), 2))
        p.drawRect(bar_x, bar_top, self.BAR_WIDTH, bar_h)

        # Fill (depth-colored bar from zero baseline)
        z = max(self.Z_MIN, min(self.Z_MAX, self._z))
        zero_y = bar_top + (self.Z_MAX / (self.Z_MAX - self.Z_MIN)) * bar_h
        z_y = bar_top + ((self.Z_MAX - z) / (self.Z_MAX - self.Z_MIN)) * bar_h
        if z < 0:
            color = _depth_color(z, self._min_cut_z)
        else:
            color = QtGui.QColor(0x4d, 0xa3, 0xff)
        p.fillRect(
            QtCore.QRectF(bar_x + 3, min(z_y, zero_y),
                          self.BAR_WIDTH - 6, abs(z_y - zero_y)),
            color,
        )

        # Zero line + label
        p.setPen(QtGui.QPen(QtGui.QColor(255, 230, 80), 2))
        p.drawLine(bar_x - 8, int(zero_y), bar_x + self.BAR_WIDTH + 8, int(zero_y))

        # Tick marks at -10, -5, 0, 5, 10
        p.setPen(QtGui.QPen(QtGui.QColor(255, 255, 255), 1))
        font.setPointSize(8)
        p.setFont(font)
        for mark in (-10, -5, 0, 5, 10):
            my = bar_top + ((self.Z_MAX - mark) / (self.Z_MAX - self.Z_MIN)) * bar_h
            p.drawLine(bar_x - 5, int(my), bar_x, int(my))
            label_color = QtGui.QColor(255, 230, 80) if mark == 0 else QtGui.QColor(255, 255, 255)
            p.setPen(QtGui.QPen(label_color, 1))
            p.drawText(QtCore.QRectF(bar_x + self.BAR_WIDTH + 4, my - 8, 30, 16),
                       QtCore.Qt.AlignVCenter | QtCore.Qt.AlignLeft,
                       f"{mark:+}")
            p.setPen(QtGui.QPen(QtGui.QColor(255, 255, 255), 1))
