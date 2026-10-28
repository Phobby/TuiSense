package device

// RegionID identifies an interactive anatomical zone on a controller visual.
// Region IDs are semantic and shared across controller families so that the
// same UI behavior (e.g. "left trigger" → trigger settings) works everywhere;
// each Descriptor declares which regions its controller actually has.
type RegionID string

const (
	RegionNone         RegionID = ""
	RegionLeftTrigger  RegionID = "left_trigger"
	RegionRightTrigger RegionID = "right_trigger"
	RegionLight        RegionID = "light" // lightbar / touchpad / guide LED
	RegionLeftStick    RegionID = "left_stick"
	RegionRightStick   RegionID = "right_stick"
	RegionHaptics      RegionID = "haptics"
	RegionMotion       RegionID = "motion"
	RegionAudio        RegionID = "audio" // mic mute button / headset jack
)

// Region describes an interactive controller zone and the capability it configures.
type Region struct {
	ID          RegionID
	Label       string     // e.g. "L2 Adaptive Trigger"
	Badge       string     // short callout text, e.g. "L2 Trigger"
	Description string     // what selecting this region configures
	Capability  Capability // capability whose panel opens on selection
}

// Source classifies where a device comes from.
type Source string

const (
	SourceHardware Source = "hardware" // real device opened by a driver
	SourceMock     Source = "mock"     // simulated device mirroring a real driver
	SourceTest     Source = "test"     // UI/architecture test device with no real backend
)

// RGB is a plain 24-bit color (kept UI-framework agnostic).
type RGB struct{ R, G, B uint8 }

// Action is a quick, one-shot operation a controller offers (e.g. a test pulse).
type Action struct {
	ID         string
	Label      string
	Capability Capability
	Command    Command
}

// Descriptor is the static description of a controller model: identity,
// branding hints, visual model, regions, baseline capabilities, and actions.
// Descriptors are pure data so they can be unit tested and rendered without hardware.
type Descriptor struct {
	ModelID     string // stable unique ID, e.g. "sony.dualsense"
	Family      string // config/profile family, e.g. "dualsense", "xbox"
	Brand       string // e.g. "Sony", "Microsoft"
	DisplayName string // e.g. "DualSense Wireless Controller"
	ShortName   string // e.g. "DualSense"
	VisualID    string // key into the UI visual registry
	Accent      RGB    // restrained per-device accent color
	VendorID    uint16
	ProductIDs  []uint16

	Regions      []Region
	Inputs       InputLayout // diagnostics contract: buttons, ranges, touchpad, audio match
	Capabilities CapabilitySet
	Actions      []Action
	// DefaultConfig builds the factory config; nil means device.DefaultConfig.
	DefaultConfig func() Config
	// DefaultProfile names the built-in profile applied on first use ("" = factory config).
	DefaultProfile string
	// LinuxDrivers are the kernel drivers known to serve this model (used by
	// Driver & Compatibility to spot an unexpected binding).
	LinuxDrivers []string
	// ModeGuide lists the controller's physical input modes (how to switch,
	// how to recognize them) for troubleshooting. Empty when not applicable.
	ModeGuide []string
}

// Region returns the region with id, if declared.
func (d Descriptor) Region(id RegionID) (Region, bool) {
	for _, r := range d.Regions {
		if r.ID == id {
			return r, true
		}
	}
	return Region{}, false
}

// RegionIDs lists declared region IDs in keyboard cycling order.
func (d Descriptor) RegionIDs() []RegionID {
	out := make([]RegionID, len(d.Regions))
	for i, r := range d.Regions {
		out[i] = r.ID
	}
	return out
}

// NewDefaultConfig returns the descriptor's default config, or the global default.
func (d Descriptor) NewDefaultConfig() Config {
	if d.DefaultConfig != nil {
		return d.DefaultConfig()
	}
	return DefaultConfig()
}

// GenericGamepadModelID is the descriptor ID of unidentified gamepads. As an
// identity confirmation it means "this is not one of the known models".
const GenericGamepadModelID = "generic.evdev_gamepad"

// PlaceholderDescriptor is used when no controller is active. It keeps the UI
// renderable (dimmed visual, empty capabilities) without special-casing nil.
func PlaceholderDescriptor() Descriptor {
	return Descriptor{
		ModelID:      "none",
		Family:       "none",
		DisplayName:  "No Controller",
		ShortName:    "No Controller",
		VisualID:     "generic",
		Accent:       RGB{110, 120, 150},
		Capabilities: NewCapabilitySet(),
	}
}
