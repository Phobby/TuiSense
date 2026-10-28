package state

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
)

// UXMode selects how much of the interface is exposed.
type UXMode string

const (
	ModeEZ       UXMode = "ez"
	ModeAdvanced UXMode = "advanced"
)

// Label returns the display name of the mode.
func (m UXMode) Label() string {
	if m == ModeAdvanced {
		return i18n.T("Advanced")
	}
	return "EZ"
}

// Toggle returns the other mode.
func (m UXMode) Toggle() UXMode {
	if m == ModeAdvanced {
		return ModeEZ
	}
	return ModeAdvanced
}

// ParseMode converts a persisted string to a mode, defaulting to EZ.
func ParseMode(s string) UXMode {
	if UXMode(s) == ModeAdvanced {
		return ModeAdvanced
	}
	return ModeEZ
}

// ModeMask is a set of UX modes a panel appears in.
type ModeMask uint8

const (
	InEZ       ModeMask = 1 << 0
	InAdvanced ModeMask = 1 << 1
	InBoth              = InEZ | InAdvanced
)

func (mm ModeMask) Includes(m UXMode) bool {
	if m == ModeAdvanced {
		return mm&InAdvanced != 0
	}
	return mm&InEZ != 0
}

// PanelID identifies an inspector panel.
type PanelID string

const (
	PanelCustomize   PanelID = "customize"
	PanelOverview    PanelID = "overview"
	PanelDiagnostics PanelID = "diagnostics"
	PanelTriggers    PanelID = "triggers"
	PanelHaptics     PanelID = "haptics"
	PanelLighting    PanelID = "lighting"
	PanelAudio       PanelID = "audio"
	PanelSticks      PanelID = "sticks"
	PanelMotion      PanelID = "motion"
	PanelProfiles    PanelID = "profiles"
	PanelDevice      PanelID = "device"
	PanelFirmware    PanelID = "firmware"
	PanelSettings    PanelID = "settings"
	PanelLogs        PanelID = "logs"
)

// PanelSpec declares a panel: which capability gates it and in which modes it appears.
// An empty Capability means the panel is always available.
type PanelSpec struct {
	ID         PanelID
	Title      string
	EZTitle    string
	Capability device.Capability
	// Feature, when set, is additionally required (e.g. configurable motors).
	Feature device.Feature
	// Access, when set, requires at least one of these operations: a panel of
	// settings never appears for hardware TuiSense can only read.
	Access device.Access
	Modes  ModeMask
	// NeedsDevice hides an otherwise ungated panel when no controller is
	// attached (an empty capability set).
	NeedsDevice bool
}

// PanelCatalog is the single source of truth for panel availability. The UI
// never hardcodes per-controller panel lists; it filters this catalog by the
// active device's capabilities and the UX mode.
var PanelCatalog = []PanelSpec{
	{ID: PanelOverview, Title: "Overview", EZTitle: "Home", Modes: InBoth},
	{ID: PanelCustomize, Title: "Customize", Modes: InBoth, NeedsDevice: true},
	{ID: PanelDiagnostics, Title: "Diagnostics", EZTitle: "Diagnostics", Capability: device.CapDiagnostics, Modes: InBoth},
	{ID: PanelLighting, Title: "Lighting", EZTitle: "Lights", Capability: device.CapLighting, Access: device.AccessConfigDevice, Modes: InBoth},
	{ID: PanelHaptics, Title: "Haptics", EZTitle: "Vibration", Capability: device.CapHaptics, Feature: device.FeatMotorConfig, Access: device.AccessConfigDevice, Modes: InBoth},
	{ID: PanelTriggers, Title: "Triggers", EZTitle: "Triggers", Capability: device.CapAdaptiveTriggers, Access: device.AccessConfigDevice, Modes: InBoth},
	{ID: PanelSticks, Title: "Sticks", Capability: device.CapSticks, Feature: device.FeatDeadzones, Access: device.AccessPreview | device.AccessConfigSoftware, Modes: InAdvanced},
	{ID: PanelMotion, Title: "Motion", Capability: device.CapMotion, Access: device.AccessPreview | device.AccessConfigSoftware, Modes: InAdvanced},
	{ID: PanelAudio, Title: "Audio", Capability: device.CapAudio, Access: device.AccessConfigDevice, Modes: InAdvanced},
	{ID: PanelProfiles, Title: "Profiles", EZTitle: "Profiles", Capability: device.CapProfiles, Access: device.AccessConfigDevice | device.AccessConfigSoftware, Modes: InBoth},
	{ID: PanelDevice, Title: "Device", Capability: device.CapConnection, Modes: InAdvanced},
	{ID: PanelLogs, Title: "Logs", Capability: device.CapDiagnostics, Modes: InAdvanced},
	{ID: PanelSettings, Title: "Settings", EZTitle: "Settings", Modes: InBoth},
	// Last, so adding it did not shift the existing number hotkeys.
	// Device details and the read-only firmware check (the check itself is
	// shown only where a provider supports it).
	{ID: PanelFirmware, Title: "Device Details", EZTitle: "Device Details", Modes: InBoth, NeedsDevice: true},
}

