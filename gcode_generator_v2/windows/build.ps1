# Build the GcodeGenV1 editor for Windows.
#
# Parallel of gcode_viewer_v3/windows/build.ps1 — same toolchain steps,
# different binary (gcodegen.exe) from a separate Go module.
#
# Run from gcode_generator_v2/windows/:
#   .\build.ps1                  (or .\build.bat to bypass execution policy)
#
# Output: gcodegen.exe at the parent gcode_generator_v2/ folder.

$ErrorActionPreference = 'Stop'

$WinDir   = $PSScriptRoot
$ProjRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
Set-Location $ProjRoot

# ── 1. Make sure goversioninfo is on PATH (install on first run) ─────────
function Get-GoBin {
    $gobin = (& go env GOBIN).Trim()
    if (-not $gobin) {
        $gopath = (& go env GOPATH).Trim()
        $gobin = Join-Path $gopath 'bin'
    }
    return $gobin
}

if (-not (Get-Command goversioninfo -ErrorAction SilentlyContinue)) {
    Write-Host "Installing goversioninfo (one-time)..." -ForegroundColor Yellow
    go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
    $goBin = Get-GoBin
    if (-not (Get-Command goversioninfo -ErrorAction SilentlyContinue)) {
        $env:PATH = "$goBin;$env:PATH"
    }
    if (-not (Get-Command goversioninfo -ErrorAction SilentlyContinue)) {
        Write-Host "goversioninfo install failed. Check '$goBin' contains goversioninfo.exe." -ForegroundColor Red
        exit 1
    }
}

go mod tidy

# ── 2. Generate resource_windows_*.syso for the icon + version metadata ─
# Push-Location into cmd/gcodegen/ so the .syso lands next to the package.
Push-Location .\cmd\gcodegen
try {
    goversioninfo -platform-specific=true -icon "$WinDir\icon.ico" "$WinDir\versioninfo.json"
} finally {
    Pop-Location
}

# ── 3. Build ────────────────────────────────────────────────────────────
$gitVersion = (git describe --tags --abbrev=0 2>$null)
if (-not $gitVersion) { $gitVersion = "dev" }

$ldflags = "-s -w -H windowsgui -X gcodegen.local/generator/internal/version.Version=$gitVersion"
go build -ldflags="$ldflags" -trimpath -o gcodegen.exe .\cmd\gcodegen

if (Test-Path gcodegen.exe) {
    $size = (Get-Item gcodegen.exe).Length / 1MB
    Write-Host ("Built gcodegen.exe ({0:N1} MB), version={1}" -f $size, $gitVersion) -ForegroundColor Green
} else {
    Write-Host "Build failed -- gcodegen.exe not produced." -ForegroundColor Red
    exit 1
}
