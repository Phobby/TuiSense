//go:build !linux

package dualsense

import (
	"fmt"
	"runtime"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// OpenLinuxBackend is unavailable until a platform adapter exists for this OS
// (see docs/ARCHITECTURE.md, "Platform adapters"); discovery finds no devices.
func OpenLinuxBackend(system.DiscoveredDevice, *logging.Logger) (device.Device, error) {
	return nil, fmt.Errorf("dualsense: hidraw backend is not available on %s", runtime.GOOS)
}
