// Package diag implements TuiSense's universal controller diagnostics: test
// availability from capabilities, measurement engines (sticks, buttons,
// triggers, touchpad, motion, haptics, battery, connection, audio), guided
// runs, and reports. It contains no UI code and no driver-specific logic; all
// device knowledge comes from device.Descriptor / InputLayout / CapabilitySet.
package diag

import (
	"fmt"
	"time"
)

// Status is the state of one diagnostic category.
type Status int

const (
	NotTested Status = iota
	Testing
	Passed
	Warning
	Failed
	Unsupported
	Unavailable
	Inconclusive
)

func (s Status) String() string {
	return [...]string{"Not tested", "Testing", "Passed", "Warning", "Failed", "Unsupported", "Unavailable", "Inconclusive"}[s]
}

// Icon returns a compact glyph for the status.
func (s Status) Icon() string {
	return [...]string{"○", "◌", "✓", "!", "✕", "–", "⊘", "?"}[s]
}

// MarshalText makes statuses readable in JSON reports.
func (s Status) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Category groups related tests.
type Category string

const (
	CatSticks     Category = "sticks"
	CatButtons    Category = "buttons"
	CatTriggers   Category = "triggers"
	CatAudioIn    Category = "audio_input"
	CatAudioOut   Category = "audio_output"
	CatMotion     Category = "motion"
	CatTouchpad   Category = "touchpad"
	CatHaptics    Category = "haptics"
	CatBattery    Category = "battery"
	CatConnection Category = "connection"
)

// AllCategories is the canonical display order.
var AllCategories = []Category{
	CatSticks, CatButtons, CatTriggers, CatAudioIn, CatAudioOut,
	CatMotion, CatTouchpad, CatHaptics, CatBattery, CatConnection,
}

// Label returns the display name.
func (c Category) Label() string {
	switch c {
	case CatSticks:
		return "Analog sticks"
	case CatButtons:
		return "Buttons"
	case CatTriggers:
		return "Triggers"
	case CatAudioIn:
		return "Microphone"
	case CatAudioOut:
		return "Speaker / audio out"
	case CatMotion:
		return "Motion sensors"
	case CatTouchpad:
		return "Touchpad"
	case CatHaptics:
		return "Vibration"
	case CatBattery:
		return "Battery"
	case CatConnection:
		return "Connection"
	}
	return string(c)
}

// Measurement is one numeric observation with its unit.
type Measurement struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
}

func (m Measurement) String() string {
	return fmt.Sprintf("%s: %.2f %s", m.Name, m.Value, m.Unit)
}

// Result is the outcome of a category's test.
type Result struct {
	Category     Category      `json:"category"`
	Status       Status        `json:"status"`
	Summary      string        `json:"summary"`
	Details      []string      `json:"details,omitempty"`
	Measurements []Measurement `json:"measurements,omitempty"`
	Limitations  []string      `json:"limitations,omitempty"`
	Simulated    bool          `json:"simulated"`
	// UserConfirmed marks results that rely on the user's perception
	// (e.g. "I felt the vibration"), not on a device measurement.
	UserConfirmed bool      `json:"user_confirmed,omitempty"`
	At            time.Time `json:"at"`
}

// Availability says whether a category can run on the active device.
type Availability struct {
	Category Category
	Status   Status // NotTested (= available), Unsupported, or Unavailable
	Reason   string
}

// Available reports whether the test can be run.
func (a Availability) Available() bool { return a.Status == NotTested }

// CategoryForRegion maps a clicked controller region to its diagnostic
// category, preferring categories available on the device.
func CategoryForRegion(region string, avail map[Category]Availability) (Category, bool) {
	pick := func(cs ...Category) (Category, bool) {
		for _, c := range cs {
			if a, ok := avail[c]; ok && a.Status != Unsupported {
				return c, true
			}
		}
		return "", false
	}
	switch region {
	case "left_trigger", "right_trigger":
		return pick(CatTriggers, CatHaptics)
	case "left_stick", "right_stick":
		return pick(CatSticks)
	case "light":
		return pick(CatTouchpad, CatButtons)
	case "haptics":
		return pick(CatHaptics)
	case "audio":
		return pick(CatAudioIn, CatAudioOut)
	case "motion":
		return pick(CatMotion)
	}
	return "", false
}
