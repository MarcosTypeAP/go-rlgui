package gui

import (
	"errors"
	"io/fs"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/MarcosTypeAP/go-assert"
	rl "github.com/gen2brain/raylib-go/raylib"
)

func returnDefault[T comparable](prop T, defautlValue T) T {
	var zero T
	if prop == zero {
		return defautlValue
	}
	return prop
}

func setDefault[T comparable](dest *T, prop T, defautlValue T) {
	var zero T
	if prop == zero {
		*dest = defautlValue
	} else {
		*dest = prop
	}
}

// Sizing holds all size-related state for a single [*Box]. The embedded
// [rl.Vector2] carries the resolved pixel dimensions (X, Y) after layout.
// The remaining fields are set from a [SizingProp] and consumed by the layout
// engine.
type Sizing struct {
	rl.Vector2
	min        rl.Vector2
	max        rl.Vector2
	ratio      float32
	percentage struct{ X, Y uint8 }
	mode       struct{ X, Y SizingMode }
}

func (s Sizing) percentageX() float32 {
	return float32(s.percentage.X) / 100
}

func (s Sizing) percentageY() float32 {
	return float32(s.percentage.Y) / 100
}

// SizingProp encodes a sizing specification for one axis of a [*Box].
type SizingProp struct {
	size       float32
	min        float32
	max        float32
	ratio      float32
	percentage uint8
	mode       SizingMode
}

// NewSizingShrink returns a [SizingProp] that sizes the box to its minimum
// content size. An optional first argument sets the minimum pixel size; an
// optional second argument sets the maximum pixel size (default: unbounded).
func NewSizingShrink(v ...float32) SizingProp {
	sizing := SizingProp{
		mode: SizingShrink,
	}

	if len(v) >= 1 {
		sizing.min = v[0]
	}
	if len(v) >= 2 {
		sizing.max = v[1]
	} else {
		sizing.max = InfPositive
	}

	return sizing
}

// NewSizingGrow returns a [SizingProp] that expands the box to fill available
// space along the parent's main axis. Optional min/max pixel constraints are
// accepted in the same way as [NewSizingShrink].
func NewSizingGrow(v ...float32) SizingProp {
	sizing := NewSizingShrink(v...)
	sizing.mode = SizingGrow
	return sizing
}

// NewSizingFixed returns a [SizingProp] that locks the box to exactly size
// pixels. Min and max are both set to size so layout cannot change it.
func NewSizingFixed(size float32) SizingProp {
	return SizingProp{
		mode: SizingFixed,
		size: size,
		min:  size,
		max:  size,
	}
}

// NewSizingPercentage returns a [SizingProp] that sizes the box to a
// percentage of the parent's size on the same axis. percentage must be in
// [0, 100]. Optional min/max pixel constraints are accepted in the same way as
// [NewSizingShrink].
func NewSizingPercentage(percentage uint8, v ...float32) SizingProp {
	assert.LessEqual(percentage, 100)

	sizing := SizingProp{
		mode:       SizingPercentage,
		percentage: percentage,
	}

	if len(v) >= 1 {
		sizing.min = v[0]
	}
	if len(v) >= 2 {
		sizing.max = v[1]
	} else {
		sizing.max = InfPositive
	}

	return sizing
}

// NewSizingAspectRatio returns a [SizingProp] for the Y axis that derives the
// height from the already-computed width divided by ratio.
// Use this on the Y axis only; setting it on the X axis panics.
func NewSizingAspectRatio(ratio float32) SizingProp {
	return SizingProp{
		mode:  SizingAspectRatio,
		max:   InfPositive,
		ratio: ratio,
	}
}

// GradientKind distinguishes between the supported gradient styles.
type GradientKind uint8

const (
	GradientKindLinear GradientKind = iota
	GradientKindRadial              // Deprecated: Unimplemented
)

// Gradient describes a color gradient used as a [*Box] background.
type Gradient struct {
	// StartColor is the first color of the gradient
	// (the "from" color for linear, or the centre color for radial).
	StartColor rl.Color

	// EndColor is the second color of the gradient
	// (the "to" color for linear, or the edge color for radial).
	EndColor rl.Color

	// Angle stores the direction in radians for a linear gradient.
	// A sentinel value of [InfPositive] marks a radial gradient.
	Angle float32
}

// NewGradientLinear creates a linear [Gradient] from startColor to endColor
// rotated by angleDegrees (0 = left-to-right).
func NewGradientLinear(startColor, endColor rl.Color, angleDegrees float32) Gradient {
	return Gradient{
		StartColor: startColor,
		EndColor:   endColor,
		Angle:      angleDegrees * rl.Deg2rad,
	}
}

// NewGradientRadial creates a radial [Gradient] from centerColor at the center
// to edgeColor at the edges.
//
// Deprecated: Unimplemented
func NewGradientRadial(centerColor, edgeColor rl.Color) Gradient {
	assert.Unimplemented()
	return Gradient{
		StartColor: centerColor,
		EndColor:   edgeColor,
		Angle:      InfPositive,
	}
}

// IsVisible reports whether the gradient will produce any visible pixels
// (i.e. at least one color has a non-zero alpha channel).
func (g Gradient) IsVisible() bool {
	return g.StartColor.A != 0 || g.EndColor.A != 0
}

// Kind returns the [GradientKind] of the gradient, determined by its internal
// Angle value.
func (g Gradient) Kind() GradientKind {
	kind := GradientKindLinear
	if g.Angle == InfPositive {
		kind = GradientKindRadial
		assert.Unimplemented()
	}
	return kind
}

// Alignment controls how children or text are positioned along one axis inside
// a container.
type Alignment uint8

const (
	AlignStart Alignment = iota
	AlignEnd
	AlignCenter
)

// SizingMode describes how a [*Box]'s size on one axis is determined during
// layout.
type SizingMode uint8

const (
	// SizingUnset means no sizing mode has been assigned, and represents an
	// invalid state.
	SizingUnset SizingMode = iota

	// SizingShrink makes the box as small as its content requires.
	SizingShrink

	// SizingGrow distributes free space among all growing siblings along the
	// parent's orientation axis.
	SizingGrow

	// SizingFixed sets the box to an exact pixel size regardless of content or
	// available space.
	SizingFixed

	// SizingPercentage sets the box size to a percentage of the parent's size
	// on the same axis.
	SizingPercentage

	// SizingAspectRatio (Y axis only) derives the height from the already-
	// computed width by dividing by a given ratio.
	SizingAspectRatio
)

// BoxSides holds four float32 values representing the four sides of a box —
// typically used for padding, or border widths.
type BoxSides struct {
	Top, Right, Bottom, Left float32
}

// Y returns the sum of Top and Bottom.
func (b BoxSides) Y() float32 {
	return b.Top + b.Bottom
}

// X returns the sum of Left and Right.
func (b BoxSides) X() float32 {
	return b.Left + b.Right
}

func (b BoxSides) slice() []float32 {
	return []float32{b.Top, b.Right, b.Bottom, b.Left}
}

func (b BoxSides) validate() error {
	if b.Top < 0 || b.Right < 0 || b.Bottom < 0 || b.Left < 0 {
		return errors.New("box sides cannot be negative")
	}
	return nil
}

// BoxCorners holds per-corner radius values for a box.
type BoxCorners struct {
	TopLeft, TopRight, BottomRight, BottomLeft float32
}

func (b BoxCorners) slice() []float32 {
	return []float32{b.TopLeft, b.TopRight, b.BottomRight, b.BottomLeft}
}

func (b BoxCorners) limit(rect rl.Rectangle) BoxCorners {
	if b.TopLeft+b.TopRight > rect.Width {
		b.TopLeft = min(b.TopLeft, rect.Width/2)
		b.TopRight = min(b.TopRight, rect.Width/2)
	}
	if b.TopRight+b.BottomRight > rect.Height {
		b.TopRight = min(b.TopRight, rect.Height/2)
		b.BottomRight = min(b.BottomRight, rect.Height/2)
	}
	if b.BottomRight+b.BottomLeft > rect.Width {
		b.BottomRight = min(b.BottomRight, rect.Width/2)
		b.BottomLeft = min(b.BottomLeft, rect.Width/2)
	}
	if b.BottomLeft+b.TopLeft > rect.Height {
		b.BottomLeft = min(b.BottomLeft, rect.Height/2)
		b.TopLeft = min(b.TopLeft, rect.Height/2)
	}
	return b
}

func (b BoxCorners) validate() error {
	if b.TopLeft < 0 || b.TopRight < 0 || b.BottomRight < 0 || b.BottomLeft < 0 {
		return errors.New("box corners cannot be negative")
	}
	return nil
}

// Orientation controls whether a [*Box]'s children are laid out in a row
// (horizontal) or a column (vertical).
type Orientation uint8

const (
	OrientationHorizontal Orientation = iota
	OrientationVertical
)

// NewBoxSides creates a [BoxSides] value following CSS shorthand conventions:
//
//	NewBoxSides(a)          → top/right/bottom/left = a
//	NewBoxSides(a, b)       → top/bottom = a, left/right = b
//	NewBoxSides(a, b, c)    → top = a, left/right = b, bottom = c
//	NewBoxSides(a, b, c, d) → top = a, right = b, bottom = c, left = d
//
// All values must be non-negative.
func NewBoxSides(v ...float32) BoxSides {
	assert.InRange(len(v), 1, 4)

	var boxSides BoxSides

	switch len(v) {
	case 1:
		boxSides = BoxSides{v[0], v[0], v[0], v[0]}
	case 2:
		boxSides = BoxSides{v[0], v[1], v[0], v[1]}
	case 3:
		boxSides = BoxSides{v[0], v[1], v[2], v[1]}
	case 4:
		boxSides = BoxSides{v[0], v[1], v[2], v[3]}
	}

	assert.NoError(boxSides.validate())
	return boxSides
}

// NewBoxSidesOverride returns a copy of boxSides with individual sides replaced.
// A negative value leaves that side unchanged.
func NewBoxSidesOverride(boxSides BoxSides, top, right, bottom, left float32) BoxSides {
	if top >= 0 {
		boxSides.Top = top
	}
	if right >= 0 {
		boxSides.Right = right
	}
	if bottom >= 0 {
		boxSides.Bottom = bottom
	}
	if left >= 0 {
		boxSides.Left = left
	}

	assert.NoError(boxSides.validate())
	return boxSides
}

