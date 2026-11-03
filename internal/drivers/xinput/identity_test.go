package xinput

import (
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/padmodels"
	"github.com/Phobby/TuiSense/internal/system/winhid"
	"github.com/Phobby/TuiSense/internal/ui/visual"
)

func models() []gamepad.Model {
	return append([]gamepad.Model{msigc20.Model()}, padmodels.Models()...)
}

func igPad(vid, pid, ver uint16, n int, product string) winhid.Device {
	return winhid.Device{Path: `\\?\hid#vid_xxxx&pid_xxxx&ig_0` + string(rune('0'+n)) + `#8&1&0&0000#{4d1e55b2}`,
		VendorID: vid, ProductID: pid, Version: ver, UsagePage: 1, Usage: 5, Product: product}
}

func openSlots(t *testing.T, hid []winhid.Device, slots ...uint32) []device.Device {
	t.Helper()
	gamepad.SetOverrides(nil)
	api := &fakeAPI{states: map[uint32]State{}, battery: map[uint32][2]uint8{}}
	for _, s := range slots {
		api.states[s] = State{}
		api.battery[s] = [2]uint8{BatteryWired, 0}
	}
	d := NewDriverWith(api, func() ([]winhid.Device, error) { return hid, nil }, models()...)
	cands, err := d.Discover()
	if err != nil || len(cands) != len(slots) {
		t.Fatalf("discover: %v %v", cands, err)
	}
	var out []device.Device
	for _, c := range cands {
		dev, err := d.Open(c, nil)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, dev)
	}
	return out
}

func assertGeneric(t *testing.T, dev device.Device) {
	t.Helper()
	desc := dev.Descriptor()
	if desc.ModelID != ModelID || desc.VisualID != visual.GenericID || desc.Brand == "Sony" || desc.Brand == "Microsoft" {
		t.Fatalf("expected a generic gamepad, got %s / %s / %s", desc.ModelID, desc.Brand, desc.VisualID)
	}
	if strings.Contains(desc.DisplayName, "DualSense") || strings.Contains(dev.Info().Model, "DualSense") {
		t.Fatal("an unidentified controller must never be named DualSense")
	}
}

// The real Windows report: an MSI Force GC20 V2 in Default (XInput) mode
// enumerates with the shared Xbox 360 ID. It must never become a DualSense,
// nor a Microsoft controller; the MSI is only offered for confirmation.
func TestMSIDefaultModeIsProbableNotBranded(t *testing.T) {
	dev := openSlots(t, []winhid.Device{igPad(0x045e, 0x028e, 0x0210, 0, "Controller (XBOX 360 For Windows)")}, 0)[0]
	assertGeneric(t, dev)
	info := dev.Info()
	if info.IdentityState != "probable" || info.IdentityCandidateID != msigc20.ModelID || info.IdentityKey != "045e:028e:0210" {
		t.Fatalf("identity %+v", info)
	}
	if info.Name != "Controller (XBOX 360 For Windows)" {
		t.Fatal("the raw Windows name is kept as the device name, separate from the model")
	}
	for _, c := range []device.Capability{device.CapBattery, device.CapAdaptiveTriggers, device.CapLighting, device.CapAudio} {
		if dev.Capabilities().Supports(c) {
			t.Errorf("a wired XInput pad must not offer %s", c)
		}
	}
}

func TestConfirmedMSIUsesItsModel(t *testing.T) {
	hid := []winhid.Device{igPad(0x045e, 0x028e, 0x0210, 0, "")}
	before := openSlots(t, hid, 0)
	gamepad.SetOverride("045e:028e:0210", msigc20.ModelID)
	defer gamepad.SetOverrides(nil)
	api := &fakeAPI{states: map[uint32]State{0: {}}, battery: map[uint32][2]uint8{0: {BatteryWired, 0}}}
	d := NewDriverWith(api, func() ([]winhid.Device, error) { return hid, nil }, models()...)
	cands, _ := d.Discover()
	dev, _ := d.Open(cands[0], nil)
	desc := dev.Descriptor()
	if desc.ModelID != msigc20.ModelID || desc.VisualID != "msi_gc20" || dev.Info().IdentityState != "confirmed" {
		t.Fatalf("confirmed MSI: %s %s %s", desc.ModelID, desc.VisualID, dev.Info().IdentityState)
	}
	if cands[0].Endpoint == "" || before[0].Info().IdentityState == "confirmed" {
		t.Fatal("the endpoint must change with the identity so the session re-opens the device")
	}
	// MSI has no battery, lighting, motion, audio: absent, never inaccessible.
	for _, c := range []device.Capability{device.CapBattery, device.CapLighting, device.CapMotion, device.CapAudio, device.CapAdaptiveTriggers} {
		if s := dev.Capabilities().Spec(c); s.Physical() == device.Present {
			t.Errorf("MSI GC20 V2 has no %s", c)
		}
	}
	if !dev.Capabilities().Supports(device.CapHaptics) || !dev.Capabilities().Supports(device.CapSticks) {
		t.Fatal("XInput input and rumble stay available")
	}
}

