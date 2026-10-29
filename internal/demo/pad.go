package demo

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/simulate"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// layout is the input layout a kernel driver reports for a model: the axis
// ranges and key codes TuiSense's evdev mapping is inferred from.
type layout struct {
	driver   string
	stick    [2]int32 // min, max
	trigger  [2]int32 // min, max; zero = digital triggers (buttons only)
	hat      bool     // d-pad as ABS_HAT0X/Y instead of BTN_DPAD_*
	triggers bool     // BTN_TL2/BTN_TR2 are reported
}

var (
	// Linux xpad, Xbox One protocol: 16-bit sticks, 10-bit triggers.
	xpadOne = layout{driver: "xpad", stick: [2]int32{-32768, 32767}, trigger: [2]int32{0, 1023}, hat: true}
	// Linux xpad, Xbox 360 protocol: 8-bit triggers.
	xpad360 = layout{driver: "xpad", stick: [2]int32{-32768, 32767}, trigger: [2]int32{0, 255}, hat: true}
	// Linux hid-playstation: 8-bit sticks and triggers plus digital L2/R2 bits.
	playstation = layout{driver: "playstation", stick: [2]int32{0, 255}, trigger: [2]int32{0, 255}, hat: true, triggers: true}
	// Linux hid-nintendo: ZL/ZR are digital buttons, the d-pad is buttons.
	nintendo = layout{driver: "nintendo", stick: [2]int32{-32767, 32767}, triggers: true}
)

func connFor(transport string) device.ConnectionType {
	if transport == "bluetooth" {
		return device.ConnBluetooth
	}
	return device.ConnUSBWired
}

// syntheticDevice is the input node a kernel driver would expose for p.
// It exists only in memory; no path in it names a real node.
func syntheticDevice(p pad, conn device.ConnectionType) (evdev.InputDevice, map[uint16]evdev.AbsInfo) {
	l := p.layout
	key, abs, ev, ff := make(evdev.Bits, 12), make(evdev.Bits, 1), make(evdev.Bits, 1), make(evdev.Bits, 2)
	set := func(b evdev.Bits, c uint16) { b[c/64] |= 1 << (c % 64) }
	set(ev, evdev.EvKey)
	set(ev, evdev.EvAbs)
	if p.spec.Hardware.Rumble {
		set(ev, evdev.EvFF)
		set(ff, evdev.FFRumble)
	}
	infos := map[uint16]evdev.AbsInfo{}
	axis := func(code uint16, min, max int32) {
		set(abs, code)
		infos[code] = evdev.AbsInfo{Minimum: min, Maximum: max, Value: (min + max) / 2}
	}
	for _, c := range []uint16{evdev.AbsX, evdev.AbsY, evdev.AbsRX, evdev.AbsRY} {
		axis(c, l.stick[0], l.stick[1])
	}
	if l.trigger[1] > l.trigger[0] {
		axis(evdev.AbsZ, l.trigger[0], l.trigger[1])
		axis(evdev.AbsRZ, l.trigger[0], l.trigger[1])
	}
	if l.hat {
		axis(evdev.AbsHat0X, -1, 1)
		axis(evdev.AbsHat0Y, -1, 1)
	} else {
		for _, c := range []uint16{evdev.BtnDpadUp, evdev.BtnDpadDown, evdev.BtnDpadLeft, evdev.BtnDpadRight} {
			set(key, c)
		}
	}
	for _, c := range []uint16{evdev.BtnSouth, evdev.BtnEast, evdev.BtnNorth, evdev.BtnWest, evdev.BtnTL, evdev.BtnTR,
		evdev.BtnSelect, evdev.BtnStart, evdev.BtnMode, evdev.BtnThumbL, evdev.BtnThumbR} {
		set(key, c)
	}
	if l.triggers {
		set(key, evdev.BtnTL2)
		set(key, evdev.BtnTR2)
	}
	for _, b := range p.spec.Hardware.RearInputs {
		set(key, b.Code)
	}
	d := evdev.InputDevice{
		EventName:    "web-demo",
		Name:         p.spec.DisplayName + " (simulated)",
		Bus:          evdev.BusUSB,
		KernelDriver: l.driver,
		Caps:         evdev.Capabilities{EV: ev, Key: key, Abs: abs, FF: ff},
	}
	if conn == device.ConnBluetooth {
		d.Bus = evdev.BusBluetooth
	}
	if len(p.spec.IDs) > 0 {
		d.Vendor, d.Product = p.spec.IDs[0].Vendor, p.spec.IDs[0].Product
	}
	return d, infos
}

