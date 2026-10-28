//go:build windows

package xinput

import (
	"syscall"
	"unsafe"
)

// winAPI calls xinput1_4.dll (Windows 8+), falling back to xinput9_1_0.dll,
// which has no battery information.
type winAPI struct {
	getState, setState, battery *syscall.LazyProc
}

const errDeviceNotConnected = 1167

func newAPI() API {
	for _, name := range []string{"xinput1_4.dll", "xinput9_1_0.dll"} {
		dll := syscall.NewLazyDLL(name)
		if dll.Load() != nil {
			continue
		}
		a := &winAPI{getState: dll.NewProc("XInputGetState"), setState: dll.NewProc("XInputSetState")}
		if p := dll.NewProc("XInputGetBatteryInformation"); p.Find() == nil {
			a.battery = p
		}
		if a.getState.Find() == nil && a.setState.Find() == nil {
			return a
		}
	}
	return nil
}

type rawState struct {
	packet  uint32
	buttons uint16
	lt, rt  uint8
	lx, ly  int16
	rx, ry  int16
}

func (a *winAPI) GetState(slot uint32) (State, error) {
	var s rawState
	r, _, _ := a.getState.Call(uintptr(slot), uintptr(unsafe.Pointer(&s)))
	switch r {
	case 0:
		return State{Packet: s.packet, Gamepad: Gamepad{Buttons: s.buttons, LeftTrigger: s.lt, RightTrigger: s.rt,
			ThumbLX: s.lx, ThumbLY: s.ly, ThumbRX: s.rx, ThumbRY: s.ry}}, nil
	case errDeviceNotConnected:
		return State{}, ErrNotConnected
	}
	return State{}, syscall.Errno(r)
}

func (a *winAPI) SetState(slot uint32, left, right uint16) error {
	v := [2]uint16{left, right}
	if r, _, _ := a.setState.Call(uintptr(slot), uintptr(unsafe.Pointer(&v))); r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func (a *winAPI) Battery(slot uint32) (uint8, uint8, error) {
	if a.battery == nil {
		return BatteryUnknown, 0, ErrNotConnected
	}
	var info [2]uint8
	if r, _, _ := a.battery.Call(uintptr(slot), 0, uintptr(unsafe.Pointer(&info))); r != 0 {
		return BatteryUnknown, 0, syscall.Errno(r)
	}
	return info[0], info[1], nil
}
