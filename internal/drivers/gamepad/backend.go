package gamepad

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

const (
	maxEventsPerPoll = 2048
	maxSamples       = 512
	maxPulse         = 1500 * time.Millisecond
)

// Backend implements device.Device for one evdev gamepad node.
type Backend struct {
	mu     sync.RWMutex
	desc   device.Descriptor
	info   device.Info
	cfg    device.Config
	tel    device.Telemetry
	m      Mapping
	dev    *evdev.Device
	logger *logging.Logger

	axes     map[uint16]int32
	keys     map[uint16]bool
	syncing  bool // between SYN_DROPPED and the next SYN_REPORT
	syns     uint64
	dropped  uint64
	readErrs uint64

	rumble   bool
	effectID int16
	stopAt   time.Time
}

// OpenBackend opens a discovered gamepad and reads its axis ranges.
func OpenBackend(f Discovered, logger *logging.Logger) (*Backend, error) {
	b := &Backend{logger: logger, axes: map[uint16]int32{}, keys: map[uint16]bool{}, effectID: -1, cfg: device.DefaultConfig()}
	d := f.Dev
	dev, err := evdev.Open(d.EventPath)
	abs := map[uint16]evdev.AbsInfo{}
	if err != nil {
		if !os.IsPermission(err) {
			return nil, fmt.Errorf("open %s: %w", d.EventPath, err)
		}
		b.info.PermissionDenied, b.info.ReadOnly = true, true
		b.info.PermissionHint = fmt.Sprintf("No read access to %s. Log in on the local seat (udev grants joystick access) or add your user to the 'input' group.", d.EventPath)
	} else {
		b.dev = dev
		for _, code := range d.Caps.Abs.Set() {
			if ai, err := dev.AbsInfo(code); err == nil {
				abs[code] = ai
				b.axes[code] = ai.Value
			}
		}
		if ks, err := dev.KeyState(); err == nil {
			for _, code := range d.Caps.Key.Set() {
				b.keys[code] = ks.Has(code)
			}
		}
		b.info.ReadOnly = !dev.Writable
	}
	b.m = InferMapping(d, abs)
	b.rumble = b.dev != nil && b.dev.Writable && d.HasFF(evdev.FFRumble)

	res := f.Resolution
	if res.Model != nil {
		b.desc = res.Model.Describe(b.m, d, b.rumble)
	} else {
		b.desc = GenericDescriptor(b.m, d, b.rumble)
	}
	b.fillInfo(f, abs)
	b.tel = b.snapshot(time.Now())
	b.tel.Samples = []device.RawSample{}
	if logger != nil {
		logger.Info("GAMEPAD", "Opened %s as %s (%s, layout %s, rumble=%v)", d.EventPath, b.desc.DisplayName, d.HardwareKey(), b.m.Layout, b.rumble)
	}
	return b, nil
}

