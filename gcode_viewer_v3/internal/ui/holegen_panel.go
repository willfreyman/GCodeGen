package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"

	"gcodegen.local/viewer/internal/holegen"
)

// HoleGen panel — the hole-grid generator from HoleGen_SPEC.md, hosted as an
// overlay inside the viewer window rather than a second OS window.
//
// Layout is manual pixel placement, same as toolbar.go, but built with a
// running vertical cursor rather than hardcoded offsets: each build* method
// takes the current y and returns the next free y, and the panel's final
// height comes from a SetSize once everything is placed. Adding or removing a
// row can't silently overlap whatever came after it.
//
// Vertical order: title bar → setup note → preset bar → four field sections →
// estimate readout → action buttons → status footer.
const (
	hgPanelW = 700.0
	hgPadX   = 16.0
	hgRowH   = 24.0

	hgTitleFontSize   = 13.0
	hgLabelFontSize   = 12.0
	hgSectionFontSize = 11.0
	hgNoteFontSize    = 10.0
	hgEstFontSize     = 14.0

	hgEditX = 356.0 // left edge of every field's entry
	hgEditW = 100   // gui.NewEdit takes an int width
	hgDiaX  = 470.0 // diameter quick-fill dropdown, on the target-diameter row
	hgDiaW  = 214.0

	// Title bar: the drag handle, plus the close button at its right end.
	hgTitleH = 28.0
	hgCloseW = 24.0

	hgFooterH = 26.0
	hgEstH    = 30.0
	hgBtnRowH = 26.0

	// Gap between stacked blocks, and the height a section header claims for
	// its label plus underline.
	hgGap       = 8.0
	hgSectionHH = 20.0

	// hgKeepVisible is how much of the panel must stay on screen horizontally.
	// Dragging it fully off the edge would leave no title bar to grab.
	hgKeepVisible = 140.0
)

// Palette. The accent is the same soft blue the view cube uses for its hover
// highlight, so the generator reads as part of the same application rather
// than a bolted-on dialog.
var (
	hgPanelBg   = math32.Color{R: 0.145, G: 0.160, B: 0.195}
	hgTitleBg   = math32.Color{R: 0.235, G: 0.265, B: 0.325}
	hgNoteBg    = math32.Color{R: 0.165, G: 0.185, B: 0.225}
	hgZebraBg   = math32.Color{R: 0.175, G: 0.192, B: 0.232}
	hgFooterBg  = math32.Color{R: 0.115, G: 0.125, B: 0.155}
	hgBorderCol = math32.Color{R: 0.32, G: 0.36, B: 0.43}
	hgRuleCol   = math32.Color{R: 0.28, G: 0.31, B: 0.38}

	hgAccent = math32.Color{R: 0.55, G: 0.70, B: 0.95}

	hgTitleFg   = math32.Color{R: 0.92, G: 0.94, B: 0.97}
	hgLabelCol  = math32.Color{R: 0.82, G: 0.85, B: 0.90}
	hgNoteCol   = math32.Color{R: 0.58, G: 0.62, B: 0.69}
	hgEstCol    = math32.Color{R: 0.93, G: 0.95, B: 0.98}
	hgStatusCol = math32.Color{R: 0.45, G: 0.90, B: 0.50} // green, per spec §12
)

// hgSections groups the 12 fields under headings. The counts consume
// holegen.Fields IN ORDER — the spec fixes that order and it happens to fall
// into four contiguous, meaningful groups, so nothing is reordered here. If
// the field list ever grows, the leftovers get their own trailing section
// rather than vanishing from the form.
var hgSections = []struct {
	Title string
	Count int
}{
	{"TOOL & HOLE", 2},      // bit diameter, target hole diameter
	{"GRID LAYOUT", 5},      // x offset, spacing X/Y, rows, columns
	{"SPINDLE & FEEDS", 3},  // RPM, vertical feed, horizontal feed
	{"MATERIAL & HELIX", 2}, // metal thickness, helical pitch
}

// HoleGenProgram is a generated program handed back to the viewer for
// preview. BitDiameter and Thickness ride along so the 3D scene can size its
// end-mill and its through-cut threshold to the same numbers the program was
// generated with — otherwise the preview would carve with whatever bit the
// toolbar happened to be set to.
type HoleGenProgram struct {
	Text        string
	DisplayName string
	BitDiameter float64
	Thickness   float64
}

