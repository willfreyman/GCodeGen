package sim

import (
	"fmt"
	"image/color"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// buildUI constructs the bottom-of-window control strip: status text,
// progress bar, speed slider, play/pause and reset buttons. The sim
// canvas (rendered manually in Draw) sits above this strip; the
// container's MinSize math reserves space for the canvas.
func (g *Game) buildUI() *ebitenui.UI {
	root := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	bottom := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(bgWindow)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(4),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(8)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionStart,
				VerticalPosition:   widget.AnchorLayoutPositionEnd,
				StretchHorizontal:  true,
			}),
			widget.WidgetOpts.MinSize(WindowWidth, WindowHeight-(40+SimH)),
		),
	)

	// Status line.
	var faceIface text.Face = g.font
	face := &faceIface
	g.statusW = widget.NewText(
		widget.TextOpts.Text("Press  Play  to start", face, statusBg),
	)
	bottom.AddChild(g.statusW)

	// Progress bar.
	g.progress = widget.NewProgressBar(
		widget.ProgressBarOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(WindowWidth-24, 14),
		),
		widget.ProgressBarOpts.Images(
			&widget.ProgressBarImage{Idle: image.NewNineSliceColor(color.NRGBA{R: 0x1a, G: 0x1a, B: 0x3a, A: 0xff})},
			&widget.ProgressBarImage{Idle: image.NewNineSliceColor(color.NRGBA{R: 0x00, G: 0xd4, B: 0xff, A: 0xff})},
		),
		widget.ProgressBarOpts.Values(0, 100, 0),
	)
	bottom.AddChild(g.progress)

	// Speed + buttons row.
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
		)),
	)
	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Speed", face, statusOn),
	))
	g.speed = widget.NewSlider(
		widget.SliderOpts.MinMax(1, 30),
		widget.SliderOpts.InitialCurrent(6),
		widget.SliderOpts.WidgetOpts(widget.WidgetOpts.MinSize(180, 14)),
		widget.SliderOpts.Images(
			&widget.SliderTrackImage{Idle: image.NewNineSliceColor(color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff})},
			&widget.ButtonImage{
				Idle:    image.NewNineSliceColor(color.NRGBA{R: 0x00, G: 0xd4, B: 0xff, A: 0xff}),
				Hover:   image.NewNineSliceColor(color.NRGBA{R: 0x33, G: 0xe0, B: 0xff, A: 0xff}),
				Pressed: image.NewNineSliceColor(color.NRGBA{R: 0x00, G: 0xa0, B: 0xc0, A: 0xff}),
			},
		),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			g.mu.Lock()
			g.state.Speed = args.Current
			g.mu.Unlock()
		}),
	)
	row.AddChild(g.speed)
	g.playBtn = widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(color.NRGBA{R: 0x00, G: 0xd4, B: 0xff, A: 0xff}),
			Hover:   image.NewNineSliceColor(color.NRGBA{R: 0x33, G: 0xe0, B: 0xff, A: 0xff}),
			Pressed: image.NewNineSliceColor(color.NRGBA{R: 0x00, G: 0xa0, B: 0xc0, A: 0xff}),
		}),
		widget.ButtonOpts.Text("Play", face, &widget.ButtonTextColor{Idle: color.Black, Hover: color.Black}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(6)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			g.togglePlay()
		}),
	)
	row.AddChild(g.playBtn)
	row.AddChild(widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(color.NRGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}),
			Hover:   image.NewNineSliceColor(color.NRGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xff}),
			Pressed: image.NewNineSliceColor(color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}),
		}),
		widget.ButtonOpts.Text("Reset", face, &widget.ButtonTextColor{Idle: color.White, Hover: color.White}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(6)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			g.resetPlayback()
		}),
	))
	bottom.AddChild(row)

	root.AddChild(bottom)
	return &ebitenui.UI{Container: root}
}

// togglePlay starts, pauses, or restarts playback based on current state.
func (g *Game) togglePlay() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Done {
		g.resetPlaybackLocked()
		g.state.Running = true
		g.playBtn.Text().Label = "Pause"
		return
	}
	if g.state.Running {
		g.state.Running = false
		g.playBtn.Text().Label = "Resume"
	} else {
		g.state.Running = true
		g.playBtn.Text().Label = "Pause"
	}
}

func (g *Game) resetPlayback() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resetPlaybackLocked()
}

func (g *Game) resetPlaybackLocked() {
	g.state.reset()
	g.trails.Clear()
	g.progress.SetCurrent(0)
	g.statusW.Label = "Press  Play  to start"
	g.playBtn.Text().Label = "Play"
}

// refreshStatusAndProgress updates UI labels each frame to reflect
// playback state.
func (g *Game) refreshStatusAndProgress() {
	if g.statusW == nil || g.progress == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Done {
		g.statusW.Label = "Complete — all paths machined"
	} else if g.state.Running && g.state.OpIdx < len(g.state.Ops) {
		op := g.state.Ops[g.state.OpIdx]
		nextEnd := len(op.Pts) - 1
		g.statusW.Label = fmt.Sprintf("Op %d/%d: %s   pt %d/%d   z=%vmm",
			g.state.OpIdx+1, len(g.state.Ops), op.Name, g.state.PtIdx, nextEnd, op.Depth)
	}
	g.progress.SetCurrent(int(g.state.progress() * 100))
}
