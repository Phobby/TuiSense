package visual

import "github.com/Phobby/TuiSense/internal/device"

func init() {
	Register(dualSense())
	Register(dualSenseEdge())
	Register(dualShock4())
}

var sonyFaceIDs = [4]string{"btn_triangle", "btn_circle", "btn_cross", "btn_square"}
var sonyFaceLabels = [4]string{"Triangle", "Circle", "Cross", "Square"}
var sonyFaceMaterials = [4]string{"btn_triangle", "btn_circle", "btn_cross", "btn_square"}

// dualSenseFront is shared by the DualSense and the DualSense Edge, which have
// the same front layout; the Edge adds Fn buttons and rear controls.
func dualSenseFront(regions bool) []Component {
	reg := func(r device.RegionID) device.RegionID {
		if regions {
			return r
		}
		return device.RegionNone
	}
	c := []Component{
		{ID: "motion", Kind: KindMotionSensor, Label: "6-axis motion sensor (internal)", Side: Center, Region: reg(device.RegionMotion),
			Verify: Approximate, Note: "Internal sensor; selected through the shell."},
		decor("core", "core", pill(0, -8, 62, 24, 1)),
		{ID: "touchpad", Kind: KindTouchpad, Label: "Touchpad", Side: Center, Surface: Front, Material: "touchpad",
			Button: device.ButtonTouchpadClick, Bound: true, Region: reg(device.RegionLight), Verify: Verified,
			Shapes: []Geometry{pill(0, 19, 54, 26, 3)}},
		{ID: "light_bar", Kind: KindLightingZone, Label: "Light bar", Side: Center, Surface: Front, Material: "light",
			Region: reg(device.RegionLight), Verify: Verified, Note: "Two strips framing the touchpad.",
			Shapes: []Geometry{
				{Shape: ShapeBox, At: at(-29.6, 19, 0), Size: sz(2.2, 25, 2.6), Rot: [3]float64{0, 0, 5}},
				{Shape: ShapeBox, At: at(29.6, 19, 0), Size: sz(2.2, 25, 2.6), Rot: [3]float64{0, 0, -5}},
			}},
		{ID: "player_leds", Kind: KindIndicator, Label: "Player indicator", Side: Center, Surface: Front, Material: "led_white",
			Region: reg(device.RegionLight), Verify: Verified, Note: "Five LEDs below the touchpad.",
			Shapes: []Geometry{disc(-6, 4.2, 0.9, 1.4), disc(-3, 4.2, 0.9, 1.4), disc(0, 4.2, 0.9, 1.4), disc(3, 4.2, 0.9, 1.4), disc(6, 4.2, 0.9, 1.4)}},
		button("create", "Create", KindSystemButton, Left, device.ButtonView, true, pill(-37, 27, 3.6, 7, 3), "small_btn"),
		button("options", "Options", KindSystemButton, Right, device.ButtonMenu, true, pill(37, 27, 3.6, 7, 3), "small_btn"),
		button("ps", "PS", KindSystemButton, Center, device.ButtonHome, true, disc(0, -9, 3.6, 2.5), "small_btn"),
		{ID: "mic_mute", Kind: KindAudioComponent, Label: "Mic mute", Side: Center, Surface: Front, Material: "mic_led",
			Button: device.ButtonMicMute, Bound: true, Region: reg(device.RegionAudio), Verify: Verified,
			Shapes: []Geometry{pill(0, -16, 7, 2.4, 2)}},
		stick(Left, "Left stick (L3)", -24, -13, 8.5, reg(device.RegionLeftStick)),
		stick(Right, "Right stick (R3)", 24, -13, 8.5, reg(device.RegionRightStick)),
		port("usb", "USB-C port", Top, 0, 9, 4, KindPort, device.RegionNone, Verified, ""),
		port("headset_jack", "3.5 mm headset jack", Bottom, 0, 5, 4, KindAudioComponent, reg(device.RegionAudio), Verified, ""),
	}
	c = append(c, dpad(-50, 9, 19, 7, "dpad")...)
	c = append(c, faceButtons(50, 9, 9.2, 4.4, sonyFaceIDs, sonyFaceLabels, sonyFaceMaterials)...)
	c = append(c, shoulders(52, 24, 21, [4]string{"L1", "R1", "L2", "R2"}, regions)...)
	c = append(c, grips(52, -36, 40, 66, 34, 6, [2]string{"Left grip (haptic actuator)", "Right grip (haptic actuator)"}, "shell", reg(device.RegionHaptics))...)
	return c
}

var dualSenseHalf = [][2]float64{
	{0, 35}, {20, 36}, {40, 37}, {56, 35}, {68, 30}, {76, 21}, {80, 8}, {80, -6}, {78, -22}, {74, -40},
	{68, -54}, {60, -64}, {51, -68}, {43, -66}, {37, -56}, {33, -42}, {27, -30}, {15, -23}, {0, -21},
}

