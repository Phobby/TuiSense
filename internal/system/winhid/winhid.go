// Package winhid lists the HID game controllers Windows reports, with the
// identity facts Windows' built-in HID driver exposes: vendor/product/version
// IDs, the top-level usage and the USB product strings. It only reads
// attributes (the device is opened with no read or write access), never
// sends reports, needs no administrator rights and no extra driver.
//
// Controllers served by Microsoft's XInput stack also appear here as a HID
// game controller whose device path contains "IG_" (documented in Microsoft's
// "XInput and DirectInput" article). TuiSense reads those through XInput and
// uses their HID entry only as identity evidence, so one physical controller
// is never listed twice.
package winhid

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// Device is one HID top-level collection.
type Device struct {
	// Path is the Windows device interface path. It contains instance IDs and,
	// for Bluetooth, the controller's address: never show or export it.
	Path                         string
	VendorID, ProductID, Version uint16
	UsagePage, Usage             uint16
	Manufacturer, Product        string
}

// IsGameController reports a Generic Desktop joystick (0x04) or gamepad (0x05).
func (d Device) IsGameController() bool {
	return d.UsagePage == 0x01 && (d.Usage == 0x04 || d.Usage == 0x05)
}

// XInput reports whether Windows serves this controller through XInput.
func (d Device) XInput() bool { return strings.Contains(strings.ToUpper(d.Path), "IG_") }

// Bluetooth reports a Bluetooth (classic HID or HID over GATT) connection.
func (d Device) Bluetooth() bool {
	p := strings.ToLower(d.Path)
	return strings.Contains(p, "{00001124-0000-1000-8000-00805f9b34fb}") || strings.Contains(p, "{00001812-0000-1000-8000-00805f9b34fb}")
}

// HardwareKey is "vid:pid:version" in lowercase hex, the same non-unique key
// the Linux backend uses to remember a model confirmation.
func (d Device) HardwareKey() string {
	return fmt.Sprintf("%04x:%04x:%04x", d.VendorID, d.ProductID, d.Version)
}

// ShortID is a stable, non-reversible handle for the device path, used as a
// session key so the path itself never reaches the UI, logs or reports.
func (d Device) ShortID() string {
	sum := sha256.Sum256([]byte(strings.ToLower(d.Path)))
	return hex.EncodeToString(sum[:4])
}

// AsInputDevice presents the HID identity in the form the model matchers of
// the gamepad driver read (IDs, product strings). The capability bits only
// state what the HID usage already proves: it is a joystick or gamepad.
func (d Device) AsInputDevice() evdev.InputDevice {
	bus := evdev.BusUSB
	if d.Bluetooth() {
		bus = evdev.BusBluetooth
	}
	in := evdev.InputDevice{Name: d.Product, Bus: bus, Vendor: d.VendorID, Product: d.ProductID, Version: d.Version,
		USB: &evdev.USBInfo{VendorID: d.VendorID, ProductID: d.ProductID, BCDDevice: d.Version, Manufacturer: d.Manufacturer, Product: d.Product}}
	if d.IsGameController() {
		in.Caps = evdev.Capabilities{EV: bits(evdev.EvKey, evdev.EvAbs), Key: bits(evdev.BtnJoystickFirst), Abs: bits(evdev.AbsX, evdev.AbsY)}
	}
	return in
}

func bits(codes ...uint16) evdev.Bits {
	var b evdev.Bits
	for _, c := range codes {
		for int(c)/64 >= len(b) {
			b = append(b, 0)
		}
		b[c/64] |= 1 << (c % 64)
	}
	return b
}

// GameControllers filters a listing to joysticks and gamepads.
func GameControllers(all []Device) []Device {
	var out []Device
	for _, d := range all {
		if d.IsGameController() {
			out = append(out, d)
		}
	}
	return out
}
