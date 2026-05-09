// Package gui provides a semi-immediate-mode, box-model UI library for raylib-go.
//
// The library is built around a tree of [Node] values. Every frame the caller
// reconstructs the tree, configures sizing, colors, gradients, etc. through
// Props structs, and then computes the layout, updates the nodes, and finally
// renders the tree:
//
//	// create a sub window
//	sw := gui.AddSubWindow(gui.NewSubWindow(...), ...)
//
//	for !rl.WindowShouldClose() {
//		// clear the node tree
//		gui.ResetLayout()
//
//		// rebuild the tree with [AddChild] calls
//		root := sw.Root() // or sw.SetRoot(gui.NewBox(...))
//		childBtn := gui.AddChild(root, gui.NewButton(...))
//
//		// resolve all sizes and positions
//		gui.ComputeLayout()
//
//		// process input, nodes, and sub windows, and post-update callbacks
//		gui.Update()
//
//		// your logic
//		if childBtn.IsLeftButtonPressed() {
//			// do something
//		}
//
//		// draw everything and run post-render callbacks
//		rl.BeginDrawing()
//		gui.Render()
//		rl.EndDrawing()
//	}
//
// Stateful widgets (ScrollBox, Slider, Toggle, TextInput, Dropdown, etc.) must
// be given a [NodeID] so that their internal state is cached and survives
// across the rebuild, or set it to [NodeIDManual] to handle its lifecycle
// manually.
//
// Custom nodes can be defined by implementing the [Node] interface.
package gui

import (
	"embed"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"io/fs"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/MarcosTypeAP/go-assert"

	rl "github.com/gen2brain/raylib-go/raylib"
)

//go:embed assets/shaders/*
var shadersFS embed.FS

//go:embed assets/icons/*
var iconsFS embed.FS

// Debug enables per-node visual debugging overlays.
// When true, [DebuggingInfo] draws outline rectangles around every node's
// bounding box and inner (post-padding) rect, and prints node details to
// stdout when the node is left-clicked.
var Debug = false

// DebugLinesColor is the color used for the debug overlay rectangles drawn
// when [Debug] is true.
var DebugLinesColor = rl.ColorAlpha(rl.Red, 0.5)

// InfPositive is +Inf float32.
var InfPositive = float32(math.Inf(1))

// InfNegative is -Inf float32.
var InfNegative = float32(math.Inf(-1))

// SubWindowHeaderHeight is the pixel height of the draggable title-bar
// rendered above floating [*SubWindow]s.
var SubWindowHeaderHeight float32 = 20

// ScrollBoxSpeed is the number of pixels scrolled per mouse-wheel tick inside a
// [*ScrollBox].
var ScrollBoxSpeed float32 = 30

// ScrollBoxThumbMargin is the margin in pixels at sides of the thumb of a
// [*ScrollBox] for interacting with the mouse left-click.
var ScrollBoxThumbMargin float32 = 4

// DefaultScrollBoxThumbWidth is the default width (or height for a horizontal
// scroll bar) of a [*ScrollBox] thumb in pixels.
var DefaultScrollBoxThumbWidth float32 = 5

// DefaultSliderThumbWidth is the default width (or height for a horizontal
// slider) of a [*Slider] thumb in pixels.
var DefaultSliderThumbWidth float32 = 20

// DefaultSliderTrackWidth is the default width (or height for a horizontal
// slider) of a [*Slider] track in pixels.
var DefaultSliderTrackWidth float32 = 10

// DefaultFontSize is the font size used by [*Text] and [*TextInput] nodes when
// no explicit size is supplied in [FontConfigProps].
var DefaultFontSize float32 = 20

// DefaultFontBaselineCorrectionFactor is a multiplier applied to the font size
// to nudge the text baseline upward.  A value of 0 disables the correction.
// Increase it slightly if text appears too low inside its bounding box.
var DefaultFontBaselineCorrectionFactor float32 = 0

// DefaultCharSpacing is the default pixel gap between glyphs used by [*Text]
// and [*TextInput] nodes.
var DefaultCharSpacing float32 = 1

// DefaultTextColor is the foreground color applied to [*Text] and [*TextInput]
// nodes when no color is set in [FontConfigProps].
var DefaultTextColor = rl.Black

// DefaultErrorColor is the border and error-message color used by [*TextInput]
// when its ErrorMessage field is non-empty.
var DefaultErrorColor = rl.Red

// DropdownOptionsBoxMarginTop is the space between the selected item and the
// options box.
var DropdownOptionsBoxMarginTop float32 = 4

// DefaultDropdownExpandIcon is the texture shown on a [*Dropdown] when it is
// collapsed. It is initialised by [LoadAssets] from the bundled icon file and
// may be replaced before creating any Dropdown nodes.
var DefaultDropdownExpandIcon rl.Texture2D

// DefaultDropdownCollapseIcon is the texture shown on a [*Dropdown] when it is
// expanded. It is initialised by [LoadAssets] from the bundled icon file and
// may be replaced before creating any Dropdown nodes.
var DefaultDropdownCollapseIcon rl.Texture2D

// DefaultDropdownIconSizeFactor is a multiplier applied to the font size to
// get the width and height used when drawing the expand/collapse icons of a
// [*Dropdown].
var DefaultDropdownIconSizeFactor float32 = 0.7

// DefaultDropdownIconGap is the horizontal pixel gap between a [*Dropdown]'s
// icon and its label text.
var DefaultDropdownIconGap float32 = 15

// ImageTextureFilter is the texture-filter mode applied to images loaded via
// [LoadImage], [LoadImageTexture] and [NewBoxImage].
var ImageTextureFilter = rl.FilterBilinear

// FontTextureFilter is the texture-filter mode applied to fonts loaded via
// [LoadFont], [LoadFontFromMemory], and [LoadFontFS].
var FontTextureFilter = rl.FilterBilinear

// RectPosition returns the top-left corner of rect as a [rl.Vector2].
func RectPosition(rect rl.Rectangle) rl.Vector2 {
	return rl.Vector2{
		X: rect.X,
		Y: rect.Y,
	}
}

// RectSize returns the width and height of rect as a [rl.Vector2].
func RectSize(rect rl.Rectangle) rl.Vector2 {
	return rl.Vector2{
		X: rect.Width,
		Y: rect.Height,
	}
}

// Number is a type constraint that matches all integer and floating-point
// kinds. It is used by generic utility functions such as [Clamp] and [Abs].
type Number interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~float32 | ~float64
}

// Clamp returns v clamped to the closed interval [min, max].
func Clamp[N Number](v, min, max N) N {
	if v < min {
		v = min
	} else if v > max {
		v = max
	}
	return v
}

// Abs returns the absolute value of v.
func Abs[N Number](v N) N {
	return N(math.Abs(float64(v)))
}

// Must unwraps a (T, error) pair. It panics if err is non-nil.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// Must2 unwraps a (T1, T2, error) triple. It panics if err is non-nil.
func Must2[T1, T2 any](v1 T1, v2 T2, err error) (T1, T2) {
	if err != nil {
		panic(err)
	}
	return v1, v2
}

// Ternary returns a if condition is true, otherwise b.
// Both a and b are evaluated eagerly; use [TernaryLazy] to avoid that.
// Useful for [Node] construction.
func Ternary[T any](condition bool, a, b T) T {
	if condition {
		return a
	}
	return b
}

// TernaryLazy is like [Ternary] but only calls the chosen branch function,
// which avoids evaluating the other branch. Useful for [Node] construction.
func TernaryLazy[T any](condition bool, a, b func() T) T {
	if condition {
		return a()
	}
	return b()
}

// Pointer returns a pointer to value.
// This is a helper for taking the address of a literal.
//
// Deprecated: Use the built-in new(value) syntax if using go >= 1.26.0
func Pointer[T any](value T) *T {
	return &value
}

var ErrFontInvalid = errors.New("invalid font")

// LoadFont loads a font from disk and applies [FontTextureFilter] to it.
//
// fileName is the path to the font file, that must supported by raylib.
// fontSize controls the rasterisation size.
// codepointCount is the number of codepoints (starting at space, 0x20) to
// rasterise; pass 0 when supplying an explicit codepoints slice.
// codepoints is an optional list of Unicode codepoints to include.
// It is invalid to supply both a non-zero codepointCount and a non-empty
// codepoints slice.
func LoadFont(fileName string, fontSize int32, codepointCount int, codepoints ...rune) (rl.Font, error) {
	font := rl.LoadFontEx(fileName, fontSize, codepoints, int32(codepointCount))
	if !rl.IsFontValid(font) {
		return rl.Font{}, ErrFontInvalid
	}
	rl.SetTextureFilter(font.Texture, FontTextureFilter)
	return font, nil
}

// LoadFontFromMemory loads a font from an in-memory byte slice.
// fileExtension must be a supported extension by raylib.
// If codepointCount > 0 and no explicit codepoints are given, the first
// codepointCount codepoints (starting at space, 0x20) are used.
// It is invalid to supply both a non-zero codepointCount and a non-empty
// codepoints slice.
func LoadFontFromMemory(fileExtension string, fileData []byte, fontSize int32, codepointCount int, codepoints ...rune) (rl.Font, error) {
	assert.False(codepointCount > 0 && len(codepoints) > 0)

	if len(codepoints) == 0 && codepointCount > 0 {
		codepoints = make([]rune, codepointCount)
		for i := range codepointCount {
			codepoints[i] = rune(i + 32) // 32 = space
		}
	}

	font := rl.LoadFontFromMemory(fileExtension, fileData, fontSize, codepoints)
	if !rl.IsFontValid(font) {
		return rl.Font{}, ErrFontInvalid
	}
	rl.SetTextureFilter(font.Texture, FontTextureFilter)
	return font, nil
}

