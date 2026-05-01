"""Debug window — separate top-level window with:

  • System / library / OpenGL info (read-only, populated once at open)
  • Live frame rate (updated once per second from a VTK render observer)
  • Log tail capturing stdout, stderr, and VTK warnings/errors

Thread-safe: log appends are marshalled to the GUI thread via a queued
signal, so writes from non-GUI threads (e.g., VTK callbacks) are safe.
"""

import platform
import sys
import time

from PyQt5 import QtCore, QtGui, QtWidgets


class DebugWindow(QtWidgets.QWidget):
    """Floating debug window. Construct once, open via Debug → Open Debug Window."""

    # Queued signal so any thread can call append_log safely.
    log_line_appended = QtCore.pyqtSignal(str)

    def __init__(self, parent=None):
        super().__init__(parent)
        self.setWindowTitle("Debug — gcode_viewer_v2")
        self.resize(760, 560)
        # Make this a real top-level window even with a parent set
        self.setWindowFlags(QtCore.Qt.Window)

        # ── Layout ───────────────────────────────────────────────────────────
        root = QtWidgets.QVBoxLayout(self)
        root.setContentsMargins(8, 8, 8, 8)

        # System info group
        info_group = QtWidgets.QGroupBox("System")
        info_layout = QtWidgets.QVBoxLayout(info_group)
        self.info_label = QtWidgets.QLabel("…")
        self.info_label.setFont(QtGui.QFont("Consolas", 9))
        self.info_label.setTextInteractionFlags(QtCore.Qt.TextSelectableByMouse)
        info_layout.addWidget(self.info_label)
        root.addWidget(info_group)

        # FPS row
        fps_group = QtWidgets.QGroupBox("Render")
        fps_layout = QtWidgets.QHBoxLayout(fps_group)
        fps_layout.addWidget(QtWidgets.QLabel("Frames per second:"))
        self.fps_label = QtWidgets.QLabel("--")
        self.fps_label.setFont(QtGui.QFont("Consolas", 12, QtGui.QFont.Bold))
        self.fps_label.setMinimumWidth(60)
        self.fps_label.setStyleSheet("color: #6cf;")
        fps_layout.addWidget(self.fps_label)
        fps_layout.addSpacing(20)
        fps_layout.addWidget(QtWidgets.QLabel("Renders since open:"))
        self.total_renders_label = QtWidgets.QLabel("0")
        self.total_renders_label.setFont(QtGui.QFont("Consolas", 11))
        fps_layout.addWidget(self.total_renders_label)
        fps_layout.addStretch()
        root.addWidget(fps_group)

        # Log group
        log_group = QtWidgets.QGroupBox("Log (stdout + stderr + VTK)")
        log_layout = QtWidgets.QVBoxLayout(log_group)
        self.log = QtWidgets.QPlainTextEdit()
        self.log.setReadOnly(True)
        self.log.setFont(QtGui.QFont("Consolas", 9))
        self.log.setLineWrapMode(QtWidgets.QPlainTextEdit.NoWrap)
        self.log.setMaximumBlockCount(5000)  # bounded buffer — drop oldest lines
        log_layout.addWidget(self.log)

        ctrl = QtWidgets.QHBoxLayout()
        clear_btn = QtWidgets.QPushButton("Clear log")
        clear_btn.clicked.connect(self.log.clear)
        ctrl.addWidget(clear_btn)
        self.autoscroll_chk = QtWidgets.QCheckBox("Auto-scroll")
        self.autoscroll_chk.setChecked(True)
        ctrl.addWidget(self.autoscroll_chk)
        ctrl.addStretch()
        log_layout.addLayout(ctrl)
        root.addWidget(log_group, stretch=1)

        # ── Wiring ───────────────────────────────────────────────────────────
        # Queued signal → main thread → QPlainTextEdit
        self.log_line_appended.connect(self._on_log_line, QtCore.Qt.QueuedConnection)

        # FPS computation
        self._renders_in_window = 0
        self._total_renders = 0
        self._fps_window_start = time.perf_counter()
        self._fps_timer = QtCore.QTimer(self)
        self._fps_timer.setInterval(1000)  # 1 Hz refresh
        self._fps_timer.timeout.connect(self._update_fps)

    # ── Public API ───────────────────────────────────────────────────────────

    def populate_specs(self, vtk_version, qt_version, gl_info=None):
        """Show system info. Call once after the GL context is initialized."""
        lines = [
            f"Python    : {sys.version.split()[0]} ({platform.python_implementation()})",
            f"Platform  : {platform.platform()}",
            f"Machine   : {platform.machine()}  •  CPU: {platform.processor() or 'unknown'}",
            f"PyQt5     : {qt_version}",
            f"VTK       : {vtk_version}",
        ]
        if gl_info:
            for label, value in gl_info.items():
                lines.append(f"{label:<10}: {value}")
        self.info_label.setText("\n".join(lines))

    def append_log(self, text):
        """Thread-safe: emit a queued signal that the GUI thread will pick up."""
        if not text:
            return
        # Strip a single trailing newline; QPlainTextEdit.appendPlainText adds one.
        if text.endswith("\n"):
            text = text[:-1]
        if text:
            self.log_line_appended.emit(text)

    def note_render(self):
        """Called from a VTK render-window observer on each Render(). Cheap."""
        self._renders_in_window += 1
        self._total_renders += 1

    # ── Slots / private ──────────────────────────────────────────────────────

    def _on_log_line(self, text):
        # GUI-thread side: append + maybe-autoscroll
        for line in text.split("\n"):
            self.log.appendPlainText(line)
        if self.autoscroll_chk.isChecked():
            sb = self.log.verticalScrollBar()
            sb.setValue(sb.maximum())

    def _update_fps(self):
        elapsed = time.perf_counter() - self._fps_window_start
        fps = self._renders_in_window / elapsed if elapsed > 1e-3 else 0.0
        self.fps_label.setText(f"{fps:5.1f}")
        self.total_renders_label.setText(str(self._total_renders))
        self._renders_in_window = 0
        self._fps_window_start = time.perf_counter()

    def showEvent(self, event):
        super().showEvent(event)
        # Reset window so the first second's fps reading isn't biased by idle time
        self._renders_in_window = 0
        self._fps_window_start = time.perf_counter()
        self._fps_timer.start()

    def hideEvent(self, event):
        super().hideEvent(event)
        self._fps_timer.stop()


