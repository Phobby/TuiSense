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
	"github.com/Phobby/TuiSense/internal/drivers/winpad"
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
		models := append([]gamepad.Model{msigc20.Model()}, padmodels.Models()...)
		_ = r.RegisterDriver(gamepad.NewDriver(models...))
		if runtime.GOOS == "windows" {
			// Windows: input through Microsoft's documented XInput API; the
			// model comes from Windows' HID listing (identity only, so the
			// DualSense family is matched by ID but never driven here).
			_ = r.RegisterDriver(xinput.NewDriver(WindowsModels()...))
			_ = r.RegisterDriver(winpad.NewDriver(WindowsModels()))
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

// WindowsModels are the model matchers used to identify Windows controllers.
func WindowsModels() []gamepad.Model {
	return append(append([]gamepad.Model{msigc20.Model()}, padmodels.Models()...),
		gamepad.FixedIDModel(dualsense.Descriptor()), gamepad.FixedIDModel(dualsense.EdgeDescriptor()))
}
