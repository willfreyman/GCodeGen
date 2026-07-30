# HoleGen — Complete Functional Specification (Go implementation target)

A reproduction spec for **HoleGen**, a desktop tool that generates CNC G-code for
milling a grid of round holes into metal tube (sized for FRC / MAXTube robotics
stock). This document describes **every feature except the graphical visualizer /
canvas preview** (explicitly out of scope).

**Implementation language: Go.** An implementation built to this spec must produce
**byte-identical G-code** to the reference output in §7.7. All arithmetic uses
IEEE-754 `float64`, so Go reproduces the reference numbers exactly. Follow every
rule literally — numeric formatting, string literals, ordering, rounding, and file
locations all matter.

Throughout, pseudocode is language-neutral; concrete API calls name the Go
standard-library package to use.

---

## 1. Technology & Packaging (Go)

- **Language:** Go (1.21+).
- **Core logic** (parsing, hole layout, G-code generation, estimation, presets)
  uses only the Go **standard library**: `fmt`, `math`, `strconv`, `strings`,
  `encoding/json`, `os`, `path/filepath`, `sort`.
- **GUI:** Go has no standard-library GUI, so use a third-party toolkit. Both of
  these give a single self-contained binary and provide the needed widgets
  (text entry, dropdown/select, buttons, file-save dialog, message dialogs):
  - **Fyne** (`fyne.io/fyne/v2`) — cross-platform, recommended. Widgets:
    `widget.Entry`, `widget.Select`, `widget.Button`, `widget.Label`; dialogs via
    the `dialog` package (`dialog.NewError`, `dialog.NewInformation`,
    `dialog.ShowConfirm`, `dialog.ShowFileSave`, `dialog.ShowCustom`/entry prompt).
  - **Walk** (`github.com/lxn/walk`) — Windows-native alternative, matches the
    Windows `.exe` deployment target closely.
  The choice of toolkit is flexible; the **logic layer must be identical** and
  should live in plain Go functions independent of the GUI so it is unit-testable
  headless.
- **Build a single binary** (no PyInstaller-style bundler needed — `go build`
  already emits one static executable):
  ```
  # Windows GUI exe with no console window:
  go build -ldflags "-H windowsgui" -o HoleGen.exe .
  ```
  (With Fyne you may instead use `fyne package -os windows` to embed an icon and
  produce `HoleGen.exe`.) Cross-compiling from Linux/macOS to Windows is
  `GOOS=windows GOARCH=amd64 go build ...` (Fyne needs a cross C toolchain such as
  the `fyne-cross` tool because it uses cgo; pure-logic builds do not).
- **Recommended layout:** put the pure logic in one package (e.g. `holegen`) with
  exported functions `HoleCenters`, `GenerateGCode`, `ParseMeasurement`,
  `EstimateRuntime`, `FormatDuration`, `LoadPresets`, `SavePresets`, and a `main`
  package for the GUI. This makes the §7.7 fixture a straightforward Go test.

---

## 2. Coordinate System & Zeroing Convention (machine setup)

The generated program assumes the operator has zeroed the machine as follows.
These are the semantic anchors the math depends on:

- **X = 0** — the position where the **edge** of the endmill just touches the
  side face of the tube. Because the tool is round, the **bit center** at that
  moment is one bit-radius away, so the **tube's left face is at X = bitDiameter/2**.
- **Y = 0** — the exact **center of the first row** of holes.
- **Z = 0** — the **top surface** of the material.
- **Z positive is up.** Cutting goes to negative Z.
- Units: **millimeters** throughout the G-code (`G21`). Absolute coordinates
  (`G90`). XY arc plane (`G17`).

---

## 3. Parameters (the 12 input fields)

Defined as an ordered list `Fields`. Order matters — it is the on-screen
top-to-bottom order **and** the iteration order for reading/saving/generating.
Reproduce EXACTLY, including label text.

