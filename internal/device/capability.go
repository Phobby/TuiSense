package device

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"sort"
	"strings"
)

// Capability identifies a high-level hardware or software feature area that a
// controller may expose. The UI is generated from capabilities: every panel,
// region, and quick action is bound to one, so a new controller only needs to
// declare what it supports for the interface to adapt automatically.
type Capability string

const (
	CapBattery          Capability = "battery"
	CapConnection       Capability = "connection"
	CapFirmwareInfo     Capability = "firmware_info"
	CapHaptics          Capability = "haptics"
	CapAdaptiveTriggers Capability = "adaptive_triggers"
	CapLighting         Capability = "lighting"
	CapSticks           Capability = "sticks"
	CapMotion           Capability = "motion"
	CapAudio            Capability = "audio"
	CapProfiles         Capability = "profiles"
	CapDiagnostics      Capability = "diagnostics"
	CapFirmwareUpdate   Capability = "firmware_update"
	// CapFirmwareCheck is a read-only comparison of the installed firmware
	// with the vendor's published metadata. It never installs anything.
	CapFirmwareCheck Capability = "firmware_check"
	CapRemapping     Capability = "remapping"
	CapCalibration   Capability = "calibration"
	// CapTouchpad is the touch surface (its click is a button).
	CapTouchpad Capability = "touchpad"
	// CapRearControls covers back buttons and paddles.
	CapRearControls Capability = "rear_controls"
)

// AllCapabilities lists every known capability in canonical display order.
var AllCapabilities = []Capability{
	CapBattery,
	CapConnection,
	CapFirmwareInfo,
	CapHaptics,
	CapAdaptiveTriggers,
	CapLighting,
	CapSticks,
	CapMotion,
	CapAudio,
	CapProfiles,
	CapDiagnostics,
	CapFirmwareUpdate,
	CapFirmwareCheck,
	CapRemapping,
	CapCalibration,
	CapTouchpad,
	CapRearControls,
}

// Label returns a human-readable capability name.
func (c Capability) Label() string {
	switch c {
	case CapBattery:
		return i18n.T("Battery Status")
	case CapConnection:
		return i18n.T("Connection Status")
	case CapFirmwareInfo:
		return i18n.T("Firmware Info")
	case CapHaptics:
		return i18n.T("Haptics")
	case CapAdaptiveTriggers:
		return i18n.T("Adaptive Triggers")
	case CapLighting:
		return i18n.T("Lighting")
	case CapSticks:
		return i18n.T("Sticks")
	case CapMotion:
		return i18n.T("Motion Sensors")
	case CapAudio:
		return i18n.T("Audio")
	case CapProfiles:
		return i18n.T("Profiles")
	case CapDiagnostics:
		return i18n.T("Diagnostics")
	case CapFirmwareUpdate:
		return i18n.T("Firmware Update")
	case CapFirmwareCheck:
		return i18n.T("Firmware Update Check")
	case CapRemapping:
		return i18n.T("Remapping")
	case CapCalibration:
		return i18n.T("Calibration")
	case CapTouchpad:
		return i18n.T("Touchpad")
	case CapRearControls:
		return i18n.T("Rear buttons and paddles")
	default:
		return string(c)
	}
}

// Feature is a fine-grained sub-feature of a capability (e.g. RGB lighting vs.
// a single white LED). Panels use features to decide which controls to show.
type Feature string

const (
	// Lighting
	FeatLightbarRGB   Feature = "lightbar_rgb"
	FeatPlayerLEDs    Feature = "player_leds"
	FeatMicLED        Feature = "mic_led"
	FeatLEDBrightness Feature = "led_brightness"
	FeatLEDAnimation  Feature = "led_animation"

	// Haptics
	FeatDualMotors    Feature = "dual_motors"
	FeatTriggerMotors Feature = "trigger_motors" // e.g. Xbox impulse triggers
	FeatTestPulse     Feature = "test_pulse"
	FeatMotorConfig   Feature = "motor_config" // persistent motor intensities configurable from the Haptics panel

	// Audio
	FeatSpeaker    Feature = "speaker"
	FeatHeadset    Feature = "headset"
	FeatMicrophone Feature = "microphone"

	// Motion
	FeatGyro  Feature = "gyro"
	FeatAccel Feature = "accel"

	// Sticks
	FeatDeadzones     Feature = "deadzones"
	FeatResponseCurve Feature = "response_curve"

	// Profiles
	FeatHardwareSlots Feature = "hardware_slots" // on-device profile slots (DualSense Edge)

	// Diagnostics
	FeatRawReports Feature = "raw_reports"
	FeatTouchpad   Feature = "touchpad"
)

// Support describes how completely a controller implements a capability.
type Support int

const (
	Unsupported Support = iota
	Limited
	Full
)

