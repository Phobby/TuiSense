package dualsense

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/simulate"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// MockBackend provides a high-fidelity simulated Sony DualSense controller
// for development, automated testing, and live demonstrations without physical hardware.
type MockBackend struct {
	mu          sync.RWMutex
	desc        device.Descriptor
	info        device.Info
	cfg         device.Config
	tel         device.Telemetry
	logger      *logging.Logger
	startedAt   time.Time
	btSeq       uint8
	pulseActive bool
	pulseUntil  time.Time

	gen         *simulate.Generator
	scenario    device.InputScenario
	scenarioAt  time.Time
	pressCounts uint64
}

// NewMockBackend constructs a simulated DualSense controller initialized with realistic telemetry.
func NewMockBackend(logger *logging.Logger) *MockBackend {
	now := time.Now()
	cfg := device.DefaultConfig()
	// Give the mock controller an expressive default adaptive trigger setup
	cfg.LeftTrigger = device.BuiltinTriggerPresets[1].Config  // Heavy Bow Draw
	cfg.RightTrigger = device.BuiltinTriggerPresets[2].Config // Crisp Pistol Break

	desc := Descriptor()
	m := &MockBackend{
		desc:       desc,
		startedAt:  now,
		scenario:   device.ScenarioDemo,
		scenarioAt: now,
		gen: simulate.New(desc.Inputs, simulate.Profile{
			StickOffset: [4]float64{0.008, -0.004, 0.012, 0.006}, // tiny, typical resting offsets
			StickNoise:  0.45,
			GyroNoise:   2,
			GyroBias:    [3]float64{3, -2, 1},
			GravityAxis: 1, GravitySign: 1,
		}, 7),
		logger: logger,
		cfg:    cfg,
		info: device.Info{
			ID:               "mock-dualsense-0",
			Name:             "Sony Interactive Entertainment DualSense Wireless Controller",
			Model:            "DualSense Wireless Controller (CFI-ZCT1W)",
			VendorID:         system.SonyVendorID,
			ProductID:        system.DualSenseProductID,
			FirmwareVersion:  "0458",
			HardwareVersion:  "0311",
			MACAddress:       "E8:47:3A:91:C2:0B",
			HIDRawPath:       "/dev/hidraw0 (simulated)",
			InputDevicePath:  "/dev/input/event19 (simulated)",
			SysfsPath:        "/sys/class/hidraw/hidraw0",
			Connection:       device.ConnUSB,
			InputMode:        "HID Report 0x01 (USB 64B · 250Hz)",
			OutputMode:       "HID Report 0x02 (USB 48B)",
			Connected:        true,
			PermissionDenied: false,
			LastSeen:         now,
		},
		tel: device.Telemetry{
			BatteryPercent: 84,
			Charging:       true,
			UpdatedAt:      now,
		},
	}

	pkt := BuildOutputReport(cfg, m.info.Connection, 0)
	m.tel.LastOutputHex = FormatHexPreview(pkt)
	m.tel.LastInputHex = "01 80 80 80 80 00 00 00 00 00 00 00 00 00 00 00"
	return m
}

func (m *MockBackend) Descriptor() device.Descriptor { return m.desc }

func (m *MockBackend) Capabilities() device.CapabilitySet {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return runtimeCapabilities(m.desc.Capabilities, m.info.Connection)
}

func (m *MockBackend) Info() device.Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.info
}

func (m *MockBackend) Telemetry() device.Telemetry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tel
}

func (m *MockBackend) CurrentConfig() device.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// ToggleConnection simulates hot-plugging or unplugging the DualSense controller.
func (m *MockBackend) ToggleConnection() bool {
	m.mu.Lock()
	m.info.Connected = !m.info.Connected
	connected := m.info.Connected
	if connected {
		m.info.LastSeen = time.Now()
	}
	m.mu.Unlock()

	if m.logger != nil {
		if connected {
			m.logger.Info("HOTPLUG", "DualSense controller connected on %s (%s)", m.info.HIDRawPath, m.info.Connection)
		} else {
			m.logger.Warn("HOTPLUG", "DualSense controller disconnected from %s", m.info.HIDRawPath)
		}
	}
	return connected
}

// CycleTransport switches the simulated transport between USB-C Wired and Bluetooth.
func (m *MockBackend) CycleTransport() device.ConnectionType {
	m.mu.Lock()
	if m.info.Connection == device.ConnUSB {
		m.info.Connection = device.ConnBluetooth
		m.info.InputMode = "HID Report 0x31 (BT 78B · CRC32)"
		m.info.OutputMode = "HID Report 0x31 (BT 78B + CRC32)"
		m.tel.Charging = false
	} else {
		m.info.Connection = device.ConnUSB
		m.info.InputMode = "HID Report 0x01 (USB 64B · 250Hz)"
		m.info.OutputMode = "HID Report 0x02 (USB 48B)"
		m.tel.Charging = true
	}
	conn := m.info.Connection
	pkt := BuildOutputReport(m.cfg, conn, m.btSeq)
	m.tel.LastOutputHex = FormatHexPreview(pkt)
	m.mu.Unlock()

	if m.logger != nil {
		m.logger.Info("TRANSPORT", "Switched controller transport to %s", conn)
	}
	return conn
}