Go definition:
```go
type Field struct {
    Key     string // stable identifier, also the JSON key in presets
    Label   string // exact on-screen text
    Default string // initial entry text
    IsInt   bool   // true => integer field, false => float
}

var Fields = []Field{
    {"bitDiameter",        "Bit diameter (mm)",                             "6.0",    false},
    {"targetHoleDiameter", "Target hole diameter (mm)",                     "28.57",  false},
    {"xOffset",            "X offset from tube edge to first column (mm)",  "10.0",   false},
    {"holeSpacingX",       "Horizontal spacing between holes, X (mm)",      "50.8",   false},
    {"holeSpacingY",       "Vertical spacing between holes, Y (mm)",        "50.8",   false},
    {"rowCount",           "Number of rows (Y)",                            "2",      true},
    {"columnCount",        "Number of columns (X)",                         "2",      true},
    {"spindleSpeed",       "Spindle RPM (max 24000)",                       "18000",  true},
    {"verticalFeedrate",   "Helical plunge vertical feedrate (Z)",          "150",    true},
    {"horizontalFeedrate", "Horizontal circular feedrate (XY)",             "600",    true},
    {"tubeThickness",      "Metal thickness (mm)",                          "3.0",    false},
    {"pitchPerTurn",       "Helical pitch, Z drop per 360 (mm)",            "1.0",    false},
}
```

| # | Key                  | Label (verbatim)                                | Default  | IsInt |
|---|----------------------|-------------------------------------------------|----------|-------|
| 1 | `bitDiameter`        | `Bit diameter (mm)`                             | `6.0`    | false |
| 2 | `targetHoleDiameter` | `Target hole diameter (mm)`                     | `28.57`  | false |
| 3 | `xOffset`            | `X offset from tube edge to first column (mm)`  | `10.0`   | false |
| 4 | `holeSpacingX`       | `Horizontal spacing between holes, X (mm)`      | `50.8`   | false |
| 5 | `holeSpacingY`       | `Vertical spacing between holes, Y (mm)`        | `50.8`   | false |
| 6 | `rowCount`           | `Number of rows (Y)`                            | `2`      | true  |
| 7 | `columnCount`        | `Number of columns (X)`                         | `2`      | true  |
| 8 | `spindleSpeed`       | `Spindle RPM (max 24000)`                       | `18000`  | true  |
| 9 | `verticalFeedrate`   | `Helical plunge vertical feedrate (Z)`          | `150`    | true  |
| 10| `horizontalFeedrate` | `Horizontal circular feedrate (XY)`             | `600`    | true  |
| 11| `tubeThickness`      | `Metal thickness (mm)`                          | `3.0`    | false |
| 12| `pitchPerTurn`       | `Helical pitch, Z drop per 360 (mm)`            | `1.0`    | false |

Feedrates are in **mm/min**. `spindleSpeed` is RPM. The "(max 24000)" note in the
label is **advisory text only — NOT enforced anywhere**.

**Parsed params container.** After parsing (see §4/§11) hold the values in a
struct. Integer fields are whole numbers but may be kept as `float64` for the
math; the loop bounds `rowCount`/`columnCount` must be used as `int`.
```go
type Params struct {
    BitDiameter        float64
    TargetHoleDiameter float64
    XOffset            float64
    HoleSpacingX       float64
    HoleSpacingY       float64
    RowCount           int
    ColumnCount        int
    SpindleSpeed       int
    VerticalFeedrate   int
    HorizontalFeedrate int
    TubeThickness      float64
    PitchPerTurn       float64
}
```

---

## 4. Unit Parsing — per-field inch/mm override

Every field value is parsed through this function. It lets the user type an
optional unit suffix on **any individual field**; inches are converted to mm for
that field only.

```
MM_PER_INCH = 25.4

ParseMeasurement(raw string, isInt bool) -> (float64, error):
    s = ToLower(TrimSpace(raw))
    inches = false
    if HasSuffix(s, "\""):  s = TrimSpace(s without last 1 char); inches = true
    else if HasSuffix(s, "in"): s = TrimSpace(s without last 2 chars); inches = true
    else if HasSuffix(s, "mm"): s = TrimSpace(s without last 2 chars)   // inches stays false
    value, err = ParseFloat(s, 64)      // strconv.ParseFloat; error if not numeric
    if err != nil: return 0, err
    if inches: value *= 25.4
    if isInt: value = Round(value)      // math.Round; then use as whole number
    return value, nil
```

Go specifics:
- `strings.ToLower`, `strings.TrimSpace`, `strings.HasSuffix`, and slice the
  string (e.g. `s[:len(s)-2]`).
- `strconv.ParseFloat(s, 64)` replaces Python `float()`. A parse error is the
  "invalid input" signal.
- For integer fields, apply `math.Round(value)` then convert to `int`. NOTE:
  Go's `math.Round` rounds halves **away from zero**; Python's `round` used
  banker's rounding. This only differs on exact `x.5` inputs, which do not occur
  for any realistic field value or in the §7.7 fixture, so G-code output is
  unaffected. Away-from-zero is the correct/expected choice here.

