package visual

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/graphics"
)

func init() {
	Register("msi_gc20", msiVisual{})
}

// msiVisual is the MSI Force GC20 V2: a matte-black wired pad with offset
// sticks, a red dragon emblem in place of a guide LED, Back / Mode / Start
// buttons, four mode LEDs, and a top Micro-USB port. Only hardware the
// controller actually has is modeled (no lightbar, touchpad or speaker).
type msiVisual struct{}

func (msiVisual) Name() string { return "MSI Force GC20 V2 3D" }

func (msiVisual) Anchors() []Anchor {
	return []Anchor{
		{Region: device.RegionLeftTrigger, Center: graphics.Vertex3D{X: -0.92, Y: 0.84, Z: 0.12}, Radius: 0.46},
		{Region: device.RegionRightTrigger, Center: graphics.Vertex3D{X: 0.92, Y: 0.84, Z: 0.12}, Radius: 0.46},
		{Region: device.RegionLeftStick, Center: graphics.Vertex3D{X: -0.64, Y: 0.24, Z: -0.36}, Radius: 0.40},
		{Region: device.RegionRightStick, Center: graphics.Vertex3D{X: 0.36, Y: -0.22, Z: -0.36}, Radius: 0.40},
		{Region: device.RegionHaptics, Center: graphics.Vertex3D{X: -1.16, Y: -0.60, Z: 0.0}, Radius: 0.56},
		{Region: device.RegionHaptics, Center: graphics.Vertex3D{X: 1.16, Y: -0.60, Z: 0.0}, Radius: 0.56},
	}
}