// NewBoxCorners creates a [BoxCorners] value following CSS shorthand
// conventions:
//
//	TL = Top Left, TR = Top Right, BR = Bottom Right, BL = Bottom Left
//
//	NewBoxCorners(a)          → TL/TR/BR/BL = a
//	NewBoxCorners(a, b)       → TL/BR = a, TR/BL = b
//	NewBoxCorners(a, b, c)    → TL = a, TR/BL = b, BR = c
//	NewBoxCorners(a, b, c, d) → TL = a, TR = b, BR = c, BL = d
//
// All values must be non-negative.
func NewBoxCorners(v ...float32) BoxCorners {
	assert.InRange(len(v), 1, 4)

	var boxCorners BoxCorners

	switch len(v) {
	case 1:
		boxCorners = BoxCorners{v[0], v[0], v[0], v[0]}
	case 2:
		boxCorners = BoxCorners{v[0], v[1], v[0], v[1]}
	case 3:
		boxCorners = BoxCorners{v[0], v[1], v[2], v[1]}
	case 4:
		boxCorners = BoxCorners{v[0], v[1], v[2], v[3]}
	}

	assert.NoError(boxCorners.validate())
	return boxCorners
}

// NewBoxCornersOverride returns a copy of boxCorners with individual corners
// replaced. A negative value leaves that corner unchanged.
func NewBoxCornersOverride(boxCorners BoxCorners, topLeft, topRight, bottomRight, bottomLeft float32) BoxCorners {
	if topLeft >= 0 {
		boxCorners.TopLeft = topLeft
	}
	if topRight >= 0 {
		boxCorners.TopRight = topRight
	}
	if bottomRight >= 0 {
		boxCorners.BottomRight = bottomRight
	}
	if bottomLeft >= 0 {
		boxCorners.BottomLeft = bottomLeft
	}

	assert.NoError(boxCorners.validate())
	return boxCorners
}

// TextWrapping controls how a [*Text] node handles content wider than its
// bounding box.
type TextWrapping uint8

const (
	// TextNoWrap renders text on a single line.
	TextNoWrap TextWrapping = iota

	// TextWrap breaks text onto multiple lines at word boundaries.
	TextWrap

	// TextEllipsisOverflow clips text and appends "..." when it overflows.
	TextEllipsisOverflow
)

// TextureFit controls how an image is scaled to fit inside a [*Box] that has a
// texture set.
type TextureFit uint8

const (
	// TextureFitCover scales the texture uniformly until it covers the entire
	// box, cropping any overflow.
	TextureFitCover TextureFit = iota

	// TextureFitContain scales the texture uniformly so it fits entirely within
	// the box, leaving empty strips if aspect ratios differ.
	TextureFitContain

	_TextureFitCount
)

// Node is the interface implemented by every element in the UI tree.
// The library provides concrete implementations (e.g. [*Box], [*Button], etc.).
// You can also implement Node yourself to create custom widgets; embed [*Box]
// (or any other node that embeds it) and override the methods you need.
//
// ComputeMinInnerSizeX / ComputeMinInnerSizeY return the minimum content size
// that should be reserved inside the padding area; override them in custom
// nodes that have intrinsic size (e.g. text, images).
type Node interface {
	box() *Box
	scrollBox() *ScrollBox
	iterChildrenNode(backward bool) iter.Seq[Node]
	iterChildrenBox(backward bool) iter.Seq[*Box]

	// ID returns the node's [NodeID].
	ID() NodeID

	// TotalArea returns the rectangle that should be used for hit-testing.
	// For most nodes this is the same as Rect(); nodes that expand visually
	// (e.g. an open [*Dropdown]) override this to report the larger area.
	TotalArea() rl.Rectangle

	// ComputeMinInnerSizeX returns the minimum width required for the node's
	// own content, not counting padding. Called during the X layout pass.
	ComputeMinInnerSizeX() float32

	// ComputeMinInnerSizeY returns the minimum height required for the node's
	// own content, not counting padding. Called during the Y layout pass.
	ComputeMinInnerSizeY() float32

	// Update is called once per frame for every node, after ComputeLayout,
	// to process input events.
	Update()

	// Render draws the node and its children. Implementations should guard
	// with [IsNodeVisible] and call [DebuggingInfo] at the end.
	Render()
}

// ChildlessNode is a mix-in for node types that must not have children.
// Embed it in custom node structs to get a panicking AddChild method.
type ChildlessNode struct{}

// AddChild panics unconditionally. Embedding [ChildlessNode] in a node type
// prevents child nodes from being attached to it.
func (n *ChildlessNode) AddChild(thisPanics Node) {
	panic("this node can not have children")
}

// BoxProps is the configuration struct for [NewBox] and for the BoxProps field
// embedded in the Props structs of derived node types.
type BoxProps struct {
	// DebugID is an optional human-readable label printed by the debug overlay.
	DebugID string

	// ID is the stable cache key for stateful widgets. Generate one with
	// [SubWindow.GetAutoID] or [NewID]. Leave zero for stateless boxes.
	// Set it to [NodeIDManual] to handle the node lifecycle manually.
	ID NodeID

	// AllocChildren pre-allocates the children slice to avoid reallocations
	// when the number of children is known in advance.
	AllocChildren uint

	// SizingX controls horizontal sizing. Defaults to [SizingShrink].
	SizingX SizingProp

	// SizingY controls vertical sizing. Defaults to [SizingShrink].
	SizingY SizingProp

	// Padding is the space between the box border and its children.
	Padding BoxSides

	// ChildGap is the pixel gap inserted between consecutive children.
	ChildGap float32

	// CornerRadius rounds the corners of the box.
	CornerRadius BoxCorners

	// BorderWidth is the per-side border thickness.
	BorderWidth BoxSides

	// Texture is an optional GPU texture drawn as the box background.
	Texture rl.Texture2D

	// Gradient is an optional gradient drawn as the box background.
	Gradient Gradient

	// BgColor is the solid fill color.
	BgColor rl.Color

	// BorderColor is the color of the border stroke.
	BorderColor rl.Color

	// TextureTint is multiplied against the texture color. Defaults to white.
	TextureTint rl.Color

	// TextureFit controls how the texture is scaled within the box.
	TextureFit TextureFit

	// Orientation controls whether children are laid out horizontally or
	// vertically.
	Orientation Orientation

	// ChildAlignX controls horizontal alignment of children inside the box.
	ChildAlignX Alignment

	// ChildAlignY controls vertical alignment of children inside the box.
	ChildAlignY Alignment

	// ChildWrap enables multi-line (wrapping) layout for horizontal boxes.
	// Children that do not fit on the current row are moved to the next row.
	ChildWrap bool

	// Invisible hides the box visually (no background drawn, no children
	// rendered) while keeping it in the layout.
	Invisible bool

	// Ignored removes the box from both layout and rendering as if it did not
	// exist.
	Ignored bool

	// HideOverflow clips any child content that extends beyond the box's inner
	// (post-padding) rectangle using a scissor rect.
	HideOverflow bool
}

// Box is the fundamental building block of the UI and the base for any node.
// It is a rectangular container that can hold children, display a background
// color, gradient, or texture, and draw a bordered rounded rectangle.
type Box struct {
	debugID   string
	id        NodeID
	parent    Node // nil = root
	subWindow *SubWindow
	children  []Node

	relPos   rl.Vector2 // abs if root
	size     Sizing
	padding  BoxSides
	childGap float32

	// Texture is an optional GPU texture drawn as this box's background.
	Texture rl.Texture2D

	// CornerRadius holds the per-corner radius values for this box.
	CornerRadius BoxCorners

	// BorderWidth holds the per-side border thickness for this box.
	BorderWidth BoxSides

	ignoredChildCount uint16

	// TextureTint is the color multiplied against the Texture. Defaults to
	// white.
	TextureTint rl.Color

	// BgColor is the solid background color of this box.
	BgColor rl.Color

	// BorderColor is the border stroke color.
	BorderColor rl.Color

	orientation Orientation
	childAlign  struct{ X, Y Alignment }
	childWrap   bool
	ignored     bool
	textureFit  TextureFit

	// Gradient is the background gradient of this box.
	Gradient Gradient

	// Invisible hides the box visually (no background drawn, no children
	// rendered) while keeping it in the layout.
	Invisible bool

	// HideOverflow clips any child content that extends beyond the box's inner
	// (post-padding) rectangle using a scissor rect.
	HideOverflow bool
}

// NewBox allocates a Box and applies props.
func NewBox(props BoxProps) *Box {
	node := new(Box)
	node.ApplyProps(props)
	return node
}

// ApplyProps updates the box in place with the values from props.
func (n *Box) ApplyProps(props BoxProps) {
	n.debugID = props.DebugID

	n.id = props.ID

	setDefault(&props.SizingX, props.SizingX, NewSizingShrink())
	setDefault(&props.SizingY, props.SizingY, NewSizingShrink())
	n.setSizingX(props.SizingX)
	n.setSizingY(props.SizingY)

	n.orientation = props.Orientation

	n.childAlign.X = props.ChildAlignX
	n.childAlign.Y = props.ChildAlignY

	n.padding = props.Padding
	n.childGap = props.ChildGap
	n.childWrap = props.ChildWrap

	n.BorderWidth = props.BorderWidth
	n.CornerRadius = props.CornerRadius
	n.BorderColor = props.BorderColor

	n.Gradient = props.Gradient
	n.BgColor = props.BgColor

	n.Texture = props.Texture
	n.textureFit = props.TextureFit

	setDefault(&n.TextureTint, props.TextureTint, rl.White)

	if props.AllocChildren > 0 {
		n.children = slices.Grow(n.children, int(props.AllocChildren))
	}

	n.ignored = props.Ignored
	n.Invisible = props.Invisible
	n.HideOverflow = props.HideOverflow
}

func (n *Box) setSizingX(sizing SizingProp) {
	assert.NotEqual(sizing.mode, SizingAspectRatio, "Only the Y axis can be in aspect ratio mode")

	n.size.mode.X = sizing.mode
	n.size.X = sizing.size
	n.size.min.X = sizing.min
	n.size.max.X = sizing.max
	n.size.percentage.X = sizing.percentage
	n.size.ratio = sizing.ratio
}

func (n *Box) setSizingY(sizing SizingProp) {
	n.size.mode.Y = sizing.mode
	n.size.Y = sizing.size
	n.size.min.Y = sizing.min
	n.size.max.Y = sizing.max
	n.size.percentage.Y = sizing.percentage
	n.size.ratio = sizing.ratio
}

func (n *Box) box() *Box {
	return n
}

func (n *Box) scrollBox() *ScrollBox {
	return nil
}

func (n *Box) iterChildrenNode(backward bool) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		if backward {
			for i := len(n.children) - 1; i >= 0; i-- {
				if n.children[i].box().ignored {
					continue
				}
				if !yield(n.children[i]) {
					return
				}
			}
		} else {
			for _, child := range n.children {
				if child.box().ignored {
					continue
				}
				if !yield(child) {
					return
				}
			}
		}
	}
}

func (n *Box) iterChildrenBox(backward bool) iter.Seq[*Box] {
	return func(yield func(*Box) bool) {
		if backward {
			for i := len(n.children) - 1; i >= 0; i-- {
				box := n.children[i].box()
				if box.ignored {
					continue
				}
				if !yield(box) {
					return
				}
			}
		} else {
			for _, child := range n.children {
				box := child.box()
				if box.ignored {
					continue
				}
				if !yield(box) {
					return
				}
			}
		}
	}
}