// PanelEntry is a visible panel with its resolved support level and hotkey.
type PanelEntry struct {
	Spec    PanelSpec
	Support device.Support
	Hotkey  rune // '1'..'9', '0', or 0 when out of digits
}

// DisplayTitle returns the mode-appropriate title.
func (e PanelEntry) DisplayTitle(m UXMode) string {
	if m == ModeEZ && e.Spec.EZTitle != "" {
		return i18n.T(e.Spec.EZTitle)
	}
	return i18n.T(e.Spec.Title)
}

// VisiblePanels filters PanelCatalog by capabilities and mode, assigning
// sequential number hotkeys. Unsupported capabilities are hidden; Limited
// ones remain visible and are marked by their Support level.
func VisiblePanels(caps device.CapabilitySet, mode UXMode) []PanelEntry {
	digits := []rune("1234567890")
	var out []PanelEntry
	for _, spec := range PanelCatalog {
		if !spec.Modes.Includes(mode) {
			continue
		}
		if spec.NeedsDevice && len(caps) == 0 {
			continue
		}
		support := device.Full
		if spec.Capability != "" {
			if !caps.Supports(spec.Capability) {
				continue
			}
			if spec.Access != 0 && caps.Spec(spec.Capability).Access&spec.Access == 0 {
				continue
			}
			support = caps.Level(spec.Capability)
			if spec.Feature != "" && !caps.HasFeature(spec.Capability, spec.Feature) {
				continue
			}
		}
		e := PanelEntry{Spec: spec, Support: support}
		if len(out) < len(digits) {
			e.Hotkey = digits[len(out)]
		}
		out = append(out, e)
	}
	hasCustomize := false
	for _, e := range out {
		if e.Spec.ID != PanelCustomize && PrimaryPanel(e.Spec.ID) == PanelCustomize {
			hasCustomize = true
		}
	}
	if !hasCustomize {
		filtered := out[:0]
		for _, e := range out {
			if e.Spec.ID != PanelCustomize {
				filtered = append(filtered, e)
			}
		}
		out = filtered
	}

	return out
}

// PanelForCapability returns the panel that configures capability c.
func PanelForCapability(c device.Capability) (PanelID, bool) {
	for _, spec := range PanelCatalog {
		if spec.Capability == c && c != "" {
			return spec.ID, true
		}
	}
	return "", false
}

// PanelVisible reports whether id is in the visible list.
func PanelVisible(panels []PanelEntry, id PanelID) bool {
	for _, p := range panels {
		if p.Spec.ID == id {
			return true
		}
	}
	return false
}

// PrimaryPanel groups capabilities without removing their individual views.
func PrimaryPanel(id PanelID) PanelID {
	switch id {
	case PanelLighting, PanelHaptics, PanelTriggers, PanelSticks, PanelMotion, PanelAudio, PanelCustomize:
		return PanelCustomize
	case PanelDevice, PanelFirmware, PanelLogs, PanelSettings:
		return PanelSettings
	default:
		return id
	}
}

// PrimaryPanels provides stable shortcuts regardless of controller capabilities.
func (s *AppState) PrimaryPanels() []PanelEntry {
	ids := []PanelID{PanelOverview, PanelDiagnostics, PanelCustomize, PanelProfiles, PanelSettings}
	titles := []string{"Home", "Diagnostics", "Customize", "Profiles", "Device / Settings"}
	var out []PanelEntry
	for i, id := range ids {
		if PanelVisible(s.VisiblePanels(), id) {
			out = append(out, PanelEntry{Spec: PanelSpec{ID: id, Title: titles[i]}, Hotkey: rune('1' + i)})
		}
	}
	return out
}