// LoadFontFS loads a font from an [fs.FS] (e.g. an embedded FS).
// filePath must be relative and include a supported extension by raylib.
// The remaining parameters are identical to [LoadFontFromMemory].
func LoadFontFS(fileSystem fs.FS, filePath string, fontSize int32, codepointCount int, codepoints ...rune) (rl.Font, error) {
	dotIdx := strings.LastIndex(filePath, ".")
	assert.NotEqual(dotIdx, -1)
	fileExtension := filePath[dotIdx:]

	file, err := fileSystem.Open(filePath)
	if err != nil {
		return rl.Font{}, fmt.Errorf("opening file system: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return rl.Font{}, fmt.Errorf("reading file: %w", err)
	}

	font, err := LoadFontFromMemory(fileExtension, data, fontSize, codepointCount, codepoints...)
	if err != nil {
		return rl.Font{}, fmt.Errorf("loading font: %w", err)
	}
	return font, nil
}

var _fallbackFont rl.Font
var _selectedFont rl.Font

// GetDefaultFont returns the font that will be used when a node does not
// specify one in its [FontConfigProps]. Falls back to the raylib default font
// if no font has been set via [SetDefaultFont].
func GetDefaultFont() rl.Font {
	if !rl.IsFontValid(_selectedFont) {
		_selectedFont = _fallbackFont
	}
	return _selectedFont
}

// SetDefaultFont replaces the font returned by [GetDefaultFont].
func SetDefaultFont(font rl.Font) {
	_selectedFont = font
}

// ColorContrast returns a brightened or darkened version of color.
// The adjustment direction is chosen automatically based on the perceived
// luminance of the color so that the result always has higher contrast against
// the original.
// factor controls the magnitude [0–1].
func ColorContrast(color rl.Color, factor float32) rl.Color {
	// source: I made it up
	if max(float32(color.R)*0.8, float32(color.G)*0.8, float32(color.B)*0.1) <= 128 {
		return rl.ColorBrightness(color, factor)
	} else {
		return rl.ColorBrightness(color, -factor)
	}
}

// ColorToUniformVec4 converts a [rl.Color] to a []float32 slice of length 4
// with each component normalised to [0, 1].  The slice is suitable for passing
// to shader uniform setters.
func ColorToUniformVec4(color rl.Color) []float32 {
	return []float32{
		float32(color.R) / 255,
		float32(color.G) / 255,
		float32(color.B) / 255,
		float32(color.A) / 255,
	}
}

func getFileData(fileSystem fs.FS, filepath string) []byte {
	file, err := fileSystem.Open(filepath)
	assert.NoError(err)
	defer file.Close()

	content, err := io.ReadAll(file)
	assert.NoError(err)

	return content
}

// Temporary (permanent) fix until next raylib-go release (https://github.com/gen2brain/raylib-go/issues/510)
func setUniform(locIndex int32, value any, uniformType, count int32) {
	var dataLength int32
	switch uniformType {
	case int32(rl.ShaderUniformFloat), int32(rl.ShaderUniformInt):
		dataLength = 1

	case int32(rl.ShaderUniformVec2):
		dataLength = 2

	case int32(rl.ShaderUniformVec3):
		dataLength = 3

	case int32(rl.ShaderUniformVec4):
		dataLength = 4

	default:
		assert.Unimplemented()
	}

	switch v := value.(type) {
	case []float32:
		assert.Equal(int32(len(v)), dataLength*count)
		rl.SetUniform(locIndex, v[:count], uniformType)

	case []int32:
		assert.Equal(int32(len(v)), dataLength*count)
		vf := make([]float32, len(v))
		for i := range len(vf) {
			vf[i] = math.Float32frombits(uint32(v[i]))
		}
		rl.SetUniform(locIndex, vf[:count], uniformType)

	default:
		assert.Unimplemented()
	}
}

func loadShader(vertexFilename, fragmentFilename string) rl.Shader {
	shadersDirFS, err := fs.Sub(shadersFS, "assets/shaders")
	assert.NoError(err)

	var vertexCode string
	if vertexFilename != "" {
		vertexCode = string(getFileData(shadersDirFS, vertexFilename))
	}

	var fragmentCode string
	if fragmentFilename != "" {
		fragmentCode = string(getFileData(shadersDirFS, fragmentFilename))
	}

	return rl.LoadShaderFromMemory(vertexCode, fragmentCode)
}

const (
	ShaderRenderModeSolidColor int32 = iota
	ShaderRenderModeGradientLinear
	ShaderRenderModeGradientRadial // Deprecated: unimplemented
	ShaderRenderModeTexture
)

type _Shaders struct {
	RoundedRect struct {
		Shader rl.Shader // don't change it!

		// vertex
		locURectTopLeft int32

		// fragment
		locURectSize           int32
		locUCornerRadius       int32
		locUBorderWidth        int32
		locUBorderColor        int32
		locUGradientStartColor int32
		locUGradientEndColor   int32
		locUGradientAngle      int32
		locURenderMode         int32
	}
}

var shaders _Shaders

func (s *_Shaders) unload() {
	shadersValue := reflect.ValueOf(s).Elem()

	for i := range shadersValue.NumField() {
		shader := shadersValue.Field(i).FieldByName("Shader").Interface().(rl.Shader)
		rl.UnloadShader(shader)
	}
}

type _Textures struct {
	Quad1x1 rl.Texture2D

	DropdownExpandIcon   rl.Texture2D
	DropdownCollapseIcon rl.Texture2D
}

var textures _Textures

func (t *_Textures) unload() {
	texturesValue := reflect.ValueOf(t).Elem()

	for i := range texturesValue.NumField() {
		texture := texturesValue.Field(i).Interface().(rl.Texture2D)
		rl.UnloadTexture(texture)
	}
}

// LoadAssets loads all shaders, built-in textures, and the fallback font
// required by the library. It must be called after [rl.InitWindow] (or
// [InitWindow], which calls it automatically) and before any rendering.
func LoadAssets() {
	assert.True(rl.IsWindowReady(), "must call rl.InitWindow first")

	_fallbackFont = rl.GetFontDefault()

	shaders.RoundedRect.Shader = loadShader("rounded-rect.vs.glsl", "rounded-rect.fs.glsl")
	shaders.RoundedRect.locURectTopLeft = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uRectTopLeft")
	shaders.RoundedRect.locURectSize = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uRectSize")
	shaders.RoundedRect.locUCornerRadius = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uCornerRadius")
	shaders.RoundedRect.locUBorderWidth = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uBorderWidth")
	shaders.RoundedRect.locUBorderColor = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uBorderColor")
	shaders.RoundedRect.locUGradientStartColor = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uGradientStartColor")
	shaders.RoundedRect.locUGradientEndColor = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uGradientEndColor")
	shaders.RoundedRect.locUGradientAngle = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uGradientAngle")
	shaders.RoundedRect.locURenderMode = rl.GetLocationUniform(shaders.RoundedRect.Shader.ID, "uRenderMode")

	{
		// white 1x1 image
		img := rl.NewImage([]byte{255, 255, 255}, 1, 1, 1, rl.UncompressedR8g8b8)
		textures.Quad1x1 = rl.LoadTextureFromImage(img)
	}

	textures.DropdownExpandIcon = Must(LoadImageTexture("assets/icons/filled-arrow-down.png", iconsFS))
	textures.DropdownCollapseIcon = Must(LoadImageTexture("assets/icons/filled-arrow-up.png", iconsFS))
	DefaultDropdownExpandIcon = textures.DropdownExpandIcon
	DefaultDropdownCollapseIcon = textures.DropdownCollapseIcon
}

// UnloadAssets frees all GPU resources (shaders, textures, fonts) that were
// loaded by [LoadAssets]. It is called automatically by [CloseWindow].
func UnloadAssets() {
	unloadCachedImages()
	unloadCachedTextures()

	rl.UnloadFont(_fallbackFont)

	shaders.unload()
	textures.unload()
}

// DrawRectangle draws a rectangle using the library's rounded-rect shader.
// It supports per-side border widths, per-corner radius, a border color, and a
// fill color. When no border (none or transparent) and no corner radius are
// needed the call falls back to the (I suppose) cheaper [rl.DrawRectangleRec].
func DrawRectangle(
	rect rl.Rectangle,
	borderWidth BoxSides, cornerRadius BoxCorners, borderColor rl.Color,
	color rl.Color,
) {
	if (borderWidth == (BoxSides{}) || borderColor.A == 0) && cornerRadius == (BoxCorners{}) {
		rl.DrawRectangleRec(rect, color)
		return
	}

	rl.BeginShaderMode(shaders.RoundedRect.Shader)
	rl.EnableShader(shaders.RoundedRect.Shader.ID)
	defer rl.EndShaderMode()

	cornerRadius = cornerRadius.limit(rect)

	setUniform(shaders.RoundedRect.locURectTopLeft, []float32{rect.X, rect.Y}, int32(rl.ShaderUniformVec2), 1)
	setUniform(shaders.RoundedRect.locURectSize, []float32{rect.Width, rect.Height}, int32(rl.ShaderUniformVec2), 1)

	setUniform(shaders.RoundedRect.locUCornerRadius, cornerRadius.slice(), int32(rl.ShaderUniformFloat), 4)
	setUniform(shaders.RoundedRect.locUBorderWidth, borderWidth.slice(), int32(rl.ShaderUniformFloat), 4)
	setUniform(shaders.RoundedRect.locUBorderColor, ColorToUniformVec4(borderColor), int32(rl.ShaderUniformVec4), 1)

	setUniform(shaders.RoundedRect.locURenderMode, []int32{ShaderRenderModeSolidColor}, int32(rl.ShaderUniformInt), 1)

	rl.DrawTexturePro(textures.Quad1x1, Rect(0, 0, 1, 1), rect, Vec2(0, 0), 0, color)
}

// DrawRectangleWithGradient is like [DrawRectangle] but fills the rectangle
// with a [Gradient] instead of a solid color.
func DrawRectangleWithGradient(
	rect rl.Rectangle,
	borderWidth BoxSides, cornerRadius BoxCorners, borderColor rl.Color,
	gradient Gradient,
) {
	rl.BeginShaderMode(shaders.RoundedRect.Shader)
	rl.EnableShader(shaders.RoundedRect.Shader.ID)
	defer rl.EndShaderMode()

	cornerRadius = cornerRadius.limit(rect)

	setUniform(shaders.RoundedRect.locURectTopLeft, []float32{rect.X, rect.Y}, int32(rl.ShaderUniformVec2), 1)
	setUniform(shaders.RoundedRect.locURectSize, []float32{rect.Width, rect.Height}, int32(rl.ShaderUniformVec2), 1)

	setUniform(shaders.RoundedRect.locUCornerRadius, cornerRadius.slice(), int32(rl.ShaderUniformFloat), 4)
	setUniform(shaders.RoundedRect.locUBorderWidth, borderWidth.slice(), int32(rl.ShaderUniformFloat), 4)
	setUniform(shaders.RoundedRect.locUBorderColor, ColorToUniformVec4(borderColor), int32(rl.ShaderUniformVec4), 1)

	setUniform(shaders.RoundedRect.locUGradientStartColor, ColorToUniformVec4(gradient.StartColor), int32(rl.ShaderUniformVec4), 1)
	setUniform(shaders.RoundedRect.locUGradientEndColor, ColorToUniformVec4(gradient.EndColor), int32(rl.ShaderUniformVec4), 1)

	switch gradient.Kind() {
	case GradientKindLinear:
		setUniform(shaders.RoundedRect.locUGradientAngle, []float32{gradient.Angle}, int32(rl.ShaderUniformFloat), 1)
		setUniform(shaders.RoundedRect.locURenderMode, []int32{ShaderRenderModeGradientLinear}, int32(rl.ShaderUniformInt), 1)

	case GradientKindRadial:
		assert.Unimplemented()
		setUniform(shaders.RoundedRect.locURenderMode, []int32{ShaderRenderModeGradientRadial}, int32(rl.ShaderUniformInt), 1)

	default:
		assert.Unimplemented()
	}

	rl.DrawTexturePro(textures.Quad1x1, Rect(0, 0, 1, 1), rect, Vec2(0, 0), 0, rl.White)
}

// DrawRectangleWithTexture is like [DrawRectangle] but fills the rectangle
// with texture according to the specified textureFit mode.
func DrawRectangleWithTexture(
	rect rl.Rectangle,
	borderWidth BoxSides, cornerRadius BoxCorners, borderColor rl.Color,
	texture rl.Texture2D, textureTint rl.Color, textureFit TextureFit,
) {
	rl.BeginShaderMode(shaders.RoundedRect.Shader)
	rl.EnableShader(shaders.RoundedRect.Shader.ID)
	defer rl.EndShaderMode()

	cornerRadius = cornerRadius.limit(rect)

	setUniform(shaders.RoundedRect.locUCornerRadius, cornerRadius.slice(), int32(rl.ShaderUniformFloat), 4)
	setUniform(shaders.RoundedRect.locUBorderWidth, borderWidth.slice(), int32(rl.ShaderUniformFloat), 4)
	setUniform(shaders.RoundedRect.locUBorderColor, ColorToUniformVec4(borderColor), int32(rl.ShaderUniformVec4), 1)

	setUniform(shaders.RoundedRect.locURenderMode, []int32{ShaderRenderModeTexture}, int32(rl.ShaderUniformInt), 1)

	textureSize := Vec2(float32(texture.Width), float32(texture.Height))

	textureAspectRatio := textureSize.X / textureSize.Y
	rectAspectRatio := rect.Width / rect.Height

	switch textureFit {
	case TextureFitContain:
		var size, offset rl.Vector2

		if textureAspectRatio <= rectAspectRatio {
			size = Vec2(rect.Height*textureAspectRatio, rect.Height)
			offset = Vec2((rect.Width-size.X)/2, 0)
		} else {
			size = Vec2(rect.Width, rect.Width/textureAspectRatio)
			offset = Vec2(0, (rect.Height-size.Y)/2)
		}

		texturePos := Vec2(rect.X+offset.X, rect.Y+offset.Y)

		src := Rect(0, 0, textureSize.X, textureSize.Y)
		dst := Rect(texturePos.X, texturePos.Y, size.X, size.Y)

		setUniform(shaders.RoundedRect.locURectTopLeft, []float32{dst.X, dst.Y}, int32(rl.ShaderUniformVec2), 1)
		setUniform(shaders.RoundedRect.locURectSize, []float32{dst.Width, dst.Height}, int32(rl.ShaderUniformVec2), 1)

		rl.DrawTexturePro(texture, src, dst, Vec2(0, 0), 0, textureTint)

	case TextureFitCover:
		var src rl.Rectangle

		if textureAspectRatio <= rectAspectRatio {
			newHeight := textureSize.X / rectAspectRatio
			src = Rect(0, (textureSize.Y-newHeight)/2, textureSize.X, newHeight)
		} else {
			newWidth := textureSize.Y * rectAspectRatio
			src = Rect((textureSize.X-newWidth)/2, 0, newWidth, textureSize.Y)
		}

		setUniform(shaders.RoundedRect.locURectTopLeft, []float32{rect.X, rect.Y}, int32(rl.ShaderUniformVec2), 1)
		setUniform(shaders.RoundedRect.locURectSize, []float32{rect.Width, rect.Height}, int32(rl.ShaderUniformVec2), 1)

		rl.DrawTexturePro(texture, src, rect, Vec2(0, 0), 0, textureTint)

	default:
		assert.Unreachable()
	}
}

// DrawTextEllipsis draws text inside bounds. If the text is too wide it is
// truncated and a "..." suffix is appended. The text is aligned horizontally
// with alignX and vertically with alignY.
func DrawTextEllipsis(
	bounds rl.Rectangle,
	alignX, alignY Alignment,
	font rl.Font,
	text string,
	fontSize float32, charSpacing float32, color rl.Color,
) {
	textSize := rl.MeasureTextEx(font, text, fontSize, charSpacing)

	switch alignY {
	case AlignStart:
	case AlignCenter:
		bounds.Y += (bounds.Height - textSize.Y) / 2
	case AlignEnd:
		bounds.Y += bounds.Height - textSize.Y
	}

	if textSize.X <= bounds.Width {
		switch alignX {
		case AlignStart:
		case AlignCenter:
			bounds.X += (bounds.Width - textSize.X) / 2
		case AlignEnd:
			bounds.X += bounds.Width - textSize.X
		}

		rl.DrawTextEx(font, text, Vec2(bounds.X, bounds.Y), fontSize, charSpacing, color)
		return
	}

	dotWidth := rl.MeasureTextEx(font, ".", fontSize, 0).X
	ellipsisWidth := dotWidth + charSpacing + dotWidth + charSpacing + dotWidth

	textWidth := -charSpacing
	for _, char := range text {
		charWidth := rl.MeasureTextEx(font, string(char), fontSize, 0).X

		if textWidth+charSpacing+charWidth+charSpacing+ellipsisWidth > bounds.Width {
			// for i := range 3 {
			// 	dotPosX := bounds.X + textWidth + charSpacing*float32(i+1) + dotWidth*float32(i)
			// 	rl.DrawTextCodepoint(font, '.', Vec2(dotPosX, bounds.Y), fontSize, color)
			// }
			rl.DrawTextEx(font, "...", Vec2(bounds.X+textWidth+charSpacing, bounds.Y), fontSize, charSpacing, color)
			textWidth += charSpacing + ellipsisWidth
			break
		}

		textWidth += charSpacing
		rl.DrawTextCodepoint(font, char, Vec2(bounds.X+textWidth, bounds.Y), fontSize, color)
		textWidth += charWidth
	}
}

// WrapText re-flows text so that no line exceeds the width of bounds.  Words
// are separated at space and tab characters; newlines in the source are
// preserved. The returned string uses "\n" as the line separator.
func WrapText(bounds rl.Rectangle, font rl.Font, text string, fontSize float32, charSpacing float32) string {
	wrappedText := strings.Builder{}
	wrappedText.Grow(len(text))

	var currLineWidth float32

	maxWidth := bounds.Width

	spaceWidth := rl.MeasureTextEx(font, " ", fontSize, 0).X
	tabWidth := rl.MeasureTextEx(font, "\t", fontSize, 0).X

	var prevWhitespace byte = ' '

	var wordStart int

	for i := 0; i <= len(text); i++ {
		var char rune

		if i < len(text) {
			var _size int
			char, _size = utf8.DecodeRuneInString(text[i:])
			assert.NotEqual(char, utf8.RuneError, fmt.Sprintf("RuneError = %d", _size))

			if char != ' ' && char != '\t' && char != '\n' {
				continue
			}
		}

		word := string(text[wordStart:i])
		wordWidth := rl.MeasureTextEx(font, word, fontSize, charSpacing).X

		if currLineWidth == 0 {
			wrappedText.WriteString(word)
			if char == '\n' {
				wrappedText.WriteByte('\n')
				currLineWidth = 0
			} else {
				currLineWidth += wordWidth
				prevWhitespace = byte(char)
			}
			if char == '\t' {
				wordStart = i
			} else {
				wordStart = i + 1
			}
			continue
		}

		var prevWhitespaceWidth float32
		if prevWhitespace == ' ' {
			prevWhitespaceWidth = charSpacing + spaceWidth + charSpacing
		} else {
			prevWhitespaceWidth = charSpacing + tabWidth + charSpacing
		}

		if currLineWidth+prevWhitespaceWidth+wordWidth <= maxWidth {
			wrappedText.WriteByte(byte(prevWhitespace))
			wrappedText.WriteString(word)
			currLineWidth += prevWhitespaceWidth + wordWidth
		} else {
			wrappedText.WriteByte('\n')
			wrappedText.WriteString(word)
			currLineWidth = wordWidth
		}

		if char == '\n' {
			wrappedText.WriteByte('\n')
			currLineWidth = 0
		}

		if char != '\n' {
			prevWhitespace = byte(char)
		}

		if char == '\t' {
			wordStart = i
		} else {
			wordStart = i + 1
		}
	}

	return wrappedText.String()
}

var scissorModeStack []rl.Rectangle

// BeginScissorMode begins a scissor-clipping region that restricts rendering
// to intersection. Calls can be nested; each call intersects the new region
// with the current top-of-stack region. Must be balanced with [EndScissorMode].
func BeginScissorMode(intersection rl.Rectangle) {
	intersection.Width = max(0, intersection.Width)
	intersection.Height = max(0, intersection.Height)

	if len(scissorModeStack) == 0 {
		scissorModeStack = append(scissorModeStack, intersection)
		rl.BeginScissorMode(int32(intersection.X), int32(intersection.Y), int32(intersection.Width), int32(intersection.Height))
		return
	}

	lastRect := scissorModeStack[len(scissorModeStack)-1]
	intersection = rl.GetCollisionRec(intersection, lastRect)
	scissorModeStack = append(scissorModeStack, intersection)

	rl.EndScissorMode()
	rl.BeginScissorMode(int32(intersection.X), int32(intersection.Y), int32(intersection.Width), int32(intersection.Height))
}

// EndScissorMode pops the current scissor region and restores the previous
// one. Panics if there is no matching [BeginScissorMode] call on the stack.
func EndScissorMode() {
	assert.Greater(len(scissorModeStack), 0)

	rl.EndScissorMode()
	scissorModeStack = scissorModeStack[:len(scissorModeStack)-1]

	if len(scissorModeStack) > 0 {
		rect := scissorModeStack[len(scissorModeStack)-1]
		rl.BeginScissorMode(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height))
	}
}