func (b *Backend) fillInfo(f Discovered, abs map[uint16]evdev.AbsInfo) {
	d, res := f.Dev, f.Resolution
	conn := device.ConnUSBWired
	if d.Bus == evdev.BusBluetooth {
		conn = device.ConnBluetooth
	}
	b.info.ID = d.EventName
	b.info.Name = d.Name
	b.info.Model = b.desc.DisplayName
	b.info.VendorID, b.info.ProductID = d.Vendor, d.Product
	b.info.InputDevicePath = d.EventPath
	b.info.SysfsPath = d.SysfsPath
	b.info.Connection = conn
	b.info.Connected = true
	b.info.LastSeen = time.Now()
	b.info.KernelDriver = d.KernelDriver
	b.info.EventDriven = true
	b.info.IdentityKey = d.HardwareKey()
	b.info.Identification = res.Reason
	switch res.Confidence {
	case Identified:
		b.info.IdentityState = "identified"
	case Confirmed:
		b.info.IdentityState = "confirmed"
	case Probable:
		b.info.IdentityState = "probable"
	default:
		b.info.IdentityState = "unidentified"
		b.info.Identification = "Not identified as a known model"
	}
	if res.Model != nil && res.Confidence == Probable {
		b.info.IdentityCandidateID, b.info.IdentityCandidateName = res.Model.ID, res.Model.Static.DisplayName
	} else if res.Candidate != nil {
		b.info.IdentityCandidateID, b.info.IdentityCandidateName = res.Candidate.ID, res.Candidate.Static.DisplayName
	}

	b.info.InputMode = LayoutLabel(b.m.Layout)
	if res.Model != nil && res.Model.ModeHint != nil {
		if h := res.Model.ModeHint(b.m, d); h != "" {
			b.info.InputMode = h
		}
	}
	b.info.OutputMode = "None"
	if b.rumble {
		b.info.OutputMode = "Linux force feedback (FF_RUMBLE)"
	}

	add := func(label, value string) {
		if value != "" {
			b.info.Details = append(b.info.Details, device.InfoField{Label: label, Value: value})
		}
	}
	add("Kernel driver", d.KernelDriver)
	add("Input device ID", fmt.Sprintf("%04x:%04x (version %04x)", d.Vendor, d.Product, d.Version))
	if u := d.USB; u != nil {
		add("USB ID", fmt.Sprintf("%04x:%04x, bcdDevice %x.%02x", u.VendorID, u.ProductID, u.BCDDevice>>8, u.BCDDevice&0xff))
		add("USB strings", usbStrings(u))
		if u.SpeedMbps != "" {
			add("USB speed", u.SpeedMbps+" Mbit/s (USB"+u.USBVersion+")")
		}
		add("USB bus/device", fmt.Sprintf("%03d/%03d", u.BusNum, u.DevNum))
	}
	add("Event node", d.EventPath)
	add("Input layout", LayoutLabel(b.m.Layout))
	trig := "none"
	switch {
	case b.m.HasTriggers && b.m.TriggerKind == device.TriggerAnalog:
		trig = fmt.Sprintf("analog (%s/%s, %d…%d)", evdev.AbsName(b.m.Triggers[0].Code), evdev.AbsName(b.m.Triggers[1].Code), b.m.Triggers[0].Info.Minimum, b.m.Triggers[0].Info.Maximum)
	case b.m.TriggerKind == device.TriggerSharedAxis:
		trig = "shared axis " + evdev.AbsName(b.m.Triggers[0].Code)
	case b.m.TriggerKind == device.TriggerDigital:
		trig = "digital buttons (BTN_TL2/BTN_TR2)"
	}
	add("Triggers", trig)
	if b.m.HasSticks() {
		a := b.m.Sticks[0].Info
		add("Stick range", fmt.Sprintf("%d…%d (kernel fuzz %d, flat %d)", a.Minimum, a.Maximum, a.Fuzz, a.Flat))
	}
	ff := "none"
	if d.HasFF(evdev.FFRumble) {
		ff = "FF_RUMBLE"
		if b.dev != nil {
			if n, err := b.dev.EffectSlots(); err == nil {
				ff += fmt.Sprintf(" (%d effect slots)", n)
			}
		}
		if !b.rumble {
			ff += " — no write access"
		}
	}
	add("Force feedback", ff)
	for _, n := range b.m.Notes {
		add("Mapping note", n)
	}
}

func (b *Backend) Descriptor() device.Descriptor { return b.desc }

func (b *Backend) Info() device.Info {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.info
}

func (b *Backend) Capabilities() device.CapabilitySet { return b.desc.Capabilities }

func (b *Backend) Telemetry() device.Telemetry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.tel
}

func (b *Backend) CurrentConfig() device.Config {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cfg
}

// Mapping returns the inferred input mapping.
func (b *Backend) Mapping() Mapping { return b.m }

// buttons returns the semantic button state.
func (b *Backend) buttons() device.ButtonSet {
	var set device.ButtonSet
	for _, bm := range b.m.Buttons {
		if bm.Code != 0 && b.keys[bm.Code] {
			set = set.With(bm.ID)
		}
	}
	if b.m.Hat {
		x, y := b.axes[evdev.AbsHat0X], b.axes[evdev.AbsHat0Y]
		switch {
		case x < 0:
			set = set.With(device.ButtonDpadLeft)
		case x > 0:
			set = set.With(device.ButtonDpadRight)
		}
		switch {
		case y < 0:
			set = set.With(device.ButtonDpadUp)
		case y > 0:
			set = set.With(device.ButtonDpadDown)
		}
	}
	return set
}

func (b *Backend) rawSample(at time.Time, held bool) device.RawSample {
	s := device.RawSample{At: at, Buttons: b.buttons(), Held: held}
	for i, a := range b.m.Sticks {
		if a.Present {
			s.Sticks[i] = b.axes[a.Code]
		}
	}
	for i, a := range b.m.Triggers {
		if a.Present {
			s.Triggers[i] = b.axes[a.Code]
		}
	}
	return s
}

// snapshot converts the current state to telemetry (no deadzone, no curves).
func (b *Backend) snapshot(now time.Time) device.Telemetry {
	l := b.desc.Inputs
	s := b.rawSample(now, false)
	tel := device.Telemetry{
		Buttons:   s.Buttons,
		Raw:       device.RawInput{Valid: true, Sticks: s.Sticks, Triggers: s.Triggers},
		UpdatedAt: b.tel.UpdatedAt,
		PacketsRx: b.syns,
		PacketsTx: b.tel.PacketsTx,
	}
	if l.HasSticks() {
		n := func(i int) float64 { return l.StickAxis(i).NormalizeBipolar(float64(s.Sticks[i])) }
		tel.LeftStickX, tel.LeftStickY, tel.RightStickX, tel.RightStickY = n(0), n(1), n(2), n(3)
	}
	switch l.TriggerKind {
	case device.TriggerDigital:
		if s.Buttons.Has(device.ButtonL2Digital) {
			tel.L2Pressure = 1
		}
		if s.Buttons.Has(device.ButtonR2Digital) {
			tel.R2Pressure = 1
		}
	case device.TriggerSharedAxis:
		v := l.TriggerAxis(0).NormalizeBipolar(float64(s.Triggers[0]))
		tel.L2Pressure, tel.R2Pressure = max(0, v), max(0, -v)
	default:
		if l.HasTriggers() {
			tel.L2Pressure = l.TriggerAxis(0).NormalizeUnipolar(float64(s.Triggers[0]))
			tel.R2Pressure = l.TriggerAxis(1).NormalizeUnipolar(float64(s.Triggers[1]))
		}
	}
	return tel
}