func (s Support) String() string {
	switch s {
	case Full:
		return "Full"
	case Limited:
		return "Limited"
	default:
		return "Unsupported"
	}
}

// CapabilitySpec declares one capability: whether the hardware has it, what
// TuiSense can actually do with it (read, test, configure on the device,
// configure through software, or only preview), on which connections, through
// which backend, and how well that is verified. "The hardware has it" and
// "TuiSense can configure it" are deliberately separate facts.
type CapabilitySpec struct {
	ID       Capability
	Support  Support // Full / Limited when accessible (Limited marks partial support in the UI)
	Features []Feature
	Note     string

	// Presence is the physical fact; zero means "implied by Support".
	Presence Presence
	// Access is what TuiSense can do; zero means nothing (the UI hides it).
	Access Access
	// Requires explains why a present feature is not accessible.
	Requires Requirement
	// Transports limits Access to these connections (nil = every connection).
	Transports []ConnectionType
	// Backend names the implementation (e.g. "dualsense-hidraw", "evdev-ff").
	Backend string
	// Verification records how the implementation was validated.
	Verification Verification
	// Persistent: a device configuration survives a disconnect (stored on the controller).
	Persistent bool
}

// Presence is whether the hardware physically contains a feature.
type Presence uint8

const (
	PresenceImplied Presence = iota // legacy: Support != Unsupported means present
	Present
	Absent
)

// Access is a set of operations TuiSense can perform on a capability.
type Access uint8

const (
	AccessRead           Access = 1 << iota // live values are read from the device
	AccessTest                              // a diagnostic can exercise it
	AccessConfigDevice                      // settings are written to the controller
	AccessConfigSoftware                    // settings change input through a software backend
	AccessPreview                           // adjustable inside TuiSense only; no effect on games or hardware
)

// Has reports whether every bit of want is granted.
func (a Access) Has(want Access) bool { return want != 0 && a&want == want }

// Requirement is why present hardware is not accessible.
type Requirement string

const (
	RequiresNothing    Requirement = ""
	RequiresDriver     Requirement = "driver"     // the OS driver does not expose it
	RequiresPermission Requirement = "permission" // device node access is missing
	RequiresConnection Requirement = "connection" // not available on the current transport
	RequiresBackend    Requirement = "backend"    // TuiSense has no implementation yet
	RequiresPlatform   Requirement = "platform"   // not implemented on this operating system
)

// Verification is how an implementation has been validated.
type Verification string

const (
	VerifiedOnHardware Verification = "hardware"     // exercised on a physical controller
	VerifiedAutomated  Verification = "automated"    // protocol/unit tests and mocks only
	Experimental       Verification = "experimental" // implemented from documentation, untested
	NotVerified        Verification = "unverified"
)

// SupportState is the single summary the UI and support matrix use.
type SupportState string

const (
	StateHardwareAbsent       SupportState = "HardwareAbsent"
	StateDetected             SupportState = "Detected"
	StateRequiresDriver       SupportState = "RequiresDriver"
	StateReadOnly             SupportState = "ReadOnly"
	StateTestable             SupportState = "Testable"
	StatePreviewOnly          SupportState = "PreviewOnly"
	StateConfigurableDevice   SupportState = "ConfigurableOnDevice"
	StateConfigurableSoftware SupportState = "ConfigurableThroughSoftware"
)

// Physical resolves the presence, honouring the legacy implication.
func (cs CapabilitySpec) Physical() Presence {
	if cs.Presence != PresenceImplied {
		return cs.Presence
	}
	if cs.Support != Unsupported {
		return Present
	}
	return Absent
}

// Accessible reports whether TuiSense can do anything with the capability.
func (cs CapabilitySpec) Accessible() bool {
	return cs.Support != Unsupported && cs.Physical() == Present && cs.Access != 0
}

// State summarises the spec.
func (cs CapabilitySpec) State() SupportState {
	switch {
	case cs.Physical() == Absent:
		return StateHardwareAbsent
	case !cs.Accessible() && (cs.Requires == RequiresDriver || cs.Requires == RequiresPermission):
		return StateRequiresDriver
	case !cs.Accessible():
		return StateDetected
	case cs.Access.Has(AccessConfigDevice):
		return StateConfigurableDevice
	case cs.Access.Has(AccessConfigSoftware):
		return StateConfigurableSoftware
	case cs.Access.Has(AccessPreview):
		return StatePreviewOnly
	case cs.Access.Has(AccessTest):
		return StateTestable
	}
	return StateReadOnly
}

// HasAccess reports whether c is accessible with every bit of want.
func (cs CapabilitySet) HasAccess(c Capability, want Access) bool {
	s := cs.Spec(c)
	return s.Accessible() && s.Access.Has(want)
}

// AbsentSpec declares hardware the controller does not have.
func AbsentSpec(id Capability) CapabilitySpec {
	return CapabilitySpec{ID: id, Support: Unsupported, Presence: Absent}
}

