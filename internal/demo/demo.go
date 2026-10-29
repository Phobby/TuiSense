// Package demo is TuiSense's web-demo device backend (tuisense --web-demo).
//
// The web demo runs the normal TuiSense interface; only the devices differ.
// Every controller here is simulated in memory:
//
//   - Sony DualSense and DualSense Edge use the DualSense mock backend.
//   - The models served by Linux kernel drivers are described by their real
//     descriptor builders (padmodels, msigc20) from a synthetic input layout
//     that matches what their kernel driver reports, so the capabilities,
//     regions, labels and visuals are exactly what a connected controller
//     of that model gets, including features TuiSense cannot reach yet.
//
// The registry built here contains no hardware driver: nothing in this
// package enumerates, opens or writes a host device, and no command it
// receives leaves the process.
package demo

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/padmodels"
	"github.com/Phobby/TuiSense/internal/logging"
)

// KeyPrefix starts the session key of every web-demo controller.
const KeyPrefix = "demo:"

// DefaultKey is the controller the web demo opens with.
const DefaultKey = KeyPrefix + dualsense.ModelDualSense

// Banner is the indicator the interface shows while running the web demo.
const Banner = "WEB DEMO — SIMULATED CONTROLLERS"

// Model is one simulated controller of the web demo.
type Model struct {
	Key     string
	ModelID string
	Name    string
}

// pad describes the input layout a model's kernel driver reports.
type pad struct {
	spec   gamepad.ModelSpec
	layout layout
	conn   string // "usb" or "bluetooth"
}

var pads = []pad{
	{padmodels.XboxSeries, xpadOne, "bluetooth"},
	{padmodels.XboxElite2, xpadOne, "usb"},
	{padmodels.DualShock4, playstation, "usb"},
	{padmodels.SwitchPro, nintendo, "bluetooth"},
	{padmodels.EightBitDoUltimate, xpadOne, "usb"},
	{padmodels.GameSirG7SE, xpadOne, "usb"},
	{padmodels.WolverineV3Pro, xpadOne, "usb"},
	{msigc20.Spec, xpad360, "usb"},
}

// Models lists the simulated controllers in the order the selector shows them.
func Models() []Model {
	out := []Model{
		{KeyPrefix + dualsense.ModelDualSense, dualsense.ModelDualSense, dualsense.Descriptor().DisplayName},
		{KeyPrefix + dualsense.ModelDualSenseEdge, dualsense.ModelDualSenseEdge, dualsense.EdgeDescriptor().DisplayName},
	}
	for _, p := range pads {
		out = append(out, Model{KeyPrefix + p.spec.ID, p.spec.ID, p.spec.DisplayName})
	}
	return out
}

// NewRegistry returns the web demo's registry: the ten simulated controllers
// and no hardware driver.
func NewRegistry() *device.Registry {
	r := device.NewRegistry()
	_ = r.RegisterVirtual(dualsense.DemoVirtual(KeyPrefix+dualsense.ModelDualSense, false))
	_ = r.RegisterVirtual(dualsense.DemoVirtual(KeyPrefix+dualsense.ModelDualSenseEdge, true))
	for i := range pads {
		p := pads[i]
		key := KeyPrefix + p.spec.ID
		desc, _ := describe(p, connFor(p.conn))
		_ = r.RegisterVirtual(device.VirtualDevice{Key: key, Source: device.SourceSimulated, Descriptor: desc, AutoAttach: true,
			New: func(logger *logging.Logger) device.Device { return newPad(p, key, int64(i+101), logger) }})
	}
	return r
}

// IsDemoKey reports whether key belongs to a web-demo controller.
func IsDemoKey(key string) bool {
	return len(key) > len(KeyPrefix) && key[:len(KeyPrefix)] == KeyPrefix
}
