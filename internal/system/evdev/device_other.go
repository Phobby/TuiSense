//go:build !linux

package evdev

import (
	"errors"
	"time"
)

// Event is one decoded input event.
type Event struct {
	Time  time.Time
	Type  uint16
	Code  uint16
	Value int32
}

// Device is unavailable outside Linux.
type Device struct{ Writable bool }

// ErrUnsupported is returned on platforms without evdev.
var ErrUnsupported = errors.New("evdev is only available on Linux")

func Open(string) (*Device, error)              { return nil, ErrUnsupported }
func (*Device) AbsInfo(uint16) (AbsInfo, error) { return AbsInfo{}, ErrUnsupported }
func (*Device) KeyState() (Bits, error)         { return nil, ErrUnsupported }
func (*Device) EffectSlots() (int, error)       { return 0, ErrUnsupported }
func (*Device) ReadEvents(int) ([]Event, error) { return nil, ErrUnsupported }
func (*Device) PlayEffect(int16, bool) error    { return ErrUnsupported }
func (*Device) EraseEffect(int16) error         { return ErrUnsupported }
func (*Device) Close() error                    { return nil }
func (*Device) UploadRumble(int16, uint16, uint16, time.Duration) (int16, error) {
	return -1, ErrUnsupported
}