// HoleGenCallbacks bundles the panel's hooks back into the viewer.
type HoleGenCallbacks struct {
	OnLoadProgram func(HoleGenProgram)
}

// HoleGenPanel is the generator overlay.
type HoleGenPanel struct {
	// Panel is added to the scene root as a SIBLING of the toolbar, not a
	// child: g3n clips a child panel to its parent's content rect (a fragment
	// discard driven by Panel.updateBounds), and this panel is far taller than
	// the 64-pixel toolbar.
	Panel *gui.Panel

	// titleBar doubles as the drag handle; it owns the drag mouse handlers.
	titleBar *gui.Panel

	// Drag state. x/y mirror the panel's top-left in window coordinates —
	// tracked here rather than read back from Panel.Position() because the
	// renderer rewrites each panel's Z every frame and we only ever want XY.
	dragging       bool
	dragDX, dragDY float32
	x, y           float32

	// Window size, refreshed from the resize handler, used to clamp the drag
	// so the title bar can't be pushed somewhere unreachable. Zero until the
	// first Resize call, which disables clamping.
	winW, winH float32

	edits    map[string]*gui.Edit
	diaDrop  *gui.DropDown
	prsDrop  *gui.DropDown
	nameEdit *gui.Edit
	estimate *gui.Label
	status   *gui.Label

	presets     holegen.Presets
	presetNames []string

	// suppress guards against re-entrancy while we set widget values
	// programmatically. gui.DropDown dispatches OnChange from SelectPos and
	// SetSelected with no silent variant, so rebuilding the preset list would
	// otherwise fire a load of whatever landed in the selected slot.
	suppress bool

	cb HoleGenCallbacks
}

// NewHoleGenPanel builds the overlay, hidden. y is where the panel's top edge
// sits (just under the toolbar).
func NewHoleGenPanel(y float32, callbacks HoleGenCallbacks) *HoleGenPanel {
	h := &HoleGenPanel{
		edits:   make(map[string]*gui.Edit, len(holegen.Fields)),
		presets: holegen.LoadPresets(),
		cb:      callbacks,
	}

	// Provisional height; the real one is computed from the layout cursor and
	// applied with SetSize once every block has been placed.
	h.Panel = gui.NewPanel(hgPanelW, 600)
	h.Panel.SetColor(&hgPanelBg)
	h.x, h.y = hgPadX, y
	h.Panel.SetPosition(h.x, h.y)
	h.Panel.SetBorders(1, 1, 1, 1)
	h.Panel.SetBordersColor(&hgBorderCol)
	h.Panel.SetVisible(false)
	// Draw above the toolbar and above the Options/Tutorials dropdowns, which
	// use a zLayerDelta of 1. The renderer buckets panels by accumulated
	// zLayerDelta and draws higher buckets last.
	h.Panel.SetZLayerDelta(2)

	h.buildTitleBar()

	cursor := float32(hgTitleH)
	cursor = h.buildNote(cursor + 6)
	cursor = h.buildPresetBar(cursor + hgGap)
	cursor = h.buildSections(cursor + hgGap)
	cursor = h.buildEstimateBox(cursor + hgGap)
	cursor = h.buildActionButtons(cursor + hgGap)
	cursor = h.buildFooter(cursor + hgGap)

	h.Panel.SetSize(hgPanelW, cursor)

	h.refreshPresetDropdown("")
	h.refreshEstimate()
	return h
}

// buildNote is the machine-setup crib from spec §12, in a tinted callout so it
// reads as reference material rather than another control.
//
// gui.Label has no word wrap — SetText rasterizes the whole string and resizes
// the label to fit — so the lines are pre-split to stay inside the panel.
func (h *HoleGenPanel) buildNote(y float32) float32 {
	const noteH = 36.0

	box := gui.NewPanel(hgPanelW-2*hgPadX, noteH)
	box.SetColor(&hgNoteBg)
	box.SetPosition(hgPadX, y)
	h.Panel.Add(box)

	for i, line := range []string{
		"X0 = endmill edge touching the tube side   ·   Y0 = center of the first row   ·   Z0 = top of material",
		`Append 'in' or " to any value to enter it in inches, e.g. 1.125in   ·   output is metric absolute (G21 G90 G17)`,
	} {
		lbl := gui.NewLabel(line)
		lbl.SetFontSize(hgNoteFontSize)
		lbl.SetColor(&hgNoteCol)
		lbl.SetPosition(8, 6+float32(i)*13)
		box.Add(lbl)
	}
	return y + noteH
}

