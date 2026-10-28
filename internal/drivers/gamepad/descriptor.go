package gamepad

import (
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// PulseAction is the quick test-vibration action for rumble-capable devices.
func PulseAction() device.Action {
	return device.Action{ID: "pulse_test", Label: "Test Rumble", Capability: device.CapHaptics,
		Command: device.Command{Type: device.CmdPulseHaptics, PulseLeft: 150, PulseRight: 150, PulseDur: 300 * time.Millisecond, PulseAbsolute: true}}
}

// GenericDescriptor describes an unidentified evdev gamepad.
func GenericDescriptor(m Mapping, d evdev.InputDevice, rt Runtime) device.Descriptor {
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
			{ID: device.RegionLeftStick, Label: "Left Stick", Badge: "L-Stick", Capability: device.CapDiagnostics,
				Description: "Open the stick diagnostics."},
			{ID: device.RegionRightStick, Label: "Right Stick", Badge: "R-Stick", Capability: device.CapDiagnostics,
				Description: "Open the stick diagnostics."},
		},
		Inputs:       m.Inputs([2]string{"Left stick", "Right stick"}, [2]string{"LT", "RT"}),
		Capabilities: Capabilities(m, nil, rt),
	}
	if d.Product != 0 {
		desc.ProductIDs = []uint16{d.Product}
	}
	if rt.Rumble {
		desc.Actions = []device.Action{PulseAction()}
	}
	return desc
}

// StaticGenericDescriptor is the registry entry for unidentified gamepads.
func StaticGenericDescriptor() device.Descriptor {
	return GenericDescriptor(Mapping{}, evdev.InputDevice{}, Runtime{})
}
