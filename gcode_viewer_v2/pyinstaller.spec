# PyInstaller spec for GcodeSimV1
#
# Build (from inside gcode_viewer_v2/):
#   pyinstaller pyinstaller.spec --noconfirm --clean
#
# Output: dist/GcodeSimV1.exe (~180-220 MB self-contained, no external runtime
# deps beyond the OpenGL driver — see README.md for details).
#
# Notes on size trimming:
# - VTK ships ~150 MB of compiled modules; we exclude the ones a CNC viewer
#   doesn't need (DICOM I/O, FreeType extras, web infrastructure, charts, etc.).
#   This typically takes the bundle from ~300 MB down to ~180-220 MB.
# - PyQt5 also ships QML, WebEngine, NetworkManager, etc. Most are unused.
# - Trim aggressively, then verify the .exe still launches on a clean Windows
#   machine. If something breaks, remove the offending entry from `excludes`.

# -*- mode: python ; coding: utf-8 -*-
import os
import sys

block_cipher = None

# Ensure the project root is on sys.path *during the spec's own analysis* so
# `collect_submodules('gcode_viewer_v2')` below can actually resolve the
# package. Without this, PyInstaller silently bundles the entry script alone
# and the .exe explodes at runtime with "No module named 'gcode_viewer_v2'".
_SPEC_DIR = os.path.dirname(os.path.abspath(SPEC))
_PROJECT_ROOT = os.path.dirname(_SPEC_DIR)
if _PROJECT_ROOT not in sys.path:
    sys.path.insert(0, _PROJECT_ROOT)

from PyInstaller.utils.hooks import collect_submodules

# VTK modules we ship by name (any not listed get auto-collected by PyInstaller's
# vtk hook). Excluding by name is safer than excluding by glob.
VTK_EXCLUDES = [
    # Document / web / network — we read .nc files only
    'vtkIOPDAL', 'vtkIOPostgreSQL', 'vtkIOMySQL', 'vtkIOODBC',
    'vtkIOXdmf2', 'vtkIOXdmf3', 'vtkIONetCDF', 'vtkIOExodus',
    'vtkIOH5part', 'vtkIOH5Rage', 'vtkIOPIO', 'vtkIOSegY',
    'vtkIOMINC', 'vtkIOLSDyna', 'vtkIOEnSight', 'vtkIOCityGML',
    'vtkIOCONVERGECFD', 'vtkIOCGNSReader', 'vtkIOPLY',  # keep PLY if you want PLY export
    'vtkIOFLUENTCFF', 'vtkIOFFMPEG', 'vtkIOMovie',
    # Heavy visualization modules we don't use
    'vtkRenderingOSPRay', 'vtkRenderingRayTracing',  # raytracing — Day 5+ feature
    'vtkRenderingVolumeAMR', 'vtkRenderingVolumeOpenGL2',
    'vtkChartsCore', 'vtkViewsContext2D', 'vtkViewsInfovis',
    'vtkInfovisCore', 'vtkInfovisLayout', 'vtkInfovisBoost',
    'vtkGeovisCore', 'vtkDomainsChemistry', 'vtkDomainsParallelChemistry',
    'vtkRenderingMatplotlib',
    # MPI / parallel — single-process app
    'vtkParallelCore', 'vtkParallelMPI', 'vtkParallelDIY',
    'vtkFiltersParallel', 'vtkFiltersParallelDIY2', 'vtkFiltersParallelGeometry',
    'vtkIOParallel', 'vtkIOParallelXML', 'vtkIOParallelExodus',
    # Java/Tcl/Python wrappers we don't need at runtime
    'vtkWrappingPython', 'vtkWrappingTools', 'vtkTestingCore',
]

