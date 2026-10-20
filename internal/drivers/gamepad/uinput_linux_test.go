//go:build linux

package gamepad

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// This integration test creates a real kernel input device through
// /dev/uinput that presents the identity documented for the MSI Force GC20 V2
// default mode (045e:028e, version 0210) with an xpad-like layout and
// FF_RUMBLE, then drives TuiSense's evdev driver against it: sysfs discovery,
// EVIOCGABS ranges, event reads, and force-feedback upload/play/erase.
// It is NOT a test of the physical controller. Run with TUISENSE_UINPUT=1.

func uioc(dir, nr, size uintptr) uintptr { return dir<<30 | size<<16 | 'U'<<8 | nr }

type uinputSetup struct {
	Bus, Vendor, Product, Version uint16
	Name                          [80]byte
	FFEffectsMax                  uint32
}

type uinputAbsSetup struct {
	Code uint16
	_    uint16
	Info evdev.AbsInfo
}

type uinputFFUpload struct {
	RequestID uint32
	Retval    int32
	Effect    ffEffectMirror
	Old       ffEffectMirror
}

type ffEffectMirror struct {
	Type, ID, Direction, TriggerButton, TriggerInterval, ReplayLength, ReplayDelay uint16
	U                                                                              [(24 + unsafe.Sizeof(uintptr(0))) / unsafe.Sizeof(uintptr(0))]uintptr
}

type uinputFFErase struct {
	RequestID uint32
	Retval    int32
	EffectID  uint32
}

type rawEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

func ioctlPtr(t *testing.T, fd uintptr, req uintptr, arg unsafe.Pointer) {
	t.Helper()
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); e != 0 {
		t.Fatalf("ioctl %#x: %v", req, e)
	}
}

func ioctlVal(t *testing.T, fd uintptr, req uintptr, v uintptr) {
	t.Helper()
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, v); e != 0 {
		t.Fatalf("ioctl %#x(%d): %v", req, v, e)
	}
}

type virtualPad struct {
	fd       int
	mu       sync.Mutex
	uploads  []ffEffectMirror
	plays    []int32
	erased   int
	stop     chan struct{}
	finished chan struct{}
}

func (v *virtualPad) emit(t *testing.T, typ, code uint16, val int32) {
	t.Helper()
	ev := rawEvent{Type: typ, Code: code, Value: val}
	b := (*[unsafe.Sizeof(rawEvent{})]byte)(unsafe.Pointer(&ev))[:]
	if _, err := syscall.Write(v.fd, b); err != nil {
		t.Fatalf("uinput write: %v", err)
	}
}

// serveFF answers the kernel's force-feedback upload/erase requests and
// records play events, as a real driver would.
func (v *virtualPad) serveFF() {
	defer close(v.finished)
	buf := make([]byte, unsafe.Sizeof(rawEvent{})*16)
	for {
		select {
		case <-v.stop:
			return
		default:
		}
		n, err := syscall.Read(v.fd, buf)
		if err != nil {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		for off := 0; off+int(unsafe.Sizeof(rawEvent{})) <= n; off += int(unsafe.Sizeof(rawEvent{})) {
			ev := (*rawEvent)(unsafe.Pointer(&buf[off]))
			switch {
			case ev.Type == 0x0101 && ev.Code == 1: // EV_UINPUT / UI_FF_UPLOAD
				up := uinputFFUpload{RequestID: uint32(ev.Value)}
				_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(v.fd), uioc(3, 200, unsafe.Sizeof(up)), uintptr(unsafe.Pointer(&up)))
				v.mu.Lock()
				v.uploads = append(v.uploads, up.Effect)
				v.mu.Unlock()
				up.Retval = 0
				_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(v.fd), uioc(1, 201, unsafe.Sizeof(up)), uintptr(unsafe.Pointer(&up)))
			case ev.Type == 0x0101 && ev.Code == 2: // UI_FF_ERASE
				er := uinputFFErase{RequestID: uint32(ev.Value)}
				_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(v.fd), uioc(3, 202, unsafe.Sizeof(er)), uintptr(unsafe.Pointer(&er)))
				er.Retval = 0
				_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(v.fd), uioc(1, 203, unsafe.Sizeof(er)), uintptr(unsafe.Pointer(&er)))
				v.mu.Lock()
				v.erased++
				v.mu.Unlock()
			case ev.Type == evdev.EvFF:
				v.mu.Lock()
				v.plays = append(v.plays, ev.Value)
				v.mu.Unlock()
			}
		}
	}
}

