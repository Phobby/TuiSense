package device

import "strings"

// ButtonID is a semantic, brand-neutral button identifier. Descriptors attach
// model-specific labels (e.g. ButtonSouth = "✕ Cross" on DualSense, "A" on Xbox).
type ButtonID uint8

const (
	ButtonSouth ButtonID = iota
	ButtonEast
	ButtonWest
	ButtonNorth
	ButtonDpadUp
	ButtonDpadDown
	ButtonDpadLeft
	ButtonDpadRight
	ButtonL1
	ButtonR1
	ButtonL2Digital
	ButtonR2Digital
	ButtonL3
	ButtonR3
	ButtonMenu  // Options / Menu / Start
	ButtonView  // Create / View / Select
	ButtonHome  // PS / Guide
	ButtonShare // Xbox Share
	ButtonTouchpadClick
	ButtonMicMute
	ButtonLeftFn
	ButtonRightFn
	ButtonLeftPaddle
	ButtonRightPaddle
	buttonCount
)

// ButtonSet is a bitmask of pressed buttons.
type ButtonSet uint64

func (b ButtonSet) Has(id ButtonID) bool       { return b&(1<<id) != 0 }
func (b ButtonSet) With(id ButtonID) ButtonSet { return b | 1<<id }

// Count returns the number of pressed buttons.
func (b ButtonSet) Count() int {
	n := 0
	for id := ButtonID(0); id < buttonCount; id++ {
		if b.Has(id) {
			n++
		}
	}
	return n
}

// ButtonDef declares one physical button with its model-specific label.
type ButtonDef struct {
	ID    ButtonID
	Label string
	Group string // "Face", "D-pad", "Shoulder", "Stick", "System", "Extra"
}

// AxisRange describes the raw value range a device reports for an input.
type AxisRange struct {
	Min, Max, Center float64
}

// HalfSpan returns the raw distance from center to the edge.
func (r AxisRange) HalfSpan() float64 { return (r.Max - r.Min) / 2 }

// Span returns the full raw range.
func (r AxisRange) Span() float64 { return r.Max - r.Min }

// DriftHeuristics are driver-declared expectations used to *interpret* stick
// measurements. They are heuristics, not manufacturer specifications, and the
// diagnostics UI presents them as such.
type DriftHeuristics struct {
	CenterTolerance float64 // raw counts of resting offset considered typical
	NoiseTolerance  float64 // raw counts of standard deviation considered typical
	Rationale       string  // shown to the user and written into reports
}

// AudioMatch tells platform audio backends how to find the controller's own
// audio endpoints (and never the computer's built-in devices).
type AudioMatch struct {
	USBIDs    []string // "054c:0ce6" (lowercase vid:pid) — the most reliable key
	NameHints []string // fallback substrings of the sound-card name
	// RequiresUSB documents that audio is only exposed over a wired USB link.
	RequiresUSB bool
}

// Matches reports whether a sound card with the given USB ID / name belongs to the controller.
func (m AudioMatch) Matches(usbID, name string) bool {
	usbID = strings.ToLower(strings.TrimSpace(usbID))
	for _, id := range m.USBIDs {
		if usbID != "" && usbID == strings.ToLower(id) {
			return true
		}
	}
	if usbID != "" && len(m.USBIDs) > 0 {
		return false // a USB card with a different ID is definitely another device
	}
	lname := strings.ToLower(name)
	for _, h := range m.NameHints {
		if h != "" && strings.Contains(lname, strings.ToLower(h)) {
			return true
		}
	}
	return false
}

// InputLayout is the diagnostics contract of a controller model: which inputs
// exist, their raw ranges, and labels. The diagnostics UI derives every test
// view from it, so a new controller needs no dashboard changes.
type InputLayout struct {
	Buttons          []ButtonDef
	StickLabels      [2]string // empty when the controller has no analog sticks
	StickRange       AxisRange
	Drift            DriftHeuristics
	TriggerLabels    [2]string // empty when there are no analog triggers
	TriggerRange     AxisRange
	TouchpadContacts int
	TouchpadSize     [2]int  // raw coordinate extent (width, height)
	GyroLSBPerDPS    float64 // nominal raw counts per °/s (0 = unknown); uncalibrated
	AccelLSBPerG     float64 // nominal raw counts per g (0 = unknown); uncalibrated
	BatteryStep      int     // reporting granularity in percent (e.g. 10)
	Audio            AudioMatch
}

// HasSticks reports whether analog sticks are declared.
func (l InputLayout) HasSticks() bool { return l.StickLabels[0] != "" }

// HasTriggers reports whether analog triggers are declared.
func (l InputLayout) HasTriggers() bool { return l.TriggerLabels[0] != "" }

// Button returns the declaration for id.
func (l InputLayout) Button(id ButtonID) (ButtonDef, bool) {
	for _, b := range l.Buttons {
		if b.ID == id {
			return b, true
		}
	}
	return ButtonDef{}, false
}