func sonyMaterials() map[string]RGB {
	return map[string]RGB{
		"shell": {236, 238, 243}, "core": {30, 32, 38}, "touchpad": {222, 225, 232}, "light": {64, 104, 210},
		"led_white": {150, 155, 165}, "small_btn": {58, 60, 68}, "mic_led": {104, 110, 126},
		"stick": {46, 48, 56}, "bumper": {44, 46, 54}, "trigger": {52, 54, 62}, "dpad": {66, 70, 80},
		"btn_triangle": {74, 112, 104}, "btn_circle": {124, 76, 84}, "btn_cross": {72, 92, 126}, "btn_square": {112, 84, 118},
		"port": {18, 20, 24}, "switch_knob": {200, 204, 212}, "back_button": {40, 42, 50},
	}
}

func dualSense() ModelSpec {
	return ModelSpec{
		ID: "dualsense", Manufacturer: "Sony", Name: "DualSense Wireless Controller", Revision: "CFI-ZCT1",
		Gallery: true, Dimensions: [3]float64{160, 66, 106}, DimensionsNote: "Approximate published W×D×H.",
		Body: Body{Half: dualSenseHalf, Smooth: 3, Depth: 30, Bevel: 6, Material: "shell", ComponentID: "motion",
			SpineWidth: 100, SpineHeight: 14, SpineDepth: 11},
		Components:       dualSenseFront(true),
		Materials:        sonyMaterials(),
		HardwareModelIDs: []string{"sony.dualsense"},
		References: []string{
			"https://www.playstation.com/en-us/accessories/dualsense-wireless-controller/",
			"https://www.playstation.com/en-us/support/hardware/ps5-controller-guide/",
		},
		Caveats: []string{
			"Silhouette and grip volume are a low-poly interpretation, not CAD data.",
			"The built-in speaker and microphones are internal and are not modelled as separate parts.",
		},
	}
}

func dualSenseEdge() ModelSpec {
	s := dualSense()
	s.ID, s.Name, s.Revision = "dualsense_edge", "DualSense Edge Wireless Controller", "CFI-ZCP1"
	s.HardwareModelIDs = []string{"sony.dualsense_edge"}
	s.References = []string{
		"https://www.playstation.com/en-us/accessories/dualsense-edge-wireless-controller/",
		"https://www.playstation.com/en-us/support/hardware/dualsense-edge-wireless-controller-guide/",
	}
	s.Materials = sonyMaterials()
	s.Materials["core"] = RGB{20, 21, 26}
	s.Materials["small_btn"] = RGB{38, 40, 46}
	s.Materials["stick"] = RGB{34, 36, 42}
	s.Components = append(dualSenseFront(true),
		decor("stick_ring_l", "stick_ring", disc(-24, -13, 12.4, 0.8)),
		decor("stick_ring_r", "stick_ring", disc(24, -13, 12.4, 0.8)),
		Component{ID: "fn_left", Kind: KindProfileButton, Label: "Fn (left)", Side: Left, Surface: Front, Material: "small_btn",
			Button: device.ButtonLeftFn, Bound: true, Verify: Approximate, Note: "Below the left stick; exact offset approximated.",
			Shapes: []Geometry{pill(-24, -26.5, 5, 2.2, 2)}},
		Component{ID: "fn_right", Kind: KindProfileButton, Label: "Fn (right)", Side: Right, Surface: Front, Material: "small_btn",
			Button: device.ButtonRightFn, Bound: true, Verify: Approximate, Note: "Below the right stick; exact offset approximated.",
			Shapes: []Geometry{pill(24, -26.5, 5, 2.2, 2)}},
		Component{ID: "back_left", Kind: KindRearButton, Label: "Left back button", Side: Left, Surface: Rear, Material: "back_button",
			Button: device.ButtonLeftPaddle, Bound: true, Verify: Approximate,
			Note:   "Interchangeable half-dome or lever attachment (half-dome shown); position approximated.",
			Shapes: []Geometry{{Shape: ShapeDome, At: at(-44, -42, -2), Size: sz(5.5, 0, 6)}}},
		Component{ID: "back_right", Kind: KindRearButton, Label: "Right back button", Side: Right, Surface: Rear, Material: "back_button",
			Button: device.ButtonRightPaddle, Bound: true, Verify: Approximate,
			Note:   "Interchangeable half-dome or lever attachment (half-dome shown); position approximated.",
			Shapes: []Geometry{{Shape: ShapeDome, At: at(44, -42, -2), Size: sz(5.5, 0, 6)}}},
		rearControl("trigger_stop_left", "Trigger stop (left)", KindHardwareSwitch, Left,
			Geometry{Shape: ShapeSlider, At: at(-47, 10, 0), Size: sz(10, 4.5, 3.5)}, "switch_track", Verified,
			"Three-level slider limiting L2 travel; the position is not reported to the host."),
		rearControl("trigger_stop_right", "Trigger stop (right)", KindHardwareSwitch, Right,
			Geometry{Shape: ShapeSlider, At: at(47, 10, 0), Size: sz(10, 4.5, 3.5)}, "switch_track", Verified,
			"Three-level slider limiting R2 travel; the position is not reported to the host."),
	)
	s.Materials["stick_ring"] = RGB{24, 25, 30}
	s.Materials["switch_track"] = RGB{30, 32, 38}
	s.Caveats = append(s.Caveats,
		"Back-button and Fn positions are approximated from product photos.",
		"Replaceable stick modules and the cable lock housing are not modelled separately.")
	return s
}

