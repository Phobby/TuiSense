package device

import (
	"context"
	"github.com/Phobby/TuiSense/internal/i18n"
	"time"
)

// ConnectionType represents how the controller is attached to the host.
type ConnectionType string

const (
	ConnDisconnected ConnectionType = "Disconnected"
	ConnUSB          ConnectionType = "USB-C Wired"
	ConnUSBWired     ConnectionType = "USB Wired"
	ConnBluetooth    ConnectionType = "Bluetooth"
	ConnWireless     ConnectionType = "Wireless Dongle"
	ConnMock         ConnectionType = "Simulated (Mock)"
)

// Info describes the dynamic identity and connection metadata of one device instance.
type Info struct {
	ID        string
	Name      string
	Model     string
	VendorID  uint16
	ProductID uint16
	// FirmwareVersion is the user-facing firmware version (for Sony
	// controllers, the update version that Sony's updater compares).
	FirmwareVersion string
	// FirmwareBuild is a separate firmware build identifier when the device
	// reports one (e.g. the DualSense's 32-bit main firmware version).
	FirmwareBuild    string
	HardwareVersion  string
	MACAddress       string
	HIDRawPath       string
	InputDevicePath  string
	SysfsPath        string
	Connection       ConnectionType
	InputMode        string
	OutputMode       string
	Connected        bool
	ReadOnly         bool
	PermissionDenied bool
	PermissionHint   string
	LastSeen         time.Time
	// KernelDriver is the OS driver bound to the device (e.g. "xpad", "hid-generic").
	KernelDriver string
	// EventDriven is true when the device only reports changes (Linux evdev),
	// so a quiet input stream is normal and not a connection problem.
	EventDriven bool
	// Identification explains how the model was recognized (shown to the user).
	Identification string
	// IdentityKey is a non-unique hardware key (vid:pid:version) used to
	// remember the user's model confirmation. Empty when not applicable.
	IdentityKey string
	// IdentityState is "identified", "probable", "confirmed" or "unidentified"
	// for devices whose model is inferred; empty for fixed-ID devices.
	IdentityState string
	// IdentityCandidate names a model the user may confirm (ID and display name).
	IdentityCandidateID   string
	IdentityCandidateName string
	// Details are extra, privacy-conscious connection facts for display.
	Details []InfoField
}

// InfoField is one labeled fact about a device.
type InfoField struct {
	Label string
	Value string
}

// Device is the hardware abstraction implemented by every controller backend:
// real drivers, mirrored mocks, and pure UI test devices alike.
type Device interface {
	// Descriptor returns the static model description (regions, visual, baseline caps).
	Descriptor() Descriptor
	// Info returns live identity and connection metadata.
	Info() Info
	// Capabilities returns the runtime capability set, which may be narrower than
	// the descriptor's baseline (e.g. speaker disabled over Bluetooth).
	Capabilities() CapabilitySet
	Telemetry() Telemetry
	CurrentConfig() Config
	Poll(ctx context.Context) (Telemetry, error)
	ApplyConfig(ctx context.Context, cfg Config) error
	ExecuteCommand(ctx context.Context, cmd Command) error
	Close() error
}

// Simulator is optionally implemented by mock/test devices to support the
// hot-plug, transport, and permission simulation controls in the UI.
type Simulator interface {
	ToggleConnection() bool
	CycleTransport() ConnectionType
	TogglePermissionError() bool
}

// Availability resolves whether capability c is usable right now on a device,
// combining connection/permission state with declared support. It returns the
// support level and a human-readable reason when not fully available.
func Availability(info Info, caps CapabilitySet, c Capability) (Support, string) {
	spec := caps.Spec(c)
	if !spec.Accessible() {
		if spec.Note != "" {
			return Unsupported, i18n.T(spec.Note)
		}
		return Unsupported, i18n.T("Not supported by this controller.")
	}
	if !info.Connected {
		return Unsupported, i18n.T("Controller is disconnected.")
	}
	if info.PermissionDenied {
		return Limited, i18n.Tf("Write access to %s denied. Install the udev rules in contrib/70-tuisense-dualsense.rules.", info.HIDRawPath)
	}
	return spec.Support, i18n.T(spec.Note)
}