func loadImage(filePath string, fileSystem fs.FS) (*rl.Image, error) {
	extension := filepath.Ext(filePath)
	if extension == "" {
		return nil, fmt.Errorf("the file must have a valid image extension: supported formats: '.png', '.jpg': %q", filePath)
	}

	var err error
	var file fs.File
	if fileSystem != nil {
		assert.False(path.IsAbs(filePath), "cannot use absolute paths with fs.FS", filePath)
		file, err = fileSystem.Open(filePath)
	} else {
		file, err = os.Open(filePath)
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w (%q)", err, filePath)
	}

	image := rl.LoadImageFromMemory(extension, fileData, int32(len(fileData)))
	if !rl.IsImageValid(image) {
		return nil, fmt.Errorf("invalid image: %q", filePath)
	}
	return image, nil
}

type _FilePathCacheKey struct {
	fileSystem fs.FS
	filepath   string
}

var _imageCache = map[_FilePathCacheKey]*rl.Image{}
var _textureCache = map[_FilePathCacheKey]rl.Texture2D{}

// LoadImage loads an image from filePath.
// The result is cached; subsequent calls with the same (filePath, fileSystem)
// pair return the cached image.
//
// If fileSystem is nil the file is read from the OS filesystem.
// If fileSystem is non-nil, filePath must be relative.
func LoadImage(filePath string, fileSystem fs.FS) (*rl.Image, error) {
	filePathCacheKey := _FilePathCacheKey{fileSystem, filePath}

	if image, ok := _imageCache[filePathCacheKey]; ok {
		return image, nil
	}

	image, err := loadImage(filePath, fileSystem)
	if err != nil {
		return nil, fmt.Errorf("loading image: %w", err)
	}

	_imageCache[filePathCacheKey] = image
	return image, nil
}

// UnloadImage removes an image from the cache.
// filePath and fileSystem must match the values passed to [LoadImage].
func UnloadImage(filePath string, fileSystem fs.FS) {
	filePathCacheKey := _FilePathCacheKey{fileSystem, filePath}

	image, ok := _imageCache[filePathCacheKey]
	assert.True(ok)
	delete(_imageCache, filePathCacheKey)

	rl.UnloadImage(image)
}

func unloadCachedImages() {
	for _, image := range _imageCache {
		rl.UnloadImage(image)
	}
}

// LoadImageTexture loads an image from filePath and returns a GPU texture.
// The result is cached; subsequent calls with the same (filePath, fileSystem)
// pair return the cached texture.
//
// If fileSystem is nil the file is read from the OS filesystem.
// If fileSystem is non-nil, filePath must be relative.
//
// The loaded texture has [ImageTextureFilter] applied.
func LoadImageTexture(filePath string, fileSystem fs.FS) (rl.Texture2D, error) {
	filePathCacheKey := _FilePathCacheKey{fileSystem, filePath}

	if texture, ok := _textureCache[filePathCacheKey]; ok {
		return texture, nil
	}

	image, err := loadImage(filePath, fileSystem)
	if err != nil {
		return rl.Texture2D{}, fmt.Errorf("loading image: %w", err)
	}
	defer rl.UnloadImage(image)

	texture := rl.LoadTextureFromImage(image)
	if !rl.IsTextureValid(texture) {
		return rl.Texture2D{}, fmt.Errorf("invalid texture: %q", filePath)
	}
	rl.SetTextureFilter(texture, ImageTextureFilter)

	_textureCache[filePathCacheKey] = texture
	return texture, nil
}

