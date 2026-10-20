//go:build linux

package evdev

import (
	"encoding/binary"
	"errors"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// Event is one decoded input event.
type Event struct {
	Time  time.Time
	Type  uint16
	Code  uint16
	Value int32
}

// inputEvent mirrors struct input_event for the running architecture.
type inputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

const eventSize = int(unsafe.Sizeof(inputEvent{}))

// ffEffect mirrors struct ff_effect. The union is pointer-aligned and holds
// up to ff_periodic_effect (24 bytes of fields + one data pointer).
type ffEffect struct {
	Type            uint16
	ID              int16
	Direction       uint16
	TriggerButton   uint16
	TriggerInterval uint16
	ReplayLength    uint16
	ReplayDelay     uint16
	U               [(24 + unsafe.Sizeof(uintptr(0))) / unsafe.Sizeof(uintptr(0))]uintptr
}

// ioctl request encoding (asm-generic).
func ioc(dir, typ, nr, size uintptr) uintptr { return dir<<30 | size<<16 | typ<<8 | nr }

const (
	iocWrite = 1
	iocRead  = 2
)

// Device is an open event node.
type Device struct {
	f        *os.File
	Writable bool
}

// ErrUnsupported is returned on platforms without evdev.
var ErrUnsupported = errors.New("evdev is only available on Linux")

// Open opens path non-blocking, read-write when permitted (needed for
// rumble), falling back to read-only.
func Open(path string) (*Device, error) {
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err == nil {
		return &Device{f: f, Writable: true}, nil
	}
	f, err2 := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err2 != nil {
		return nil, err
	}
	return &Device{f: f}, nil
}

func (d *Device) ioctl(req uintptr, arg uintptr) error {
	rc, err := d.f.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	if cerr := rc.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg)
	}); cerr != nil {
		return cerr
	}
	if errno != 0 {
		return errno
	}
	return nil
}

// AbsInfo reads the range of an absolute axis (EVIOCGABS).
func (d *Device) AbsInfo(code uint16) (AbsInfo, error) {
	var ai AbsInfo
	err := d.ioctl(ioc(iocRead, 'E', 0x40+uintptr(code), unsafe.Sizeof(ai)), uintptr(unsafe.Pointer(&ai)))
	return ai, err
}

// KeyState returns the currently pressed keys (EVIOCGKEY).
func (d *Device) KeyState() (Bits, error) {
	buf := make([]byte, (int(KeyMax)+8)/8)
	if err := d.ioctl(ioc(iocRead, 'E', 0x18, uintptr(len(buf))), uintptr(unsafe.Pointer(&buf[0]))); err != nil {
		return nil, err
	}
	out := make(Bits, (len(buf)+7)/8)
	for i, b := range buf {
		out[i/8] |= uint64(b) << (8 * uint(i%8))
	}
	return out, nil
}

// EffectSlots returns how many force-feedback effects can be uploaded at once.
func (d *Device) EffectSlots() (int, error) {
	var n int32
	err := d.ioctl(ioc(iocRead, 'E', 0x84, unsafe.Sizeof(n)), uintptr(unsafe.Pointer(&n)))
	return int(n), err
}

// ReadEvents reads every queued event without blocking. It returns (nil, nil)
// when the queue is empty and the read error otherwise (ENODEV = unplugged).
func (d *Device) ReadEvents(max int) ([]Event, error) {
	rc, err := d.f.SyscallConn()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, eventSize*64)
	var out []Event
	for len(out) < max {
		var n int
		var rerr error
		if err := rc.Read(func(fd uintptr) bool {
			n, rerr = syscall.Read(int(fd), buf)
			return true // never wait for readiness
		}); err != nil {
			return out, err
		}
		if rerr != nil {
			if errors.Is(rerr, syscall.EAGAIN) {
				return out, nil
			}
			return out, rerr
		}
		if n <= 0 {
			return out, nil
		}
		for off := 0; off+eventSize <= n; off += eventSize {
			ev := (*inputEvent)(unsafe.Pointer(&buf[off]))
			out = append(out, Event{
				Time:  time.Unix(int64(ev.Time.Sec), int64(ev.Time.Usec)*1000), //nolint:unconvert // field widths differ per arch
				Type:  ev.Type,
				Code:  ev.Code,
				Value: ev.Value,
			})
		}
	}
	return out, nil
}

// UploadRumble uploads (id = -1) or updates an FF_RUMBLE effect and returns
// its id. The kernel stops the effect by itself after length, so a crash can
// never leave the motors running.
func (d *Device) UploadRumble(id int16, strong, weak uint16, length time.Duration) (int16, error) {
	e := ffEffect{Type: FFRumble, ID: id, ReplayLength: uint16(min(length.Milliseconds(), 0xffff))}
	u := (*[4]byte)(unsafe.Pointer(&e.U[0]))
	binary.NativeEndian.PutUint16(u[0:2], strong)
	binary.NativeEndian.PutUint16(u[2:4], weak)
	if err := d.ioctl(ioc(iocWrite, 'E', 0x80, unsafe.Sizeof(e)), uintptr(unsafe.Pointer(&e))); err != nil {
		return -1, err
	}
	return e.ID, nil
}

// PlayEffect starts (play=true) or stops an uploaded effect.
func (d *Device) PlayEffect(id int16, play bool) error {
	ev := inputEvent{Type: EvFF, Code: uint16(id)}
	if play {
		ev.Value = 1
	}
	b := (*[unsafe.Sizeof(inputEvent{})]byte)(unsafe.Pointer(&ev))[:]
	_, err := d.f.Write(b)
	return err
}

// EraseEffect removes an uploaded effect (EVIOCRMFF).
func (d *Device) EraseEffect(id int16) error {
	return d.ioctl(ioc(iocWrite, 'E', 0x81, unsafe.Sizeof(int32(0))), uintptr(id))
}

// Close closes the node.
func (d *Device) Close() error { return d.f.Close() }