Rules & exact behavior (unchanged from original):
- Case-insensitive (string is lowercased first): `IN`, `In`, `MM` all work.
- Whitespace between number and unit allowed: `1.125 in`, `1.125in`, `1.125"`
  all yield **28.575**.
- Suffix precedence: inch mark `"` first, then `in`, then `mm`.
- No suffix → millimeters (plain float parse).
- Malformed numeric part (`""`, `"abc"`, `"in"` alone) → error.

Worked results (must match): `1.125in`→28.575, `1.125"`→28.575, `0.5IN`→12.7,
`28.585mm`→28.585, `28.585`→28.585, `2in` (int field)→51.

---

## 5. Target-Hole-Diameter Dropdown (named quick-fill presets)

Next to the `targetHoleDiameter` text entry there is a **dropdown** (Fyne
`widget.Select`, or a read-only combo) of named diameters. Selecting one **writes
its numeric string into the diameter entry** (the entry stays freely editable).

Data (ordered list of `(value_string, friendly_name)`):
```go
type DiaPreset struct{ Value, Name string }

var DiameterPresets = []DiaPreset{
    {"6.0",    "6 mm hole"},
    {"12.7",   `1/2" shaft`},
    {"28.585", "bearing hole"},
    {"50.8",   `2" hole`},
}
```
- The dropdown's option strings are `Value + " — " + Name` using an em-dash `—`
  (U+2014), e.g. `6.0 — 6 mm hole`, `28.585 — bearing hole`. In Go build these
  with `fmt.Sprintf("%s — %s", v.Value, v.Name)`.
- On selection: match the chosen option string back to its preset, then set the
  diameter entry's text to `Value` (the raw string, e.g. `"28.585"`), and refresh
  the live estimate display.
- The dropdown does not accept typed input (selection only).

---

## 6. Hole Ordering — snake (boustrophedon) pattern

Holes are cut column-by-column; alternate columns run in opposite Y direction to
minimize rapid travel.

For column index `i` in `0..columnCount-1` and row index `o` in `0..rowCount-1`:
```
center_x = (bitDiameter / 2) + xOffset + (holeSpacingX * i)

if i is even:  center_y = holeSpacingY * o
else        :  center_y = holeSpacingY * (rowCount - o - 1)
```
Nested loop: `i` (column) outer, `o` (row) inner. Even columns go bottom→top in Y;
odd columns go top→bottom. Each column's end Y equals the next column's start Y,
so between-column travel is pure X.

Go helper returns the holes in this exact order; reuse it in BOTH the generator
and the estimator (same formulas, same order):
```go
type Hole struct {
    Col, Row int
    X, Y     float64
}
func HoleCenters(p Params) []Hole {
    holes := []Hole{}
    for i := 0; i < p.ColumnCount; i++ {
        for o := 0; o < p.RowCount; o++ {
            cx := p.BitDiameter/2.0 + p.XOffset + p.HoleSpacingX*float64(i)
            var cy float64
            if i%2 == 0 {
                cy = p.HoleSpacingY * float64(o)
            } else {
                cy = p.HoleSpacingY * float64(p.RowCount-o-1)
            }
            holes = append(holes, Hole{i, o, cx, cy})
        }
    }
    return holes
}
```

---

## 7. G-code Generation — exact output specification

`GenerateGCode(p Params) (lines []string, cutRadius, totalDepth float64, err error)`.
Each element of `lines` already ends with `\n`; the final file is the plain
concatenation of all elements (no extra separators — Go: `strings.Join(lines, "")`
or write each with `io.WriteString`).

### 7.1 Derived values
```
cutRadius  = (targetHoleDiameter - bitDiameter) / 2
totalDepth = tubeThickness + 1.5        // 1.5 mm breakthrough past material
```

### 7.2 Validation (in the generator)
```
if cutRadius < 0:
    return error: "Your bit diameter cannot be larger than your target hole diameter!"