// UnloadImageTexture removes an image from the cache and unloads its GPU
// texture. filePath and fileSystem must match the values passed to
// [LoadImageTexture].
func UnloadImageTexture(filePath string, fileSystem fs.FS) {
	filePathCacheKey := _FilePathCacheKey{fileSystem, filePath}

	texture, ok := _textureCache[filePathCacheKey]
	if !ok {
		return
	}
	delete(_textureCache, filePathCacheKey)

	rl.UnloadTexture(texture)
}

func unloadCachedTextures() {
	for _, texture := range _textureCache {
		rl.UnloadTexture(texture)
	}
}

func isPointInNodeVisibleArea(point rl.Vector2, node Node) bool {
	box := node.box()

	if box.ignored {
		return false
	}

	visibleNodeArea := node.TotalArea()
	curr := box.parent
	for curr != nil {
		if scrollBox := curr.scrollBox(); scrollBox != nil {
			visibleNodeArea = rl.GetCollisionRec(visibleNodeArea, scrollBox.Rect())

			if !rl.CheckCollisionPointRec(point, visibleNodeArea) {
				return false
			}
		}
		curr = curr.box().parent
	}
	return rl.CheckCollisionPointRec(point, visibleNodeArea)
}

// IsNodeVisible reports whether node will produce visible pixels when rendered.
// A node is not visible if it is marked Invisible, if its Ignored flag is set,
// or if it is entirely clipped by an ancestor [*ScrollBox].
func IsNodeVisible(node Node) bool {
	box := node.box()

	if box.Invisible || box.ignored {
		return false
	}

	visibleNodeArea := node.TotalArea()
	curr := box.parent
	for curr != nil {
		if scrollBox := curr.scrollBox(); scrollBox != nil {
			visibleNodeArea = rl.GetCollisionRec(visibleNodeArea, scrollBox.Rect())
			if visibleNodeArea.Width == 0 || visibleNodeArea.Height == 0 {
				return false
			}
		}
		curr = curr.box().parent
	}
	return true
}

// IsNodeHovered reports whether the mouse cursor is currently positioned over
// node and no higher-priority node (see [AddNodeToHighPriorityList]) or a
// higher-z-index [*SubWindow] is blocking it.
func IsNodeHovered(node Node) bool {
	assert.GreaterEqual(len(subWindows), 1)

	box := node.box()

	if box.ignored {
		return false
	}

	mousePos := rl.GetMousePosition()

	if !rl.CheckCollisionPointRec(mousePos, node.TotalArea()) {
		return false
	}

	if highPriorityNode := GetHighestPriorityNodeUnderMouse(); highPriorityNode != nil {
		return highPriorityNode == node
	}

	for i := len(subWindows) - 1; i >= 0; i-- {
		if subWindows[i].IsHidden() {
			continue
		}
		if subWindows[i] == box.subWindow {
			return isPointInNodeVisibleArea(mousePos, node)
		}
		if rl.CheckCollisionPointRec(mousePos, subWindows[i].Rect()) {
			return false
		}
	}

	assert.Unreachable()
	return false
}

// ResetIDs clears all auto-generated IDs from all sub-windows and removes the
// corresponding nodes from the cache.
func ResetIDs() {
	for i := range subWindows {
		subWindows[i].ResetIDs()
	}
	for _, id := range uniqueIDs {
		RemoveNodeFromCache(id)
	}
	uniqueIDs = uniqueIDs[:0]
}

// RestartIDs rewinds the auto-ID cursors on all sub-windows without evicting
// anything from the node cache. This is called automatically by [ResetLayout]
// each frame so that auto-IDs are handed out in the same sequence on every
// rebuild.
func RestartIDs() {
	for i := range subWindows {
		subWindows[i].RestartIDs()
	}
	uniqueIDs = uniqueIDs[:0]
}

// ResetLayout clears the child list of every sub-window's root node and
// restarts the auto-ID counters. Call this at the beginning of each frame
// before rebuilding the UI tree.
func ResetLayout() {
	for _, subWindow := range subWindows {
		subWindow.ResetLayout()
	}
	RestartIDs()
}

// ComputeLayout computes the layout on all sub-windows. Call this after
// building the node tree and before [Update] or [Render].
func ComputeLayout() {
	for _, subWindow := range subWindows {
		subWindow.ComputeLayout()
	}
}

func boolToUint8(v bool) uint8 {
	// fast
	var i uint8
	if v {
		i = 1
	}
	return i
}

var pressedButtons uint8
var releasedButtons uint8

func gatherPressedMouseButtons() uint8 {
	var buttons uint8
	buttons |= boolToUint8(rl.IsMouseButtonPressed(rl.MouseButtonLeft)) << rl.MouseButtonLeft
	buttons |= boolToUint8(rl.IsMouseButtonPressed(rl.MouseButtonRight)) << rl.MouseButtonRight
	buttons |= boolToUint8(rl.IsMouseButtonPressed(rl.MouseButtonMiddle)) << rl.MouseButtonMiddle
	buttons |= boolToUint8(rl.IsMouseButtonPressed(rl.MouseButtonSide)) << rl.MouseButtonSide
	buttons |= boolToUint8(rl.IsMouseButtonPressed(rl.MouseButtonExtra)) << rl.MouseButtonExtra
	buttons |= boolToUint8(rl.IsMouseButtonPressed(rl.MouseButtonForward)) << rl.MouseButtonForward
	buttons |= boolToUint8(rl.IsMouseButtonPressed(rl.MouseButtonBack)) << rl.MouseButtonBack
	return buttons
}

func gatherReleasedMouseButtons() uint8 {
	var buttons uint8
	buttons |= boolToUint8(rl.IsMouseButtonReleased(rl.MouseButtonLeft)) << rl.MouseButtonLeft
	buttons |= boolToUint8(rl.IsMouseButtonReleased(rl.MouseButtonRight)) << rl.MouseButtonRight
	buttons |= boolToUint8(rl.IsMouseButtonReleased(rl.MouseButtonMiddle)) << rl.MouseButtonMiddle
	buttons |= boolToUint8(rl.IsMouseButtonReleased(rl.MouseButtonSide)) << rl.MouseButtonSide
	buttons |= boolToUint8(rl.IsMouseButtonReleased(rl.MouseButtonExtra)) << rl.MouseButtonExtra
	buttons |= boolToUint8(rl.IsMouseButtonReleased(rl.MouseButtonForward)) << rl.MouseButtonForward
	buttons |= boolToUint8(rl.IsMouseButtonReleased(rl.MouseButtonBack)) << rl.MouseButtonBack
	return buttons
}

// IsMouseButtonPressed reports whether button was pressed this frame.
// Unlike [rl.IsMouseButtonPressed], this reads from a state snapshot taken at
// the start of [Update], allowing the event to be consumed via
// [ConsumeMouseButtonPressed].
func IsMouseButtonPressed(button rl.MouseButton) bool {
	return pressedButtons>>button&1 == 1
}

// IsMouseButtonReleased reports whether button was released this frame.
// Unlike [rl.IsMouseButtonReleased], this reads from a state snapshot taken at
// the start of [Update], allowing the event to be consumed via
// [ConsumeMouseButtonReleased].
func IsMouseButtonReleased(button rl.MouseButton) bool {
	return releasedButtons>>button&1 == 1
}

// ConsumeMouseButtonPressed clears the pressed-this-frame flag for button so
// that subsequent calls to [IsMouseButtonPressed] for the same button return
// false. Use this in interactive nodes to prevent multiple handlers from
// reacting to the same press event.
func ConsumeMouseButtonPressed(button rl.MouseButton) {
	pressedButtons &^= 1 << button
}

// ConsumeMouseButtonReleased clears the pressed-this-frame flag for button so
// that subsequent calls to [IsMouseButtonReleased] for the same button return
// false. Use this in interactive nodes to prevent multiple handlers from
// reacting to the same press event.
func ConsumeMouseButtonReleased(button rl.MouseButton) {
	releasedButtons &^= 1 << button
}

// IsMouseButtonPressedConsume is like [IsMouseButtonPressed], but consumes
// the event if button was pressed.
func IsMouseButtonPressedConsume(button rl.MouseButton) bool {
	if IsMouseButtonPressed(button) {
		ConsumeMouseButtonPressed(button)
		return true
	}
	return false
}

// IsMouseButtonReleasedConsume is like [IsMouseButtonReleased], but consumes
// the event if button was released.
func IsMouseButtonReleasedConsume(button rl.MouseButton) bool {
	if IsMouseButtonReleased(button) {
		ConsumeMouseButtonReleased(button)
		return true
	}
	return false
}

var postUpdateCalls []func()

// AddPostUpdate schedules fn to be called at the end of the current [Update]
// call, after all nodes have processed input.
// Useful to keep locality of behavior.
func AddPostUpdate(fn func()) {
	postUpdateCalls = append(postUpdateCalls, fn)
}

var highPriorityNodes []Node

// AddNodeToHighPriorityList inserts node into the high-priority hover list.
// Nodes on this list take hover precedence over any other node under the mouse
// (e.g. an open [*Dropdown]).
// The list is ordered; use [MoveNodeToTopOfHighPriorityList] to adjust
// priority after insertion. Has no effect if node is already in the list.
func AddNodeToHighPriorityList(node Node) {
	if slices.Index(highPriorityNodes, node) != -1 {
		return
	}
	highPriorityNodes = append(highPriorityNodes, node)
}

// RemoveNodeFromHighPriorityList removes node from the high-priority list.
// Has no effect if node is not in the list.
func RemoveNodeFromHighPriorityList(node Node) {
	idx := slices.Index(highPriorityNodes, node)
	if idx == -1 {
		return
	}
	highPriorityNodes = slices.Delete(highPriorityNodes, idx, idx+1)
}

// MoveNodeToTopOfHighPriorityList moves an already-registered node to the
// highest-priority position. Panics if node is not currently in the list.
func MoveNodeToTopOfHighPriorityList(node Node) {
	idx := slices.Index(highPriorityNodes, node)
	assert.NotEqual(idx, -1)
	highPriorityNodes = slices.Delete(highPriorityNodes, idx, idx+1)
	highPriorityNodes = append(highPriorityNodes, node)
}

// IsNodeInHighPriorityList reports whether node is currently registered in the
// high-priority hover list.
func IsNodeInHighPriorityList(node Node) bool {
	return slices.Index(highPriorityNodes, node) != -1
}

// GetHighestPriorityNodeUnderMouse returns the highest-priority node in the
// high-priority list whose total area contains the mouse cursor, or nil if no
// such node exists.
func GetHighestPriorityNodeUnderMouse() Node {
	if len(highPriorityNodes) == 0 {
		return nil
	}

	mousePos := rl.GetMousePosition()

	for i := len(highPriorityNodes) - 1; i >= 0; i-- {
		if rl.CheckCollisionPointRec(mousePos, highPriorityNodes[i].TotalArea()) {
			return highPriorityNodes[i]
		}
	}
	return nil
}

var screenSize rl.Vector2

// GetScreenSize returns the current window dimensions as a Vector2.
// The value is updated once per frame by [UpdateScreenSize] (called inside
// [Update]).
func GetScreenSize() rl.Vector2 {
	return screenSize
}

