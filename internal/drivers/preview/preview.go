// Package preview turns a controller model into a settings preview: a device
// that the whole interface can configure, while nothing it is told leaves
// TuiSense. The Controller Gallery uses it so every model's settings can be
// explored without owning the controller, on every operating system.
package preview

import (
	"context"
	"sync"

	"github.com/Phobby/TuiSense/internal/device"
)

// Key returns the session key of the preview for a model.
func Key(modelID string) string { return "preview:" + modelID }

// Device previews desc. It reports no input, battery or firmware: those
// need the real controller.
type Device struct {
	desc device.Descriptor
	caps device.CapabilitySet

	mu  sync.Mutex
	cfg device.Config
}

// New returns a preview of desc with its wired capability set, minus what
// only a connected controller can provide.
func New(desc device.Descriptor) *Device {
	caps := desc.Capabilities.ForConnection(device.ConnUSB)
	for _, c := range []device.Capability{device.CapBattery, device.CapFirmwareInfo, device.CapFirmwareCheck, device.CapFirmwareUpdate} {
		delete(caps, c)
	}
	return &Device{desc: desc, caps: caps, cfg: desc.NewDefaultConfig()}
}

func (d *Device) Descriptor() device.Descriptor { return d.desc }

func (d *Device) Info() device.Info {
	return device.Info{ID: Key(d.desc.ModelID), Name: d.desc.DisplayName, Model: d.desc.DisplayName,
		Connection: device.ConnUSB, Connected: true, OutputMode: "preview (nothing is sent)"}
}

func (d *Device) Capabilities() device.CapabilitySet { return d.caps }
func (d *Device) Telemetry() device.Telemetry        { return device.Telemetry{} }

func (d *Device) CurrentConfig() device.Config {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cfg
}

func (d *Device) Poll(context.Context) (device.Telemetry, error) { return device.Telemetry{}, nil }

// ApplyConfig keeps the configuration in memory only.
func (d *Device) ApplyConfig(_ context.Context, cfg device.Config) error {
	d.mu.Lock()
	d.cfg = cfg
	d.mu.Unlock()
	return nil
}

// ExecuteCommand accepts every authorised command and sends it nowhere.
func (d *Device) ExecuteCommand(context.Context, device.Command) error { return nil }

func (d *Device) Close() error { return nil }
