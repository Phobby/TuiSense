package dualsense

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// MockKey is the registry key of the simulated DualSense.
const MockKey = "mock:dualsense"

// TestKey identifies the explicit, always-listed DualSense test device.
const TestKey = "test:dualsense"

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
	for _, d := range uniqueDevices(devs) {
		out = append(out, device.Candidate{
			Key:      physicalKey(d),
			Endpoint: d.HIDRawPath,
			Label:    fmt.Sprintf("%s (%s)", d.Model, d.Connection),
			Payload:  d,
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
	_ device.Device    = (*MockBackend)(nil)
	_ device.Simulator = (*MockBackend)(nil)
)

var (
	_ device.ScenarioSimulator        = (*MockBackend)(nil)
	_ device.BatteryDetailsProvider   = (*MockBackend)(nil)
	_ device.FirmwareIdentityProvider = (*MockBackend)(nil)
	_ device.DetailsProvider          = (*MockBackend)(nil)
)

// TestVirtual lists a simulated DualSense alongside the other test controllers,
// even when real hardware is connected or automatic mock fallback is disabled.
func TestVirtual() device.VirtualDevice {
	desc := Descriptor()
	desc.ShortName = "DualSense (Test)"
	desc.DisplayName = "Sony DualSense (Test)"
	return device.VirtualDevice{Key: TestKey, Source: device.SourceTest, Descriptor: desc, AutoAttach: true,
		New: func(logger *logging.Logger) device.Device { return NewTestBackend(logger) }}
}

// physicalKey uses only a validated unique address, never a model name.
// Hashing avoids exposing controller addresses in selector and preference keys.
func physicalKey(d system.DiscoveredDevice) string {
	raw := strings.TrimSpace(d.MACAddress)
	if mac, err := net.ParseMAC(raw); err == nil && len(mac) == 6 {
		valid := false
		for _, b := range mac {
			valid = valid || b != 0
		}
		if valid && mac.String() != "ff:ff:ff:ff:ff:ff" {
			return fmt.Sprintf("dualsense:%x", sha256.Sum256([]byte(mac.String())))
		}
	}
	if d.SysfsPath != "" {
		if resolved, err := filepath.EvalSymlinks(filepath.Join(d.SysfsPath, "device")); err == nil {
			identity := resolved
			// A USB device ancestor joins HID interfaces, but never identical models on different ports.
			for parent := resolved; parent != "/" && parent != "."; parent = filepath.Dir(parent) {
				if _, err := os.Stat(filepath.Join(parent, "idVendor")); err == nil {
					if _, err := os.Stat(filepath.Join(parent, "idProduct")); err == nil {
						identity = parent
						break
					}
				}
			}
			return fmt.Sprintf("dualsense:sysfs:%x", sha256.Sum256([]byte(identity)))
		}
	}
	return "hidraw:" + d.HIDRawPath
}

// Prefer an accessible USB interface when two transports identify one pad.
func uniqueDevices(in []system.DiscoveredDevice) []system.DiscoveredDevice {
	var out []system.DiscoveredDevice
	seen := map[string]int{}
	score := func(d system.DiscoveredDevice) int {
		v := 0
		if d.Readable {
			v += 4
		}
		if d.Writable {
			v += 2
		}
		if d.Connection == device.ConnUSB {
			v++
		}
		return v
	}
	for _, d := range in {
		key := physicalKey(d)
		if i, ok := seen[key]; ok {
			if score(d) > score(out[i]) {
				out[i] = d
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, d)
	}
	return out
}