// UpdateScreenSize refreshes the cached screen size when the window has been
// resized. This is called automatically by [Update]; you only need to call it
// manually if you resize the window outside the normal update loop.
func UpdateScreenSize() {
	if rl.IsWindowResized() {
		screenSize = Vec2(float32(rl.GetScreenWidth()), float32(rl.GetScreenHeight()))
	}
}

// InitWindow creates the raylib window with the given size and title, and
// calls [LoadAssets]. Use this instead of [rl.InitWindow] so that all GUI
// resources are ready before the first frame.
func InitWindow(size rl.Vector2, title string) {
	rl.InitWindow(int32(size.X), int32(size.Y), title)
	screenSize = size
	LoadAssets()
}

// CloseWindow calls [UnloadAssets] and then closes the raylib window.
// Defer this after [InitWindow].
func CloseWindow() {
	UnloadAssets()
	rl.CloseWindow()
}

var currentMouseCursor = rl.MouseCursorDefault
var frameMouseCursor = rl.MouseCursorDefault

// SetMouseCursor requests that the OS cursor be changed to cursor for the
// current frame. The last call wins; the cursor is applied at the end of
// [Update].
func SetMouseCursor(cursor rl.MouseCursor) {
	frameMouseCursor = cursor
}

// Update runs the per-frame input processing pass.
//
// It snapshots pressed/released mouse buttons, updates the screen size,
// bubbles the clicked sub-window to the top of the z-order, runs the top
// sub-window's own Update (for dragging floating windows), calls Update on
// every node in every sub-window, applies the cursor, and finally executes
// all functions queued with [AddPostUpdate].
//
// Must be called after [ComputeLayout] and before [Render].
func Update() {
	if len(subWindows) == 0 {
		if currentMouseCursor != rl.MouseCursorDefault {
			currentMouseCursor = rl.MouseCursorDefault
			rl.SetMouseCursor(rl.MouseCursorDefault)
		}
		return
	}

	frameMouseCursor = rl.MouseCursorDefault

	pressedButtons = gatherPressedMouseButtons()
	releasedButtons = gatherReleasedMouseButtons()

	UpdateScreenSize()

	topSubWindow := subWindows[len(subWindows)-1]

	if IsMouseButtonPressed(rl.MouseButtonLeft) {
		mousePos := rl.GetMousePosition()

		for i := len(subWindows) - 1; i >= 0; i-- {
			if !subWindows[i].IsHidden() && rl.CheckCollisionPointRec(mousePos, subWindows[i].Rect()) {
				if topSubWindow != subWindows[i] && topSubWindow.zIndex == ZIndexEphemeral {
					topSubWindow.Hide()
				}
				bubbleSubWindowUp(subWindows[i])
				topSubWindow = subWindows[len(subWindows)-1]
				break
			}
		}
	}

	topSubWindow.Update()

	for _, subWindow := range subWindows {
		updateNodes(subWindow.root)
	}

	if frameMouseCursor != currentMouseCursor {
		currentMouseCursor = frameMouseCursor
		rl.SetMouseCursor(frameMouseCursor)
	}

	for _, fn := range postUpdateCalls {
		fn()
	}
	postUpdateCalls = postUpdateCalls[:0]
}

func updateNodes(node Node) {
	if node == nil {
		return
	}

	// children have priority over parent
	for child := range node.iterChildrenNode(false) {
		updateNodes(child)
	}

	node.Update()
}

var postRenderCalls []func()

// AddPostRender schedules fn to be called at the end of the current [Render]
// call, after all sub-windows have been rendered.
func AddPostRender(fn func()) {
	postRenderCalls = append(postRenderCalls, fn)
}

// Render draws all sub-windows and their node trees in z-index order (lowest
// first). Must be called between [rl.BeginDrawing] and [rl.EndDrawing].
// Panics if [LoadAssets] has not been called.
func Render() {
	assert.True(rl.IsShaderValid(shaders.RoundedRect.Shader), "assets are not loaded, run gui.LoadAssets() after rl.InitWindow()")

	for _, subWindow := range subWindows {
		subWindow.Render()
	}

	for _, fn := range postRenderCalls {
		fn()
	}
	postRenderCalls = postRenderCalls[:0]
}

var _cachedNodes = make(map[NodeID]Node)

// CacheNode stores node in the per-frame-persistent cache under its ID.
// Stateful widgets (ScrollBox, Slider, Toggle, TextInput, Dropdown) call this
// in their constructors so that their state survives the tree rebuild each
// frame.
// Panics if node ID is not set.
// If node ID is [NodeIDManual], this is NO-OP.
func CacheNode(node Node) {
	assert.NotEqual(node, nil)

	id := node.box().id
	assert.NotEqual(id, NodeIDUnset)
	if id == NodeIDManual {
		return
	}
	_cachedNodes[id] = node
}

// RemoveNodeFromCache removes the node associated with ID from the cache. Use
// this when you need to force a stateful widget to reset to its initial state
// (e.g. after the data it displays has changed programmatically).
// Panics if node ID is not set.
// If node ID is [NodeIDManual], this is NO-OP.
func RemoveNodeFromCache(id NodeID) {
	assert.NotEqual(id, NodeIDUnset)
	if id == NodeIDManual {
		return
	}
	delete(_cachedNodes, id)
}

// GetNodeFromCache retrieves a previously cached node of type N by its ID.
// If found, the node's layout is reset (parent and position cleared, children
// list emptied) so it is ready to be re-inserted into the tree.
// Returns the zero value of N if no node with that ID is cached or the ID is
// [NodeIDManual].
// Panics if node ID is not set.
func GetNodeFromCache[N Node](id NodeID) N {
	assert.NotEqual(id, NodeIDUnset)
	var zero N
	if id == NodeIDManual {
		return zero
	}
	if node, ok := _cachedNodes[id]; ok {
		node.box().resetLayout()
		return node.(N)
	}
	return zero
}

// NodeID is an opaque 64-bit identifier for a node. It is used as a cache key
// for stateful widgets. Generate stable IDs with [NewID] or obtain
// auto-incremented IDs from [SubWindow.GetAutoID].
type NodeID uint64

const (
	// NodeIDUnset is the zero value for [NodeID]. A node with this ID has no
	// assigned identity and will be skipped by the cache.
	NodeIDUnset NodeID = iota

	// NodeIDManual is a reserved sentinel for nodes whose lifecycle is
	// managed entirely by the caller (e.g. the node will not be cached).
	NodeIDManual
)

// NodeIDGenerator issues NodeID values from a fixed sequence that is
// reproduced identically on every frame rebuild by restarting the cursor.
// Each [*SubWindow] owns one and issues IDs through [SubWindow.GetAutoID].
// The zero value for [NodeIDGenerator] is an empty generator ready to use.
type NodeIDGenerator struct {
	ids    []NodeID
	cursor int
}

// GetID returns the next ID in the sequence, if any, otherwise a new random ID
// is generated and appended.
func (g *NodeIDGenerator) GetID() NodeID {
	if g.cursor >= len(g.ids) {
		id := NodeID(rand.Uint64())
		g.ids = append(g.ids, id)
	}
	id := g.ids[g.cursor]
	g.cursor++
	return id
}

// Restart rewinds the cursor to the beginning of the sequence without
// removing any IDs or touching the cache.
func (g *NodeIDGenerator) Restart() {
	g.cursor = 0
}

// Reset rewinds the cursor and evicts all IDs in the sequence from the node
// cache, effectively discarding all stateful widget state issued by this
// generator.
func (g *NodeIDGenerator) Reset() {
	g.cursor = 0
	for _, id := range g.ids {
		RemoveNodeFromCache(id)
	}
	g.ids = g.ids[:0]
}

// ZIndex is the type used for sub-window layering. Higher values render on
// top.
type ZIndex = int16

const (
	// ZIndexEphemeral is the z-index assigned to ephemeral sub-windows.
	// It marks the window as a one-shot overlay (e.g. a context menu)
	// that is automatically hidden when the user clicks outside of it.
	// It is the highest possible value so they always render on top.
	ZIndexEphemeral ZIndex = math.MaxInt16

	// ZIndexPopup is a z-index suitable for modal or pop-up sub-windows that
	// should appear above normal content but below ephemeral overlays.
	ZIndexPopup ZIndex = 10_000

	// ZIndexBase is the default z-index for ordinary sub-windows.
	ZIndexBase ZIndex = 0

	// ZIndexRoot is the lowest possible z-index. Assign it to the primary
	// root sub-window so that all other windows render on top of it.
	ZIndexRoot ZIndex = math.MinInt16
)

var subWindows []*SubWindow
var uniqueIDs []NodeID

var _crc64Table = crc64.MakeTable(crc64.ISO)

// NewID derives a deterministic [NodeID] from a string by computing a CRC-64
// hash. Panics if the same string is used twice in the same frame (duplicate
// IDs). Use this for nodes whose position in the tree may change.
func NewID(id string) NodeID {
	hash := NodeID(crc64.Checksum([]byte(id), _crc64Table))

	if slices.Contains(uniqueIDs, hash) {
		panic(fmt.Errorf("duplicated unique node ID: %q (hash: %d)", id, hash))
	}
	uniqueIDs = append(uniqueIDs, hash)
	return hash
}

// AddSubWindow registers subWindow with the global sub-window list and places
// its root at position. For floating windows position is the content area
// origin (the header is drawn above it); the header offset is applied
// automatically.
//
// Sub-windows are kept sorted by [ZIndex]; same-index windows are ordered by
// insertion time. The function returns subWindow for convenience.
func AddSubWindow(subWindow *SubWindow, position rl.Vector2) *SubWindow {
	if subWindow.floating {
		position.Y += SubWindowHeaderHeight
	}

	subWindow.root.relPos = position

	subWindows = append(subWindows, subWindow)
	slices.SortStableFunc(subWindows, func(a, b *SubWindow) int {
		var out int
		if a.zIndex > b.zIndex {
			out = 1
		} else if a.zIndex < b.zIndex {
			out = -1
		}
		return out
	})

	return subWindow
}

// RemoveSubWindow unregisters subWindow from the global sub-window list.
// Panics if subWindow is not currently registered.
func RemoveSubWindow(subWindow *SubWindow) {
	found := false
	idx := len(subWindows) - 1
	for ; idx >= 0; idx-- {
		if subWindows[idx] == subWindow {
			found = true
			break
		}
	}
	assert.True(found)

	for ; idx < len(subWindows)-1; idx++ {
		subWindows[idx] = subWindows[idx+1]
	}

	subWindows = subWindows[:len(subWindows)-1]
}

func bubbleSubWindowUp(subWindow *SubWindow) {
	idx := len(subWindows) - 1
	for ; idx >= 0; idx-- {
		if subWindows[idx] == subWindow {
			break
		}
	}

	for ; idx < len(subWindows)-1; idx++ {
		if subWindows[idx].zIndex < subWindows[idx+1].zIndex {
			break
		}
		subWindows[idx] = subWindows[idx+1]
	}

	subWindows[idx] = subWindow
}

