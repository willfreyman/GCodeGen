package img

import "image"

// Binary is a 1-bit foreground mask the size of the source image.
// Pix[y*W + x] is true when the pixel is foreground (dark, to be cut).
type Binary struct {
	W, H int
	Pix  []bool
}

// Get returns the value at (x, y), or false if out of bounds. The
// out-of-bounds-is-false convention simplifies neighborhood lookups at
// image edges.
func (b *Binary) Get(x, y int) bool {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return false
	}
	return b.Pix[y*b.W+x]
}

// Set writes (x, y). No-op if out of bounds.
func (b *Binary) Set(x, y int, v bool) {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return
	}
	b.Pix[y*b.W+x] = v
}

// Clone returns a deep copy.
func (b *Binary) Clone() *Binary {
	pix := make([]bool, len(b.Pix))
	copy(pix, b.Pix)
	return &Binary{W: b.W, H: b.H, Pix: pix}
}

// Binarize converts im to a 1-bit foreground mask using luma threshold.
// A pixel is foreground when its perceptual luma is < threshold. Alpha
// is multiplied in so transparent pixels are background regardless of
// underlying color (common in PNG clipart with cut-out alpha).
func Binarize(im image.Image, threshold uint8) *Binary {
	b := im.Bounds()
	w, h := b.Dx(), b.Dy()
	out := &Binary{W: w, H: h, Pix: make([]bool, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := im.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				continue
			}
			// Rec. 601 luma in 16-bit channels, then to 8-bit.
			luma16 := (299*r + 587*g + 114*bl) / 1000
			luma8 := uint8(luma16 >> 8)
			// Pre-multiply alpha: transparent pixels read as background.
			if a < 0xffff {
				// Blend with white: out = luma*a + 255*(1-a)
				af := float64(a) / 0xffff
				luma8 = uint8(float64(luma8)*af + 255*(1-af))
			}
			if luma8 < threshold {
				out.Pix[y*w+x] = true
			}
		}
	}
	return out
}
