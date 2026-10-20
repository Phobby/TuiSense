// Package xboxmock provides a simulated Xbox-style controller used purely to
// exercise TuiSense's multi-device architecture: device selection, visual
// switching, and capability-driven panels. It talks to no hardware.
package xboxmock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/simulate"
	"github.com/Phobby/TuiSense/internal/logging"
)

const (
	Key     = "test:xbox"
	ModelID = "microsoft.xbox_wireless.test"
	Family  = "xbox"
)

var regions = []device.Region{
	{ID: device.RegionLeftTrigger, Label: "LT Impulse Trigger", Badge: "LT", Capability: device.CapHaptics, Slot: device.SlotTopLeft,
		Description: "Impulse trigger rumble motor strength (simulated)."},
	{ID: device.RegionRightTrigger, Label: "RT Impulse Trigger", Badge: "RT", Capability: device.CapHaptics, Slot: device.SlotTopRight,
		Description: "Impulse trigger rumble motor strength (simulated)."},
	{ID: device.RegionLight, Label: "Guide Button LED", Badge: "Guide", Capability: device.CapLighting, Slot: device.SlotTopCenter,
		Description: "White guide button LED brightness (no RGB)."},
	{ID: device.RegionLeftStick, Label: "Left Thumbstick", Badge: "L-Stick", Capability: device.CapSticks, Slot: device.SlotMidLeft,
		Description: "Tune deadzones and response curves for the offset left stick."},
	{ID: device.RegionRightStick, Label: "Right Thumbstick", Badge: "R-Stick", Capability: device.CapSticks, Slot: device.SlotMidRight,
		Description: "Tune deadzones and response curves for the right stick."},
	{ID: device.RegionHaptics, Label: "Rumble Motors", Badge: "Rumble", Capability: device.CapHaptics, Slot: device.SlotLowLeft,
		Description: "Adjust the four rumble motors (two grips, two impulse triggers)."},
	{ID: device.RegionAudio, Label: "3.5mm Headset Jack", Badge: "Headset", Capability: device.CapAudio, Slot: device.SlotLowRight,
		Description: "Headset volume and chat mix (no internal speaker)."},
}

var xboxButtons = []device.ButtonDef{
	{ID: device.ButtonSouth, Label: "A", Group: "Face"},
	{ID: device.ButtonEast, Label: "B", Group: "Face"},
	{ID: device.ButtonWest, Label: "X", Group: "Face"},
	{ID: device.ButtonNorth, Label: "Y", Group: "Face"},
	{ID: device.ButtonDpadUp, Label: "D-pad ↑", Group: "D-pad"},
	{ID: device.ButtonDpadDown, Label: "D-pad ↓", Group: "D-pad"},
	{ID: device.ButtonDpadLeft, Label: "D-pad ←", Group: "D-pad"},
	{ID: device.ButtonDpadRight, Label: "D-pad →", Group: "D-pad"},
	{ID: device.ButtonL1, Label: "LB", Group: "Shoulder"},
	{ID: device.ButtonR1, Label: "RB", Group: "Shoulder"},
	{ID: device.ButtonL3, Label: "LS (click)", Group: "Stick"},
	{ID: device.ButtonR3, Label: "RS (click)", Group: "Stick"},
	{ID: device.ButtonMenu, Label: "≡ Menu", Group: "System"},
	{ID: device.ButtonView, Label: "⧉ View", Group: "System"},
	{ID: device.ButtonHome, Label: "Xbox", Group: "System"},
	{ID: device.ButtonShare, Label: "Share", Group: "System"},
}

func inputs() device.InputLayout {
	return device.InputLayout{
		Buttons:     xboxButtons,
		StickLabels: [2]string{"Left stick", "Right stick"},
		StickRange:  device.AxisRange{Min: -32768, Max: 32767, Center: -0.5},
		Drift: device.DriftHeuristics{
			CenterTolerance: 600,
			NoiseTolerance:  120,
			Rationale:       "Simulated test-device heuristics for a 16-bit stick range; not an Xbox specification.",
		},
		TriggerLabels: [2]string{"LT", "RT"},
		TriggerRange:  device.AxisRange{Min: 0, Max: 1023},
		BatteryStep:   1,
	}
}