// SubWindowProps holds the configuration options for [NewSubWindow].
type SubWindowProps struct {
	// For internal debugging
	DebugID string

	// SizingX controls how the sub-window's root box sizes itself horizontally.
	// Defaults to [SizingShrink] when zero.
	SizingX SizingProp

	// SizingY controls how the sub-window's root box sizes itself vertically.
	// Defaults to [SizingShrink] when zero.
	SizingY SizingProp

	// ZIndex determines the rendering order among all sub-windows. Use the
	// [ZIndexRoot], [ZIndexBase], [ZIndexPopup], [ZIndexEphemeral] constants
	// as starting points.
	ZIndex ZIndex

	// Floating makes the sub-window draggable with a header bar rendered above
	// it. Floating windows are clamped to the screen bounds while dragging.
	Floating bool

	// Hidden sets the initial visibility of the sub-window.
	Hidden bool

	// Closable adds an × button to the header of floating sub-windows.
	// Clicking it calls [RemoveSubWindow].
	Closable bool

	// HeaderColor is the fill color of the draggable header bar drawn above
	// floating sub-windows.
	HeaderColor rl.Color
}

// SubWindow is a self-contained layer that owns a root [*Box] and a
// [NodeIDGenerator]. The entire GUI is composed of one or more sub-windows
// stacked in z-index order. The primary full-screen window typically uses
// [ZIndexRoot]; transient overlays (dropdowns, tooltips, floating panels) get
// higher z-indices.
type SubWindow struct {
	// For internal debugging
	DebugID     string
	root        *Box
	idGenerator NodeIDGenerator

	// HeaderColor is the background color of the draggable title bar rendered
	// above floating sub-windows.
	HeaderColor rl.Color

	zIndex         ZIndex
	hidden         bool
	skipNextRender bool
	floating       bool
	closable       bool
	isDragging     bool
}

// NewSubWindow allocates and initialises a SubWindow from props.
// After creation, register it with [AddSubWindow].
func NewSubWindow(props SubWindowProps) *SubWindow {
	if props.HeaderColor == (Color{}) {
		props.HeaderColor = rl.LightGray
	}

	subWindow := &SubWindow{
		DebugID:     props.DebugID,
		floating:    props.Floating,
		hidden:      props.Hidden,
		closable:    props.Closable,
		zIndex:      props.ZIndex,
		HeaderColor: props.HeaderColor,
		root: NewBox(BoxProps{
			SizingX: props.SizingX,
			SizingY: props.SizingY,
		}),
	}
	subWindow.root.subWindow = subWindow

	return subWindow
}

// ResetIDs clears all auto-generated IDs in this sub-window's generator and
// removes the associated nodes from the global cache.
func (w *SubWindow) ResetIDs() {
	w.idGenerator.Reset()
}

// RestartIDs rewinds the auto-ID cursor to zero without touching the cache.
// Called automatically by [ResetLayout] each frame.
func (w *SubWindow) RestartIDs() {
	w.idGenerator.Restart()
}

// GetAutoID issues the next auto-incremented [NodeID] from this sub-window's
// generator. Use this in the BoxProps.ID field of stateful widgets (Slider,
// Toggle, TextInput, ScrollBox, Dropdown) that do not have a stable position in
// the tree. The sequence must be called in exactly the same order every frame
// so that IDs remain stable.
func (w *SubWindow) GetAutoID() NodeID {
	return w.idGenerator.GetID()
}

// Root returns the current root [*Box] of the sub-window.
func (w *SubWindow) Root() *Box {
	return w.root
}

// SetRoot replaces the sub-window's root box with root, transferring the
// current position and sub-window reference. Returns root for convenience.
func (w *SubWindow) SetRoot(root *Box) *Box {
	root.relPos = w.root.relPos
	root.subWindow = w
	w.root = root
	return root
}

// IsHidden reports whether the sub-window is currently hidden.
func (w *SubWindow) IsHidden() bool {
	return w.hidden
}

// SetHidden shows or hides the sub-window. Equivalent to calling [Show] or
// [Hide].
func (w *SubWindow) SetHidden(hidden bool) {
	if hidden {
		w.Hide()
	} else {
		w.Show()
	}
}

// Hide marks the sub-window as hidden.
func (w *SubWindow) Hide() {
	w.hidden = true
	w.skipNextRender = true
}

// Show marks the sub-window as visible and brings it to the top of its z-index
// tier.
func (w *SubWindow) Show() {
	w.hidden = false
	w.skipNextRender = true
	bubbleSubWindowUp(w)
}

// SetSizingX updates the horizontal sizing mode of the sub-window's root box.
func (w *SubWindow) SetSizingX(sizing SizingProp) {
	w.root.setSizingX(sizing)
}

// SetSizingY updates the vertical sizing mode of the sub-window's root box.
func (w *SubWindow) SetSizingY(sizing SizingProp) {
	w.root.setSizingY(sizing)
}

// Move sets the absolute position of the sub-window's content area to
// position.
func (w *SubWindow) Move(position rl.Vector2) {
	w.root.relPos = position
}

// Translate shifts the sub-window by delta pixels.
func (w *SubWindow) Translate(delta rl.Vector2) {
	w.root.relPos = rl.Vector2Add(w.root.relPos, delta)
}

// Drag is identical to [Translate] but clamps the resulting position so the
// window stays within the screen bounds (including the header).
// Only valid for floating sub-windows; panics otherwise.
func (w *SubWindow) Drag(delta rl.Vector2) {
	assert.True(w.floating)

	newPos := rl.Vector2Add(w.root.relPos, delta)

	newPos.X = Clamp(newPos.X, 0, screenSize.X-w.Rect().Width)
	newPos.Y = Clamp(newPos.Y, SubWindowHeaderHeight, screenSize.Y)

	w.root.relPos = newPos
}

// Rect returns the bounding rectangle of the sub-window, including the header
// bar if it is a floating window.
func (w *SubWindow) Rect() rl.Rectangle {
	rect := w.root.Rect()
	if w.floating {
		rect.Y -= SubWindowHeaderHeight
		rect.Height += SubWindowHeaderHeight
	}
	return rect
}

// HeaderBounds returns the rectangle of the draggable header bar for floating
// sub-windows. Returns an empty rectangle for non-floating windows.
func (w *SubWindow) HeaderBounds() rl.Rectangle {
	if !w.floating {
		return rl.Rectangle{}
	}

	rect := w.root.Rect()
	rect.Y -= SubWindowHeaderHeight
	rect.Height = SubWindowHeaderHeight
	return rect
}

// ResetLayout clears the root node's children list. Called automatically by
// [ResetLayout] each frame.
func (w *SubWindow) ResetLayout() {
	w.root.children = w.root.children[:0]
	clear(w.root.children[:cap(w.root.children)])
}

// ComputeLayout computes the layout for this sub-window.
// Sub-windows that are hidden are skipped.
func (w *SubWindow) ComputeLayout() {
	if w.IsHidden() {
		return
	}

	if w.root.size.max.X == 0 {
		w.root.size.max.X = InfPositive
	}
	if w.root.size.max.Y == 0 {
		w.root.size.max.Y = InfPositive
	}

	computeNodeSizeX(w.root)
	switch w.root.size.mode.X {
	case SizingGrow:
		w.root.size.X = min(screenSize.X, w.root.size.max.X)
	case SizingPercentage:
		w.root.size.X = min(screenSize.X*w.root.size.percentageX(), w.root.size.max.X)
	case SizingFixed, SizingShrink:
	case SizingAspectRatio:
		assert.Unreachable("x axis's sizing mode cannot be SizingAspectRatio")
	default:
		assert.Unimplemented(w.root.size.mode.X)
	}
	computeChildrenSizeX(w.root)

	wrapTextNodeChildren(w.root)

	computeNodeSizeY(w.root)
	switch w.root.size.mode.Y {
	case SizingGrow:
		w.root.size.Y = min(screenSize.Y, w.root.size.max.Y)
	case SizingPercentage:
		w.root.size.Y = min(screenSize.Y*w.root.size.percentageY(), w.root.size.max.Y)
	case SizingFixed, SizingShrink, SizingAspectRatio:
	default:
		assert.Unimplemented(w.root.size.mode.Y)
	}
	computeChildrenSizeY(w.root)

	computeScrollNodeContentSize(w.root)

	computeChildrenPosition(w.root)

	rect := w.root.Rect()
	w.root.relPos.X = Clamp(rect.X, 0, screenSize.X-rect.Width)
	w.root.relPos.Y = Clamp(rect.Y, 0, screenSize.Y-rect.Height)
}

// Update handles dragging and the close-button interaction for floating
// sub-windows. Hidden sub-windows are skipped.
// Called automatically by [Update].
func (w *SubWindow) Update() {
	if w.IsHidden() || !w.floating {
		return
	}

	if w.isDragging {
		if rl.IsMouseButtonDown(rl.MouseButtonLeft) {
			w.Drag(rl.GetMouseDelta())

		} else if IsMouseButtonReleased(rl.MouseButtonLeft) {
			w.isDragging = false
		}

	} else if GetHighestPriorityNodeUnderMouse() == nil && IsMouseButtonPressed(rl.MouseLeftButton) {
		headerRect := w.HeaderBounds()

		if rl.CheckCollisionPointRec(rl.GetMousePosition(), headerRect) {
			ConsumeMouseButtonPressed(rl.MouseLeftButton)

			closeButtonRect := headerRect
			closeButtonRect.X += headerRect.Width - SubWindowHeaderHeight
			closeButtonRect.Width = SubWindowHeaderHeight
			closeButtonRect.Height = SubWindowHeaderHeight

			if w.closable && rl.CheckCollisionPointRec(rl.GetMousePosition(), closeButtonRect) {
				RemoveSubWindow(w)
			} else {
				w.isDragging = true
			}
		}
	}
}

// Render draws the sub-window's root node tree and, for floating windows, the
// draggable header bar (including the × close button if Closable is set).
// Hidden sub-windows are skipped; the first frame after Show/Hide is also
// skipped to prevent one-frame flicker.
func (w *SubWindow) Render() {
	if w.skipNextRender {
		w.skipNextRender = false
		return
	}

	if w.hidden {
		return
	}

	rootCornerRadius := w.root.CornerRadius
	if w.floating {
		w.root.CornerRadius.TopLeft = 0
		w.root.CornerRadius.TopRight = 0
	}
	w.root.Render()
	w.root.CornerRadius = rootCornerRadius

	rect := w.root.Rect()

	wholeSubWindowRect := rect

	if w.floating {
		const lineWidth = 2
		lineColor := ColorContrast(w.HeaderColor, 0.5)

		wholeSubWindowRect.Y -= SubWindowHeaderHeight
		wholeSubWindowRect.Height += SubWindowHeaderHeight

		headerRect := rect
		headerRect.Y -= SubWindowHeaderHeight
		headerRect.Height = SubWindowHeaderHeight

		headerCornerRadius := w.root.CornerRadius
		headerCornerRadius.BottomLeft = 0
		headerCornerRadius.BottomRight = 0
		DrawRectangle(headerRect, BoxSides{}, headerCornerRadius, Color{}, w.HeaderColor)
		// lineY := headerRect.Y + headerRect.Height
		// rl.DrawLineEx(
		// 	Vec2(headerRect.X, lineY),
		// 	Vec2(headerRect.X+headerRect.Width, lineY),
		// 	lineWidth, lineColor,
		// )

		if w.closable {
			closeButtonX := headerRect.X + headerRect.Width - SubWindowHeaderHeight
			// rl.DrawLineEx(
			// 	Vec2(closeButtonX, headerRect.Y),
			// 	Vec2(closeButtonX, headerRect.Y+headerRect.Height),
			// 	lineWidth, lineColor,
			// )

			var pad = SubWindowHeaderHeight / 3.
			const err = 1.
			rl.DrawLineEx(
				Vec2(closeButtonX+pad-err, headerRect.Y+pad),
				Vec2(closeButtonX+SubWindowHeaderHeight-pad, headerRect.Y+headerRect.Height-pad+err),
				lineWidth, lineColor,
			)
			rl.DrawLineEx(
				Vec2(closeButtonX+pad-err, headerRect.Y+headerRect.Height-pad+err),
				Vec2(closeButtonX+SubWindowHeaderHeight-pad, headerRect.Y+pad),
				lineWidth, lineColor,
			)
		}
	}

	// rl.DrawRectangleLinesEx(wholeSubWindowRect, w.BorderWidth, w.BorderColor)
}