// describe builds the model's descriptor exactly as the evdev driver does
// for a connected controller whose kernel driver exposes rumble and battery
// whenever the model has them.
func describe(p pad, conn device.ConnectionType) (device.Descriptor, gamepad.Mapping) {
	d, infos := syntheticDevice(p, conn)
	m := gamepad.InferMapping(d, infos)
	rt := gamepad.Runtime{Rumble: p.spec.Hardware.Rumble, RumbleFF: p.spec.Hardware.Rumble,
		Battery: p.spec.Hardware.Battery, Readable: true, Conn: conn}
	desc := p.spec.Describe(m, d, rt)
	// Accessible capabilities are served by the simulation, not by Linux.
	for id, cs := range desc.Capabilities {
		if !cs.Accessible() {
			continue
		}
		cs.Backend = "simulated"
		if id == device.CapBattery {
			cs.Note = "Simulated battery level (web demo). No battery health is measured."
		}
		desc.Capabilities[id] = cs
	}
	return desc, m
}

// Pad is a simulated controller of a model served by a Linux kernel driver.
type Pad struct {
	p      pad
	gen    *simulate.Generator
	logger *logging.Logger

	mu         sync.RWMutex
	desc       device.Descriptor
	info       device.Info
	cfg        device.Config
	tel        device.Telemetry
	scenario   device.InputScenario
	scenarioAt time.Time
	startedAt  time.Time
	rumbleTill time.Time
	batStart   int // simulated level at start (-1 = the model has no battery)
}

var (
	_ device.Device                 = (*Pad)(nil)
	_ device.Simulator              = (*Pad)(nil)
	_ device.ScenarioSimulator      = (*Pad)(nil)
	_ device.BatteryDetailsProvider = (*Pad)(nil)
)

func newPad(p pad, key string, seed int64, logger *logging.Logger) *Pad {
	now := time.Now()
	conn := connFor(p.conn)
	desc, _ := describe(p, conn)
	sr := desc.Inputs.StickRange
	// Each model rests slightly differently; the right stick of every other
	// model drifts visibly so the drift test has something to report.
	drift := 0.0
	if seed%2 == 0 {
		drift = 0.05
	}
	d := &Pad{
		p: p, logger: logger, desc: desc, cfg: desc.NewDefaultConfig(),
		gen: simulate.New(desc.Inputs, simulate.Profile{
			StickOffset: [4]float64{0.004, -0.006, drift, -drift / 2},
			StickNoise:  0.004 * sr.HalfSpan(),
		}, seed),
		scenario: device.ScenarioDemo, scenarioAt: now, startedAt: now,
		info: device.Info{
			ID:              key,
			Name:            p.spec.DisplayName,
			Model:           p.spec.DisplayName + " · simulated",
			VendorID:        desc.VendorID,
			InputDevicePath: "web demo (simulated, no device node)",
			KernelDriver:    p.layout.driver,
			Connection:      conn,
			InputMode:       "Simulated " + p.layout.driver + " input",
			OutputMode:      "Simulated (nothing is sent)",
			Connected:       true,
			LastSeen:        now,
			Identification:  "Simulated web-demo controller",
		},
	}
	if len(desc.ProductIDs) > 0 {
		d.info.ProductID = desc.ProductIDs[0]
	}
	d.batStart = -1
	if desc.Capabilities.Supports(device.CapBattery) {
		d.batStart = 55 + int(seed%5)*8
	}
	d.tel.BatteryPercent = d.batStart
	d.tel.UpdatedAt = now
	return d
}

func (d *Pad) Descriptor() device.Descriptor {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.desc
}

func (d *Pad) Capabilities() device.CapabilitySet {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.desc.Capabilities
}

func (d *Pad) Info() device.Info {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.info
}

func (d *Pad) Telemetry() device.Telemetry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.tel
}

func (d *Pad) CurrentConfig() device.Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