// ID returns the [NodeID] assigned to this box.
func (n *Box) ID() NodeID {
	return n.id
}

// TotalArea implements [Node.TotalArea].
func (n *Box) TotalArea() rl.Rectangle {
	return n.Rect()
}

// Rect returns the box's bounding rectangle in screen-space coordinates.
func (n *Box) Rect() rl.Rectangle {
	pos := n.AbsPos()
	return rl.Rectangle{
		X:      pos.X,
		Y:      pos.Y,
		Width:  n.size.X,
		Height: n.size.Y,
	}
}

// InnerRect returns the rectangle of the content area, i.e. the bounding
// rectangle shrunk by the box's padding on all sides.
func (n *Box) InnerRect() rl.Rectangle {
	pos := n.AbsPos()
	return rl.Rectangle{
		X:      pos.X + n.padding.Left,
		Y:      pos.Y + n.padding.Top,
		Width:  n.size.X - n.padding.X(),
		Height: n.size.Y - n.padding.Y(),
	}
}

// AbsPos returns the absolute screen-space position of the box's top-left
// corner.
func (n *Box) AbsPos() rl.Vector2 {
	if n.parent == nil {
		return n.relPos
	}
	parentPos := n.parent.box().AbsPos()
	return rl.Vector2Add(parentPos, n.relPos)
}

func (n *Box) totalChildGaps() float32 {
	return n.childGap * float32(n.GetComputedChildCount()-1)
}

// ComputeMinInnerSizeX implements [Node.ComputeMinInnerSizeX].
// ComputeMinInnerSizeX returns 0 for a plain box (no intrinsic width).
func (n *Box) ComputeMinInnerSizeX() float32 {
	return 0
}

// ComputeMinInnerSizeY implements [Node.ComputeMinInnerSizeY].
// ComputeMinInnerSizeY returns 0 for a plain box (no intrinsic height).
func (n *Box) ComputeMinInnerSizeY() float32 {
	return 0
}

func (n *Box) resetLayout() {
	n.parent = nil
	n.relPos = rl.Vector2{}
	if n.size.mode.X != SizingFixed {
		n.size.X = 0
	}
	if n.size.mode.Y != SizingFixed {
		n.size.Y = 0
	}
	n.children = n.children[:0]
}

// Update implements [Node.Update].
// Update is a no-op for plain boxes.
func (n *Box) Update() {}

// Render implements [Node.Render].
// Render draws the box background (solid color, gradient, or texture), then
// recursively renders all children. If HideOverflow is set, children are
// clipped to the [Box.InnerRect].
func (n *Box) Render() {
	if !IsNodeVisible(n) {
		return
	}

	rect := n.Rect()

	switch {
	case rl.IsTextureValid(n.Texture):
		DrawRectangleWithTexture(rect, n.BorderWidth, n.CornerRadius, n.BorderColor, n.Texture, n.TextureTint, n.textureFit)

	case n.Gradient.IsVisible():
		DrawRectangleWithGradient(rect, n.BorderWidth, n.CornerRadius, n.BorderColor, n.Gradient)

	case n.BgColor != (Color{}) || (n.BorderWidth != (BoxSides{}) && n.BorderColor != (rl.Color{})):
		DrawRectangle(rect, n.BorderWidth, n.CornerRadius, n.BorderColor, n.BgColor)
	}

	if n.HideOverflow {
		BeginScissorMode(n.InnerRect())
		defer EndScissorMode()
	}

	for child := range n.iterChildrenNode(false) {
		child.Render()
	}

	DebuggingInfo(n)
}

func (n *Box) addChild(parent, child Node) {
	childBox := child.box()
	childBox.parent = parent
	childBox.subWindow = n.subWindow
	childBox.updateChildrenSubWindow()
	n.children = append(n.children, child)
	if childBox.ignored {
		n.ignoredChildCount++
	}
}

func (n *Box) updateChildrenSubWindow() {
	for child := range n.iterChildrenBox(false) {
		child.subWindow = n.subWindow
		child.updateChildrenSubWindow()
	}
}

// GetChild returns the child at index idx. Negative indices count from the
// end (e.g. -1 returns the last child). Panics if idx is out of range.
func (n *Box) GetChild(idx int) Node {
	if idx < 0 {
		idx = len(n.children) + idx
	}
	assert.Greater(len(n.children), idx)
	return n.children[idx]
}

// GetComputedChildCount returns the number of children that participate in
// layout (i.e. total children minus those with Ignored set).
func (n *Box) GetComputedChildCount() int {
	return len(n.children) - int(n.ignoredChildCount)
}

// NewBoxImage creates a [*Box] whose background is an image loaded from
// imagePath inside fileSystem. The image is loaded via [LoadImageTexture] and
// cached. Ensure imagePath and fileSystem are valid to avoid panicking.
func NewBoxImage(props BoxProps, imagePath string, fileSystem fs.FS) *Box {
	var err error
	props.Texture, err = LoadImageTexture(imagePath, fileSystem)
	assert.NoError(err, imagePath)

	node := new(Box)
	node.ApplyProps(props)
	return node
}

// Spacer is an invisible box used to consume free space along the parent's
// main axis.
type Spacer struct {
	Box
}

// NewSpacerX creates a horizontal spacer. An optional [SizingProp] controls
// how much space it occupies; defaults to [NewSizingGrow].
func NewSpacerX(sizing ...SizingProp) *Spacer {
	assert.InRange(len(sizing), 0, 1)

	var sizingX SizingProp
	if len(sizing) > 0 {
		sizingX = sizing[0]
	} else {
		sizingX = NewSizingGrow()
	}

	node := new(Spacer)
	node.ApplyProps(BoxProps{
		SizingX: sizingX,
	})
	return node
}

// NewSpacerY creates a vertical spacer. An optional [SizingProp] controls
// how much space it occupies; defaults to [NewSizingGrow].
func NewSpacerY(sizing ...SizingProp) *Spacer {
	assert.InRange(len(sizing), 0, 1)

	var sizingY SizingProp
	if len(sizing) > 0 {
		sizingY = sizing[0]
	} else {
		sizingY = NewSizingGrow()
	}

	node := new(Spacer)
	node.ApplyProps(BoxProps{
		SizingY: sizingY,
	})
	return node
}

// Render implements [Node.Render].
// Render is a no-op; spacers are invisible.
func (n *Spacer) Render() {}

// FontConfigProps is the configuration for font rendering inside a node.
// Embed in node Props structs that display text.
type FontConfigProps struct {
	// FgColor is the text color; defaults to [DefaultTextColor].
	FgColor rl.Color

	// Font is the font to use; defaults to [GetDefaultFont].
	Font rl.Font

	// FontSize is the text size in pixels; defaults to [DefaultFontSize].
	FontSize float32

	// CharSpacing is the extra pixel gap between glyphs; defaults to
	// [DefaultCharSpacing].
	CharSpacing float32

	// BaselineCorrectionFactor nudges the text upward by this fraction of the
	// font size; defaults to [DefaultFontBaselineCorrectionFactor].
	BaselineCorrectionFactor float32
}

// FontConfig holds the resolved font configuration for a node after defaults
// have been applied. It is embedded in node types that render text.
type FontConfig struct {
	font                     rl.Font
	fontSize                 float32
	charSpacing              float32
	baselineCorrectionFactor float32
	FgColor                  rl.Color
}

// ApplyProps resolves the FontConfigProps into the FontConfig, substituting
// library-level defaults for any zero values.
func (c *FontConfig) ApplyProps(props FontConfigProps) {
	if !rl.IsFontValid(props.Font) {
		props.Font = GetDefaultFont()
	}

	c.font = props.Font
	setDefault(&c.fontSize, props.FontSize, DefaultFontSize)
	setDefault(&c.charSpacing, props.CharSpacing, DefaultCharSpacing)
	setDefault(&c.baselineCorrectionFactor, props.BaselineCorrectionFactor, DefaultFontBaselineCorrectionFactor)
	setDefault(&c.FgColor, props.FgColor, DefaultTextColor)
}

// TextProps is the configuration struct for [Text].
type TextProps struct {
	BoxProps

	FontConfigProps

	// Wrapping controls how text behaves when it exceeds its bounding box
	// width.
	Wrapping TextWrapping
}

// Text is a node that renders a string of text.
type Text struct {
	Box
	FontConfig

	text         string
	textComputed bool
	wrapping     TextWrapping
}

// NewText creates a Text node with the given props and content string.
func NewText(props TextProps, text string) *Text {
	node := &Text{
		text: text,
	}
	node.ApplyProps(props)
	return node
}

// ApplyProps updates the Text node's configuration.
func (n *Text) ApplyProps(props TextProps) {
	n.Box.ApplyProps(props.BoxProps)

	n.FontConfig.ApplyProps(props.FontConfigProps)

	n.wrapping = props.Wrapping
}

func (n *Text) wrapText() {
	n.textComputed = true

	if n.wrapping != TextWrap {
		return
	}

	innerRect := n.InnerRect()
	n.text = WrapText(innerRect, n.font, n.text, n.fontSize, n.charSpacing)
}

// ComputeMinInnerSizeX implements [Node.ComputeMinInnerSizeX].
// ComputeMinInnerSizeX returns the minimum horizontal space required for the
// text content.
func (n *Text) ComputeMinInnerSizeX() float32 {
	assert.NotEqual(n.size.mode.X, SizingFixed)

	if n.text == "" {
		return 0
	}

	switch n.wrapping {
	case TextWrap:
		var maxWidth float32
		for word := range strings.SplitSeq(n.text, " ") {
			size := rl.MeasureTextEx(n.font, word, n.fontSize, n.charSpacing)
			maxWidth = max(maxWidth, size.X)
		}
		return maxWidth

	case TextNoWrap:
		size := rl.MeasureTextEx(n.font, n.text, n.fontSize, n.charSpacing)
		return size.X

	case TextEllipsisOverflow:
		return 0
	}

	assert.Unreachable()
	return 0
}

// ComputeMinInnerSizeY implements [Node.ComputeMinInnerSizeY].
// ComputeMinInnerSizeY returns the height of a single line of text (accounting
// for the baseline correction factor).
func (n *Text) ComputeMinInnerSizeY() float32 {
	assert.NotEqual(n.size.mode.Y, SizingFixed)

	if n.text == "" {
		return 0
	}

	height := rl.MeasureTextEx(n.font, n.text, n.fontSize, n.charSpacing).Y
	return height - n.fontSize*n.baselineCorrectionFactor
}

