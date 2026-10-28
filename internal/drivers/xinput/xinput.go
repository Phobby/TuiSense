// Package xinput reads XInput controllers on Windows through Microsoft's
// documented XInput API (xinput1_4.dll): buttons, sticks and triggers, the
// two rumble motors, and the battery level of wireless pads. It needs no
// third-party driver.
//
// XInput does not expose vendor/product IDs, motion sensors, touchpads, rear
// paddles, the Guide button or any vendor feature, so every XInput device is
// described as a generic Xbox-layout controller, never as a specific model.
// The backend is compiled and unit-tested with a fake API on every platform;
// it has not been run on Windows hardware yet (Verification: experimental).
package xinput

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
)

// DriverID is the registry ID.
const DriverID = "xinput"

// ModelID is the descriptor of every XInput device.
const ModelID = "windows.xinput_controller"

// Gamepad mirrors XINPUT_GAMEPAD.
type Gamepad struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

// State mirrors XINPUT_STATE.
type State struct {
	Packet  uint32
	Gamepad Gamepad
}

// Battery types and levels (XINPUT_BATTERY_INFORMATION).
const (
	BatteryDisconnected uint8 = 0x00
	BatteryWired        uint8 = 0x01
	BatteryAlkaline     uint8 = 0x02
	BatteryNiMH         uint8 = 0x03
	BatteryUnknown      uint8 = 0xff
)

// ErrNotConnected is returned for an empty user slot.
var ErrNotConnected = errors.New("xinput: controller not connected")

// API is the XInput surface used (implemented with syscalls on Windows).
type API interface {
	GetState(slot uint32) (State, error)
	SetState(slot uint32, left, right uint16) error
	Battery(slot uint32) (kind, level uint8, err error)
}

// Button bits of XINPUT_GAMEPAD.wButtons.
var buttonBits = []struct {
	bit   uint16
	id    device.ButtonID
	label string
	group string
}{
	{0x1000, device.ButtonSouth, "A", "Face"}, {0x2000, device.ButtonEast, "B", "Face"},
	{0x4000, device.ButtonWest, "X", "Face"}, {0x8000, device.ButtonNorth, "Y", "Face"},
	{0x0100, device.ButtonL1, "LB", "Shoulder"}, {0x0200, device.ButtonR1, "RB", "Shoulder"},
	{0x0020, device.ButtonView, "Back / View", "System"}, {0x0010, device.ButtonMenu, "Start / Menu", "System"},
	{0x0040, device.ButtonL3, "LS (click)", "Stick"}, {0x0080, device.ButtonR3, "RS (click)", "Stick"},
	{0x0001, device.ButtonDpadUp, "D-pad ↑", "D-pad"}, {0x0002, device.ButtonDpadDown, "D-pad ↓", "D-pad"},
	{0x0004, device.ButtonDpadLeft, "D-pad ←", "D-pad"}, {0x0008, device.ButtonDpadRight, "D-pad →", "D-pad"},
}

// Buttons converts wButtons to TuiSense's semantic set.
func Buttons(b uint16) device.ButtonSet {
	var s device.ButtonSet
	for _, m := range buttonBits {
		if b&m.bit != 0 {
			s = s.With(m.id)
		}
	}
	return s
}

func inputs() device.InputLayout {
	l := device.InputLayout{
		StickLabels:   [2]string{"Left stick", "Right stick"},
		StickRange:    device.AxisRange{Min: -32768, Max: 32767, Center: 0},
		TriggerLabels: [2]string{"LT", "RT"},
		TriggerRange:  device.AxisRange{Min: 0, Max: 255},
		AxisNames:     [6]string{"sThumbLX", "sThumbLY", "sThumbRX", "sThumbRY", "bLeftTrigger", "bRightTrigger"},
		Drift: device.DriftHeuristics{CenterTolerance: 983, NoiseTolerance: 328,
			Rationale: "Generic heuristics (3% center, 1% noise of the XInput range); not a manufacturer specification."},
	}
	// XInput reports Y up as positive; TuiSense uses negative Y = up.
	l.StickAxes = [4]device.AxisRange{l.StickRange, {Min: -32768, Max: 32767, Invert: true}, l.StickRange, {Min: -32768, Max: 32767, Invert: true}}
	for _, b := range buttonBits {
		l.Buttons = append(l.Buttons, device.ButtonDef{ID: b.id, Label: b.label, Group: b.group})
	}
	return l
}

