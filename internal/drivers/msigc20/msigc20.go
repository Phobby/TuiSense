// Package msigc20 identifies and describes the MSI Force GC20 V2 wired
// controller on top of the generic evdev gamepad driver.
//
// What is known (and what is not):
//   - MSI's manual documents four input modes switched with the Mode button
//     and shown by four LEDs: Default (LEDs 1+2), DirectInput Analog (1+3),
//     DirectInput Digital (1+4) and Android (3+4). Each mode can enumerate as a
//     different USB device, so no single VID/PID or axis map is assumed.
//   - The SDL community controller database lists "MSI GC20 V2" as
//     045e:028e with version 0x0210, i.e. the Default mode presents itself
//     with Microsoft's generic Xbox 360 controller ID. That ID is shared with
//     genuine Xbox 360 pads and many clones, so a match is only "probable".
//   - MSI's own USB vendor ID is 0db0. A gamepad using it, or naming MSI in
//     its USB strings, is treated as identified.
//
// Inputs are always mapped from what Linux reports (see gamepad.InferMapping);
// this package only adds identification, labels, visuals and mode hints.
// No mode-changing or other undocumented commands are ever sent.
package msigc20

import (
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

const (
	ModelID = "msi.force_gc20_v2"
	Family  = "msi_gc20"

	msiVendorID      uint16 = 0x0db0
	xbox360VendorID  uint16 = 0x045e
	xbox360ProductID uint16 = 0x028e
	sdlDocumentedBCD uint16 = 0x0210 // version in the SDL database entry "MSI GC20 V2"
)

// ModeGuide is verified against https://www.msi.com/faq/6413 and
// https://download.msi.com/archive/mnu_exe/gear/GC30%2BGC20_manual.pdf.
var ModeGuide = []string{
	"Default mode — mode LEDs 1+2 (usually reported by Linux as an XInput/xpad controller)",
	"Analog mode (DirectInput) — LEDs 1+3: hold the Mode button 3 s from Default mode",
	"Digital mode (DirectInput) — LEDs 1+4: press the Mode button 1 s from Analog mode",
	"Android mode — LEDs 3+4: hold the Mode button 3 s from Digital mode",
}

// Identify decides whether an evdev gamepad is an MSI Force GC20 V2.
func Identify(d evdev.InputDevice) (gamepad.Confidence, string) {
	if !d.IsGamepad() {
		return gamepad.NoMatch, ""
	}
	strs := ""
	if d.USB != nil {
		strs = strings.ToLower(d.USB.Manufacturer + " " + d.USB.Product)
	}
	name := strings.ToLower(d.Name)
	switch {
	case strings.Contains(strs, "gc20") || strings.Contains(name, "gc20"):
		return gamepad.Identified, "USB product string names the GC20"
	case d.Vendor == msiVendorID || (d.USB != nil && d.USB.VendorID == msiVendorID):
		return gamepad.Identified, "MSI USB vendor ID (0db0)"
	case strings.Contains(strs, "micro-star") || strings.Contains(strs, "msi "):
		return gamepad.Identified, "USB manufacturer string names MSI"
	case d.Vendor == xbox360VendorID && d.Product == xbox360ProductID && d.Version == sdlDocumentedBCD:
		return gamepad.Probable, "USB ID 045e:028e version 0210 matches the SDL community entry for the MSI GC20 V2 (shared with other Xbox 360-compatible pads)"
	}
	return gamepad.NoMatch, ""
}

// ModeHint explains the observed input mode in terms of MSI's mode LEDs.
func ModeHint(m gamepad.Mapping, d evdev.InputDevice) string {
	switch m.Layout {
	case gamepad.LayoutXInput:
		return "Unverified: Default mode (XInput, LEDs 1+2) — inferred from the xpad layout"
	case gamepad.LayoutJoystick:
		return "Unverified: DirectInput mode (Analog LEDs 1+3 or Digital LEDs 1+4) — inferred; check the LEDs"
	}
	return "Unverified: HID gamepad mode (possibly Android, LEDs 3+4) — inferred; check the LEDs"
}

var buttonLabels = map[device.ButtonID]string{
	device.ButtonSouth: "A", device.ButtonEast: "B", device.ButtonWest: "X", device.ButtonNorth: "Y",
	device.ButtonL1: "LB", device.ButtonR1: "RB", device.ButtonL2Digital: "LT", device.ButtonR2Digital: "RT",
	device.ButtonView: "Back", device.ButtonMenu: "Start", device.ButtonHome: "Mode",
	device.ButtonL3: "LS (click)", device.ButtonR3: "RS (click)",
}

// Spec is the GC20 V2's physical description. It is wired-only (no battery),
// has two vibration motors and no lighting, motion, touchpad, audio or rear
// controls (MSI datasheet and manual).
var Spec = gamepad.ModelSpec{
	ID: ModelID, Family: Family, Brand: "MSI", DisplayName: "MSI Force GC20 V2", ShortName: "GC20 V2",
	VisualID: "msi_gc20", Accent: device.RGB{R: 220, G: 60, B: 60},
	Labels:        buttonLabels,
	StickLabels:   [2]string{"Left stick", "Right stick"},
	TriggerLabels: [2]string{"LT", "RT"},
	Hardware:      gamepad.Hardware{Rumble: true},
	ModeGuide:     ModeGuide,
	KernelDrivers: []string{"xpad", "hid-generic"},
	ModeHint:      ModeHint,
}

// Describe builds the runtime descriptor from the observed mapping.
func Describe(m gamepad.Mapping, d evdev.InputDevice, rt gamepad.Runtime) device.Descriptor {
	return Spec.Describe(m, d, rt)
}

// Model returns the matcher registered with the gamepad driver. MSI keeps
// its own identification (USB strings, MSI's vendor ID, or the SDL-documented
// shared Xbox 360 ID as a probable match).
func Model() gamepad.Model {
	m := Spec.Model()
	m.Identify = Identify
	return m
}
