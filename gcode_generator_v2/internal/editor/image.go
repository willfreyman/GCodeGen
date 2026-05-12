package editor

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gcodegen.local/generator/internal/gen"
	"gcodegen.local/generator/internal/img"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/sqweek/dialog"
)

// imageState owns everything related to the loaded background image:
// the decoded source, the cached binarized mask, the rendered preview
// the canvas blits each frame, and the user's trace-mode knobs.
type imageState struct {
	src image.Image // decoded source (nil = nothing loaded)
	bin *img.Binary // cached binarize result at current Threshold
	prv *ebiten.Image // preview rendered from bin, scaled-blitted onto canvas

	Threshold      uint8 // 0..255
	Mode           img.Mode
	IncludeInner   bool
	SimplifyEps    float64
	MinPoints      int
	previewDirty   bool
}

// newImageState returns sensible defaults: a 128 cutoff, contour mode,
// 1.5-pixel simplify epsilon, drops polylines shorter than 8 points.
func newImageState() *imageState {
	return &imageState{
		Threshold:    128,
		Mode:         img.ModeContour,
		IncludeInner: false,
		SimplifyEps:  1.5,
		MinPoints:    8,
	}
}

// hasImage reports whether the user has loaded anything.
func (s *imageState) hasImage() bool { return s != nil && s.src != nil }

// onLoadImageClicked pops a file-picker and loads the chosen image.
func (g *Game) onLoadImageClicked() {
	path, err := dialog.File().
		Filter("Images", "png", "jpg", "jpeg", "webp").
		Filter("All files", "*").
		Title("Load image to trace").
		Load()
	if err != nil {
		if err.Error() == "Cancelled" {
			return
		}
		showError("Load image", "%s", err.Error())
		return
	}
	im, err := img.LoadFile(path)
	if err != nil {
		showError("Load image", "%s", err.Error())
		return
	}
	g.image.src = im
	g.image.previewDirty = true
}

// onClearImageClicked removes the loaded image without touching strokes.
func (g *Game) onClearImageClicked() {
	g.image.src = nil
	g.image.bin = nil
	g.image.prv = nil
}

// rebuildImagePreviewIfDirty rebuilds the binary mask + ebiten preview
// when threshold or source changed. Called from Update() each frame.
func (g *Game) rebuildImagePreviewIfDirty() {
	if !g.image.previewDirty || g.image.src == nil {
		return
	}
	g.image.bin = img.Binarize(g.image.src, g.image.Threshold)
	g.image.prv = buildPreviewImage(g.image.bin)
	g.image.previewDirty = false
}

// buildPreviewImage paints a translucent dark RGBA where the binary is
// foreground, fully transparent where it is background. Used as the
// "what will be traced" indicator on the canvas.
func buildPreviewImage(bin *img.Binary) *ebiten.Image {
	rgba := image.NewRGBA(image.Rect(0, 0, bin.W, bin.H))
	fg := color.RGBA{R: 0x10, G: 0x10, B: 0x14, A: 0xc0}
	for y := 0; y < bin.H; y++ {
		for x := 0; x < bin.W; x++ {
			if bin.Pix[y*bin.W+x] {
				rgba.SetRGBA(x, y, fg)
			}
		}
	}
	return ebiten.NewImageFromImage(rgba)
}

// imageFitRect computes where in canvas pixel coords the source image
// renders. Fits inside the perim rectangle (preserving aspect ratio,
// centered). Returns a zero rect when there's no source or the perim
// is degenerate.
func (g *Game) imageFitRect() (x0, y0, x1, y1 float64, ok bool) {
	if !g.image.hasImage() {
		return 0, 0, 0, 0, false
	}
	b := g.image.src.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw == 0 || ih == 0 {
		return 0, 0, 0, 0, false
	}
	p := g.editor.Perim
	px0 := math.Min(p.X0, p.X1)
	py0 := math.Min(p.Y0, p.Y1)
	px1 := math.Max(p.X0, p.X1)
	py1 := math.Max(p.Y0, p.Y1)
	pw, ph := px1-px0, py1-py0
	if pw <= 1 || ph <= 1 {
		return 0, 0, 0, 0, false
	}
	imgAspect := float64(iw) / float64(ih)
	perimAspect := pw / ph
	var w, h float64
	if imgAspect > perimAspect {
		w = pw
		h = w / imgAspect
	} else {
		h = ph
		w = h * imgAspect
	}
	x := px0 + (pw-w)/2
	y := py0 + (ph-h)/2
	return x, y, x + w, y + h, true
}

