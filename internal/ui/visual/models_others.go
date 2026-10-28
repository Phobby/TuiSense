package visual

import "github.com/Phobby/TuiSense/internal/device"

// KindNFC is a model-specific kind registered by the Switch Pro Controller.
const KindNFC ComponentKind = "nfc_reader"

func init() {
	RegisterKind(KindNFC, "NFC reader")
	Register(switchPro())
	Register(eightBitDoUltimate())
	Register(gameSirG7SE())
	Register(razerWolverineV3Pro())
	Register(msiGC20V2())
	Register(genericGamepad())
}

func switchPro() ModelSpec {
	c := []Component{
		{ID: "motion", Kind: KindMotionSensor, Label: "Motion sensor (gyroscope/accelerometer)", Side: Center, Verify: Approximate,
			Note: "Internal sensor; selected through the shell."},
		stick(Left, "Left stick", -37, 12, 8, device.RegionLeftStick),
		stick(Right, "Right stick", 21, -12, 8, device.RegionRightStick),
		button("minus", "−", KindSystemButton, Left, device.ButtonView, true, pill(-15, 24, 5, 1.8, 2.2), "small_btn"),
		{ID: "plus", Kind: KindSystemButton, Label: "+", Side: Right, Surface: Front, Material: "small_btn",
			Button: device.ButtonMenu, Bound: true, Verify: Verified,
			Shapes: []Geometry{{Shape: ShapeCross, At: at(15, 24, 0), Size: sz(5.2, 1.8, 2.2)}}},
		button("capture", "Capture", KindSystemButton, Left, 0, false, pill(-11, 12, 4.4, 4.4, 2), "small_btn"),
		button("home", "HOME", KindSystemButton, Right, device.ButtonHome, true, disc(11, 12, 3.2, 2.4), "small_btn"),
		decor("home_ring", "ring", disc(11, 12, 4.4, 0.8)),
		{ID: "nfc", Kind: KindNFC, Label: "NFC touchpoint", Side: Center, Surface: Front, Material: "nfc",
			Verify: Approximate, Note: "Centre-top touchpoint; outline approximated.",
			Shapes: []Geometry{pill(0, 31, 18, 6, 0.8)}},
		{ID: "player_leds", Kind: KindIndicator, Label: "Player indicator", Side: Center, Surface: Bottom, Material: "led_green",
			Verify: Unverified, Note: "Four player LEDs exist; their exact placement is not verified.",
			Shapes: []Geometry{{Shape: ShapeBox, At: at(-4.5, -0.6, -6), Size: sz(1.6, 1.6, 1.4)}, {Shape: ShapeBox, At: at(-1.5, -0.6, -6), Size: sz(1.6, 1.6, 1.4)},
				{Shape: ShapeBox, At: at(1.5, -0.6, -6), Size: sz(1.6, 1.6, 1.4)}, {Shape: ShapeBox, At: at(4.5, -0.6, -6), Size: sz(1.6, 1.6, 1.4)}}},
		{ID: "sync", Kind: KindSystemButton, Label: "SYNC button", Side: Center, Surface: Top, Material: "small_btn", Verify: Verified,
			Note: "Pairing; not reported as a gamepad input.", Shapes: []Geometry{{Shape: ShapeBox, At: at(-10, -1, 0), Size: sz(4, 3, 3)}}},
		port("usb", "USB-C port", Top, 0, 9, 4, KindPort, device.RegionNone, Verified, ""),
	}
	c = append(c, dpad(-21, -12, 18, 6.5, "dpad")...)
	c = append(c, nintendoFaceButtons(37, 12, 8.6, 4.0, [4]string{"btn", "btn", "btn", "btn"})...)
	sh := shoulders(50, 26, 20, [4]string{"L", "R", "ZL", "ZR"}, true)
	for i := 2; i < 4; i++ {
		sh[i].Note = "Digital trigger (on/off)."
	}
	c = append(c, sh...)
	c = append(c, grips(49, -38, 38, 60, 34, 6, [2]string{"Left grip (HD rumble)", "Right grip (HD rumble)"}, "grip", device.RegionHaptics)...)
	return ModelSpec{
		ID: "switch_pro", Manufacturer: "Nintendo", Name: "Nintendo Switch Pro Controller", Revision: "HAC-013",
		Gallery: true, Dimensions: [3]float64{152, 60, 106}, DimensionsNote: "Approximate.",
		Body: Body{Half: [][2]float64{
			{0, 38}, {26, 38}, {46, 37}, {60, 34}, {70, 27}, {75, 16}, {77, 2}, {75, -16}, {71, -34}, {65, -50},
			{57, -61}, {48, -66}, {40, -62}, {35, -50}, {31, -36}, {25, -27}, {12, -22}, {0, -21},
		}, Smooth: 3, Depth: 29, Bevel: 7, Material: "shell", ComponentID: "motion", SpineWidth: 100, SpineHeight: 13, SpineDepth: 10},
		Components: c,
		Materials: map[string]RGB{
			"shell": {54, 56, 63}, "grip": {40, 42, 48}, "small_btn": {30, 31, 36}, "ring": {70, 72, 80}, "nfc": {62, 64, 72},
			"led_green": {70, 190, 110}, "stick": {36, 37, 42}, "bumper": {34, 35, 40}, "trigger": {38, 39, 45},
			"dpad": {36, 37, 42}, "btn": {40, 41, 47}, "port": {14, 15, 18},
		},
		References: []string{
			"https://www.nintendo.com/us/store/products/pro-controller-110010/",
			"https://en-americas-support.nintendo.com/app/answers/detail/a_id/22585/",
		},
		HardwareModelIDs: []string{"nintendo.switch_pro"},
		Caveats: []string{
			"Player LED and NFC touchpoint placement are approximate.",
		},
	}
}

