package dualsense

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/simulate"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// DemoVirtual describes the simulated DualSense (edge=false) or DualSense
// Edge (edge=true) of the web demo under key. It is the same mock backend
// the --mock mode uses, with the Edge's descriptor, inputs and identity
// when edge is set. Nothing it is told leaves the process.
func DemoVirtual(key string, edge bool) device.VirtualDevice {
	desc := Descriptor()
	if edge {
		desc = EdgeDescriptor()
	}
	return device.VirtualDevice{Key: key, Source: device.SourceSimulated, Descriptor: desc, AutoAttach: true,
		New: func(logger *logging.Logger) device.Device { return NewDemoBackend(logger, key, edge) }}
}

// NewDemoBackend creates a web-demo DualSense or DualSense Edge.
func NewDemoBackend(logger *logging.Logger, key string, edge bool) *MockBackend {
	m := newMockBackend(logger, false)
	m.info.ID = key
	m.info.HIDRawPath = "web demo (simulated, no device node)"
	m.info.InputDevicePath = "web demo (simulated, no device node)"
	m.info.SysfsPath = ""
	m.info.MACAddress = "02:00:00:00:10:01"
	if !edge {
		m.rebuildDetails()
		return m
	}
	m.desc = EdgeDescriptor()
	m.gen = simulate.New(m.desc.Inputs, simulate.Profile{
		StickOffset: [4]float64{-0.006, 0.01, 0.004, -0.009},
		StickNoise:  0.5,
		GyroNoise:   2,
		GyroBias:    [3]float64{-2, 3, 1},
		GravityAxis: 1, GravitySign: 1,
	}, 17)
	m.info.Name = "Sony Interactive Entertainment DualSense Edge Wireless Controller"
	m.info.Model = "DualSense Edge Wireless Controller (CFI-ZCP1)"
	m.info.ProductID = system.DualSenseEdgePID
	m.info.MACAddress = "02:00:00:00:10:02"
	m.tel.BatteryPercent = 71
	m.rebuildDetails()
	return m
}

// rebuildDetails refreshes the simulated identification data after the
// identity changed.
func (m *MockBackend) rebuildDetails() {
	fi := FirmwareInfo{BuildDate: "Jun  5 2024", BuildTime: "11:24:13", HardwareInfo: 0x00000311, FirmwareVersion: 0x0110002A, UpdateVersion: 0x0630}
	m.details = BuildDetails(DetailsInput{
		Info: m.info, Firmware: &fi, FirmwareSource: "simulated report 0x20", Simulated: true,
		Sysfs: system.HIDSysfsInfo{Found: true, Driver: "playstation", Name: m.info.Name, Attrs: map[string]string{
			SysfsFirmwareVersion: "0x0110002a", SysfsHardwareVersion: "0x00000311",
		}},
	})
}
