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
	CapRemapping        Capability = "remapping"
	CapCalibration      Capability = "calibration"
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
	CapRemapping,
	CapCalibration,
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
	case CapRemapping:
		return i18n.T("Remapping")
	case CapCalibration:
		return i18n.T("Calibration")
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

// CapabilitySpec declares one capability's support level, its sub-features,
// and an optional note explaining a limitation.
type CapabilitySpec struct {
	ID       Capability
	Support  Support
	Features []Feature
	Note     string
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

// Supports reports whether c is at least partially supported.
func (cs CapabilitySet) Supports(c Capability) bool {
	return cs.Level(c) != Unsupported
}

// HasFeature reports whether c is supported and lists feat.
func (cs CapabilitySet) HasFeature(c Capability, feat Feature) bool {
	s := cs.Spec(c)
	return s.Support != Unsupported && s.HasFeature(feat)
}

// Clone returns a deep copy so callers can derive runtime variants safely.
func (cs CapabilitySet) Clone() CapabilitySet {
	out := make(CapabilitySet, len(cs))
	for k, v := range cs {
		v.Features = append([]Feature(nil), v.Features...)
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
		if s.Support != Unsupported && !isKnownCapability(c) {
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