// buildSections emits the field rows grouped under headings, with alternating
// row tints so the eye can track a long label across the gap to its entry.
func (h *HoleGenPanel) buildSections(y float32) float32 {
	next := 0
	for _, sec := range hgSections {
		if next >= len(holegen.Fields) {
			break
		}
		end := next + sec.Count
		if end > len(holegen.Fields) {
			end = len(holegen.Fields)
		}
		y = h.buildSection(y, sec.Title, next, end) + hgGap
		next = end
	}
	// Anything hgSections didn't account for still gets rendered.
	if next < len(holegen.Fields) {
		y = h.buildSection(y, "ADDITIONAL", next, len(holegen.Fields)) + hgGap
	}
	return y - hgGap
}

// buildSection draws one heading + underline, then fields [from, to).
func (h *HoleGenPanel) buildSection(y float32, title string, from, to int) float32 {
	head := gui.NewLabel(title)
	head.SetFontSize(hgSectionFontSize)
	head.SetColor(&hgAccent)
	head.SetPosition(hgPadX, y)
	h.Panel.Add(head)

	rule := gui.NewPanel(hgPanelW-2*hgPadX, 1)
	rule.SetColor(&hgRuleCol)
	rule.SetPosition(hgPadX, y+hgSectionHH-5)
	h.Panel.Add(rule)

	rowY := y + hgSectionHH
	for i := from; i < to; i++ {
		h.buildFieldRow(holegen.Fields[i], i, rowY)
		rowY += hgRowH
	}
	return rowY
}

// buildEstimateBox is the live readout. It gets the largest type on the panel
// and an accent bar down its left edge — it's the number the user is actually
// steering by while they tune the fields.
func (h *HoleGenPanel) buildEstimateBox(y float32) float32 {
	box := gui.NewPanel(hgPanelW-2*hgPadX, hgEstH)
	box.SetColor(&hgNoteBg)
	box.SetBorders(0, 0, 0, 3)
	box.SetBordersColor(&hgAccent)
	box.SetPosition(hgPadX, y)
	h.Panel.Add(box)

	h.estimate = gui.NewLabel("")
	h.estimate.SetFontSize(hgEstFontSize)
	h.estimate.SetColor(&hgEstCol)
	h.estimate.SetPosition(10, 6)
	box.Add(h.estimate)

	return y + hgEstH
}

// buildFooter is the full-width status strip that anchors the bottom edge.
func (h *HoleGenPanel) buildFooter(y float32) float32 {
	strip := gui.NewPanel(hgPanelW, hgFooterH)
	strip.SetColor(&hgFooterBg)
	strip.SetPosition(0, y)
	h.Panel.Add(strip)

	h.status = gui.NewLabel("")
	h.status.SetFontSize(hgLabelFontSize)
	h.status.SetColor(&hgStatusCol)
	h.status.SetPosition(hgPadX, 6)
	strip.Add(h.status)

	return y + hgFooterH
}