func computeNodeSizeX(node Node) rl.Vector2 {
	assert.False(node == nil)

	box := node.box()
	if box.ignored {
		return rl.Vector2{}
	}

	if box.size.mode.X == SizingFixed {
		for child := range box.iterChildrenNode(false) {
			computeNodeSizeX(child)
		}
		return box.size.Vector2
	}

	if box.GetComputedChildCount() == 0 {
		box.size.X = node.ComputeMinInnerSizeX() + box.padding.X()
		goto End
	}

	switch box.orientation {
	case OrientationHorizontal:
		if scrollBox := node.scrollBox(); scrollBox != nil && scrollBox.scrollOrientation == OrientationHorizontal && !scrollBox.childWrap {
			box.size.X = node.ComputeMinInnerSizeX()
			for child := range box.iterChildrenNode(false) {
				computeNodeSizeX(child)
			}

		} else if box.childWrap {
			box.size.X = node.ComputeMinInnerSizeX() + box.padding.X()
			var widestChild float32
			for child := range box.iterChildrenNode(false) {
				childSize := computeNodeSizeX(child)
				widestChild = max(widestChild, childSize.X)
			}
			box.size.X += widestChild

		} else {
			box.size.X = node.ComputeMinInnerSizeX() + box.padding.X() + box.totalChildGaps()
			for child := range box.iterChildrenNode(false) {
				childSize := computeNodeSizeX(child)
				box.size.X += childSize.X
			}
		}

	case OrientationVertical:
		box.size.X = node.ComputeMinInnerSizeX() + box.padding.X()
		var widestChild float32
		for child := range box.iterChildrenNode(false) {
			childSize := computeNodeSizeX(child)
			widestChild = max(widestChild, childSize.X)
		}
		box.size.X += widestChild

	default:
		assert.Unreachable()
	}

End:
	if box.size.max.X == 0 {
		box.size.max.X = InfPositive
	}
	box.size.X = Clamp(box.size.X, box.size.min.X, box.size.max.X)

	return box.size.Vector2
}

func computeNodeSizeY(node Node) rl.Vector2 {
	assert.False(node == nil)

	box := node.box()
	if box.ignored {
		return rl.Vector2{}
	}

	if box.size.mode.Y == SizingFixed {
		for child := range box.iterChildrenNode(false) {
			computeNodeSizeY(child)
		}
		return box.size.Vector2
	}

	if box.size.mode.Y == SizingAspectRatio {
		for child := range box.iterChildrenNode(false) {
			computeNodeSizeY(child)
		}
		box.size.Y = box.size.X / box.size.ratio
		return box.size.Vector2
	}

	if box.GetComputedChildCount() == 0 {
		box.size.Y = node.ComputeMinInnerSizeY() + box.padding.Y()
		goto End
	}

	switch box.orientation {
	case OrientationHorizontal:
		if scrollBox := node.scrollBox(); scrollBox != nil && scrollBox.scrollOrientation == OrientationVertical && box.childWrap {
			box.size.Y = node.ComputeMinInnerSizeY()
			for child := range box.iterChildrenNode(false) {
				computeNodeSizeY(child)
			}

		} else if scrollBox == nil && box.childWrap {
			box.size.Y = node.ComputeMinInnerSizeY() + box.padding.Y()
			maxInnerWidth := box.size.X - box.padding.X()
			rowCount := 1
			var currRowHeight float32
			var cursor float32
			if box.GetComputedChildCount() > 0 {
				cursor = -box.childGap
			}
			for child := range box.iterChildrenBox(false) {
				if cursor+box.childGap+child.size.X > maxInnerWidth {
					rowCount++
					box.size.Y += box.childGap + currRowHeight
					cursor = child.size.X
					currRowHeight = child.size.Y
					continue
				}
				currRowHeight = max(currRowHeight, child.size.Y)
				cursor += box.childGap + child.size.X
			}
			if rowCount == 1 {
				box.size.Y -= box.childGap
			}
			box.size.Y += box.childGap + currRowHeight
			for child := range box.iterChildrenNode(false) {
				computeNodeSizeY(child)
			}

		} else {
			box.size.Y = node.ComputeMinInnerSizeY() + box.padding.Y()
			var highestChild float32
			for child := range box.iterChildrenNode(false) {
				childSize := computeNodeSizeY(child)
				highestChild = max(highestChild, childSize.Y)
			}
			box.size.Y += highestChild
		}

	case OrientationVertical:
		if scrollBox := node.scrollBox(); scrollBox != nil && scrollBox.scrollOrientation == OrientationVertical {
			box.size.Y = node.ComputeMinInnerSizeY()
			for child := range box.iterChildrenNode(false) {
				computeNodeSizeY(child)
			}

		} else {
			box.size.Y = node.ComputeMinInnerSizeY() + box.padding.Y() + box.totalChildGaps()
			for child := range box.iterChildrenNode(false) {
				childSize := computeNodeSizeY(child)
				box.size.Y += childSize.Y
			}
		}

	default:
		assert.Unreachable()
	}

End:
	if box.size.max.Y == 0 {
		box.size.max.Y = InfPositive
	}
	box.size.Y = Clamp(box.size.Y, box.size.min.Y, box.size.max.Y)

	return box.size.Vector2
}

func computeChildrenSizeX(node Node) {
	assert.False(node == nil)

	box := node.box()
	if box.ignored {
		return
	}

	if box.GetComputedChildCount() == 0 {
		return
	}

	switch box.orientation {
	case OrientationHorizontal:
		freeSpace := box.size.X - box.padding.X() - box.totalChildGaps()

		nodesBuf1 := make([]*Box, 0, box.GetComputedChildCount())
		nodesBuf2 := make([]*Box, 0, box.GetComputedChildCount())

		for child := range box.iterChildrenBox(false) {
			if child.size.mode.X == SizingPercentage {
				child.size.X = Clamp((box.size.X-box.padding.X())*child.size.percentageX(), child.size.min.X, child.size.max.X)
			}

			freeSpace -= child.size.X

			if child.size.mode.X == SizingGrow {
				nodesBuf1 = append(nodesBuf1, child)
			}
		}

		for len(nodesBuf1) > 0 && freeSpace > 0.1 {
			smallest := InfPositive
			secondSmallest := InfPositive

			for _, child := range nodesBuf1 {
				if child.size.X < smallest {
					secondSmallest = smallest
					smallest = child.size.X
					continue
				}
				if child.size.X == smallest {
					continue
				}
				if child.size.X < secondSmallest {
					secondSmallest = child.size.X
				}
			}

			widthToAdd := secondSmallest - smallest
			if widthToAdd == 0 {
				widthToAdd = freeSpace / float32(len(nodesBuf1))
			} else {
				widthToAdd = min(widthToAdd, freeSpace/float32(len(nodesBuf1)))
			}

			for _, child := range nodesBuf1 {
				if child.size.X == smallest {
					child.size.X += widthToAdd
					freeSpace -= widthToAdd

					if child.size.X > child.size.max.X {
						freeSpace += child.size.X - child.size.max.X
						child.size.X = child.size.max.X
					}

					if child.size.X == child.size.max.X {
						continue
					}
				}
				nodesBuf2 = append(nodesBuf2, child)
			}
			nodesBuf1, nodesBuf2 = nodesBuf2, nodesBuf1[:0]
		}

	case OrientationVertical:
		freeSpace := box.size.X - box.padding.X()

		for child := range box.iterChildrenBox(false) {
			switch child.size.mode.X {
			case SizingPercentage:
				child.size.X = Clamp((box.size.X-box.padding.X())*child.size.percentageX(), child.size.min.X, child.size.max.X)
			case SizingGrow:
				child.size.X = Clamp(freeSpace, child.size.min.X, child.size.max.X)
			}
		}

	default:
		assert.Unreachable()
	}

	for child := range box.iterChildrenNode(false) {
		computeChildrenSizeX(child)
	}
}

func computeChildrenSizeY(node Node) {
	assert.False(node == nil)

	box := node.box()
	if box.ignored {
		return
	}

	if box.GetComputedChildCount() == 0 {
		return
	}

	switch box.orientation {
	case OrientationHorizontal:
		freeSpace := box.size.Y - box.padding.Y()

		for child := range box.iterChildrenBox(false) {
			switch child.size.mode.Y {
			case SizingPercentage:
				child.size.Y = Clamp((box.size.Y-box.padding.Y())*child.size.percentageY(), child.size.min.Y, child.size.max.Y)
			case SizingGrow:
				child.size.Y = Clamp(freeSpace, child.size.min.Y, child.size.max.Y)
			}
		}

	case OrientationVertical:
		freeSpace := box.size.Y - box.padding.Y() - box.totalChildGaps()

		nodesBuf1 := make([]*Box, 0, box.GetComputedChildCount())
		nodesBuf2 := make([]*Box, 0, box.GetComputedChildCount())

		for child := range box.iterChildrenBox(false) {
			if child.size.mode.Y == SizingPercentage {
				child.size.Y = Clamp((box.size.Y-box.padding.Y())*child.size.percentageY(), child.size.min.Y, child.size.max.Y)
			}

			freeSpace -= child.size.Y

			if child.size.mode.Y == SizingGrow {
				nodesBuf1 = append(nodesBuf1, child)
			}
		}

		for len(nodesBuf1) > 0 && freeSpace > 0.1 {
			smallest := float32(math.Inf(1))
			secondSmallest := float32(math.Inf(1))

			for _, child := range nodesBuf1 {
				if child.size.Y < smallest {
					secondSmallest = smallest
					smallest = child.size.Y
					continue
				}
				if child.size.Y == smallest {
					continue
				}
				if child.size.Y < secondSmallest {
					secondSmallest = child.size.Y
				}
			}

			heightToAdd := secondSmallest - smallest
			if heightToAdd == 0 {
				heightToAdd = freeSpace / float32(len(nodesBuf1))
			} else {
				heightToAdd = min(heightToAdd, freeSpace/float32(len(nodesBuf1)))
			}

			for _, child := range nodesBuf1 {
				if child.size.Y == smallest {
					child.size.Y += heightToAdd
					freeSpace -= heightToAdd

					if child.size.Y > child.size.max.Y {
						freeSpace += child.size.Y - child.size.max.Y
						child.size.Y = child.size.max.Y
					}

					if child.size.Y == child.size.max.Y {
						continue
					}
				}
				nodesBuf2 = append(nodesBuf2, child)
			}
			nodesBuf1, nodesBuf2 = nodesBuf2, nodesBuf1[:0]
		}

	default:
		assert.Unreachable()
	}

	for child := range box.iterChildrenNode(false) {
		computeChildrenSizeY(child)
	}
}

var _computeChildrenPositionRowsBuf []struct {
	width     float32
	height    float32
	nodeCount uint16
}

