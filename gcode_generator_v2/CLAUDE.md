# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Scope of this file

This is the **`gcode_generator_v2/`** subtree — the active Go port of the
Tkinter editor (`gcodegen.py`). The repo-root `CLAUDE.md` one level up
covers the wider monorepo (all five apps, cross-cutting decisions,
deferred work). Read that first for context this file deliberately does
NOT repeat: module-vs-monorepo layout, why Ebiten + ebitenui, why
subprocess multi-window, the polygon-renderer deferral, etc.

This file focuses on **package-level boundaries and conventions inside
this module** that a future Claude needs to know when editing here.

## Commands (run from this directory)

```
go run ./cmd/gcodegen              # editor (main window — 1024×600)
go run ./cmd/gcodegen sim          # toolpath simulation subprocess (reads JSON from stdin)
go run ./cmd/gcodegen preview      # finished-product preview subprocess (reads JSON from stdin)
go test ./internal/gen/...         # 25 unit tests — model, coords, emit (golden), ipc
go test ./internal/gen -run TestEmit_SingleStroke   # single test
cd windows && .\build.bat          # Windows release → ../gcodegen.exe
cd mac && ./build.sh               # macOS release → ../GcodeGenV1.app
```

The `sim` and `preview` modes are normally launched by the editor as
subprocesses — running them manually is useful when debugging IPC
shape, but they sit waiting for `UpdateMessage` JSON on stdin and show
an empty canvas until they get one.

## Module dispatch

`cmd/gcodegen/main.go` is a 39-line switch on `os.Args[1]`:

| `os.Args[1]` | Calls            | Package            |
| ------------ | ---------------- | ------------------ |
| (empty)      | `editor.Run()`   | `internal/editor`  |
| `sim`        | `sim.Run()`      | `internal/sim`     |
| `preview`    | `preview.Run()`  | `internal/preview` |

Ebiten can only call `RunGame` once per process, so each window is a
re-exec of the same binary. The editor calls `os.Executable()` →
`exec.Command(exe, mode)` in `internal/editor/childproc.go`.

## Package boundaries

```
internal/
├── gen/        ← pure data model + G-code emitter + IPC wire types. NO UI deps.
├── img/        ← image load + threshold + contour/centerline trace + RDP simplify. NO UI deps.
├── editor/     ← main editor Ebiten game. Imports gen + img + shared.
├── sim/        ← sim subprocess Ebiten game. Imports gen + shared + assets.
├── preview/    ← preview subprocess Ebiten game. Imports gen + shared + assets.
├── shared/     ← diaglog, crashlog, settings, redirect_*. Importable from all UI pkgs.
├── assets/     ← //go:embed FreeSansBold.ttf — single source of truth for the font.
└── version/    ← build-time version stamp + GitHub release check.
```

The `gen` package is the **stable contract**. `editor` mutates a
`gen.Editor`, calls `gen.Emit` for export, and ships `gen.UpdateMessage`
snapshots over pipes. `sim` and `preview` parse `gen.UpdateMessage` and
re-render. None of the UI packages import each other — they only share
through `gen`.

## The IPC contract (`internal/gen/ipc.go`)

Newline-delimited JSON over pipes (editor stdout → child stdin).
`UpdateMessage.Kind` is the discriminator:

- `"state"` — full snapshot (strokes, perim, origin, machine). Used on
  startup and after every editor mutation.
- `"play"` / `"pause"` / `"reset"` — sim playback control.
- `"shutdown"` — graceful exit (EOF also triggers exit).

**Important:**

- The Scanner buffer is bumped to **4 MiB** in `ReadMessages` because
  a full snapshot with many strokes can exceed Go's default 64 KB line
  limit. Don't shrink it back.
- `EncodeMessage` calls `json.Encoder.Encode` which appends `\n`.
  Don't add an extra newline — `Scanner` splits on `\n` and an empty
  line will parse as `{}` and silently drop the message.
- The editor broadcasts every **3 frames (~20 Hz)** via
  `Game.broadcastState`, not every frame. Subprocesses use
  `atomic.Pointer[UpdateMessage]` + `Swap` so they always render the
  latest snapshot and drop intermediate ones.