// buildTitleBar creates the draggable header strip and the close button.
//
// The title LABEL is a child of the bar so that grabbing the text drags too:
// g3n dispatches a mouse event to the panel under the cursor and then walks up
// the ancestry until something has a subscriber, so the label's mouse-down
// bubbles into the bar's handler.
//
// The close button, by contrast, is a child of the PANEL — a sibling of the
// bar, not a descendant. That keeps its mouse-down out of the bar's ancestry
// chain entirely, so clicking the X can never also begin a drag. (It matters:
// gui.Button consumes OnMouseUp, so a drag started by a mouse-down on the
// button would never receive the release that ends it, and the panel would
// stay glued to the cursor with the camera's event capture stolen.)
func (h *HoleGenPanel) buildTitleBar() {
	h.titleBar = gui.NewPanel(hgPanelW, hgTitleH)
	h.titleBar.SetColor(&hgTitleBg)
	h.titleBar.SetPosition(0, 0)
	h.titleBar.Subscribe(gui.OnMouseDown, h.onTitleMouseDown)
	h.titleBar.Subscribe(gui.OnMouseUp, h.onTitleMouseUp)
	h.titleBar.Subscribe(gui.OnCursor, h.onTitleCursor)
	h.Panel.Add(h.titleBar)

	// Accent stripe at the left end of the bar — cheap way to make the overlay
	// read as titled rather than as a floating grey rectangle.
	stripe := gui.NewPanel(4, hgTitleH)
	stripe.SetColor(&hgAccent)
	stripe.SetPosition(0, 0)
	h.titleBar.Add(stripe)

	title := gui.NewLabel("HoleGen")
	title.SetFontSize(hgTitleFontSize)
	title.SetColor(&hgTitleFg)
	title.SetPosition(14, 5)
	h.titleBar.Add(title)

	subtitle := gui.NewLabel("hole grid generator   ·   drag this bar to move")
	subtitle.SetFontSize(hgNoteFontSize)
	subtitle.SetColor(&hgNoteCol)
	subtitle.SetPosition(76, 9)
	h.titleBar.Add(subtitle)

	// Plain ASCII "X" rather than a glyph like ✕ — the bundled GUI font isn't
	// guaranteed to carry it, and a missing glyph renders as a blank box.
	closeBtn := gui.NewButton("X")
	closeBtn.SetSize(hgCloseW, hgTitleH-10)
	closeBtn.SetPosition(hgPanelW-hgCloseW-7, 5)
	closeBtn.Subscribe(gui.OnClick, func(string, interface{}) {
		h.SetVisible(false)
	})
	h.Panel.Add(closeBtn)
}

// hgAccentButtonStyles is the default button styling recoloured for the one
// primary action, so the button you press when you're finished stands out from
// the two you press while iterating.
//
// gui.StyleDefault() returns the shared theme, so the struct is COPIED before
// being modified — mutating it in place would restyle every button in the app.
func hgAccentButtonStyles() *gui.ButtonStyles {
	s := gui.StyleDefault().Button

	accent := math32.Color4{R: hgAccent.R, G: hgAccent.G, B: hgAccent.B, A: 1}
	hover := math32.Color4{R: 0.66, G: 0.79, B: 1.0, A: 1}
	press := math32.Color4{R: 0.44, G: 0.58, B: 0.82, A: 1}
	ink := math32.Color4{R: 0.07, G: 0.09, B: 0.13, A: 1}

	s.Normal.BgColor = accent
	s.Normal.FgColor = ink
	s.Normal.BorderColor = accent

	s.Over = s.Normal
	s.Over.BgColor = hover
	s.Over.BorderColor = hover

	s.Focus = s.Over

	s.Pressed = s.Normal
	s.Pressed.BgColor = press
	s.Pressed.BorderColor = press

	s.Disabled = s.Normal
	return &s
}

// hgEditStyles darkens the entries so they read as input wells against the
// panel instead of blending into it. Same copy-don't-mutate rule as above.
func hgEditStyles() *gui.EditStyles {
	s := gui.StyleDefault().Edit

	well := math32.Color4{R: 0.09, G: 0.10, B: 0.13, A: 1}
	border := math32.Color4{R: 0.30, G: 0.34, B: 0.41, A: 1}
	text := math32.Color4{R: 0.94, G: 0.96, B: 0.99, A: 1}

	s.Normal.BgColor = well
	s.Normal.BorderColor = border
	s.Normal.FgColor = text

	s.Over = s.Normal
	s.Over.BorderColor = math32.Color4{R: 0.42, G: 0.48, B: 0.58, A: 1}

	// Focused entry gets an accent outline — the only reliable "you are typing
	// here" cue available, since g3n draws no caret highlight of its own.
	s.Focus = s.Normal
	s.Focus.BorderColor = math32.Color4{R: hgAccent.R, G: hgAccent.G, B: hgAccent.B, A: 1}

	s.Disabled = s.Normal
	s.Disabled.FgColor = math32.Color4{R: 0.45, G: 0.47, B: 0.52, A: 1}
	return &s
}

