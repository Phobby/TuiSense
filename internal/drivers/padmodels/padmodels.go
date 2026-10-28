// Package padmodels identifies and describes controller models that Linux
// already serves through its own kernel drivers (xpad, hid-playstation,
// hid-sony, hid-nintendo, hid-microsoft, xpadneo). TuiSense reads them
// through the generic evdev gamepad backend: input, kernel force-feedback
// rumble and kernel power_supply battery. No vendor-specific output report or
// configuration command is ever sent to these controllers.
//
// Every model declares its physical hardware so that the UI shows exactly
// what the controller has, and nothing it does not; features the kernel or
// TuiSense cannot reach are reported (with the reason) only in Driver &
// Compatibility. Identification uses documented vendor:product IDs where they
// are known, and otherwise device-name evidence marked as such.
package padmodels

import (
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// Model IDs.
const (
	XboxSeriesID  = "microsoft.xbox_wireless_1914"
	XboxElite2ID  = "microsoft.xbox_elite_series2"
	DualShock4ID  = "sony.dualshock4"
	SwitchProID   = "nintendo.switch_pro"
	EightBitDoID  = "8bitdo.ultimate_bluetooth"
	GameSirG7SEID = "gamesir.g7_se"
	WolverineID   = "razer.wolverine_v3_pro"
)

// Kernel key codes for xpad's Elite paddles (BTN_TRIGGER_HAPPY5…8, Linux ≥ 6.1).
const (
	btnTriggerHappy5 uint16 = 0x2c4
	btnTriggerHappy6 uint16 = 0x2c5
	btnTriggerHappy7 uint16 = 0x2c6
	btnTriggerHappy8 uint16 = 0x2c7
)

var xboxLabels = map[device.ButtonID]string{
	device.ButtonSouth: "A", device.ButtonEast: "B", device.ButtonWest: "X", device.ButtonNorth: "Y",
	device.ButtonL1: "LB", device.ButtonR1: "RB", device.ButtonL2Digital: "LT", device.ButtonR2Digital: "RT",
	device.ButtonView: "View", device.ButtonMenu: "Menu", device.ButtonHome: "Xbox",
	device.ButtonL3: "LS (click)", device.ButtonR3: "RS (click)",
}

var playstationLabels = map[device.ButtonID]string{
	device.ButtonSouth: "✕ Cross", device.ButtonEast: "○ Circle", device.ButtonWest: "□ Square", device.ButtonNorth: "△ Triangle",
	device.ButtonL1: "L1", device.ButtonR1: "R1", device.ButtonL2Digital: "L2 (digital)", device.ButtonR2Digital: "R2 (digital)",
	device.ButtonView: "Share", device.ButtonMenu: "Options", device.ButtonHome: "PS",
	device.ButtonL3: "L3", device.ButtonR3: "R3",
}

// Nintendo layout: the east button is A and the south button is B.
var nintendoLabels = map[device.ButtonID]string{
	device.ButtonSouth: "B", device.ButtonEast: "A", device.ButtonWest: "Y", device.ButtonNorth: "X",
	device.ButtonL1: "L", device.ButtonR1: "R", device.ButtonL2Digital: "ZL", device.ButtonR2Digital: "ZR",
	device.ButtonView: "−", device.ButtonMenu: "+", device.ButtonHome: "HOME",
	device.ButtonL3: "L-stick (click)", device.ButtonR3: "R-stick (click)",
}

const xboxUpdater = "Firmware is updated with Microsoft's Xbox Accessories app (Windows or Xbox console); TuiSense never installs firmware."

// XboxSeries is the Xbox Wireless Controller (model 1914).
var XboxSeries = gamepad.ModelSpec{
	ID: XboxSeriesID, Family: "xbox", Brand: "Microsoft", DisplayName: "Xbox Wireless Controller", ShortName: "Xbox Wireless",
	VisualID: "xbox_series", Accent: device.RGB{R: 60, G: 190, B: 90},
	IDs: []gamepad.DeviceID{
		{Vendor: 0x045e, Product: 0x0b12, Transport: "usb", Source: "Linux xpad device table"},
		{Vendor: 0x045e, Product: 0x0b13, Transport: "bluetooth", Source: "Linux hid-ids.h"},
	},
	Labels: xboxLabels, TriggerLabels: [2]string{"LT", "RT"},
	// Button remapping exists (Xbox Accessories app, stored on the controller).
	Hardware: gamepad.Hardware{Battery: true, Rumble: true, TriggerRumble: true, Audio: true, VendorRemapping: true, FirmwareUpdater: xboxUpdater},
	ModeGuide: []string{
		"USB: Linux xpad driver (input and rumble)",
		"Bluetooth: Linux hid-microsoft driver; rumble and battery depend on the kernel version (xpadneo is an optional out-of-tree alternative)",
		"Xbox Wireless Adapter: requires the out-of-tree xone driver",
	},
	KernelDrivers: []string{"xpad", "microsoft", "xpadneo", "xone"},
}

// XboxElite2 is the Xbox Elite Wireless Controller Series 2 (model 1797).
var XboxElite2 = gamepad.ModelSpec{
	ID: XboxElite2ID, Family: "xbox_elite", Brand: "Microsoft", DisplayName: "Xbox Elite Wireless Controller Series 2", ShortName: "Elite Series 2",
	VisualID: "xbox_elite2", Accent: device.RGB{R: 150, G: 156, B: 166},
	IDs: []gamepad.DeviceID{
		{Vendor: 0x045e, Product: 0x0b00, Transport: "usb", Source: "Linux xpad device table"},
		{Vendor: 0x045e, Product: 0x0b05, Transport: "bluetooth", Source: "Linux hid-ids.h"},
		{Vendor: 0x045e, Product: 0x0b22, Transport: "bluetooth", Source: "Linux HID-BPF Elite 2 fix (newer firmware)"},
	},
	Labels: xboxLabels, TriggerLabels: [2]string{"LT", "RT"},
	Hardware: gamepad.Hardware{Battery: true, Rumble: true, TriggerRumble: true, Audio: true, RearControls: true,
		OnboardProfiles: true, VendorRemapping: true, FirmwareUpdater: xboxUpdater,
		// xpad (Linux 6.1+) and the Bluetooth HID-BPF fix report the four
		// paddles as BTN_TRIGGER_HAPPY5…8, in this order.
		RearInputs: []gamepad.ButtonMap{
			{Code: btnTriggerHappy5, ID: device.ButtonPaddle1, Label: "P1", Group: "Rear"},
			{Code: btnTriggerHappy6, ID: device.ButtonPaddle2, Label: "P2", Group: "Rear"},
			{Code: btnTriggerHappy7, ID: device.ButtonPaddle3, Label: "P3", Group: "Rear"},
			{Code: btnTriggerHappy8, ID: device.ButtonPaddle4, Label: "P4", Group: "Rear"},
		}},
	ModeGuide: []string{
		"USB: Linux xpad reports the four paddles individually since Linux 6.1 (firmware 5.11+ needs the extra packet xpad enables)",
		"Bluetooth: paddles are reported only with the kernel's HID-BPF fix for this controller",
		"While an on-board profile maps the paddles to other buttons, they may not be reported separately",
	},
	KernelDrivers: []string{"xpad", "microsoft", "xpadneo"},
}

// DualShock4 is the Sony DualShock 4 (CUH-ZCT1 and CUH-ZCT2).
var DualShock4 = gamepad.ModelSpec{
	ID: DualShock4ID, Family: "dualshock4", Brand: "Sony", DisplayName: "DualShock 4 Wireless Controller", ShortName: "DualShock 4",
	VisualID: "dualshock4", Accent: device.RGB{R: 70, G: 110, B: 230},
	IDs: []gamepad.DeviceID{
		{Vendor: 0x054c, Product: 0x05c4, Transport: "usb", Source: "Linux hid-ids.h (CUH-ZCT1)"},
		{Vendor: 0x054c, Product: 0x09cc, Transport: "usb", Source: "Linux hid-ids.h (CUH-ZCT2)"},
		{Vendor: 0x054c, Product: 0x0ba0, Transport: "dongle", Source: "Linux hid-ids.h (Sony wireless adapter)"},
	},
	Labels: playstationLabels, TriggerLabels: [2]string{"L2", "R2"},
	Hardware: gamepad.Hardware{Battery: true, Rumble: true, Motion: true, Touchpad: true, Lighting: true, Audio: true,
		AudioUSBIDs:   []string{"054c:05c4", "054c:09cc"},
		AudioFeatures: []device.Feature{device.FeatSpeaker, device.FeatHeadset, device.FeatMicrophone}},
	ModeGuide: []string{
		"Linux hid-playstation (6.2+) or hid-sony serves input, rumble, battery, light bar LEDs and a separate motion/touchpad node",
		"USB: the controller is also a USB audio device (speaker, headset jack, microphone)",
	},
	KernelDrivers: []string{"playstation", "sony"},
}

// SwitchPro is the Nintendo Switch Pro Controller (HAC-013).
var SwitchPro = gamepad.ModelSpec{
	ID: SwitchProID, Family: "switch_pro", Brand: "Nintendo", DisplayName: "Nintendo Switch Pro Controller", ShortName: "Switch Pro",
	VisualID: "switch_pro", Accent: device.RGB{R: 220, G: 60, B: 60},
	IDs: []gamepad.DeviceID{
		{Vendor: 0x057e, Product: 0x2009, Transport: "usb", Source: "Linux hid-ids.h (Pro Controller)"},
	},
	Labels: nintendoLabels, TriggerLabels: [2]string{"ZL", "ZR"},
	Hardware: gamepad.Hardware{Battery: true, Rumble: true, Motion: true, PlayerLEDs: true,
		FirmwareUpdater: "Firmware is updated from a Nintendo Switch console (System Settings → Controllers and Sensors)."},
	ModeGuide: []string{
		"Linux hid-nintendo serves input, HD rumble (as force feedback), battery, player LEDs and a separate IMU node",
		"Some third-party pads emulate this USB ID in Switch mode and are indistinguishable by ID",
	},
	KernelDrivers: []string{"nintendo"},
}

// EightBitDoUltimate is the 8BitDo Ultimate Bluetooth Controller. 8BitDo's
// IDs for this exact revision could not be verified, so a name match is
// only probable and the user can confirm it.
var EightBitDoUltimate = gamepad.ModelSpec{
	ID: EightBitDoID, Family: "8bitdo_ultimate", Brand: "8BitDo", DisplayName: "8BitDo Ultimate Bluetooth Controller", ShortName: "8BitDo Ultimate",
	VisualID: "8bitdo_ultimate_bt", Accent: device.RGB{R: 200, G: 200, B: 210},
	Strings:  [][]string{{"8bitdo", "ultimate"}},
	Probable: true,
	Exclude: func(d evdev.InputDevice) bool {
		return containsAny(d, "2c", "2.4g", "ultimate 2 ") // other Ultimate products
	},
	Labels: nintendoLabels, TriggerLabels: [2]string{"ZL", "ZR"},
	Hardware: gamepad.Hardware{Battery: true, Rumble: true, Motion: true, RearControls: true, OnboardProfiles: true, VendorRemapping: true,
		FirmwareUpdater: "Firmware is updated with 8BitDo's official Firmware Updater / Ultimate Software."},
	ModeGuide: []string{
		"Bluetooth mode targets the Nintendo Switch and may appear as a Switch Pro Controller",
		"X-input over USB or the 2.4 GHz receiver is the PC mode; rear paddles are mapped by the controller itself",
	},
	KernelDrivers: []string{"xpad", "nintendo", "hid-generic"},
}

// GameSirG7SE is the GameSir G7 SE wired Xbox controller. Its USB product ID
// could not be verified; the device name identifies it.
var GameSirG7SE = gamepad.ModelSpec{
	ID: GameSirG7SEID, Family: "gamesir_g7se", Brand: "GameSir", DisplayName: "GameSir G7 SE", ShortName: "G7 SE",
	VisualID: "gamesir_g7se", Accent: device.RGB{R: 230, G: 230, B: 235},
	Strings: [][]string{{"g7 se"}},
	Labels:  xboxLabels, TriggerLabels: [2]string{"LT", "RT"},
	Hardware: gamepad.Hardware{Rumble: true, Audio: true, RearControls: true, OnboardProfiles: true, VendorRemapping: true,
		FirmwareUpdater: "Firmware is updated with the GameSir Nexus app."},
	ModeGuide: []string{
		"Wired USB-C, Xbox protocol: served by Linux xpad when the kernel knows GameSir's IDs",
		"L4/R4 are assigned by the controller (M + L4/R4) and are not reported as separate inputs",
	},
	KernelDrivers: []string{"xpad"},
}

// WolverineV3Pro is the Razer Wolverine V3 Pro.
var WolverineV3Pro = gamepad.ModelSpec{
	ID: WolverineID, Family: "razer_wolverine_v3", Brand: "Razer", DisplayName: "Razer Wolverine V3 Pro", ShortName: "Wolverine V3 Pro",
	VisualID: "razer_wolverine_v3pro", Accent: device.RGB{R: 68, G: 214, B: 44},
	IDs: []gamepad.DeviceID{
		{Vendor: 0x1532, Product: 0x0a57, Transport: "usb", Source: "Linux xpad patch (Linux 7.0)"},
		{Vendor: 0x1532, Product: 0x0a59, Transport: "dongle", Source: "Linux xpad patch (Linux 7.0)"},
	},
	Labels: xboxLabels, TriggerLabels: [2]string{"LT", "RT"},
	Hardware: gamepad.Hardware{Battery: true, Rumble: true, TriggerRumble: true, RearControls: true, VendorRemapping: true},
	ModeGuide: []string{
		"Wired and 2.4 GHz receiver: Xbox 360 protocol through Linux xpad (Linux 7.0 adds the IDs)",
		"The four back buttons and two extra bumpers are remapped by Razer's app and are not reported separately",
	},
	KernelDrivers: []string{"xpad"},
}

func containsAny(d evdev.InputDevice, subs ...string) bool {
	text := lower(d.Name)
	if d.USB != nil {
		text += " " + lower(d.USB.Manufacturer+" "+d.USB.Product)
	}
	for _, s := range subs {
		if contains(text, s) {
			return true
		}
	}
	return false
}

// Specs lists every model of this package, for the registry and documentation.
func Specs() []gamepad.ModelSpec {
	return []gamepad.ModelSpec{XboxSeries, XboxElite2, DualShock4, SwitchPro, EightBitDoUltimate, GameSirG7SE, WolverineV3Pro}
}

// Models returns the matchers registered with the evdev gamepad driver.
func Models() []gamepad.Model {
	var out []gamepad.Model
	for _, s := range Specs() {
		out = append(out, s.Model())
	}
	return out
}

func lower(s string) string { return strings.ToLower(s) }

func contains(s, sub string) bool { return strings.Contains(s, sub) }