```
When `cutRadius == 0` (hole Ø == bit Ø) a special center-plunge branch is used
(see 7.5).

### 7.3 Startup / safety block (emitted once, in THIS ORDER)
```
G90 G21 G17\n            # Absolute, metric, XY plane
G0 Z5.0000\n             # Retract to safe height FIRST
M03 S{spindleSpeed}\n    # THEN start spindle (e.g. M03 S18000)
G4 P4000\n               # Dwell 4000 ms (4 s) to reach speed
```
**CRITICAL SAFETY ORDER:** the Z retract (`G0 Z5.0000`) MUST come **before** the
spindle-on `M03`. The tool must never be commanded to spin while possibly still
down in/near the material. Do not reorder.
`{spindleSpeed}` is an integer → `fmt.Sprintf("M03 S%d\n", p.SpindleSpeed)`.

### 7.4 Per-hole block (loop over holes in snake order from §6)
For each hole emit, in order:
```
\n( --- HOLE LOCATION: Col {Col+1}, Row {Row+1} --- )\n
G0 X{center_x:.4f} Y{center_y:.4f}\n     # rapid to hole XY
G0 Z1.0000\n                             # rapid down to 1 mm above material
```
Then, **if `cutRadius > 0`** (normal helical-bored hole):
```
start_arc_x = center_x + cutRadius
G1 X{start_arc_x:.4f} F{horizontalFeedrate}\n     # move out to +X circle edge

# helical descent (corkscrew): full-circle CCW arcs, each dropping pitchPerTurn
current_z = 0.0
loop while current_z > -totalDepth:
    current_z -= pitchPerTurn
    if current_z < -totalDepth: current_z = -totalDepth   # clamp final pass
    emit: G03 X{start_arc_x:.4f} Y{center_y:.4f} Z{current_z:.4f} I{-cutRadius:.4f} J0.0000 F{verticalFeedrate}\n

# flat spring pass at final depth (NO Z word) to clean the floor
G03 X{start_arc_x:.4f} Y{center_y:.4f} I{-cutRadius:.4f} J0.0000 F{horizontalFeedrate}\n

# return to center before lifting, to avoid gouging the wall
G1 X{center_x:.4f}\n
```
**Else (`cutRadius == 0`, hole == bit diameter):**
```
G1 Z{-totalDepth:.4f} F{verticalFeedrate}\n     # straight center plunge
```
Then, for every hole (both branches):
```
G0 Z5.0000\n                             # retract to safe clearance
```

Go note on the loop: use `float64` accumulation exactly as written
(`currentZ -= p.PitchPerTurn`); this matches the reference bit-for-bit.

Arc/geometry notes:
- `G03` = counter-clockwise arc. `I` is the X offset from the arc **start point**
  to the arc **center**. Start point is `center_x + cutRadius`, center is
  `center_x`, so `I = -cutRadius`. `J = 0` (`J0.0000`). Because start and end XY
  are identical, each `G03` is a **full 360° circle**.
- The helical `G03` lines include a descending `Z` word; the final spring-pass
  `G03` has **no** `Z` word (stays at final depth).
- Number of helical passes = `ceil(totalDepth / pitchPerTurn)`, final pass clamped
  exactly to `-totalDepth`. Example: totalDepth 4.5, pitch 1.0 → passes at
  Z = -1, -2, -3, -4, -4.5 (5 passes).
- The first helical pass drops from the current Z `+1.0` (from `G0 Z1.0000`) to
  `Z-1.0000` — a 2 mm effective descent on the first turn (upper 1 mm is air).
  Preserve this behavior.

### 7.5 End-of-program block (emitted once)
```
\n( --- END OF PROGRAM --- )\n
M05\n            # spindle off
G0 X0 Y0\n       # return home for unloading
M30\n            # program end / reset
```

### 7.6 Numeric formatting rules (must match exactly)
- Coordinates X, Y, Z and arc I/J: **4 decimal places** → Go `%.4f`.
  Examples: `X13.0000`, `Z-4.5000`, `I-11.2850`, `J0.0000`.
- Feedrates and spindle speed: **integer, no decimals** → Go `%d`
  (`F600`, `F150`, `S18000`).
- Home return uses bare integers: literal `G0 X0 Y0` (NOT `X0.0000`).
- Safe/clearance heights are the literal strings `G0 Z5.0000` and `G0 Z1.0000`.
- Each hole block is preceded by a blank line (the leading `\n` in the comment
  line). Comment format exactly: `( --- HOLE LOCATION: Col N, Row M --- )` with
  1-based numbers (`Col+1`, `Row+1`).
- Go's `%.4f` and Python's `:.4f` both round ties-to-even and produce identical
  strings for these values, so the output matches byte-for-byte.
- Example line build:
  `fmt.Sprintf("G03 X%.4f Y%.4f Z%.4f I%.4f J0.0000 F%d\n", sx, cy, z, -cutRadius, p.VerticalFeedrate)`

### 7.7 Reference output (verification fixture)
With **default parameters** (2 cols × 2 rows, bit 6.0, target 28.57, xOffset 10,
spacing 50.8/50.8, spindle 18000, vFeed 150, hFeed 600, thickness 3.0, pitch 1.0)
the generator yields `cutRadius = 11.2850`, `totalDepth = 4.5000`, and this
**exact** text (byte-for-byte target). Add this as a Go test comparing
`strings.Join(lines, "")` to the string below.

```
G90 G21 G17
G0 Z5.0000
M03 S18000
G4 P4000

