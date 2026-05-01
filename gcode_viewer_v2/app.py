"""Entry point for the v2 viewer.

Boot sequence is split deliberately so the user sees a window quickly:

  1. Import only PyQt5 (light) and create QApplication.
  2. Show QSplashScreen with the splash.png asset.
  3. processEvents() — splash actually paints to screen.
  4. Dismiss PyInstaller's bundled splash (if running as a frozen .exe).
  5. THEN import main_window — which transitively imports VTK (slow, ~1-2 sec).
  6. Construct MainWindow.
  7. Splash finishes when the main window is ready.

Run with:
    python -m gcode_viewer_v2.app
or directly:
    python gcode_viewer_v2/app.py
"""

import os
import sys
import time

# Make the gcode_viewer_v2 package importable from this entry script in
# both modes:
#
#   • Source: `python gcode_viewer_v2/app.py`  →  __file__ is
#     <project>/gcode_viewer_v2/app.py, so the package's parent is two
#     levels up.
#
#   • Frozen by PyInstaller (--onefile)  →  __file__ is at sys._MEIPASS,
#     and PyInstaller extracts the bundled package as
#     <_MEIPASS>/gcode_viewer_v2/ — so _MEIPASS itself is the parent of
#     the package, NOT _MEIPASS's parent.
#
# The earlier code computed `os.path.dirname(os.path.dirname(__file__))`
# unconditionally, which gave the right answer in source mode but pointed
# OUTSIDE _MEIPASS in frozen mode — hence
# "ModuleNotFoundError: No module named 'gcode_viewer_v2'".
if __name__ == "__main__" and __package__ in (None, ""):
    if getattr(sys, "frozen", False) and hasattr(sys, "_MEIPASS"):
        pkg_root = sys._MEIPASS
    else:
        pkg_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    if pkg_root not in sys.path:
        sys.path.insert(0, pkg_root)
    __package__ = "gcode_viewer_v2"


def _splash_image_path():
    """Locate splash.png whether running from source or a PyInstaller bundle."""
    here = os.path.dirname(os.path.abspath(__file__))
    # When frozen by PyInstaller, datas land under sys._MEIPASS
    base = getattr(sys, "_MEIPASS", here)
    candidates = [
        os.path.join(base, "assets", "splash.png"),
        os.path.join(here, "assets", "splash.png"),
    ]
    for c in candidates:
        if os.path.exists(c):
            return c
    return None


def main():
    # Step 1-2: lightweight imports + QApplication + splash before anything else
    from PyQt5 import QtWidgets, QtCore, QtGui

    app = QtWidgets.QApplication(sys.argv)

    splash_path = _splash_image_path()
    if splash_path:
        pix = QtGui.QPixmap(splash_path)
    else:
        # Fallback: draw a minimal splash on the fly so we never silently fail
        pix = QtGui.QPixmap(460, 220)
        pix.fill(QtGui.QColor(20, 22, 38))
        p = QtGui.QPainter(pix)
        p.setPen(QtGui.QColor(220, 230, 255))
        p.setFont(QtGui.QFont("Consolas", 14, QtGui.QFont.Bold))
        p.drawText(pix.rect(), QtCore.Qt.AlignCenter,
                   "CNC G-Code Viewer\nLoading…")
        p.end()

    splash = QtWidgets.QSplashScreen(pix, QtCore.Qt.WindowStaysOnTopHint)
    splash.show()
    splash.showMessage("Starting…",
                       QtCore.Qt.AlignBottom | QtCore.Qt.AlignHCenter,
                       QtGui.QColor(180, 200, 230))
    # Force the splash to paint before we block on heavy imports
    app.processEvents()

    # Step 4: hand off from PyInstaller's pre-Python splash (if any)
    try:
        import pyi_splash  # only present when running as a frozen .exe
        pyi_splash.update_text("Loading 3D engine…")
        pyi_splash.close()
    except ImportError:
        pass

    # Step 5: heavy import — this is what makes cold starts slow (~1-2 sec).
    # Use an ABSOLUTE import (not `from .ui...`) so PyInstaller's static
    # analyzer can resolve the dotted path against the package on pathex
    # without depending on the runtime `__package__` patch above. Relative
    # imports in an entry script are not seen by PyInstaller and won't be
    # bundled — that's how the previous "No module named 'gcode_viewer_v2'"
    # came about.
    splash.showMessage("Loading 3D engine (VTK)…",
                       QtCore.Qt.AlignBottom | QtCore.Qt.AlignHCenter,
                       QtGui.QColor(180, 200, 230))
    app.processEvents()
    from gcode_viewer_v2.ui.main_window import MainWindow

    # Step 6: construct main window (imports done; this is fast)
    splash.showMessage("Initializing window…",
                       QtCore.Qt.AlignBottom | QtCore.Qt.AlignHCenter,
                       QtGui.QColor(180, 200, 230))
    app.processEvents()
    win = MainWindow()
    win.show_and_initialize()

    # Step 7: dismiss splash once the main window is up
    splash.finish(win)

    return app.exec_()


if __name__ == "__main__":
    sys.exit(main())
