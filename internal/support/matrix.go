// Package support generates the controller support matrix from the actual
// driver descriptors, so documentation and runtime can never disagree.
package support

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/padmodels"
	"github.com/Phobby/TuiSense/internal/system/evdev"
	"github.com/Phobby/TuiSense/internal/ui/visual"
)

// Row is one controller model.
type Row struct {
	Model        string
	ModelID      string
	Visual       string
	Detection    string
	Platforms    string
	Connections  string
	Backend      string
	Drivers      string
	Input        string
	Battery      string
	Vibration    string
	Lighting     string
	Triggers     string
	Rear         string
	Remapping    string
	Profiles     string
	Firmware     string
	Verification string
}

// cell describes one capability in support-matrix words.
func cell(caps device.CapabilitySet, c device.Capability) string {
	s := caps.Spec(c)
	switch s.State() {
	case device.StateHardwareAbsent:
		return "—"
	case device.StateConfigurableDevice:
		return "Implemented (configures device)"
	case device.StateConfigurableSoftware:
		return "Implemented (software)"
	case device.StatePreviewOnly:
		return "Preview only"
	case device.StateTestable:
		return "Implemented (read/test)"
	case device.StateReadOnly:
		return "Implemented (read-only)"
	case device.StateRequiresDriver:
		return "Depends on OS driver"
	}
	if s.Requires == device.RequiresConnection {
		return "Other connection"
	}
	return "Not implemented"
}

func verification(caps device.CapabilitySet) string {
	worst := device.VerifiedOnHardware
	for _, s := range caps {
		if !s.Accessible() {
			continue
		}
		switch s.Verification {
		case device.Experimental:
			worst = device.Experimental
		case device.VerifiedAutomated, device.NotVerified, "":
			if worst == device.VerifiedOnHardware {
				worst = device.VerifiedAutomated
			}
		}
	}
	switch worst {
	case device.VerifiedOnHardware:
		return "Verified on hardware"
	case device.Experimental:
		return "Automated tests; parts experimental; not verified on hardware"
	}
	return "Automated tests only; not verified on hardware"
}

// typical is the runtime a model has when its kernel driver works as
// documented (used only to describe capabilities, never to claim them live).
func typical(s gamepad.ModelSpec) device.Descriptor {
	d := evdev.InputDevice{}
	rt := gamepad.Runtime{Rumble: s.Hardware.Rumble, RumbleFF: s.Hardware.Rumble, Battery: s.Hardware.Battery, Readable: true, Conn: device.ConnUSBWired}
	m := gamepad.Mapping{Sticks: [4]gamepad.Axis{{Present: true}, {Present: true}, {Present: true}, {Present: true}}}
	if len(s.Hardware.RearInputs) > 0 {
		rt.RearMapped = true
	}
	return s.Describe(m, d, rt)
}

func detection(s gamepad.ModelSpec) string {
	var parts []string
	for _, id := range s.IDs {
		parts = append(parts, fmt.Sprintf("%04x:%04x %s", id.Vendor, id.Product, id.Transport))
	}
	for _, all := range s.Strings {
		kind := "name match"
		if s.Probable {
			kind = "name match, probable (user can confirm)"
		}
		parts = append(parts, kind+": "+strings.Join(all, " + "))
	}
	return strings.Join(parts, "; ")
}

func row(d device.Descriptor, det, conns, backend, drivers string) Row {
	caps := d.Capabilities
	input := "Implemented (read/test)"
	if !caps.Supports(device.CapDiagnostics) {
		input = "Not implemented"
	}
	firmware := "—"
	switch {
	case caps.Supports(device.CapFirmwareCheck):
		firmware = "Read-only update check"
	case caps.Spec(device.CapFirmwareUpdate).Presence == device.Present:
		firmware = "Official updater instructions"
	}
	return Row{
		Model: d.DisplayName, ModelID: d.ModelID, Visual: d.VisualID, Detection: det, Platforms: "Linux",
		Connections: conns, Backend: backend, Drivers: drivers, Input: input,
		Battery: cell(caps, device.CapBattery), Vibration: cell(caps, device.CapHaptics), Lighting: cell(caps, device.CapLighting),
		Triggers: cell(caps, device.CapAdaptiveTriggers), Rear: cell(caps, device.CapRearControls), Remapping: cell(caps, device.CapRemapping),
		Profiles: cell(caps, device.CapProfiles), Firmware: firmware, Verification: verification(caps),
	}
}

