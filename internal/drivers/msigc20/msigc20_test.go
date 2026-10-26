package msigc20

import (
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

func pad(vendor, product, version uint16, usb *evdev.USBInfo) evdev.InputDevice {
	var key, abs evdev.Bits = make(evdev.Bits, 5), evdev.Bits{0x3003f}
	key[4] = 1<<(evdev.BtnSouth%64) | 1<<(evdev.BtnEast%64)
	return evdev.InputDevice{Vendor: vendor, Product: product, Version: version, USB: usb, KernelDriver: "xpad",
		Caps: evdev.Capabilities{EV: evdev.Bits{0x0b}, Key: key, Abs: abs}}
}

func TestIdentify(t *testing.T) {
	cases := []struct {
		name string
		d    evdev.InputDevice
		want gamepad.Confidence
	}{
		{"SDL-documented default mode ID", pad(0x045e, 0x028e, 0x0210, nil), gamepad.Probable},
		{"genuine Xbox 360 pad version", pad(0x045e, 0x028e, 0x0114, nil), gamepad.NoMatch},
		{"MSI vendor ID", pad(0x0db0, 0x1234, 0x0100, nil), gamepad.Identified},
		{"USB product string", pad(0x1111, 0x2222, 0, &evdev.USBInfo{Manufacturer: "MSI", Product: "FORCE GC20 V2"}), gamepad.Identified},
		{"unrelated pad", pad(0x046d, 0xc21d, 0x4014, nil), gamepad.NoMatch},
	}
	for _, c := range cases {
		if got, _ := Identify(c.d); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	// A non-gamepad MSI device (e.g. MYSTIC LIGHT RGB controller) is never a controller.
	light := evdev.InputDevice{Vendor: 0x0db0, Product: 0x0076, Caps: evdev.Capabilities{EV: evdev.Bits{0x09}, Abs: evdev.Bits{1 << 40}}}
	if got, _ := Identify(light); got != gamepad.NoMatch {
		t.Fatalf("MSI RGB controller must not be identified as a gamepad, got %v", got)
	}
}

func TestDescriptorShowsOnlyRealHardware(t *testing.T) {
	m := gamepad.Mapping{}
	d := Describe(m, evdev.InputDevice{Vendor: 0x045e, Product: 0x028e}, false)
	if d.ModelID != ModelID || d.Family == "xbox" || d.VisualID != "msi_gc20" || d.Brand != "MSI" {
		t.Fatalf("unexpected MSI descriptor %+v", d)
	}
	for _, c := range []device.Capability{device.CapAdaptiveTriggers, device.CapLighting, device.CapMotion, device.CapAudio, device.CapBattery, device.CapProfiles, device.CapFirmwareInfo, device.CapFirmwareCheck, device.CapFirmwareUpdate} {
		if d.Capabilities.Supports(c) {
			t.Fatalf("GC20 V2 must not claim %s", c)
		}
	}
	if d.Inputs.TouchpadContacts != 0 {
		t.Fatal("no touchpad")
	}
	for _, r := range d.Regions {
		if r.ID == device.RegionHaptics {
			t.Fatal("without Linux rumble support the vibration region is hidden")
		}
	}
	if len(d.ModeGuide) != 4 {
		t.Fatal("the documented four input modes should be listed")
	}
	if d2 := Describe(m, evdev.InputDevice{}, true); !d2.Capabilities.Supports(device.CapHaptics) || len(d2.Actions) != 1 {
		t.Fatal("rumble support should enable the vibration capability and test action")
	}
}
