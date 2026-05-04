package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
)

// Toolbar layout: two rows.
//
//	Row 1 (height = toolbarRowH):
//	  [Open .nc] [Play/Pause] [Reset] [R] | Speed: [---|---] 1.0x | Bit: [6.35] mm [Set]
//	Row 2 (height = toolbarRowH):
//	  [progress slider, full width — slidable to scrub]
const (
	toolbarRowH      = 32.0
	toolbarHeight    = toolbarRowH * 2
	toolbarPadding   = 6.0
	buttonHeight     = 22.0
	buttonGap        = 6.0
	buttonOpenW      = 100.0
	buttonTutorialsW = 100.0
	buttonPlayW      = 70.0
	buttonResetW     = 60.0
	buttonReframeW   = 24.0
	speedLabelW      = 50.0
	speedSliderW     = 160.0
	speedValueW      = 56.0
	bitLabelW        = 30.0
	bitEditW         = 60.0
	bitUnitW         = 26.0
	bitApplyW        = 44.0
	progressPaddingX = 6.0
	progressHeight   = 18.0
)

var toolbarBgColor = math32.Color{R: 0.18, G: 0.20, B: 0.24}

// ToolbarCallbacks bundles the user-action hooks.
type ToolbarCallbacks struct {
	OnOpen                     func()
	OnPlayPause                func()
	OnReset                    func()
	OnReframe                  func()
	OnSpeedChanged             func(speedMult float64)
	OnBitDiaApplied            func(diameter float64)
	OnProgressScrub            func(fraction float64)
	OnMaterialThicknessApplied func(mm float64) // 0 = no through-cut
	OnTutorialSelected         func(displayName string)
}

// Toolbar is the top control strip.
type Toolbar struct {
	Panel    *gui.Panel
	progress *gui.Slider
	speed    *gui.Slider
	speedVal *gui.Label
	bitEdit  *gui.Edit
	playBtn  *gui.Button

	optionsBtn   *gui.Button
	tutorialsBtn *gui.Button

	// OptionsPanel / TutorialsPanel are dropdowns that appear below the
	// toolbar when their button is clicked. Exposed so the caller can add
	// them to the scene root SEPARATELY from the toolbar Panel — child
	// panels are clipped to their parent's bounds, so the dropdown wouldn't
	// render (it'd be entirely outside the toolbar's 64-pixel rectangle).
	OptionsPanel   *gui.Panel
	TutorialsPanel *gui.Panel

	matEdit *gui.Edit

	cb ToolbarCallbacks

	// Suppress OnChange dispatch into callbacks while we set values
	// programmatically (e.g. animation tick advancing the progress slider).
	suppressEvents bool

	bitDiameter       float64
	materialThickness float64
}

