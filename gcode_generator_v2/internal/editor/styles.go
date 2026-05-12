package editor

import (
	"image/color"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// nineSlice returns a solid-color NineSlice. ebitenui requires NineSlice
// images for backgrounds; constructing them programmatically avoids
// bundling png assets just for flat fills.
func nineSlice(c color.Color) *image.NineSlice {
	return image.NewNineSliceColor(c)
}

// borderedNineSlice returns a 3x3 NineSlice with a 1-pixel inner border
// of `border` around a `fill` interior. The border preserves at all
// stretched sizes because ebitenui only stretches the center cell.
func borderedNineSlice(fill, border color.Color) *image.NineSlice {
	const sz = 3
	src := ebiten.NewImage(sz, sz)
	src.Fill(fill)
	for i := 0; i < sz; i++ {
		src.Set(i, 0, border)
		src.Set(i, sz-1, border)
		src.Set(0, i, border)
		src.Set(sz-1, i, border)
	}
	return image.NewNineSlice(src, [3]int{1, 1, 1}, [3]int{1, 1, 1})
}

// buttonImage is the standard chrome for secondary buttons.
func buttonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:    borderedNineSlice(btnIdle, sectionDiv),
		Hover:   borderedNineSlice(btnHover, sectionDiv),
		Pressed: borderedNineSlice(btnPressed, sectionDiv),
	}
}

// primaryButtonImage is the call-to-action chrome (Generate G-code).
func primaryButtonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:    nineSlice(primaryIdle),
		Hover:   nineSlice(primaryHover),
		Pressed: nineSlice(primaryPressed),
	}
}

// activePresetButtonImage is the highlighted state for the currently
// selected material preset.
func activePresetButtonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:    nineSlice(activeChip),
		Hover:   nineSlice(activeChip),
		Pressed: nineSlice(activeChip),
	}
}

// deleteButtonImage is the soft-red chrome used for the op-list ✕ button.
func deleteButtonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:    borderedNineSlice(deleteIdle, sectionDiv),
		Hover:   borderedNineSlice(deleteHover, sectionDiv),
		Pressed: borderedNineSlice(deleteHover, sectionDiv),
	}
}

func textInputImage() *widget.TextInputImage {
	return &widget.TextInputImage{
		Idle:     borderedNineSlice(bgInput, inputBorder),
		Disabled: borderedNineSlice(bgInputDis, inputBorder),
	}
}

func textInputColor() *widget.TextInputColor {
	return &widget.TextInputColor{
		Idle:          textPrimary,
		Disabled:      textMuted,
		Caret:         textPrimary,
		DisabledCaret: textMuted,
	}
}

// checkboxImage builds a two-state checkbox using bordered nine-slices.
func checkboxImage() *widget.CheckboxImage {
	unchecked := borderedNineSlice(bgInput, inputBorder)
	checked := nineSlice(activeChip)
	hover := borderedNineSlice(btnHover, inputBorder)
	greyed := borderedNineSlice(bgInputDis, inputBorder)
	return &widget.CheckboxImage{
		Unchecked:         unchecked,
		UncheckedHovered:  hover,
		UncheckedDisabled: greyed,
		Checked:           checked,
		CheckedHovered:    checked,
		CheckedDisabled:   greyed,
		Greyed:            greyed,
		GreyedHovered:     greyed,
		GreyedDisabled:    greyed,
	}
}

func textColor() *widget.LabelColor {
	return &widget.LabelColor{
		Idle:     textPrimary,
		Disabled: textMuted,
	}
}

// sliderTrackImage is the background bar for image-trace sliders.
func sliderTrackImage() *widget.SliderTrackImage {
	return &widget.SliderTrackImage{
		Idle:     borderedNineSlice(bgInput, inputBorder),
		Disabled: borderedNineSlice(bgInputDis, inputBorder),
	}
}

// sliderHandleImage is the draggable thumb on image-trace sliders.
// Reuses the standard secondary button chrome for visual consistency.
func sliderHandleImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:    borderedNineSlice(btnIdle, sectionDiv),
		Hover:   borderedNineSlice(btnHover, sectionDiv),
		Pressed: borderedNineSlice(btnPressed, sectionDiv),
	}
}
