package editor

import (
	"image/color"
	"strconv"
	"strings"

	"gcodegen.local/generator/internal/gen"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

const (
	panelPadding   = 14
	panelInnerPad  = 12
	rowSpacing     = 6
	sectionGap     = 12
	labelWidth     = 110
	inputWidth     = 130
	inputHeight    = 26
	smallButtonH   = 28
	primaryButtonH = 34
)

// buildUI constructs the right-side control panel and returns the
// ebitenui.UI root container ready to be Update()'d / Draw()'d each
// frame. The panel sections are wrapped in a ScrollContainer because
// total content height exceeds the 600px window — mouse wheel scrolls
// the panel, and the side scrollbar can be dragged.
func (g *Game) buildUI() *ebitenui.UI {
	headingVal := fontFace(14)
	bodyVal := fontFace(13)
	smallVal := fontFace(11)
	if headingVal == nil || bodyVal == nil || smallVal == nil {
		return &ebitenui.UI{Container: widget.NewContainer()}
	}
	heading := &headingVal
	face := &bodyVal
	small := &smallVal

	root := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	// Outer panel frame: anchors to right edge, fills window height,
	// owns the dark background. A horizontal RowLayout holds the
	// scroll container and the scrollbar side-by-side.
	const scrollbarWidth = 12
	panel := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(nineSlice(bgPanel)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
				VerticalPosition:   widget.AnchorLayoutPositionStart,
				StretchVertical:    true,
			}),
			widget.WidgetOpts.MinSize(PanelWidth, WindowHeight),
		),
	)

	// Inner content: vertical stack of sections.
	content := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(sectionGap),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(panelPadding)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(PanelWidth-scrollbarWidth, 0),
		),
	)
	content.AddChild(g.buildViewSection(heading, face))
	content.AddChild(g.buildPerimSection(heading, face))
	content.AddChild(g.buildNewOpSection(heading, face, small))
	content.AddChild(g.buildImageSection(heading, face, small))
	content.AddChild(g.buildSelectedSection(heading, face, small))
	g.opListContainer = g.buildOpListSection(heading, face, small)
	content.AddChild(g.opListContainer)
	content.AddChild(g.buildMaterialSection(heading, face))
	content.AddChild(g.buildMachineSection(heading, face))
	content.AddChild(g.buildActionsSection(heading, face))

	// Scroll container wraps content. Mouse wheel events on the
	// container body adjust ScrollTop directly (ebitenui's scroll
	// container doesn't subscribe to wheel events on its own).
	scroll := widget.NewScrollContainer(
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle:     nineSlice(bgPanel),
			Disabled: nineSlice(bgPanel),
			Mask:     nineSlice(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}),
		}),
		widget.ScrollContainerOpts.Content(content),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Stretch:  true,
				Position: widget.RowLayoutPositionStart,
			}),
			widget.WidgetOpts.MinSize(PanelWidth-scrollbarWidth, WindowHeight),
		),
	)
	scroll.GetWidget().ScrolledEvent.AddHandler(func(args interface{}) {
		a, ok := args.(*widget.WidgetScrolledEventArgs)
		if !ok {
			return
		}
		// Y is the wheel delta. Negative = wheel up = scroll content up.
		// ContentRect/ViewRect determine how 1 wheel notch maps to ScrollTop.
		ch := scroll.ContentRect().Dy()
		vh := scroll.ViewRect().Dy()
		span := ch - vh
		if span <= 0 {
			return
		}
		// ~30 pixels per wheel notch.
		scroll.ScrollTop -= a.Y * 30.0 / float64(span)
		if scroll.ScrollTop < 0 {
			scroll.ScrollTop = 0
		} else if scroll.ScrollTop > 1 {
			scroll.ScrollTop = 1
		}
	})

	// Side scrollbar so users without scroll wheels still have a
	// way to navigate (and so scroll position is visible).
	scrollbar := widget.NewSlider(
		widget.SliderOpts.Direction(widget.DirectionVertical),
		widget.SliderOpts.MinMax(0, 1000),
		widget.SliderOpts.InitialCurrent(0),
		widget.SliderOpts.PageSizeFunc(func() int { return 100 }),
		widget.SliderOpts.Images(sliderTrackImage(), sliderHandleImage()),
		widget.SliderOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Stretch:  true,
				Position: widget.RowLayoutPositionStart,
			}),
			widget.WidgetOpts.MinSize(scrollbarWidth, WindowHeight),
		),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			scroll.ScrollTop = float64(args.Current) / 1000.0
		}),
	)

	panel.AddChild(scroll)
	panel.AddChild(scrollbar)
	root.AddChild(panel)
	return &ebitenui.UI{Container: root}
}