func (h *HoleGenPanel) onTitleMouseDown(_ string, ev interface{}) {
	me, ok := ev.(*window.MouseEvent)
	if !ok || me.Button != window.MouseButtonLeft {
		return
	}
	h.dragging = true
	h.dragDX = me.Xpos - h.x
	h.dragDY = me.Ypos - h.y
	// Capture cursor events for the duration of the drag, the same trick the
	// orbiter uses: gui.Manager sends OnCursor exclusively to the focus target
	// and stops re-picking what's under the pointer, so the panel keeps
	// following even when the cursor outruns the title bar. It also means the
	// camera can't orbit mid-drag.
	gui.Manager().SetCursorFocus(h.titleBar)
}

func (h *HoleGenPanel) onTitleMouseUp(_ string, _ interface{}) {
	if !h.dragging {
		return
	}
	h.dragging = false
	gui.Manager().SetCursorFocus(nil)
}

func (h *HoleGenPanel) onTitleCursor(_ string, ev interface{}) {
	if !h.dragging {
		return
	}
	ce, ok := ev.(*window.CursorEvent)
	if !ok {
		return
	}
	h.moveTo(ce.Xpos-h.dragDX, ce.Ypos-h.dragDY)
}

// moveTo repositions the panel, clamped so the title bar stays reachable: it
// can't slide under the toolbar, off the bottom, or so far sideways that
// there's nothing left to grab.
func (h *HoleGenPanel) moveTo(x, y float32) {
	if h.winW > 0 {
		panelW, _ := h.Panel.Size()
		minX := -(panelW - hgKeepVisible)
		maxX := h.winW - hgKeepVisible
		if x < minX {
			x = minX
		}
		if x > maxX {
			x = maxX
		}
	}
	if h.winH > 0 {
		minY := float32(toolbarHeight)
		maxY := h.winH - hgTitleH
		if maxY < minY {
			maxY = minY
		}
		if y < minY {
			y = minY
		}
		if y > maxY {
			y = maxY
		}
	}
	h.x, h.y = x, y
	h.Panel.SetPosition(x, y)
}

// Resize records the window size for drag clamping and pulls the panel back
// into view if the window shrank out from under it.
func (h *HoleGenPanel) Resize(winW, winH float32) {
	h.winW, h.winH = winW, winH
	h.moveTo(h.x, h.y)
}

// buildPresetBar lays out: Preset: [dropdown] Name: [entry] [Save] [Delete]
//
// The spec's save flow prompts for a name in a modal text dialog. sqweek/dialog
// (the only dialog library already vendored here) has no text-entry dialog, and
// spawning a second OS window is explicitly out of scope, so the name lives in
// an inline entry instead. Everything else about the flow — pre-filling with the
// current selection, rejecting a blank name, confirming an overwrite — is
// unchanged.
func (h *HoleGenPanel) buildPresetBar(y float32) float32 {
	lbl := gui.NewLabel("Preset")
	lbl.SetFontSize(hgLabelFontSize)
	lbl.SetColor(&hgLabelCol)
	lbl.SetPosition(hgPadX, y+4)
	h.Panel.Add(lbl)

	h.prsDrop = gui.NewDropDown(150, gui.NewImageLabel(""))
	h.prsDrop.SetPosition(64, y)
	h.prsDrop.Subscribe(gui.OnChange, func(string, interface{}) {
		if h.suppress {
			return
		}
		h.loadSelectedPreset()
	})
	h.Panel.Add(h.prsDrop)

	nameLbl := gui.NewLabel("Name")
	nameLbl.SetFontSize(hgLabelFontSize)
	nameLbl.SetColor(&hgLabelCol)
	nameLbl.SetPosition(228, y+4)
	h.Panel.Add(nameLbl)

	h.nameEdit = gui.NewEdit(150, "preset name")
	h.nameEdit.SetStyles(hgEditStyles())
	h.nameEdit.SetFontSize(hgLabelFontSize)
	h.nameEdit.SetPosition(272, y)
	h.Panel.Add(h.nameEdit)

	saveBtn := gui.NewButton("Save")
	saveBtn.SetSize(58, buttonHeight)
	saveBtn.SetPosition(432, y)
	saveBtn.Subscribe(gui.OnClick, func(string, interface{}) { h.savePreset() })
	h.Panel.Add(saveBtn)

	delBtn := gui.NewButton("Delete")
	delBtn.SetSize(64, buttonHeight)
	delBtn.SetPosition(494, y)
	delBtn.Subscribe(gui.OnClick, func(string, interface{}) { h.deletePreset() })
	h.Panel.Add(delBtn)

	// Rule under the preset row, separating the "which parameter set" zone from
	// the parameters themselves.
	rule := gui.NewPanel(hgPanelW-2*hgPadX, 1)
	rule.SetColor(&hgRuleCol)
	rule.SetPosition(hgPadX, y+buttonHeight+7)
	h.Panel.Add(rule)

	return y + buttonHeight + 8
}

