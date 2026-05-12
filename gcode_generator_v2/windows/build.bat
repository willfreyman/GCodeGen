@echo off
REM Wrapper that runs build.ps1 without tripping PowerShell's default
REM execution policy. Just double-click this file or run `.\build.bat`.
REM
REM Output: gcodegen.exe in the parent gcode_generator_v2/ folder, with
REM embedded icon + version info.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1" %*
