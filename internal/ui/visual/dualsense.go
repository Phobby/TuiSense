package visual

import (
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/graphics"
	"github.com/ygzztrk/tuisense/internal/device"
)

func init() {
	Register("dualsense", dualSenseVisual{})
	// The no-controller placeholder reuses the DualSense silhouette (dimmed).
	Register("generic", dualSenseVisual{})
}

type dualSenseVisual struct{}

func (dualSenseVisual) Name() string { return "Sony DualSense 3D" }

func (dualSenseVisual) Anchors() []Anchor {
	return []Anchor{
		{Region: device.RegionLeftTrigger, Center: graphics.Vertex3D{X: -0.98, Y: 0.92, Z: 0.10}, Radius: 0.50},
		{Region: device.RegionRightTrigger, Center: graphics.Vertex3D{X: 0.98, Y: 0.92, Z: 0.10}, Radius: 0.50},
		{Region: device.RegionLight, Center: graphics.Vertex3D{X: 0.0, Y: 0.44, Z: -0.22}, Radius: 0.65},
		{Region: device.RegionLeftStick, Center: graphics.Vertex3D{X: -0.54, Y: -0.16, Z: -0.36}, Radius: 0.42},
		{Region: device.RegionRightStick, Center: graphics.Vertex3D{X: 0.54, Y: -0.16, Z: -0.36}, Radius: 0.42},
		{Region: device.RegionAudio, Center: graphics.Vertex3D{X: 0.0, Y: -0.32, Z: -0.24}, Radius: 0.32},
		{Region: device.RegionMotion, Center: graphics.Vertex3D{X: 0.0, Y: 0.02, Z: -0.18}, Radius: 0.42},
		{Region: device.RegionHaptics, Center: graphics.Vertex3D{X: -1.28, Y: -0.56, Z: 0.0}, Radius: 0.60},
		{Region: device.RegionHaptics, Center: graphics.Vertex3D{X: 1.28, Y: -0.56, Z: 0.0}, Radius: 0.60},
	}
}