- The editor auto-spawns the preview at startup (mirrors
  `gcodegen.py`'s `window.after(120, open_preview)`). The sim is
  spawned on demand by the Simulate button.

## The `Emit` byte-identity invariant

`gen.Emit` must produce **byte-for-byte identical output** to
`gcodegen.py`'s `generate_gcode` (lines 380-405) for the same inputs.
The golden tests in `internal/gen/emit_test.go` pin this against
`testdata/perim_only.nc`, `single_stroke.nc`, `three_strokes.nc`.

Pitfalls that have broken these tests before:

- **Comment lines use `pyFloat`, not `%g` or `%.3f`.** `pyFloat`
  reproduces Python's `str(float)` — `-1.0` → `"-1.0"` (not `"-1"`),
  `1.5` → `"1.5"`. Go's `strconv.FormatFloat('g', -1)` drops the
  trailing `.0`; `pyFloat` appends it back when the formatted value
  has no `.`/`e`/`E`.
- **G0/G1 coords use `"%.3f"`** (always 3 decimals).
- **Feed rates and RPM are truncated to `int`**, not rounded.
- The file ends with `M30\n` — no extra trailing newline.

If you touch `emit.go`, run `go test ./internal/gen -run TestEmit` and
read the diff line-by-line if it fails. Don't "fix" the golden file —
fix the formatter.

## `internal/editor` conventions

- `Game.Update` is the per-frame entry: input → ebitenui → op-list
  refresh check → 3-frame-throttled broadcast.
- **Op-list rebuild is gated by two signals** — `len(Strokes) !=
  prevStrokeCount` (add/delete) OR `strokesDirty` (reorder, rename,
  recolor). Set `g.strokesDirty = true` after any in-place stroke
  mutation; the watch in `Update` clears it.
- **Live widget refs** (`safeZInput`, `feedXYInput`, `feedZInput`,
  `rpmInput`, `opListContainer`, `opListInner`) are kept on `Game`
  because preset-button handlers and stroke add/delete need to mutate
  widgets that live outside their construction site. Don't try to
  funnel these through ebitenui events — there isn't a clean API for
  it.
- **Child-process pointers are guarded by `procMu`** because the
  wait-for-exit goroutine in `waitForExit` clears them when the child
  dies. The respawn-on-click pattern (`if g.simProc == nil { spawn }`)
  is correct because `procMu.Lock` serializes the nil-check + assign.
- Input is **polled, not event-driven** — see `input.go`. Ebiten only
  exposes `inpututil.IsMouseButtonJustPressed/Released` and current
  cursor pos; the press → drag → release state machine is rebuilt
  per-frame from those primitives.

## `internal/sim` conventions

Two-image layered render — don't merge them:

- **`ghosts`** is rebuilt only when the message reports a structural
  change (`State.loadFromMessage` returns `true` on op-list diff). All
  unanimated path overlays go here.
- **`trails`** is **append-only during playback**. `playback.step()`
  returns the new segments to paint; `paintToTrails` blits them onto
  the persistent image. Never clear `trails` mid-playback — that's why
  it survives across frames at 60 fps without re-rendering every cut.

`Game.applyMessage` calls `state.reset()` + `ghosts.Clear()` +
`trails.Clear()` ONLY on structural change so a steady 20 Hz update
burst (e.g. user dragging the origin dot) doesn't restart playback.

## `internal/preview` conventions

The expensive surface render is gated by a `dirty` flag:

- Any state mutation (incoming `UpdateMessage`, bit-slider change,
  material-button click) sets `g.dirty = true`.
- `Update()` reads-and-clears `dirty` and calls `rerender()` at most
  once per frame.

The preview is intentionally slower-to-redraw than the sim (renders the
material surface texture, not just line strokes) so the dirty flag
matters more here.

## `internal/shared` conventions

- **Every subprocess `Run()` must start with**:
  ```go
  shared.DiagInit("editor")  // or "sim", "preview"
  defer shared.RecoverAndLog("editor")
  ```
  `DiagInit` opens `<UserConfigDir>/GcodeGen/diag_<mode>.log` (truncated
  per launch) and redirects stderr to `diag_<mode>.log.stderr` — this
  is **load-bearing** because the .exe runs with no console window on
  Windows, so without the redirect, runtime panics + cgo crashes
  disappear silently when users double-click. `RecoverAndLog` catches
  Go panics that bypass ebiten's loop and writes them to the same
  file.
- `GCODEGEN_DIAG=0` disables the diagnostic log (useful during tests).
- `redirect_windows.go` uses `windows.SetStdHandle` to dup stderr to
  the file; `redirect_other.go` is a no-op stub. The build-tagged
  split is the only place this module reaches platform-specific.
- `settings.go` persists user prefs (skipped update versions, last
  material choice, etc.) to `<UserConfigDir>/GcodeGen/settings.json`.

## Adding a feature

1. Add fields to `gen.Editor` (and `gen.UpdateMessage` if subprocesses
   need to see them). Update `SnapshotState` to wire them in.
2. Add tests under `internal/gen` for any new pure logic, especially
   anything touching `Emit`.
3. Wire the editor UI in `internal/editor/panel.go` (right panel) or
   `input.go` (canvas interactions). Set `strokesDirty` if the op
   list needs to refresh.
4. Consume the new field in `internal/sim/playback.go` (if it affects
   simulation) and/or `internal/preview/render.go` (if it affects the
   surface render).

The editor never imports `sim` or `preview` directly — it talks to
them through `UpdateMessage` only. Keep that boundary.
