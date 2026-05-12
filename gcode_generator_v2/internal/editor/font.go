package editor

import (
	"bytes"
	"log"

	"gcodegen.local/generator/internal/assets"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// fontSource is the parsed FreeSansBold TTF, loaded once at package
// init. Multiple FontFace instances at different sizes share this source.
var fontSource *text.GoTextFaceSource

func init() {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(assets.FreeSansBoldTTF))
	if err != nil {
		log.Printf("editor: failed to parse FreeSansBold: %v (text rendering disabled)", err)
		return
	}
	fontSource = src
}

// fontFace returns a text.Face at the requested point size; nil if the
// font failed to load at init.
func fontFace(size float64) text.Face {
	if fontSource == nil {
		return nil
	}
	return &text.GoTextFace{Source: fontSource, Size: size}
}