// Matrix lists the ten target controllers in gallery order.
func Matrix() []Row {
	byVisual := map[string]Row{}
	byVisual["dualsense"] = row(dualsense.Descriptor(), "054c:0ce6 usb/bluetooth (hidraw)", "USB, Bluetooth", "dualsense-hidraw", "hid-playstation + hidraw")
	byVisual["dualsense_edge"] = row(dualsense.EdgeDescriptor(), "054c:0df2 usb/bluetooth (hidraw)", "USB, Bluetooth", "dualsense-hidraw", "hid-playstation + hidraw")
	specs := append([]gamepad.ModelSpec{msigc20.Spec}, padmodels.Specs()...)
	for _, s := range specs {
		det := detection(s)
		if s.ID == msigc20.ModelID {
			det = "USB strings / MSI VID 0db0; 045e:028e v0210 probable"
		}
		conns := "USB"
		if s.Hardware.Battery {
			conns = "USB, wireless (per driver)"
		}
		r := row(typical(s), det, conns, "linux-evdev (+ ff, power_supply)", strings.Join(s.KernelDrivers, ", "))
		for _, kd := range s.KernelDrivers {
			if kd == "xpad" { // speaks the XInput protocol
				r.Platforms = "Linux; Windows: generic XInput (input, rumble, battery; model not identified; experimental)"
			}
		}
		byVisual[s.VisualID] = r
	}
	var out []Row
	for _, spec := range visual.Gallery() {
		if r, ok := byVisual[spec.ID]; ok {
			out = append(out, r)
		}
	}
	return out
}

// Markdown renders the matrix as the document checked into docs/.
func Markdown(rows []Row) string {
	var b strings.Builder
	b.WriteString("# Controller support matrix\n\n")
	b.WriteString("Generated from the driver descriptors by `tuisense debug support-matrix`; do not edit by hand.\n")
	b.WriteString("\"—\" means the controller does not have the hardware. \"Depends on OS driver\" means the hardware exists but Linux must expose it. ")
	b.WriteString("\"Not implemented\" means the hardware exists and TuiSense does not drive it yet. Vibration and battery are shown for a working kernel driver; ")
	b.WriteString("at runtime TuiSense re-checks them for the actual connection. Hardware verification is not claimed for any model.\n\n")
	header := []string{"Model", "Detection", "Platforms", "Connections", "Linux drivers", "Input", "Battery", "Vibration", "Lighting", "Adaptive triggers", "Rear controls", "Remapping", "Profiles", "Firmware", "Verification"}
	b.WriteString("| " + strings.Join(header, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" --- |", len(header)) + "\n")
	for _, r := range rows {
		cols := []string{r.Model, r.Detection, r.Platforms, r.Connections, r.Drivers, r.Input, r.Battery, r.Vibration, r.Lighting, r.Triggers, r.Rear, r.Remapping, r.Profiles, r.Firmware, r.Verification}
		b.WriteString("| " + strings.Join(cols, " | ") + " |\n")
	}
	b.WriteString("\nWindows: controllers in XInput mode are read through Microsoft's XInput API as a generic Xbox-layout controller (input, rumble, battery); XInput cannot identify the model, so model-specific features are Linux-only. This backend is unit-tested but has not been run on Windows hardware. DualSense, DualShock 4 and Switch Pro have no Windows backend yet.\n")
	b.WriteString("macOS: TuiSense builds and runs (gallery, profiles, reports), but no controller backend is implemented yet.\n")
	return b.String()
}

// Sorted returns model IDs (for tests).
func Sorted(rows []Row) []string {
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ModelID)
	}
	sort.Strings(ids)
	return ids
}

var (
	cacheOnce sync.Once
	cache     map[string]Row
)

// ForVisual returns the matrix row of the controller rendered with a visual.
func ForVisual(visualID string) (Row, bool) {
	cacheOnce.Do(func() {
		cache = map[string]Row{}
		for _, r := range Matrix() {
			cache[r.Visual] = r
		}
	})
	r, ok := cache[visualID]
	return r, ok
}

// Implemented lists the features a row marks as implemented, in display order.
func (r Row) Implemented() []string {
	var out []string
	for _, f := range []struct{ name, cell string }{
		{"Input", r.Input}, {"Vibration", r.Vibration}, {"Battery", r.Battery}, {"Lighting", r.Lighting},
		{"Adaptive triggers", r.Triggers}, {"Rear controls", r.Rear},
	} {
		if strings.HasPrefix(f.cell, "Implemented") {
			out = append(out, f.name)
		}
	}
	return out
}