// section returns a card-style container with a heading row and a
// vertical stack of children. Visually separated from siblings by its
// own background and inner padding.
func section(title string, heading *text.Face) *widget.Container {
	c := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(nineSlice(bgSection)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(rowSpacing),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(panelInnerPad)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	c.AddChild(widget.NewText(
		widget.TextOpts.Text(strings.ToUpper(title), heading, textHeading),
	))
	return c
}

// labeledRow places a label on the left and the input widget on the right.
func labeledRow(face *text.Face, label string, input widget.PreferredSizeLocateableWidget) *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	lbl := widget.NewText(
		widget.TextOpts.Text(label, face, textSecondary),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: false}),
			widget.WidgetOpts.MinSize(labelWidth, 0),
		),
	)
	row.AddChild(lbl)
	row.AddChild(input)
	return row
}

func floatInput(face *text.Face, value float64, onCommit func(float64)) *widget.TextInput {
	ti := widget.NewTextInput(
		widget.TextInputOpts.Image(textInputImage()),
		widget.TextInputOpts.Color(textInputColor()),
		widget.TextInputOpts.Face(face),
		widget.TextInputOpts.Padding(widget.NewInsetsSimple(5)),
		widget.TextInputOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(inputWidth, inputHeight),
		),
		widget.TextInputOpts.SubmitHandler(func(args *widget.TextInputChangedEventArgs) {
			if v, err := strconv.ParseFloat(args.InputText, 64); err == nil {
				onCommit(v)
			}
		}),
	)
	ti.SetText(formatFloat(value))
	// Commit on focus loss too — ebitenui's SubmitHandler only fires
	// on Enter, so without this a user who types "-19.05" then clicks
	// Generate G-code loses the typed value silently and runs against
	// the previous depth.
	ti.GetWidget().FocusEvent.AddHandler(func(args interface{}) {
		if e, ok := args.(*widget.WidgetFocusEventArgs); ok && !e.Focused {
			ti.Submit()
		}
	})
	return ti
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func stringInput(face *text.Face, value string, onCommit func(string)) *widget.TextInput {
	ti := widget.NewTextInput(
		widget.TextInputOpts.Image(textInputImage()),
		widget.TextInputOpts.Color(textInputColor()),
		widget.TextInputOpts.Face(face),
		widget.TextInputOpts.Padding(widget.NewInsetsSimple(5)),
		widget.TextInputOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(inputWidth, inputHeight),
		),
		widget.TextInputOpts.Placeholder("(auto)"),
		widget.TextInputOpts.SubmitHandler(func(args *widget.TextInputChangedEventArgs) {
			onCommit(args.InputText)
		}),
		widget.TextInputOpts.ChangedHandler(func(args *widget.TextInputChangedEventArgs) {
			onCommit(args.InputText)
		}),
	)
	ti.SetText(value)
	return ti
}

func standardButton(face *text.Face, label string, onClick func()) *widget.Button {
	return widget.NewButton(
		widget.ButtonOpts.Image(buttonImage()),
		widget.ButtonOpts.Text(label, face, &widget.ButtonTextColor{
			Idle: textPrimary, Hover: textPrimary, Pressed: textPrimary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(8)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, smallButtonH),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) { onClick() }),
	)
}

func primaryButton(face *text.Face, label string, onClick func()) *widget.Button {
	return widget.NewButton(
		widget.ButtonOpts.Image(primaryButtonImage()),
		widget.ButtonOpts.Text(label, face, &widget.ButtonTextColor{
			Idle: color.White, Hover: color.White, Pressed: color.White,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(10)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, primaryButtonH),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) { onClick() }),
	)
}

