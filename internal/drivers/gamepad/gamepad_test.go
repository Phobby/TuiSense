package gamepad

import (
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

func bits(codes ...uint16) evdev.Bits {
	var b evdev.Bits
	for _, c := range codes {
		w := int(c) / 64
		for len(b) <= w {
			b = append(b, 0)
		}
		b[w] |= 1 << (uint(c) % 64)
	}
	return b
}

// XInputDevice is an xpad-style controller as Linux reports it.
func XInputDevice() (evdev.InputDevice, map[uint16]evdev.AbsInfo) {
	d := evdev.InputDevice{
		Name: "Microsoft X-Box 360 pad", Bus: evdev.BusUSB, Vendor: 0x045e, Product: 0x028e, Version: 0x0210,
		KernelDriver: "xpad", EventPath: "/dev/input/event23", EventName: "event23",
		Caps: evdev.Capabilities{
			EV: bits(evdev.EvSyn, evdev.EvKey, evdev.EvAbs, evdev.EvFF),
			Key: bits(evdev.BtnSouth, evdev.BtnEast, evdev.BtnNorth, evdev.BtnWest, evdev.BtnTL, evdev.BtnTR,
				evdev.BtnSelect, evdev.BtnStart, evdev.BtnMode, evdev.BtnThumbL, evdev.BtnThumbR),
			Abs: bits(evdev.AbsX, evdev.AbsY, evdev.AbsZ, evdev.AbsRX, evdev.AbsRY, evdev.AbsRZ, evdev.AbsHat0X, evdev.AbsHat0Y),
			FF:  bits(evdev.FFRumble, evdev.FFPeriodic, evdev.FFGain),
		},
	}
	stick := evdev.AbsInfo{Minimum: -32768, Maximum: 32767, Fuzz: 16, Flat: 128}
	trig := evdev.AbsInfo{Minimum: 0, Maximum: 255}
	hat := evdev.AbsInfo{Minimum: -1, Maximum: 1}
	return d, map[uint16]evdev.AbsInfo{evdev.AbsX: stick, evdev.AbsY: stick, evdev.AbsRX: stick, evdev.AbsRY: stick,
		evdev.AbsZ: trig, evdev.AbsRZ: trig, evdev.AbsHat0X: hat, evdev.AbsHat0Y: hat}
}

// DirectInputDevice is a generic joystick-style HID gamepad (DirectInput mode).
func DirectInputDevice() (evdev.InputDevice, map[uint16]evdev.AbsInfo) {
	var keys []uint16
	for c := evdev.BtnJoystickFirst; c < evdev.BtnJoystickFirst+12; c++ {
		keys = append(keys, c)
	}
	d := evdev.InputDevice{
		Name: "Generic USB Joystick", Bus: evdev.BusUSB, Vendor: 0x1234, Product: 0x5678, KernelDriver: "hid-generic",
		Caps: evdev.Capabilities{
			EV:  bits(evdev.EvSyn, evdev.EvKey, evdev.EvAbs),
			Key: bits(keys...),
			Abs: bits(evdev.AbsX, evdev.AbsY, evdev.AbsZ, evdev.AbsRZ, evdev.AbsHat0X, evdev.AbsHat0Y),
		},
	}
	a := evdev.AbsInfo{Minimum: 0, Maximum: 255, Value: 128}
	hat := evdev.AbsInfo{Minimum: -1, Maximum: 1}
	return d, map[uint16]evdev.AbsInfo{evdev.AbsX: a, evdev.AbsY: a, evdev.AbsZ: a, evdev.AbsRZ: a, evdev.AbsHat0X: hat, evdev.AbsHat0Y: hat}
}

func TestInferXInputMapping(t *testing.T) {
	d, abs := XInputDevice()
	m := InferMapping(d, abs)
	if m.Layout != LayoutXInput || !m.HasSticks() || m.TriggerKind != device.TriggerAnalog || !m.HasTriggers || !m.Hat {
		t.Fatalf("unexpected xinput mapping %+v", m)
	}
	if m.Sticks[2].Code != evdev.AbsRX || m.Triggers[0].Code != evdev.AbsZ || m.Triggers[1].Code != evdev.AbsRZ {
		t.Fatalf("axis assignment wrong: %+v", m)
	}
	// xpad reports the X face button as BTN_X (0x133): it is the west button.
	if b, ok := m.ButtonFor(evdev.BtnNorth); !ok || b.ID != device.ButtonWest || b.Label != "X" {
		t.Fatalf("BTN_X must map to the west 'X' button, got %+v", b)
	}
	in := m.Inputs([2]string{"Left stick", "Right stick"}, [2]string{"LT", "RT"})
	if in.StickAxis(0).Min != -32768 || in.StickAxis(0).Max != 32767 || in.TriggerAxis(1).Max != 255 {
		t.Fatalf("ranges must come from the kernel: %+v", in)
	}
	if v := in.StickAxis(0).NormalizeBipolar(32767); v != 1 {
		t.Fatalf("full deflection must normalize to 1, got %v", v)
	}
	for _, id := range []device.ButtonID{device.ButtonDpadUp, device.ButtonDpadLeft} {
		if _, ok := in.Button(id); !ok {
			t.Fatalf("hat d-pad should become d-pad buttons")
		}
	}
}

func TestInferDirectInputMapping(t *testing.T) {
	d, abs := DirectInputDevice()
	m := InferMapping(d, abs)
	if m.Layout != LayoutJoystick || !m.HasSticks() {
		t.Fatalf("joystick-style layout with Z/RZ right stick expected, got %+v", m)
	}
	if m.Sticks[2].Code != evdev.AbsZ || m.Sticks[3].Code != evdev.AbsRZ {
		t.Fatalf("DirectInput right stick should be Z/RZ: %+v", m.Sticks)
	}
	if m.HasTriggers {
		t.Fatalf("no trigger axes are exposed; triggers must not be assumed analog")
	}
	if len(m.Buttons) < 12 || m.Buttons[0].Label != "Button 1" || m.Buttons[0].Group != "Unmapped" {
		t.Fatalf("unknown joystick buttons must stay generic: %+v", m.Buttons[:2])
	}
	if len(m.Notes) == 0 {
		t.Fatal("inferences must be explained")
	}
}

func TestInferDigitalAndSharedTriggers(t *testing.T) {
	d, abs := XInputDevice()
	d.KernelDriver = "hid-generic"
	d.Caps.Abs = bits(evdev.AbsX, evdev.AbsY, evdev.AbsRX, evdev.AbsRY)
	d.Caps.Key = bits(evdev.BtnSouth, evdev.BtnEast, evdev.BtnTL, evdev.BtnTR, evdev.BtnTL2, evdev.BtnTR2)
	m := InferMapping(d, abs)
	if m.TriggerKind != device.TriggerDigital || !m.HasTriggers {
		t.Fatalf("BTN_TL2/TR2 without trigger axes → digital triggers, got %+v", m)
	}
	if m.Inputs([2]string{"L", "R"}, [2]string{"LT", "RT"}).TriggerKind != device.TriggerDigital {
		t.Fatal("layout must carry the digital trigger kind")
	}

	d.Caps.Abs = bits(evdev.AbsX, evdev.AbsY, evdev.AbsRX, evdev.AbsRY, evdev.AbsZ)
	m = InferMapping(d, abs)
	if m.TriggerKind != device.TriggerSharedAxis || m.Triggers[0].Code != evdev.AbsZ {
		t.Fatalf("a lone ABS_Z with RX/RY sticks → shared trigger axis, got %+v", m)
	}
}

func TestModeChangeRebuildsMapping(t *testing.T) {
	x, xa := XInputDevice()
	di, da := DirectInputDevice()
	mx, md := InferMapping(x, xa), InferMapping(di, da)
	if mx.Layout == md.Layout || mx.TriggerKind == md.TriggerKind && mx.HasTriggers == md.HasTriggers {
		t.Fatal("different input modes must produce different mappings")
	}
	if CandidateKey(x.EventPath, "a") == CandidateKey(x.EventPath, "b") {
		t.Fatal("the registry key must change with the model so re-identification re-opens the device")
	}
}

func testModel() Model {
	return Model{
		ID:     "test.model",
		Static: device.Descriptor{ModelID: "test.model", DisplayName: "Test Pad"},
		Identify: func(d evdev.InputDevice) (Confidence, string) {
			if d.Vendor == 0x045e && d.Version == 0x0210 {
				return Probable, "test evidence"
			}
			return NoMatch, ""
		},
	}
}

func TestResolveHonorsConfirmations(t *testing.T) {
	SetOverrides(nil)
	defer SetOverrides(nil)
	models := []Model{testModel()}
	d, _ := XInputDevice()
	if r := Resolve(models, d); r.Confidence != Probable || r.Model == nil {
		t.Fatalf("expected probable match, got %+v", r)
	}
	SetOverride(d.HardwareKey(), "test.model")
	if r := Resolve(models, d); r.Confidence != Confirmed {
		t.Fatalf("user confirmation should upgrade the match, got %+v", r)
	}
	SetOverride(d.HardwareKey(), GenericModelID)
	if r := Resolve(models, d); r.Model != nil || r.Candidate != nil {
		t.Fatalf("'not this model' must win over a probable match, got %+v", r)
	}
	SetOverride(d.HardwareKey(), "")
	other, _ := DirectInputDevice()
	if r := Resolve(models, other); r.Model != nil || r.Candidate == nil {
		t.Fatalf("unidentified pads are generic but may be confirmed, got %+v", r)
	}
	SetOverride(other.HardwareKey(), "test.model")
	if r := Resolve(models, other); r.Model == nil || r.Confidence != Confirmed {
		t.Fatalf("a confirmed unknown pad uses the model, got %+v", r)
	}
}

func TestCapabilitiesNeverOverclaim(t *testing.T) {
	d, abs := XInputDevice()
	m := InferMapping(d, abs)
	caps := Capabilities(m, nil, Runtime{})
	for _, c := range []device.Capability{device.CapBattery, device.CapLighting, device.CapAdaptiveTriggers, device.CapMotion, device.CapAudio, device.CapFirmwareInfo, device.CapHaptics} {
		if caps.Supports(c) {
			t.Fatalf("%s must not be claimed", c)
		}
	}
	if Capabilities(m, nil, RuntimeWith(true)).HasFeature(device.CapHaptics, device.FeatMotorConfig) {
		t.Fatal("rumble pulses are not a configurable motor backend")
	}
	if caps.HasFeature(device.CapSticks, device.FeatDeadzones) {
		t.Fatal("there is no input-processing backend for deadzones")
	}
}
