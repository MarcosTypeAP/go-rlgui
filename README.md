# go-rlgui

<a href="https://pkg.go.dev/github.com/MarcosTypeAP/go-rlgui"><img src="https://pkg.go.dev/badge/github.com/MarcosTypeAP/go-rlgui.svg" alt="Go Reference"></a>

go-rlgui is a box-model UI library for [raylib-go](https://github.com/gen2brain/raylib-go) that sits between immediate-mode and retained-mode: the node tree is rebuilt from scratch every frame, so your UI code reads top-to-bottom like an immediate-mode library, but widget state (scroll offsets, slider positions, text input contents, dropdown selections) is persisted across frames in a cache keyed by stable node IDs.
 
> A real world example using this library can be found [here](https://github.com/MarcosTypeAP/soundpad).

---

## Installation

```sh
go get github.com/MarcosTypeAP/go-rlgui
```

> **Requirements:** Go 1.24+ and a working [raylib-go](https://github.com/gen2brain/raylib-go) setup.

---

## Quick start

```go
package main

import (
    gui "github.com/MarcosTypeAP/go-rlgui"
    rl  "github.com/gen2brain/raylib-go/raylib"
)

func main() {
	rl.SetConfigFlags(rl.FlagWindowResizable | rl.FlagMsaa4xHint)

    gui.InitWindow(gui.Vec2(800, 600), "My App")
    defer gui.CloseWindow()

    rl.SetTargetFPS(60)

    // Every [SubWindow] needs a root. [ZIndexRoot] pins it behind floating windows.
    sw := gui.AddSubWindow(gui.NewSubWindow(gui.SubWindowProps{
        SizingX: gui.Grow(),
        SizingY: gui.Grow(),
        ZIndex:  gui.ZIndexRoot,
    }), gui.Vec2(0, 0))

    for !rl.WindowShouldClose() {
        // 1. Clear the previous tree
        gui.ResetLayout()

        // 2. Rebuild the tree
        root := sw.SetRoot(gui.NewBox(gui.BoxProps{
            SizingX:     gui.Grow(),
            SizingY:     gui.Grow(),
            Padding:     gui.Padding(20),
            BgColor:     gui.ColorHex(0x1A1F25FF),
            Orientation: gui.Vertical,
            ChildGap:    12,
        }))

        btn := gui.AddChild(root, gui.NewButton(gui.ButtonProps{
            BoxProps: gui.BoxProps{
                Padding:      gui.Padding(10, 20),
                BgColor:      rl.DarkBlue,
                CornerRadius: gui.Radius(8),
                ChildAlignX:  gui.Center,
            },
            OnHover: gui.EffectBrighten,
        }))
        gui.AddChild(btn, gui.NewText(gui.TextProps{}, "Click me"))

        // 3. Resolve all sizes and positions
        gui.ComputeLayout()

        // 4. Process input and widget state
        gui.Update()       

        // 5. React to input after Update,
        //    or add a callback with [gui.AddPostUpdate].
        if btn.IsLeftButtonPressed() {
            // handle click
        }

        // 6. Draw everything
        rl.BeginDrawing()
        gui.Render()
        rl.EndDrawing()
    }
}
```

---

## Frame loop

Every frame must follow this order:

| Step |   |
|------|---|
| 1 | Call `gui.ResetLayout()` to clear the node tree (does **not** wipe widget state caches). |
| 2 | Build the tree calling `gui.AddChild(...)` to the root (`SubWindow.Root()` / `SubWindow.SetRoot(...)`) to describe the UI. |
| 3 | Call `gui.ComputeLayout()` to resolve all sizes and absolute positions. |
| 4 | Call `gui.Update()` to process input, update nodes, run `gui.AddPostUpdate` callbacks. |
| 5 | React to input reading node state (`Button.IsLeftButtonPressed()`, `Slider.GetValue()`, …). |
| 6 | Call `gui.Render()` to draw the tree; must be called between `rl.BeginDrawing` / `rl.EndDrawing`. |

Post-update logic can be registered with `gui.AddPostUpdate(func() { ... })` during tree construction so it runs after `gui.Update()`.

---

## Sizing

Every axis of every box is sized independently via a `SizingProp`. Short aliases from `aliases.go` are shown alongside the full constructors.

| Alias | Constructor | Behaviour |
|-------|-------------|-----------|
| `Shrink(min?, max?)` | `NewSizingShrink` | Shrinks to fit content. Optional min/max pixel constraints. |
| `Grow(min?, max?)` | `NewSizingGrow` | Expands to fill remaining space in the parent. Optional min/max pixel constraints. |
| `Fixed(px)` | `NewSizingFixed` | Locked to an exact pixel size. |
| `Percentage(pct, min?, max?)` | `NewSizingPercentage` | Fraction of the parent's size on the same axis (0–100). Optional min/max pixel constraints. |
| `AspectRatio(ratio)` | `NewSizingAspectRatio` | Y axis only — height = width / ratio. |

```go
// A box that fills 40 % of its parent's width and grows vertically
gui.BoxProps{
    SizingX: gui.Percentage(40),
    SizingY: gui.Grow(),
}
```

---

## Styling helpers

| Constructor | Alias | Usage |
|-------|-------------|-------|
| `NewBoxSides(v…)` | `Padding` / `Border` | Accepts between 1 and 4 values, following CSS padding shorthand conventions. |
| `NewBoxSidesOverride(base, top, right, bottom, left)` | `PaddingOverride` / `BorderOverride` | Start from an existing `BoxSides` and override specific sides. |
| `NewBoxCorners(v…)` | `Radius` | Accepts between 1 and 4 values, following CSS border-radius shorthand conventions. |
| `NewBoxCornersOverride(base, topLeft, topRight, bottomRight, bottomLeft)` | `RadiusOverride` | Start from an existing `BoxCorners` and override specific corners. |

---

## Built-in widgets

### Box

It's the fundamental building block of the UI and the base for any node.
It is a rectangular container that can hold children, display a background color, gradient, or texture, and draw a bordered rounded rectangle.

```go
gui.NewBox(gui.BoxProps{
    SizingX:     gui.Grow(),
    SizingY:     gui.Fixed(60),
    Padding:     gui.Padding(10),
    BgColor:     gui.ColorHex(0x2F3439FF),
    CornerRadius: gui.Radius(10),
    Orientation: gui.Vertical, // gui.Horizontal or gui.Vertical
    ChildGap:    8,
    ChildAlignX: gui.Center, // gui.Start | gui.Center | gui.End
    ChildAlignY: gui.Start,
    HideOverflow: true,
})
```

### BoxImage

Displays a texture loaded from an `fs.FS` or the filesystem.

```go
//go:embed icon.png
var assetsFS embed.FS

gui.AddChild(parent, gui.NewBoxImage(gui.BoxProps{
    SizingX:     gui.Fixed(32),
    SizingY:     gui.AspectRatio(1),
}, "icon.png", assetsFS))
```

### Spacer

It's a helper `Box` constructor that creates a horizontal/vertical spacer.
An optional `gui.SizingProp` controls how much space it occupies; defaults to `gui.NewSizingGrow()`.

```go
gui.AddChild(parent, gui.NewBox( /* … */ ))
gui.AddChild(parent, gui.NewSpacerX()) // Similar effect to CSS "justify-content: space-between";
gui.AddChild(parent, gui.NewBox( /* … */ ))
```

### Text

It's a node that renders a string of text.

```go
gui.NewText(gui.TextProps{
    BoxProps: gui.BoxProps{SizingX: gui.Grow()},
    FontConfigProps: gui.FontConfigProps{
        FgColor:  rl.White,
        FontSize: 24,
    },
    Wrapping: gui.Wrap, // gui.NoWrap | gui.Wrap | gui.EllipsisOverflow
}, "Hello, world!")
```

### Button

It's a clickable container node.
It reports press and release events for both mouse buttons and applies optional visual effects on hover and press.
Children are added with `gui.AddChild(...)` to build the button's visual content (icons, labels, etc.).

```go
btn := gui.AddChild(parent, gui.NewButton(gui.ButtonProps{
    BoxProps: gui.BoxProps{ /* … */ },
    OnHover: gui.EffectBrighten,
    OnPress: func(box *gui.Box) func() {
        box.BgColor = rl.DarkGray
        return nil // optional cleanup callback, called when released
    },
}))

// After gui.Update() or inside a gui.AddPostUpdate(...) callback:
if btn.IsLeftButtonPressed() { /* … */ }
```

### ScrollBox

It's a container that clips its children to its visible area and allows scrolling via the mouse wheel or by dragging the scroll thumb.

```go
scroll := gui.AddChild(parent, gui.NewScrollBox(gui.ScrollBoxProps{
    BoxProps: gui.BoxProps{
        ID:          sw.GetAutoID(),
        SizingX:     gui.Grow(),
        SizingY:     gui.Grow(),
        Orientation: gui.Vertical,
        ChildGap:    10,
    },
    ScrollOrientation: gui.Vertical,
    ThumbWidth:        10,
    ThumbColor:        gui.ColorHex(0xFFFFFFAA),
    ThumbCornerRadius: 10,
}))
```

> **StatefulWidget note:** `ScrollBox`, `Slider`, `Toggle`, `CheckBox`, `TextInput`, and `Dropdown` all require a stable `NodeID` field so their internal state survives `gui.ResetLayout`. Use `SubWindow.GetAutoID()` for automatic sequential IDs, or assign an ID manually by calling `gui.NewID(...)`.

### Slider

It's a numeric range input that lets the user drag a thumb along a track to pick a value between min and max.

```go
slider := gui.AddChild(parent, gui.NewSlider(gui.SliderProps{
    BoxProps: gui.BoxProps{
        ID:      sw.GetAutoID(),
        SizingX: gui.Grow(),
        SizingY: gui.Fixed(30),
    },
    TrackActiveColor:  rl.Blue,
    TrackCornerRadius: gui.Radius(420),
    ThumbCornerRadius: gui.Radius(69),
}, 0 /*min*/, 100 /*max*/, 50 /*initial*/))

// After gui.Update() or inside a gui.AddPostUpdate(...) callback:
if slider.IsChanging() {
    value := slider.GetValue()
}
```

### Toggle / CheckBox

It's a boolean switch widget.
It renders either as a sliding pill (when created with `gui.NewToggle(...)`) or as a filled square/circle checkbox (when created with `gui.NewCheckBox(...)`).

```go
toggle := gui.AddChild(parent, gui.NewToggle(gui.ToggleProps{
    BoxProps: gui.BoxProps{
        ID:           sw.GetAutoID(),
        SizingX:      gui.Fixed(40),
        SizingY:      gui.Fixed(20),
        CornerRadius: gui.Radius(69),
    },
    OnColor:  rl.White,
    OffColor: rl.Gray,
}, false /*initial*/))

// gui.NewCheckBox uses the same ToggleProps signature.
checkbox := gui.AddChild(parent, gui.NewCheckBox(gui.ToggleProps{ /* … */ }, false))

// After gui.Update() or inside a gui.AddPostUpdate(...) callback:
isOn := toggle.GetValue()
```

### Dropdown

It's a single-select control that shows a list of string options.
When closed it displays the selected option (or a placeholder); when open it expands downward to show all options.
Clicking outside or selecting an option closes the list.

```go
dd := gui.AddChild(parent, gui.NewDropdown(gui.DropdownProps{
    BoxProps: gui.BoxProps{
        ID:           sw.GetAutoID(),
        Padding:      gui.Padding(10, 20),
        CornerRadius: gui.Radius(10),
        BgColor:      gui.ColorHex(0x1A1F25FF),
        BorderWidth:  gui.Border(2),
        BorderColor:  rl.White,
    },
}, "Select…", []string{"Option A", "Option B", "Option C"}, -1 /*initial index, -1 = none*/))

// After gui.Update() or inside a gui.AddPostUpdate(...) callback:
if dd.HasChanged() {
    idx := dd.GetSelectedIdx()
}
```

### TextInput

It's a single-line editable text field.
It supports a placeholder string shown when the field is empty, an optional error message displayed below the field, keyboard navigation (arrows, backspace, delete, Ctrl+V), mouse navigation (click to move cursor), and a blinking text cursor.

```go
input := gui.AddChild(parent, gui.NewTextInput(gui.TextInputProps{
    BoxProps: gui.BoxProps{
        ID:           sw.GetAutoID(),
        SizingX:      gui.Grow(),
        BgColor:      rl.DarkGray,
        Padding:      gui.Padding(10, 15),
        CornerRadius: gui.Radius(8),
    },
    PlaceholderColor: rl.Gray,
}, "Placeholder", "Initial value"))

// After gui.Update() or inside a gui.AddPostUpdate(...) callback:
text := input.Value()
```

---

## Gradients

```go
// Linear gradient at 45 degrees
gui.BoxProps{
    Gradient: gui.GradientLinear(rl.Red, rl.Blue, 45),
}
```

`GradientRadial` is declared but not yet implemented (it's just a placeholder for a (perhaps) future implementation).

---

## Sub-windows

Sub-windows are independent layout roots. Floating sub-windows can be dragged and optionally closed.

```go
// Non-floating root window
root := gui.AddSubWindow(gui.NewSubWindow(gui.SubWindowProps{
    SizingX: gui.Grow(),
    SizingY: gui.Grow(),
    ZIndex:  gui.ZIndexRoot,
}), gui.Vec2(0, 0))

// Floating, closable window
floating := gui.AddSubWindow(gui.NewSubWindow(gui.SubWindowProps{
    Floating: true,
    Closable: true,
}), gui.Vec2(200, 150))

// Ephemeral window (hidden as soon as loses focus)
floating := gui.AddSubWindow(gui.NewSubWindow(gui.SubWindowProps{
    Hidden: true,
    ZIndex: gui.ZIndexEphemeral,
}), gui.Vec2(100, 50))
```

---

## Custom nodes

Implement the `Node` interface to create fully custom widgets. Embed `gui.Box`, call `gui.CacheNode` / `gui.GetNodeFromCache` for state persistence, and call `gui.DebuggingInfo()` at the end of `gui.Render()` to participate in the debug overlay.

```go
type MyWidget struct {
    gui.Box
    // your state fields
}

func NewMyWidget(props gui.BoxProps) *MyWidget {
    if node := gui.GetNodeFromCache[*MyWidget](props.ID); node != nil {
        return node
    }

    n := &MyWidget{}
    n.ApplyProps(props)

    gui.CacheNode(n)
    return n
}

func (n *MyWidget) Update() {
    if !gui.IsNodeHovered(n) {
        return
    }
    // handle input
}

func (n *MyWidget) Render() {
    rect := n.Rect()
    rl.DrawRectangleRec(rect, rl.Purple)

    gui.DebuggingInfo(n) // participates in debug overlay
}
```

See `cmd/example/example.go` for a complete `PaintNode` implementation that draws on a `rl.RenderTexture2D`.

---

## Global defaults

These package-level variables can be set to tune the library's default behavior (see [gui.go](gui.go)).

E.g.: `gui.DefaultTextColor`, `gui.DefaultFontSize`, `gui.DefaultCharSpacing`, `gui.ScrollBoxSpeed`, `gui.FontTextureFilter`, ...

---

## Utility functions

| Function | Description |
|----------|-------------|
| `Clamp(v, min, max)` | Generic numeric clamp. |
| `Abs(v)` | Generic absolute value. |
| `Ternary(cond, a, b)` | Inline conditional; both branches evaluated eagerly. |
| `TernaryLazy(cond, a, b)` | Same, but branches are `func() T` — only the chosen one is called. |
| `Must(v, err)` | Unwraps `(T, error)`, panics on error. |
| `Must2(v1, v2, err)` | Unwraps `(T1, T2, error)`, panics on error. |
| `Vec2(x, y)` | Short alias for `rl.NewVector2`. |
| `Rect(x, y, w, h)` | Short alias for `rl.NewRectangle`. |
| `RectPosition(rect)` | Returns the top-left corner as `rl.Vector2`. |
| `RectSize(rect)` | Returns the width and height as `rl.Vector2`. |
| `ColorHex(0xRRGGBBAA)` | Short alias for `rl.GetColor`. |
| `ColorContrast(color, factor)` | Returns a brightened or darkened version of color. |
| `ColorToUniformVec4(color)` | Returns a `[]float32` with each channel normalised to `[0, 1]`. |

---

## Custom drawing functions
 
The library exposes several drawing helpers that use the same internal rounded-rect shader as the built-in widgets.
 
### DrawRectangle
 
Draws a filled rectangle with optional per-corner radius, per-side border width, and a solid fill color.
 
```go
gui.DrawRectangle(
    gui.Rect(x, y, w, h),
    gui.Border(2),           // per-side border widths
    gui.Radius(10),          // per-corner radii
    rl.White,                // border color
    gui.ColorHex(0x2F3439FF), // fill color
)
```
 
### DrawRectangleWithGradient
 
Same as `DrawRectangle` but fills with a `Gradient` instead of a solid color.
 
```go
gui.DrawRectangleWithGradient(
    gui.Rect(x, y, w, h),
    gui.Border(0),
    gui.Radius(12),
    rl.Blank,
    gui.GradientLinear(rl.Red, rl.Blue, 90),
)
```
 
### DrawRectangleWithTexture
 
Same as `DrawRectangle` but fills with a texture. The `textureFit` parameter controls how the texture is scaled inside the rectangle.
 
```go
gui.DrawRectangleWithTexture(
    gui.Rect(x, y, w, h),
    gui.Border(0),
    gui.Radius(8),
    rl.Blank,
    myTexture, rl.White,
    gui.FitCover,   // or gui.FitContain
)
```
 
### DrawTextEllipsis
 
Draws text inside a bounding rectangle. If the text is wider than the bounds it is truncated and a `…` suffix is appended. Supports horizontal and vertical alignment.
 
```go
gui.DrawTextEllipsis(
    gui.Rect(x, y, w, h),
    gui.Center, gui.Center,  // alignX, alignY
    gui.GetDefaultFont(),
    "Some long label that might overflow",
    20, 1, rl.White,         // fontSize, charSpacing, color
)
```
 
### WrapText
 
Re-flows a string so no line exceeds the width of `bounds`. Words are split at spaces and tabs; existing newlines are preserved. Returns the wrapped string with `\n` separators.
 
```go
wrapped := gui.WrapText(
    gui.Rect(0, 0, 300, 1000),
    gui.GetDefaultFont(),
    "A long paragraph that needs wrapping",
    20, 1,
)
```
 
### Scissor clipping
 
`BeginScissorMode` / `EndScissorMode` are nestable wrappers around raylib's scissor API. Each call intersects the new region with the current top-of-stack region, so nested clipping works correctly.
 
```go
func (n *MyWidget) Render() {
    rect := n.Rect()
    gui.BeginScissorMode(rect)
    defer gui.EndScissorMode()
 
    // anything drawn here is clipped to rect
    rl.DrawCircleV(somePos, 20, rl.Red)
}
```
 
### Scheduling draw calls
 
Use `AddPostRender` to schedule a draw callback that runs after all sub-windows and nodes have been rendered — useful for top-level overlays or debug visualizations that must always appear on top.
 
```go
gui.AddPostRender(func() {
    rl.DrawText("FPS: "+strconv.Itoa(int(rl.GetFPS())), 10, 10, 20, rl.Green)
})
```

---

## Examples

### Showcase

An example showcase lives in [cmd/example/example.go](cmd/example/example.go). It demonstrates gradients, a scrollable button grid, a paint canvas (custom node), text input, toggles, checkboxes, a dropdown, sliders, and spawnable floating windows.

```sh
go run ./cmd/example
```

Press **Q** to quit.

### Soundpad

A real world example using this library can be found [here](https://github.com/MarcosTypeAP/soundpad).

---

## Licenses

See [LICENSE](LICENSE) and [THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES).
