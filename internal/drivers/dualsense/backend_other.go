//go:build !linux && !(darwin && cgo)

package dualsense

import (
	"fmt"
	"runtime"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// No raw HID adapter on this OS (Windows identifies the DualSense through its
// HID listing instead; macOS needs a cgo build): discovery finds nothing.
const rawHID = false

func scanDevices() ([]system.DiscoveredDevice, error) { return nil, nil }

func openBackend(system.DiscoveredDevice, *logging.Logger) (device.Device, error) {
	return nil, fmt.Errorf("dualsense: no raw HID backend on %s in this build", runtime.GOOS)
}