func createVirtualPad(t *testing.T, name string) *virtualPad {
	t.Helper()
	fd, err := syscall.Open("/dev/uinput", syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Skipf("no /dev/uinput access: %v", err)
	}
	f := uintptr(fd)
	for _, ev := range []uint16{evdev.EvSyn, evdev.EvKey, evdev.EvAbs, evdev.EvFF} {
		ioctlVal(t, f, uioc(1, 100, 4), uintptr(ev))
	}
	for _, k := range []uint16{evdev.BtnSouth, evdev.BtnEast, evdev.BtnNorth, evdev.BtnWest, evdev.BtnTL, evdev.BtnTR,
		evdev.BtnSelect, evdev.BtnStart, evdev.BtnMode, evdev.BtnThumbL, evdev.BtnThumbR} {
		ioctlVal(t, f, uioc(1, 101, 4), uintptr(k))
	}
	ioctlVal(t, f, uioc(1, 107, 4), uintptr(evdev.FFRumble))
	abs := map[uint16]evdev.AbsInfo{
		evdev.AbsX: {Minimum: -32768, Maximum: 32767, Fuzz: 16, Flat: 128}, evdev.AbsY: {Minimum: -32768, Maximum: 32767, Fuzz: 16, Flat: 128},
		evdev.AbsRX: {Minimum: -32768, Maximum: 32767, Fuzz: 16, Flat: 128}, evdev.AbsRY: {Minimum: -32768, Maximum: 32767, Fuzz: 16, Flat: 128},
		evdev.AbsZ: {Minimum: 0, Maximum: 255}, evdev.AbsRZ: {Minimum: 0, Maximum: 255},
		evdev.AbsHat0X: {Minimum: -1, Maximum: 1}, evdev.AbsHat0Y: {Minimum: -1, Maximum: 1},
	}
	for code, info := range abs {
		ioctlVal(t, f, uioc(1, 103, 4), uintptr(code))
		as := uinputAbsSetup{Code: code, Info: info}
		ioctlPtr(t, f, uioc(1, 4, unsafe.Sizeof(as)), unsafe.Pointer(&as))
	}
	setup := uinputSetup{Bus: evdev.BusUSB, Vendor: 0x045e, Product: 0x028e, Version: 0x0210, FFEffectsMax: 16}
	copy(setup.Name[:], name)
	ioctlPtr(t, f, uioc(1, 3, unsafe.Sizeof(setup)), unsafe.Pointer(&setup))
	ioctlVal(t, f, uioc(0, 1, 0), 0)
	v := &virtualPad{fd: fd, stop: make(chan struct{}), finished: make(chan struct{})}
	go v.serveFF()
	t.Cleanup(func() {
		close(v.stop)
		<-v.finished
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, f, uioc(0, 2, 0), 0)
		_ = syscall.Close(fd)
	})
	return v
}