func eightBitDoUltimate() ModelSpec {
	c := []Component{
		{ID: "motion", Kind: KindMotionSensor, Label: "Motion sensor (gyroscope)", Side: Center, Verify: Approximate,
			Note: "Six-axis motion is advertised for Switch; internal position not modelled."},
		stick(Left, "Left stick", -36, 10, 7.8, device.RegionLeftStick),
		stick(Right, "Right stick", 21, -12, 7.8, device.RegionRightStick),
		button("minus", "−", KindSystemButton, Left, device.ButtonView, true, pill(-12, 21, 4.6, 1.8, 2.2), "small_btn"),
		{ID: "plus", Kind: KindSystemButton, Label: "+", Side: Right, Surface: Front, Material: "small_btn",
			Button: device.ButtonMenu, Bound: true, Verify: Verified,
			Shapes: []Geometry{{Shape: ShapeCross, At: at(12, 21, 0), Size: sz(4.8, 1.8, 2.2)}}},
		button("home", "Home", KindSystemButton, Center, device.ButtonHome, true, disc(0, 14, 3.4, 2.4), "small_btn"),
		{ID: "profile", Kind: KindProfileButton, Label: "Profile switch button", Side: Center, Surface: Front, Material: "small_btn",
			Verify: Approximate, Note: "Switches between three custom profiles; exact position approximated.",
			Shapes: []Geometry{pill(0, 4, 5, 2.4, 2)}},
		{ID: "profile_leds", Kind: KindIndicator, Label: "Profile indicator", Side: Center, Surface: Front, Material: "led_white",
			Verify: Unverified, Shapes: []Geometry{disc(-3, 0.4, 0.7, 1.2), disc(0, 0.4, 0.7, 1.2), disc(3, 0.4, 0.7, 1.2)}},
		{ID: "paddle_left", Kind: KindRearPaddle, Label: "Left back paddle", Side: Left, Surface: Rear, Material: "paddle",
			Verify: Approximate, Note: "Two rear paddles are documented; the label used by 8BitDo's software is not verified.",
			Shapes: []Geometry{{Shape: ShapePaddle, At: at(-33, -30, 0.5), Size: sz(6, 15, 2.2), Rot: [3]float64{0, 0, -18}}}},
		{ID: "paddle_right", Kind: KindRearPaddle, Label: "Right back paddle", Side: Right, Surface: Rear, Material: "paddle",
			Verify: Approximate, Note: "Two rear paddles are documented; the label used by 8BitDo's software is not verified.",
			Shapes: []Geometry{{Shape: ShapePaddle, At: at(33, -30, 0.5), Size: sz(6, 15, 2.2), Rot: [3]float64{0, 0, 18}}}},
		rearControl("mode_switch", "Mode switch", KindHardwareSwitch, Center,
			Geometry{Shape: ShapeSlider, At: at(0, 8, 0), Size: sz(10, 4, 3)}, "switch_track", Unverified,
			"Connection-mode switch per product listings; its position is not verified."),
		port("usb", "USB-C port", Top, 0, 9, 4, KindPort, device.RegionNone, Verified, ""),
	}
	c = append(c, dpad(-21, -12, 17, 6, "dpad")...)
	c = append(c, nintendoFaceButtons(36, 10, 8.2, 3.9, [4]string{"btn", "btn", "btn", "btn"})...)
	c = append(c, shoulders(48, 25, 19, [4]string{"L", "R", "ZL", "ZR"}, true)...)
	c = append(c, grips(47, -34, 36, 56, 32, 5, [2]string{"Left grip (rumble motor)", "Right grip (rumble motor)"}, "grip", device.RegionHaptics)...)
	return ModelSpec{
		ID: "8bitdo_ultimate_bt", Manufacturer: "8BitDo", Name: "Ultimate Bluetooth Controller", Revision: "2022, with charging dock",
		Gallery: true, Dimensions: [3]float64{152, 60, 102}, DimensionsNote: "Unverified estimate.",
		Body: Body{Half: [][2]float64{
			{0, 35}, {22, 36}, {40, 35}, {55, 32}, {66, 26}, {73, 16}, {76, 3}, {75, -12}, {72, -28}, {66, -43},
			{58, -55}, {49, -60}, {41, -58}, {35, -48}, {30, -35}, {23, -26}, {11, -22}, {0, -21},
		}, Smooth: 3, Depth: 27, Bevel: 7, Material: "shell", ComponentID: "motion", SpineWidth: 92, SpineHeight: 12, SpineDepth: 10},
		Components: c,
		Materials: map[string]RGB{
			"shell": {220, 222, 228}, "grip": {198, 201, 208}, "small_btn": {70, 72, 80}, "led_white": {150, 155, 165},
			"paddle": {150, 154, 162}, "switch_track": {90, 92, 100}, "stick": {58, 60, 68}, "bumper": {184, 187, 195},
			"trigger": {176, 179, 187}, "dpad": {64, 66, 74}, "btn": {72, 74, 82}, "port": {30, 31, 36},
		},
		References:       []string{"https://www.8bitdo.com/ultimate-bluetooth-controller/"},
		HardwareModelIDs: []string{"8bitdo.ultimate_bluetooth"},
		Caveats: []string{
			"Exact outline, dimensions, rear-paddle naming and mode-switch position are not verified.",
			"A Capture/screenshot button may exist on this revision but is not modelled because it could not be verified.",
		},
	}
}

