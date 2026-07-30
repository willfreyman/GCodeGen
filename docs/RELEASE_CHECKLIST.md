# GcodeSimV3 Release Checklist

Every step, in order, for shipping a new version on both Windows and macOS.

---

## 1. Pre-flight — what to check before tagging

- [ ] All changes committed and pushed to `main` on GitHub
- [ ] `go test ./internal/parser/...` passes (parser golden tests)
- [ ] `go test ./internal/scene/...` passes (scene tests)
- [ ] `go vet ./...` is clean
- [ ] The 6 bundled tutorials still load and behave right (manual check in the app)
- [ ] If this is a new feature: `docs/MANUAL.md` is updated to cover it
- [ ] All in-progress work from `CLAUDE.md` "Deferred work" section is either done or re-acknowledged

---

## 2. Bump the version metadata (`windows/versioninfo.json`)

This file controls what users see when they right-click the .exe → Properties → Details.

**File:** `gcode_viewer_v3/windows/versioninfo.json`

Two sets of fields to update:

```json
// Struct fields (lines 3-4):
"FileVersion":    {"Major": 3, "Minor": 1, "Patch": 0, "Build": 0},
"ProductVersion": {"Major": 3, "Minor": 1, "Patch": 0, "Build": 0},

// String fields (lines 15, 22):
"FileVersion":      "3.1.0.0",
"ProductVersion":   "3.1.0.0",
```

Bump all four every release. Commit this change.

### Also bump the macOS bundle (`mac/Info.plist`)

Easy to forget — it drifted from `3.0` all the way to the v3.2.0 release
before anyone noticed. Two keys, both the plain version:

```xml
<key>CFBundleVersion</key>
<string>3.2.0</string>
<key>CFBundleShortVersionString</key>
<string>3.2.0</string>
```

This is what Finder shows under Get Info. Unlike the Windows metadata it is
NOT generated from the git tag, so it only changes when you change it.

---

## 3. Create the git tag

Tags must be pushed to GitHub — the build scripts read `git describe --tags --abbrev=0` to stamp the version into the binary, and the app's update checker compares against GitHub release tags.

```powershell
# Create a local, signed tag (annotated):
git tag -a v3.1.0 -m "GcodeSimV3 v3.1.0"

# Push to GitHub:
git push origin v3.1.0
```

**Tag naming convention:** `v3.x.x` (semver with leading `v` prefix).

---

## 4. Create the GitHub Release

```powershell
gh release create v3.1.0 `
  --title "GcodeSim v3.1.0" `
  --notes "What's new in this release:

- New feature 1: ...
- New feature 2: ...
- Bugfix: ...
"
```

This creates the release shell on GitHub. Binaries will be uploaded next.

---

## 5. Build Windows binary

### What the build script does

`gcode_viewer_v3/windows/build.ps1` (invoked via `build.bat`):

1. Installs `goversioninfo` (one-time) — a Go tool that embeds the `.ico` icon and version metadata from `versioninfo.json` into the .exe as Windows resources
2. Runs `go mod tidy` — syncs `go.sum` with any new dependencies
3. Generates `resource_windows_amd64.syso` into `cmd/gcodesim/` — Go's linker auto-picks up `.syso` files in the package directory on Windows builds
4. Runs `git describe --tags --abbrev=0` to get the current tag (e.g. `v3.1.0`)
5. Runs `go build` with these flags:
   - `-s -w` — strip debug symbols (smaller binary)
   - `-H windowsgui` — no console window when launched from Explorer
   - `-X gcodegen.local/viewer/internal/version.Version=v3.1.0` — stamps the version string so the app's update checker works
   - `-trimpath` — removes local filesystem paths from the binary

### Run it

```powershell
cd gcode_viewer_v3\windows
.\build.bat
```

Output: `gcode_viewer_v3\gcodesim.exe` (~5 MB)

### Verify

- [ ] `.exe` was produced (`Test-Path ..\gcodesim.exe`)
- [ ] **Right-click → Properties → Details** shows the correct version number
- [ ] **Double-click the .exe** — app launches without a console window
- [ ] **Help → About** (or the title bar) shows the correct version
- [ ] It loads a `.nc` file and renders correctly

