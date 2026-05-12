// Package preview implements the finished-product preview subprocess: a
// 580×620 Ebiten window that renders a simulated material surface based on
// the parent editor's strokes, machine settings, material preset, and bit
// diameter. Reads UpdateMessage JSON lines from stdin.
//
// Mirrors gcodegen.py:open_preview (line 567-716).
package preview

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"sync"
	"sync/atomic"

	"gcodegen.local/generator/internal/assets"
	"gcodegen.local/generator/internal/gen"
	"gcodegen.local/generator/internal/shared"

	"github.com/ebitenui/ebitenui"
	euimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

const (
	WindowWidth  = 700
	WindowHeight = 620
	WindowTitle  = "GcodeGen — Finished Product Preview"
)

var (
	bgWindow = color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}
	bgEmpty  = color.RGBA{R: 0x2a, G: 0x2a, B: 0x2a, A: 0xff}
	titleClr = color.RGBA{R: 0xf0, G: 0xc0, B: 0x40, A: 0xff}
	infoClr  = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
)

// State is the wire-state digest the preview consumes — strokes in
// pixel space + perim corners + perim mm dims so we can compute the
// bit/area ratio for the quality indicator.
type State struct {
	Strokes []gen.StrokeWire
	HasPerim bool
	PerimX0, PerimY0, PerimX1, PerimY1 float64
	PerimWMM, PerimHMM float64
	PerimCut bool
}

// Game is the Ebiten game for the preview subprocess.
type Game struct {
	mu      sync.Mutex
	state   State
	pending atomic.Pointer[gen.UpdateMessage]
	canvas  *ebiten.Image
	dirty   bool

	bitMM    float64
	material string

	ui      *ebitenui.UI
	bitW    *widget.Slider
	infoW   *widget.Text
	font    *text.GoTextFace
	smallF  *text.GoTextFace
}

// Run boots the preview window and blocks until it closes.
func Run() {
	defer shared.RecoverAndLog("preview")
	fmt.Fprintln(os.Stderr, "gcodegen: starting preview subprocess")
	g := newGame()
	go func() {
		err := gen.ReadMessages(os.Stdin, func(m gen.UpdateMessage) {
			cp := m
			g.pending.Store(&cp)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "preview: stdin reader exited:", err)
		}
		os.Exit(0)
	}()
	ebiten.SetWindowSize(WindowWidth, WindowHeight)
	ebiten.SetWindowTitle(WindowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	if err := ebiten.RunGame(g); err != nil {
		fmt.Fprintln(os.Stderr, "preview exited:", err)
		os.Exit(1)
	}
}

func newGame() *Game {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(assets.FreeSansBoldTTF))
	if err != nil {
		fmt.Fprintln(os.Stderr, "preview: font load:", err)
	}
	g := &Game{
		canvas:   ebiten.NewImage(PreviewW, PreviewH),
		bitMM:    1.0,
		material: "Wood",
	}
	if src != nil {
		g.font = &text.GoTextFace{Source: src, Size: 12}
		g.smallF = &text.GoTextFace{Source: src, Size: 10}
	}
	g.ui = g.buildUI()
	g.dirty = true
	return g
}

func (g *Game) Update() error {
	if msg := g.pending.Swap(nil); msg != nil {
		g.applyMessage(*msg)
	}
	if g.ui != nil {
		g.ui.Update()
	}
	if g.dirty {
		g.rerender()
		g.dirty = false
	}
	return nil
}

func (g *Game) applyMessage(m gen.UpdateMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state.Strokes = append([]gen.StrokeWire(nil), m.Strokes...)
	if m.Perim != nil {
		g.state.HasPerim = true
		g.state.PerimX0 = m.Perim.X0
		g.state.PerimY0 = m.Perim.Y0
		g.state.PerimX1 = m.Perim.X1
		g.state.PerimY1 = m.Perim.Y1
		g.state.PerimWMM = m.Perim.WidthMM
		g.state.PerimHMM = m.Perim.HeightMM
		g.state.PerimCut = m.Perim.Cut
	}
	g.dirty = true
}