func (b *Backend) resync() {
	for code := range b.axes {
		if ai, err := b.dev.AbsInfo(code); err == nil {
			b.axes[code] = ai.Value
		}
	}
	if ks, err := b.dev.KeyState(); err == nil {
		for code := range b.keys {
			b.keys[code] = ks.Has(code)
		}
	}
}

// Poll drains queued events. Every SYN_REPORT becomes one raw sample; when
// nothing arrived, one held sample keeps time-based measurements going.
func (b *Backend) Poll(_ context.Context) (device.Telemetry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if b.dev == nil {
		return b.tel, nil
	}
	b.expireRumble(now)
	events, err := b.dev.ReadEvents(maxEventsPerPoll)
	var samples []device.RawSample
	var latched device.ButtonSet
	for _, ev := range events {
		switch ev.Type {
		case evdev.EvAbs:
			b.axes[ev.Code] = ev.Value
		case evdev.EvKey:
			b.keys[ev.Code] = ev.Value != 0
		case evdev.EvSyn:
			switch ev.Code {
			case evdev.SynDropped:
				b.syncing = true
				b.dropped++
			case evdev.SynReport:
				if b.syncing {
					b.resync()
					b.syncing = false
				}
				b.syns++
				s := b.rawSample(ev.Time, false)
				latched |= s.Buttons
				if len(samples) < maxSamples {
					samples = append(samples, s)
				}
				b.tel.UpdatedAt = ev.Time
				b.info.LastSeen = now
			}
		}
	}
	if err != nil {
		b.readErrs++
		if errors.Is(err, syscall.ENODEV) || errors.Is(err, os.ErrClosed) {
			b.info.Connected = false
			return b.tel, fmt.Errorf("%s disconnected: %w", b.info.InputDevicePath, err)
		}
	}
	tel := b.snapshot(now)
	if len(samples) == 0 {
		samples = []device.RawSample{b.rawSample(now, true)}
	}
	tel.Samples = samples
	tel.ButtonsLatched = latched | tel.Buttons
	tel.ReportsDropped = b.dropped
	tel.ReadErrors = b.readErrs
	b.tel = tel
	return tel, nil
}

func (b *Backend) ApplyConfig(_ context.Context, cfg device.Config) error {
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
	return nil
}

// ExecuteCommand only acts on rumble pulses; every other setting is stored
// but has no hardware effect on this device (and is never claimed to).
func (b *Backend) ExecuteCommand(_ context.Context, cmd device.Command) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch cmd.Type {
	case device.CmdPulseHaptics:
		return b.pulseLocked(cmd)
	case device.CmdApplyFullConfig:
		b.cfg = cmd.Config
	}
	return nil
}

func (b *Backend) pulseLocked(cmd device.Command) error {
	if !b.rumble {
		return fmt.Errorf("rumble is not available on %s", b.info.InputDevicePath)
	}
	dur := cmd.PulseDur
	if dur <= 0 {
		dur = 280 * time.Millisecond
	}
	if dur > maxPulse {
		dur = maxPulse
	}
	gain := 1.0
	if !cmd.PulseAbsolute {
		gain = float64(b.cfg.Haptics.MasterIntensity) / 100
	}
	strong := uint16(min(65535, float64(cmd.PulseLeft)*257*gain))
	weak := uint16(min(65535, float64(cmd.PulseRight)*257*gain))
	id, err := b.dev.UploadRumble(b.effectID, strong, weak, dur)
	if err != nil {
		return fmt.Errorf("upload rumble effect: %w", err)
	}
	b.effectID = id
	if err := b.dev.PlayEffect(id, true); err != nil {
		return fmt.Errorf("play rumble effect: %w", err)
	}
	b.tel.PacketsTx++
	b.stopAt = time.Now().Add(dur)
	return nil
}

// expireRumble explicitly stops a finished pulse (the kernel also stops it
// after the effect's replay length).
func (b *Backend) expireRumble(now time.Time) {
	if b.effectID >= 0 && !b.stopAt.IsZero() && !now.Before(b.stopAt) {
		_ = b.dev.PlayEffect(b.effectID, false)
		b.stopAt = time.Time{}
	}
}

func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dev == nil {
		return nil
	}
	if b.effectID >= 0 {
		_ = b.dev.PlayEffect(b.effectID, false)
		_ = b.dev.EraseEffect(b.effectID)
		b.effectID = -1
	}
	err := b.dev.Close()
	b.dev = nil
	return err
}

var _ device.Device = (*Backend)(nil)