// Descriptor returns the static description of the Xbox test controller.
func Descriptor() device.Descriptor {
	return device.Descriptor{
		ModelID:     ModelID,
		Family:      Family,
		Brand:       "Microsoft",
		DisplayName: "Xbox Wireless Controller (Test Device)",
		ShortName:   "Xbox (Test)",
		VisualID:    "xbox",
		Accent:      device.RGB{R: 60, G: 190, B: 90},
		VendorID:    0x045E,
		ProductIDs:  []uint16{0x0B13},
		Regions:     regions,
		Inputs:      inputs(),
		Capabilities: device.NewCapabilitySet(
			device.CapabilitySpec{ID: device.CapBattery, Support: device.Full, Note: "2× AA batteries (simulated)."},
			device.CapabilitySpec{ID: device.CapConnection, Support: device.Full},
			device.CapabilitySpec{ID: device.CapFirmwareInfo, Support: device.Full},
			device.CapabilitySpec{ID: device.CapHaptics, Support: device.Full,
				Features: []device.Feature{device.FeatDualMotors, device.FeatTriggerMotors, device.FeatTestPulse, device.FeatMotorConfig}},
			device.CapabilitySpec{ID: device.CapAdaptiveTriggers, Support: device.Unsupported,
				Note: "Xbox controllers use impulse trigger motors instead of adaptive resistance."},
			device.CapabilitySpec{ID: device.CapLighting, Support: device.Limited,
				Features: []device.Feature{device.FeatLEDBrightness},
				Note:     "Only the white guide button LED brightness is adjustable."},
			device.CapabilitySpec{ID: device.CapSticks, Support: device.Full,
				Features: []device.Feature{device.FeatDeadzones, device.FeatResponseCurve}},
			device.CapabilitySpec{ID: device.CapMotion, Support: device.Unsupported,
				Note: "This controller has no motion sensors."},
			device.CapabilitySpec{ID: device.CapAudio, Support: device.Limited,
				Features: []device.Feature{device.FeatHeadset},
				Note:     "Headset jack only; there is no internal speaker or microphone."},
			device.CapabilitySpec{ID: device.CapProfiles, Support: device.Full},
			device.CapabilitySpec{ID: device.CapDiagnostics, Support: device.Full},
			device.CapabilitySpec{ID: device.CapFirmwareUpdate, Support: device.Limited,
				Note: "Simulated firmware check only — no real update is performed."},
			device.CapabilitySpec{ID: device.CapRemapping, Support: device.Unsupported,
				Note: "Remapping is planned for a future Xbox backend."},
			device.CapabilitySpec{ID: device.CapCalibration, Support: device.Unsupported},
		),
		Actions: []device.Action{
			{ID: "pulse_test", Label: "Test Rumble", Capability: device.CapHaptics,
				Command: device.Command{Type: device.CmdPulseHaptics, PulseLeft: 200, PulseRight: 160, PulseDur: 300 * time.Millisecond}},
		},
		DefaultConfig: defaultConfig,
	}
}

func defaultConfig() device.Config {
	cfg := device.DefaultConfig()
	cfg.LeftTrigger = device.DefaultTriggerConfig()
	cfg.RightTrigger = device.DefaultTriggerConfig()
	cfg.Lighting.Red, cfg.Lighting.Green, cfg.Lighting.Blue = 235, 240, 235
	cfg.Haptics.LeftTriggerMotor = 120
	cfg.Haptics.RightTriggerMotor = 120
	return cfg
}

// Virtual returns the registry entry for the Xbox test device.
func Virtual() device.VirtualDevice {
	return device.VirtualDevice{
		Key:        Key,
		Source:     device.SourceTest,
		Descriptor: Descriptor(),
		AutoAttach: true,
		New:        func(logger *logging.Logger) device.Device { return New(logger) },
	}
}

// Device is the simulated Xbox controller.
type Device struct {
	gen        *simulate.Generator
	scenario   device.InputScenario
	scenarioAt time.Time

	mu        sync.RWMutex
	desc      device.Descriptor
	info      device.Info
	cfg       device.Config
	tel       device.Telemetry
	logger    *logging.Logger
	startedAt time.Time
}

var (
	_ device.Device                 = (*Device)(nil)
	_ device.Simulator              = (*Device)(nil)
	_ device.ScenarioSimulator      = (*Device)(nil)
	_ device.BatteryDetailsProvider = (*Device)(nil)
)

// New creates a connected Xbox test device.
func New(logger *logging.Logger) *Device {
	now := time.Now()
	desc := Descriptor()
	return &Device{
		gen: simulate.New(desc.Inputs, simulate.Profile{
			// The right stick rests visibly off-center so the drift test has
			// something to report on this simulated device.
			StickOffset: [4]float64{0.004, -0.006, 0.052, -0.031},
			StickNoise:  45,
		}, 11),
		scenario:   device.ScenarioDemo,
		scenarioAt: now,
		desc:       desc,
		cfg:        desc.NewDefaultConfig(),
		logger:     logger,
		startedAt:  now,
		info: device.Info{
			ID:              Key,
			Name:            "Xbox Wireless Controller",
			Model:           "Xbox Wireless Controller (Model 1914) · TEST",
			VendorID:        0x045E,
			ProductID:       0x0B13,
			FirmwareVersion: "5.17.3202",
			HardwareVersion: "1914",
			MACAddress:      "98:7A:14:00:BE:EF",
			HIDRawPath:      "(virtual test device)",
			Connection:      device.ConnBluetooth,
			InputMode:       "Simulated GIP input",
			OutputMode:      "Simulated rumble output",
			Connected:       true,
			LastSeen:        now,
		},
		tel: device.Telemetry{BatteryPercent: 63, UpdatedAt: now},
	}
}

