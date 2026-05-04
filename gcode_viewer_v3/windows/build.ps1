# Build the GcodeSimV3 viewer for Windows.
#
# What this does:
#   1. Ensure goversioninfo is installed (one-time auto-install via go install).
#   2. Generate resource_windows_*.syso into ../cmd/gcodesim/ from the
#      versioninfo.json + icon.ico that live alongside this script.
#      Go's linker picks up any .syso in the package dir on Windows builds,
#      so the next `go build` embeds the icon + version info into the exe.
#   3. Build gcodesim.exe with stripped symbols and the GUI subsystem flag
#      (no console window when launched from Explorer).
#
# Run from gcode_viewer_v3/windows/:
#   .\build.ps1                  (or .\build.bat to bypass execution policy)
#
# Output: gcodesim.exe (with icon, version info, no console) at the parent
# gcode_viewer_v3/ folder — same place older builds put it, so the
# `gh release upload` recipe in the README still works unchanged.

$ErrorActionPreference = 'Stop'

# Anchor every relative path to the project root (gcode_viewer_v3/), one
# level above this script. Avoids "what was the cwd when they ran me?"
# fragility — `go build`, `go mod tidy`, and `git describe` all want to
# run from the module root.
$WinDir   = $PSScriptRoot
$ProjRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
Set-Location $ProjRoot

# ── 1. Make sure goversioninfo is on PATH (install on first run) ─────────
function Get-GoBin {
    # `go env GOBIN` is empty unless explicitly set; fall back to GOPATH/bin
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
        # PATH not refreshed in this session — add it ad-hoc
        $env:PATH = "$goBin;$env:PATH"
    }
    if (-not (Get-Command goversioninfo -ErrorAction SilentlyContinue)) {
        Write-Host "goversioninfo install failed. Check '$goBin' contains goversioninfo.exe." -ForegroundColor Red
        exit 1
    }
}

# Sync module dependencies. Catches imports that were added since the
# last build (e.g. golang.org/x/sys/windows/registry on the
# --register-file-types path) — `go build` alone won't auto-fetch new
# top-level modules, only transitive ones already in go.sum.
go mod tidy

# ── 2. Generate resource_windows_*.syso for the icon + version metadata ─
# Use -platform-specific so the .syso files are named resource_windows_amd64.syso
# (Windows-only); cross-builds for other OSes won't try to link them.
#
# We Push-Location into cmd/gcodesim/ because Go's linker only finds .syso
# files that sit in the package directory. The icon + JSON inputs are
# passed as absolute paths so we can read them from windows/ regardless
# of cwd.
Push-Location .\cmd\gcodesim
try {
    goversioninfo -platform-specific=true -icon "$WinDir\icon.ico" "$WinDir\versioninfo.json"
} finally {
    Pop-Location
}

# ── 3. Build ────────────────────────────────────────────────────────────
# Inject the most recent git tag into internal/version.Version so the
# running binary can compare itself against the GitHub Releases API.
#
# We use --abbrev=0 (latest tag, NOT the offset-from-tag form) so a build
# from HEAD that's one commit past v3.0.1 still stamps as "v3.0.1" rather
# than "v3.0.1-1-gabc1234". The cluttered offset-form was confusing the
# update check — the local stamp didn't match the release tag, so the
# binary thought there was an update available even when running the
# very build it was checking against.
$gitVersion = (git describe --tags --abbrev=0 2>$null)
if (-not $gitVersion) { $gitVersion = "dev" }

$ldflags = "-s -w -H windowsgui -X gcodegen.local/viewer/internal/version.Version=$gitVersion"
go build -ldflags="$ldflags" -trimpath -o gcodesim.exe .\cmd\gcodesim

if (Test-Path gcodesim.exe) {
    $size = (Get-Item gcodesim.exe).Length / 1MB
    Write-Host ("Built gcodesim.exe ({0:N1} MB), version={1}" -f $size, $gitVersion) -ForegroundColor Green
} else {
    Write-Host "Build failed -- gcodesim.exe not produced." -ForegroundColor Red
    exit 1
}