func (g *Game) rerender() {
	g.mu.Lock()
	defer g.mu.Unlock()
	renderSurface(g.canvas, &g.state, g.material, g.bitMM)
	if g.infoW != nil {
		pxmm := pixelsPerMM(&g.state)
		g.infoW.Label = fmt.Sprintf("Bit %.1fmm  |  Material: %s  |  Scale: %.1fpx/mm  |  Min feature ~ %.1fmm",
			g.bitMM, g.material, pxmm, g.bitMM)
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(bgWindow)
	// Title at top.
	if g.font != nil {
		op := &text.DrawOptions{}
		op.GeoM.Translate(12, 8)
		op.ColorScale.ScaleWithColor(titleClr)
		text.Draw(screen, "FINISHED PRODUCT PREVIEW", g.font, op)
	}
	// Canvas region: row 0 reserved for title (32px); canvas at y=70
	// after the controls row (32-70).
	canvasY := float32(70)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(0, float64(canvasY))
	screen.DrawImage(g.canvas, op)
	// Empty-state hint over the canvas.
	g.mu.Lock()
	hasContent := len(g.state.Strokes) > 0 || g.state.PerimCut
	g.mu.Unlock()
	if !hasContent && g.font != nil {
		op := &text.DrawOptions{}
		op.GeoM.Translate(WindowWidth/2-110, float64(canvasY)+PreviewH/2)
		op.ColorScale.ScaleWithColor(color.RGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff})
		text.Draw(screen, "Draw something to preview", g.font, op)
	}
	// Quality indicator overlay (bottom-right of canvas).
	g.mu.Lock()
	areaMM2 := g.state.PerimWMM * g.state.PerimHMM
	g.mu.Unlock()
	label, qcol := QualityIndicator(g.bitMM, areaMM2)
	if g.smallF != nil && hasContent {
		op := &text.DrawOptions{}
		s := fmt.Sprintf("Bit %.1fmm  |  %s", g.bitMM, label)
		// Right-align: rough estimate width = len*7 (small font)
		op.GeoM.Translate(float64(WindowWidth)-float64(len(s)*7)-8, float64(canvasY)+PreviewH-18)
		op.ColorScale.ScaleWithColor(qcol)
		text.Draw(screen, s, g.smallF, op)
	}
	if g.ui != nil {
		g.ui.Draw(screen)
	}
}

func (g *Game) Layout(_, _ int) (int, int) { return WindowWidth, WindowHeight }

// buildUI builds the top controls row (bit slider + material picker)
// and the bottom info label.
func (g *Game) buildUI() *ebitenui.UI {
	root := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	if g.font == nil {
		return &ebitenui.UI{Container: root}
	}
	var faceI text.Face = g.font
	face := &faceI

	// Top controls row at y=32.
	top := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(euimage.NewNineSliceColor(bgWindow)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(6),
			widget.RowLayoutOpts.Padding(&widget.Insets{Top: 32, Left: 8}),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionStart,
				VerticalPosition:   widget.AnchorLayoutPositionStart,
				StretchHorizontal:  true,
			}),
			widget.WidgetOpts.MinSize(WindowWidth, 38),
		),
	)
	top.AddChild(widget.NewText(
		widget.TextOpts.Text("Bit (mm)", face, infoClr),
	))
	g.bitW = widget.NewSlider(
		widget.SliderOpts.MinMax(1, 120), // 0.1 mm increments
		widget.SliderOpts.InitialCurrent(int(g.bitMM*10)),
		widget.SliderOpts.WidgetOpts(widget.WidgetOpts.MinSize(180, 14)),
		widget.SliderOpts.Images(
			&widget.SliderTrackImage{Idle: euimage.NewNineSliceColor(color.NRGBA{R: 0x44, G: 0x44, B: 0x44, A: 0xff})},
			&widget.ButtonImage{
				Idle:    euimage.NewNineSliceColor(color.NRGBA{R: 0xf0, G: 0xc0, B: 0x40, A: 0xff}),
				Hover:   euimage.NewNineSliceColor(color.NRGBA{R: 0xff, G: 0xd0, B: 0x60, A: 0xff}),
				Pressed: euimage.NewNineSliceColor(color.NRGBA{R: 0xc0, G: 0xa0, B: 0x30, A: 0xff}),
			},
		),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			g.mu.Lock()
			g.bitMM = float64(args.Current) / 10.0
			g.dirty = true
			g.mu.Unlock()
		}),
	)
	top.AddChild(g.bitW)
	for _, name := range MaterialList {
		matName := name
		top.AddChild(widget.NewButton(
			widget.ButtonOpts.Image(&widget.ButtonImage{
				Idle:    euimage.NewNineSliceColor(color.NRGBA{R: 0x44, G: 0x44, B: 0x44, A: 0xff}),
				Hover:   euimage.NewNineSliceColor(color.NRGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xff}),
				Pressed: euimage.NewNineSliceColor(color.NRGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}),
			}),
			widget.ButtonOpts.Text(matName, face, &widget.ButtonTextColor{Idle: color.White, Hover: color.White}),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(3)),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				g.mu.Lock()
				g.material = matName
				g.dirty = true
				g.mu.Unlock()
			}),
		))
	}
	root.AddChild(top)

	// Bottom info label.
	bottom := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(euimage.NewNineSliceColor(bgWindow)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(6)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionStart,
				VerticalPosition:   widget.AnchorLayoutPositionEnd,
				StretchHorizontal:  true,
			}),
			widget.WidgetOpts.MinSize(WindowWidth, 26),
		),
	)
	g.infoW = widget.NewText(widget.TextOpts.Text("", face, infoClr))
	bottom.AddChild(g.infoW)
	root.AddChild(bottom)

	return &ebitenui.UI{Container: root}
}