// buildFieldRow emits one label+entry row. index is the field's position in the
// full holegen.Fields list and drives the zebra tint, so the striping stays
// continuous across section boundaries.
//
// The stripe is added BEFORE the label and entry: sibling panels draw in child
// order, so anything added later lands on top.
func (h *HoleGenPanel) buildFieldRow(f holegen.Field, index int, rowY float32) {
	if index%2 == 1 {
		stripe := gui.NewPanel(hgPanelW-2*hgPadX, hgRowH)
		stripe.SetColor(&hgZebraBg)
		stripe.SetPosition(hgPadX, rowY)
		h.Panel.Add(stripe)
	}

	lbl := gui.NewLabel(f.Label)
	lbl.SetFontSize(hgLabelFontSize)
	lbl.SetColor(&hgLabelCol)
	lbl.SetPosition(hgPadX+6, rowY+5)
	h.Panel.Add(lbl)

	ed := gui.NewEdit(hgEditW, f.Default)
	ed.SetStyles(hgEditStyles())
	ed.SetText(f.Default)
	ed.SetFontSize(hgLabelFontSize)
	ed.SetPosition(hgEditX, rowY+1)
	// gui.Edit dispatches OnChange on every keystroke (and on backspace /
	// delete), but NOT on SetText — so programmatic fills below don't
	// recurse through here.
	ed.Subscribe(gui.OnChange, func(string, interface{}) {
		if h.suppress {
			return
		}
		h.refreshEstimate()
	})
	h.Panel.Add(ed)
	h.edits[f.Key] = ed

	if f.Key == "targetHoleDiameter" {
		h.buildDiameterDropdown(rowY + 1)
	}
}

// buildDiameterDropdown is the named-diameter quick-fill (spec §5). Selecting
// an option writes its raw value string into the diameter entry, which stays
// freely editable.
func (h *HoleGenPanel) buildDiameterDropdown(rowY float32) {
	h.diaDrop = gui.NewDropDown(hgDiaW, gui.NewImageLabel(""))
	h.diaDrop.SetPosition(hgDiaX, rowY)
	for _, p := range holegen.DiameterPresets {
		h.diaDrop.Add(gui.NewImageLabel(p.Option()))
	}
	h.diaDrop.Subscribe(gui.OnChange, func(string, interface{}) {
		if h.suppress {
			return
		}
		// Read by position rather than by the selected ImageLabel's text: the
		// position indexes DiameterPresets directly, so no string round-trip.
		pos := h.diaDrop.SelectedPos()
		if pos < 0 || pos >= len(holegen.DiameterPresets) {
			return
		}
		h.edits["targetHoleDiameter"].SetText(holegen.DiameterPresets[pos].Value)
		h.refreshEstimate()
	})
	h.Panel.Add(h.diaDrop)
}

// buildActionButtons places the action row: the two iterate-and-look actions on
// the left with the terminal one accented, and Close pushed to the right edge so
// it can't be hit by muscle memory aimed at Generate.
func (h *HoleGenPanel) buildActionButtons(y float32) float32 {
	preview := gui.NewButton("Preview in viewer")
	preview.SetSize(140, hgBtnRowH)
	preview.SetPosition(hgPadX, y)
	preview.Subscribe(gui.OnClick, func(string, interface{}) { h.preview() })
	h.Panel.Add(preview)

	generate := gui.NewButton("Generate .nc File")
	generate.SetStyles(hgAccentButtonStyles())
	generate.SetSize(150, hgBtnRowH)
	generate.SetPosition(hgPadX+148, y)
	generate.Subscribe(gui.OnClick, func(string, interface{}) { h.generate() })
	h.Panel.Add(generate)

	closeBtn := gui.NewButton("Close")
	closeBtn.SetSize(70, hgBtnRowH)
	closeBtn.SetPosition(hgPanelW-hgPadX-70, y)
	closeBtn.Subscribe(gui.OnClick, func(string, interface{}) { h.SetVisible(false) })
	h.Panel.Add(closeBtn)

	return y + hgBtnRowH
}