// Build constructs the multi-part parametric DualSense (-Z front, +Y top, +X right)
// with live stick/trigger articulation and lightbar/mic LED colors.
func (dualSenseVisual) Build(ctx Context) (graphics.Model3D, []cell.Color) {
	var b meshBuilder
	tel := ctx.Telemetry
	if !ctx.Connected {
		tel = device.Telemetry{}
	}

	// Central bridge & lower deck
	b.box(0.0, 0.14, 0.0, 1.48, 0.80, 0.38, 0, 0, 0, device.RegionMotion, "grip_dark")
	b.box(0.0, -0.18, -0.02, 1.24, 0.42, 0.36, 0, 0, 0, device.RegionMotion, "grip_dark")

	// Sculpted white faceplate wings and shoulder collars
	b.box(-0.98, 0.24, -0.03, 0.72, 0.86, 0.40, 0, 10, 12, device.RegionNone, "shell")
	b.box(0.98, 0.24, -0.03, 0.72, 0.86, 0.40, 0, -10, -12, device.RegionNone, "shell")
	b.box(-0.88, 0.62, -0.02, 0.62, 0.30, 0.38, 0, 8, 18, device.RegionNone, "shell")
	b.box(0.88, 0.62, -0.02, 0.62, 0.30, 0.38, 0, -8, -18, device.RegionNone, "shell")

	// Swept handles / haptic actuator chambers
	b.box(-1.26, -0.46, 0.02, 0.56, 0.96, 0.42, -6, 8, 20, device.RegionHaptics, "shell")
	b.box(-1.35, -0.84, 0.04, 0.44, 0.56, 0.36, -10, 6, 25, device.RegionHaptics, "grip_dark")
	b.box(1.26, -0.46, 0.02, 0.56, 0.96, 0.42, -6, -8, -20, device.RegionHaptics, "shell")
	b.box(1.35, -0.84, 0.04, 0.44, 0.56, 0.36, -10, -6, -25, device.RegionHaptics, "grip_dark")

	// Touchpad (click switch) and lightbar strips
	b.asButton(device.ButtonTouchpadClick, func() {
		b.box(0.0, 0.44, -0.20, 1.12, 0.60, 0.10, 6, 0, 0, device.RegionLight, "touchpad")
	})
	b.box(-0.60, 0.44, -0.22, 0.10, 0.62, 0.10, 6, 0, -5, device.RegionLight, "lightbar")
	b.box(0.60, 0.44, -0.22, 0.10, 0.62, 0.10, 6, 0, 5, device.RegionLight, "lightbar")
	b.box(0.0, 0.11, -0.22, 0.96, 0.08, 0.10, 6, 0, 0, device.RegionLight, "lightbar")

	// L1/R1 bumpers and L2/R2 adaptive triggers
	l2Pull, r2Pull := tel.L2Pressure*22.0, tel.R2Pressure*22.0
	b.asButton(device.ButtonL1, func() {
		b.box(-0.96, 0.80, -0.02, 0.54, 0.16, 0.32, 0, 0, 10, device.RegionLeftTrigger, "bumper")
	})
	b.asButton(device.ButtonR1, func() {
		b.box(0.96, 0.80, -0.02, 0.54, 0.16, 0.32, 0, 0, -10, device.RegionRightTrigger, "bumper")
	})
	b.asButton(device.ButtonL2Digital, func() {
		b.box(-0.98, 0.96-0.08*tel.L2Pressure, 0.14+0.08*tel.L2Pressure, 0.48, 0.32, 0.34, 18+l2Pull, 0, 8, device.RegionLeftTrigger, "trigger")
	})
	b.asButton(device.ButtonR2Digital, func() {
		b.box(0.98, 0.96-0.08*tel.R2Pressure, 0.14+0.08*tel.R2Pressure, 0.48, 0.32, 0.34, 18+r2Pull, 0, -8, device.RegionRightTrigger, "trigger")
	})

	// Symmetric analog sticks
	addStick := func(x float64, sx, sy float64, region device.RegionID, click device.ButtonID) {
		ox, oy := sx*0.14, -sy*0.14
		b.disk(x, -0.16, -0.21, 0.24, 0.08, 0, 0, region, "grip_dark")
		b.disk(x+ox*0.5, -0.16+oy*0.5, -0.28, 0.13, 0.14, -sy*18, sx*18, region, "stick")
		b.asButton(click, func() {
			b.disk(x+ox, -0.16+oy, -0.37, 0.23, 0.09, -sy*22, sx*22, region, "stick_cap")
		})
	}
	addStick(-0.54, tel.LeftStickX, tel.LeftStickY, device.RegionLeftStick, device.ButtonL3)
	addStick(0.54, tel.RightStickX, tel.RightStickY, device.RegionRightStick, device.ButtonR3)

	// D-pad as four arms so each direction can light up
	dpad := func(id device.ButtonID, dx, dy, sx, sy float64) {
		b.asButton(id, func() { b.box(-0.98+dx, 0.30+dy, -0.25, sx, sy, 0.08, 0, 10, 0, device.RegionNone, "dpad") })
	}
	dpad(device.ButtonDpadUp, 0, 0.12, 0.11, 0.13)
	dpad(device.ButtonDpadDown, 0, -0.12, 0.11, 0.13)
	dpad(device.ButtonDpadLeft, -0.12, 0, 0.13, 0.11)
	dpad(device.ButtonDpadRight, 0.12, 0, 0.13, 0.11)
	b.box(-0.98, 0.30, -0.25, 0.11, 0.11, 0.08, 0, 10, 0, device.RegionNone, "dpad")

	face := func(id device.ButtonID, x, y float64, mat string) {
		b.asButton(id, func() { b.disk(x, y, -0.25, 0.08, 0.08, 0, -10, device.RegionNone, mat) })
	}
	face(device.ButtonNorth, 0.98, 0.46, "btn_triangle")
	face(device.ButtonEast, 1.14, 0.30, "btn_circle")
	face(device.ButtonSouth, 0.98, 0.14, "btn_cross")
	face(device.ButtonWest, 0.82, 0.30, "btn_square")

	// Create / Options / PS buttons
	b.asButton(device.ButtonView, func() { b.disk(-0.56, 0.66, -0.24, 0.05, 0.06, 0, 0, device.RegionNone, "dpad") })
	b.asButton(device.ButtonMenu, func() { b.disk(0.56, 0.66, -0.24, 0.05, 0.06, 0, 0, device.RegionNone, "dpad") })
	b.asButton(device.ButtonHome, func() { b.disk(0.0, -0.06, -0.24, 0.07, 0.06, 0, 0, device.RegionNone, "dpad") })

	// Microphone mute button & LED
	b.asButton(device.ButtonMicMute, func() {
		b.box(0.0, -0.30, -0.23, 0.22, 0.10, 0.08, 0, 0, 0, device.RegionAudio, "mic_led")
	})

	p := ctx.Palette
	cfg := ctx.Config
	lightbar := rgb(cfg.Lighting.Red, cfg.Lighting.Green, cfg.Lighting.Blue)
	if !cfg.Lighting.Enabled || cfg.Lighting.Animation == device.AnimOff || !ctx.Connected {
		lightbar = rgb(55, 70, 95)
	}
	mic := rgb(95, 110, 140)
	if ctx.Connected && (cfg.Lighting.MicLED != device.MicLEDOff || tel.MicMutedButton) {
		mic = rgb(255, 170, 0)
	}

	colors := b.colorize(ctx, func(mat string) cell.Color {
		switch mat {
		case "shell":
			return p.ShellWhite
		case "grip_dark":
			return p.ShellGripDark
		case "touchpad":
			return p.ShellTouchpad
		case "lightbar":
			return lightbar
		case "bumper":
			return rgb(90, 108, 145)
		case "trigger":
			return rgb(120, 145, 195)
		case "stick":
			return rgb(68, 84, 118)
		case "stick_cap":
			return rgb(92, 112, 152)
		case "dpad":
			return rgb(130, 150, 190)
		case "btn_triangle":
			return rgb(60, 220, 180)
		case "btn_circle":
			return rgb(255, 95, 115)
		case "btn_cross":
			return rgb(95, 165, 255)
		case "btn_square":
			return rgb(235, 130, 235)
		case "mic_led":
			return mic
		}
		return p.ShellWhite
	})
	return b.model("Sony DualSense 3D"), colors
}