// NewToolbar builds the two-row toolbar at the given width.
func NewToolbar(width float32, initialBitDia float64, callbacks ToolbarCallbacks) *Toolbar {
	tb := &Toolbar{cb: callbacks, bitDiameter: initialBitDia}

	tb.Panel = gui.NewPanel(width, toolbarHeight)
	tb.Panel.SetColor(&toolbarBgColor)
	tb.Panel.SetPosition(0, 0)

	yRow1 := float32((toolbarRowH - buttonHeight) / 2)
	x := float32(toolbarPadding)

	// --- Row 1: Open
	openBtn := gui.NewButton("Open file")
	openBtn.SetSize(buttonOpenW, buttonHeight)
	openBtn.SetPosition(x, yRow1)
	openBtn.Subscribe(gui.OnClick, func(string, interface{}) {
		if tb.cb.OnOpen != nil {
			tb.cb.OnOpen()
		}
	})
	tb.Panel.Add(openBtn)
	x += buttonOpenW + buttonGap

	// Tutorials ▾ — opens a dropdown listing the bundled .nc tutorials.
	// Embedded in the binary so first-time users have something to load
	// without hunting for files.
	tb.tutorialsBtn = gui.NewButton("Tutorials ▾")
	tb.tutorialsBtn.SetSize(buttonTutorialsW, buttonHeight)
	tb.tutorialsBtn.SetPosition(x, yRow1)
	tutorialsBtnX := x
	tb.tutorialsBtn.Subscribe(gui.OnClick, func(string, interface{}) {
		// Toggle Tutorials; close Options if it was open so they don't overlap.
		open := !tb.TutorialsPanel.Visible()
		tb.TutorialsPanel.SetVisible(open)
		if open && tb.OptionsPanel != nil {
			tb.OptionsPanel.SetVisible(false)
		}
	})
	tb.Panel.Add(tb.tutorialsBtn)
	x += buttonTutorialsW + buttonGap

	// Play / Pause toggle
	tb.playBtn = gui.NewButton("Play")
	tb.playBtn.SetSize(buttonPlayW, buttonHeight)
	tb.playBtn.SetPosition(x, yRow1)
	tb.playBtn.Subscribe(gui.OnClick, func(string, interface{}) {
		if tb.cb.OnPlayPause != nil {
			tb.cb.OnPlayPause()
		}
	})
	tb.Panel.Add(tb.playBtn)
	x += buttonPlayW + buttonGap

	// Reset
	resetBtn := gui.NewButton("Reset")
	resetBtn.SetSize(buttonResetW, buttonHeight)
	resetBtn.SetPosition(x, yRow1)
	resetBtn.Subscribe(gui.OnClick, func(string, interface{}) {
		if tb.cb.OnReset != nil {
			tb.cb.OnReset()
		}
	})
	tb.Panel.Add(resetBtn)
	x += buttonResetW + buttonGap

	// Reframe (R key equivalent)
	reframeBtn := gui.NewButton("R")
	reframeBtn.SetSize(buttonReframeW, buttonHeight)
	reframeBtn.SetPosition(x, yRow1)
	reframeBtn.Subscribe(gui.OnClick, func(string, interface{}) {
		if tb.cb.OnReframe != nil {
			tb.cb.OnReframe()
		}
	})
	tb.Panel.Add(reframeBtn)
	x += buttonReframeW + buttonGap*2

	// Speed: label + slider + value
	speedLabel := gui.NewLabel("Speed:")
	speedLabel.SetPosition(x, yRow1+3)
	tb.Panel.Add(speedLabel)
	x += speedLabelW

	tb.speed = gui.NewHSlider(speedSliderW, buttonHeight)
	tb.speed.SetPosition(x, yRow1)
	// Default value = 1× speed → slider position computed inverse-mapping.
	tb.speed.SetValue(speedToSliderValue(1.0))
	tb.speed.Subscribe(gui.OnChange, func(string, interface{}) {
		if tb.suppressEvents {
			return
		}
		mult := sliderValueToSpeed(tb.speed.Value())
		if tb.speedVal != nil {
			tb.speedVal.SetText(fmt.Sprintf("%.1fx", mult))
		}
		if tb.cb.OnSpeedChanged != nil {
			tb.cb.OnSpeedChanged(mult)
		}
	})
	tb.Panel.Add(tb.speed)
	x += speedSliderW + buttonGap

	tb.speedVal = gui.NewLabel("1.0x")
	tb.speedVal.SetPosition(x, yRow1+3)
	tb.Panel.Add(tb.speedVal)
	x += speedValueW + buttonGap*2

	// Bit dia: label + edit + units + Set button
	bitLabel := gui.NewLabel("Bit:")
	bitLabel.SetPosition(x, yRow1+3)
	tb.Panel.Add(bitLabel)
	x += bitLabelW

	tb.bitEdit = gui.NewEdit(int(bitEditW), "6.35")
	tb.bitEdit.SetText(strconv.FormatFloat(initialBitDia, 'f', -1, 64))
	tb.bitEdit.SetPosition(x, yRow1)
	tb.Panel.Add(tb.bitEdit)
	x += bitEditW + 2

	bitUnit := gui.NewLabel("mm")
	bitUnit.SetPosition(x, yRow1+3)
	tb.Panel.Add(bitUnit)
	x += bitUnitW

	bitApply := gui.NewButton("Set")
	bitApply.SetSize(bitApplyW, buttonHeight)
	bitApply.SetPosition(x, yRow1)
	bitApply.Subscribe(gui.OnClick, func(string, interface{}) {
		tb.commitBitDia()
	})
	tb.Panel.Add(bitApply)
	x += bitApplyW + buttonGap*2

	// Options ▾ — toggles a panel below the toolbar with extra controls
	// (currently: material thickness for through-cut).
	tb.optionsBtn = gui.NewButton("Options ▾")
	tb.optionsBtn.SetSize(90, buttonHeight)
	tb.optionsBtn.SetPosition(x, yRow1)
	optionsBtnX := x
	tb.optionsBtn.Subscribe(gui.OnClick, func(string, interface{}) {
		open := !tb.OptionsPanel.Visible()
		tb.OptionsPanel.SetVisible(open)
		if open && tb.TutorialsPanel != nil {
			tb.TutorialsPanel.SetVisible(false)
		}
	})
	tb.Panel.Add(tb.optionsBtn)

	// Build the dropdown options panel (initially hidden). It's positioned
	// in WINDOW coordinates (not relative to the toolbar) — the caller is
	// responsible for adding it to the scene root so it isn't clipped to
	// the toolbar's bounds.
	tb.OptionsPanel = buildOptionsPanel(optionsBtnX, toolbarHeight, tb)
	tb.TutorialsPanel = buildTutorialsPanel(tutorialsBtnX, toolbarHeight, tb)

	// Bit edit also commits on Enter and on focus loss.
	tb.bitEdit.Subscribe(window.OnKeyDown, func(_ string, ev interface{}) {
		if ke, ok := ev.(*window.KeyEvent); ok && ke.Key == window.KeyEnter {
			tb.commitBitDia()
		}
	})
	tb.bitEdit.Subscribe(gui.OnFocusLost, func(string, interface{}) {
		tb.commitBitDia()
	})

	// --- Row 2: progress slider, full width
	tb.progress = gui.NewHSlider(width-progressPaddingX*2, progressHeight)
	tb.progress.SetPosition(progressPaddingX, toolbarRowH+(toolbarRowH-progressHeight)/2)
	tb.progress.SetValue(0)
	tb.progress.SetText("0%")
	tb.progress.Subscribe(gui.OnChange, func(string, interface{}) {
		if tb.suppressEvents {
			return
		}
		if tb.cb.OnProgressScrub != nil {
			tb.cb.OnProgressScrub(float64(tb.progress.Value()))
		}
	})
	tb.Panel.Add(tb.progress)

	return tb
}