// Render implements [Node.Render].
// Render draws the text. Panics if [ComputeLayout] has not been called since
// the last [*Text.SetText] or [ResetLayout] (i.e. the text has not been wrapped yet).
func (n *Text) Render() {
	assert.True(n.textComputed, "Text node was not computed. You must call gui.ComputeLayout() before render")

	if !IsNodeVisible(n) {
		return
	}

	n.Box.Render()

	innerRect := n.InnerRect()

	switch n.wrapping {
	case TextNoWrap:
		textSize := rl.MeasureTextEx(n.font, n.text, n.fontSize, n.charSpacing)
		var pos rl.Vector2
		switch n.childAlign.X {
		case AlignStart:
			pos.X = innerRect.X
		case AlignEnd:
			pos.X = innerRect.X + innerRect.Width - textSize.X
		case AlignCenter:
			pos.X = innerRect.X + (innerRect.Width-textSize.X)/2
		}
		switch n.childAlign.Y {
		case AlignStart:
			pos.Y = innerRect.Y
		case AlignEnd:
			pos.Y = innerRect.Y + innerRect.Height - textSize.Y
		case AlignCenter:
			pos.Y = innerRect.Y + (innerRect.Height-textSize.Y)/2
		}
		rl.DrawTextEx(n.font, n.text, pos, n.fontSize, n.charSpacing, n.FgColor)

	case TextWrap:
		pos := Vec2(innerRect.X, innerRect.Y)
		rl.DrawTextEx(n.font, n.text, pos, n.fontSize, n.charSpacing, n.FgColor)

	case TextEllipsisOverflow:
		DrawTextEllipsis(innerRect, n.childAlign.X, n.childAlign.Y, n.font, n.text, n.fontSize, n.charSpacing, n.FgColor)

	default:
		assert.Unreachable()
	}

	DebuggingInfo(n)
}

// SetText updates the displayed string. If text is identical to the current
// value the call is a no-op; otherwise the node is marked as uncomputed, so
// [ComputeLayout] must be called before the next call to [Render].
func (n *Text) SetText(text string) {
	if text == n.text {
		return
	}
	n.text = text
	n.textComputed = false
}

// TextInputProps is the configuration struct for [*TextInput].
type TextInputProps struct {
	BoxProps

	FontConfigProps

	// ErrorColor is the border and error message color when ErrorMessage is
	// set. Defaults to [DefaultErrorColor].
	ErrorColor rl.Color

	// PlaceholderColor is the color used to render the placeholder text.
	// Defaults to a contrast-adjusted version of the foreground color.
	PlaceholderColor rl.Color
}

// TextInput is a single-line editable text field. It supports a placeholder
// string shown when the field is empty, an optional error message displayed
// below the field, keyboard navigation (arrows, backspace, delete, Ctrl+V),
// mouse navigation (click to move cursor), and a blinking text cursor.
//
// TextInput requires an ID. State is persisted in the node cache.
type TextInput struct {
	Box
	ChildlessNode
	FontConfig

	value                  []rune
	placeholder            string
	cursorBlinkStartMillis int64

	cursor      uint16
	renderShift uint16

	isFocused  bool
	hasChanged bool

	// ErrorMessage is displayed below the field in ErrorColor when non-empty.
	// Setting it to a non-empty string also highlights the field border.
	ErrorMessage string

	// ErrorColor is the color used for the error border and error message.
	ErrorColor rl.Color

	// PlaceholderColor is the color used to draw the placeholder text when
	// the field is empty.
	PlaceholderColor rl.Color
}

// NewTextInput creates (or retrieves from cache) a TextInput node.
// placeholder is shown when the field is empty.
// value is the initial text content.
// Requires a non-zero ID in props.
func NewTextInput(props TextInputProps, placeholder, value string) *TextInput {
	assert.NotEqual(props.ID, NodeIDUnset, "TextInput needs an ID")

	if node := GetNodeFromCache[*TextInput](props.ID); node != nil {
		return node
	}

	node := &TextInput{
		value:       []rune(value),
		placeholder: placeholder,
	}
	node.ApplyProps(props)

	CacheNode(node)
	return node
}

// ApplyProps updates the [*TextInput] configuration.
func (n *TextInput) ApplyProps(props TextInputProps) {
	n.Box.ApplyProps(props.BoxProps)

	n.FontConfig.ApplyProps(props.FontConfigProps)

	setDefault(&n.ErrorColor, props.ErrorColor, DefaultErrorColor)
	setDefault(&n.PlaceholderColor, props.PlaceholderColor, ColorContrast(n.FgColor, 0.6))
}

func (n *TextInput) textRect() rl.Rectangle {
	pos := n.AbsPos()
	height := rl.MeasureTextEx(n.font, "A", n.fontSize, 0).Y - n.fontSize*n.baselineCorrectionFactor

	return rl.Rectangle{
		X:      pos.X + n.padding.Left,
		Y:      pos.Y + (n.size.Y-height)/2,
		Width:  n.size.X - n.padding.X(),
		Height: height,
	}
}

// ComputeMinInnerSizeY implements [Node.ComputeMinInnerSizeY].
// ComputeMinInnerSizeY returns the height of its text value, used to set a minimum
// height for the field.
func (n *TextInput) ComputeMinInnerSizeY() float32 {
	return n.fontSize - n.fontSize*n.baselineCorrectionFactor
}

// Focus programmatically gives keyboard focus to this [*TextInput].
func (n *TextInput) Focus() {
	n.isFocused = true
	AddNodeToHighPriorityList(n)
}

// Blur programmatically removes keyboard focus from this [*TextInput].
func (n *TextInput) Blur() {
	n.isFocused = false
	RemoveNodeFromHighPriorityList(n)
}

// Update implements [Node.Update].
// Update processes keyboard input, mouse clicks for focus/cursor placement,
// and clipboard paste (Ctrl+V). Escape and Enter blur the field.
func (n *TextInput) Update() {
	n.hasChanged = false

	if IsNodeHovered(n) {
		SetMouseCursor(rl.MouseCursorIBeam)

		if !n.isFocused && IsMouseButtonPressed(rl.MouseButtonLeft) {
			n.Focus()
		}
	} else if IsMouseButtonPressed(rl.MouseButtonLeft) {
		n.Blur()
	}

	if !n.isFocused {
		return
	}

	startCursor := n.cursor

	switch {
	case rl.IsKeyPressed(rl.KeyBackspace) || rl.IsKeyPressedRepeat(rl.KeyBackspace):
		if n.cursor > 0 {
			n.hasChanged = true
			n.value = slices.Delete(n.value, int(n.cursor-1), int(n.cursor))
			n.cursor--
		}

	case rl.IsKeyPressed(rl.KeyDelete) || rl.IsKeyPressedRepeat(rl.KeyDelete):
		if n.cursor < uint16(len(n.value)) {
			n.hasChanged = true
			n.value = slices.Delete(n.value, int(n.cursor), int(n.cursor+1))
		}

	case rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressedRepeat(rl.KeyLeft):
		if n.cursor > 0 {
			n.cursor--
		}

	case rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressedRepeat(rl.KeyRight):
		if n.cursor < uint16(len(n.value)) {
			n.cursor++
		}

	case rl.IsKeyDown(rl.KeyLeftControl) && rl.IsKeyPressed(rl.KeyV):
		clipboardText := []rune(rl.GetClipboardText())
		n.hasChanged = true
		n.value = slices.Insert(n.value, int(n.cursor), clipboardText...)
		n.cursor += uint16(len(clipboardText))

	case rl.IsKeyPressed(rl.KeyEscape) || rl.IsKeyPressed(rl.KeyEnter):
		n.Blur()
	}

	textRect := n.textRect()
	mousePos := rl.GetMousePosition()

	if IsNodeHovered(n) && IsMouseButtonPressedConsume(rl.MouseButtonLeft) {
		cursorSet := false

		relMousePos := mousePos.X - textRect.X

		var textWidth float32
		for i, char := range n.value[n.renderShift:] {
			charWidth := rl.MeasureTextEx(n.font, string(char), n.fontSize, 0).X

			windowStart := textWidth - n.charSpacing
			windowEnd := textWidth + charWidth + n.charSpacing

			if relMousePos >= windowStart && relMousePos <= windowEnd {
				if relMousePos <= windowStart+(windowEnd-windowStart)/2 {
					n.cursor = n.renderShift + uint16(i)
				} else {
					n.cursor = n.renderShift + uint16(i+1)
				}
				cursorSet = true
				break
			}

			textWidth += charWidth + n.charSpacing

			if textWidth > textRect.Width {
				break
			}
		}

		if !cursorSet {
			n.cursor = uint16(len(n.value))
		} else {
			n.cursorBlinkStartMillis = time.Now().UnixMilli()
		}
	}

	if char := rl.GetCharPressed(); char != 0 {
		n.hasChanged = true
		n.value = slices.Insert(n.value, int(n.cursor), char)
		n.cursor++
	}

	if n.cursor < n.renderShift {
		n.renderShift = n.cursor
	}

	if rl.MeasureTextEx(n.font, string(n.value[n.renderShift:]), n.fontSize, n.charSpacing).X < textRect.Width {
		textWidth := -n.charSpacing
		for i, char := range slices.Backward(n.value) {
			charWidth := rl.MeasureTextEx(n.font, string(char), n.fontSize, 0).X
			textWidth += n.charSpacing + charWidth
			if textWidth > textRect.Width {
				break
			}
			n.renderShift = uint16(i)
		}
	} else {
		textWidthStartRenderToCursor := rl.MeasureTextEx(n.font, string(n.value[n.renderShift:n.cursor]), n.fontSize, n.charSpacing).X
		if textWidthStartRenderToCursor > textRect.Width {
			for _, char := range n.value[n.renderShift:n.cursor] {
				charWidth := rl.MeasureTextEx(n.font, string(char), n.fontSize, 0).X

				textWidthStartRenderToCursor -= charWidth + n.charSpacing
				n.renderShift++

				if textWidthStartRenderToCursor <= textRect.Width {
					break
				}
			}
		}
	}

	if n.cursor != startCursor {
		n.cursorBlinkStartMillis = time.Now().UnixMilli()
	}
}