func TestIdentifiedXboxSeriesAndElite(t *testing.T) {
	dev := openSlots(t, []winhid.Device{igPad(0x045e, 0x0b12, 0x0510, 0, "")}, 0)[0]
	if dev.Descriptor().VisualID != "xbox_series" || dev.Info().IdentityState != "identified" {
		t.Fatalf("Xbox Series: %s %s", dev.Descriptor().VisualID, dev.Info().IdentityState)
	}
	elite := openSlots(t, []winhid.Device{igPad(0x045e, 0x0b00, 0x0510, 0, "")}, 0)[0]
	if elite.Descriptor().VisualID != "xbox_elite2" {
		t.Fatal("Elite Series 2 by ID")
	}
	if elite.Capabilities().Supports(device.CapRearControls) || elite.Capabilities().Spec(device.CapRearControls).Physical() != device.Present {
		t.Fatal("Elite paddles exist but are not reachable through XInput")
	}
}

func TestUnknownOrAmbiguousStaysGeneric(t *testing.T) {
	// No HID evidence.
	assertGeneric(t, openSlots(t, nil, 0)[0])
	// Unknown vendor.
	assertGeneric(t, openSlots(t, []winhid.Device{igPad(0x1234, 0x5678, 1, 0, "USB Gamepad")}, 0)[0])
	// Two different controllers: Windows does not map slots to HID entries.
	for _, dev := range openSlots(t, []winhid.Device{igPad(0x045e, 0x0b12, 1, 0, ""), igPad(0x045e, 0x028e, 0x0210, 1, "")}, 0, 1) {
		assertGeneric(t, dev)
	}
	// Slot/HID count mismatch.
	assertGeneric(t, openSlots(t, []winhid.Device{igPad(0x045e, 0x0b12, 1, 0, "")}, 0, 1)[0])
}

func TestTwoIdenticalControllersStaySeparate(t *testing.T) {
	devs := openSlots(t, []winhid.Device{igPad(0x045e, 0x0b12, 1, 0, ""), igPad(0x045e, 0x0b12, 1, 1, "")}, 0, 2)
	if len(devs) != 2 || devs[0].Info().ID == devs[1].Info().ID {
		t.Fatal("two identical controllers are two devices")
	}
	for _, d := range devs {
		if d.Descriptor().VisualID != "xbox_series" {
			t.Fatal("identical controllers share the exact identity")
		}
	}
}

// Every model an XInput device can be identified as has a 3D component for
// each XInput button and region.
func TestIdentifiedDescriptorsMatchTheirVisual(t *testing.T) {
	for _, m := range models() {
		gamepad.SetOverride("1234:5678:0001", m.ID)
		dev := openSlots2(t, m.ID)
		spec, ok := visual.Spec(dev.Descriptor().VisualID)
		if !ok || spec.ID == visual.GenericID {
			t.Errorf("%s: visual %q", m.ID, dev.Descriptor().VisualID)
			continue
		}
		buttons := map[device.ButtonID]bool{}
		for _, c := range spec.Components {
			if c.Bound {
				buttons[c.Button] = true
			}
		}
		for _, b := range dev.Descriptor().Inputs.Buttons {
			if !buttons[b.ID] {
				t.Errorf("%s: button %s has no component", m.ID, b.Label)
			}
		}
	}
	gamepad.SetOverrides(nil)
}

func openSlots2(t *testing.T, modelID string) device.Device {
	t.Helper()
	api := &fakeAPI{states: map[uint32]State{0: {}}, battery: map[uint32][2]uint8{0: {BatteryWired, 0}}}
	hid := []winhid.Device{igPad(0x1234, 0x5678, 1, 0, "")}
	d := NewDriverWith(api, func() ([]winhid.Device, error) { return hid, nil }, models()...)
	cands, _ := d.Discover()
	dev, _ := d.Open(cands[0], nil)
	if dev.Descriptor().ModelID != modelID {
		t.Fatalf("override to %s ignored: %s", modelID, dev.Descriptor().ModelID)
	}
	return dev
}