func (msiVisual) Build(ctx Context) (graphics.Model3D, []cell.Color) {
	var b meshBuilder
	tel := ctx.Telemetry
	if !ctx.Connected {
		tel = device.Telemetry{}
	}

	// Angular body: central plate, raised shoulders, long grips with red trim.
	b.box(0.0, 0.14, 0.0, 1.66, 0.82, 0.40, 0, 0, 0, device.RegionNone, "shell")
	b.box(0.0, -0.24, 0.0, 1.04, 0.42, 0.38, 0, 0, 0, device.RegionNone, "shell")
	b.box(-0.80, 0.48, 0.0, 0.64, 0.30, 0.38, 0, 0, 16, device.RegionNone, "shell")
	b.box(0.80, 0.48, 0.0, 0.64, 0.30, 0.38, 0, 0, -16, device.RegionNone, "shell")
	b.box(-1.08, -0.42, 0.02, 0.60, 0.92, 0.42, -4, 6, 24, device.RegionHaptics, "shell")
	b.box(-1.20, -0.80, 0.05, 0.44, 0.48, 0.38, -8, 6, 30, device.RegionHaptics, "grip")
	b.box(1.08, -0.42, 0.02, 0.60, 0.92, 0.42, -4, -6, -24, device.RegionHaptics, "shell")
	b.box(1.20, -0.80, 0.05, 0.44, 0.48, 0.38, -8, -6, -30, device.RegionHaptics, "grip")
	b.box(-0.98, -0.30, -0.22, 0.06, 0.62, 0.04, 0, 0, 24, device.RegionNone, "trim")
	b.box(0.98, -0.30, -0.22, 0.06, 0.62, 0.04, 0, 0, -24, device.RegionNone, "trim")

	// LB/RB bumpers and LT/RT triggers (travel follows the reported value).
	b.asButton(device.ButtonL1, func() { b.box(-0.86, 0.68, 0.0, 0.56, 0.14, 0.30, 0, 0, 13, device.RegionLeftTrigger, "bumper") })
	b.asButton(device.ButtonR1, func() { b.box(0.86, 0.68, 0.0, 0.56, 0.14, 0.30, 0, 0, -13, device.RegionRightTrigger, "bumper") })
	b.asButton(device.ButtonL2Digital, func() {
		b.box(-0.92, 0.86-0.08*tel.L2Pressure, 0.16+0.08*tel.L2Pressure, 0.42, 0.30, 0.34, 20+tel.L2Pressure*24, 0, 10, device.RegionLeftTrigger, "trigger")
	})
	b.asButton(device.ButtonR2Digital, func() {
		b.box(0.92, 0.86-0.08*tel.R2Pressure, 0.16+0.08*tel.R2Pressure, 0.42, 0.30, 0.34, 20+tel.R2Pressure*24, 0, -10, device.RegionRightTrigger, "trigger")
	})
	// Micro-USB port on the top edge.
	b.box(0.0, 0.57, 0.0, 0.16, 0.05, 0.10, 0, 0, 0, device.RegionNone, "port")

	// Dragon emblem (also the guide button when the mode reports one).
	b.asButton(device.ButtonHome, func() { b.disk(0.0, 0.44, -0.24, 0.12, 0.08, 0, 0, device.RegionNone, "emblem") })
	b.asButton(device.ButtonView, func() { b.box(-0.24, 0.26, -0.22, 0.10, 0.05, 0.06, 0, 0, 0, device.RegionNone, "small_btn") })
	b.asButton(device.ButtonMenu, func() { b.box(0.24, 0.26, -0.22, 0.10, 0.05, 0.06, 0, 0, 0, device.RegionNone, "small_btn") })
	b.disk(0.0, 0.22, -0.22, 0.045, 0.06, 0, 0, device.RegionNone, "small_btn") // Mode button (not reported to the host)
	for i := 0; i < 4; i++ {
		b.box(-0.105+0.07*float64(i), 0.10, -0.21, 0.035, 0.035, 0.04, 0, 0, 0, device.RegionNone, "led")
	}

	addStick := func(x, y, sx, sy float64, region device.RegionID, click device.ButtonID) {
		ox, oy := sx*0.13, -sy*0.13
		b.disk(x, y, -0.22, 0.24, 0.08, 0, 0, region, "well")
		b.disk(x+ox*0.5, y+oy*0.5, -0.29, 0.12, 0.14, -sy*18, sx*18, region, "stick")
		b.asButton(click, func() { b.disk(x+ox, y+oy, -0.37, 0.22, 0.09, -sy*22, sx*22, region, "stick_cap") })
	}
	addStick(-0.64, 0.24, tel.LeftStickX, tel.LeftStickY, device.RegionLeftStick, device.ButtonL3)
	addStick(0.36, -0.22, tel.RightStickX, tel.RightStickY, device.RegionRightStick, device.ButtonR3)

	dpad := func(id device.ButtonID, dx, dy, sx, sy float64) {
		b.asButton(id, func() { b.box(-0.36+dx, -0.22+dy, -0.24, sx, sy, 0.08, 0, 0, 0, device.RegionNone, "dpad") })
	}
	dpad(device.ButtonDpadUp, 0, 0.11, 0.10, 0.12)
	dpad(device.ButtonDpadDown, 0, -0.11, 0.10, 0.12)
	dpad(device.ButtonDpadLeft, -0.11, 0, 0.12, 0.10)
	dpad(device.ButtonDpadRight, 0.11, 0, 0.12, 0.10)
	b.box(-0.36, -0.22, -0.24, 0.10, 0.10, 0.08, 0, 0, 0, device.RegionNone, "dpad")

	face := func(id device.ButtonID, x, y float64, mat string) {
		b.asButton(id, func() { b.disk(x, y, -0.25, 0.075, 0.08, 0, 0, device.RegionNone, mat) })
	}
	face(device.ButtonNorth, 0.66, 0.40, "btn_y")
	face(device.ButtonEast, 0.82, 0.24, "btn_b")
	face(device.ButtonSouth, 0.66, 0.08, "btn_a")
	face(device.ButtonWest, 0.50, 0.24, "btn_x")

	p := ctx.Palette
	led := rgb(40, 44, 50)
	if ctx.Connected {
		led = rgb(230, 70, 60) // the host cannot read which LEDs are lit; shown as "powered"
	}
	colors := b.colorize(ctx, func(mat string) cell.Color {
		switch mat {
		case "shell":
			return rgb(44, 46, 52)
		case "grip":
			return rgb(30, 32, 36)
		case "trim":
			return rgb(196, 40, 44)
		case "bumper":
			return rgb(70, 72, 80)
		case "trigger":
			return rgb(84, 86, 94)
		case "port":
			return rgb(20, 22, 26)
		case "emblem":
			return rgb(214, 44, 48)
		case "small_btn":
			return rgb(92, 96, 106)
		case "led":
			return led
		case "well":
			return rgb(26, 28, 32)
		case "stick":
			return rgb(50, 52, 58)
		case "stick_cap":
			return rgb(72, 74, 82)
		case "dpad":
			return rgb(86, 90, 98)
		case "btn_a":
			return rgb(70, 150, 80)
		case "btn_b":
			return rgb(180, 64, 64)
		case "btn_x":
			return rgb(64, 110, 190)
		case "btn_y":
			return rgb(190, 160, 50)
		}
		return p.ShellWhite
	})
	return b.model("MSI Force GC20 V2 3D"), colors
}
