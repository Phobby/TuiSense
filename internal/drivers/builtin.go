// Package drivers wires every built-in controller driver and virtual device
// into a device.Registry. Adding a new controller family is one line here.
package drivers

import (
	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/drivers/dualsense"
	"github.com/ygzztrk/tuisense/internal/drivers/xboxmock"
)

// RegistryOptions selects which built-ins are registered.
type RegistryOptions struct {
	Hardware    bool // register real hardware drivers
	TestDevices bool // register UI/architecture test devices (mock Xbox)
}

// NewBuiltinRegistry returns a registry containing TuiSense's built-in drivers.
func NewBuiltinRegistry(opts RegistryOptions) *device.Registry {
	r := device.NewRegistry()
	if opts.Hardware {
		_ = r.RegisterDriver(dualsense.NewDriver())
	}
	// The simulated DualSense is always registered; the session decides
	// whether to attach it (forced --mock, or fallback when no hardware).
	_ = r.RegisterVirtual(dualsense.MockVirtual())
	if opts.TestDevices {
		_ = r.RegisterVirtual(xboxmock.Virtual())
	}
	return r
}