---

## 6. Build macOS binary (must run on a Mac)

### What the build script does

`gcode_viewer_v3/mac/build.sh`:

1. Runs `go mod tidy`
2. Gets the git tag via `git describe --tags --abbrev=0`
3. If `lipo` is available: builds both `arm64` and `amd64` binaries separately, then merges them into a **universal binary** (one .exe that runs natively on both Apple Silicon and Intel Macs)
4. Falls back to a single host-arch build if `lipo` is missing
5. Converts `icon.ico` → `icon.icns` (macOS icon format) using `sips` + `iconutil`
6. Bundles into `GcodeSimV3.app/Contents/MacOS/gcodesim`
7. Packages into `GcodeSimV3.app.zip` for distribution (the raw .app folder is a directory; zipping preserves it for download/email)
8. Registers the app with Launch Services so `.nc` / `.gcode` / `.tap` files associate with GcodeSimV3

### Run it

```bash
cd gcode_viewer_v3/mac
./build.sh
```

Outputs:
- `gcode_viewer_v3/GcodeSimV3.app` (double-clickable)
- `gcode_viewer_v3/GcodeSimV3.app.zip` (share this one)

### Verify

- [ ] `.app` was produced
- [ ] `.app.zip` was produced
- [ ] Double-click `.app` — launches correctly
- [ ] First launch: right-click → Open (Gatekeeper bypass, one-time)
- [ ] `.nc` files open in GcodeSimV3 by default (not TextEdit)

---

## 7. Upload assets to the release

```powershell
# Windows
cd gcode_viewer_v3
gh release upload v3.1.0 gcodesim.exe --clobber

# macOS (run from the Mac, or transfer the .zip and upload from Windows)
gh release upload v3.1.0 GcodeSimV3.app.zip --clobber
```

`--clobber` lets you re-upload if you need to rebuild and re-upload the same version. Omit it if you want uploads to fail on duplicates.

---

## 8. Post-release verification

- [ ] Visit `https://github.com/willfreyman/GCodeGen/releases/tag/v3.1.0`
- [ ] Both `.exe` and `.app.zip` appear as downloadable assets
- [ ] Download the `.exe` on a clean Windows machine — it launches
- [ ] Download the `.app.zip` on a clean Mac — it unzips and launches

---

## What gets a version number

Only the **v3 viewer** (`gcode_viewer_v3/`) gets tagged releases. The v2 viewer (`gcode_viewer_v2/`) and editor (`gcodegen.py` / `gcode_generator_v2/`) use version numbers from their own build systems:

| App | Binary | Version source |
|---|---|---|
| GcodeSimV3 (Go viewer) | `gcodesim.exe` / `GcodeSimV3.app` | Git tag `v3.x.x` |
| GcodeSimV1 (Python viewer) | `GcodeSimV1.exe` | PyInstaller build, not tagged |
| gcodegen (Python editor) | `gcodegenV1.0.exe` | Committed directly, not tagged |
| gcodegen v2 (Go editor) | `gcodegen.exe` | Separate tagging TBD |

---

## Troubleshooting

### `goversioninfo` can't be found after install
The build script appends `$(go env GOPATH)/bin` to PATH if needed. If it still fails:
```powershell
go env GOPATH   # find where Go installs binaries
# Add that path\bin to your system PATH, or run build again (the script retries)
```

### "This app can't run on your PC"
Rarely, a missing CGo dependency. Run from PowerShell to see the actual error:
```powershell
.\gcodesim.exe
```
If it mentions a missing DLL, you need TDM-GCC or MSYS2 mingw-w64 installed for CGo.

### macOS: "unidentified developer"
Right-click the `.app` → Open → confirm. One-time per machine.

### Version in the app doesn't match the tag
The build script reads `git describe --tags --abbrev=0`. Make sure you:
1. Created the tag locally (`git tag -a v3.x.x -m "..."`)
2. Pushed it to GitHub (`git push origin v3.x.x`)
3. Re-ran the build script *after* the tag was created locally