// Render implements [Node.Render].
// Render draws the text field background, the current value (or placeholder
// when empty), the text cursor when focused, and the error message when set.
func (n *TextInput) Render() {
	if !IsNodeVisible(n) {
		return
	}

	borderColor := n.BorderColor
	borderWidth := n.BorderWidth
	if n.ErrorMessage != "" {
		n.BorderColor = n.ErrorColor
		if n.BorderWidth == (BoxSides{}) {
			n.BorderWidth = Border(2)
		}
	}
	{
		n.Box.Render()
	}
	n.BorderColor = borderColor
	n.BorderWidth = borderWidth

	textRect := n.textRect()

	if len(n.value) == 0 {
		dotWidth := rl.MeasureTextEx(n.font, ".", n.fontSize, 0).X
		ellipsisWidth := dotWidth + n.charSpacing + dotWidth + n.charSpacing + dotWidth

		textWidth := -n.charSpacing
		for _, char := range n.placeholder {
			charWidth := rl.MeasureTextEx(n.font, string(char), n.fontSize, 0).X

			if textWidth+n.charSpacing+charWidth+n.charSpacing+ellipsisWidth > textRect.Width {
				for i := range 3 {
					dotPosX := textRect.X + textWidth + n.charSpacing*float32(i+1) + dotWidth*float32(i)
					rl.DrawTextCodepoint(n.font, '.', Vec2(dotPosX, textRect.Y), n.fontSize, n.PlaceholderColor)
				}
				break
			}

			textWidth += n.charSpacing
			rl.DrawTextCodepoint(n.font, char, Vec2(textRect.X+textWidth, textRect.Y), n.fontSize, n.PlaceholderColor)
			textWidth += charWidth
		}
	} else {
		textWidth := -n.charSpacing
		for _, char := range n.value[n.renderShift:] {
			charWidth := rl.MeasureTextEx(n.font, string(char), n.fontSize, 0).X
			if textWidth+n.charSpacing+charWidth > textRect.Width {
				break
			}
			textWidth += n.charSpacing
			rl.DrawTextCodepoint(n.font, char, Vec2(textRect.X+textWidth, textRect.Y), n.fontSize, n.FgColor)
			textWidth += charWidth
		}
	}

	if n.isFocused {
		if (time.Now().UnixMilli()-n.cursorBlinkStartMillis)/1000%2 == 0 {
			cursorX := rl.MeasureTextEx(n.font, string(n.value[n.renderShift:n.cursor]), n.fontSize, n.charSpacing).X + n.charSpacing/2
			rl.DrawLineEx(Vec2(textRect.X+cursorX, textRect.Y), Vec2(textRect.X+cursorX, textRect.Y+textRect.Height), 2, n.FgColor)
		}
	}

	if n.ErrorMessage != "" {
		rect := n.Rect()
		errMsgPos := Vec2(rect.X+5, rect.Y+rect.Height+5)
		rl.DrawTextEx(n.font, n.ErrorMessage, errMsgPos, n.fontSize, n.charSpacing, n.ErrorColor)
	}

	DebuggingInfo(n)
}

// SetValue replaces the current text content with value, moving the cursor to
// the end.
func (n *TextInput) SetValue(value string) {
	n.value = n.value[:0]
	for _, char := range value {
		n.value = append(n.value, char)
	}
}

// Value returns the current text content as a string.
func (n *TextInput) Value() string {
	return string(n.value)
}

// HasChanged reports whether the text content changed during the last [Update]
// call.
func (n *TextInput) HasChanged() bool {
	return n.hasChanged
}

// Submitted reports whether the user pressed Enter while this field was
// focused.
func (n *TextInput) Submitted() bool {
	return n.isFocused && rl.IsKeyPressed(rl.KeyEnter)
}

// EffectFunc is a callback type used by [*Button] for hover and press effects.
// It receives a pointer to the button's [*Box] so that properties such as
// BgColor or Gradient can be modified temporarily for the frame. The returned
// function, if non-nil, is called after the box has been rendered to restore
// any properties that were mutated.
type EffectFunc func(box *Box) func()

// EffectBrighten is a simple [EffectFunc] util that lightens the button's
// background color for the current frame. The mutation is permanent within the
// frame and the returned cleanup function is nil.
func EffectBrighten(box *Box) func() {
	box.BgColor = rl.ColorLerp(box.BgColor, rl.White, 0.2)
	return nil
}

// ButtonProps is the configuration struct for [*Button].
type ButtonProps struct {
	BoxProps

	// OnHover is called each frame the button is hovered. Its return value
	// is called after the button is rendered.
	OnHover EffectFunc

	// OnPress is called each frame the left mouse button is held down over the
	// button. Its return value is called after rendering.
	OnPress EffectFunc
}

// Button is a clickable container node. It reports press and release events
// for both mouse buttons and applies optional visual effects on hover and
// press. Children are added with [AddChild] to build the button's visual
// content (icons, labels, etc.).
type Button struct {
	Box

	isLeftPressed  bool
	isRightPressed bool

	isLeftReleased  bool
	isRightReleased bool

	onHover EffectFunc
	onPress EffectFunc
}

// NewButton allocates a Button node and applies props.
func NewButton(props ButtonProps) *Button {
	node := new(Button)
	node.ApplyProps(props)
	return node
}

// ApplyProps updates the button's configuration.
func (n *Button) ApplyProps(props ButtonProps) {
	n.onHover = props.OnHover
	n.onPress = props.OnPress
	n.Box.ApplyProps(props.BoxProps)
}

// Update implements [Node.Update].
// Update records which mouse buttons were pressed or released over the button
// this frame and sets the mouse cursor to a pointer hand when hovered.
func (n *Button) Update() {
	n.isLeftPressed = false
	n.isRightPressed = false

	n.isLeftReleased = false
	n.isRightReleased = false

	if !IsNodeHovered(n) {
		return
	}
	SetMouseCursor(rl.MouseCursorPointingHand)

	n.isLeftPressed = IsMouseButtonPressedConsume(rl.MouseLeftButton)
	n.isRightPressed = IsMouseButtonPressedConsume(rl.MouseRightButton)

	n.isLeftReleased = IsMouseButtonReleasedConsume(rl.MouseLeftButton)
	n.isRightReleased = IsMouseButtonReleasedConsume(rl.MouseRightButton)
}

// Render implements [Node.Render].
// Render applies OnPress or OnHover effects when appropriate, draws the button
// background and children, and then calls the cleanup function returned by the
// effect (if any).
func (n *Button) Render() {
	if !IsNodeVisible(n) {
		return
	}

	var clean func()

	if IsNodeHovered(n) {
		if n.onPress != nil && rl.IsMouseButtonDown(rl.MouseButtonLeft) {
			clean = n.onPress(&n.Box)
		} else if n.onHover != nil {
			clean = n.onHover(&n.Box)
		}
	}

	n.Box.Render()

	if clean != nil {
		clean()
	}
}

// IsLeftButtonPressed reports whether the left mouse button was pressed over
// this button this frame.
func (n *Button) IsLeftButtonPressed() bool {
	return n.isLeftPressed
}

// IsLeftButtonReleased reports whether the left mouse button was released over
// this button this frame.
func (n *Button) IsLeftButtonReleased() bool {
	return n.isLeftReleased
}

// IsLeftButtonDown reports whether the left mouse button is currently held
// down while the cursor is over this button.
func (n *Button) IsLeftButtonDown() bool {
	return rl.IsMouseButtonDown(rl.MouseLeftButton) && IsNodeHovered(n)
}

// IsLeftButtonUp reports whether the left mouse button is currently up while
// the cursor is over this button.
func (n *Button) IsLeftButtonUp() bool {
	return rl.IsMouseButtonUp(rl.MouseLeftButton) && IsNodeHovered(n)
}

// IsRightButtonPressed reports whether the right mouse button was pressed over
// this button this frame.
func (n *Button) IsRightButtonPressed() bool {
	return n.isRightPressed
}

// IsRightButtonReleased reports whether the right mouse button was released
// over this button this frame.
func (n *Button) IsRightButtonReleased() bool {
	return n.isRightReleased
}

// IsRightButtonDown reports whether the right mouse button is currently held
// down while the cursor is over this button.
func (n *Button) IsRightButtonDown() bool {
	return rl.IsMouseButtonDown(rl.MouseRightButton) && IsNodeHovered(n)
}

// IsRightButtonUp reports whether the right mouse button is currently up while
// the cursor is over this button.
func (n *Button) IsRightButtonUp() bool {
	return rl.IsMouseButtonUp(rl.MouseRightButton) && IsNodeHovered(n)
}

// ToggleProps is the configuration struct for [*Toggle].
type ToggleProps struct {
	BoxProps

	// OnColor is the color of the thumb/fill when the toggle is on.
	// Defaults to rl.RayWhite.
	OnColor rl.Color

	// OffColor is the color of the thumb/fill when the toggle is off.
	// Defaults to rl.Gray.
	OffColor rl.Color
}

// Toggle is a boolean switch widget. It renders either as a sliding pill
// (when created with [NewToggle]) or as a filled square/circle checkbox (when
// created with [NewCheckBox]).
//
// Toggle requires an ID. State is persisted in the node cache.
type Toggle struct {
	Box
	ChildlessNode

	// OffColor is the color of the thumb/fill when the toggle is off.
	OffColor rl.Color

	// OnColor is the color of the thumb/fill when the toggle is on.
	OnColor rl.Color

	isCheckBox bool
	value      bool

	hasChanged bool
}

// NewToggle creates (or retrieves from cache) a sliding pill toggle.
// value is the initial on/off state. Requires a non-zero ID in props.
func NewToggle(props ToggleProps, value bool) *Toggle {
	assert.NotEqual(props.ID, NodeIDUnset, "Toggle needs an ID")

	if node := GetNodeFromCache[*Toggle](props.ID); node != nil {
		return node
	}

	node := &Toggle{
		value:      value,
		isCheckBox: false,
	}
	node.ApplyProps(props)

	CacheNode(node)
	return node
}

// NewCheckBox creates (or retrieves from cache) a checkbox-style toggle.
// value is the initial on/off state. Requires a non-zero ID in props.
func NewCheckBox(props ToggleProps, value bool) *Toggle {
	assert.NotEqual(props.ID, NodeIDUnset, "Toggle needs an ID")

	if node := GetNodeFromCache[*Toggle](props.ID); node != nil {
		return node
	}

	node := &Toggle{
		value:      value,
		isCheckBox: true,
	}
	node.ApplyProps(props)

	CacheNode(node)
	return node
}

// ApplyProps updates the toggle's visual properties, applying style-specific
// defaults (pill vs checkbox).
func (n *Toggle) ApplyProps(props ToggleProps) {
	if n.isCheckBox {
		setDefault(&props.SizingX, props.SizingX, NewSizingFixed(20))
		setDefault(&props.SizingY, props.SizingY, NewSizingAspectRatio(1))
		setDefault(&props.OnColor, props.OnColor, rl.RayWhite)
		setDefault(&props.OffColor, props.OffColor, rl.DarkGray)
	} else {
		setDefault(&props.SizingX, props.SizingX, NewSizingFixed(40))
		setDefault(&props.SizingY, props.SizingY, NewSizingAspectRatio(0.5))
		setDefault(&props.BgColor, props.BgColor, rl.DarkGray)
		setDefault(&props.OnColor, props.OnColor, rl.RayWhite)
		setDefault(&props.OffColor, props.OffColor, rl.Gray)
	}

	n.Box.ApplyProps(props.BoxProps)

	n.OnColor = props.OnColor
	n.OffColor = props.OffColor
}

