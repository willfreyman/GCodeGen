# Build the GcodeSimV3 viewer for Windows.
#
# What this does:
#   1. Ensure goversioninfo is installed (one-time auto-install via go install).
#   2. Generate resource_windows_*.syso from cmd/gcodesim/versioninfo.json.
#      Go's linker picks up any .syso in a package dir on Windows builds, so
#      the next `go build` embeds the icon + version info into the exe.
#   3. Build gcodesim.exe with stripped symbols and the GUI subsystem flag
#      (no console window when launched from Explorer).
#
# Run from gcode_viewer_v3/:
#   .\build.ps1
#
# Output: gcodesim.exe (with icon, version info, no console) in this folder.

$ErrorActionPreference = 'Stop'

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

# ── 2. Generate resource_windows_*.syso for the icon + version metadata ─
# Use -platform-specific so the .syso files are named resource_windows_amd64.syso
# (Windows-only); cross-builds for other OSes won't try to link them.
Push-Location .\cmd\gcodesim
try {
    goversioninfo -platform-specific=true
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
