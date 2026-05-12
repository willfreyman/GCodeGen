package editor

import (
	"fmt"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// buildSelectedSection adds an "Edit selected" card that shows which
// stroke is currently selected and lets the user clear the selection.
// The actual depth editing happens inline in the operations table —
// see the depth TextInput inside each row in oplist.go — so this
// section is intentionally status-only to avoid two depth inputs that
// could fight each other (typing in one would rebuild the row and
// wipe the other mid-edit).
func (g *Game) buildSelectedSection(heading, face, small *text.Face) widget.PreferredSizeLocateableWidget {
	c := section("Edit selected", heading)

	g.selectedStatus = widget.NewText(
		widget.TextOpts.Text("Click a stroke (canvas or row) to select it; edit depth inline in the row.", small, textMuted),
	)
	c.AddChild(g.selectedStatus)

	c.AddChild(standardButton(face, "Deselect", func() {
		g.editor.SelectedIdx = -1
	}))
	return c
}

// refreshSelectedSection updates the status text to match the
// currently-selected stroke. Called from Update() whenever SelectedIdx
// changes.
func (g *Game) refreshSelectedSection() {
	if g.selectedStatus == nil {
		return
	}
	i := g.editor.SelectedIdx
	if i < 0 || i >= len(g.editor.Strokes) {
		g.selectedStatus.Label = "Click a stroke (canvas or row) to select it; edit depth inline in the row."
		return
	}
	s := g.editor.Strokes[i]
	g.selectedStatus.Label = fmt.Sprintf("Selected: %s", s.Name)
}