func capabilities(battery bool) device.CapabilitySet {
	exp := device.Experimental
	specs := []device.CapabilitySpec{
		{ID: device.CapConnection, Support: device.Full, Access: device.AccessRead, Backend: "windows-xinput", Verification: exp},
		{ID: device.CapDiagnostics, Support: device.Full, Access: device.AccessRead | device.AccessTest, Backend: "windows-xinput", Verification: exp},
		{ID: device.CapSticks, Support: device.Limited, Access: device.AccessRead | device.AccessTest, Backend: "windows-xinput", Verification: exp,
			Note: "Live view and diagnostics only. TuiSense has no input-processing backend, so it cannot change what games receive."},
		{ID: device.CapHaptics, Support: device.Limited, Access: device.AccessTest, Backend: "windows-xinput", Verification: exp,
			Features: []device.Feature{device.FeatDualMotors, device.FeatTestPulse},
			Note:     "Rumble through XInputSetState (left: low-frequency motor, right: high-frequency motor)."},
	}
	if battery {
		specs = append(specs, device.CapabilitySpec{ID: device.CapBattery, Support: device.Limited, Access: device.AccessRead,
			Backend: "windows-xinput", Verification: exp, Note: "XInput reports the battery in four steps (empty, low, medium, full)."})
	}
	return device.NewCapabilitySet(specs...)
}

// Descriptor describes a generic XInput controller.
func Descriptor(battery bool) device.Descriptor {
	return device.Descriptor{
		ModelID: ModelID, Family: "xinput", Brand: "XInput", DisplayName: "XInput controller", ShortName: "XInput",
		VisualID: "generic", Accent: device.RGB{R: 140, G: 150, B: 175},
		Regions: []device.Region{
			{ID: device.RegionLeftTrigger, Label: "LT Trigger", Badge: "LT", Capability: device.CapDiagnostics, Description: "Open the trigger diagnostics."},
			{ID: device.RegionRightTrigger, Label: "RT Trigger", Badge: "RT", Capability: device.CapDiagnostics, Description: "Open the trigger diagnostics."},
			{ID: device.RegionLeftStick, Label: "Left Stick", Badge: "L-Stick", Capability: device.CapDiagnostics, Description: "Open the stick and deadzone diagnostics."},
			{ID: device.RegionRightStick, Label: "Right Stick", Badge: "R-Stick", Capability: device.CapDiagnostics, Description: "Open the stick and deadzone diagnostics."},
			{ID: device.RegionHaptics, Label: "Vibration Motors", Badge: "Rumble", Capability: device.CapDiagnostics, Description: "Open the vibration test."},
		},
		Inputs:       inputs(),
		Capabilities: capabilities(battery),
		Actions: []device.Action{{ID: "pulse_test", Label: "Test Rumble", Capability: device.CapHaptics,
			Command: device.Command{Type: device.CmdPulseHaptics, PulseLeft: 150, PulseRight: 150, PulseDur: 300 * time.Millisecond, PulseAbsolute: true}}},
	}
}

// Driver enumerates the four XInput user slots.
type Driver struct{ api API }

// NewDriver returns the driver; on systems without XInput it finds nothing.
func NewDriver() *Driver { return &Driver{api: newAPI()} }

// NewDriverWith uses an explicit API (tests).
func NewDriverWith(api API) *Driver { return &Driver{api: api} }

func (*Driver) ID() string                  { return DriverID }
func (*Driver) Models() []device.Descriptor { return []device.Descriptor{Descriptor(false)} }

func (d *Driver) Discover() ([]device.Candidate, error) {
	if d.api == nil {
		return nil, nil
	}
	var out []device.Candidate
	for slot := uint32(0); slot < 4; slot++ {
		if _, err := d.api.GetState(slot); err == nil {
			out = append(out, device.Candidate{Key: fmt.Sprintf("xinput:%d", slot), Label: fmt.Sprintf("XInput controller (slot %d)", slot+1), Payload: slot})
		}
	}
	return out, nil
}

func (d *Driver) Open(c device.Candidate, logger *logging.Logger) (device.Device, error) {
	slot, ok := c.Payload.(uint32)
	if !ok || d.api == nil {
		return nil, fmt.Errorf("xinput: bad candidate %v", c.Key)
	}
	b := &Backend{api: d.api, slot: slot, logger: logger, cfg: device.DefaultConfig()}
	kind, level, err := d.api.Battery(slot)
	b.battery = err == nil && (kind == BatteryAlkaline || kind == BatteryNiMH)
	b.desc = Descriptor(b.battery)
	conn := device.ConnWireless
	if err == nil && kind == BatteryWired {
		conn = device.ConnUSBWired
	}
	b.info = device.Info{ID: c.Key, Name: c.Label, Model: "XInput controller", Connection: conn, Connected: true, LastSeen: time.Now(),
		InputMode: "XInput", OutputMode: "XInputSetState", IdentityState: "unidentified",
		Identification: "XInput does not report vendor or product IDs, so the exact model cannot be identified.",
		Details:        []device.InfoField{{Label: "XInput slot", Value: fmt.Sprint(slot + 1)}}}
	b.tel.BatteryPercent = -1
	if b.battery {
		b.tel.BatteryPercent = BatteryPercent(level)
	}
	return b, nil
}