func dualShock4() ModelSpec {
	m := sonyMaterials()
	m["shell"], m["core"], m["touchpad"] = RGB{38, 40, 46}, RGB{32, 34, 40}, RGB{48, 50, 58}
	m["grille"] = RGB{24, 25, 30}
	m["light"] = RGB{60, 110, 230}
	c := []Component{
		{ID: "motion", Kind: KindMotionSensor, Label: "6-axis motion sensor (internal)", Side: Center, Verify: Approximate,
			Note: "Internal sensor; selected through the shell."},
		{ID: "touchpad", Kind: KindTouchpad, Label: "Touchpad", Side: Center, Surface: Front, Material: "touchpad",
			Button: device.ButtonTouchpadClick, Bound: true, Verify: Verified, Shapes: []Geometry{pill(0, 15, 46, 23, 3)}},
		{ID: "light_bar", Kind: KindLightingZone, Label: "Light bar", Side: Center, Surface: Front, Material: "light", Verify: Verified,
			Note: "Top-edge light bar; the CUH-ZCT2 revision also shows a strip at the top of the touchpad.",
			Shapes: []Geometry{
				{Shape: ShapeBox, At: at(0, -0.4, 9), Size: sz(42, 3, 6), Surface: Top},
				{Shape: ShapeBox, At: at(0, 27.4, 0), Size: sz(40, 1.3, 1.6)},
			}},
		button("share", "Share", KindSystemButton, Left, device.ButtonView, true, pill(-32, 25, 3.4, 6, 3), "small_btn"),
		button("options", "Options", KindSystemButton, Right, device.ButtonMenu, true, pill(32, 25, 3.4, 6, 3), "small_btn"),
		button("ps", "PS", KindSystemButton, Center, device.ButtonHome, true, disc(0, -9, 3.6, 2.5), "small_btn"),
		{ID: "speaker", Kind: KindAudioComponent, Label: "Speaker", Side: Center, Surface: Front, Material: "grille", Verify: Verified,
			Shapes: []Geometry{pill(0, -16.5, 9, 3.2, 1)}},
		stick(Left, "Left stick (L3)", -21, -17, 8, device.RegionLeftStick),
		stick(Right, "Right stick (R3)", 21, -17, 8, device.RegionRightStick),
		port("usb", "Micro-USB port", Top, 0, 8, 3, KindPort, device.RegionNone, Verified, ""),
		port("headset_jack", "3.5 mm headset jack", Bottom, -5, 4.5, 4, KindAudioComponent, device.RegionNone, Approximate, "Exact offset approximated."),
		port("extension_port", "Extension port", Bottom, 6, 8, 3, KindPort, device.RegionNone, Approximate, "Exact offset approximated."),
	}
	c = append(c, dpad(-50, 6, 19, 7, "dpad")...)
	c = append(c, faceButtons(50, 6, 9.5, 4.4, sonyFaceIDs, sonyFaceLabels, sonyFaceMaterials)...)
	c = append(c, shoulders(52, 24, 20, [4]string{"L1", "R1", "L2", "R2"}, true)...)
	c = append(c, grips(53, -40, 32, 52, 36, 6, [2]string{"Left grip (rumble motor)", "Right grip (rumble motor)"}, "shell", device.RegionHaptics)...)
	return ModelSpec{
		ID: "dualshock4", Manufacturer: "Sony", Name: "DualShock 4 Wireless Controller", Revision: "CUH-ZCT2",
		Gallery: true, Dimensions: [3]float64{162, 52, 98}, DimensionsNote: "Approximate published W×D×H.",
		Body: Body{Half: [][2]float64{
			{0, 29}, {24, 30}, {44, 30}, {60, 28}, {71, 23}, {77, 13}, {80, 0}, {79, -16}, {75, -34}, {69, -50},
			{61, -62}, {51, -67}, {42, -63}, {37, -50}, {33, -37}, {26, -27}, {14, -21}, {0, -20},
		}, Smooth: 3, Depth: 26, Bevel: 5, Material: "shell", ComponentID: "motion", SpineWidth: 112, SpineHeight: 14, SpineDepth: 14},
		Components:       c,
		Materials:        m,
		HardwareModelIDs: []string{"sony.dualshock4"},
		References:       []string{"https://www.playstation.com/en-us/support/hardware/ps4-controller-guide/"},
		Caveats: []string{
			"The touchpad-top light strip is drawn as part of the light bar.",
		},
	}
}
