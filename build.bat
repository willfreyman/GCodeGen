@echo off
REM ── build.bat ─────────────────────────────────────────────────────────────
REM Builds GcodeSimV1.exe from source and drops it in exe\ for distribution.
REM
REM Run from the project root:
REM     build.bat
REM
REM Requires Python 3.10+ already on PATH. The script installs the runtime
REM dependencies (vtk, PyQt5, numpy) and PyInstaller into the active Python
REM environment, then builds. If you want isolation, activate a venv first.
REM ────────────────────────────────────────────────────────────────────────

setlocal

echo.
echo ── Installing build dependencies ─────────────────────────────────────
pushd gcode_viewer_v2
pip install -r requirements.txt || goto :err
pip install pyinstaller || goto :err

echo.
echo ── Running PyInstaller ───────────────────────────────────────────────
pyinstaller pyinstaller.spec --noconfirm --clean || goto :err

if not exist dist\GcodeSimV1.exe (
    echo.
    echo [FAIL] PyInstaller finished but dist\GcodeSimV1.exe is missing.
    goto :err
)

echo.
echo ── Publishing to exe\GcodeSimV1.exe ──────────────────────────────────
if not exist ..\exe mkdir ..\exe
copy /Y dist\GcodeSimV1.exe ..\exe\GcodeSimV1.exe || goto :err

popd
for %%I in (exe\GcodeSimV1.exe) do (
    echo.
    echo [OK] exe\GcodeSimV1.exe  (%%~zI bytes^)
    echo Double-click it, or send it to a Windows user. No Python required.
)
endlocal
exit /b 0

:err
echo.
echo [FAIL] Build aborted.
popd 2>nul
endlocal
exit /b 1