// buildOptionsPanel constructs the dropdown options pane shown below the
// toolbar when the Options button is clicked. Hosts the material-thickness
// edit + apply, plus room for future settings.
func buildOptionsPanel(x, y float32, tb *Toolbar) *gui.Panel {
	const (
		panelW = 280.0
		panelH = 90.0
	)
	panel := gui.NewPanel(panelW, panelH)
	panel.SetColor(&math32.Color{R: 0.22, G: 0.24, B: 0.28})
	panel.SetPosition(x, y)
	panel.SetVisible(false)

	matLabel := gui.NewLabel("Material thickness:")
	matLabel.SetPosition(8, 8)
	panel.Add(matLabel)

	tb.matEdit = gui.NewEdit(110, "0 = off")
	tb.matEdit.SetText("")
	tb.matEdit.SetPosition(8, 28)
	panel.Add(tb.matEdit)

	matUnit := gui.NewLabel("(mm or 0.75in / 0.75\")")
	matUnit.SetPosition(124, 32)
	panel.Add(matUnit)

	matApply := gui.NewButton("Apply")
	matApply.SetSize(54, buttonHeight)
	matApply.SetPosition(8, 56)
	matApply.Subscribe(gui.OnClick, func(string, interface{}) {
		tb.commitMaterialThickness()
	})
	panel.Add(matApply)

	hint := gui.NewLabel("Cuts deeper than this slice through")
	hint.SetPosition(70, 60)
	panel.Add(hint)

	// Commit on Enter or focus loss too, like the bit-diameter edit.
	tb.matEdit.Subscribe(gui.OnFocusLost, func(string, interface{}) {
		tb.commitMaterialThickness()
	})

	return panel
}

// buildTutorialsPanel constructs the dropdown shown when the Tutorials
// button is clicked. One button per Tutorial entry, vertically stacked.
// Click → fires OnTutorialSelected with the display name and closes the
// panel.
func buildTutorialsPanel(x, y float32, tb *Toolbar) *gui.Panel {
	const (
		panelW    = 240.0
		rowHeight = 26.0
		padTop    = 8.0
		padX      = 8.0
		padBottom = 8.0
	)
	count := float32(len(Tutorials))
	panelH := padTop + count*rowHeight + (count-1)*4 + padBottom

	panel := gui.NewPanel(panelW, panelH)
	panel.SetColor(&math32.Color{R: 0.22, G: 0.24, B: 0.28})
	panel.SetPosition(x, y)
	panel.SetVisible(false)

	rowY := float32(padTop)
	for _, t := range Tutorials {
		name := t.DisplayName // capture loop var
		btn := gui.NewButton(name)
		btn.SetSize(panelW-padX*2, rowHeight)
		btn.SetPosition(padX, rowY)
		btn.Subscribe(gui.OnClick, func(string, interface{}) {
			panel.SetVisible(false)
			if tb.cb.OnTutorialSelected != nil {
				tb.cb.OnTutorialSelected(name)
			}
		})
		panel.Add(btn)
		rowY += rowHeight + 4
	}
	return panel
}

