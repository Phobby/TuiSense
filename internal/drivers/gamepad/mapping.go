// Package gamepad is TuiSense's generic Linux evdev gamepad driver. It maps
// whatever input layout the kernel exposes (XInput-style via xpad, HID
// gamepads, joystick-style DirectInput devices) onto TuiSense's semantic
// inputs, and identifies specific controller models through pluggable Model
// matchers (e.g. the MSI Force GC20 V2). Unidentified gamepads are still
// usable for diagnostics with a generic descriptor.
package gamepad

import (
	"fmt"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// Layout classifies the observed input layout.
type Layout string

const (
	LayoutXInput   Layout = "xinput"   // xpad-style: X/Y + RX/RY sticks, Z/RZ analog triggers, hat d-pad
	LayoutGamepad  Layout = "gamepad"  // HID gamepad with BTN_A… semantic buttons
	LayoutJoystick Layout = "joystick" // generic joystick-style buttons (BTN_TRIGGER…), typical of DirectInput modes
)

// Axis binds a semantic input to an absolute axis.
type Axis struct {
	Code    uint16
	Present bool
	Info    evdev.AbsInfo
}

// Range converts the kernel's absinfo into a device.AxisRange.
func (a Axis) Range() device.AxisRange {
	return device.AxisRange{
		Min:    float64(a.Info.Minimum),
		Max:    float64(a.Info.Maximum),
		Center: (float64(a.Info.Minimum) + float64(a.Info.Maximum)) / 2,
	}
}

// ButtonMap binds a kernel key code to a semantic button.
type ButtonMap struct {
	Code  uint16
	ID    device.ButtonID
	Label string
	Group string
}

// Mapping is the inferred relation between kernel events and TuiSense inputs.
type Mapping struct {
	Layout      Layout
	Sticks      [4]Axis // LX, LY, RX, RY
	Triggers    [2]Axis // analog triggers, or Triggers[0] = shared axis
	TriggerKind device.TriggerKind
	HasTriggers bool
	Hat         bool // d-pad reported as ABS_HAT0X/Y
	Buttons     []ButtonMap
	// Notes explain inferences the user may need to know about.
	Notes []string
}

// HasSticks reports whether two full analog sticks were found.
func (m Mapping) HasSticks() bool {
	for _, a := range m.Sticks {
		if !a.Present {
			return false
		}
	}
	return true
}

// ButtonFor returns the mapping of a kernel key code.
func (m Mapping) ButtonFor(code uint16) (ButtonMap, bool) {
	for _, b := range m.Buttons {
		if b.Code == code {
			return b, true
		}
	}
	return ButtonMap{}, false
}

// semantic maps standard gamepad key codes. Labels are Xbox-style; models
// may relabel them. Linux's xpad reports the X button as BTN_X (0x133) and Y
// as BTN_Y (0x134), so 0x133 is the west face button here.
var semantic = []ButtonMap{
	{evdev.BtnSouth, device.ButtonSouth, "A", "Face"},
	{evdev.BtnEast, device.ButtonEast, "B", "Face"},
	{evdev.BtnNorth, device.ButtonWest, "X", "Face"},
	{evdev.BtnWest, device.ButtonNorth, "Y", "Face"},
	{evdev.BtnTL, device.ButtonL1, "LB", "Shoulder"},
	{evdev.BtnTR, device.ButtonR1, "RB", "Shoulder"},
	{evdev.BtnTL2, device.ButtonL2Digital, "LT", "Shoulder"},
	{evdev.BtnTR2, device.ButtonR2Digital, "RT", "Shoulder"},
	{evdev.BtnSelect, device.ButtonView, "Back", "System"},
	{evdev.BtnStart, device.ButtonMenu, "Start", "System"},
	{evdev.BtnMode, device.ButtonHome, "Guide", "System"},
	{evdev.BtnThumbL, device.ButtonL3, "LS (click)", "Stick"},
	{evdev.BtnThumbR, device.ButtonR3, "RS (click)", "Stick"},
	{evdev.BtnDpadUp, device.ButtonDpadUp, "D-pad ↑", "D-pad"},
	{evdev.BtnDpadDown, device.ButtonDpadDown, "D-pad ↓", "D-pad"},
	{evdev.BtnDpadLeft, device.ButtonDpadLeft, "D-pad ←", "D-pad"},
	{evdev.BtnDpadRight, device.ButtonDpadRight, "D-pad →", "D-pad"},
}

// joystickSlots are the semantic IDs given, in order, to generic joystick
// buttons whose physical meaning is unknown. Labels say "Button N" so the UI
// never pretends to know which physical button it is.
var joystickSlots = []device.ButtonID{
	device.ButtonSouth, device.ButtonEast, device.ButtonWest, device.ButtonNorth,
	device.ButtonL1, device.ButtonR1, device.ButtonL2Digital, device.ButtonR2Digital,
	device.ButtonView, device.ButtonMenu, device.ButtonL3, device.ButtonR3,
	device.ButtonHome, device.ButtonShare, device.ButtonLeftFn, device.ButtonRightFn,
}

// InferMapping derives the input mapping from the capabilities and axis
// ranges the kernel reports. It never assumes a fixed per-model layout, so
// the same controller in a different input mode is mapped by what it
// actually exposes.
func InferMapping(dev evdev.InputDevice, abs map[uint16]evdev.AbsInfo) Mapping {
	var m Mapping
	axis := func(code uint16) Axis {
		info, ok := abs[code]
		ok = ok && dev.Caps.Abs.Has(code) && info.Maximum > info.Minimum
		return Axis{Code: code, Present: ok, Info: info}
	}
	has := func(code uint16) bool { return axis(code).Present }

	m.Sticks[0], m.Sticks[1] = axis(evdev.AbsX), axis(evdev.AbsY)
	switch {
	case has(evdev.AbsRX) && has(evdev.AbsRY):
		m.Sticks[2], m.Sticks[3] = axis(evdev.AbsRX), axis(evdev.AbsRY)
		switch {
		case has(evdev.AbsZ) && has(evdev.AbsRZ):
			m.Triggers = [2]Axis{axis(evdev.AbsZ), axis(evdev.AbsRZ)}
		case has(evdev.AbsBrake) && has(evdev.AbsGas):
			m.Triggers = [2]Axis{axis(evdev.AbsBrake), axis(evdev.AbsGas)}
		case has(evdev.AbsZ):
			m.Triggers = [2]Axis{axis(evdev.AbsZ), axis(evdev.AbsZ)}
			m.TriggerKind = device.TriggerSharedAxis
			m.Notes = append(m.Notes, "Both triggers share ABS_Z in this input mode.")
		}
	case has(evdev.AbsZ) && has(evdev.AbsRZ):
		// DirectInput convention: Z/RZ carry the right stick.
		m.Sticks[2], m.Sticks[3] = axis(evdev.AbsZ), axis(evdev.AbsRZ)
		m.Notes = append(m.Notes, "Right stick inferred on ABS_Z/ABS_RZ (DirectInput-style layout).")
		if has(evdev.AbsBrake) && has(evdev.AbsGas) {
			m.Triggers = [2]Axis{axis(evdev.AbsBrake), axis(evdev.AbsGas)}
		}
	}
	m.Hat = has(evdev.AbsHat0X) && has(evdev.AbsHat0Y)

	joystickStyle := false
	for c := evdev.BtnJoystickFirst; c <= evdev.BtnJoystickLast; c++ {
		if dev.Caps.Key.Has(c) {
			joystickStyle = true
			break
		}
	}
	for _, b := range semantic {
		if dev.Caps.Key.Has(b.Code) {
			m.Buttons = append(m.Buttons, b)
		}
	}
	if joystickStyle && len(m.Buttons) == 0 {
		n := 0
		for c := evdev.BtnJoystickFirst; c <= evdev.BtnJoystickLast && n < len(joystickSlots); c++ {
			if dev.Caps.Key.Has(c) {
				m.Buttons = append(m.Buttons, ButtonMap{Code: c, ID: joystickSlots[n], Label: fmt.Sprintf("Button %d", n+1), Group: "Unmapped"})
				n++
			}
		}
		m.Notes = append(m.Notes, "Buttons are reported as generic joystick buttons; their physical positions are not known, so they are shown as Button 1…N.")
	}
	if m.Hat {
		for _, id := range []device.ButtonID{device.ButtonDpadUp, device.ButtonDpadDown, device.ButtonDpadLeft, device.ButtonDpadRight} {
			if _, dup := m.button(id); !dup {
				m.Buttons = append(m.Buttons, ButtonMap{Code: 0, ID: id, Label: dpadLabel(id), Group: "D-pad"})
			}
		}
	}

	switch {
	case m.Triggers[0].Present && m.TriggerKind != device.TriggerSharedAxis:
		m.TriggerKind, m.HasTriggers = device.TriggerAnalog, true
	case m.TriggerKind == device.TriggerSharedAxis:
		m.HasTriggers = true
	default:
		_, l2 := m.button(device.ButtonL2Digital)
		_, r2 := m.button(device.ButtonR2Digital)
		if l2 && r2 && !joystickStyle {
			m.TriggerKind, m.HasTriggers = device.TriggerDigital, true
		} else if joystickStyle {
			m.Notes = append(m.Notes, "No analog trigger axes are exposed; the triggers are probably among the generic buttons in this input mode.")
		}
	}

	switch {
	case dev.KernelDriver == "xpad" || (m.Sticks[2].Code == evdev.AbsRX && m.TriggerKind == device.TriggerAnalog && m.Triggers[0].Code == evdev.AbsZ):
		m.Layout = LayoutXInput
	case joystickStyle && !dev.Caps.Key.Has(evdev.BtnSouth):
		m.Layout = LayoutJoystick
	default:
		m.Layout = LayoutGamepad
	}
	return m
}

func (m Mapping) button(id device.ButtonID) (ButtonMap, bool) {
	for _, b := range m.Buttons {
		if b.ID == id {
			return b, true
		}
	}
	return ButtonMap{}, false
}

func dpadLabel(id device.ButtonID) string {
	switch id {
	case device.ButtonDpadUp:
		return "D-pad ↑"
	case device.ButtonDpadDown:
		return "D-pad ↓"
	case device.ButtonDpadLeft:
		return "D-pad ←"
	}
	return "D-pad →"
}

// Inputs builds the diagnostics input layout from the mapping. Ranges come
// from the kernel; drift tolerances are generic heuristics (3% / 1% of travel).
func (m Mapping) Inputs(stickLabels, triggerLabels [2]string) device.InputLayout {
	l := device.InputLayout{}
	for _, b := range m.Buttons {
		l.Buttons = append(l.Buttons, device.ButtonDef{ID: b.ID, Label: b.Label, Group: b.Group})
	}
	if m.HasSticks() {
		l.StickLabels = stickLabels
		for i, a := range m.Sticks {
			l.StickAxes[i] = a.Range()
			l.AxisNames[i] = evdev.AbsName(a.Code)
		}
		l.StickRange = l.StickAxes[0]
		hs := l.StickRange.HalfSpan()
		l.Drift = device.DriftHeuristics{
			CenterTolerance: 0.03 * hs,
			NoiseTolerance:  0.01 * hs,
			Rationale:       "Generic heuristics (3% center, 1% noise of the kernel-reported range); not a manufacturer specification for this controller.",
		}
	}
	if m.HasTriggers {
		l.TriggerLabels = triggerLabels
		l.TriggerKind = m.TriggerKind
		switch m.TriggerKind {
		case device.TriggerDigital:
			l.TriggerRange = device.AxisRange{Min: 0, Max: 1}
		default:
			l.TriggerAxes = [2]device.AxisRange{m.Triggers[0].Range(), m.Triggers[1].Range()}
			l.TriggerRange = l.TriggerAxes[0]
			l.AxisNames[4], l.AxisNames[5] = evdev.AbsName(m.Triggers[0].Code), evdev.AbsName(m.Triggers[1].Code)
		}
	}
	return l
}