( --- HOLE LOCATION: Col 1, Row 1 --- )
G0 X13.0000 Y0.0000
G0 Z1.0000
G1 X24.2850 F600
G03 X24.2850 Y0.0000 Z-1.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 Z-2.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 Z-3.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 Z-4.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 Z-4.5000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 I-11.2850 J0.0000 F600
G1 X13.0000
G0 Z5.0000

( --- HOLE LOCATION: Col 1, Row 2 --- )
G0 X13.0000 Y50.8000
G0 Z1.0000
G1 X24.2850 F600
G03 X24.2850 Y50.8000 Z-1.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 Z-2.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 Z-3.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 Z-4.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 Z-4.5000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 I-11.2850 J0.0000 F600
G1 X13.0000
G0 Z5.0000

( --- HOLE LOCATION: Col 2, Row 1 --- )
G0 X63.8000 Y50.8000
G0 Z1.0000
G1 X75.0850 F600
G03 X75.0850 Y50.8000 Z-1.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 Z-2.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 Z-3.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 Z-4.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 Z-4.5000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 I-11.2850 J0.0000 F600
G1 X63.8000
G0 Z5.0000

( --- HOLE LOCATION: Col 2, Row 2 --- )
G0 X63.8000 Y0.0000
G0 Z1.0000
G1 X75.0850 F600
G03 X75.0850 Y0.0000 Z-1.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 Z-2.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 Z-3.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 Z-4.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 Z-4.5000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 I-11.2850 J0.0000 F600
G1 X63.8000
G0 Z5.0000

( --- END OF PROGRAM --- )
M05
G0 X0 Y0
M30
```
(Note the snake order in the coordinates: Col1 goes Y0→Y50.8, Col2 goes
Y50.8→Y0.)

---

## 8. Run-Time Estimation

`EstimateRuntime(p Params) float64` returns estimated total machining time in
**seconds**. Cutting moves use the real feedrates (exact); rapid moves use an
**assumed constant** because GRBL rapid speed is machine-specific and not encoded
in the program.

```
RAPID_RATE = 3000.0        // mm/min, used ONLY for the estimate
```

Algorithm (time for a move = distance_mm / feed_mm_per_min * 60 seconds). Use
`math.Hypot`, `math.Ceil`, `math.Pi`:
```
cutRadius  = (targetHoleDiameter - bitDiameter) / 2
totalDepth = tubeThickness + 1.5
vf = max(verticalFeedrate, 1)      // guard divide-by-zero; use float64
hf = max(horizontalFeedrate, 1)
pitch = max(pitchPerTurn, 1e-6)

seconds = 4.0                       // G4 P4000 startup dwell (4 s)
prevX = prevY = 0.0                 // machine referenced at origin
for each hole (x, y) in HoleCenters(p):
    seconds += hypot(x-prevX, y-prevY) / RAPID_RATE * 60     // rapid to XY
    seconds += 4.0 / RAPID_RATE * 60                          // rapid Z5 -> Z1 (4 mm)
    if cutRadius > 0:
        circ    = 2 * pi * cutRadius
        nTurns  = ceil(totalDepth / pitch)
        helixLen= nTurns * hypot(circ, pitch)
        seconds += cutRadius / hf * 60      // feed out to start edge
        seconds += helixLen  / vf * 60      // helical descent
        seconds += circ      / hf * 60      // flat spring pass (one full circle)
        seconds += cutRadius / hf * 60      // feed back to center
    else:
        seconds += (1.0 + totalDepth) / vf * 60     // center plunge from Z+1 to -totalDepth
    seconds += (5.0 + totalDepth) / RAPID_RATE * 60 // rapid up to Z5 from -totalDepth
    prevX, prevY = x, y