// InaccessibleSpec declares hardware that exists but TuiSense cannot use
// (the reason is shown only in Driver & Compatibility, never as a failure).
func InaccessibleSpec(id Capability, why Requirement, note string) CapabilitySpec {
	return CapabilitySpec{ID: id, Support: Unsupported, Presence: Present, Requires: why, Note: note}
}

// ForConnection narrows capabilities whose access is limited to certain
// transports: on any other connection they become present-but-inaccessible.
func (cs CapabilitySet) ForConnection(conn ConnectionType) CapabilitySet {
	out := cs.Clone()
	if conn == "" { // unknown transport (static descriptors): keep the declaration
		return out
	}
	for id, s := range out {
		if len(s.Transports) == 0 || !s.Accessible() {
			continue
		}
		ok := false
		for _, t := range s.Transports {
			ok = ok || t == conn || (t == ConnUSB && conn == ConnUSBWired) || (t == ConnUSBWired && conn == ConnUSB)
		}
		if !ok {
			s.Presence, s.Support, s.Access, s.Requires = Present, Unsupported, 0, RequiresConnection
			out[id] = s
		}
	}
	return out
}

// HasFeature reports whether the spec lists feat.
func (cs CapabilitySpec) HasFeature(feat Feature) bool {
	for _, f := range cs.Features {
		if f == feat {
			return true
		}
	}
	return false
}

// CapabilitySet is an immutable-by-convention map of declared capabilities.
// Absent capabilities are Unsupported.
type CapabilitySet map[Capability]CapabilitySpec

// NewCapabilitySet builds a set from specs.
func NewCapabilitySet(specs ...CapabilitySpec) CapabilitySet {
	set := make(CapabilitySet, len(specs))
	for _, s := range specs {
		set[s.ID] = s
	}
	return set
}

// Spec returns the declared spec for c, or an Unsupported spec if absent.
func (cs CapabilitySet) Spec(c Capability) CapabilitySpec {
	if s, ok := cs[c]; ok {
		return s
	}
	return CapabilitySpec{ID: c, Support: Unsupported}
}

// Level returns the support level for c.
func (cs CapabilitySet) Level(c Capability) Support {
	return cs.Spec(c).Support
}

// Supports reports whether TuiSense can actually use c: the hardware has it
// and at least one operation is implemented. Physically present but
// inaccessible features are not "supported" and are hidden from the UI.
func (cs CapabilitySet) Supports(c Capability) bool {
	return cs.Spec(c).Accessible()
}

// HasFeature reports whether c is supported and lists feat.
func (cs CapabilitySet) HasFeature(c Capability, feat Feature) bool {
	s := cs.Spec(c)
	return s.Accessible() && s.HasFeature(feat)
}

// Clone returns a deep copy so callers can derive runtime variants safely.
func (cs CapabilitySet) Clone() CapabilitySet {
	out := make(CapabilitySet, len(cs))
	for k, v := range cs {
		v.Features = append([]Feature(nil), v.Features...)
		v.Transports = append([]ConnectionType(nil), v.Transports...)
		out[k] = v
	}
	return out
}

// WithoutFeature returns a copy where feat is removed from c. If c has no
// features left and was Full, it is downgraded to Limited with note.
func (cs CapabilitySet) WithoutFeature(c Capability, feat Feature, note string) CapabilitySet {
	out := cs.Clone()
	s, ok := out[c]
	if !ok {
		return out
	}
	kept := s.Features[:0]
	for _, f := range s.Features {
		if f != feat {
			kept = append(kept, f)
		}
	}
	s.Features = kept
	if s.Support == Full {
		s.Support = Limited
	}
	if note != "" {
		s.Note = note
	}
	out[c] = s
	return out
}

// Supported returns supported capabilities in canonical order.
func (cs CapabilitySet) Supported() []Capability {
	var out []Capability
	for _, c := range AllCapabilities {
		if cs.Supports(c) {
			out = append(out, c)
		}
	}
	// Include custom capabilities a third-party driver may have declared.
	var extra []Capability
	for c, s := range cs {
		if s.Accessible() && !isKnownCapability(c) {
			extra = append(extra, c)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	return append(out, extra...)
}

// Summary returns a compact middot-separated list of supported capability labels.
func (cs CapabilitySet) Summary() string {
	caps := cs.Supported()
	if len(caps) == 0 {
		return "None"
	}
	labels := make([]string, len(caps))
	for i, c := range caps {
		labels[i] = c.Label()
		if cs.Level(c) == Limited {
			labels[i] += "*"
		}
	}
	return strings.Join(labels, " · ")
}

func isKnownCapability(c Capability) bool {
	for _, k := range AllCapabilities {
		if k == c {
			return true
		}
	}
	return false
}