// Update implements [Node.Update].
// Update flips the toggle state when clicked.
func (n *Toggle) Update() {
	if !IsNodeHovered(n) {
		return
	}
	SetMouseCursor(rl.MouseCursorPointingHand)

	n.hasChanged = false

	if IsMouseButtonPressed(rl.MouseLeftButton) {
		n.Toggle()
		n.hasChanged = true
	}
}

// Render implements [Node.Render].
// Render draws the toggle. For checkboxes it fills with OnColor or OffColor.
// For pill toggles it draws a background track and a sliding thumb.
func (n *Toggle) Render() {
	if !IsNodeVisible(n) {
		return
	}

	rect := n.Rect()

	if n.isCheckBox {
		if n.value {
			DrawRectangle(rect, n.BorderWidth, n.CornerRadius, n.BorderColor, n.OnColor)
		} else {
			DrawRectangle(rect, n.BorderWidth, n.CornerRadius, n.BorderColor, n.OffColor)
		}
	} else {
		DrawRectangle(rect, n.BorderWidth, n.CornerRadius, n.BorderColor, n.BgColor)
		handleRect := rect
		handleRect.Height = rect.Height - n.BorderWidth.X()
		handleRect.Width = rect.Height - n.BorderWidth.Y()
		handleRect.Y += n.BorderWidth.Top
		var fgColor rl.Color
		if n.value {
			fgColor = n.OnColor
			handleRect.X = rect.X + rect.Width - handleRect.Width - n.BorderWidth.Right
		} else {
			fgColor = n.OffColor
			handleRect.X += n.BorderWidth.Left
		}
		DrawRectangle(handleRect, BoxSides{}, n.CornerRadius, rl.Blank, fgColor)
	}

	DebuggingInfo(n)
}

// SetValue programmatically sets the toggle state.
func (n *Toggle) SetValue(value bool) {
	n.value = value
}

// GetValue returns the current toggle state.
func (n *Toggle) GetValue() bool {
	return n.value
}

// HasChanged reports whether the toggle state changed during the last [Update]
// call.
func (n *Toggle) HasChanged() bool {
	return n.hasChanged
}

// Toggle flips the current boolean state.
func (n *Toggle) Toggle() {
	n.value = !n.value
}

// DropdownProps is the configuration struct for [*Dropdown].
type DropdownProps struct {
	BoxProps
	FontConfigProps

	// OptionsCornerRadius rounds the corners of the options box.
	OptionsCornerRadius BoxCorners

	// ExpandIcon is the texture shown when the list is collapsed. Defaults to
	// [DefaultDropdownExpandIcon].
	ExpandIcon rl.Texture2D

	// CollapseIcon is the texture shown when the list is expanded. Defaults
	// to [DefaultDropdownCollapseIcon].
	CollapseIcon rl.Texture2D

	// IconSize is the width and height of the expand/collapse icon in pixels.
	// Defaults to [FontConfigProps.FontSize] * [DefaultDropdownIconSizeFactor].
	IconSize float32

	// IconGap is the horizontal gap between the icon and the label text.
	// Defaults to [BoxProps.Padding].Right or [DefaultDropdownIconGap].
	IconGap float32
}

// Dropdown is a single-select control that shows a list of string options.
// When closed it displays the selected option (or a placeholder); when open it
// expands downward to show all options. Clicking outside or selecting an
// option closes the list.
//
// Dropdown requires an ID. State is persisted in the node cache.
type Dropdown struct {
	Box
	ChildlessNode
	FontConfig

	OptionsCornerRadius BoxCorners
	expandIcon          rl.Texture2D
	collapseIcon        rl.Texture2D
	iconSize            float32
	iconGap             float32

	placeholder string
	options     []string
	selected    int16
	isOpen      bool
	hasChanged  bool

	isFocused bool

	minInnerSizeXCache float32
}

// NewDropdown creates (or retrieves from cache) a Dropdown node.
// placeholder is shown when no option is selected.
// options is the list of selectable strings; they cannot contain '\n'.
// selected is the zero-based index of the initially selected option, or -1 for
// no selection. Requires a non-zero ID in props.
func NewDropdown(props DropdownProps, placeholder string, options []string, selected int16) *Dropdown {
	assert.NotEqual(props.ID, NodeIDUnset, "Dropdown needs an ID")

	if node := GetNodeFromCache[*Dropdown](props.ID); node != nil {
		return node
	}

	for _, option := range options {
		assert.False(strings.ContainsRune(option, '\n'), "options cannot contain newlines ('\\n')")
	}

	if selected >= 0 {
		assert.Less(int(selected), len(options))
	}

	if placeholder == "" {
		placeholder = "Select"
	}
	node := &Dropdown{
		options:     options,
		placeholder: placeholder,
		selected:    selected,
	}
	node.ApplyProps(props)

	CacheNode(node)
	return node
}

// ApplyProps updates the dropdown configuration.
func (n *Dropdown) ApplyProps(props DropdownProps) {
	n.Box.ApplyProps(props.BoxProps)
	n.FontConfig.ApplyProps(props.FontConfigProps)

	n.OptionsCornerRadius = props.OptionsCornerRadius

	if rl.IsTextureValid(props.ExpandIcon) {
		n.expandIcon = props.ExpandIcon
	} else {
		n.expandIcon = DefaultDropdownExpandIcon
	}

	if rl.IsTextureValid(props.CollapseIcon) {
		n.collapseIcon = props.CollapseIcon
	} else {
		n.collapseIcon = DefaultDropdownCollapseIcon
	}

	setDefault(&n.iconSize, props.IconSize, n.fontSize*DefaultDropdownIconSizeFactor)
	if n.padding.Right != 0 {
		setDefault(&n.iconGap, props.IconGap, n.padding.Right)
	} else {
		setDefault(&n.iconGap, props.IconGap, DefaultDropdownIconGap)
	}
}

// ComputeMinInnerSizeX implements [Node.ComputeMinInnerSizeX].
// ComputeMinInnerSizeX returns the minimum width that can accommodate the
// widest option label plus the icons and gaps.
func (n *Dropdown) ComputeMinInnerSizeX() float32 {
	assert.NotEqual(n.size.mode.X, SizingFixed)

	if n.placeholder != "" && n.selected >= 0 {
		n.placeholder = ""
		n.minInnerSizeXCache = 0
	}

	if n.minInnerSizeXCache > 0 {
		return n.minInnerSizeXCache
	}

	widest := rl.MeasureTextEx(n.font, n.placeholder, n.fontSize, n.charSpacing).X
	for _, text := range n.options {
		textWidth := rl.MeasureTextEx(n.font, text, n.fontSize, n.charSpacing).X
		widest = max(widest, textWidth)
	}
	n.minInnerSizeXCache = n.iconSize + n.iconGap + widest + n.iconGap + n.iconSize + 1 // give a small error window
	return n.minInnerSizeXCache
}

// ComputeMinInnerSizeY implements [Node.ComputeMinInnerSizeY].
// ComputeMinInnerSizeY returns the minimum height needed to render one row
// (the taller of the text height and the icon height).
func (n *Dropdown) ComputeMinInnerSizeY() float32 {
	assert.NotEqual(n.size.mode.Y, SizingFixed)

	return max(n.fontSize-n.fontSize*n.baselineCorrectionFactor, n.iconSize)
}

// Update implements [Node.Update].
// Update handles opening/closing the dropdown and selecting an option.
// Clicking anywhere outside the dropdown while it is open closes it.
func (n *Dropdown) Update() {
	n.hasChanged = false

	if !IsNodeHovered(n) {
		if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
			n.Close()
		}
		return
	}
	SetMouseCursor(rl.MouseCursorPointingHand)

	if !n.isOpen {
		if IsMouseButtonPressedConsume(rl.MouseButtonLeft) {
			n.Open()
		}
		return
	}

	if !IsMouseButtonPressedConsume(rl.MouseLeftButton) {
		return
	}

	pos := n.AbsPos()

	relMousePos := rl.Vector2Subtract(rl.GetMousePosition(), pos)

	if relMousePos.Y > n.size.Y+DropdownOptionsBoxMarginTop {
		relMousePos.Y -= n.size.Y + DropdownOptionsBoxMarginTop

		selected := int16(relMousePos.Y / n.size.Y)
		assert.InRange(int(selected), 0, len(n.options))

		if selected != n.selected {
			n.hasChanged = true
		}
		n.selected = selected
	}

	n.Close()
}

// Render implements [Node.Render].
// Render draws the collapsed dropdown header (or the full expanded list when
// open). When open, rendering is deferred to the post-render pass via
// [AddPostRender] so the list appears on top of all other content.
func (n *Dropdown) Render() {
	if !IsNodeVisible(n) {
		return
	}

	if n.isOpen {
		AddPostRender(n.Render)
	}

	rect := n.Rect()

	DrawRectangle(rect, n.BorderWidth, n.CornerRadius, n.BorderColor, n.BgColor)

	textRect := Rect(
		rect.X+n.padding.Left+n.iconSize+n.iconGap,
		rect.Y+n.padding.Top,
		rect.Width-n.iconSize*2-n.iconGap*2-n.padding.X(),
		rect.Height-n.padding.Y(),
	)

	var firstElement string
	if n.selected < 0 {
		firstElement = n.placeholder
	} else {
		firstElement = n.options[n.selected]
	}
	DrawTextEllipsis(textRect, AlignCenter, AlignCenter, n.font, firstElement, n.fontSize, n.charSpacing, n.FgColor)

	iconRect := Rect(
		rect.X+rect.Width-n.padding.Right-n.iconSize,
		rect.Y+max(n.padding.Top, (rect.Height-n.iconSize)/2),
		n.iconSize,
		n.iconSize,
	)
	var iconTexture rl.Texture2D
	if n.isOpen {
		iconTexture = n.collapseIcon
	} else {
		iconTexture = n.expandIcon
	}
	rl.DrawTexturePro(iconTexture, Rect(0, 0, float32(iconTexture.Width), float32(iconTexture.Height)), iconRect, Vec2(0, 0), 0, n.FgColor)

	if n.isOpen {
		openRect := rect
		openRect.Y += rect.Height + DropdownOptionsBoxMarginTop
		openRect.Height *= float32(len(n.options))
		DrawRectangle(openRect, n.BorderWidth, n.OptionsCornerRadius, n.BorderColor, n.BgColor)

		separatorColor := n.BorderColor
		if separatorColor == (Color{}) {
			separatorColor = ColorContrast(n.BgColor, 0.4)
		}

		textRect.Y += DropdownOptionsBoxMarginTop

		for i, option := range n.options {
			textRect.Y += n.size.Y
			DrawTextEllipsis(textRect, AlignCenter, AlignCenter, n.font, option, n.fontSize, n.charSpacing, n.FgColor)

			if i == 0 {
				continue
			}

			const SeparatorMarginX = 10

			separatorY := float32(int(rect.Y+rect.Height*float32(i+1))) + DropdownOptionsBoxMarginTop
			rl.DrawLineEx(Vec2(rect.X+SeparatorMarginX, separatorY), Vec2(rect.X+rect.Width-SeparatorMarginX, separatorY), 1, separatorColor)
		}
	}

	DebuggingInfo(n)
}