// BatteryPercent maps XInput's four battery levels to a percentage.
func BatteryPercent(level uint8) int {
	switch level {
	case 0:
		return 5
	case 1:
		return 25
	case 2:
		return 60
	}
	return 100
}

// Backend implements device.Device for one XInput slot.
type Backend struct {
	mu      sync.Mutex
	api     API
	slot    uint32
	logger  *logging.Logger
	desc    device.Descriptor
	info    device.Info
	tel     device.Telemetry
	cfg     device.Config
	battery bool
	batAt   time.Time
	stopAt  time.Time
	packets uint64
}

func (b *Backend) Descriptor() device.Descriptor      { return b.desc }
func (b *Backend) Capabilities() device.CapabilitySet { return b.desc.Capabilities }
func (b *Backend) Info() device.Info {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.info
}
func (b *Backend) Telemetry() device.Telemetry {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tel
}
func (b *Backend) CurrentConfig() device.Config {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cfg
}

func (b *Backend) Poll(context.Context) (device.Telemetry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if !b.stopAt.IsZero() && !now.Before(b.stopAt) {
		_ = b.api.SetState(b.slot, 0, 0)
		b.stopAt = time.Time{}
	}
	st, err := b.api.GetState(b.slot)
	if err != nil {
		b.info.Connected = false
		return b.tel, fmt.Errorf("xinput slot %d: %w", b.slot+1, err)
	}
	if b.battery && now.Sub(b.batAt) > 5*time.Second {
		b.batAt = now
		if _, level, err := b.api.Battery(b.slot); err == nil {
			b.tel.BatteryPercent = BatteryPercent(level)
		}
	}
	g := st.Gamepad
	l := b.desc.Inputs
	raw := device.RawSample{At: now, Sticks: [4]int32{int32(g.ThumbLX), int32(g.ThumbLY), int32(g.ThumbRX), int32(g.ThumbRY)},
		Triggers: [2]int32{int32(g.LeftTrigger), int32(g.RightTrigger)}, Buttons: Buttons(g.Buttons), Held: st.Packet == uint32(b.packets)}
	b.packets = uint64(st.Packet)
	b.tel.Buttons, b.tel.ButtonsLatched = raw.Buttons, raw.Buttons
	b.tel.LeftStickX = l.StickAxis(0).NormalizeBipolar(float64(g.ThumbLX))
	b.tel.LeftStickY = l.StickAxis(1).NormalizeBipolar(float64(g.ThumbLY))
	b.tel.RightStickX = l.StickAxis(2).NormalizeBipolar(float64(g.ThumbRX))
	b.tel.RightStickY = l.StickAxis(3).NormalizeBipolar(float64(g.ThumbRY))
	b.tel.L2Pressure = l.TriggerAxis(0).NormalizeUnipolar(float64(g.LeftTrigger))
	b.tel.R2Pressure = l.TriggerAxis(1).NormalizeUnipolar(float64(g.RightTrigger))
	b.tel.Raw = device.RawInput{Valid: true, Sticks: raw.Sticks, Triggers: raw.Triggers}
	b.tel.Samples = []device.RawSample{raw}
	b.tel.UpdatedAt = now
	b.tel.PacketsRx++
	b.info.LastSeen = now
	return b.tel, nil
}

func (b *Backend) ApplyConfig(_ context.Context, cfg device.Config) error {
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
	return nil
}

// ExecuteCommand plays bounded rumble pulses; stopping (CmdSetHaptics) stops
// the motors. Everything else is refused by the command gate before it gets here.
func (b *Backend) ExecuteCommand(_ context.Context, cmd device.Command) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch cmd.Type {
	case device.CmdPulseHaptics:
		dur := cmd.PulseDur
		if dur <= 0 {
			dur = 280 * time.Millisecond
		}
		if dur > 1500*time.Millisecond {
			dur = 1500 * time.Millisecond
		}
		gain := 1.0
		if !cmd.PulseAbsolute {
			gain = float64(b.cfg.Haptics.MasterIntensity) / 100
		}
		left := uint16(min(65535, float64(cmd.PulseLeft)*257*gain))
		right := uint16(min(65535, float64(cmd.PulseRight)*257*gain))
		if err := b.api.SetState(b.slot, left, right); err != nil {
			return err
		}
		b.stopAt = time.Now().Add(dur)
		b.tel.PacketsTx++
	case device.CmdSetHaptics:
		b.stopAt = time.Time{}
		return b.api.SetState(b.slot, 0, 0)
	case device.CmdApplyFullConfig:
		b.cfg = cmd.Config
	}
	return nil
}

func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.api.SetState(b.slot, 0, 0)
}

var _ device.Device = (*Backend)(nil)
var _ device.Driver = (*Driver)(nil)
