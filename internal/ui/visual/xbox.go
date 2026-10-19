package visual

import (
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/graphics"
	"github.com/ygzztrk/tuisense/internal/device"
)

func init() {
	Register("xbox", xboxVisual{})
}

// xboxVisual is an Xbox-style gamepad: offset (asymmetric) sticks, a round
// guide button, colored ABXY buttons, and impulse triggers.
type xboxVisual struct{}

func (xboxVisual) Name() string { return "Xbox-style Gamepad 3D" }

func (xboxVisual) Anchors() []Anchor {
	return []Anchor{
		{Region: device.RegionLeftTrigger, Center: graphics.Vertex3D{X: -0.92, Y: 0.86, Z: 0.12}, Radius: 0.48},
		{Region: device.RegionRightTrigger, Center: graphics.Vertex3D{X: 0.92, Y: 0.86, Z: 0.12}, Radius: 0.48},
		{Region: device.RegionLight, Center: graphics.Vertex3D{X: 0.0, Y: 0.50, Z: -0.26}, Radius: 0.30},
		{Region: device.RegionLeftStick, Center: graphics.Vertex3D{X: -0.66, Y: 0.26, Z: -0.36}, Radius: 0.40},
		{Region: device.RegionRightStick, Center: graphics.Vertex3D{X: 0.36, Y: -0.20, Z: -0.36}, Radius: 0.40},
		{Region: device.RegionAudio, Center: graphics.Vertex3D{X: 0.0, Y: -0.46, Z: -0.10}, Radius: 0.26},
		{Region: device.RegionHaptics, Center: graphics.Vertex3D{X: -1.18, Y: -0.58, Z: 0.0}, Radius: 0.58},
		{Region: device.RegionHaptics, Center: graphics.Vertex3D{X: 1.18, Y: -0.58, Z: 0.0}, Radius: 0.58},
	}
}

