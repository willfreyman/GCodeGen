@echo off
REM Wrapper that runs build.ps1 without tripping PowerShell's default
REM execution policy (which blocks unsigned .ps1 files on most Windows
REM installs). Just double-click this file or run `.\build.bat`.
REM
REM Output: gcodesim.exe in this folder, with embedded icon + version info.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1" %*
