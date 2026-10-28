package gamepad

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// DriverID is the registry ID of the evdev gamepad driver.
const DriverID = "evdev_gamepad"

// excludedIDs are handled by dedicated drivers: the DualSense and DualSense
// Edge use the hidraw protocol driver. Other Sony controllers (DualShock 4)
// are served here through the kernel's hid-playstation / hid-sony drivers.
var excludedIDs = map[[2]uint16]bool{{0x054c, 0x0ce6}: true, {0x054c, 0x0df2}: true}

// Discovered is the payload of a candidate.
type Discovered struct {
	Dev        evdev.InputDevice
	Resolution Resolution
}

// Driver discovers evdev gamepads and opens them with the best matching model.
type Driver struct {
	models  []Model
	sysRoot string
	devRoot string
	// IncludeVirtual also lists software-created pads (uinput). Off by
	// default so Steam Input's virtual controllers are never mistaken for
	// hardware; TUISENSE_INCLUDE_VIRTUAL_INPUT=1 enables it for development.
	IncludeVirtual bool
}

// NewDriver creates the driver with the given model matchers.
func NewDriver(models ...Model) *Driver {
	return &Driver{models: models, sysRoot: "/sys", devRoot: "/dev/input", IncludeVirtual: os.Getenv("TUISENSE_INCLUDE_VIRTUAL_INPUT") == "1"}
}

func (*Driver) ID() string { return DriverID }

func (d *Driver) Models() []device.Descriptor {
	out := []device.Descriptor{StaticGenericDescriptor()}
	for _, m := range d.models {
		out = append(out, m.Static)
	}
	return out
}

// Scan returns every gamepad node with its identification (used by
// Discover and by `tuisense debug devices`).
func (d *Driver) Scan() ([]Discovered, error) {
	devs, err := evdev.Scan(d.sysRoot, d.devRoot)
	if err != nil {
		return nil, err
	}
	var out []Discovered
	seen := map[string]bool{}
	for _, dev := range devs {
		if !dev.IsGamepad() || excludedIDs[[2]uint16{dev.Vendor, dev.Product}] || (dev.Virtual() && !d.IncludeVirtual) {
			continue
		}
		if dev.SysfsPath != "" {
			if seen[dev.SysfsPath] {
				continue
			}
			seen[dev.SysfsPath] = true
		}
		out = append(out, Discovered{Dev: dev, Resolution: Resolve(d.models, dev)})
	}
	return out, nil
}

// CandidateKey builds the registry key. The model is part of the key, so a
// confirmed identification re-opens the device with the new descriptor.
func CandidateKey(eventPath, modelID string) string { return "evdev:" + eventPath + "#" + modelID }

func (d *Driver) Discover() ([]device.Candidate, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}
	found, err := d.Scan()
	if err != nil {
		return nil, err
	}
	out := make([]device.Candidate, 0, len(found))
	for _, f := range found {
		id, name := GenericModelID, f.Dev.Name
		if m := f.Resolution.Model; m != nil {
			id, name = m.ID, m.Static.DisplayName
		}
		out = append(out, device.Candidate{
			Key:     CandidateKey(f.Dev.EventPath, id),
			Label:   fmt.Sprintf("%s (%s, %s)", name, f.Dev.EventName, f.Dev.HardwareKey()),
			Payload: f,
		})
	}
	return out, nil
}

func (d *Driver) Open(c device.Candidate, logger *logging.Logger) (device.Device, error) {
	f, ok := c.Payload.(Discovered)
	if !ok {
		return nil, fmt.Errorf("gamepad: unexpected candidate payload %T", c.Payload)
	}
	return OpenBackend(f, logger)
}

// LayoutLabel describes an observed layout in plain words.
func LayoutLabel(l Layout) string {
	switch l {
	case LayoutXInput:
		return "XInput-style layout"
	case LayoutJoystick:
		return "Generic joystick (DirectInput-style)"
	}
	return "HID gamepad"
}

// usbStrings joins the USB manufacturer and product strings.
func usbStrings(u *evdev.USBInfo) string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(u.Manufacturer + " " + u.Product)
}

var _ device.Driver = (*Driver)(nil)