// TogglePermissionError simulates a Linux udev permission error for testing recovery UI.
func (m *MockBackend) TogglePermissionError() bool {
	m.mu.Lock()
	m.info.PermissionDenied = !m.info.PermissionDenied
	if m.info.PermissionDenied {
		m.info.PermissionHint = system.PermissionExplanation("/dev/hidraw0")
	} else {
		m.info.PermissionHint = ""
	}
	denied := m.info.PermissionDenied
	m.mu.Unlock()

	if m.logger != nil {
		if denied {
			m.logger.Error("PERMISSION", "%s", system.PermissionExplanation("/dev/hidraw0"))
		} else {
			m.logger.Info("PERMISSION", "udev permissions restored for /dev/hidraw0")
		}
	}
	return denied
}

func (m *MockBackend) Poll(_ context.Context) (device.Telemetry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.info.Connected {
		return m.tel, nil
	}

	now := time.Now()
	t := now.Sub(m.startedAt).Seconds()

	if m.pulseActive && now.After(m.pulseUntil) {
		m.pulseActive = false
		m.cfg.Haptics.LeftMotor = 0
		m.cfg.Haptics.RightMotor = 0
	}

	fr := m.gen.Frame(m.scenario, now.Sub(m.scenarioAt).Seconds())
	raw := m.gen.Raw(fr)
	sr := m.desc.Inputs.StickRange
	m.tel.Raw = raw
	m.tel.LeftStickX = simulate.Normalize(raw.Sticks[0], sr)
	m.tel.LeftStickY = simulate.Normalize(raw.Sticks[1], sr)
	m.tel.RightStickX = simulate.Normalize(raw.Sticks[2], sr)
	m.tel.RightStickY = simulate.Normalize(raw.Sticks[3], sr)
	m.tel.L2Pressure = float64(raw.Triggers[0]) / 255
	m.tel.R2Pressure = float64(raw.Triggers[1]) / 255
	m.tel.Buttons = fr.Buttons
	m.tel.ButtonsLatched = fr.Buttons
	m.tel.MicMutedButton = fr.Buttons.Has(device.ButtonMicMute)
	m.tel.Touch = fr.Touch
	m.tel.TouchActive, m.tel.TouchX, m.tel.TouchY = fr.Touch[0].Active, fr.Touch[0].X, fr.Touch[0].Y
	m.tel.Simulated = true
	if m.cfg.Motion.GyroEnabled {
		sens := float64(m.cfg.Motion.GyroSensitivity) / 100.0
		pitch := fr.Tilt[0] * sens
		if m.cfg.Motion.InvertPitch {
			pitch = -pitch
		}
		m.tel.GyroPitch, m.tel.GyroYaw, m.tel.GyroRoll = pitch, fr.Tilt[1]*sens, fr.Tilt[2]*sens
	} else {
		m.tel.GyroPitch, m.tel.GyroYaw, m.tel.GyroRoll = 0, 0, 0
	}
	m.tel.AccelX = float64(raw.Accel[0]) / 8192
	m.tel.AccelY = float64(raw.Accel[1]) / 8192
	m.tel.AccelZ = float64(raw.Accel[2]) / 8192

	// Dynamic lightbar animation in mock mode
	switch m.cfg.Lighting.Animation {
	case device.AnimRainbow:
		hue := math.Mod(t*45.0, 360.0)
		r, g, b := hsvToRGB(hue, 0.9, 1.0)
		m.cfg.Lighting.Red, m.cfg.Lighting.Green, m.cfg.Lighting.Blue = r, g, b
	case device.AnimBattery:
		ratio := float64(m.tel.BatteryPercent) / 100.0
		m.cfg.Lighting.Red = uint8((1.0 - ratio) * 255)
		m.cfg.Lighting.Green = uint8(ratio * 230)
		m.cfg.Lighting.Blue = 40
	}

	m.tel.PacketsRx++
	m.tel.UpdatedAt = now
	m.info.LastSeen = now

	lx := uint8((m.tel.LeftStickX + 1.0) * 127.5)
	ly := uint8((m.tel.LeftStickY + 1.0) * 127.5)
	rx := uint8((m.tel.RightStickX + 1.0) * 127.5)
	ry := uint8((m.tel.RightStickY + 1.0) * 127.5)
	l2 := uint8(m.tel.L2Pressure * 255)
	r2 := uint8(m.tel.R2Pressure * 255)
	m.tel.LastInputHex = fmt.Sprintf("01 %02X %02X %02X %02X %02X %02X 00 00 00 18", lx, ly, rx, ry, l2, r2)

	return m.tel, nil
}