// commitMaterialThickness parses the matEdit text. Accepts plain mm values
// ("19.05"), or inch values with "in" or `"` suffix ("0.75in", "0.75\"").
// Empty or 0 disables through-cut. Non-numeric input is rejected silently
// (edit reverts to last good value).
func (t *Toolbar) commitMaterialThickness() {
	raw := strings.TrimSpace(t.matEdit.Text())
	if raw == "" {
		t.materialThickness = 0
		if t.cb.OnMaterialThicknessApplied != nil {
			t.cb.OnMaterialThicknessApplied(0)
		}
		return
	}

	mm, ok := parseLengthMM(raw)
	if !ok || mm < 0 {
		// Revert to last-good display
		if t.materialThickness > 0 {
			t.matEdit.SetText(strconv.FormatFloat(t.materialThickness, 'f', -1, 64))
		} else {
			t.matEdit.SetText("")
		}
		return
	}
	t.materialThickness = mm
	if t.cb.OnMaterialThicknessApplied != nil {
		t.cb.OnMaterialThicknessApplied(mm)
	}
}

// parseLengthMM parses a length string. Supports mm (default) and inch
// suffixes "in" and `"`. Returns (mm, ok).
func parseLengthMM(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "mm") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "mm"))
		v, err := strconv.ParseFloat(s, 64)
		return v, err == nil
	}
	if strings.HasSuffix(s, "in") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "in"))
		v, err := strconv.ParseFloat(s, 64)
		return v * 25.4, err == nil
	}
	if strings.HasSuffix(s, `"`) {
		s = strings.TrimSpace(strings.TrimSuffix(s, `"`))
		v, err := strconv.ParseFloat(s, 64)
		return v * 25.4, err == nil
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

// commitBitDia parses the edit's current text and fires OnBitDiaApplied
// if the value is a valid positive number that differs from the current.
func (t *Toolbar) commitBitDia() {
	raw := strings.TrimSpace(t.bitEdit.Text())
	if raw == "" {
		return
	}
	d, err := strconv.ParseFloat(raw, 64)
	if err != nil || d <= 0 {
		// Restore last-good value
		t.bitEdit.SetText(strconv.FormatFloat(t.bitDiameter, 'f', -1, 64))
		return
	}
	if d == t.bitDiameter {
		return
	}
	t.bitDiameter = d
	if t.cb.OnBitDiaApplied != nil {
		t.cb.OnBitDiaApplied(d)
	}
}

// Resize updates the toolbar's width when the window resizes.
func (t *Toolbar) Resize(width float32) {
	t.Panel.SetSize(width, toolbarHeight)
	if t.progress != nil {
		t.progress.SetSize(width-progressPaddingX*2, progressHeight)
	}
}

// SetPlaying flips the play/pause button label.
func (t *Toolbar) SetPlaying(playing bool) {
	if playing {
		t.playBtn.Label.SetText("Pause")
	} else {
		t.playBtn.Label.SetText("Play")
	}
}

// SetProgress writes the slider position from playback (without dispatching
// OnChange back into the scrub callback).
func (t *Toolbar) SetProgress(fraction float64) {
	t.suppressEvents = true
	t.progress.SetValue(float32(fraction))
	t.progress.SetText(fmt.Sprintf("%.0f%%", fraction*100))
	t.suppressEvents = false
}

// speedToSliderValue / sliderValueToSpeed map between the slider's [0, 1]
// and a useful multiplier range. Exponential mapping so 1× sits roughly
// 1/3 of the way along and the high end reaches 50×.
//
//	value=0   → 0.5×
//	value≈0.15→ 1×
//	value=0.5 → ~5×
//	value=1   → 50×
const (
	speedMin = 0.5
	speedMax = 50.0
)

func sliderValueToSpeed(v float32) float64 {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	// 0.5 × (100^v) gives 0.5 … 50 when v is in [0, 1]
	return speedMin * math.Pow(speedMax/speedMin, float64(v))
}

func speedToSliderValue(mult float64) float32 {
	if mult <= speedMin {
		return 0
	}
	if mult >= speedMax {
		return 1
	}
	return float32(math.Log(mult/speedMin) / math.Log(speedMax/speedMin))
}