// buildViewSection adds the View card at the very top of the panel —
// three buttons that drive canvas zoom directly. Always works
// regardless of cursor position (no cross-fire possible with panel
// wheel events).
func (g *Game) buildViewSection(heading, face *text.Face) widget.PreferredSizeLocateableWidget {
	c := section("View", heading)
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Spacing(6, 0),
			widget.GridLayoutOpts.Stretch([]bool{true, true, true}, nil),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	row.AddChild(standardButton(face, "Zoom +", g.zoomInButton))
	row.AddChild(standardButton(face, "Zoom −", g.zoomOutButton))
	row.AddChild(standardButton(face, "Reset", g.resetView))
	c.AddChild(row)
	return c
}

func (g *Game) buildPerimSection(heading, face *text.Face) widget.PreferredSizeLocateableWidget {
	c := section("Perimeter", heading)
	c.AddChild(labeledRow(face, "Width (mm)",
		floatInput(face, g.editor.Perim.WidthMM, func(v float64) { g.editor.Perim.WidthMM = v })))
	c.AddChild(labeledRow(face, "Height (mm)",
		floatInput(face, g.editor.Perim.HeightMM, func(v float64) { g.editor.Perim.HeightMM = v })))
	// Tracked so the global Depth field in "New Operation" can refresh
	// this input visually when it propagates the depth to the perim.
	g.perimDepthInput = floatInput(face, g.editor.Perim.DepthMM, func(v float64) { g.editor.Perim.DepthMM = v })
	c.AddChild(labeledRow(face, "Cut depth (mm)", g.perimDepthInput))
	c.AddChild(widget.NewCheckbox(
		widget.CheckboxOpts.Image(checkboxImage()),
		widget.CheckboxOpts.Text("Cut perimeter", face, textColor()),
		widget.CheckboxOpts.Spacing(8),
		widget.CheckboxOpts.WidgetOpts(widget.WidgetOpts.MinSize(20, 20)),
		widget.CheckboxOpts.StateChangedHandler(func(args *widget.CheckboxChangedEventArgs) {
			g.editor.Perim.Cut = args.State == widget.WidgetChecked
		}),
	))
	c.AddChild(standardButton(face, "Snap origin to corner", g.editor.SnapOrigin))
	return c
}

func (g *Game) buildNewOpSection(heading, face, small *text.Face) widget.PreferredSizeLocateableWidget {
	c := section("New Operation", heading)
	c.AddChild(labeledRow(face, "Name",
		stringInput(face, g.editor.NewOpName, func(s string) { g.editor.NewOpName = s })))
	// Depth here is the GLOBAL cut depth: committing it propagates to
	// every existing stroke and to the perimeter, not just to the
	// "next-drawn stroke" default. This matches user intent — typing
	// "-19.05" is "I want my cuts at -19.05 mm". Per-stroke overrides
	// still work via the inline depth input on each operations-table row.
	c.AddChild(labeledRow(face, "Depth (mm)",
		floatInput(face, g.editor.NewOpDepth, func(v float64) {
			g.editor.NewOpDepth = v
			for i := range g.editor.Strokes {
				g.editor.SetDepth(i, v)
			}
			g.editor.Perim.DepthMM = v
			if g.perimDepthInput != nil {
				g.perimDepthInput.SetText(formatFloat(v))
			}
			g.strokesDirty = true
		})))
	c.AddChild(widget.NewText(
		widget.TextOpts.Text("Depth applies to all strokes + the perimeter. Override per-stroke in the table below.", small, textMuted),
	))
	c.AddChild(widget.NewText(
		widget.TextOpts.Text("Click + drag on the canvas to draw a stroke.", small, textMuted),
	))
	c.AddChild(widget.NewText(
		widget.TextOpts.Text("Zoom with the View buttons above, or Ctrl+scroll over the canvas.", small, textMuted),
	))
	return c
}

func (g *Game) buildOpListSection(heading, face, small *text.Face) *widget.Container {
	c := section("Operations", heading)
	g.opListInner = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(nineSlice(bgOpListInner)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(0),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(0)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	scroll := widget.NewScrollContainer(
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle:     borderedNineSlice(bgOpListInner, inputBorder),
			Disabled: borderedNineSlice(bgInputDis, inputBorder),
			Mask:     nineSlice(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}),
		}),
		widget.ScrollContainerOpts.Content(g.opListInner),
		widget.ScrollContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(0, 130),
		),
	)
	c.AddChild(scroll)
	c.AddChild(widget.NewText(
		widget.TextOpts.Text("Click row to edit depth · ✕ deletes.", small, textMuted),
	))
	g.refreshOpListFace = face
	g.refreshOpList()
	return c
}