func TestUinputVirtualPadEndToEnd(t *testing.T) {
	if os.Getenv("TUISENSE_UINPUT") != "1" {
		t.Skip("set TUISENSE_UINPUT=1 to run the uinput integration test")
	}
	const name = "TuiSense uinput test pad"
	pad := createVirtualPad(t, name)

	drv := NewDriver(Model{
		ID:     "test.msi_like",
		Static: device.Descriptor{ModelID: "test.msi_like", DisplayName: "MSI-like test model"},
		Identify: func(d evdev.InputDevice) (Confidence, string) {
			if d.Vendor == 0x045e && d.Product == 0x028e && d.Version == 0x0210 {
				return Probable, "documented default-mode ID"
			}
			return NoMatch, ""
		},
		Describe: func(m Mapping, d evdev.InputDevice, rumble bool) device.Descriptor {
			desc := GenericDescriptor(m, d, rumble)
			desc.ModelID = "test.msi_like"
			return desc
		},
	})
	if len(probe(drv, name)) > 0 {
		t.Fatal("virtual (uinput) pads must be ignored by default")
	}
	drv.IncludeVirtual = true
	var cand *device.Candidate
	deadline := time.Now().Add(3 * time.Second)
	for cand == nil && time.Now().Before(deadline) {
		cands, err := drv.Discover()
		if err != nil {
			t.Fatal(err)
		}
		for i, c := range cands {
			if f := c.Payload.(Discovered); f.Dev.Name == name {
				cand = &cands[i]
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if cand == nil {
		t.Fatal("virtual pad was not discovered through sysfs")
	}
	if !strings.HasSuffix(cand.Key, "#test.msi_like") {
		t.Fatalf("expected a probable model match, key %s", cand.Key)
	}
	var dev device.Device
	var err error
	for time.Now().Before(deadline.Add(2 * time.Second)) {
		if dev, err = drv.Open(*cand, nil); err == nil && !dev.Info().PermissionDenied {
			break
		}
		time.Sleep(100 * time.Millisecond) // udev may still be applying the seat ACL
	}
	if err != nil {
		t.Fatal(err)
	}
	if dev.Info().PermissionDenied {
		t.Skipf("no access to the event node: %s", dev.Info().PermissionHint)
	}
	defer dev.Close()
	b := dev.(*Backend)
	if b.Mapping().Layout != LayoutXInput || b.Mapping().TriggerKind != device.TriggerAnalog {
		t.Fatalf("unexpected mapping %+v", b.Mapping())
	}
	in := dev.Descriptor().Inputs
	if in.StickAxis(0).Min != -32768 || in.TriggerAxis(0).Max != 255 {
		t.Fatalf("ranges must come from EVIOCGABS: %+v", in.StickAxes)
	}

	pad.emit(t, evdev.EvAbs, evdev.AbsX, 16384)
	pad.emit(t, evdev.EvAbs, evdev.AbsRZ, 255)
	pad.emit(t, evdev.EvAbs, evdev.AbsHat0Y, -1)
	pad.emit(t, evdev.EvKey, evdev.BtnSouth, 1)
	pad.emit(t, evdev.EvSyn, evdev.SynReport, 0)
	pad.emit(t, evdev.EvKey, evdev.BtnSouth, 0)
	pad.emit(t, evdev.EvSyn, evdev.SynReport, 0)
	time.Sleep(30 * time.Millisecond)
	tel, err := dev.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tel.Samples) != 2 || tel.Samples[0].Held {
		t.Fatalf("each SYN_REPORT must become one sample, got %d", len(tel.Samples))
	}
	if tel.LeftStickX < 0.49 || tel.LeftStickX > 0.51 || tel.R2Pressure != 1 {
		t.Fatalf("normalized values wrong: lx=%.3f r2=%.2f", tel.LeftStickX, tel.R2Pressure)
	}
	if !tel.ButtonsLatched.Has(device.ButtonSouth) || tel.Buttons.Has(device.ButtonSouth) || !tel.Buttons.Has(device.ButtonDpadUp) {
		t.Fatalf("buttons: latched=%b now=%b", tel.ButtonsLatched, tel.Buttons)
	}
	tel, _ = dev.Poll(context.Background())
	if len(tel.Samples) != 1 || !tel.Samples[0].Held {
		t.Fatalf("a quiet poll yields one held sample, got %+v", tel.Samples)
	}

	if !dev.Capabilities().Supports(device.CapHaptics) {
		t.Fatal("FF_RUMBLE with write access should enable vibration")
	}
	if err := dev.ExecuteCommand(context.Background(), device.Command{Type: device.CmdPulseHaptics, PulseLeft: 200, PulseRight: 50, PulseDur: 200 * time.Millisecond, PulseAbsolute: true}); err != nil {
		t.Fatalf("rumble pulse: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	pad.mu.Lock()
	if len(pad.uploads) != 1 || len(pad.plays) == 0 || pad.plays[0] != 1 {
		pad.mu.Unlock()
		t.Fatalf("expected one upload and a play event, got %d uploads, plays %v", len(pad.uploads), pad.plays)
	}
	e := pad.uploads[0]
	u := (*[4]byte)(unsafe.Pointer(&e.U[0]))
	strong, weak := binary.NativeEndian.Uint16(u[0:2]), binary.NativeEndian.Uint16(u[2:4])
	pad.mu.Unlock()
	if e.Type != evdev.FFRumble || strong != 200*257 || weak != 50*257 || e.ReplayLength != 200 {
		t.Fatalf("rumble effect wrong: type %#x strong %d weak %d length %d", e.Type, strong, weak, e.ReplayLength)
	}
	time.Sleep(250 * time.Millisecond)
	_, _ = dev.Poll(context.Background()) // expires the pulse
	if err := dev.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	pad.mu.Lock()
	defer pad.mu.Unlock()
	if pad.erased != 1 || pad.plays[len(pad.plays)-1] != 0 {
		t.Fatalf("closing must stop and erase the effect: erased=%d plays=%v", pad.erased, pad.plays)
	}
}

func probe(drv *Driver, name string) []device.Candidate {
	time.Sleep(200 * time.Millisecond)
	cands, _ := drv.Discover()
	var out []device.Candidate
	for _, c := range cands {
		if c.Payload.(Discovered).Dev.Name == name {
			out = append(out, c)
		}
	}
	return out
}