# PyQt5 — exclude unused modules
QT_EXCLUDES = [
    'PyQt5.QtWebEngine', 'PyQt5.QtWebEngineCore', 'PyQt5.QtWebEngineWidgets',
    'PyQt5.QtWebChannel', 'PyQt5.QtWebSockets',
    'PyQt5.QtQml', 'PyQt5.QtQuick', 'PyQt5.QtQuickWidgets', 'PyQt5.QtQuick3D',
    'PyQt5.QtMultimedia', 'PyQt5.QtMultimediaWidgets',
    'PyQt5.QtBluetooth', 'PyQt5.QtNfc', 'PyQt5.QtPositioning',
    'PyQt5.QtSerialPort', 'PyQt5.QtSensors', 'PyQt5.QtTest',
    'PyQt5.QtSql', 'PyQt5.QtXml', 'PyQt5.QtHelp', 'PyQt5.QtDesigner',
    'PyQt5.Qt3DCore', 'PyQt5.Qt3DRender', 'PyQt5.Qt3DInput',
    'PyQt5.Qt3DLogic', 'PyQt5.Qt3DAnimation', 'PyQt5.Qt3DExtras',
]

# Stdlib / general — these tend to balloon the bundle for no reason
GENERAL_EXCLUDES = [
    'tkinter',          # we no longer ship the Tk viewer in this exe
    'matplotlib',       # pulled in by vtk wheel but we don't use it
    'pandas', 'scipy',  # not used here
    'IPython', 'jupyter', 'notebook',
    'pytest', 'unittest', 'doctest',
    'curses', '_curses',
    'sqlite3', 'dbm',
    'lib2to3', 'distutils.command',
]

# pathex tells PyInstaller's analyzer where to look for modules:
#   • spec dir          → for the entry script app.py
#   • spec dir's parent → so `gcode_viewer_v2.ui.main_window` resolves
#                         (parent contains the gcode_viewer_v2 package).
a = Analysis(
    ['app.py'],
    pathex=[_SPEC_DIR, _PROJECT_ROOT],
    binaries=[],
    # Ship the splash image as a data file so the Qt splash can find it after
    # the .exe extracts itself. PyInstaller's own pre-Python splash uses the
    # same image via the Splash() block below.
    datas=[
        ('assets/splash.png', 'assets'),
    ],
    # Force-bundle every submodule of gcode_viewer_v2. PyInstaller's static
    # analyzer doesn't always discover package members imported through an
    # entry-script bootstrap, so listing them via collect_submodules is the
    # belt-and-suspenders fix.
    hiddenimports=[
        *collect_submodules('gcode_viewer_v2'),
        'vtkmodules.qt.QVTKRenderWindowInteractor',
        'vtkmodules.util.numpy_support',
    ],
    hookspath=[],
    runtime_hooks=[],
    excludes=GENERAL_EXCLUDES + QT_EXCLUDES + VTK_EXCLUDES,
    win_no_prefer_redirects=False,
    win_private_assemblies=False,
    cipher=block_cipher,
    noarchive=False,
)

pyz = PYZ(a.pure, a.zipped_data, cipher=block_cipher)

# Pre-Python splash: shown by PyInstaller's bootloader during the
# self-extraction phase (the part that happens BEFORE Python starts).
# This is the most-noticeable boot delay for the .exe — without this,
# users see nothing for several seconds after double-click.
splash = Splash(
    'assets/splash.png',
    binaries=a.binaries,
    datas=a.datas,
    text_pos=(20, 200),
    text_size=10,
    text_color='#a0c8d0',
    minify_script=True,
    always_on_top=True,
)

exe = EXE(
    pyz,
    a.scripts,
    splash,                 # the Splash() block above
    splash.binaries,        # extra runtime binaries needed by pyi_splash
    a.binaries,
    a.zipfiles,
    a.datas,
    [],
    name='GcodeSimV1',
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,            # upx-compress the .exe; huge size win, slight startup cost
    upx_exclude=[
        # UPX-compressed Qt DLLs sometimes cause runtime errors on Windows;
        # exclude the most error-prone ones if you hit issues.
        'Qt5Core.dll', 'Qt5Gui.dll',
    ],
    runtime_tmpdir=None,
    console=False,       # no console window — pure GUI
    icon='../icon.ico',  # reuse the existing app icon
)
