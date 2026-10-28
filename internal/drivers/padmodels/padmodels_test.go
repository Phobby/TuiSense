package padmodels

import (
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

func bits(codes ...uint16) evdev.Bits {
	b := make(evdev.Bits, 12)
	for _, c := range codes {
		b[c/64] |= 1 << (c % 64)
	}
	return b
}

var stdKeys = []uint16{evdev.BtnSouth, evdev.BtnEast, evdev.BtnNorth, evdev.BtnWest, evdev.BtnTL, evdev.BtnTR,
	evdev.BtnSelect, evdev.BtnStart, evdev.BtnMode, evdev.BtnThumbL, evdev.BtnThumbR}

// pad builds a gamepad node as Linux would expose it.
func pad(vendor, product uint16, driver, name string, extraKeys ...uint16) (evdev.InputDevice, map[uint16]evdev.AbsInfo) {
	d := evdev.InputDevice{
		Name: name, Bus: evdev.BusUSB, Vendor: vendor, Product: product, KernelDriver: driver,
		EventPath: "/dev/input/event9", EventName: "event9",
		Caps: evdev.Capabilities{
			EV:  bits(evdev.EvSyn, evdev.EvKey, evdev.EvAbs, evdev.EvFF),
			Key: bits(append(append([]uint16{}, stdKeys...), extraKeys...)...),
			Abs: bits(evdev.AbsX, evdev.AbsY, evdev.AbsZ, evdev.AbsRX, evdev.AbsRY, evdev.AbsRZ, evdev.AbsHat0X, evdev.AbsHat0Y),
			FF:  bits(evdev.FFRumble),
		},
	}
	stick := evdev.AbsInfo{Minimum: -32768, Maximum: 32767}
	trig := evdev.AbsInfo{Minimum: 0, Maximum: 255}
	hat := evdev.AbsInfo{Minimum: -1, Maximum: 1}
	return d, map[uint16]evdev.AbsInfo{evdev.AbsX: stick, evdev.AbsY: stick, evdev.AbsRX: stick, evdev.AbsRY: stick,
		evdev.AbsZ: trig, evdev.AbsRZ: trig, evdev.AbsHat0X: hat, evdev.AbsHat0Y: hat}
}

func all() []gamepad.Model { return append([]gamepad.Model{msigc20.Model()}, Models()...) }

func resolve(d evdev.InputDevice) (string, gamepad.Confidence) {
	gamepad.SetOverrides(nil)
	r := gamepad.Resolve(all(), d)
	if r.Model == nil {
		return "", gamepad.NoMatch
	}
	return r.Model.ID, r.Confidence
}

func TestExactIdentificationAndWrongModelRejection(t *testing.T) {
	cases := []struct {
		vendor, product uint16
		driver, name    string
		want            string
		conf            gamepad.Confidence
	}{
		{0x045e, 0x0b12, "xpad", "Microsoft Xbox Series S|X Controller", XboxSeriesID, gamepad.Identified},
		{0x045e, 0x0b13, "microsoft", "Xbox Wireless Controller", XboxSeriesID, gamepad.Identified},
		{0x045e, 0x0b00, "xpad", "Microsoft X-Box One Elite 2 pad", XboxElite2ID, gamepad.Identified},
		{0x045e, 0x0b22, "microsoft", "Xbox Wireless Controller", XboxElite2ID, gamepad.Identified},
		{0x054c, 0x09cc, "playstation", "Sony Interactive Entertainment Wireless Controller", DualShock4ID, gamepad.Identified},
		{0x054c, 0x05c4, "sony", "Sony Computer Entertainment Wireless Controller", DualShock4ID, gamepad.Identified},
		{0x057e, 0x2009, "nintendo", "Nintendo Switch Pro Controller", SwitchProID, gamepad.Identified},
		{0x1532, 0x0a57, "xpad", "Razer Wolverine V3 Pro", WolverineID, gamepad.Identified},
		{0x1532, 0x0a59, "xpad", "Razer Wolverine V3 Pro Wireless", WolverineID, gamepad.Identified},
		{0x3537, 0x1234, "xpad", "GameSir G7 SE Controller for Xbox", GameSirG7SEID, gamepad.Identified},
		{0x2dc8, 0x1234, "xpad", "8BitDo Ultimate Wireless Controller", EightBitDoID, gamepad.Probable},
		// Look-alikes that must not be claimed.
		{0x045e, 0x028e, "xpad", "Microsoft X-Box 360 pad", "", gamepad.NoMatch},                // generic 360 ID (no MSI version)
		{0x1532, 0x0a29, "xpad", "Razer Wolverine V2", "", gamepad.NoMatch},                     // another Razer pad
		{0x3537, 0x1004, "xpad", "GameSir T4 Kaleid", "", gamepad.NoMatch},                      // another GameSir pad
		{0x2dc8, 0x3106, "xpad", "8BitDo Ultimate 2C Wireless Controller", "", gamepad.NoMatch}, // another 8BitDo Ultimate
		{0x054c, 0x0ce6, "playstation", "DualSense Wireless Controller", "", gamepad.NoMatch},   // the hidraw driver's
	}
	for _, c := range cases {
		d, _ := pad(c.vendor, c.product, c.driver, c.name)
		id, conf := resolve(d)
		if id != c.want || conf != c.conf {
			t.Errorf("%04x:%04x %q → %q (%d), want %q (%d)", c.vendor, c.product, c.name, id, conf, c.want, c.conf)
		}
	}
}

func describe(t *testing.T, spec gamepad.ModelSpec, d evdev.InputDevice, abs map[uint16]evdev.AbsInfo, rt gamepad.Runtime) device.Descriptor {
	t.Helper()
	m := gamepad.InferMapping(d, abs)
	if len(spec.Hardware.RearInputs) > 0 {
		rt.RearMapped = m.MapExtra(d, spec.Hardware.RearInputs)
	}
	return spec.Describe(m, d, rt)
}

func button(desc device.Descriptor, id device.ButtonID) (device.ButtonDef, bool) {
	return desc.Inputs.Button(id)
}

// Elite paddles are offered only when the kernel reports all four codes.
func TestElitePaddlesOnlyWhenReportedIndividually(t *testing.T) {
	rt := gamepad.RuntimeWith(true)
	d, abs := pad(0x045e, 0x0b00, "xpad", "Microsoft X-Box One Elite 2 pad", btnTriggerHappy5, btnTriggerHappy6, btnTriggerHappy7, btnTriggerHappy8)
	desc := describe(t, XboxElite2, d, abs, rt)
	for i, id := range []device.ButtonID{device.ButtonPaddle1, device.ButtonPaddle2, device.ButtonPaddle3, device.ButtonPaddle4} {
		b, ok := button(desc, id)
		if !ok || b.Label != []string{"P1", "P2", "P3", "P4"}[i] || b.Group != "Rear" {
			t.Fatalf("paddle %d not mapped: %+v", i+1, b)
		}
	}
	if !desc.Capabilities.HasAccess(device.CapRearControls, device.AccessTest) {
		t.Fatal("reported paddles are testable")
	}
	if desc.Capabilities.Supports(device.CapRemapping) {
		t.Fatal("paddle remapping is never offered: TuiSense does not write Elite profiles")
	}

	d, abs = pad(0x045e, 0x0b00, "xpad", "Microsoft X-Box One Elite 2 pad", btnTriggerHappy5, btnTriggerHappy6)
	desc = describe(t, XboxElite2, d, abs, rt)
	if _, ok := button(desc, device.ButtonPaddle1); ok {
		t.Fatal("a partial report must not invent a paddle order")
	}
	rear := desc.Capabilities.Spec(device.CapRearControls)
	if rear.Accessible() || rear.Physical() != device.Present || rear.Requires != device.RequiresDriver {
		t.Fatalf("unreported paddles exist but need the driver: %+v", rear)
	}
}

func TestPositionalFaceButtonsOnPlayStationAndNintendo(t *testing.T) {
	d, abs := pad(0x054c, 0x09cc, "playstation", "Sony Interactive Entertainment Wireless Controller")
	desc := describe(t, DualShock4, d, abs, gamepad.RuntimeWith(true))
	if b, _ := button(desc, device.ButtonNorth); b.Label != "△ Triangle" {
		t.Fatalf("hid-playstation BTN_NORTH is the top button, got %q", b.Label)
	}
	if b, _ := button(desc, device.ButtonWest); b.Label != "□ Square" {
		t.Fatalf("hid-playstation BTN_WEST is the left button, got %q", b.Label)
	}
	d, abs = pad(0x057e, 0x2009, "nintendo", "Nintendo Switch Pro Controller")
	desc = describe(t, SwitchPro, d, abs, gamepad.RuntimeWith(true))
	if b, _ := button(desc, device.ButtonSouth); b.Label != "B" {
		t.Fatalf("Switch south button is B, got %q", b.Label)
	}
	// xpad keeps the historical mapping: BTN_X (= BTN_NORTH) is the left button.
	d, abs = pad(0x045e, 0x0b12, "xpad", "Microsoft Xbox Series S|X Controller")
	desc = describe(t, XboxSeries, d, abs, gamepad.RuntimeWith(true))
	if b, _ := button(desc, device.ButtonWest); b.Label != "X" {
		t.Fatalf("xpad BTN_X is the left X button, got %q", b.Label)
	}
}

// USB and Bluetooth expose different features for the same controller.
func TestConnectionSpecificCapabilities(t *testing.T) {
	d, abs := pad(0x054c, 0x09cc, "playstation", "Sony Interactive Entertainment Wireless Controller")
	usb := gamepad.RuntimeWith(true)
	usb.Battery = true
	if desc := describe(t, DualShock4, d, abs, usb); !desc.Capabilities.HasAccess(device.CapAudio, device.AccessTest) || !desc.Capabilities.Supports(device.CapBattery) {
		t.Fatal("over USB the DualShock 4 is a USB audio device and reports its battery")
	}
	bt := usb
	bt.Conn = device.ConnBluetooth
	d.Bus = evdev.BusBluetooth
	desc := describe(t, DualShock4, d, abs, bt)
	audio := desc.Capabilities.Spec(device.CapAudio)
	if audio.Accessible() || audio.Requires != device.RequiresConnection {
		t.Fatalf("over Bluetooth audio needs USB: %+v", audio)
	}
	if !desc.Capabilities.Supports(device.CapBattery) {
		t.Fatal("battery stays available over Bluetooth")
	}
	if desc.Capabilities.Supports(device.CapMotion) || desc.Capabilities.Spec(device.CapMotion).Physical() != device.Present {
		t.Fatal("motion exists but is not read yet: present and hidden")
	}

	d, _ = pad(0x1532, 0x0a59, "xpad", "Razer Wolverine V3 Pro Wireless")
	if WolverineV3Pro.Connection(d) != device.ConnWireless {
		t.Fatal("the Razer 2.4 GHz receiver ID is a wireless connection")
	}
}

// What a controller does not have is absent, never "unsupported but shown".
func TestPhysicalDeclarationsNeverOverclaim(t *testing.T) {
	for _, s := range append(Specs(), msigc20.Spec) {
		caps := s.Model().Static.Capabilities
		hw := s.Hardware
		for c, has := range map[device.Capability]bool{
			device.CapBattery: hw.Battery, device.CapMotion: hw.Motion, device.CapTouchpad: hw.Touchpad,
			device.CapLighting: hw.Lighting || hw.PlayerLEDs, device.CapAudio: hw.Audio, device.CapRearControls: hw.RearControls,
			device.CapAdaptiveTriggers: false,
		} {
			if got := caps.Spec(c).Physical() == device.Present; got != has {
				t.Errorf("%s: %s presence %v, want %v", s.ID, c, got, has)
			}
		}
		for _, c := range []device.Capability{device.CapLighting, device.CapAdaptiveTriggers, device.CapRemapping, device.CapProfiles, device.CapFirmwareUpdate} {
			if caps.Supports(c) {
				t.Errorf("%s: %s must not be offered (no verified protocol)", s.ID, c)
			}
		}
	}
	// Wired-only controllers have no battery anywhere.
	for _, s := range []gamepad.ModelSpec{GameSirG7SE, msigc20.Spec} {
		if s.Model().Static.Capabilities.Spec(device.CapBattery).Physical() != device.Absent {
			t.Errorf("%s is wired-only", s.ID)
		}
	}
}