func gameSirG7SE() ModelSpec {
	c := []Component{
		stick(Left, "Left stick (LS)", -38, 12, 8, device.RegionLeftStick),
		stick(Right, "Right stick (RS)", 22, -11, 8, device.RegionRightStick),
		button("share", "Share", KindSystemButton, Center, device.ButtonShare, true, pill(0, 7, 5.5, 3, 2.4), "small_btn"),
		{ID: "m_button", Kind: KindProfileButton, Label: "M", Side: Left, Surface: Front, Material: "small_btn", Verify: Verified,
			Note:   "Positioned underneath the D-pad (GameSir manual); used for profiles, back-button mapping and audio shortcuts.",
			Shapes: []Geometry{pill(-22, -23.5, 4.2, 2.4, 2)}},
		{ID: "mute", Kind: KindAudioComponent, Label: "Mute button", Side: Center, Surface: Front, Material: "small_btn",
			Verify: Unverified, Note: "Documented button; its position is not stated in the manual.",
			Shapes: []Geometry{pill(0, -15.5, 5, 2.2, 2)}},
		{ID: "mute_led", Kind: KindIndicator, Label: "Mic mute indicator", Side: Center, Surface: Front, Material: "mic_led",
			Verify: Approximate, Note: "Red LED near the bottom of the controller (GameSir manual).",
			Shapes: []Geometry{disc(0, -19.5, 0.8, 1.2)}},
		{ID: "back_l4", Kind: KindRearButton, Label: "L4", Side: Left, Surface: Rear, Material: "paddle",
			Verify: Approximate, Note: "Lockable back button (GameSir manual); position approximated.",
			Shapes: []Geometry{{Shape: ShapePaddle, At: at(-34, -28, 0.5), Size: sz(6, 12, 2.4), Rot: [3]float64{0, 0, -15}}}},
		{ID: "back_r4", Kind: KindRearButton, Label: "R4", Side: Right, Surface: Rear, Material: "paddle",
			Verify: Approximate, Note: "Lockable back button (GameSir manual); position approximated.",
			Shapes: []Geometry{{Shape: ShapePaddle, At: at(34, -28, 0.5), Size: sz(6, 12, 2.4), Rot: [3]float64{0, 0, 15}}}},
		rearControl("latch_left", "Quick latch (left)", KindHardwareSwitch, Left,
			Geometry{Shape: ShapeSlider, At: at(-42, -10, 0), Size: sz(8, 4, 3)}, "switch_track", Unverified,
			"Enables/disables L4; documented, position not stated."),
		rearControl("latch_right", "Quick latch (right)", KindHardwareSwitch, Right,
			Geometry{Shape: ShapeSlider, At: at(42, -10, 0), Size: sz(8, 4, 3)}, "switch_track", Unverified,
			"Enables/disables R4; documented, position not stated."),
		port("usb", "USB-C port", Top, 0, 9, 4, KindPort, device.RegionNone, Approximate, "Position approximated."),
		port("headset_jack", "3.5 mm headset jack", Bottom, 0, 5, 4, KindAudioComponent, device.RegionNone, Approximate, "Position approximated."),
	}
	c = append(c, xboxCenter(23, 9, 12, device.RegionNone)...)
	c = append(c, dpad(-22, -11, 17, 6.2, "dpad")...)
	c = append(c, xboxFaceButtons(38, 12, 8.4, 4.1, [4]string{"btn", "btn", "btn", "btn"})...)
	c = append(c, shoulders(50, 26, 20, [4]string{"LB", "RB", "LT", "RT"}, true)...)
	c = append(c, grips(50, -36, 40, 62, 32, 6, [2]string{"Left grip (rumble motor)", "Right grip (rumble motor)"}, "grip", device.RegionHaptics)...)
	return ModelSpec{
		ID: "gamesir_g7se", Manufacturer: "GameSir", Name: "G7 SE Wired Controller", Revision: "Xbox / PC",
		Gallery: true, Dimensions: [3]float64{155, 62, 105}, DimensionsNote: "Unverified estimate.",
		Body: Body{Half: [][2]float64{
			{0, 39}, {20, 39}, {40, 39}, {56, 36}, {67, 30}, {74, 20}, {77, 6}, {77, -10}, {74, -28}, {68, -45},
			{60, -57}, {51, -63}, {42, -61}, {36, -51}, {31, -38}, {24, -28}, {12, -24}, {0, -23},
		}, Smooth: 2, Depth: 28, Bevel: 6, Material: "shell", SpineWidth: 96, SpineHeight: 13, SpineDepth: 10},
		Components: c,
		Materials: map[string]RGB{
			"shell": {232, 234, 238}, "grip": {210, 213, 219}, "light": {236, 238, 242}, "small_btn": {44, 46, 52},
			"mic_led": {120, 60, 60}, "paddle": {60, 62, 70}, "switch_track": {70, 72, 80}, "stick": {40, 42, 48},
			"bumper": {200, 203, 210}, "trigger": {190, 193, 200}, "dpad": {48, 50, 56}, "btn": {44, 46, 52}, "port": {24, 25, 30},
		},
		References: []string{
			"https://gamesir.com/support/manuals/gamesir-g7-se",
			"https://gamesir.com/pages/tutorial-how-to-use-gamesir-g7-se",
		},
		HardwareModelIDs: []string{"gamesir.g7_se"},
		Caveats: []string{
			"Mute button, quick latch and port positions are not stated by GameSir and are approximated.",
		},
	}
}

