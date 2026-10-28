package gamepad

import (
	"fmt"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// Hardware is what a controller model physically has. It is a statement
// about the product, independent of what the operating system exposes; the
// backend combines it with the Runtime it observes to decide what TuiSense
// can actually read, test or configure. Nothing absent here is ever shown.
type Hardware struct {
	Battery       bool
	Rumble        bool
	TriggerRumble bool // impulse-trigger motors (no Linux evdev interface)
	Motion        bool
	Touchpad      bool
	Lighting      bool
	PlayerLEDs    bool
	Audio         bool // headset jack, microphone or speaker
	// AudioUSBIDs are the controller's own USB audio IDs ("054c:09cc") when
	// it is a standard USB audio device; audio diagnostics then probe ALSA.
	AudioUSBIDs   []string
	AudioFeatures []device.Feature
	RearControls  bool
	// RearInputs are kernel key codes that report rear controls one by one
	// (e.g. xpad's BTN_TRIGGER_HAPPY5…8 for Elite paddles). They are mapped
	// only when the device actually advertises every code.
	RearInputs      []ButtonMap
	OnboardProfiles bool
	VendorRemapping bool
	// FirmwareUpdater names the vendor's official updater ("" = none known).
	FirmwareUpdater string
}

// Runtime is what the backend observed on the opened device.
type Runtime struct {
	Rumble     bool // FF_RUMBLE advertised and the node is writable
	RumbleFF   bool // FF_RUMBLE advertised (possibly without write access)
	Battery    bool // a kernel power_supply was found for the device
	Conn       device.ConnectionType
	Readable   bool // the event node could be opened for reading
	RearMapped bool // every RearInputs code is advertised
}

// RuntimeWith is a convenience for tests and static descriptors.
func RuntimeWith(rumble bool) Runtime {
	return Runtime{Rumble: rumble, RumbleFF: rumble, Readable: true, Conn: device.ConnUSBWired}
}

const evdevBackend = "linux-evdev"

// Capabilities derives the capability set of an evdev gamepad. A generic
// device (hw == nil) only gets what is observably accessible; a known model
// also declares its physical features, marked inaccessible with a reason
// when Linux or TuiSense cannot reach them (shown only in Driver &
// Compatibility, never as a missing feature).
func Capabilities(m Mapping, hw *Hardware, rt Runtime) device.CapabilitySet {
	exp := device.VerifiedAutomated
	specs := []device.CapabilitySpec{
		{ID: device.CapConnection, Support: device.Full, Access: device.AccessRead, Backend: evdevBackend, Verification: exp},
		{ID: device.CapDiagnostics, Support: device.Full, Access: device.AccessRead | device.AccessTest, Backend: evdevBackend, Verification: exp,
			Features: []device.Feature{device.FeatRawReports}},
	}
	add := func(s device.CapabilitySpec) { specs = append(specs, s) }
	if m.HasSticks() {
		add(device.CapabilitySpec{ID: device.CapSticks, Support: device.Limited, Access: device.AccessRead | device.AccessTest, Backend: evdevBackend, Verification: exp,
			Note: "Live view and diagnostics only. TuiSense has no input-processing backend, so it cannot change what games receive."})
	}
	switch {
	case rt.Rumble:
		add(device.CapabilitySpec{ID: device.CapHaptics, Support: device.Limited, Access: device.AccessTest, Backend: "linux-evdev-ff", Verification: exp,
			Features: []device.Feature{device.FeatDualMotors, device.FeatTestPulse},
			Note:     "Rumble through Linux force feedback (FF_RUMBLE). TuiSense sends separate strong/weak magnitudes; whether they drive separate motors depends on the kernel driver."})
	case hw != nil && hw.Rumble && rt.RumbleFF:
		add(device.InaccessibleSpec(device.CapHaptics, device.RequiresPermission, "The kernel exposes rumble, but the event node is not writable."))
	case hw != nil && hw.Rumble:
		add(device.InaccessibleSpec(device.CapHaptics, device.RequiresDriver, "The kernel driver exposes no force-feedback (rumble) interface for this device in its current input mode."))
	case hw != nil:
		add(device.AbsentSpec(device.CapHaptics))
	}
	if hw == nil {
		if rt.Battery {
			add(device.CapabilitySpec{ID: device.CapBattery, Support: device.Limited, Access: device.AccessRead, Backend: "linux-power-supply", Verification: exp})
		}
		return device.NewCapabilitySet(specs...)
	}
	switch {
	case hw.Battery && rt.Battery:
		add(device.CapabilitySpec{ID: device.CapBattery, Support: device.Limited, Access: device.AccessRead, Backend: "linux-power-supply", Verification: exp,
			Note: "Battery level and charging state as reported by the kernel driver."})
	case hw.Battery:
		add(device.InaccessibleSpec(device.CapBattery, device.RequiresDriver, "The kernel driver reports no battery level on this connection."))
	default:
		add(device.AbsentSpec(device.CapBattery))
	}
	presence := func(c device.Capability, has bool, why device.Requirement, note string) {
		if has {
			add(device.InaccessibleSpec(c, why, note))
		} else {
			add(device.AbsentSpec(c))
		}
	}
	presence(device.CapMotion, hw.Motion, device.RequiresBackend, "The kernel reports motion sensors on a separate input node that TuiSense does not read yet.")
	presence(device.CapTouchpad, hw.Touchpad, device.RequiresBackend, "The kernel reports the touchpad on a separate input node that TuiSense does not read yet.")
	presence(device.CapLighting, hw.Lighting || hw.PlayerLEDs, device.RequiresBackend, "LED control through the kernel LED class is not implemented in TuiSense.")
	if hw.Audio && len(hw.AudioUSBIDs) > 0 {
		add(device.CapabilitySpec{ID: device.CapAudio, Support: device.Limited, Access: device.AccessTest, Backend: "alsa-usb-audio",
			Verification: device.Experimental, Transports: []device.ConnectionType{device.ConnUSB}, Features: hw.AudioFeatures,
			Note: "Controller audio is a USB audio device; connect with a USB cable."})
	} else {
		presence(device.CapAudio, hw.Audio, device.RequiresDriver, "Controller audio is not exposed as a sound card by Linux for this connection.")
	}
	presence(device.CapProfiles, hw.OnboardProfiles, device.RequiresBackend, "On-board profiles use the vendor's protocol, which TuiSense does not implement.")
	presence(device.CapRemapping, hw.VendorRemapping, device.RequiresBackend, "Remapping is done by the vendor's app; TuiSense does not send vendor configuration commands.")
	presence(device.CapAdaptiveTriggers, false, "", "")
	presence(device.CapFirmwareUpdate, hw.FirmwareUpdater != "", device.RequiresBackend, hw.FirmwareUpdater)
	switch {
	case hw.RearControls && rt.RearMapped:
		add(device.CapabilitySpec{ID: device.CapRearControls, Support: device.Limited, Access: device.AccessRead | device.AccessTest, Backend: evdevBackend,
			Verification: device.Experimental, Note: "Rear controls are reported individually by the kernel driver; the order of the codes follows the driver."})
	case hw.RearControls:
		add(device.InaccessibleSpec(device.CapRearControls, device.RequiresDriver,
			"The rear controls are not reported as separate inputs in this mode; the controller maps them to other buttons itself."))
	default:
		add(device.AbsentSpec(device.CapRearControls))
	}
	return device.NewCapabilitySet(specs...).ForConnection(rt.Conn)
}

// ModelSpec is a data-only controller model on top of the evdev driver:
// identification evidence, labels, physical hardware and the visual.
type ModelSpec struct {
	ID, Family, Brand, DisplayName, ShortName, VisualID string
	Accent                                              device.RGB
	// IDs are verified USB/Bluetooth vendor:product pairs (with their source).
	IDs []DeviceID
	// Strings match the USB/input name (lowercase substrings, all required)
	// when IDs are unknown; Probable marks a string match as indirect.
	Strings  [][]string
	Probable bool
	// Exclude rejects look-alikes (e.g. another product of the same vendor).
	Exclude       func(evdev.InputDevice) bool
	Labels        map[device.ButtonID]string
	StickLabels   [2]string
	TriggerLabels [2]string
	Hardware      Hardware
	ModeGuide     []string
	// KernelDrivers are the Linux drivers known to serve the controller.
	KernelDrivers []string
	ModeHint      func(Mapping, evdev.InputDevice) string
}

// DeviceID is one verified vendor/product pair.
type DeviceID struct {
	Vendor, Product uint16
	Transport       string // "usb", "bluetooth", "dongle"
	Source          string // where the ID is documented
}

// Identify implements Model.Identify from the spec's evidence.
func (s ModelSpec) Identify(d evdev.InputDevice) (Confidence, string) {
	if !d.IsGamepad() || (s.Exclude != nil && s.Exclude(d)) {
		return NoMatch, ""
	}
	for _, id := range s.IDs {
		if d.Vendor == id.Vendor && d.Product == id.Product {
			return Identified, fmt.Sprintf("Device ID %04x:%04x (%s, %s)", id.Vendor, id.Product, id.Transport, id.Source)
		}
	}
	text := strings.ToLower(d.Name + " " + usbStrings(d.USB))
	for _, all := range s.Strings {
		ok := len(all) > 0
		for _, sub := range all {
			ok = ok && strings.Contains(text, sub)
		}
		if ok {
			if s.Probable {
				return Probable, "Device name matches " + strings.Join(all, " + ") + " (not a unique identifier)"
			}
			return Identified, "Device name names the model (" + strings.Join(all, " + ") + ")"
		}
	}
	return NoMatch, ""
}

// Describe implements Model.Describe.
func (s ModelSpec) Describe(m Mapping, d evdev.InputDevice, rt Runtime) device.Descriptor {
	hw := s.Hardware
	if len(hw.RearInputs) > 0 && !rt.RearMapped {
		rt.RearMapped = m.MapExtra(d, hw.RearInputs)
	}
	sticks, triggers := s.StickLabels, s.TriggerLabels
	if sticks[0] == "" {
		sticks = [2]string{"Left stick", "Right stick"}
	}
	if triggers[0] == "" {
		triggers = [2]string{"LT", "RT"}
	}
	in := m.Inputs(sticks, triggers)
	if len(hw.AudioUSBIDs) > 0 {
		in.Audio = device.AudioMatch{USBIDs: hw.AudioUSBIDs, RequiresUSB: true}
	}
	for i := range in.Buttons {
		if l, ok := s.Labels[in.Buttons[i].ID]; ok && in.Buttons[i].Group != "Unmapped" {
			in.Buttons[i].Label = l
		}
	}
	caps := Capabilities(m, &hw, rt)
	desc := device.Descriptor{
		ModelID: s.ID, Family: s.Family, Brand: s.Brand, DisplayName: s.DisplayName, ShortName: s.ShortName,
		VisualID: s.VisualID, Accent: s.Accent, VendorID: d.Vendor,
		Regions:      StandardRegions(m, rt.Rumble, triggers),
		Inputs:       in,
		Capabilities: caps,
		ModeGuide:    s.ModeGuide,
		LinuxDrivers: s.KernelDrivers,
	}
	if d.Product != 0 {
		desc.ProductIDs = []uint16{d.Product}
	}
	for _, id := range s.IDs {
		if d.Product == 0 && !containsU16(desc.ProductIDs, id.Product) {
			desc.ProductIDs = append(desc.ProductIDs, id.Product)
			desc.VendorID = id.Vendor
		}
	}
	if rt.Rumble {
		desc.Actions = []device.Action{PulseAction()}
	}
	return desc
}

func containsU16(list []uint16, v uint16) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Connection returns the transport implied by a matched dongle ID ("" = by bus).
func (s ModelSpec) Connection(d evdev.InputDevice) device.ConnectionType {
	for _, id := range s.IDs {
		if d.Vendor == id.Vendor && d.Product == id.Product && id.Transport == "dongle" {
			return device.ConnWireless
		}
	}
	return ""
}

// Model turns the spec into the matcher registered with the driver.
func (s ModelSpec) Model() Model {
	m := Model{ID: s.ID, Identify: s.Identify, Describe: s.Describe, ModeHint: s.ModeHint, Spec: &s}
	m.Static = s.Describe(Mapping{}, evdev.InputDevice{}, Runtime{})
	return m
}

// StandardRegions are the capability regions of an evdev gamepad: sticks and
// triggers open their diagnostics; the grips open the vibration test when
// rumble is reachable.
func StandardRegions(m Mapping, rumble bool, triggers [2]string) []device.Region {
	var out []device.Region
	if m.HasTriggers || len(m.Buttons) == 0 {
		out = append(out,
			device.Region{ID: device.RegionLeftTrigger, Label: triggers[0] + " Trigger", Badge: triggers[0], Capability: device.CapDiagnostics,
				Description: "Open the trigger diagnostics (analog or digital, depending on the input mode)."},
			device.Region{ID: device.RegionRightTrigger, Label: triggers[1] + " Trigger", Badge: triggers[1], Capability: device.CapDiagnostics,
				Description: "Open the trigger diagnostics (analog or digital, depending on the input mode)."})
	}
	out = append(out,
		device.Region{ID: device.RegionLeftStick, Label: "Left Stick", Badge: "L-Stick", Capability: device.CapDiagnostics,
			Description: "Open the stick and deadzone diagnostics."},
		device.Region{ID: device.RegionRightStick, Label: "Right Stick", Badge: "R-Stick", Capability: device.CapDiagnostics,
			Description: "Open the stick and deadzone diagnostics."})
	if rumble {
		out = append(out, device.Region{ID: device.RegionHaptics, Label: "Vibration Motors", Badge: "Rumble", Capability: device.CapDiagnostics,
			Description: "Open the vibration test (only when Linux exposes rumble)."})
	}
	return out
}

// MapExtra appends button mappings when the device advertises every code of
// extras (all or nothing, so a partial match never invents an order).
func (m *Mapping) MapExtra(d evdev.InputDevice, extras []ButtonMap) bool {
	for _, b := range extras {
		if !d.Caps.Key.Has(b.Code) {
			return false
		}
	}
	for _, b := range extras {
		if _, dup := m.button(b.ID); !dup {
			m.Buttons = append(m.Buttons, b)
		}
	}
	return true
}