// BatteryAttribute is one battery-related value exposed by the device or the OS.
type BatteryAttribute struct {
	Name   string // e.g. "voltage_now"
	Value  string // as reported, with unit when known
	Source string // e.g. "sysfs power_supply", "input report"
}

// BatteryDetailsProvider is optionally implemented by devices that can expose
// additional battery attributes. Only genuinely reported values may be returned.
type BatteryDetailsProvider interface {
	BatteryDetails() []BatteryAttribute
}

// FirmwareIdentity is firmware data a driver read from the device itself, used
// by read-only update checks. Drivers fill only what they actually read and
// validated; nothing here is inferred or defaulted.
type FirmwareIdentity struct {
	// Read is true when the identity was read from the device and validated.
	Read bool
	// Source describes how it was read (e.g. "HID feature report 0x20").
	Source string
	// Error explains why it could not be read (when Read is false).
	Error string
	// UpdateVersion is the version that the vendor's update metadata compares
	// against. It is only meaningful when HasUpdateVersion is true.
	UpdateVersion    uint32
	HasUpdateVersion bool
	// Target is the vendor firmware update target, set only when the device
	// reports it in a documented, verified form ("" = unknown).
	Target string
}

// FirmwareIdentityProvider is optionally implemented by devices that can read
// their firmware identity. Implementations must only read from the device.
type FirmwareIdentityProvider interface {
	FirmwareIdentity() FirmwareIdentity
}

// DetailGroup groups device details for display.
type DetailGroup string

const (
	DetailDevice   DetailGroup = "device"
	DetailFirmware DetailGroup = "firmware"
	DetailHardware DetailGroup = "hardware"
	DetailDriver   DetailGroup = "driver"
)

// Detail is one reading about a device together with where it came from.
// Drivers only report values they actually read; a value whose meaning is
// not confirmed by a reliable source is marked Raw and labeled as such.
type Detail struct {
	Group DetailGroup
	Key   string // stable identifier (tests, accessibility)
	Label string
	Value string
	// Origin says where the value was read (e.g. "sysfs firmware_version").
	Origin string
	// Note explains the value (its meaning, or why it is unavailable).
	Note string
	// Raw marks a value whose meaning is not verified.
	Raw bool
	// Unavailable marks a field that could not be read; Value explains why.
	Unavailable bool
	// Advanced details are hidden in EZ mode.
	Advanced bool
}

// DetailsProvider is optionally implemented by devices that can describe
// their hardware and firmware in detail. Implementations only read.
type DetailsProvider interface {
	Details() []Detail
}

// InputScenario selects simulated input behavior on mock/test devices so the
// diagnostics UI can be exercised without hardware.
type InputScenario string

const (
	ScenarioDemo     InputScenario = "demo"     // continuous showcase motion
	ScenarioIdle     InputScenario = "idle"     // inputs released (with simulated noise/offset)
	ScenarioMove     InputScenario = "move"     // sticks rotate through full range
	ScenarioButtons  InputScenario = "buttons"  // declared buttons pressed one by one
	ScenarioTriggers InputScenario = "triggers" // triggers sweep full travel
	ScenarioTouch    InputScenario = "touch"    // touch contacts move and click
	ScenarioStill    InputScenario = "still"    // controller resting flat
	ScenarioMotion   InputScenario = "motion"   // controller rotated on every axis
	ScenarioSnapback InputScenario = "snapback" // sticks pushed to the edge and released suddenly
)

// ScenarioSimulator is implemented by mock/test devices only.
type ScenarioSimulator interface {
	SetInputScenario(InputScenario)
}
