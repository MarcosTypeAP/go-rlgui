package gui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	_ "unsafe"
)

// Color is an alias for [rl.Color].
type Color = rl.Color

// Vec2 is a short alias for [rl.NewVector2].
//
//go:linkname Vec2 github.com/gen2brain/raylib-go/raylib.NewVector2
func Vec2(x, y float32) rl.Vector2

var _ = rl.NewVector2 // type check

// Rect is a short alias for [rl.NewRectangle].
//
//go:linkname Rect github.com/gen2brain/raylib-go/raylib.NewRectangle
func Rect(x, y, width, height float32) rl.Rectangle

var _ = rl.NewRectangle // type check

// ColorHex is a short alias for [rl.GetColor].
//
//go:linkname ColorHex github.com/gen2brain/raylib-go/raylib.GetColor
func ColorHex(hexValue uint) rl.Color

var _ = rl.GetColor // type check

// Shrink is a short alias for [NewSizingShrink].
//
//go:linkname Shrink github.com/MarcosTypeAP/go-rlgui.NewSizingShrink
func Shrink(v ...float32) SizingProp

var _ = NewSizingShrink // type check

// Grow is a short alias for [NewSizingGrow].
//
//go:linkname Grow github.com/MarcosTypeAP/go-rlgui.NewSizingGrow
func Grow(v ...float32) SizingProp

var _ = NewSizingGrow // type check

// Fixed is a short alias for [NewSizingFixed].
//
//go:linkname Fixed github.com/MarcosTypeAP/go-rlgui.NewSizingFixed
func Fixed(size float32) SizingProp

var _ = NewSizingFixed // type check

// Percentage is a short alias for [NewSizingPercentage].
//
//go:linkname Percentage github.com/MarcosTypeAP/go-rlgui.NewSizingPercentage
func Percentage(percentage uint8, v ...float32) SizingProp

var _ = NewSizingPercentage // type check

// AspectRatio is a short alias for [NewSizingAspectRatio].
//
//go:linkname AspectRatio github.com/MarcosTypeAP/go-rlgui.NewSizingAspectRatio
func AspectRatio(ratio float32) SizingProp

var _ = NewSizingAspectRatio // type check

// GradientLinear is a short alias for [NewGradientLinear].
//
//go:linkname GradientLinear github.com/MarcosTypeAP/go-rlgui.NewGradientLinear
func GradientLinear(startColor, endColor rl.Color, angleDegrees float32) Gradient

var _ = NewGradientLinear // type check

// GradientRadial is a short alias for [NewGradientRadial].
//
// Deprecated: Unimplemented.
//
//go:linkname GradientRadial github.com/MarcosTypeAP/go-rlgui.NewGradientRadial
func GradientRadial(centerColor, edgeColor rl.Color) Gradient

var _ = NewGradientRadial // type check

// Padding is a short alias for [NewBoxSides].
//
//go:linkname Padding github.com/MarcosTypeAP/go-rlgui.NewBoxSides
func Padding(v ...float32) BoxSides

var _ = NewBoxSides // type check

// PaddingOverride is a short alias for [NewBoxSidesOverride].
//
//go:linkname PaddingOverride github.com/MarcosTypeAP/go-rlgui.NewBoxSidesOverride
func PaddingOverride(padding BoxSides, top, right, bottom, left float32) BoxSides

var _ = NewBoxSidesOverride // type check

// Border is a short alias for [NewBoxSides].
//
//go:linkname Border github.com/MarcosTypeAP/go-rlgui.NewBoxSides
func Border(v ...float32) BoxSides

var _ = NewBoxSides // type check

// BorderOverride is a short alias for [NewBoxSidesOverride].
//
//go:linkname BorderOverride github.com/MarcosTypeAP/go-rlgui.NewBoxSidesOverride
func BorderOverride(borders BoxSides, top, right, bottom, left float32) BoxSides

var _ = NewBoxSidesOverride // type check

// Radius is a short alias for [NewBoxCorners].
//
//go:linkname Radius github.com/MarcosTypeAP/go-rlgui.NewBoxCorners
func Radius(v ...float32) BoxCorners

var _ = NewBoxCorners // type check

// RadiusOverride is a short alias for [NewBoxCornersOverride].
//
//go:linkname RadiusOverride github.com/MarcosTypeAP/go-rlgui.NewBoxCornersOverride
func RadiusOverride(radius BoxCorners, topLeft, topRight, bottomRight, bottomLeft float32) BoxCorners

var _ = NewBoxCornersOverride // type check

const (
	// Horizontal is a short alias for [OrientationHorizontal].
	Horizontal = OrientationHorizontal

	// Vertical is a short alias for [OrientationVertical].
	Vertical = OrientationVertical
)

const (
	// NoWrap is a short alias for [TextNoWrap].
	NoWrap = TextNoWrap

	// Wrap is a short alias for [TextWrap].
	Wrap = TextWrap

	// EllipsisOverflow is a short alias for [TextEllipsisOverflow].
	EllipsisOverflow = TextEllipsisOverflow
)

const (
	// FitCover is a short alias for [TextureFitCover].
	FitCover = TextureFitCover

	// FitContain is a short alias for [TextureFitContain].
	FitContain = TextureFitContain
)

const (
	// Start is a short alias for [AlignStart].
	Start = AlignStart

	// End is a short alias for [AlignEnd].
	End = AlignEnd

	// Center is a short alias for [AlignCenter].
	Center = AlignCenter
)
