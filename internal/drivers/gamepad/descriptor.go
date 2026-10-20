package gamepad

import (
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// Capabilities derives the capability set from what the kernel exposes.
// Nothing is declared that TuiSense cannot actually do with this device.
func Capabilities(m Mapping, rumble bool) device.CapabilitySet {
	specs := []device.CapabilitySpec{
		{ID: device.CapConnection, Support: device.Full},
		{ID: device.CapDiagnostics, Support: device.Full, Features: []device.Feature{device.FeatRawReports}},
	}
	if m.HasSticks() {
		specs = append(specs, device.CapabilitySpec{ID: device.CapSticks, Support: device.Limited,
			Note: "Live view and diagnostics only. TuiSense has no input-processing backend, so it cannot change what games receive."})
	}
	if rumble {
		specs = append(specs, device.CapabilitySpec{ID: device.CapHaptics, Support: device.Limited,
			Features: []device.Feature{device.FeatDualMotors, device.FeatTestPulse},
			Note:     "Rumble through Linux force feedback (FF_RUMBLE). TuiSense sends separate strong/weak magnitudes; whether they drive separate motors depends on the kernel driver."})
	} else {
		specs = append(specs, device.CapabilitySpec{ID: device.CapHaptics, Support: device.Unsupported,
			Note: "The kernel driver exposes no force-feedback (rumble) interface for this device in its current input mode."})
	}
	return device.NewCapabilitySet(specs...)
}

// PulseAction is the quick test-vibration action for rumble-capable devices.
func PulseAction() device.Action {
	return device.Action{ID: "pulse_test", Label: "Test Rumble", Capability: device.CapHaptics,
		Command: device.Command{Type: device.CmdPulseHaptics, PulseLeft: 150, PulseRight: 150, PulseDur: 300 * time.Millisecond, PulseAbsolute: true}}
}

// GenericDescriptor describes an unidentified evdev gamepad.
func GenericDescriptor(m Mapping, d evdev.InputDevice, rumble bool) device.Descriptor {
	desc := device.Descriptor{
		ModelID:     GenericModelID,
		Family:      "gamepad",
		Brand:       "Generic",
		DisplayName: "USB Gamepad (unidentified)",
		ShortName:   "Gamepad",
		VisualID:    "generic",
		Accent:      device.RGB{R: 140, G: 150, B: 175},
		VendorID:    d.Vendor,
		Regions: []device.Region{
			{ID: device.RegionLeftStick, Label: "Left Stick", Badge: "L-Stick", Capability: device.CapDiagnostics, Slot: device.SlotMidLeft,
				Description: "Open the stick diagnostics."},
			{ID: device.RegionRightStick, Label: "Right Stick", Badge: "R-Stick", Capability: device.CapDiagnostics, Slot: device.SlotMidRight,
				Description: "Open the stick diagnostics."},
		},
		Inputs:       m.Inputs([2]string{"Left stick", "Right stick"}, [2]string{"LT", "RT"}),
		Capabilities: Capabilities(m, rumble),
	}
	if d.Product != 0 {
		desc.ProductIDs = []uint16{d.Product}
	}
	if rumble {
		desc.Actions = []device.Action{PulseAction()}
	}
	return desc
}

// StaticGenericDescriptor is the registry entry for unidentified gamepads.
func StaticGenericDescriptor() device.Descriptor {
	return GenericDescriptor(Mapping{}, evdev.InputDevice{}, false)
}