func razerWolverineV3Pro() ModelSpec {
	back := func(id, label string, side Side, x, y float64) Component {
		return Component{ID: id, Kind: KindRearButton, Label: label, Side: side, Surface: Rear, Material: "back_button",
			Verify: Approximate, Note: "One of four mouse-click back buttons (Razer); Razer's M-numbering is not verified.",
			Shapes: []Geometry{pill(x, y, 6, 10, 2.6)}}
	}
	claw := func(id, label string, kind ComponentKind, side Side, x float64) Component {
		return Component{ID: id, Kind: kind, Label: label, Side: side, Surface: Top, Material: "bumper",
			Verify: Approximate, Note: "Claw grip bumper next to LB/RB (Razer); position approximated, M-numbering not verified.",
			Shapes: []Geometry{{Shape: ShapeBumper, At: at(x, 0, -4), Size: sz(12, 4, 9)}}}
	}
	c := []Component{
		stick(Left, "Left stick (LS)", -40, 13, 8.2, device.RegionLeftStick),
		stick(Right, "Right stick (RS)", 23, -13, 8.2, device.RegionRightStick),
		decor("dpad_disc", "dpad_disc", disc(-23, -13, 11, 1.4)),
		button("share", "Share", KindSystemButton, Center, device.ButtonShare, true, pill(0, 8, 5.5, 3, 2.4), "small_btn"),
		claw("claw_left", "Left claw grip bumper", KindExtraBumper, Left, -26),
		claw("claw_right", "Right claw grip bumper", KindExtraBumper, Right, 26),
		back("back_upper_left", "Upper left back button", Left, -30, -20),
		back("back_lower_left", "Lower left back button", Left, -40, -38),
		back("back_upper_right", "Upper right back button", Right, 30, -20),
		back("back_lower_right", "Lower right back button", Right, 40, -38),
		rearControl("hypertrigger_left", "HyperTrigger switch (left)", KindHardwareSwitch, Left,
			Geometry{Shape: ShapeSlider, At: at(-46, 12, 0), Size: sz(10, 4.5, 3.5)}, "switch_track", Approximate,
			"Slide-lock switching LT between analog and instant actuation; not reported to the host."),
		rearControl("hypertrigger_right", "HyperTrigger switch (right)", KindHardwareSwitch, Right,
			Geometry{Shape: ShapeSlider, At: at(46, 12, 0), Size: sz(10, 4.5, 3.5)}, "switch_track", Approximate,
			"Slide-lock switching RT between analog and instant actuation; not reported to the host."),
		port("usb", "USB-C port", Top, 0, 9, 4, KindPort, device.RegionNone, Approximate, "Position approximated."),
	}
	c = append(c, xboxCenter(25, 10, 13, device.RegionNone)...)
	c = append(c, dpad(-23, -13, 19, 7, "dpad")...)
	c = append(c, xboxFaceButtons(40, 13, 8.8, 4.2, [4]string{"btn", "btn", "btn", "btn"})...)
	c = append(c, shoulders(53, 24, 21, [4]string{"LB", "RB", "LT", "RT"}, true)...)
	c = append(c, grips(52, -38, 42, 66, 36, 7, [2]string{"Left grip (rumble motor)", "Right grip (rumble motor)"}, "rubber", device.RegionHaptics)...)
	return ModelSpec{
		ID: "razer_wolverine_v3pro", Manufacturer: "Razer", Name: "Wolverine V3 Pro", Revision: "Xbox / PC, wireless",
		Gallery: true, Dimensions: [3]float64{160, 66, 108}, DimensionsNote: "Unverified estimate.",
		Body: Body{Half: [][2]float64{
			{0, 41}, {22, 42}, {42, 41}, {58, 38}, {70, 31}, {77, 21}, {80, 6}, {80, -10}, {77, -29}, {71, -47},
			{63, -60}, {53, -67}, {43, -66}, {36, -55}, {31, -41}, {24, -30}, {12, -26}, {0, -25},
		}, Smooth: 2, Depth: 30, Bevel: 6, Material: "shell", SpineWidth: 104, SpineHeight: 14, SpineDepth: 11},
		Components: c,
		Materials: map[string]RGB{
			"shell": {42, 44, 49}, "rubber": {24, 25, 28}, "light": {226, 230, 236}, "small_btn": {26, 27, 31},
			"back_button": {112, 116, 124}, "switch_track": {20, 21, 24}, "stick": {30, 31, 35}, "bumper": {36, 37, 42},
			"trigger": {40, 41, 46}, "dpad": {50, 52, 58}, "dpad_disc": {30, 31, 35}, "btn": {34, 35, 40}, "port": {14, 15, 18},
		},
		References: []string{
			"https://www.razer.com/console-controllers/razer-wolverine-v3-pro",
			"https://www.ifixit.com/Device/Razer_Wolverine_V3_Pro",
		},
		HardwareModelIDs: []string{"razer.wolverine_v3_pro"},
		Caveats: []string{
			"Razer documents four back buttons and two claw grip bumpers; the M1–M6 assignment differs between sources, so neutral positional names are used.",
			"Razer's shortcuts imply an additional function button whose location could not be verified; it is not modelled.",
			"Offset (Xbox-style) stick layout and dimensions are approximated from product photos.",
		},
	}
}

