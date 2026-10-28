package visual

import "github.com/Phobby/TuiSense/internal/device"

// KindBattery is a model-specific kind registered by the Xbox models.
const KindBattery ComponentKind = "battery_compartment"

func init() {
	RegisterKind(KindBattery, "Battery compartment")
	Register(xboxSeries())
	Register(xboxElite2())
}

// xboxCenter is the guide / View / Menu cluster shared by Xbox-layout pads.
func xboxCenter(guideY, smallY, smallDX float64, guideRegion device.RegionID) []Component {
	return []Component{
		{ID: "xbox_button", Kind: KindSystemButton, Label: "Xbox button", Side: Center, Surface: Front, Material: "light",
			Button: device.ButtonHome, Bound: true, Region: guideRegion, Verify: Verified, Note: "Illuminated guide button.",
			Shapes: []Geometry{disc(0, guideY, 6.2, 3)}},
		button("view", "View", KindSystemButton, Left, device.ButtonView, true, disc(-smallDX, smallY, 2.8, 2.6), "small_btn"),
		button("menu", "Menu", KindSystemButton, Right, device.ButtonMenu, true, disc(smallDX, smallY, 2.8, 2.6), "small_btn"),
	}
}

var xboxSeriesHalf = [][2]float64{
	{0, 38}, {20, 39}, {38, 38}, {54, 35}, {66, 29}, {73, 19}, {76, 6}, {76, -8}, {73, -25}, {68, -42},
	{61, -55}, {52, -63}, {43, -63}, {36, -54}, {31, -40}, {24, -28}, {12, -23}, {0, -22},
}

func xboxSeries() ModelSpec {
	c := []Component{
		stick(Left, "Left stick (LS)", -38, 12, 8, device.RegionLeftStick),
		stick(Right, "Right stick (RS)", 22, -13, 8, device.RegionRightStick),
		decor("dpad_dish", "dpad_dish", disc(-22, -13, 10.5, 1.4)),
		button("share", "Share", KindSystemButton, Center, device.ButtonShare, true, pill(0, 7, 5.5, 3, 2.4), "small_btn"),
		{ID: "pair", Kind: KindSystemButton, Label: "Pair button", Side: Center, Surface: Top, Material: "small_btn", Verify: Verified,
			Note: "Wireless pairing; not reported as a gamepad input.", Shapes: []Geometry{{Shape: ShapeBox, At: at(-10, -1, 0), Size: sz(5, 3, 3)}}},
		port("usb", "USB-C port", Top, 0, 9, 4, KindPort, device.RegionNone, Verified, ""),
		port("headset_jack", "3.5 mm headset jack", Bottom, 0, 5, 4, KindAudioComponent, device.RegionAudio, Verified, ""),
		{ID: "battery", Kind: KindBattery, Label: "Battery compartment (2× AA)", Side: Center, Surface: Rear, Material: "door",
			Verify: Verified, Note: "Removable door for AA batteries or a rechargeable pack.",
			Shapes: []Geometry{{Shape: ShapePad, At: at(0, -6, -0.2), Size: sz(40, 22, 1.4)}}},
	}
	c = append(c, xboxCenter(23, 9, 12, device.RegionLight)...)
	c = append(c, dpad(-22, -13, 18, 6.5, "dpad")...)
	c = append(c, xboxFaceButtons(38, 12, 8.4, 4.1, [4]string{"btn_y", "btn_b", "btn_a", "btn_x"})...)
	c = append(c, shoulders(50, 26, 20, [4]string{"LB", "RB", "LT", "RT"}, true)...)
	c = append(c, grips(50, -36, 40, 62, 32, 6, [2]string{"Left grip (rumble motor)", "Right grip (rumble motor)"}, "grip", device.RegionHaptics)...)
	return ModelSpec{
		ID: "xbox_series", Manufacturer: "Microsoft", Name: "Xbox Wireless Controller", Revision: "Model 1914 (Series X|S)",
		Gallery: true, Dimensions: [3]float64{154, 61, 104}, DimensionsNote: "Approximate.",
		Body:       Body{Half: xboxSeriesHalf, Smooth: 3, Depth: 28, Bevel: 7, Material: "shell", SpineWidth: 96, SpineHeight: 13, SpineDepth: 10},
		Components: c,
		Materials: map[string]RGB{
			"shell": {44, 46, 52}, "grip": {36, 38, 43}, "light": {232, 236, 242}, "small_btn": {30, 32, 36},
			"stick": {34, 36, 41}, "bumper": {38, 40, 46}, "trigger": {42, 44, 50}, "dpad": {52, 54, 60}, "dpad_dish": {32, 34, 39},
			"btn_a": {50, 98, 62}, "btn_b": {118, 54, 56}, "btn_x": {54, 78, 124}, "btn_y": {128, 112, 50},
			"port": {16, 17, 20}, "door": {40, 42, 48},
		},
		References: []string{
			"https://www.xbox.com/en-US/accessories/controllers/xbox-wireless-controller",
			"https://support.xbox.com/en-US/help/hardware-network/controller/get-to-know-your-xbox-series-x-s-controller",
		},
		HardwareModelIDs: []string{"microsoft.xbox_wireless_1914"},
	}
}