// Visible reports whether the overlay is currently shown.
func (h *HoleGenPanel) Visible() bool { return h.Panel.Visible() }

// SetVisible shows or hides the overlay.
func (h *HoleGenPanel) SetVisible(v bool) {
	if v {
		// Re-clamp: the window may have been resized while we were hidden.
		h.moveTo(h.x, h.y)
	} else if h.dragging {
		// Closing mid-drag (Escape, or the X while the button is held) must
		// release the cursor capture, or the camera never gets another cursor
		// event.
		h.dragging = false
		gui.Manager().SetCursorFocus(nil)
	}
	h.Panel.SetVisible(v)
}

// values snapshots the current raw entry text for every field.
func (h *HoleGenPanel) values() map[string]string {
	out := make(map[string]string, len(h.edits))
	for key, ed := range h.edits {
		out[key] = ed.Text()
	}
	return out
}

// readParams parses the form, reporting failures the way the spec asks
// (error dialog titled "Invalid input").
func (h *HoleGenPanel) readParams() (holegen.Params, bool) {
	p, err := holegen.ReadParams(h.values())
	if err != nil {
		ShowError("Invalid input", "%v", err)
		return holegen.Params{}, false
	}
	return p, true
}

// refreshEstimate updates the live "N holes | Ø.. | ~time" line.
//
// A parse error means the user is mid-edit (an entry momentarily holding "1."
// or ""), so we return without touching the label — the last good line stays
// on screen rather than flashing an error, per spec §12.
func (h *HoleGenPanel) refreshEstimate() {
	p, err := holegen.ReadParams(h.values())
	if err != nil {
		return
	}
	h.estimate.SetText(holegen.Summary(p))
}

func (h *HoleGenPanel) setStatus(format string, args ...interface{}) {
	h.status.SetText(fmt.Sprintf(format, args...))
}

// preview generates in memory and hands the program to the viewer without
// touching the disk — the fast iterate-and-look loop. The panel stays open so
// the user can tweak a number and preview again.
func (h *HoleGenPanel) preview() {
	p, ok := h.readParams()
	if !ok {
		return
	}
	text, cutRadius, totalDepth, err := holegen.Program(p)
	if err != nil {
		ShowError("Invalid input", "%v", err)
		return
	}
	h.load(p, text, "HoleGen preview")
	h.setStatus("Previewing %d holes  |  cut radius %.3fmm  |  depth %.2fmm  |  est. run time ~%s",
		p.RowCount*p.ColumnCount, cutRadius, totalDepth,
		holegen.FormatDuration(holegen.EstimateRuntime(p)))
}

// generate runs the full spec §13 flow: validate, save-file dialog, write,
// then report. It also previews the result, since the whole point of running
// this inside the viewer is to see what you just made.
func (h *HoleGenPanel) generate() {
	p, ok := h.readParams()
	if !ok {
		return
	}
	text, cutRadius, totalDepth, err := holegen.Program(p)
	if err != nil {
		ShowError("Invalid input", "%v", err)
		return
	}

	path, err := SaveGCodeFile("holes.nc")
	if err != nil {
		ShowError("Save failed", "Could not open the save dialog:\n%v", err)
		return
	}
	if path == "" {
		return // cancelled
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		ShowError("Save failed", "%v", err)
		return
	}

	est := holegen.FormatDuration(holegen.EstimateRuntime(p))
	h.refreshEstimate()
	h.setStatus("Saved %d holes  |  cut radius %.3fmm  |  depth %.2fmm  |  est. run time ~%s",
		p.RowCount*p.ColumnCount, cutRadius, totalDepth, est)

	ShowInfo("Done", "G-code file generated:\n%s\n\nEstimated run time: ~%s\n(rapids assumed at 3000 mm/min)",
		path, est)

	h.load(p, text, filepath.Base(path))
	h.Panel.SetVisible(false)
}

