package main

import (
	"embed"
	"math/rand"

	gui "github.com/MarcosTypeAP/go-rlgui"
	rl "github.com/gen2brain/raylib-go/raylib"
)

//go:embed plus.png
//go:embed music-note-256.png
var iconsFS embed.FS

func main() {
	rl.SetConfigFlags(rl.FlagWindowResizable | rl.FlagMsaa4xHint)

	gui.InitWindow(gui.Vec2(800, 600), "Example")
	defer gui.CloseWindow()

	rl.SetTargetFPS(60)
	rl.SetExitKey(rl.KeyQ)

	gui.DefaultTextColor = rl.White

	colorPairs := []struct {
		name           string
		color1, color2 rl.Color
	}{
		{"Red / Cyan", gui.ColorHex(0xFF0000FF), rl.GetColor(0x00FFFFFF)},
		{"Magenta / Green", gui.ColorHex(0xFF00FFFF), rl.GetColor(0x00FF00FF)},
		{"Yellow / Blue", gui.ColorHex(0xFFFF00FF), rl.GetColor(0x0000FFFF)},
	}
	getColorPairNames := func() []string {
		out := make([]string, len(colorPairs))
		for i := range colorPairs {
			out[i] = colorPairs[i].name
		}
		return out
	}
	selectedColorPairIdx := 0

	gradientAngle := float32(45)

	paintRenderTexture := rl.LoadRenderTexture(400*4/3, 400)

	floatingWindows := []*gui.SubWindow{}

	rootSubWindow := gui.AddSubWindow(gui.NewSubWindow(gui.SubWindowProps{
		SizingX: gui.Grow(),
		SizingY: gui.Grow(),
		ZIndex:  gui.ZIndexRoot,
	}), gui.Vec2(0, 0))

	for !rl.WindowShouldClose() {
		color1 := colorPairs[selectedColorPairIdx].color1
		color2 := colorPairs[selectedColorPairIdx].color2

		gui.ResetLayout()

		root := rootSubWindow.SetRoot(gui.NewBox(gui.BoxProps{
			SizingX:     gui.Grow(),
			SizingY:     gui.Grow(),
			Padding:     gui.Padding(20),
			BgColor:     gui.ColorHex(0x1A1F25FF),
			ChildGap:    20,
			Orientation: gui.Vertical,
		}))

		header := gui.AddChild(root, gui.NewBox(gui.BoxProps{
			SizingX:  gui.Grow(),
			SizingY:  gui.Fixed(60),
			ChildGap: 20,
		}))
		gui.AddChild(header, gui.NewBox(gui.BoxProps{
			SizingX:      gui.Grow(20),
			SizingY:      gui.Grow(),
			Gradient:     gui.GradientLinear(color1, color2, gradientAngle),
			CornerRadius: gui.Radius(10),
		}))
		gradientColorPairDropdown := gui.AddChild(header, gui.NewDropdown(gui.DropdownProps{
			BoxProps: gui.BoxProps{
				ID:           rootSubWindow.GetAutoID(),
				SizingY:      gui.Grow(),
				Padding:      gui.Padding(0, 20),
				BorderWidth:  gui.Border(2),
				BorderColor:  rl.White,
				CornerRadius: gui.Radius(10),
				BgColor:      gui.ColorHex(0x1A1F25FF),
			},
		}, "", getColorPairNames(), int16(selectedColorPairIdx)))
		gradientAngleSlider := gui.AddChild(header, gui.NewSlider(gui.SliderProps{
			BoxProps: gui.BoxProps{
				ID:      rootSubWindow.GetAutoID(),
				SizingX: gui.Grow(30),
				SizingY: gui.Grow(),
			},
			ThumbCornerRadius: gui.Radius(69),
			TrackCornerRadius: gui.Radius(420),
			TrackActiveColor:  color1,
		}, 0, 360, gradientAngle))
		createWindowBtn := gui.AddChild(header, gui.NewButton(gui.ButtonProps{
			BoxProps: gui.BoxProps{
				SizingY:      gui.Grow(),
				Padding:      gui.Padding(10, 15),
				BgColor:      rl.ColorBrightness(color1, -0.4),
				ChildAlignY:  gui.Center,
				CornerRadius: gui.Radius(10),
				BorderWidth:  gui.Border(2),
				BorderColor:  rl.White,
				ChildGap:     10,
			},
			OnHover: gui.EffectBrighten,
			OnPress: func(box *gui.Box) func() {
				box.BgColor = rl.ColorBrightness(color2, -0.4)
				return nil
			},
		}))
		gui.AddChild(createWindowBtn, gui.NewBoxImage(gui.BoxProps{
			SizingX:     gui.Fixed(24),
			SizingY:     gui.AspectRatio(1),
			TextureTint: rl.White,
		}, "plus.png", iconsFS))
		gui.AddChild(createWindowBtn, gui.NewText(gui.TextProps{}, "Create window"))
		gui.AddPostUpdate(func() {
			if gradientColorPairDropdown.HasChanged() {
				selectedColorPairIdx = gradientColorPairDropdown.GetSelectedIdx()
				gui.RemoveNodeFromCache(gradientAngleSlider.ID())
			}

			if gradientAngleSlider.IsChanging() {
				gradientAngle = gradientAngleSlider.GetValue()
			}

			if createWindowBtn.IsLeftButtonPressed() {
				pos := gui.Vec2(
					rand.Float32()*float32(rl.GetScreenWidth()-100),
					rand.Float32()*float32(rl.GetScreenHeight()-100),
				)
				subWindow := gui.AddSubWindow(gui.NewSubWindow(gui.SubWindowProps{
					Floating: true,
					Closable: true,
				}), pos)
				floatingWindows = append(floatingWindows, subWindow)
			}
		})

		for _, subWindow := range floatingWindows {
			winRoot := subWindow.SetRoot(gui.NewBox(gui.BoxProps{
				Padding:      gui.Padding(20),
				CornerRadius: gui.Radius(10),
				BgColor:      rl.DarkGray,
				Orientation:  gui.Vertical,
				ChildGap:     20,
				ChildAlignX:  gui.Center,
			}))
			gui.AddChild(winRoot, gui.NewDropdown(gui.DropdownProps{
				BoxProps: gui.BoxProps{
					ID:           subWindow.GetAutoID(),
					Padding:      gui.Padding(10),
					CornerRadius: gui.Radius(10),
					BgColor:      rl.Gray,
				},
			}, "Select", []string{"Option 1", "Option 2", "Option 3"}, -1))
		}

		body := gui.AddChild(root, gui.NewBox(gui.BoxProps{
			SizingX:  gui.Grow(),
			SizingY:  gui.Grow(),
			ChildGap: 20,
		}))

		leftSide := gui.AddChild(body, gui.NewBox(gui.BoxProps{
			SizingX:     gui.Percentage(40),
			SizingY:     gui.Grow(),
			Orientation: gui.Vertical,
			ChildGap:    20,
		}))
		leftSideTop := gui.AddChild(leftSide, gui.NewBox(gui.BoxProps{
			SizingX:      gui.Grow(),
			SizingY:      gui.Percentage(30),
			Padding:      gui.Padding(20),
			BgColor:      gui.ColorHex(0x2F3439FF),
			CornerRadius: gui.Radius(10),
			HideOverflow: true,
		}))
		gui.AddChild(leftSideTop, gui.NewText(gui.TextProps{
			BoxProps: gui.BoxProps{
				SizingX: gui.Grow(),
				SizingY: gui.Grow(),
			},
			FontConfigProps: gui.FontConfigProps{
				FgColor: color1,
			},
			Wrapping: gui.Wrap,
		}, "Lorem ipsum dolor sit amet, consetetur sadipscing elitr, sed diam nonumy eirmod tempor invidunt ut labore et dolore magna aliquyam"))
		leftSideBottom := gui.AddChild(leftSide, gui.NewBox(gui.BoxProps{
			SizingX:      gui.Grow(),
			SizingY:      gui.Grow(),
			Padding:      gui.Padding(20),
			BgColor:      gui.ColorHex(0x2F3439FF),
			CornerRadius: gui.Radius(10),
		}))
		scrollWrappedBox := gui.AddChild(leftSideBottom, gui.NewScrollBox(gui.ScrollBoxProps{
			BoxProps: gui.BoxProps{
				ID:        rootSubWindow.GetAutoID(),
				SizingX:   gui.Grow(),
				SizingY:   gui.Grow(),
				ChildGap:  20,
				ChildWrap: true,
			},
			ThumbWidth:        10,
			ThumbColor:        gui.ColorHex(0xFFFFFFAA),
			ThumbCornerRadius: 10,
			ScrollOrientation: gui.Vertical,
		}))
		for i := range 50 {
			btn := gui.AddChild(scrollWrappedBox, gui.NewButton(gui.ButtonProps{
				BoxProps: gui.BoxProps{
					SizingX:      gui.Fixed(50),
					SizingY:      gui.AspectRatio(1),
					Gradient:     gui.GradientLinear(gui.Ternary(i%3 == 0, color1, gui.ColorHex(0xA2CACFFF)), rl.GetColor(0x464A4FFF), 45),
					Padding:      gui.Padding(10),
					CornerRadius: gui.Radius(10),
				},
				OnHover: func(box *gui.Box) func() {
					box.Gradient.StartColor = rl.ColorBrightness(box.Gradient.StartColor, 0.5)
					return nil
				},
				OnPress: func(box *gui.Box) func() {
					box.Gradient.StartColor = color2
					return nil
				},
			}))
			gui.AddChild(btn, gui.NewBoxImage(gui.BoxProps{
				SizingX: gui.Grow(),
				SizingY: gui.Grow(),
			}, "music-note-256.png", iconsFS))
		}

		rightSide := gui.AddChild(body, gui.NewBox(gui.BoxProps{
			SizingX:     gui.Grow(),
			SizingY:     gui.Grow(),
			Orientation: gui.Vertical,
			ChildGap:    20,
		}))

		rightSideTop := gui.AddChild(rightSide, gui.NewBox(gui.BoxProps{
			SizingX:      gui.Grow(),
			BgColor:      gui.ColorHex(0x2F3439FF),
			CornerRadius: gui.Radius(10),
			Padding:      gui.Padding(20),
			ChildGap:     20,
			HideOverflow: true,
		}))
		rightSideTopLeft := gui.AddChild(rightSideTop, gui.NewBox(gui.BoxProps{
			SizingX:     gui.Grow(),
			SizingY:     gui.Grow(),
			ChildGap:    10,
			Orientation: gui.Vertical,
		}))
		for _, msg := range []string{"Left click to paint", "Right click to erase", "Wheel to change size"} {
			gui.AddChild(rightSideTopLeft, gui.NewText(gui.TextProps{
				BoxProps: gui.BoxProps{
					SizingX: gui.Grow(),
				},
				Wrapping: gui.EllipsisOverflow,
			}, msg))
		}
		for i := range 2 {
			toggleBox := gui.AddChild(rightSideTopLeft, gui.NewBox(gui.BoxProps{
				SizingX:     gui.Grow(),
				ChildGap:    10,
				ChildAlignY: gui.Center,
			}))
			toggle := gui.AddChild(toggleBox, gui.NewToggle(gui.ToggleProps{
				BoxProps: gui.BoxProps{
					ID:           rootSubWindow.GetAutoID(),
					SizingX:      gui.Fixed(40),
					SizingY:      gui.Fixed(20),
					BgColor:      rl.DarkGray,
					CornerRadius: gui.Radius(69),
				},
				OnColor:  rl.White,
				OffColor: rl.Gray,
			}, i%2 == 0))
			gui.AddChild(toggleBox, gui.NewText(gui.TextProps{
				BoxProps: gui.BoxProps{
					SizingX: gui.Grow(),
				},
				Wrapping: gui.EllipsisOverflow,
			}, gui.Ternary(toggle.GetValue(), "Toggle on", "Toggle off")))
		}
		{
			toggleBox := gui.AddChild(rightSideTopLeft, gui.NewBox(gui.BoxProps{
				SizingX:     gui.Grow(),
				ChildGap:    10,
				ChildAlignY: gui.Center,
			}))
			toggle := gui.AddChild(toggleBox, gui.NewCheckBox(gui.ToggleProps{
				BoxProps: gui.BoxProps{
					ID:           rootSubWindow.GetAutoID(),
					SizingX:      gui.Fixed(24),
					SizingY:      gui.Fixed(24),
					CornerRadius: gui.Radius(69),
					BorderWidth:  gui.Border(4),
					BorderColor:  rl.Gray,
				},
				OnColor:  rl.White,
				OffColor: rl.DarkGray,
			}, false))
			gui.AddChild(toggleBox, gui.NewText(gui.TextProps{
				BoxProps: gui.BoxProps{
					SizingX: gui.Grow(),
				},
				Wrapping: gui.EllipsisOverflow,
			}, gui.Ternary(toggle.GetValue(), "Toggle on", "Toggle off")))
		}
		gui.AddChild(rightSideTop, NewPaintNode(gui.BoxProps{
			ID:      rootSubWindow.GetAutoID(),
			SizingX: gui.Percentage(50),
			SizingY: gui.AspectRatio(4 / 3.),
		}, paintRenderTexture))

		rightSideBottom := gui.AddChild(rightSide, gui.NewBox(gui.BoxProps{
			SizingX:      gui.Grow(),
			SizingY:      gui.Grow(),
			BgColor:      gui.ColorHex(0x2F3439FF),
			CornerRadius: gui.Radius(10),
			Padding:      gui.Padding(20),
			ChildGap:     20,
			Orientation:  gui.Vertical,
		}))
		textInput := gui.AddChild(rightSideBottom, gui.NewTextInput(gui.TextInputProps{
			BoxProps: gui.BoxProps{
				ID:           rootSubWindow.GetAutoID(),
				SizingX:      gui.Grow(),
				BgColor:      rl.DarkGray,
				Padding:      gui.Padding(15, 15),
				CornerRadius: gui.Radius(10),
			},
			PlaceholderColor: rl.Gray,
		}, "Type something", "Type something"))
		textScrollBox := gui.AddChild(rightSideBottom, gui.NewScrollBox(gui.ScrollBoxProps{
			BoxProps: gui.BoxProps{
				ID:          rootSubWindow.GetAutoID(),
				SizingX:     gui.Grow(),
				SizingY:     gui.Grow(),
				ChildGap:    20,
				Orientation: gui.Vertical,
			},
			ThumbWidth:        10,
			ThumbColor:        gui.ColorHex(0xFFFFFFAA),
			ThumbCornerRadius: 10,
			ScrollOrientation: gui.Vertical,
		}))
		for i := range 15 {
			gui.AddChild(textScrollBox, gui.NewText(gui.TextProps{
				BoxProps: gui.BoxProps{
					SizingX: gui.Grow(),
				},
				FontConfigProps: gui.FontConfigProps{
					FgColor:  gui.Ternary(i%3 == 0, color1, rl.LightGray),
					FontSize: gui.DefaultFontSize + float32(i*3),
				},
				Wrapping: gui.EllipsisOverflow,
			}, textInput.Value()))
		}

		gui.ComputeLayout()

		gui.Update()

		rl.BeginDrawing()
		gui.Render()
		rl.EndDrawing()
	}
}

