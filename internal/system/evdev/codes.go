// Package evdev reads Linux input devices (/dev/input/event*): sysfs
// discovery, capability bitmaps, absolute-axis ranges, input events, and
// force-feedback rumble. The sysfs scanner is plain file parsing and works
// on any OS against a directory tree; device I/O is Linux-only.
package evdev

import "fmt"

// Event types.
const (
	EvSyn uint16 = 0x00
	EvKey uint16 = 0x01
	EvAbs uint16 = 0x03
	EvFF  uint16 = 0x15
)

// Synchronization codes.
const (
	SynReport  uint16 = 0
	SynDropped uint16 = 3
)

// Absolute axes.
const (
	AbsX        uint16 = 0x00
	AbsY        uint16 = 0x01
	AbsZ        uint16 = 0x02
	AbsRX       uint16 = 0x03
	AbsRY       uint16 = 0x04
	AbsRZ       uint16 = 0x05
	AbsThrottle uint16 = 0x06
	AbsRudder   uint16 = 0x07
	AbsWheel    uint16 = 0x08
	AbsGas      uint16 = 0x09
	AbsBrake    uint16 = 0x0a
	AbsHat0X    uint16 = 0x10
	AbsHat0Y    uint16 = 0x11
	AbsMax      uint16 = 0x3f
)

// Keys and buttons used by gamepads.
const (
	BtnJoystickFirst uint16 = 0x120 // BTN_TRIGGER: generic joystick buttons 0x120..0x12f
	BtnJoystickLast  uint16 = 0x12f
	BtnSouth         uint16 = 0x130 // BTN_A
	BtnEast          uint16 = 0x131 // BTN_B
	BtnC             uint16 = 0x132
	BtnNorth         uint16 = 0x133 // BTN_X (xpad reports the X button here)
	BtnWest          uint16 = 0x134 // BTN_Y (xpad reports the Y button here)
	BtnZ             uint16 = 0x135
	BtnTL            uint16 = 0x136
	BtnTR            uint16 = 0x137
	BtnTL2           uint16 = 0x138
	BtnTR2           uint16 = 0x139
	BtnSelect        uint16 = 0x13a
	BtnStart         uint16 = 0x13b
	BtnMode          uint16 = 0x13c
	BtnThumbL        uint16 = 0x13d
	BtnThumbR        uint16 = 0x13e
	BtnDpadUp        uint16 = 0x220
	BtnDpadDown      uint16 = 0x221
	BtnDpadLeft      uint16 = 0x222
	BtnDpadRight     uint16 = 0x223
	KeyMax           uint16 = 0x2ff
)

// Force-feedback effect types.
const (
	FFRumble   uint16 = 0x50
	FFPeriodic uint16 = 0x51
	FFConstant uint16 = 0x52
	FFGain     uint16 = 0x60
)

// Bus types (input_id.bustype).
const (
	BusUSB       uint16 = 0x03
	BusBluetooth uint16 = 0x05
)

var absNames = map[uint16]string{
	AbsX: "ABS_X", AbsY: "ABS_Y", AbsZ: "ABS_Z", AbsRX: "ABS_RX", AbsRY: "ABS_RY", AbsRZ: "ABS_RZ",
	AbsThrottle: "ABS_THROTTLE", AbsRudder: "ABS_RUDDER", AbsWheel: "ABS_WHEEL", AbsGas: "ABS_GAS", AbsBrake: "ABS_BRAKE",
	AbsHat0X: "ABS_HAT0X", AbsHat0Y: "ABS_HAT0Y", 0x12: "ABS_HAT1X", 0x13: "ABS_HAT1Y", 0x14: "ABS_HAT2X", 0x15: "ABS_HAT2Y",
	0x16: "ABS_HAT3X", 0x17: "ABS_HAT3Y", 0x18: "ABS_PRESSURE", 0x19: "ABS_DISTANCE", 0x28: "ABS_MISC",
}

var keyNames = map[uint16]string{
	BtnSouth: "BTN_A", BtnEast: "BTN_B", BtnC: "BTN_C", BtnNorth: "BTN_X", BtnWest: "BTN_Y", BtnZ: "BTN_Z",
	BtnTL: "BTN_TL", BtnTR: "BTN_TR", BtnTL2: "BTN_TL2", BtnTR2: "BTN_TR2", BtnSelect: "BTN_SELECT", BtnStart: "BTN_START",
	BtnMode: "BTN_MODE", BtnThumbL: "BTN_THUMBL", BtnThumbR: "BTN_THUMBR",
	BtnDpadUp: "BTN_DPAD_UP", BtnDpadDown: "BTN_DPAD_DOWN", BtnDpadLeft: "BTN_DPAD_LEFT", BtnDpadRight: "BTN_DPAD_RIGHT",
	0x120: "BTN_TRIGGER", 0x121: "BTN_THUMB", 0x122: "BTN_THUMB2", 0x123: "BTN_TOP", 0x124: "BTN_TOP2", 0x125: "BTN_PINKIE",
	0x126: "BTN_BASE", 0x127: "BTN_BASE2", 0x128: "BTN_BASE3", 0x129: "BTN_BASE4", 0x12a: "BTN_BASE5", 0x12b: "BTN_BASE6",
	0x12f: "BTN_DEAD",
}

var ffNames = map[uint16]string{
	FFRumble: "FF_RUMBLE", FFPeriodic: "FF_PERIODIC", FFConstant: "FF_CONSTANT", 0x53: "FF_SPRING", 0x54: "FF_FRICTION",
	0x55: "FF_DAMPER", 0x56: "FF_INERTIA", 0x57: "FF_RAMP", FFGain: "FF_GAIN", 0x61: "FF_AUTOCENTER",
}

// AbsName returns the kernel name of an absolute axis code.
func AbsName(code uint16) string {
	if n, ok := absNames[code]; ok {
		return n
	}
	return fmt.Sprintf("ABS_0x%02x", code)
}

// KeyName returns the kernel name of a key/button code.
func KeyName(code uint16) string {
	if n, ok := keyNames[code]; ok {
		return n
	}
	return fmt.Sprintf("KEY_0x%03x", code)
}

// FFName returns the kernel name of a force-feedback code.
func FFName(code uint16) string {
	if n, ok := ffNames[code]; ok {
		return n
	}
	return fmt.Sprintf("FF_0x%02x", code)
}

// AbsInfo mirrors struct input_absinfo.
type AbsInfo struct {
	Value, Minimum, Maximum, Fuzz, Flat, Resolution int32
}