class _TeeStream:
    """File-like wrapper that mirrors writes to the original stream AND a
    callback (used to pipe stdout/stderr into the debug window's log).
    """
    def __init__(self, original, on_write):
        self._original = original
        self._on_write = on_write

    def write(self, text):
        try:
            self._original.write(text)
        except Exception:
            pass
        try:
            self._on_write(text)
        except Exception:
            pass

    def flush(self):
        try:
            self._original.flush()
        except Exception:
            pass

    def isatty(self):
        try:
            return self._original.isatty()
        except Exception:
            return False

    def __getattr__(self, name):
        # Forward anything else to the original stream
        return getattr(self._original, name)


def install_stdio_tee(debug_window):
    """Wrap sys.stdout and sys.stderr so all writes also land in `debug_window`.
    Returns the originals so caller can restore on close."""
    orig_stdout = sys.stdout
    orig_stderr = sys.stderr
    sys.stdout = _TeeStream(orig_stdout, debug_window.append_log)
    sys.stderr = _TeeStream(orig_stderr, debug_window.append_log)
    return orig_stdout, orig_stderr


def restore_stdio(orig_stdout, orig_stderr):
    sys.stdout = orig_stdout
    sys.stderr = orig_stderr


def gather_gl_info(render_window):
    """Best-effort OpenGL info from a vtkRenderWindow.

    Falls back to whatever ReportCapabilities() returns if the explicit
    vendor/renderer/version getters aren't available on this VTK build.
    """
    info = {}
    try:
        caps = render_window.ReportCapabilities() or ""
    except Exception:
        caps = ""
    for line in caps.split("\n"):
        line = line.strip()
        for prefix, label in (
            ("OpenGL vendor string", "GL Vendor"),
            ("OpenGL renderer string", "GL Renderer"),
            ("OpenGL version string", "GL Version"),
        ):
            if line.startswith(prefix):
                # split on first ':'
                _, _, value = line.partition(":")
                info[label] = value.strip()
    return info or None