func (xboxVisual) Build(ctx Context) (graphics.Model3D, []cell.Color) {
	var b meshBuilder
	tel := ctx.Telemetry
	if !ctx.Connected {
		tel = device.Telemetry{}
	}

	// One-piece rounded body: central mass + softened shoulders
	b.box(0.0, 0.16, 0.0, 1.70, 0.86, 0.40, 0, 0, 0, device.RegionNone, "shell")
	b.box(0.0, -0.22, 0.0, 1.10, 0.40, 0.38, 0, 0, 0, device.RegionNone, "shell")
	b.box(-0.80, 0.50, 0.0, 0.66, 0.30, 0.38, 0, 0, 14, device.RegionNone, "shell")
	b.box(0.80, 0.50, 0.0, 0.66, 0.30, 0.38, 0, 0, -14, device.RegionNone, "shell")

	// Grips (haptic rumble motors) with textured dark inserts
	b.box(-1.10, -0.40, 0.02, 0.62, 0.90, 0.42, -4, 6, 26, device.RegionHaptics, "shell")
	b.box(-1.22, -0.78, 0.05, 0.46, 0.50, 0.38, -8, 6, 32, device.RegionHaptics, "grip")
	b.box(1.10, -0.40, 0.02, 0.62, 0.90, 0.42, -4, -6, -26, device.RegionHaptics, "shell")
	b.box(1.22, -0.78, 0.05, 0.46, 0.50, 0.38, -8, -6, -32, device.RegionHaptics, "grip")

	// LB/RB bumpers and LT/RT impulse triggers
	b.asButton(device.ButtonL1, func() { b.box(-0.88, 0.70, 0.0, 0.58, 0.14, 0.30, 0, 0, 12, device.RegionLeftTrigger, "bumper") })
	b.asButton(device.ButtonR1, func() { b.box(0.88, 0.70, 0.0, 0.58, 0.14, 0.30, 0, 0, -12, device.RegionRightTrigger, "bumper") })
	b.box(-0.92, 0.88-0.08*tel.L2Pressure, 0.16+0.08*tel.L2Pressure, 0.44, 0.30, 0.34, 20+tel.L2Pressure*24, 0, 10, device.RegionLeftTrigger, "trigger")
	b.box(0.92, 0.88-0.08*tel.R2Pressure, 0.16+0.08*tel.R2Pressure, 0.44, 0.30, 0.34, 20+tel.R2Pressure*24, 0, -10, device.RegionRightTrigger, "trigger")

	// Guide button (glowing LED) plus view/menu/share buttons
	b.asButton(device.ButtonHome, func() { b.disk(0.0, 0.50, -0.24, 0.13, 0.08, 0, 0, device.RegionLight, "guide") })
	b.asButton(device.ButtonView, func() { b.disk(-0.22, 0.26, -0.22, 0.05, 0.06, 0, 0, device.RegionNone, "small_btn") })
	b.asButton(device.ButtonMenu, func() { b.disk(0.22, 0.26, -0.22, 0.05, 0.06, 0, 0, device.RegionNone, "small_btn") })
	b.asButton(device.ButtonShare, func() { b.box(0.0, 0.14, -0.22, 0.10, 0.06, 0.06, 0, 0, 0, device.RegionNone, "small_btn") })

	// Offset sticks: left high, right low
	addStick := func(x, y, sx, sy float64, region device.RegionID, click device.ButtonID) {
		ox, oy := sx*0.13, -sy*0.13
		b.disk(x, y, -0.22, 0.24, 0.08, 0, 0, region, "well")
		b.disk(x+ox*0.5, y+oy*0.5, -0.29, 0.12, 0.14, -sy*18, sx*18, region, "stick")
		b.asButton(click, func() { b.disk(x+ox, y+oy, -0.37, 0.22, 0.09, -sy*22, sx*22, region, "stick_cap") })
	}
	addStick(-0.66, 0.26, tel.LeftStickX, tel.LeftStickY, device.RegionLeftStick, device.ButtonL3)
	addStick(0.36, -0.20, tel.RightStickX, tel.RightStickY, device.RegionRightStick, device.ButtonR3)

	// D-pad (lower left) as four arms
	dpad := func(id device.ButtonID, dx, dy, sx, sy float64) {
		b.asButton(id, func() { b.box(-0.36+dx, -0.20+dy, -0.24, sx, sy, 0.08, 0, 0, 0, device.RegionNone, "dpad") })
	}
	dpad(device.ButtonDpadUp, 0, 0.11, 0.10, 0.12)
	dpad(device.ButtonDpadDown, 0, -0.11, 0.10, 0.12)
	dpad(device.ButtonDpadLeft, -0.11, 0, 0.12, 0.10)
	dpad(device.ButtonDpadRight, 0.11, 0, 0.12, 0.10)
	b.box(-0.36, -0.20, -0.24, 0.10, 0.10, 0.08, 0, 0, 0, device.RegionNone, "dpad")

	// ABXY (upper right): Y top, B right, A bottom, X left
	face := func(id device.ButtonID, x, y float64, mat string) {
		b.asButton(id, func() { b.disk(x, y, -0.25, 0.075, 0.08, 0, 0, device.RegionNone, mat) })
	}
	face(device.ButtonNorth, 0.66, 0.42, "btn_y")
	face(device.ButtonEast, 0.82, 0.26, "btn_b")
	face(device.ButtonSouth, 0.66, 0.10, "btn_a")
	face(device.ButtonWest, 0.50, 0.26, "btn_x")

	// Bottom 3.5mm headset jack
	b.box(0.0, -0.44, -0.08, 0.18, 0.08, 0.10, 0, 0, 0, device.RegionAudio, "jack")

	p := ctx.Palette
	cfg := ctx.Config
	guide := rgb(80, 90, 105)
	if ctx.Connected && cfg.Lighting.Enabled {
		switch cfg.Lighting.Brightness {
		case device.BrightnessLow:
			guide = rgb(150, 160, 160)
		case device.BrightnessMedium:
			guide = rgb(205, 215, 210)
		default:
			guide = rgb(250, 255, 250)
		}
	}

	colors := b.colorize(ctx, func(mat string) cell.Color {
		switch mat {
		case "shell":
			return rgb(222, 226, 230)
		case "grip":
			return rgb(52, 58, 68)
		case "bumper":
			return rgb(150, 156, 166)
		case "trigger":
			return rgb(120, 128, 140)
		case "guide":
			return guide
		case "small_btn":
			return rgb(70, 76, 88)
		case "well":
			return rgb(46, 52, 62)
		case "stick":
			return rgb(60, 66, 78)
		case "stick_cap":
			return rgb(82, 90, 104)
		case "dpad":
			return rgb(70, 76, 88)
		case "btn_a":
			return rgb(80, 200, 90)
		case "btn_b":
			return rgb(235, 80, 80)
		case "btn_x":
			return rgb(70, 140, 255)
		case "btn_y":
			return rgb(245, 200, 50)
		case "jack":
			return rgb(30, 34, 40)
		}
		return p.ShellWhite
	})
	return b.model("Xbox-style Gamepad 3D"), colors
}