// TotalArea implements [Node.TotalArea].
// TotalArea returns the full expanded rectangle when the dropdown is open,
// ensuring hover detection works over the list.
func (n *Dropdown) TotalArea() rl.Rectangle {
	rect := n.Rect()
	if n.isOpen {
		rect.Height = rect.Height*float32(len(n.options)+1) + DropdownOptionsBoxMarginTop
	}
	return rect
}

// Open expands the dropdown list and adds this node to the high-priority list
// so that hover detection works through the expanded area.
func (n *Dropdown) Open() {
	n.isOpen = true
	AddNodeToHighPriorityList(n)
}

// Close collapses the dropdown list and removes this node from the high-priority
// list.
func (n *Dropdown) Close() {
	n.isOpen = false
	RemoveNodeFromHighPriorityList(n)
}

// IsOpen reports whether the dropdown list is currently expanded.
func (n *Dropdown) IsOpen() bool {
	return n.isOpen
}

// HasChanged reports whether the selected option changed during the last
// [Update] call.
func (n *Dropdown) HasChanged() bool {
	return n.hasChanged
}

// GetSelectedIdx returns the zero-based index of the currently selected option,
// or -1 if no option is selected.
func (n *Dropdown) GetSelectedIdx() int {
	if len(n.options) == 0 || n.selected < 0 {
		n.selected = -1
	}
	assert.InRange(int(n.selected), -1, len(n.options)-1)
	return int(n.selected)
}

// GetSelected returns the string value of the currently selected option, or ""
// if no option is selected.
func (n *Dropdown) GetSelected() string {
	idx := n.GetSelectedIdx()
	if idx == -1 {
		return ""
	}
	return n.options[n.selected]
}

// HasSelection reports whether an option is currently selected (i.e.
// [GetSelectedIdx] != -1).
func (n *Dropdown) HasSelection() bool {
	return n.GetSelectedIdx() >= 0
}

// GetNumberOfOptions returns the total number of options in the dropdown.
func (n *Dropdown) GetNumberOfOptions() int {
	return len(n.options)
}

// ScrollBoxProps is the configuration struct for [*ScrollBox].
type ScrollBoxProps struct {
	BoxProps

	// ThumbWidth is the width (or height for a horizontal scroll bar) of the
	// scroll thumb in pixels. Defaults to [DefaultScrollBoxThumbWidth].
	ThumbWidth float32

	// ThumbColor is the fill color of the scroll thumb. Defaults to a
	// contrast color derived from BgColor.
	ThumbColor rl.Color

	// ThumbCornerRadius is the uniform corner radius of the scroll thumb.
	ThumbCornerRadius float32

	// HideScrollBar hides the scroll-bar thumb while still allowing scrolling.
	HideScrollBar bool

	// ScrollOrientation controls the scroll direction.
	ScrollOrientation Orientation
}

// ScrollBox is a container that clips its children to its visible area and
// allows scrolling via the mouse wheel or by dragging the scroll thumb.
//
// ScrollBox requires an ID. State is persisted in the node cache.
type ScrollBox struct {
	Box

	isDraggingTheThumb bool
	contentSize        float32
	scrollDistance     float32
	thumbWidth         float32

	// ThumbColor is the fill color of the scroll thumb.
	ThumbColor rl.Color

	// ThumbCornerRadius is the per-corner radius of the scroll thumb.
	ThumbCornerRadius BoxCorners

	hideSrollBar      bool
	scrollOrientation Orientation
}

// NewScrollBox creates (or retrieves from cache) a [*ScrollBox] node.
// Requires a non-zero ID in props.
func NewScrollBox(props ScrollBoxProps) *ScrollBox {
	assert.NotEqual(props.ID, NodeIDUnset, "ScrollBox needs an ID")

	if node := GetNodeFromCache[*ScrollBox](props.ID); node != nil {
		return node
	}

	node := new(ScrollBox)
	node.ApplyProps(props)

	CacheNode(node)
	return node
}

// ApplyProps updates the ScrollBox configuration.
func (n *ScrollBox) ApplyProps(props ScrollBoxProps) {
	n.Box.ApplyProps(props.BoxProps)

	assert.NoError(props.CornerRadius.validate())

	setDefault(&n.ThumbColor, props.ThumbColor, ColorContrast(props.BgColor, 0.2))
	setDefault(&n.thumbWidth, props.ThumbWidth, DefaultScrollBoxThumbWidth)
	n.ThumbCornerRadius = Radius(props.ThumbCornerRadius)
	n.hideSrollBar = props.HideScrollBar
	n.scrollOrientation = props.ScrollOrientation
}

func (n *ScrollBox) scrollBox() *ScrollBox {
	return n
}

func (n *ScrollBox) calcThumbLength() float32 {
	switch n.scrollOrientation {
	case OrientationHorizontal:
		return n.size.X * n.size.X / n.contentSize
	case OrientationVertical:
		return n.size.Y * n.size.Y / n.contentSize
	}

	assert.Unreachable()
	return 0
}

// Update implements [Node.Update].
// Update handles mouse wheel scrolling and scroll-thumb dragging.
// When content fits in the visible area, scrollDistance is reset to 0.
func (n *ScrollBox) Update() {
	if !IsNodeHovered(n) && !n.isDraggingTheThumb {
		return
	}

	var boxLength float32
	if n.scrollOrientation == OrientationHorizontal {
		boxLength = n.size.X
	} else {
		boxLength = n.size.Y
	}

	if n.contentSize <= boxLength {
		n.scrollDistance = 0
		return
	}

	wheelDelta := rl.GetMouseWheelMove()

	n.scrollDistance += -wheelDelta * ScrollBoxSpeed
	n.scrollDistance = Clamp(n.scrollDistance, 0, n.contentSize-boxLength)

	if n.isDraggingTheThumb && IsMouseButtonReleased(rl.MouseLeftButton) {
		n.isDraggingTheThumb = false
		return
	}

	if !rl.IsMouseButtonDown(rl.MouseLeftButton) && !rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		return
	}

	rect := n.Rect()

	const MouseExtendedDistance = 5

	trackWidth := ScrollBoxThumbMargin*2 + n.thumbWidth + MouseExtendedDistance*2
	thumbLength := n.calcThumbLength()

	var trackRect rl.Rectangle
	if n.scrollOrientation == OrientationHorizontal {
		trackRect = Rect(
			rect.X,
			rect.Y+rect.Height-trackWidth,
			rect.Width,
			trackWidth,
		)
	} else {
		trackRect = Rect(
			rect.X+rect.Width-trackWidth,
			rect.Y,
			trackWidth,
			rect.Height,
		)
	}

	mousePos := rl.GetMousePosition()

	if !n.isDraggingTheThumb && IsMouseButtonPressed(rl.MouseLeftButton) && rl.CheckCollisionPointRec(mousePos, trackRect) {
		n.isDraggingTheThumb = true
	}

	if !n.isDraggingTheThumb {
		return
	}

	var mouseAlongTheTrackPercentage float32 // with a padding of thumbLength/2
	if n.scrollOrientation == OrientationHorizontal {
		mouseAlongTheTrackPercentage = (mousePos.X - trackRect.X - thumbLength/2) / (n.size.X - thumbLength)
	} else {
		mouseAlongTheTrackPercentage = (mousePos.Y - trackRect.Y - thumbLength/2) / (n.size.Y - thumbLength)
	}

	mouseAlongTheTrackPercentage = Clamp(mouseAlongTheTrackPercentage, 0, 1)
	n.scrollDistance = (n.contentSize - boxLength) * mouseAlongTheTrackPercentage
}

// Render implements [Node.Render].
// Render draws the scroll box content within a scissor clip and then overlays
// the scroll-bar thumb (unless HideScrollBar is set or the content fits).
func (n *ScrollBox) Render() {
	if !IsNodeVisible(n) {
		return
	}

	rect := n.Rect()

	BeginScissorMode(rect)
	n.Box.Render()
	EndScissorMode()

	if !n.hideSrollBar {
		thumbLength := n.calcThumbLength()
		var thumbRect rl.Rectangle

		switch n.scrollOrientation {
		case OrientationHorizontal:
			if n.contentSize <= n.size.X {
				break
			}
			scrollTrackWidth := n.size.X - ScrollBoxThumbMargin*2
			scrollProgress := n.scrollDistance / (n.contentSize - n.size.X)
			thumbRect = Rect(
				rect.X+ScrollBoxThumbMargin+(scrollTrackWidth-thumbLength)*scrollProgress,
				rect.Y+rect.Height-n.thumbWidth-ScrollBoxThumbMargin,
				thumbLength,
				n.thumbWidth,
			)

		case OrientationVertical:
			if n.contentSize <= n.size.Y {
				break
			}
			scrollTrackHeight := n.size.Y - ScrollBoxThumbMargin*2
			scrollProgress := n.scrollDistance / (n.contentSize - n.size.Y)
			thumbRect = Rect(
				rect.X+rect.Width-n.thumbWidth-ScrollBoxThumbMargin,
				rect.Y+ScrollBoxThumbMargin+(scrollTrackHeight-thumbLength)*scrollProgress,
				n.thumbWidth,
				thumbLength,
			)

		default:
			assert.Unreachable()
		}

		if n.ThumbCornerRadius.TopLeft > 0 {
			DrawRectangle(thumbRect, BoxSides{}, n.ThumbCornerRadius, rl.Blank, n.ThumbColor)
		} else {
			rl.DrawRectangleRec(thumbRect, n.ThumbColor)
		}
	}

	DebuggingInfo(n)
}

// SliderProps is the configuration struct for [*Slider].
type SliderProps struct {
	BoxProps

	// ThumbWidth is the pixel size of the circular thumb (diameter).
	// Defaults to [DefaultSliderThumbWidth].
	ThumbWidth float32

	// ThumbCornerRadius is the corner radius of the thumb rectangle.
	ThumbCornerRadius BoxCorners

	// ThumbColor is the fill color of the thumb. Defaults to rl.RayWhite.
	ThumbColor rl.Color

	// TrackWidth is the pixel thickness of the slider track. Defaults to
	// [DefaultSliderTrackWidth].
	TrackWidth float32

	// TrackCornerRadius is the corner radius of the track rectangles.
	TrackCornerRadius BoxCorners

	// TrackActiveColor is the color of the portion of the track between the
	// start and the thumb. Defaults to rl.LightGray.
	TrackActiveColor rl.Color

	// TrackInactiveColor is the color of the track portion after the thumb.
	// Defaults to rl.DarkGray.
	TrackInactiveColor rl.Color

	// Step is the quantisation step size. When non-zero the value snaps to
	// multiples of Step. A zero Step means continuous.
	Step float32

	// Orientation is the direction of the slider.
	Orientation Orientation
}