func msiGC20V2() ModelSpec {
	c := []Component{
		stick(Left, "Left stick (LS)", -38, 12, 8, device.RegionLeftStick),
		stick(Right, "Right stick (RS)", 22, -13, 8, device.RegionRightStick),
		decor("dpad_cover", "dpad_cover", disc(-22, -13, 10.5, 1.6)),
		{ID: "emblem", Kind: KindSystemButton, Label: "Centre emblem button", Side: Center, Surface: Front, Material: "emblem",
			Button: device.ButtonHome, Bound: true, Verify: Unverified,
			Note:   "Bound to the guide/mode event Linux reports (BTN_MODE); the physical correspondence is not verified.",
			Shapes: []Geometry{disc(0, 24, 6, 3)}},
		button("back", "Back", KindSystemButton, Left, device.ButtonView, true, pill(-12, 14, 5, 2.6, 2.4), "small_btn"),
		button("start", "Start", KindSystemButton, Right, device.ButtonMenu, true, pill(12, 14, 5, 2.6, 2.4), "small_btn"),
		{ID: "mode", Kind: KindHardwareSwitch, Label: "Mode button", Side: Center, Surface: Front, Material: "small_btn",
			Verify: Verified, Note: "Switches the input mode (MSI manual); handled by the controller, not reported to the host.",
			Shapes: []Geometry{disc(0, 11, 2.4, 2.4)}},
		{ID: "mode_leds", Kind: KindIndicator, Label: "Mode indicator LEDs", Side: Center, Surface: Front, Material: "light",
			Verify: Verified, Note: "Four LEDs show the input mode (MSI manual); the host cannot read which are lit.",
			Shapes: []Geometry{disc(-6.3, 4, 0.9, 1.4), disc(-2.1, 4, 0.9, 1.4), disc(2.1, 4, 0.9, 1.4), disc(6.3, 4, 0.9, 1.4)}},
		decor("trim", "trim",
			Geometry{Shape: ShapeBox, At: at(-60, -30, 0), Size: sz(2.2, 32, 1), Rot: [3]float64{0, 0, 22}},
			Geometry{Shape: ShapeBox, At: at(60, -30, 0), Size: sz(2.2, 32, 1), Rot: [3]float64{0, 0, -22}}),
		port("usb", "USB port", Top, 0, 8, 3, KindPort, device.RegionNone, Approximate, "Wired USB 2.0 (MSI datasheet); connector type not verified."),
	}
	c = append(c, dpad(-22, -13, 18, 6.5, "dpad")...)
	c = append(c, xboxFaceButtons(38, 12, 8.4, 4.1, [4]string{"btn_y", "btn_b", "btn_a", "btn_x"})...)
	c = append(c, shoulders(50, 26, 20, [4]string{"LB", "RB", "LT", "RT"}, true)...)
	c = append(c, grips(50, -36, 40, 62, 34, 6, [2]string{"Left grip (vibration motor)", "Right grip (vibration motor)"}, "grip", device.RegionHaptics)...)
	return ModelSpec{
		ID: "msi_gc20", Manufacturer: "MSI", Name: "Force GC20 V2", Revision: "Wired, PC / Android",
		Gallery: true, Dimensions: [3]float64{156, 62.5, 105}, DimensionsNote: "MSI datasheet: 156 × 105 × 62.5 mm.",
		Body: Body{Half: [][2]float64{
			{0, 38}, {24, 38}, {44, 37}, {58, 34}, {68, 28}, {75, 18}, {78, 4}, {78, -12}, {75, -30}, {69, -46},
			{61, -58}, {51, -64}, {42, -62}, {36, -51}, {31, -38}, {24, -28}, {12, -24}, {0, -23},
		}, Smooth: 1, Depth: 28, Bevel: 5, Material: "shell", SpineWidth: 96, SpineHeight: 13, SpineDepth: 10},
		Components: c,
		Materials: map[string]RGB{
			"shell": {44, 46, 52}, "grip": {30, 32, 36}, "trim": {196, 40, 44}, "emblem": {210, 44, 48},
			"small_btn": {92, 96, 106}, "light": {230, 70, 60}, "stick": {50, 52, 58}, "bumper": {70, 72, 80},
			"trigger": {84, 86, 94}, "dpad": {86, 90, 98}, "dpad_cover": {60, 62, 70},
			"btn_a": {70, 150, 80}, "btn_b": {180, 64, 64}, "btn_x": {64, 110, 190}, "btn_y": {190, 160, 50}, "port": {20, 22, 26},
		},
		HardwareModelIDs: []string{"msi.force_gc20_v2"},
		References: []string{
			"https://www.msi.com/Gaming-Gear/FORCE-GC20-V2",
			"https://storage-asset.msi.com/datasheet/gd/au/FORCE-GC20-V2-WHITE.pdf",
			"https://download.msi.com/archive/mnu_exe/gear/GC30%2BGC20_manual.pdf",
		},
		Caveats: []string{
			"Two magnetic D-pad covers are included (MSI datasheet); the circular cover is shown.",
			"Red trim accents and the exact outline are stylised.",
		},
	}
}

