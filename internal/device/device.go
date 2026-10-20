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
	ID               string
	Name             string
	Model            string
	VendorID         uint16
	ProductID        uint16
	FirmwareVersion  string
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
	if spec.Support == Unsupported {
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
