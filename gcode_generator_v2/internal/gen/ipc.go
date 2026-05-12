package gen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
)

// UpdateMessage is one newline-delimited JSON record sent over the
// editor → subprocess pipe. Kind is the discriminator; field semantics
// depend on Kind:
//
//   - "state"    full editor state (every field below is meaningful)
//   - "play"     simulator: start/resume animation
//   - "pause"    simulator: pause animation
//   - "reset"    simulator: rewind to start
//   - "shutdown" subprocess should exit cleanly (also triggered by EOF)
type UpdateMessage struct {
	Kind     string       `json:"kind"`
	Strokes  []StrokeWire `json:"strokes,omitempty"`
	Perim    *PerimWire   `json:"perim,omitempty"`
	Origin   *OriginWire  `json:"origin,omitempty"`
	Machine  *MachineWire `json:"machine,omitempty"`
	BitMM    float64      `json:"bit_mm,omitempty"`
	Material string       `json:"material,omitempty"`
}

// StrokeWire is the over-the-wire form of Stroke. Color is "#rrggbb".
type StrokeWire struct {
	Points [][2]float64 `json:"points"`
	Name   string       `json:"name"`
	Depth  float64      `json:"depth"`
	Color  string       `json:"color"`
}

// PerimWire mirrors Perim. Pixel rect + mm extents + cut flag.
type PerimWire struct {
	X0       float64 `json:"x0"`
	Y0       float64 `json:"y0"`
	X1       float64 `json:"x1"`
	Y1       float64 `json:"y1"`
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
	DepthMM  float64 `json:"depth_mm"`
	Cut      bool    `json:"cut"`
}

// OriginWire mirrors Origin.
type OriginWire struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// MachineWire mirrors Machine.
type MachineWire struct {
	SafeZ  float64 `json:"safe_z"`
	FeedXY float64 `json:"feed_xy"`
	FeedZ  float64 `json:"feed_z"`
	RPM    float64 `json:"rpm"`
}

// SnapshotState builds an UpdateMessage{Kind: "state", ...} from the
// editor's current state. The aux subprocess applies it atomically.
func (e *Editor) SnapshotState() UpdateMessage {
	strokes := make([]StrokeWire, len(e.Strokes))
	for i, s := range e.Strokes {
		pts := make([][2]float64, len(s.Points))
		for j, p := range s.Points {
			pts[j] = [2]float64{p.X, p.Y}
		}
		strokes[i] = StrokeWire{
			Points: pts,
			Name:   s.Name,
			Depth:  s.Depth,
			Color:  hexColor(s.Color),
		}
	}
	pw := PerimWire{
		X0: e.Perim.X0, Y0: e.Perim.Y0, X1: e.Perim.X1, Y1: e.Perim.Y1,
		WidthMM: e.Perim.WidthMM, HeightMM: e.Perim.HeightMM, DepthMM: e.Perim.DepthMM,
		Cut: e.Perim.Cut,
	}
	ow := OriginWire{X: e.Origin.X, Y: e.Origin.Y}
	mw := MachineWire{SafeZ: e.Machine.SafeZ, FeedXY: e.Machine.FeedXY, FeedZ: e.Machine.FeedZ, RPM: e.Machine.RPM}
	return UpdateMessage{
		Kind:    "state",
		Strokes: strokes,
		Perim:   &pw,
		Origin:  &ow,
		Machine: &mw,
	}
}

// hexColor formats an RGBA as "#rrggbb" (alpha dropped — UI doesn't use it).
func hexColor(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// EncodeMessage serializes m and writes it as one line ending in '\n'.
func EncodeMessage(w io.Writer, m UpdateMessage) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(m) // Encode appends a trailing '\n'
}

// ReadMessages decodes newline-delimited JSON UpdateMessage records from
// r and calls onMsg for each. Returns when r returns EOF or a decode
// error. Used by aux subprocesses on os.Stdin.
func ReadMessages(r io.Reader, onMsg func(UpdateMessage)) error {
	sc := bufio.NewScanner(r)
	// Allow large messages — full state can be tens of KB with many
	// strokes; default Scanner buffer is 64 KB which may be tight.
	const maxLine = 4 << 20 // 4 MiB
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	for sc.Scan() {
		var m UpdateMessage
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			return err
		}
		onMsg(m)
	}
	return sc.Err()
}