func (d *Device) Descriptor() device.Descriptor { return d.desc }

func (d *Device) Capabilities() device.CapabilitySet { return d.desc.Capabilities }

func (d *Device) Info() device.Info {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.info
}

func (d *Device) Telemetry() device.Telemetry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.tel
}

func (d *Device) CurrentConfig() device.Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

func (d *Device) Poll(_ context.Context) (device.Telemetry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.info.Connected {
		return d.tel, nil
	}
	now := time.Now()
	t := now.Sub(d.startedAt).Seconds()
	fr := d.gen.Frame(d.scenario, now.Sub(d.scenarioAt).Seconds())
	raw := d.gen.Raw(fr)
	sr := d.desc.Inputs.StickRange
	d.tel.Raw = raw
	d.tel.LeftStickX = simulate.Normalize(raw.Sticks[0], sr)
	d.tel.LeftStickY = simulate.Normalize(raw.Sticks[1], sr)
	d.tel.RightStickX = simulate.Normalize(raw.Sticks[2], sr)
	d.tel.RightStickY = simulate.Normalize(raw.Sticks[3], sr)
	d.tel.L2Pressure = float64(raw.Triggers[0]) / 1023
	d.tel.R2Pressure = float64(raw.Triggers[1]) / 1023
	d.tel.Buttons, d.tel.ButtonsLatched = fr.Buttons, fr.Buttons
	d.tel.Simulated = true
	// Simulated AA drain: 1% every 45 s, never below 5%.
	if pct := 63 - int(t/45); pct >= 5 {
		d.tel.BatteryPercent = pct
	}
	d.tel.PacketsRx++
	d.tel.UpdatedAt = now
	d.tel.LastInputHex = fmt.Sprintf("GIP 20 %04X %04X %03X %03X (simulated)",
		uint16(raw.Sticks[0]), uint16(raw.Sticks[1]), raw.Triggers[0], raw.Triggers[1])
	d.info.LastSeen = now
	return d.tel, nil
}

func (d *Device) ApplyConfig(_ context.Context, cfg device.Config) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.info.Connected {
		return fmt.Errorf("controller is disconnected")
	}
	d.cfg = cfg
	d.tel.PacketsTx++
	return nil
}

func (d *Device) ExecuteCommand(_ context.Context, cmd device.Command) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.info.Connected {
		return fmt.Errorf("cannot execute %s: controller disconnected", cmd.Type)
	}
	switch cmd.Type {
	case device.CmdApplyFullConfig:
		d.cfg = cmd.Config
	case device.CmdSetLighting:
		d.cfg.Lighting.Brightness = cmd.Config.Lighting.Brightness
		d.cfg.Lighting.Enabled = cmd.Config.Lighting.Enabled
	case device.CmdSetHaptics:
		d.cfg.Haptics = cmd.Config.Haptics
	case device.CmdSetSpeakerVol:
		d.cfg.Audio.HeadphoneVolume = cmd.Config.Audio.HeadphoneVolume
	case device.CmdSetTrigger:
		return fmt.Errorf("adaptive triggers are not supported by %s", d.desc.ShortName)
	}
	d.tel.PacketsTx++
	d.tel.LastOutputHex = fmt.Sprintf("GIP 09 rumble L=%02X R=%02X LT=%02X RT=%02X",
		d.cfg.Haptics.LeftMotor, d.cfg.Haptics.RightMotor, d.cfg.Haptics.LeftTriggerMotor, d.cfg.Haptics.RightTriggerMotor)
	return nil
}

func (d *Device) Close() error { return nil }

// SetInputScenario switches the simulated input behavior.
func (d *Device) SetInputScenario(scn device.InputScenario) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.scenario != scn {
		d.scenario, d.scenarioAt = scn, time.Now()
	}
}

// BatteryDetails reports the simulated battery source. No health, capacity,
// or cycle data is invented — not even for the test device.
func (d *Device) BatteryDetails() []device.BatteryAttribute {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return []device.BatteryAttribute{
		{Name: "capacity", Value: fmt.Sprintf("%d%%", d.tel.BatteryPercent), Source: "simulated"},
		{Name: "power source", Value: "2× AA batteries", Source: "simulated"},
	}
}

// ToggleConnection simulates unplugging/replugging the test controller.
func (d *Device) ToggleConnection() bool {
	d.mu.Lock()
	d.info.Connected = !d.info.Connected
	connected := d.info.Connected
	d.mu.Unlock()
	if d.logger != nil {
		d.logger.Info("XBOX-TEST", "Test controller connected=%v", connected)
	}
	return connected
}

// CycleTransport toggles between the wireless dongle and Bluetooth.
func (d *Device) CycleTransport() device.ConnectionType {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.info.Connection == device.ConnBluetooth {
		d.info.Connection = device.ConnWireless
	} else {
		d.info.Connection = device.ConnBluetooth
	}
	return d.info.Connection
}

// TogglePermissionError is a no-op for the test device (no OS device node).
func (d *Device) TogglePermissionError() bool { return false }