// load hands a generated program to the viewer for preview.
func (h *HoleGenPanel) load(p holegen.Params, text, displayName string) {
	if h.cb.OnLoadProgram == nil {
		return
	}
	h.cb.OnLoadProgram(HoleGenProgram{
		Text:        text,
		DisplayName: displayName,
		BitDiameter: p.BitDiameter,
		Thickness:   p.TubeThickness,
	})
}

// ── presets (spec §10.4) ──

// refreshPresetDropdown rebuilds the dropdown from h.presets, then re-selects
// `selectName` if it still exists.
//
// gui.DropDown has no Clear() and no way to return to "nothing selected", so
// items are removed from the end and the whole rebuild runs under suppress to
// keep the OnChange it fires from triggering a load.
func (h *HoleGenPanel) refreshPresetDropdown(selectName string) {
	h.suppress = true
	defer func() { h.suppress = false }()

	for i := h.prsDrop.Len() - 1; i >= 0; i-- {
		h.prsDrop.RemoveAt(i)
	}
	h.presetNames = h.presets.Names()
	for _, name := range h.presetNames {
		h.prsDrop.Add(gui.NewImageLabel(name))
	}
	for i, name := range h.presetNames {
		if name == selectName {
			h.prsDrop.SelectPos(i)
			h.nameEdit.SetText(name)
			return
		}
	}
}

// selectedPresetName returns the currently selected preset name, or "".
func (h *HoleGenPanel) selectedPresetName() string {
	pos := h.prsDrop.SelectedPos()
	if pos < 0 || pos >= len(h.presetNames) {
		return ""
	}
	return h.presetNames[pos]
}

// loadSelectedPreset fills the form from the selected preset. Only keys the
// preset actually stores are written, so a preset saved before a field existed
// leaves that field at whatever it currently holds.
func (h *HoleGenPanel) loadSelectedPreset() {
	name := h.selectedPresetName()
	if name == "" {
		return
	}
	stored := h.presets[name]
	if len(stored) == 0 {
		return
	}
	for key, value := range stored {
		if ed, ok := h.edits[key]; ok {
			ed.SetText(value)
		}
	}
	h.nameEdit.SetText(name)
	h.refreshEstimate()
	h.setStatus("Loaded preset '%s'.", name)
}

// savePreset validates every field first — a preset must never capture invalid
// input — then stores the RAW entry strings so a value typed as "1.125in"
// round-trips as inches.
func (h *HoleGenPanel) savePreset() {
	if _, ok := h.readParams(); !ok {
		return
	}

	name := strings.TrimSpace(h.nameEdit.Text())
	if name == "" {
		ShowError("Save preset", "Please enter a name.")
		return
	}
	if _, exists := h.presets[name]; exists {
		if !Confirm("Overwrite preset",
			"A preset named '%s' already exists. Overwrite it?", name) {
			return
		}
	}

	if h.presets == nil {
		h.presets = holegen.Presets{}
	}
	h.presets[name] = holegen.Values(h.values())
	if err := holegen.SavePresets(h.presets); err != nil {
		ShowError("Save preset", "Could not write presets:\n%v", err)
		return
	}

	h.refreshPresetDropdown(name)
	h.setStatus("Saved preset '%s'.", name)
	ShowInfo("Save preset", "Preset '%s' was saved to:\n%s", name, holegen.PresetsPath())
}

func (h *HoleGenPanel) deletePreset() {
	name := h.selectedPresetName()
	if name == "" {
		ShowInfo("Delete preset", "Select a preset to delete first.")
		return
	}
	if !Confirm("Delete preset", "Delete preset '%s'?", name) {
		return
	}
	delete(h.presets, name)
	if err := holegen.SavePresets(h.presets); err != nil {
		ShowError("Delete preset", "Could not write presets:\n%v", err)
		return
	}
	h.refreshPresetDropdown("")
	h.nameEdit.SetText("")
	h.setStatus("Deleted preset '%s'.", name)
}