func (g *Game) buildMaterialSection(heading, face *text.Face) widget.PreferredSizeLocateableWidget {
	c := section("Material preset", heading)
	// 4-column grid wraps automatically — 7 materials fill 2 rows
	// (4 + 3) without overflowing the 324-px panel.
	grid := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(4),
			widget.GridLayoutOpts.Spacing(4, 4),
			widget.GridLayoutOpts.Stretch([]bool{true, true, true, true}, nil),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	for _, m := range gen.MaterialPresets {
		name := m.Name
		grid.AddChild(widget.NewButton(
			widget.ButtonOpts.Image(buttonImage()),
			widget.ButtonOpts.Text(name, face, &widget.ButtonTextColor{
				Idle: textPrimary, Hover: textPrimary, Pressed: textPrimary,
			}),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(4)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, 26),
			),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				g.editor.ApplyPreset(name)
				g.machineRefresh()
			}),
		))
	}
	c.AddChild(grid)
	return c
}

func (g *Game) buildMachineSection(heading, face *text.Face) widget.PreferredSizeLocateableWidget {
	c := section("Machine Settings", heading)
	g.safeZInput = floatInput(face, g.editor.Machine.SafeZ, func(v float64) { g.editor.Machine.SafeZ = v })
	g.feedXYInput = floatInput(face, g.editor.Machine.FeedXY, func(v float64) { g.editor.Machine.FeedXY = v })
	g.feedZInput = floatInput(face, g.editor.Machine.FeedZ, func(v float64) { g.editor.Machine.FeedZ = v })
	g.rpmInput = floatInput(face, g.editor.Machine.RPM, func(v float64) { g.editor.Machine.RPM = v })
	g.stepDownInput = floatInput(face, g.editor.Machine.StepDown, func(v float64) { g.editor.Machine.StepDown = v })
	c.AddChild(labeledRow(face, "Safe Z (mm)", g.safeZInput))
	c.AddChild(labeledRow(face, "Feed XY (mm/min)", g.feedXYInput))
	c.AddChild(labeledRow(face, "Feed Z (mm/min)", g.feedZInput))
	c.AddChild(labeledRow(face, "Spindle RPM", g.rpmInput))
	c.AddChild(labeledRow(face, "Step down (mm)", g.stepDownInput))
	return c
}

func (g *Game) buildActionsSection(heading, face *text.Face) widget.PreferredSizeLocateableWidget {
	c := section("Actions", heading)
	c.AddChild(standardButton(face, "Simulate Toolpath", g.onSimulateClicked))
	c.AddChild(standardButton(face, "Finished Product Preview", g.onPreviewClicked))
	c.AddChild(primaryButton(face, "Generate G-code", g.onGenerateClicked))
	return c
}

// onGenerateClicked emits G-code from the editor's current state and
// pops a save dialog. Mirrors gcodegen.py:generate_gcode (line 380-405).
func (g *Game) onGenerateClicked() {
	ops := g.editor.OpsMM()
	if len(ops) == 0 {
		showError("Nothing to generate", "Add strokes or enable perimeter cutting.")
		return
	}
	contents := gen.Emit(ops, g.editor.Machine)
	path, err := saveGCodeFile(contents)
	if err != nil {
		showError("Save failed", "%s", err.Error())
		return
	}
	if path != "" {
		showInfo("Saved", "Saved to:\n%s", path)
	}
}

// machineRefresh syncs the machine-settings inputs to the editor's
// current Machine values (called after a preset is applied).
func (g *Game) machineRefresh() {
	if g.safeZInput != nil {
		g.safeZInput.SetText(formatFloat(g.editor.Machine.SafeZ))
	}
	if g.feedXYInput != nil {
		g.feedXYInput.SetText(formatFloat(g.editor.Machine.FeedXY))
	}
	if g.feedZInput != nil {
		g.feedZInput.SetText(formatFloat(g.editor.Machine.FeedZ))
	}
	if g.rpmInput != nil {
		g.rpmInput.SetText(formatFloat(g.editor.Machine.RPM))
	}
	if g.stepDownInput != nil {
		g.stepDownInput.SetText(formatFloat(g.editor.Machine.StepDown))
	}
}
