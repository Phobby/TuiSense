//go:build linux

package dualsense

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system"
)

func TestRealBackendDoesNotInventBatteryOrUSBCharging(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake-hidraw")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	backend, err := OpenLinuxBackend(system.DiscoveredDevice{HIDRawPath: path, Connection: device.ConnUSB, VendorID: system.SonyVendorID, ProductID: system.DualSenseProductID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	tel := backend.Telemetry()
	if tel.BatteryPercent != -1 || tel.Charging {
		t.Fatalf("invented initial hardware telemetry: %+v", tel)
	}
}
