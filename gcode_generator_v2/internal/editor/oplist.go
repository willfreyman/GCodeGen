package editor

import (
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// refreshOpList rebuilds the rows in the operations list from the
// current set of strokes. Called whenever a stroke is added, removed,
// or has its depth edited. Mirrors gcodegen.py:_refresh_list (line 228).
func (g *Game) refreshOpList() {
	if g.opListInner == nil {
		return
	}
	g.opListInner.RemoveChildren()
	g.opRows = g.opRows[:0]
	face := g.refreshOpListFace
	if face == nil {
		return
	}
	if len(g.editor.Strokes) == 0 {
		g.opListInner.AddChild(widget.NewText(
			widget.TextOpts.Text("  No strokes yet — draw one on the canvas.", face, textMuted),
		))
		return
	}
	for i, s := range g.editor.Strokes {
		idx := i
		stroke := s
		bg := bgOpListInner
		if i%2 == 1 {
			bg = bgOpRowAlt
		}
		if i == g.editor.SelectedIdx {
			bg = bgOpRowSelected
		}
		row := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(nineSlice(bg)),
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(8),
				widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(6)),
			)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
				// Click on the row (anywhere outside the ✕ button) selects
				// this stroke so its depth can be edited in the "Edit
				// selected" section. The ✕ button captures clicks on its
				// own rect and runs Delete instead.
				widget.WidgetOpts.MouseButtonClickedHandler(func(args *widget.WidgetMouseButtonClickedEventArgs) {
					if args.Button != ebiten.MouseButtonLeft {
						return
					}
					if idx < len(g.editor.Strokes) {
						g.editor.SelectedIdx = idx
					}
				}),
			),
		)
		// Color swatch — a Container with a colored background.
		swatch := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(borderedNineSlice(stroke.Color, sectionDiv)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(20, 20),
			),
		)
		row.AddChild(swatch)
		row.AddChild(widget.NewText(
			widget.TextOpts.Text(stroke.Name, face, textPrimary),
			widget.TextOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		))
		// Inline depth field — edit-in-place. SubmitHandler updates the
		// model directly. We DON'T set strokesDirty here, because that
		// would rebuild the row mid-typing and destroy focus/text. The
		// input already shows the value the user just typed, and the
		// model is in sync.
		depthInput := widget.NewTextInput(
			widget.TextInputOpts.Image(textInputImage()),
			widget.TextInputOpts.Color(textInputColor()),
			widget.TextInputOpts.Face(face),
			widget.TextInputOpts.Padding(widget.NewInsetsSimple(3)),
			widget.TextInputOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(54, 22),
			),
			widget.TextInputOpts.SubmitHandler(func(args *widget.TextInputChangedEventArgs) {
				if v, err := strconv.ParseFloat(args.InputText, 64); err == nil {
					g.editor.SetDepth(idx, v)
				}
			}),
		)
		depthInput.SetText(formatFloat(stroke.Depth))
		// Same focus-loss commit pattern as panel.go's floatInput —
		// without it, typing in the row and clicking Generate G-code
		// silently discards the typed value.
		depthInput.GetWidget().FocusEvent.AddHandler(func(args interface{}) {
			if e, ok := args.(*widget.WidgetFocusEventArgs); ok && !e.Focused {
				depthInput.Submit()
			}
		})
		row.AddChild(depthInput)
		row.AddChild(widget.NewButton(
			widget.ButtonOpts.Image(deleteButtonImage()),
			widget.ButtonOpts.Text("×", face, &widget.ButtonTextColor{
				Idle: deleteText, Hover: deleteText, Pressed: deleteText,
			}),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(4)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(28, 22),
			),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				g.editor.DeleteStroke(idx)
				g.strokesDirty = true
			}),
		))
		g.opListInner.AddChild(row)
		g.opRows = append(g.opRows, row)
	}
}

// updateRowHighlights mutates each row's background to reflect the
// current SelectedIdx without re-creating any child widgets. Called
// from Update when selection changes; preserves any depth-input the
// user is mid-typing into.
func (g *Game) updateRowHighlights() {
	for i, row := range g.opRows {
		bg := bgOpListInner
		if i%2 == 1 {
			bg = bgOpRowAlt
		}
		if i == g.editor.SelectedIdx {
			bg = bgOpRowSelected
		}
		row.SetBackgroundImage(nineSlice(bg))
	}
}
