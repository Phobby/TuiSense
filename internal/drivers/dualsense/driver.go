package dualsense

import (
	"fmt"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// MockKey is the registry key of the simulated DualSense.
const MockKey = "mock:dualsense"

// Driver discovers and opens physical DualSense / DualSense Edge controllers
// through Linux hidraw. On other platforms Discover returns no candidates until
// a platform adapter is added (see docs/ARCHITECTURE.md, "Platform adapters").
type Driver struct{}

// NewDriver creates the DualSense hardware driver.
func NewDriver() *Driver { return &Driver{} }

func (*Driver) ID() string { return Family }

func (*Driver) Models() []device.Descriptor {
	return []device.Descriptor{Descriptor(), EdgeDescriptor()}
}

func (*Driver) Discover() ([]device.Candidate, error) {
	devs, err := system.ScanDualSenseDevices()
	if err != nil {
		return nil, err
	}
	out := make([]device.Candidate, 0, len(devs))
	for _, d := range devs {
		out = append(out, device.Candidate{
			Key:     "hidraw:" + d.HIDRawPath,
			Label:   fmt.Sprintf("%s (%s)", d.Model, d.Connection),
			Payload: d,
		})
	}
	return out, nil
}

func (*Driver) Open(c device.Candidate, logger *logging.Logger) (device.Device, error) {
	dev, ok := c.Payload.(system.DiscoveredDevice)
	if !ok {
		return nil, fmt.Errorf("dualsense: unexpected candidate payload %T", c.Payload)
	}
	return OpenLinuxBackend(dev, logger)
}

// MockVirtual describes the simulated DualSense for the registry.
func MockVirtual() device.VirtualDevice {
	return device.VirtualDevice{
		Key:        MockKey,
		Source:     device.SourceMock,
		Descriptor: Descriptor(),
		New: func(logger *logging.Logger) device.Device {
			return NewMockBackend(logger)
		},
	}
}

var (
	_ device.Driver    = (*Driver)(nil)
	_ device.Device    = (*LinuxBackend)(nil)
	_ device.Device    = (*MockBackend)(nil)
	_ device.Simulator = (*MockBackend)(nil)
)

var (
	_ device.ScenarioSimulator      = (*MockBackend)(nil)
	_ device.BatteryDetailsProvider = (*MockBackend)(nil)
	_ device.BatteryDetailsProvider = (*LinuxBackend)(nil)
)