func xboxElite2() ModelSpec {
	paddle := func(id, label string, side Side, btn device.ButtonID, x, y, h, tilt float64, note string) Component {
		return Component{ID: id, Kind: KindRearPaddle, Label: label, Side: side, Surface: Rear, Material: "metal",
			Button: btn, Bound: true, Region: device.RegionNone, Verify: Approximate, Note: note,
			Shapes: []Geometry{{Shape: ShapePaddle, At: at(x, y, 0.5), Size: sz(6.5, h, 2.2), Rot: [3]float64{0, 0, tilt}}}}
	}
	const src = " Microsoft numbering; positions follow the community-confirmed SDL mapping, not an official diagram."
	c := []Component{
		stick(Left, "Left stick (LS)", -38, 12, 8, device.RegionLeftStick),
		stick(Right, "Right stick (RS)", 22, -13, 8, device.RegionRightStick),
		decor("dpad_dish", "dish", Geometry{Shape: ShapeDisc, At: at(-22, -13, 0), Size: sz(10.5, 0, 1.8), Segments: 8}),
		{ID: "profile", Kind: KindProfileButton, Label: "Profile button", Side: Center, Surface: Front, Material: "small_btn",
			Verify: Verified, Note: "Cycles the three on-board profiles; not reported as a standard gamepad input.",
			Shapes: []Geometry{disc(0, 7, 2.6, 2.4)}},
		{ID: "profile_leds", Kind: KindIndicator, Label: "Profile indicator", Side: Center, Surface: Front, Material: "led_white",
			Verify: Verified, Shapes: []Geometry{disc(-3.6, 2.4, 0.8, 1.2), disc(0, 2.0, 0.8, 1.2), disc(3.6, 2.4, 0.8, 1.2)}},
		{ID: "pair", Kind: KindSystemButton, Label: "Pair button", Side: Center, Surface: Top, Material: "small_btn", Verify: Verified,
			Shapes: []Geometry{{Shape: ShapeBox, At: at(-10, -1, 0), Size: sz(5, 3, 3)}}},
		port("usb", "USB-C port", Top, 0, 9, 4, KindPort, device.RegionNone, Verified, ""),
		port("headset_jack", "3.5 mm headset jack", Bottom, 0, 5, 4, KindAudioComponent, device.RegionNone, Verified, ""),
		paddle("paddle_p1", "P1", Right, device.ButtonPaddle1, 29, -16, 17, 14, "Upper right paddle."+src),
		paddle("paddle_p2", "P2", Right, device.ButtonPaddle2, 40, -36, 22, 24, "Lower right paddle."+src),
		paddle("paddle_p3", "P3", Left, device.ButtonPaddle3, -29, -16, 17, -14, "Upper left paddle."+src),
		paddle("paddle_p4", "P4", Left, device.ButtonPaddle4, -40, -36, 22, -24, "Lower left paddle."+src),
		rearControl("hair_trigger_left", "Hair trigger lock (left)", KindHardwareSwitch, Left,
			Geometry{Shape: ShapeSlider, At: at(-45, 12, 0), Size: sz(10, 4.5, 3.5)}, "switch_track", Verified,
			"Three-position travel limiter for LT; not reported to the host."),
		rearControl("hair_trigger_right", "Hair trigger lock (right)", KindHardwareSwitch, Right,
			Geometry{Shape: ShapeSlider, At: at(45, 12, 0), Size: sz(10, 4.5, 3.5)}, "switch_track", Verified,
			"Three-position travel limiter for RT; not reported to the host."),
	}
	c = append(c, xboxCenter(24, 12, 12, device.RegionNone)...)
	c = append(c, dpad(-22, -13, 18, 6.5, "metal")...)
	c = append(c, xboxFaceButtons(38, 12, 8.4, 4.1, [4]string{"btn_y", "btn_b", "btn_a", "btn_x"})...)
	c = append(c, shoulders(50, 27, 21, [4]string{"LB", "RB", "LT", "RT"}, true)...)
	c = append(c, grips(50, -36, 40, 64, 36, 7, [2]string{"Left grip (rumble motor)", "Right grip (rumble motor)"}, "rubber", device.RegionHaptics)...)
	return ModelSpec{
		ID: "xbox_elite2", Manufacturer: "Microsoft", Name: "Xbox Elite Wireless Controller Series 2", Revision: "Model 1797",
		Gallery: true, Dimensions: [3]float64{153, 66, 104}, DimensionsNote: "Approximate.",
		Body: Body{Half: [][2]float64{
			{0, 39}, {22, 40}, {40, 39}, {56, 36}, {67, 30}, {74, 20}, {77, 6}, {77, -9}, {74, -27}, {69, -44},
			{62, -57}, {53, -65}, {43, -64}, {36, -55}, {31, -41}, {24, -29}, {12, -24}, {0, -23},
		}, Smooth: 2, Depth: 30, Bevel: 6, Material: "shell", SpineWidth: 98, SpineHeight: 14, SpineDepth: 11},
		Components: c,
		Materials: map[string]RGB{
			"shell": {30, 31, 36}, "rubber": {14, 15, 18}, "metal": {150, 154, 164}, "dish": {84, 88, 96}, "light": {232, 236, 242},
			"small_btn": {26, 27, 31}, "led_white": {150, 155, 165}, "stick": {30, 31, 36}, "bumper": {34, 35, 40},
			"trigger": {38, 39, 45}, "btn_a": {40, 44, 48}, "btn_b": {40, 44, 48}, "btn_x": {40, 44, 48}, "btn_y": {40, 44, 48},
			"port": {14, 15, 18}, "switch_track": {20, 21, 25},
		},
		References: []string{
			"https://www.xbox.com/en-US/accessories/controllers/elite-wireless-controller-series-2",
			"https://support.xbox.com/en-US/help/hardware-network/accessories/xbox-elite-wireless-controller-series-2",
			"https://discourse.libsdl.org/t/sdl-clarified-where-the-paddle-locations-are/40970",
		},
		Caveats: []string{
			"Paddle positions and angles are approximate; P1–P4 numbering follows Microsoft, positions follow SDL's community-confirmed mapping.",
			"Swappable D-pad (faceted dish shown) and adjustable-tension sticks are not modelled as separate parts.",
			"Paddles light up only when the Linux driver reports them individually (xpad on Linux 6.1+ over USB, or the Bluetooth HID-BPF fix).",
		},
		HardwareModelIDs: []string{"microsoft.xbox_elite_series2"},
	}
}