func genericGamepad() ModelSpec {
	c := []Component{
		stick(Left, "Left stick", -22, -12, 8, device.RegionLeftStick),
		stick(Right, "Right stick", 22, -12, 8, device.RegionRightStick),
		button("select", "Select / Back", KindSystemButton, Left, device.ButtonView, true, pill(-10, 14, 5, 2.4, 2.2), "small_btn"),
		button("start", "Start", KindSystemButton, Right, device.ButtonMenu, true, pill(10, 14, 5, 2.4, 2.2), "small_btn"),
		button("home", "Home / Guide", KindSystemButton, Center, device.ButtonHome, true, disc(0, 4, 3.4, 2.4), "small_btn"),
	}
	c = append(c, dpad(-44, 8, 18, 6.5, "dpad")...)
	c = append(c, faceButtons(44, 8, 8.4, 4, [4]string{"btn_north", "btn_east", "btn_south", "btn_west"},
		[4]string{"North button", "East button", "South button", "West button"}, [4]string{"btn", "btn", "btn", "btn"})...)
	c = append(c, shoulders(46, 24, 19, [4]string{"Left bumper", "Right bumper", "Left trigger", "Right trigger"}, true)...)
	c = append(c, grips(46, -32, 34, 52, 30, 5, [2]string{"Left grip", "Right grip"}, "shell", device.RegionHaptics)...)
	return ModelSpec{
		ID: GenericID, Manufacturer: "Generic", Name: "Gamepad", Revision: "Neutral fallback",
		Body: Body{Half: [][2]float64{
			{0, 34}, {30, 34}, {50, 32}, {63, 26}, {70, 14}, {72, 0}, {70, -18}, {64, -36}, {55, -48}, {45, -50},
			{38, -42}, {32, -30}, {20, -24}, {0, -22},
		}, Smooth: 2, Depth: 26, Bevel: 6, Material: "shell", SpineWidth: 90, SpineHeight: 12, SpineDepth: 9},
		Components: c,
		Materials: map[string]RGB{
			"shell": {120, 128, 146}, "small_btn": {60, 64, 74}, "stick": {58, 62, 72}, "bumper": {84, 90, 104},
			"trigger": {90, 96, 110}, "dpad": {70, 74, 86}, "btn": {76, 80, 92},
		},
		Caveats: []string{"Neutral shape for unidentified gamepads; it does not represent a specific product."},
	}
}
