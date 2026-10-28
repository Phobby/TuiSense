// Package drivers wires every built-in controller driver and virtual device
// into a device.Registry. Adding a new controller family is one line here.
package drivers

import (
	"runtime"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/padmodels"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
	"github.com/Phobby/TuiSense/internal/drivers/xinput"
)

// RegistryOptions selects which built-ins are registered.
type RegistryOptions struct {
	Mock        bool // register the explicit demo factory
	Hardware    bool // register real hardware drivers
	TestDevices bool // register UI/architecture test devices (simulated Xbox and DualSense)
}

// NewBuiltinRegistry returns a registry containing TuiSense's built-in drivers.
func NewBuiltinRegistry(opts RegistryOptions) *device.Registry {
	r := device.NewRegistry()
	if opts.Hardware {
		_ = r.RegisterDriver(dualsense.NewDriver())
		// Generic Linux evdev gamepads, with model matchers for real
		// controllers that have no dedicated protocol driver.
		_ = r.RegisterDriver(gamepad.NewDriver(append([]gamepad.Model{msigc20.Model()}, padmodels.Models()...)...))
		if runtime.GOOS == "windows" {
			// Windows: XInput controllers through Microsoft's documented API.
			_ = r.RegisterDriver(xinput.NewDriver())
		}
	}
	// Virtual factories are absent from normal startup.
	if opts.Mock || opts.TestDevices {
		_ = r.RegisterVirtual(dualsense.MockVirtual())
	}
	if opts.TestDevices {
		_ = r.RegisterVirtual(xboxmock.Virtual())
		_ = r.RegisterVirtual(dualsense.TestVirtual())
	}
	return r
}
