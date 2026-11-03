package winpad

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system/winhid"
)

// DriverID is the registry ID of the HID game-controller driver.
const DriverID = "windows-hid"

// ModelID is the descriptor of an unidentified HID game controller.
const ModelID = "windows.hid_game_controller"

// Backend name shown in Driver & Compatibility.
const Backend = "windows-hid-identity"

// NoInputNote is the user-facing state of every device of this driver.
const NoInputNote = "Controller detected, but input access is unavailable. Open Driver & Compatibility for details."

const inaccessibleNote = "Windows lists this controller as a HID (DirectInput-style) game controller. TuiSense reads controller input on Windows only through XInput so far, so this feature is not available in the controller's current mode."

// Descriptor is the neutral description of a HID game controller TuiSense
// can identify but not read: connection facts only, no diagnostics, panels
// or commands.
func Descriptor() device.Descriptor {
	return device.Descriptor{
		ModelID: ModelID, Family: "generic", Brand: "Generic", DisplayName: "Generic Gamepad", ShortName: "Gamepad",
		VisualID: "generic", Accent: device.RGB{R: 140, G: 150, B: 175},
		Capabilities: device.NewCapabilitySet(
			device.CapabilitySpec{ID: device.CapConnection, Support: device.Limited, Access: device.AccessRead, Backend: Backend,
				Verification: device.Experimental, Note: NoInputNote},
		),
	}
}

// Driver lists HID game controllers that Windows does not serve through
// XInput (XInput-class entries belong to the XInput driver).
type Driver struct {
	models []gamepad.Model
	list   func() ([]winhid.Device, error)
}

// NewDriver uses the real HID enumeration (empty on other systems).
func NewDriver(models []gamepad.Model) *Driver {
	return &Driver{models: models, list: winhid.Enumerate}
}

// NewDriverWith uses an explicit listing (tests).
func NewDriverWith(models []gamepad.Model, list func() ([]winhid.Device, error)) *Driver {
	return &Driver{models: models, list: list}
}

func (*Driver) ID() string                  { return DriverID }
func (*Driver) Models() []device.Descriptor { return []device.Descriptor{Descriptor()} }

func (d *Driver) Discover() ([]device.Candidate, error) {
	all, err := d.list()
	if err != nil {
		return nil, err
	}
	var out []device.Candidate
	for _, h := range winhid.GameControllers(all) {
		if h.XInput() {
			continue
		}
		id := Resolve(d.models, h)
		out = append(out, device.Candidate{Key: "winhid:" + h.ShortID(), Endpoint: id.Endpoint(),
			Label: "HID game controller " + h.HardwareKey(), Payload: id})
	}
	return out, nil
}

func (d *Driver) Open(c device.Candidate, logger *logging.Logger) (device.Device, error) {
	id, ok := c.Payload.(Identity)
	if !ok {
		return nil, errors.New("windows-hid: unexpected candidate payload")
	}
	b := &hidDevice{desc: Descriptor()}
	conn := device.ConnUSBWired
	if id.HID.Bluetooth() {
		conn = device.ConnBluetooth
	}
	b.info = device.Info{ID: c.Key, Name: "HID game controller", Connection: conn, Connected: true, ReadOnly: true, LastSeen: time.Now(),
		InputMode: "HID (DirectInput-style)", OutputMode: "none", PermissionHint: NoInputNote,
		Details: []device.InfoField{{Label: "Input", Value: "Not readable in this mode"}}}
	id.Apply(&b.desc, &b.info, device.RequiresPlatform, inaccessibleNote)
	if logger != nil {
		logger.Info("WINHID", "Detected %s (%s, %s) without input access", b.desc.DisplayName, id.HID.HardwareKey(), b.info.IdentityState)
	}
	return b, nil
}

// hidDevice is a detected controller without an input backend.
type hidDevice struct {
	mu   sync.Mutex
	desc device.Descriptor
	info device.Info
	cfg  device.Config
}

func (b *hidDevice) Descriptor() device.Descriptor      { return b.desc }
func (b *hidDevice) Capabilities() device.CapabilitySet { return b.desc.Capabilities }
func (b *hidDevice) Info() device.Info {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.info
}
func (b *hidDevice) Telemetry() device.Telemetry { return device.Telemetry{BatteryPercent: -1} }
func (b *hidDevice) CurrentConfig() device.Config {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cfg
}

// Poll reports no input; disconnection is noticed by the hot-plug scan.
func (b *hidDevice) Poll(context.Context) (device.Telemetry, error) { return b.Telemetry(), nil }

func (b *hidDevice) ApplyConfig(_ context.Context, cfg device.Config) error {
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
	return nil
}

// ExecuteCommand refuses everything: no output report is ever sent.
func (b *hidDevice) ExecuteCommand(context.Context, device.Command) error {
	return errors.New("this controller's input mode has no TuiSense backend on Windows")
}

func (b *hidDevice) Close() error { return nil }

var _ device.Driver = (*Driver)(nil)
var _ device.Device = (*hidDevice)(nil)