type PaintNode struct {
	gui.Box
	penThickness float32
	canvas       rl.RenderTexture2D
}

func NewPaintNode(boxProps gui.BoxProps, renderTexture rl.RenderTexture2D) *PaintNode {
	if node := gui.GetNodeFromCache[*PaintNode](boxProps.ID); node != nil {
		return node
	}

	rl.BeginTextureMode(renderTexture)
	{
		rl.ClearBackground(rl.White)
	}
	rl.EndTextureMode()

	node := &PaintNode{
		canvas:       renderTexture,
		penThickness: 6,
	}
	node.ApplyProps(boxProps)

	gui.CacheNode(node)
	return node
}

func (n *PaintNode) Update() {
	if !gui.IsNodeHovered(n) {
		return
	}

	n.penThickness = gui.Clamp(n.penThickness+(rl.GetMouseWheelMove()*n.penThickness/10), 2, 100)

	if !rl.IsMouseButtonDown(rl.MouseLeftButton) && !rl.IsMouseButtonDown(rl.MouseRightButton) {
		return
	}

	color := rl.Black
	if rl.IsMouseButtonDown(rl.MouseRightButton) {
		color = rl.White
	}

	pos := n.AbsPos()
	rect := n.Rect()

	scale := float32(n.canvas.Texture.Width) / rect.Width

	rl.BeginTextureMode(n.canvas)
	{
		cursor := rl.Vector2Subtract(rl.GetMousePosition(), pos)
		start := rl.Vector2Scale(cursor, scale)
		end := rl.Vector2Scale(rl.Vector2Subtract(cursor, rl.GetMouseDelta()), scale)

		delta := rl.Vector2Length(rl.Vector2Scale(rl.GetMouseDelta(), scale))
		delta *= rl.GetFrameTime()*20 + 1
		var step = 1 / delta
		for i := range int(delta) {
			pos := rl.Vector2Lerp(start, end, float32(i)*step)
			rl.DrawCircleV(pos, n.penThickness, color)
		}
	}
	rl.EndTextureMode()
}

func (n *PaintNode) Render() {
	isHovered := gui.IsNodeHovered(n)

	if isHovered {
		gui.SetMouseCursor(rl.MouseCursorCrosshair)
	}

	rect := n.Rect()

	src := gui.Rect(0, 0, float32(n.canvas.Texture.Width), float32(-n.canvas.Texture.Height))
	dst := rect
	rl.DrawTexturePro(n.canvas.Texture, src, dst, gui.Vec2(0, 0), 0, rl.White)

	if isHovered {
		scale := rect.Width / float32(n.canvas.Texture.Width)
		rl.DrawCircleLinesV(rl.GetMousePosition(), n.penThickness*scale, rl.Black)
	}
}
