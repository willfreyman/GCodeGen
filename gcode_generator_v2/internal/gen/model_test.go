package gen

import "testing"

func TestNewEditor_Defaults(t *testing.T) {
	e := NewEditor()
	if e.HoverIdx != -1 {
		t.Errorf("HoverIdx default: got %d want -1", e.HoverIdx)
	}
	if e.Machine.SafeZ != DefaultSafeZ {
		t.Errorf("SafeZ default: got %v want %v", e.Machine.SafeZ, DefaultSafeZ)
	}
	if e.Perim != DefaultPerim {
		t.Errorf("Perim default: got %+v want %+v", e.Perim, DefaultPerim)
	}
}

func TestFinalizeStroke_TooFewPoints(t *testing.T) {
	e := NewEditor()
	e.Current = []Point{{X: 10, Y: 10}}
	if e.FinalizeStroke() {
		t.Error("FinalizeStroke should return false with <2 points")
	}
	if len(e.Strokes) != 0 {
		t.Errorf("strokes should be empty; got %d", len(e.Strokes))
	}
	if e.Current != nil {
		t.Error("Current should be cleared even on failure")
	}
}

func TestFinalizeStroke_RotatesColor(t *testing.T) {
	e := NewEditor()
	for i := 0; i < 3; i++ {
		e.Current = []Point{{X: float64(i * 10), Y: 0}, {X: float64(i*10 + 10), Y: 0}}
		if !e.FinalizeStroke() {
			t.Fatalf("FinalizeStroke %d returned false", i)
		}
	}
	if len(e.Strokes) != 3 {
		t.Errorf("got %d strokes want 3", len(e.Strokes))
	}
	if e.Strokes[0].Color == e.Strokes[1].Color {
		t.Error("stroke 0 and 1 should have different colors")
	}
	if e.Strokes[0].Name != "Cut 1" {
		t.Errorf("default name: got %q want %q", e.Strokes[0].Name, "Cut 1")
	}
}

func TestFinalizeStroke_CustomName(t *testing.T) {
	e := NewEditor()
	e.NewOpName = "Pocket"
	e.Current = []Point{{X: 0, Y: 0}, {X: 10, Y: 0}}
	if !e.FinalizeStroke() {
		t.Fatal("FinalizeStroke returned false")
	}
	if e.Strokes[0].Name != "Pocket" {
		t.Errorf("got %q want %q", e.Strokes[0].Name, "Pocket")
	}
}

func TestDeleteStroke(t *testing.T) {
	e := NewEditor()
	for i := 0; i < 3; i++ {
		e.Current = []Point{{X: 0, Y: float64(i * 10)}, {X: 10, Y: float64(i * 10)}}
		e.FinalizeStroke()
	}
	e.HoverIdx = 1
	e.DeleteStroke(1)
	if len(e.Strokes) != 2 {
		t.Errorf("after delete: got %d want 2", len(e.Strokes))
	}
	if e.HoverIdx != -1 {
		t.Errorf("HoverIdx after deleting hovered stroke: got %d want -1", e.HoverIdx)
	}
}

func TestDeleteStroke_HoverShifts(t *testing.T) {
	e := NewEditor()
	for i := 0; i < 4; i++ {
		e.Current = []Point{{X: 0, Y: float64(i * 10)}, {X: 10, Y: float64(i * 10)}}
		e.FinalizeStroke()
	}
	e.HoverIdx = 3
	e.DeleteStroke(1) // index 3 should shift to 2
	if e.HoverIdx != 2 {
		t.Errorf("HoverIdx after deleting earlier stroke: got %d want 2", e.HoverIdx)
	}
}

func TestSnapOrigin(t *testing.T) {
	e := NewEditor()
	e.Origin = Origin{X: 999, Y: 999}
	e.SnapOrigin()
	wantX := e.Perim.X0
	if e.Perim.X1 < wantX {
		wantX = e.Perim.X1
	}
	wantY := e.Perim.Y0
	if e.Perim.Y1 > wantY {
		wantY = e.Perim.Y1
	}
	if e.Origin.X != wantX || e.Origin.Y != wantY {
		t.Errorf("after snap: got (%v, %v) want (%v, %v)", e.Origin.X, e.Origin.Y, wantX, wantY)
	}
}

func TestApplyPreset(t *testing.T) {
	e := NewEditor()
	if !e.ApplyPreset("Wood") {
		t.Fatal("ApplyPreset(Wood) returned false")
	}
	if e.Machine.FeedXY != 900 || e.Machine.FeedZ != 60 || e.Machine.RPM != 12000 {
		t.Errorf("Wood preset not applied; got %+v", e.Machine)
	}
	if e.ApplyPreset("Unobtainium") {
		t.Error("ApplyPreset on unknown material should return false")
	}
}