// Poll produces the next simulated sample, normalized exactly as the evdev
// backend normalizes what the kernel reports.
func (d *Pad) Poll(_ context.Context) (device.Telemetry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.info.Connected {
		return d.tel, nil
	}
	now := time.Now()
	fr := d.gen.Frame(d.scenario, now.Sub(d.scenarioAt).Seconds())
	raw := d.gen.Raw(fr)
	l := d.desc.Inputs
	d.tel.Raw = raw
	d.tel.Buttons, d.tel.ButtonsLatched = fr.Buttons, fr.Buttons
	if l.HasSticks() {
		n := func(i int) float64 { return l.StickAxis(i).NormalizeBipolar(float64(raw.Sticks[i])) }
		d.tel.LeftStickX, d.tel.LeftStickY, d.tel.RightStickX, d.tel.RightStickY = n(0), n(1), n(2), n(3)
	}
	switch l.TriggerKind {
	case device.TriggerDigital:
		d.tel.L2Pressure, d.tel.R2Pressure = 0, 0
		if fr.Buttons.Has(device.ButtonL2Digital) {
			d.tel.L2Pressure = 1
		}
		if fr.Buttons.Has(device.ButtonR2Digital) {
			d.tel.R2Pressure = 1
		}
	default:
		if l.HasTriggers() {
			d.tel.L2Pressure = l.TriggerAxis(0).NormalizeUnipolar(float64(raw.Triggers[0]))
			d.tel.R2Pressure = l.TriggerAxis(1).NormalizeUnipolar(float64(raw.Triggers[1]))
		}
	}
	if d.batStart >= 0 {
		// Simulated drain: 1% per minute, never below 8%.
		d.tel.BatteryPercent = max(8, d.batStart-int(now.Sub(d.startedAt).Minutes()))
	}
	if !d.rumbleTill.IsZero() && now.After(d.rumbleTill) {
		d.rumbleTill = time.Time{}
		d.cfg.Haptics.LeftMotor, d.cfg.Haptics.RightMotor = 0, 0
	}
	d.tel.Simulated = true
	d.tel.PacketsRx++
	d.tel.UpdatedAt = now
	d.tel.LastInputHex = fmt.Sprintf("simulated %s report: %d %d %d %d · %d %d",
		d.p.layout.driver, raw.Sticks[0], raw.Sticks[1], raw.Sticks[2], raw.Sticks[3], raw.Triggers[0], raw.Triggers[1])
	d.info.LastSeen = now
	return d.tel, nil
}

// ApplyConfig keeps the configuration in memory only.
func (d *Pad) ApplyConfig(_ context.Context, cfg device.Config) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.info.Connected {
		return fmt.Errorf("controller is disconnected")
	}
	d.cfg = cfg
	d.tel.PacketsTx++
	return nil
}

// ExecuteCommand records authorised commands in memory; nothing is sent.
func (d *Pad) ExecuteCommand(_ context.Context, cmd device.Command) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.info.Connected {
		return fmt.Errorf("cannot execute %s: controller disconnected", cmd.Type)
	}
	switch cmd.Type {
	case device.CmdApplyFullConfig:
		d.cfg = cmd.Config
	case device.CmdSetHaptics:
		if cmd.Haptics != nil {
			d.cfg.Haptics = *cmd.Haptics
		} else {
			d.cfg.Haptics = cmd.Config.Haptics
		}
	case device.CmdPulseHaptics:
		dur := cmd.PulseDur
		if dur <= 0 {
			dur = 300 * time.Millisecond
		}
		d.cfg.Haptics.LeftMotor, d.cfg.Haptics.RightMotor = cmd.PulseLeft, cmd.PulseRight
		d.rumbleTill = time.Now().Add(dur)
	default:
		return fmt.Errorf("%s is not available on %s", cmd.Type, d.desc.ShortName)
	}
	d.tel.PacketsTx++
	d.tel.LastOutputHex = fmt.Sprintf("simulated FF_RUMBLE strong=%d weak=%d (no vibration: web demo)", d.cfg.Haptics.LeftMotor, d.cfg.Haptics.RightMotor)
	return nil
}

func (d *Pad) Close() error { return nil }

// SetInputScenario switches the simulated input behavior.
func (d *Pad) SetInputScenario(scn device.InputScenario) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.scenario != scn {
		d.scenario, d.scenarioAt = scn, time.Now()
	}
}

// BatteryDetails reports the simulated level only; no health, capacity or
// cycle data is invented.
func (d *Pad) BatteryDetails() []device.BatteryAttribute {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tel.BatteryPercent < 0 {
		return nil
	}
	return []device.BatteryAttribute{{Name: "capacity", Value: fmt.Sprintf("%d%%", d.tel.BatteryPercent), Source: "simulated"}}
}

// ToggleConnection simulates unplugging and reconnecting the controller.
func (d *Pad) ToggleConnection() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.info.Connected = !d.info.Connected
	return d.info.Connected
}

// CycleTransport switches between USB and Bluetooth for wireless models;
// wired-only models stay on USB. The capabilities follow the transport.
func (d *Pad) CycleTransport() device.ConnectionType {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.p.spec.Hardware.Battery {
		return d.info.Connection // wired-only controller
	}
	conn := device.ConnBluetooth
	if d.info.Connection == device.ConnBluetooth {
		conn = device.ConnUSBWired
	}
	d.desc, _ = describe(d.p, conn)
	d.info.Connection = conn
	return conn
}

// TogglePermissionError is a no-op: a simulated device has no device node.
func (d *Pad) TogglePermissionError() bool { return false }