func computeChildrenPosition(node Node) {
	assert.False(node == nil)

	box := node.box()
	if box.ignored {
		return
	}

	if box.GetComputedChildCount() == 0 {
		return
	}

	rowsBuf := _computeChildrenPositionRowsBuf[:0]

	if box.orientation == OrientationHorizontal && box.childWrap {
		isFirstRow := true
		limitX := box.size.X - box.padding.Right
		cursorX := box.padding.Left
		for child := range box.iterChildrenBox(false) {
			if isFirstRow || cursorX+child.size.X > limitX {
				isFirstRow = true
				cursorX = box.padding.Left
				rowsBuf = append(rowsBuf, struct {
					width     float32
					height    float32
					nodeCount uint16
				}{
					width: -box.childGap,
				})
			}
			isFirstRow = false
			rowsBuf[len(rowsBuf)-1].nodeCount++
			rowsBuf[len(rowsBuf)-1].width += box.childGap + child.size.X
			rowsBuf[len(rowsBuf)-1].height = max(rowsBuf[len(rowsBuf)-1].height, child.size.Y)
			cursorX += child.size.X + box.childGap
		}
	}

	var scrollOffset rl.Vector2
	if scrollBox := node.scrollBox(); scrollBox != nil {
		switch scrollBox.scrollOrientation {
		case OrientationHorizontal:
			scrollOffset.X = -scrollBox.scrollDistance
		case OrientationVertical:
			scrollOffset.Y = -scrollBox.scrollDistance
		}
	}

	switch box.orientation {
	case OrientationHorizontal:
		switch box.childAlign.X {
		case AlignStart:
			if box.childWrap {
				rowIdx := 0
				row := rowsBuf[rowIdx]
				cursorPos := box.padding.Left
				for child := range box.iterChildrenBox(false) {
					if row.nodeCount == 0 {
						rowIdx++
						row = rowsBuf[rowIdx]
						cursorPos = box.padding.Left
					}
					row.nodeCount--
					child.relPos.X = cursorPos + scrollOffset.X
					cursorPos += child.size.X + box.childGap
				}
			} else {
				cursorPos := box.padding.Left
				for child := range box.iterChildrenBox(false) {
					child.relPos.X = cursorPos + scrollOffset.X
					cursorPos += child.size.X + box.childGap
				}
			}

		case AlignEnd:
			if box.childWrap {
				rowIdx := 0
				row := rowsBuf[rowIdx]
				cursorPos := box.size.X - box.padding.Right
				for child := range box.iterChildrenBox(false) {
					if row.nodeCount == 0 {
						rowIdx++
						row = rowsBuf[rowIdx]
						cursorPos = box.size.X - box.padding.Right
					}
					row.nodeCount--
					child.relPos.X = cursorPos - child.size.X + scrollOffset.X
					cursorPos -= child.size.X + box.childGap
				}
			} else {
				cursorPos := box.size.X - box.padding.Right
				for child := range box.iterChildrenBox(true) {
					child.relPos.X = cursorPos - child.size.X + scrollOffset.X
					cursorPos -= child.size.X + box.childGap
				}
			}

		case AlignCenter:
			if box.childWrap {
				rowIdx := 0
				row := rowsBuf[rowIdx]
				cursorPos := max((box.size.X-row.width)/2, box.padding.Left)
				for child := range box.iterChildrenBox(false) {
					if row.nodeCount == 0 {
						rowIdx++
						row = rowsBuf[rowIdx]
						cursorPos = max((box.size.X-row.width)/2, box.padding.Left)
					}
					row.nodeCount--
					child.relPos.X = cursorPos + scrollOffset.X
					cursorPos += child.size.X + box.childGap
				}
			} else {
				totalWidth := box.totalChildGaps()
				for child := range box.iterChildrenNode(false) {
					totalWidth += child.box().size.X
				}
				cursorPos := max((box.size.X-totalWidth)/2, box.padding.Left)
				for child := range box.iterChildrenBox(false) {
					child.relPos.X = cursorPos + scrollOffset.X
					cursorPos += child.size.X + box.childGap
				}
			}
		}

		switch box.childAlign.Y {
		case AlignStart:
			if box.childWrap {
				rowIdx := 0
				row := rowsBuf[rowIdx]
				cursorPos := box.padding.Top
				for child := range box.iterChildrenBox(false) {
					if row.nodeCount == 0 {
						rowIdx++
						row = rowsBuf[rowIdx]
						cursorPos += box.childGap + row.height
					}
					row.nodeCount--
					child.relPos.Y = cursorPos + scrollOffset.Y
				}
			} else {
				for child := range box.iterChildrenNode(false) {
					child.box().relPos.Y = box.padding.Top + scrollOffset.Y
				}
			}

		case AlignEnd:
			if box.childWrap {
				rowIdx := 0
				row := rowsBuf[rowIdx]
				cursorPos := box.size.Y - box.padding.Bottom - row.height
				for child := range box.iterChildrenBox(true) {
					if row.nodeCount == 0 {
						rowIdx++
						row = rowsBuf[rowIdx]
						cursorPos -= box.childGap + row.height
					}
					row.nodeCount--
					child.relPos.Y = cursorPos + scrollOffset.Y
				}
			} else {
				for child := range box.iterChildrenBox(true) {
					child.relPos.Y = box.size.Y - box.padding.Bottom - child.size.Y + scrollOffset.Y
				}
			}

		case AlignCenter:
			if box.childWrap {
				rowsHeightSum := float32(len(rowsBuf)-1) * box.childGap
				for _, row := range rowsBuf {
					rowsHeightSum += row.height
				}
				rowIdx := 0
				row := rowsBuf[rowIdx]
				cursorPos := max((box.size.Y-rowsHeightSum)/2, box.padding.Top)
				for child := range box.iterChildrenBox(false) {
					if row.nodeCount == 0 {
						cursorPos += box.childGap + row.height
						rowIdx++
						row = rowsBuf[rowIdx]
					}
					row.nodeCount--
					child.relPos.Y = cursorPos + scrollOffset.Y
				}
			} else {
				for child := range box.iterChildrenBox(false) {
					child.relPos.Y = max((box.size.Y-child.size.Y)/2, box.padding.Top) + scrollOffset.Y
				}
			}
		}

	case OrientationVertical:
		switch box.childAlign.X {
		case AlignStart:
			for child := range box.iterChildrenNode(false) {
				child.box().relPos.X = box.padding.Left + scrollOffset.X
			}
		case AlignEnd:
			for child := range box.iterChildrenBox(false) {
				child.relPos.X = box.size.X - box.padding.Right - child.size.X + scrollOffset.X
			}
		case AlignCenter:
			for child := range box.iterChildrenBox(false) {
				child.relPos.X = max((box.size.X-child.size.X)/2, box.padding.Left) + scrollOffset.X
			}
		}

		switch box.childAlign.Y {
		case AlignStart:
			cursorPos := box.padding.Top
			for child := range box.iterChildrenBox(false) {
				child.relPos.Y = cursorPos + scrollOffset.Y
				cursorPos += child.size.Y + box.childGap
			}
		case AlignEnd:
			cursorPos := box.size.Y - box.padding.Bottom
			for child := range box.iterChildrenBox(true) {
				child.relPos.Y = cursorPos - child.size.Y + scrollOffset.Y
				cursorPos -= child.size.Y + box.childGap
			}
		case AlignCenter:
			totalHeight := box.totalChildGaps()
			for child := range box.iterChildrenNode(false) {
				totalHeight += child.box().size.Y
			}
			cursorPos := max((box.size.Y-totalHeight)/2, box.padding.Top)
			for child := range box.iterChildrenBox(false) {
				child.relPos.Y = cursorPos + scrollOffset.Y
				cursorPos += child.size.Y + box.childGap
			}
		}

	default:
		assert.Unreachable()
	}

	_computeChildrenPositionRowsBuf = rowsBuf

	for child := range box.iterChildrenNode(false) {
		computeChildrenPosition(child)
	}
}

func wrapTextNodeChildren(node Node) {
	assert.False(node == nil)

	if node.box().ignored {
		return
	}

	switch n := node.(type) {
	case *Text:
		n.wrapText()

	default:
		for _, child := range node.box().children {
			wrapTextNodeChildren(child)
		}
	}
}

func computeScrollNodeContentSize(node Node) {
	assert.False(node == nil)

	box := node.box()
	if box.ignored {
		return
	}

	for child := range box.iterChildrenNode(false) {
		computeScrollNodeContentSize(child)
	}

	scrollBox := node.scrollBox()
	if scrollBox == nil {
		return
	}

	switch scrollBox.orientation {
	case OrientationHorizontal:
		if scrollBox.scrollOrientation == OrientationHorizontal && !scrollBox.childWrap {
			scrollBox.contentSize = scrollBox.padding.X() + scrollBox.totalChildGaps()
			for childBox := range scrollBox.iterChildrenBox(false) {
				scrollBox.contentSize += childBox.size.X
			}

		} else if scrollBox.scrollOrientation == OrientationVertical && scrollBox.childWrap {
			scrollBox.contentSize = box.padding.Y()

			maxRowWidth := box.size.X - box.padding.X()

			var rowWidth float32
			var tallestChild float32

			for childBox := range box.iterChildrenBox(false) {
				if rowWidth == 0 {
					rowWidth = childBox.size.X
					tallestChild = childBox.size.Y

				} else if rowWidth+box.childGap+childBox.size.X <= maxRowWidth {
					rowWidth += box.childGap + childBox.size.X
					tallestChild = max(tallestChild, childBox.size.Y)

				} else {
					scrollBox.contentSize += tallestChild + box.childGap
					rowWidth = childBox.size.X
					tallestChild = childBox.size.Y
				}
			}
			scrollBox.contentSize += tallestChild

		} else {
			scrollBox.contentSize = 0
		}

	case OrientationVertical:
		if scrollBox.scrollOrientation == OrientationVertical {
			scrollBox.contentSize = scrollBox.padding.Y() + scrollBox.totalChildGaps()
			for child := range scrollBox.iterChildrenNode(false) {
				scrollBox.contentSize += child.box().size.Y
			}

		} else {
			scrollBox.contentSize = 0
		}

	default:
		assert.Unreachable()
	}
}

// AddChild appends child to parent's child list and returns child.
func AddChild[T Node](parent Node, child T) T {
	parent.box().addChild(parent, child)
	return child
}

// DebuggingInfo draws the debug overlay for node when [Debug] is true.
// Custom [Node] implementations should call this at the end of their Render
// method to participate in the debug visualisation.
func DebuggingInfo(node Node) {
	if Debug {
		box := node.box()
		pos := box.AbsPos()
		pad := box.padding
		if pad.X()+pad.Y() > 0 {
			rl.DrawRectangleLinesEx(Rect(pos.X+pad.Left-1, pos.Y+pad.Top-1, box.size.X-pad.Left-pad.Right+2, box.size.Y-pad.Top-pad.Bottom+2), 1, DebugLinesColor)
		}
		rl.DrawRectangleLinesEx(Rect(pos.X-1, pos.Y-1, box.size.X+2, box.size.Y+2), 1, DebugLinesColor)

		printNodeInfoOnMousePress(node)
	}
}

func printNodeInfoOnMousePress(node Node) {
	box := node.box()
	pos := box.AbsPos()
	rect := box.Rect()

	if !rl.IsMouseButtonPressed(rl.MouseButtonLeft) || !rl.CheckCollisionPointRec(rl.GetMousePosition(), rect) {
		return
	}

	kind := strings.SplitN(reflect.ValueOf(node).Type().String(), ".", 2)[1]

	if box.debugID != "" {
		fmt.Print("Debug ID: ", box.debugID, "  ")
	}
	fmt.Println("Kind:", kind, "Pos:", pos, "Sizing:", box.size)
}