// Slider is a numeric range input that lets the user drag a thumb along a
// track to pick a value between min and max.
//
// Slider requires an ID. State is persisted in the node cache.
type Slider struct {
	Box
	ChildlessNode

	min  float32
	max  float32
	step float32

	progress float32

	thumbWidth float32

	// ThumbCornerRadius is the per-corner radius of the slider thumb.
	ThumbCornerRadius BoxCorners

	// ThumbColor is the fill color of the slider thumb.
	ThumbColor rl.Color

	trackWidth float32

	// TrackCornerRadius is the per-corner radius of the active/inactive track
	// segments.
	TrackCornerRadius BoxCorners

	// TrackActiveColor is the color of the active (filled) portion of the
	// track.
	TrackActiveColor rl.Color

	// TrackInactiveColor is the color of the inactive (empty) portion of the
	// track.
	TrackInactiveColor rl.Color

	orientation   Orientation
	isExponential bool
	isDragging    bool
	hasChanged    bool
}

// NewSlider creates (or retrieves from cache) a Slider node.
// min and max define the value range; value is the initial position.
// Requires a non-zero ID in props.
func NewSlider(props SliderProps, min, max, value float32) *Slider {
	assert.NotEqual(props.ID, NodeIDUnset, "Slider needs an ID")

	if node := GetNodeFromCache[*Slider](props.ID); node != nil {
		return node
	}

	if props.Step != 0 {
		value = float32(int(value/props.Step)) * props.Step
	}
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}

	node := &Slider{
		min:      min,
		max:      max,
		progress: value / (max - min),
	}
	node.ApplyProps(props)

	CacheNode(node)
	return node
}

// ApplyProps updates the Slider properties.
func (n *Slider) ApplyProps(props SliderProps) {
	n.Box.ApplyProps(props.BoxProps)

	n.step = props.Step
	n.orientation = props.Orientation

	setDefault(&n.thumbWidth, props.ThumbWidth, DefaultSliderThumbWidth)
	setDefault(&n.trackWidth, props.TrackWidth, DefaultSliderTrackWidth)
	setDefault(&n.ThumbColor, props.ThumbColor, rl.RayWhite)
	n.ThumbCornerRadius = props.ThumbCornerRadius

	setDefault(&n.TrackInactiveColor, props.TrackInactiveColor, rl.DarkGray)
	setDefault(&n.TrackActiveColor, props.TrackActiveColor, rl.LightGray)
	n.TrackCornerRadius = props.TrackCornerRadius
}

// ComputeMinInnerSizeX implements [Node.ComputeMinInnerSizeX].
// ComputeMinInnerSizeX returns the minimum horizontal size required. For a
// vertical slider this is the larger of thumb and track widths; for a
// horizontal slider it is 0.
func (n *Slider) ComputeMinInnerSizeX() float32 {
	if n.orientation == OrientationVertical {
		return max(n.thumbWidth, n.trackWidth)
	}
	return 0
}

// ComputeMinInnerSizeY implements [Node.ComputeMinInnerSizeY].
// ComputeMinInnerSizeY returns the minimum vertical size required. For a
// horizontal slider this is the larger of thumb and track widths; for a
// vertical slider it is 0.
func (n *Slider) ComputeMinInnerSizeY() float32 {
	if n.orientation == OrientationHorizontal {
		return max(n.thumbWidth, n.trackWidth)
	}
	return 0
}

// Update implements [Node.Update].
// Update processes mouse input to drag the thumb. While dragging, the cursor
// is changed to a pointing hand.
func (n *Slider) Update() {
	n.hasChanged = false

	if !n.isDragging && !IsNodeHovered(n) {
		return
	}

	if IsMouseButtonReleasedConsume(rl.MouseButtonLeft) {
		n.isDragging = false
		n.hasChanged = true
		return
	}

	innerRect := n.InnerRect()

	const TrackMargin = 5

	var trackMouseRect rl.Rectangle
	var trackRect rl.Rectangle
	var trackUsedSize float32

	switch n.orientation {
	case OrientationHorizontal:
		trackUsedSize = innerRect.Width - n.thumbWidth

		trackMouseRect = Rect(
			innerRect.X+n.thumbWidth/2,
			innerRect.Y+(innerRect.Height-n.trackWidth)/2-TrackMargin,
			trackUsedSize,
			n.trackWidth+TrackMargin*2,
		)

		trackRect = trackMouseRect
		trackRect.X -= n.thumbWidth / 2
		trackRect.Width += n.thumbWidth

	case OrientationVertical:
		trackUsedSize = innerRect.Height - n.thumbWidth

		trackMouseRect = Rect(
			innerRect.X+(innerRect.Width-n.trackWidth)/2-TrackMargin,
			innerRect.Y+n.thumbWidth/2,
			n.trackWidth+TrackMargin*2,
			trackUsedSize,
		)

		trackRect = trackMouseRect
		trackRect.Y -= n.thumbWidth / 2
		trackRect.Height += n.thumbWidth

	default:
		assert.Unreachable()
	}

	mousePos := rl.GetMousePosition()

	if !n.isDragging && !rl.CheckCollisionPointRec(mousePos, trackRect) {
		return
	}
	SetMouseCursor(rl.MouseCursorPointingHand)

	if IsMouseButtonPressedConsume(rl.MouseButtonLeft) {
		n.isDragging = true
	}

	if !n.isDragging {
		return
	}

	switch n.orientation {
	case OrientationHorizontal:
		n.progress = Clamp((mousePos.X-trackMouseRect.X)/trackUsedSize, 0, 1)

	case OrientationVertical:
		n.progress = 1 - Clamp((mousePos.Y-trackMouseRect.Y)/trackUsedSize, 0, 1)
	}

	if n.step != 0 {
		progressStep := 1 / ((n.max - n.min) / n.step)
		n.progress = float32(int(n.progress/progressStep)) * progressStep
	}
}

// Render implements [Node.Render].
// Render draws the active track segment, the inactive track segment, and the
// thumb.
func (n *Slider) Render() {
	if !IsNodeVisible(n) {
		return
	}

	innerRect := n.InnerRect()

	var trackUsedSize float32
	var trackActiveRect, trackInactiveRect rl.Rectangle
	var trackActiveCornerRadius, trackInactiveCornerRadius BoxCorners
	var thumbRect rl.Rectangle

	switch n.orientation {
	case OrientationHorizontal:
		trackUsedSize = innerRect.Width - n.thumbWidth

		trackActiveRect = Rect(
			innerRect.X,
			innerRect.Y+(innerRect.Height-n.trackWidth)/2,
			n.thumbWidth/2+trackUsedSize*n.progress,
			n.trackWidth,
		)
		trackInactiveRect = Rect(
			trackActiveRect.X+trackActiveRect.Width,
			innerRect.Y+(innerRect.Height-n.trackWidth)/2,
			n.thumbWidth/2+trackUsedSize*(1-n.progress),
			n.trackWidth,
		)

		trackActiveCornerRadius = n.TrackCornerRadius
		trackActiveCornerRadius.TopRight = 0
		trackActiveCornerRadius.BottomRight = 0

		trackInactiveCornerRadius = n.TrackCornerRadius
		trackInactiveCornerRadius.TopLeft = 0
		trackInactiveCornerRadius.BottomLeft = 0

		thumbRect = Rect(
			trackInactiveRect.X-n.thumbWidth/2,
			innerRect.Y+(innerRect.Height-n.thumbWidth)/2,
			n.thumbWidth,
			n.thumbWidth,
		)

	case OrientationVertical:
		trackUsedSize = innerRect.Height - n.thumbWidth

		trackInactiveRect = Rect(
			innerRect.X+(innerRect.Width-n.trackWidth)/2,
			innerRect.Y,
			n.trackWidth,
			n.thumbWidth/2+trackUsedSize*(1-n.progress),
		)
		trackActiveRect = Rect(
			innerRect.X+(innerRect.Width-n.trackWidth)/2,
			trackInactiveRect.Y+trackInactiveRect.Height,
			n.trackWidth,
			n.thumbWidth/2+trackUsedSize*n.progress,
		)

		trackActiveCornerRadius = n.TrackCornerRadius
		trackActiveCornerRadius.TopLeft = 0
		trackActiveCornerRadius.TopRight = 0

		trackInactiveCornerRadius = n.TrackCornerRadius
		trackInactiveCornerRadius.BottomLeft = 0
		trackInactiveCornerRadius.BottomRight = 0

		thumbRect = Rect(
			innerRect.X+(innerRect.Width-n.thumbWidth)/2,
			trackActiveRect.Y-n.thumbWidth/2,
			n.thumbWidth,
			n.thumbWidth,
		)

	default:
		assert.Unreachable()
	}

	DrawRectangle(trackActiveRect, BoxSides{}, trackActiveCornerRadius, Color{}, n.TrackActiveColor)
	DrawRectangle(trackInactiveRect, BoxSides{}, trackInactiveCornerRadius, Color{}, n.TrackInactiveColor)
	DrawRectangle(thumbRect, BoxSides{}, n.ThumbCornerRadius, Color{}, n.ThumbColor)

	DebuggingInfo(n)
}

// IsChanging reports whether the user is currently dragging the thumb.
// Use this to apply real-time updates (e.g. live preview) while the drag is in
// progress.
func (n *Slider) IsChanging() bool {
	return n.isDragging
}

// HasChanged reports whether the drag ended this frame (mouse button released).
// Use this to trigger actions that should only fire once per interaction, not
// on every drag frame.
func (n *Slider) HasChanged() bool {
	return n.hasChanged
}

// GetValue returns the current slider value in the [min, max] range, snapped
// to the nearest [SliderProps.Step] if one was specified.
func (n *Slider) GetValue() float32 {
	assert.InRange(n.progress, 0, 1)

	value := n.min + (n.max-n.min)*n.progress
	if n.step != 0 {
		value = float32(int(value/n.step)) * n.step
	}
	return value
}

// SetValue moves the thumb to position the slider at value. value is clamped
// to [min, max].
func (n *Slider) SetValue(value float32) {
	n.progress = Clamp(value, n.min, n.max) / (n.max - n.min)
}

// GetProgress returns the current position of the thumb as a normalised value
// in [0, 1].
func (n *Slider) GetProgress() float32 {
	assert.InRange(n.progress, 0, 1)
	return n.progress
}

// SetProgress moves the thumb to a normalised position. progress must be in
// [0, 1].
func (n *Slider) SetProgress(progress float32) {
	assert.InRange(progress, 0, 1)
	n.progress = progress
}