func (m *MockBackend) ApplyConfig(_ context.Context, cfg device.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.info.Connected {
		return fmt.Errorf("controller is disconnected")
	}
	if m.info.PermissionDenied {
		return fmt.Errorf("%s", m.info.PermissionHint)
	}
	m.cfg = cfg
	pkt := BuildOutputReport(m.cfg, m.info.Connection, m.btSeq)
	m.btSeq = (m.btSeq + 1) & 0x0F
	m.tel.LastOutputHex = FormatHexPreview(pkt)
	m.tel.PacketsTx++
	m.info.LastSeen = time.Now()
	return nil
}

func (m *MockBackend) ExecuteCommand(ctx context.Context, cmd device.Command) error {
	m.mu.Lock()
	if !m.info.Connected {
		m.mu.Unlock()
		return fmt.Errorf("cannot execute %s: controller disconnected", cmd.Type)
	}
	if m.info.PermissionDenied {
		hint := m.info.PermissionHint
		m.mu.Unlock()
		return fmt.Errorf("%s", hint)
	}

	switch cmd.Type {
	case device.CmdApplyFullConfig:
		m.cfg = cmd.Config
	case device.CmdSetLighting:
		if cmd.Lighting != nil {
			m.cfg.Lighting = *cmd.Lighting
		} else {
			m.cfg.Lighting = cmd.Config.Lighting
		}
	case device.CmdSetTrigger:
		if cmd.LeftTrigger != nil {
			m.cfg.LeftTrigger = *cmd.LeftTrigger
		} else {
			m.cfg.LeftTrigger = cmd.Config.LeftTrigger
		}
		if cmd.RightTrigger != nil {
			m.cfg.RightTrigger = *cmd.RightTrigger
		} else {
			m.cfg.RightTrigger = cmd.Config.RightTrigger
		}
	case device.CmdSetHaptics:
		if cmd.Haptics != nil {
			m.cfg.Haptics = *cmd.Haptics
		} else {
			m.cfg.Haptics = cmd.Config.Haptics
		}
	case device.CmdSetMicLED:
		if cmd.MicLED != nil {
			m.cfg.Lighting.MicLED = *cmd.MicLED
		}
	case device.CmdSetSpeakerVol:
		m.cfg.Audio = cmd.Config.Audio
	case device.CmdPulseHaptics:
		m.mu.Unlock()
		return m.TriggerPulse(ctx, cmd.PulseLeft, cmd.PulseRight, cmd.PulseDur)
	}

	pkt := BuildOutputReport(m.cfg, m.info.Connection, m.btSeq)
	m.btSeq = (m.btSeq + 1) & 0x0F
	m.tel.LastOutputHex = FormatHexPreview(pkt)
	m.tel.PacketsTx++
	m.info.LastSeen = time.Now()
	m.mu.Unlock()
	return nil
}

func (m *MockBackend) TriggerPulse(_ context.Context, left, right uint8, duration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.info.Connected {
		return fmt.Errorf("cannot pulse haptics: controller disconnected")
	}
	if m.info.PermissionDenied {
		return fmt.Errorf("%s", m.info.PermissionHint)
	}
	if duration <= 0 {
		duration = 350 * time.Millisecond
	}
	m.cfg.Haptics.LeftMotor = left
	m.cfg.Haptics.RightMotor = right
	m.pulseActive = true
	m.pulseUntil = time.Now().Add(duration)
	pkt := BuildOutputReport(m.cfg, m.info.Connection, m.btSeq)
	m.btSeq = (m.btSeq + 1) & 0x0F
	m.tel.LastOutputHex = FormatHexPreview(pkt)
	m.tel.PacketsTx++
	return nil
}

// SetInputScenario switches the simulated input behavior (diagnostics demos).
func (m *MockBackend) SetInputScenario(scn device.InputScenario) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.scenario != scn {
		m.scenario = scn
		m.scenarioAt = time.Now()
	}
}

// BatteryDetails exposes the mock's battery state, labeled as simulated.
func (m *MockBackend) BatteryDetails() []device.BatteryAttribute {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := "Discharging"
	if m.tel.Charging {
		status = "Charging"
	}
	return []device.BatteryAttribute{
		{Name: "capacity", Value: fmt.Sprintf("%d%%", m.tel.BatteryPercent), Source: "simulated"},
		{Name: "status", Value: status, Source: "simulated"},
	}
}

func (m *MockBackend) Close() error {
	return nil
}

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60.0, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255)
}
