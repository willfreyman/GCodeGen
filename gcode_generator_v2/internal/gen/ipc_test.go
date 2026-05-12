package gen

import (
	"bytes"
	"image/color"
	"testing"
)

func TestIPC_RoundTrip(t *testing.T) {
	e := NewEditor()
	e.Strokes = []Stroke{
		{
			Points: []Point{{X: 100, Y: 200}, {X: 150, Y: 220}},
			Name:   "Cut 1",
			Depth:  -1.5,
			Color:  color.RGBA{R: 0xe6, G: 0x39, B: 0x46, A: 0xff},
		},
	}
	e.Perim.Cut = true

	in := e.SnapshotState()

	var buf bytes.Buffer
	if err := EncodeMessage(&buf, in); err != nil {
		t.Fatalf("encode: %v", err)
	}

	var got UpdateMessage
	if err := ReadMessages(&buf, func(m UpdateMessage) { got = m }); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Kind != "state" {
		t.Errorf("kind: got %q want state", got.Kind)
	}
	if len(got.Strokes) != 1 {
		t.Fatalf("strokes len: got %d want 1", len(got.Strokes))
	}
	s := got.Strokes[0]
	if s.Name != "Cut 1" || s.Depth != -1.5 {
		t.Errorf("stroke wire mismatch: %+v", s)
	}
	if s.Color != "#e63946" {
		t.Errorf("color: got %q want #e63946", s.Color)
	}
	if got.Perim == nil || !got.Perim.Cut {
		t.Errorf("perim Cut not preserved: %+v", got.Perim)
	}
}

func TestIPC_MultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	for _, kind := range []string{"play", "pause", "reset"} {
		if err := EncodeMessage(&buf, UpdateMessage{Kind: kind}); err != nil {
			t.Fatalf("encode %s: %v", kind, err)
		}
	}
	var kinds []string
	if err := ReadMessages(&buf, func(m UpdateMessage) { kinds = append(kinds, m.Kind) }); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"play", "pause", "reset"}
	if len(kinds) != 3 || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] {
		t.Errorf("kinds: got %v want %v", kinds, want)
	}
}
