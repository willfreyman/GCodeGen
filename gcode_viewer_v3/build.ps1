# Build the GcodeSimV3 viewer.
#
# -H windowsgui   GUI subsystem (no console window when launched from Explorer)
# -s -w           strip symbol/debug tables (~30% size reduction)
# -trimpath       remove filesystem path leakage from binary
#
# Run from gcode_viewer_v3/:
#   .\build.ps1
#
# Output: gcodesim.exe in the current directory.

$ErrorActionPreference = 'Stop'

go build -ldflags='-s -w -H windowsgui' -trimpath -o gcodesim.exe .\cmd\gcodesim

if (Test-Path gcodesim.exe) {
    $size = (Get-Item gcodesim.exe).Length / 1MB
    Write-Host ("Built gcodesim.exe ({0:N1} MB)" -f $size) -ForegroundColor Green
} else {
    Write-Host "Build failed — gcodesim.exe not produced." -ForegroundColor Red
    exit 1
}