seconds += hypot(prevX, prevY) / RAPID_RATE * 60    // final rapid home to X0 Y0
return seconds
```

Go notes:
- `nTurns` is an integer count via `math.Ceil`; multiply as `float64`.
- Go has no built-in numeric `max` before 1.21; use the builtin `max` (Go 1.21+)
  or a small helper. The guards convert the int feedrates to `float64` first.
- Not modeled: spindle spin-up beyond the 4 s dwell, accel/decel ramping, GRBL
  look-ahead. It is intentionally rough and tends to slightly under-estimate.
- Sanity fixture: default params → **≈ 614 s → "10m 14s"** (see §9).

---

## 9. Duration Formatting

`FormatDuration(seconds float64) string`:
```
total = int(Round(seconds))          // math.Round, then int
h = total / 3600
rem = total % 3600
m = rem / 60
s = rem % 60
if h > 0:  return Sprintf("%dh %02dm %02ds", h, m, s)   // e.g. "1h 04m 09s"
if m > 0:  return Sprintf("%dm %02ds", m, s)            // e.g. "3m 20s"
else:      return Sprintf("%ds", s)                      // e.g. "45s"
```
Use Go integer division/modulo (replaces Python `divmod`). `%02d` zero-pads to 2
digits. Exact examples: `45`→`"45s"`, `200`→`"3m 20s"`, `3849`→`"1h 04m 09s"`.

---

## 10. Presets — persistent named parameter sets

Save current field values under a name, reload them, and delete them. Persist
between runs.

### 10.1 Storage location & format
- File path: the user's home directory + `.holegen_presets.json`. In Go:
  ```go
  home, _ := os.UserHomeDir()
  presetsFile := filepath.Join(home, ".holegen_presets.json")
  ```
  (Home-relative so presets are found regardless of the working directory.)
- On-disk format: a single JSON object mapping
  **preset name → { field_key: value_string }**. Values are the RAW entry strings
  (e.g. `"28.585"`, even `"1.125in"`), NOT parsed numbers. Modeled in Go as
  `map[string]map[string]string`. Write with 2-space indent:
  `json.MarshalIndent(presets, "", "  ")`.
- Example:
  ```json
  {
    "Aluminum 3mm bearing": {
      "bitDiameter": "6.0",
      "targetHoleDiameter": "28.585",
      "...": "..."
    }
  }
  ```

### 10.2 Loading (`LoadPresets() map[string]map[string]string`), defensive
- Read the file; if it does not exist or is unreadable → return an empty map.
- Unmarshal; on any JSON error (corrupt/invalid) → return an empty map (never
  crash/propagate).
- Modeling it directly as `map[string]map[string]string` already discards any
  entry whose shape does not match (name→object-of-strings). If you unmarshal into
  a looser type, keep only entries where the name is a string and the value is an
  object of string→string, coercing values to strings; drop malformed entries
  silently.

### 10.3 Saving (`SavePresets(presets) error`)
- `json.MarshalIndent(presets, "", "  ")`, then `os.WriteFile(presetsFile, data,
  0644)`. Return any I/O error (callers show an error dialog).

### 10.4 Behaviors (identical to original)
- **Save:** first validate ALL fields (the same parse used by generate). On any
  invalid field → show an error dialog and abort (never save invalid input). Then
  prompt for a name (pre-filled with the currently selected preset name). If the
  prompt is cancelled → abort. Empty/blank name → error dialog
  "Please enter a name." If the name already exists → confirm overwrite via a
  yes/no dialog ("A preset named 'X' already exists. Overwrite it?"); "No" aborts.
  On success: store `{key: TrimSpace(entryText)}` for ALL fields, write to disk,
  refresh the dropdown selecting the new name, set status to `Saved preset 'X'.`,
  AND show an info dialog: `Preset 'X' was saved to:` + newline + the file path.
- **Load:** on selecting a name in the preset dropdown, look it up; if
  missing/empty do nothing. For each field key present in the stored preset, set
  that entry's text to the stored string. Refresh the live estimate and set status
  to `Loaded preset 'X'.`
- **Delete:** if no valid preset selected → info dialog "Select a preset to delete
  first." Else confirm via yes/no ("Delete preset 'X'?"); on "Yes" remove the key,
  write to disk, refresh the dropdown, set status `Deleted preset 'X'.`
- **Dropdown population:** values = preset names sorted alphabetically
  (`sort.Strings`). After a save, reselect the saved name; if the current
  selection no longer exists, clear it.

---

## 11. Input Validation (`ReadParams`)

Build a `Params` from the entry widgets, parsing every field via
`ParseMeasurement` (§4):
- For each field: take the entry text, `TrimSpace`, parse with the field's
  `IsInt`.
- On a parse error for a field, return an error whose message is exactly:
  `'{Label}' must be a valid {typeName} (optionally suffixed with 'mm' or 'in', e.g. 1.125in).`
  where `typeName` is `int` for integer fields and `float` for float fields.
  (Keep the words `int`/`float` verbatim even though Go's types differ — this is
  user-facing text copied from the original.)
- After all fields parse: if `rowCount < 1` OR `columnCount < 1` → return error
  `"Rows and columns must each be at least 1."`.
- Assign integer fields (`rowCount`, `columnCount`, `spindleSpeed`,
  `verticalFeedrate`, `horizontalFeedrate`) as `int`; the rest as `float64`.

`ReadParams` is called:
- Live, whenever any field changes, to refresh the estimate; errors while
  mid-typing are swallowed silently (§12).
- On Save (validate before storing a preset).
- On Generate (validate before producing G-code; errors shown in a dialog).

There is **no** RPM cap enforcement, **no** non-negative checks on
feedrates/spacings/diameters beyond `cutRadius >= 0` in the generator, and **no**
upper bounds. Only the two rules above are enforced.

---

## 12. GUI Layout & Widgets (visualizer excluded)

A single main window.

- **Window title:** `G-code Hole & MAXTube Grid Generator`.
- **Fixed size / not resizable** (Fyne: `SetFixedSize(true)` or a fixed content
  size; Walk: fixed window style).
- A consistent small padding around fields (original used 8px horizontal / 3px
  vertical); a form/grid layout with labels left-aligned and entries right of them.

Widget stack, top to bottom:

1. **Header label**, muted gray, wrapping ~520px wide, two lines (verbatim):
   ```
   Setup:  X=0 = endmill edge touching tube side   |   Y=0 = center of first row   |   Z=0 = top of material
   Tip: append 'in' (or ") to any value to enter it in inches, e.g. 1.125in.
   ```
2. **Preset bar** (a horizontal row): a `Preset:` label, a dropdown/select of
   saved preset names, a `Save` button, and a `Delete` button. Selecting a name
   loads that preset; the buttons run the save/delete flows in §10.4.
3. **The 12 field rows**, generated by iterating `Fields`: each row is the label
   plus a text entry pre-filled with the field's default string. Changing any
   entry recomputes and shows the live estimate. 
   - **Special case — `targetHoleDiameter` row:** the value cell holds a small
     text entry PLUS the diameter dropdown from §5 to its right.
4. **Generate button:** text `Generate .nc File` — runs the generate/save flow
   (§13).
5. **Status label** (bottom, full width), green, initially empty — shows transient
   messages (preset saved/loaded/deleted, generation summary).

On startup, do an initial estimate refresh once the window is shown.

**Live estimate display.** The original painted the estimate on the canvas; with
the visualizer removed, show it in the **status label** (or a dedicated label)
instead. On each field change, when `ReadParams` succeeds, display e.g.
`4 holes  |  Ø28.57mm  |  ~10m 14s`, built as
`fmt.Sprintf("%d holes  |  Ø%.2fmm  |  ~%s", holes, p.TargetHoleDiameter, FormatDuration(EstimateRuntime(p)))`.
When `ReadParams` returns an error (fields mid-edit), silently leave the last good
value.

Message/prompt dialogs to implement (Fyne `dialog` package): error, information,
yes/no confirm, a single-line text prompt (for the preset name), and a file-save
dialog (§13).

---

## 13. Generate action & file output

1. `ReadParams`; then `GenerateGCode`. On any error from either → show an error
   dialog titled "Invalid input" with the message; abort.
2. Open a **save-file dialog** with: title `Save G-code`, default filename
   `holes.nc`, default extension `.nc`, and a G-code filter (`*.nc`) plus an
   all-files option. If the user cancels → abort silently. (Fyne:
   `dialog.ShowFileSave`, set the filter to `.nc` and the default file name.)
3. Write the file as UTF-8: concatenate the lines (each already ends with `\n`)
   and write them (`os.WriteFile` or a buffered writer). On an I/O error → show an
   error dialog titled "Save failed" with the message; abort.
4. On success:
   - Refresh the live estimate display.
   - Compute `holes = rowCount * columnCount` and
     `est = FormatDuration(EstimateRuntime(p))`.
   - Set the status label to:
     `Saved {holes} holes  |  cut radius {cutRadius:.3f}mm  |  depth {totalDepth:.2f}mm  |  est. run time ~{est}`
     (Go: `%.3f` and `%.2f`).
   - Show an info dialog titled "Done" with body:
     ```
     G-code file generated:
     {path}

     Estimated run time: ~{est}
     (rapids assumed at 3000 mm/min)
     ```

---

## 14. Complete list of user-facing features (checklist)

1. Generate CNC G-code for a rectangular grid of round holes in metal tube.
2. Twelve configurable parameters (§3) with robotics defaults.
3. Helical (corkscrew) boring of each hole via full-circle CCW `G03` arcs,
   descending `pitchPerTurn` mm per turn, final pass clamped to depth.
4. Flat spring/finish pass at final depth to clean the hole floor.
5. Automatic 1.5 mm breakthrough past the material thickness.
6. Retract-to-center before lift on each hole to avoid gouging the wall.
7. Degenerate case: hole Ø == bit Ø → straight center plunge instead of a helix.
8. Snake/boustrophedon cut ordering to minimize rapid travel.
9. Safety-first startup: retract Z to safe height BEFORE spindle start; 4 s
   spin-up dwell; safe `Z5` between holes; spindle-off + home + `M30` at end.
10. Per-field **inch/mm unit override** by typing `in` / `"` / `mm` suffixes.
11. Named **target-diameter dropdown** (6 mm, 1/2" shaft, bearing hole, 2" hole)
    that quick-fills the diameter field.
12. **Estimated run time** shown live and in the generation summary/dialog.
13. **Named presets**: save / load / delete, persisted to
    `~/.holegen_presets.json`, graceful on missing/corrupt files, with overwrite
    confirmation.
14. Input validation with friendly error dialogs; robust against mid-typing
    partial input (live updates silently skipped when invalid).
15. Save G-code to a `.nc` file via a native save dialog (UTF-8), with success
    and failure dialogs.
16. Distributable as a single native executable (`go build`; Windows: add
    `-ldflags "-H windowsgui"` to hide the console).

---

## 15. Exact string/format constants reference (quick table)

`%.4f`/`%.3f`/`%.2f`/`%d` below are Go `fmt` verbs.

| Thing                         | Exact value / format                                   |
|-------------------------------|--------------------------------------------------------|
| Safe height rapid             | `G0 Z5.0000\n`                                         |
| Above-material rapid          | `G0 Z1.0000\n`                                         |
| Startup line                  | `G90 G21 G17\n`                                         |
| Spindle on                    | `M03 S%d\n` (spindleSpeed)                             |
| Spin-up dwell                 | `G4 P4000\n`                                            |
| Hole comment                  | `\n( --- HOLE LOCATION: Col %d, Row %d --- )\n` (Col+1, Row+1) |
| Move to edge                  | `G1 X%.4f F%d\n` (start_arc_x, horizontalFeedrate)     |
| Helical arc                   | `G03 X%.4f Y%.4f Z%.4f I%.4f J0.0000 F%d\n` (sx, cy, z, -cutRadius, verticalFeedrate) |
| Spring pass                   | `G03 X%.4f Y%.4f I%.4f J0.0000 F%d\n` (sx, cy, -cutRadius, horizontalFeedrate) |
| Return to center              | `G1 X%.4f\n` (center_x)                                |
| Center plunge (cutRadius==0)  | `G1 Z%.4f F%d\n` (-totalDepth, verticalFeedrate)      |
| End block                     | `\n( --- END OF PROGRAM --- )\n` `M05\n` `G0 X0 Y0\n` `M30\n` |
| Breakthrough                  | `totalDepth = tubeThickness + 1.5`                     |
| Cut radius                    | `(targetHoleDiameter - bitDiameter) / 2`               |
| Inch factor                   | `MM_PER_INCH = 25.4`                                    |
| Rapid estimate rate           | `RAPID_RATE = 3000.0` mm/min                            |
| Presets file                  | `~/.holegen_presets.json` (`os.UserHomeDir()` + join)  |
| Window title                  | `G-code Hole & MAXTube Grid Generator`                 |
| Default save filename         | `holes.nc`                                              |