// onTraceClicked runs the configured trace pipeline against the
// current image+threshold, maps polylines to canvas coords, and
// appends each as a new Stroke at the current depth/color.
func (g *Game) onTraceClicked() {
	if !g.image.hasImage() {
		showError("Trace image", "Load an image first.")
		return
	}
	g.rebuildImagePreviewIfDirty()
	x0, y0, x1, y1, ok := g.imageFitRect()
	if !ok {
		showError("Trace image", "Set the perimeter size before tracing.")
		return
	}
	opts := img.TraceOptions{
		Threshold:    g.image.Threshold,
		Mode:         g.image.Mode,
		IncludeInner: g.image.IncludeInner,
		SimplifyEps:  g.image.SimplifyEps,
		MinPoints:    g.image.MinPoints,
	}
	// Run trace on the cached binary directly to avoid binarize-twice.
	var polylines [][]image.Point
	switch opts.Mode {
	case img.ModeCenterline:
		polylines = img.Centerlines(g.image.bin)
	default:
		polylines = img.Contours(g.image.bin, opts.IncludeInner)
	}
	if opts.SimplifyEps > 0 {
		for i, p := range polylines {
			polylines[i] = img.RDPSimplify(p, opts.SimplifyEps)
		}
	}
	if opts.MinPoints > 0 {
		filtered := polylines[:0]
		for _, p := range polylines {
			if len(p) >= opts.MinPoints {
				filtered = append(filtered, p)
			}
		}
		polylines = filtered
	}
	if len(polylines) == 0 {
		showError("Trace image", "No traceable contours at this threshold. Try a different threshold or image.")
		return
	}
	// Map image pixel coords -> canvas pixel coords using the fit rect.
	b := g.image.src.Bounds()
	iw, ih := float64(b.Dx()), float64(b.Dy())
	rectW := x1 - x0
	rectH := y1 - y0
	added := 0
	for _, poly := range polylines {
		pts := make([]gen.Point, 0, len(poly))
		for _, p := range poly {
			pts = append(pts, gen.Point{
				X: x0 + float64(p.X)/iw*rectW,
				Y: y0 + float64(p.Y)/ih*rectH,
			})
		}
		stroke := gen.Stroke{
			Points: pts,
			Name:   fmt.Sprintf("Trace %d", len(g.editor.Strokes)+1),
			Depth:  g.editor.NewOpDepth,
			Color:  g.editor.CurrentColor(),
		}
		g.editor.Strokes = append(g.editor.Strokes, stroke)
		g.editor.ColorIdx = (g.editor.ColorIdx + 1) % len(gen.Palette)
		added++
	}
	g.strokesDirty = true
}

// buildImageSection adds the right-panel section that owns image
// loading + trace controls. Slot it between New Operation and the
// op list so it sits visually near where its outputs will appear.
func (g *Game) buildImageSection(heading, face, small *text.Face) widget.PreferredSizeLocateableWidget {
	c := section("Image trace", heading)

	c.AddChild(standardButton(face, "Load image…", g.onLoadImageClicked))

	// Threshold slider + live numeric label.
	var thrLabel *widget.Text
	thrLabel = widget.NewText(widget.TextOpts.Text(
		fmt.Sprintf("Threshold: %d", g.image.Threshold), face, textSecondary))
	thrSlider := widget.NewSlider(
		widget.SliderOpts.MinMax(0, 255),
		widget.SliderOpts.InitialCurrent(int(g.image.Threshold)),
		widget.SliderOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, 16)),
		widget.SliderOpts.Images(sliderTrackImage(), sliderHandleImage()),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			g.image.Threshold = uint8(args.Current)
			g.image.previewDirty = true
			thrLabel.Label = fmt.Sprintf("Threshold: %d", g.image.Threshold)
		}),
	)
	c.AddChild(thrLabel)
	c.AddChild(thrSlider)

	// Mode toggle: single button whose label flips between modes.
	var modeBtn *widget.Button
	modeLabel := func() string {
		if g.image.Mode == img.ModeCenterline {
			return "Mode: Centerline"
		}
		return "Mode: Contour"
	}
	modeBtn = standardButton(face, modeLabel(), func() {
		if g.image.Mode == img.ModeContour {
			g.image.Mode = img.ModeCenterline
		} else {
			g.image.Mode = img.ModeContour
		}
		modeBtn.SetText(modeLabel())
	})
	c.AddChild(modeBtn)

	c.AddChild(widget.NewCheckbox(
		widget.CheckboxOpts.Image(checkboxImage()),
		widget.CheckboxOpts.Text("Include inner contours", face, textColor()),
		widget.CheckboxOpts.Spacing(8),
		widget.CheckboxOpts.WidgetOpts(widget.WidgetOpts.MinSize(20, 20)),
		widget.CheckboxOpts.StateChangedHandler(func(args *widget.CheckboxChangedEventArgs) {
			g.image.IncludeInner = args.State == widget.WidgetChecked
		}),
	))

	// Simplify slider: 0..50 represents 0.0..5.0 px tolerance.
	var simpLabel *widget.Text
	simpLabel = widget.NewText(widget.TextOpts.Text(
		fmt.Sprintf("Simplify: %.1f px", g.image.SimplifyEps), face, textSecondary))
	simpSlider := widget.NewSlider(
		widget.SliderOpts.MinMax(0, 50),
		widget.SliderOpts.InitialCurrent(int(g.image.SimplifyEps*10)),
		widget.SliderOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, 16)),
		widget.SliderOpts.Images(sliderTrackImage(), sliderHandleImage()),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			g.image.SimplifyEps = float64(args.Current) / 10.0
			simpLabel.Label = fmt.Sprintf("Simplify: %.1f px", g.image.SimplifyEps)
		}),
	)
	c.AddChild(simpLabel)
	c.AddChild(simpSlider)

	c.AddChild(widget.NewText(widget.TextOpts.Text(
		"Threshold dims pixels darker than this. Trace turns them into strokes.",
		small, textMuted)))

	c.AddChild(primaryButton(face, "Trace", g.onTraceClicked))
	c.AddChild(standardButton(face, "Clear image", g.onClearImageClicked))
	return c
}

// drawImagePreview blits the cached preview image onto dst, scaled to
// the fit rect. Called from drawCanvas before strokes so the image
// reads as a background layer.
func (g *Game) drawImagePreview(dst *ebiten.Image) {
	if g.image.prv == nil {
		return
	}
	x0, y0, x1, y1, ok := g.imageFitRect()
	if !ok {
		return
	}
	b := g.image.prv.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return
	}
	sx := (x1 - x0) / float64(b.Dx())
	sy := (y1 - y0) / float64(b.Dy())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(sx, sy)
	op.GeoM.Translate(x0, y0)
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(g.image.prv, op)
}
