package dualsense

import (
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system"
)

const (
	ModelDualSense     = "sony.dualsense"
	ModelDualSenseEdge = "sony.dualsense_edge"
	Family             = "dualsense"
)

var dualSenseRegions = []device.Region{
	{ID: device.RegionLeftTrigger, Label: "L2 Adaptive Trigger", Badge: "L2 Trigger", Capability: device.CapAdaptiveTriggers, Slot: device.SlotTopLeft,
		Description: "Configure adaptive trigger resistance modes, weapon snap points, and force feedback curves."},
	{ID: device.RegionRightTrigger, Label: "R2 Adaptive Trigger", Badge: "R2 Trigger", Capability: device.CapAdaptiveTriggers, Slot: device.SlotTopRight,
		Description: "Configure adaptive trigger resistance modes, weapon snap points, and force feedback curves."},
	{ID: device.RegionLight, Label: "Touchpad & Lightbar", Badge: "Lightbar", Capability: device.CapLighting, Slot: device.SlotTopCenter,
		Description: "Customize lightbar RGB color, brightness, animations, and player indicator LEDs."},
	{ID: device.RegionLeftStick, Label: "Left Analog Stick", Badge: "L-Stick", Capability: device.CapSticks, Slot: device.SlotMidLeft,
		Description: "Tune deadzones and response curves, and inspect live stick drift."},
	{ID: device.RegionRightStick, Label: "Right Analog Stick", Badge: "R-Stick", Capability: device.CapSticks, Slot: device.SlotMidRight,
		Description: "Tune deadzones and response curves, and inspect live stick drift."},
	{ID: device.RegionHaptics, Label: "Haptic Actuators", Badge: "Haptics", Capability: device.CapHaptics, Slot: device.SlotLowLeft,
		Description: "Adjust dual voice-coil actuator intensity and fire live test pulses."},
	{ID: device.RegionAudio, Label: "Microphone & Speaker", Badge: "Mic/Audio", Capability: device.CapAudio, Slot: device.SlotLowRight,
		Description: "Control the mute LED, microphone gain, speaker, and headset volume."},
	{ID: device.RegionMotion, Label: "6-Axis Motion Sensor", Badge: "Gyro/IMU", Capability: device.CapMotion, Slot: device.SlotBottomCenter,
		Description: "Toggle gyroscope/accelerometer, sensitivity, and 3D tilt sync."},
}

func baseCapabilities() device.CapabilitySet {
	return device.NewCapabilitySet(
		device.CapabilitySpec{ID: device.CapBattery, Support: device.Full},
		device.CapabilitySpec{ID: device.CapConnection, Support: device.Full},
		device.CapabilitySpec{ID: device.CapFirmwareInfo, Support: device.Full},
		device.CapabilitySpec{ID: device.CapHaptics, Support: device.Full,
			Features: []device.Feature{device.FeatDualMotors, device.FeatTestPulse, device.FeatMotorConfig}},
		device.CapabilitySpec{ID: device.CapAdaptiveTriggers, Support: device.Full},
		device.CapabilitySpec{ID: device.CapLighting, Support: device.Full,
			Features: []device.Feature{device.FeatLightbarRGB, device.FeatPlayerLEDs, device.FeatMicLED, device.FeatLEDBrightness, device.FeatLEDAnimation}},
		device.CapabilitySpec{ID: device.CapSticks, Support: device.Full,
			Features: []device.Feature{device.FeatDeadzones, device.FeatResponseCurve}},
		device.CapabilitySpec{ID: device.CapMotion, Support: device.Full,
			Features: []device.Feature{device.FeatGyro, device.FeatAccel}},
		device.CapabilitySpec{ID: device.CapAudio, Support: device.Full,
			Features: []device.Feature{device.FeatSpeaker, device.FeatHeadset, device.FeatMicrophone}},
		device.CapabilitySpec{ID: device.CapProfiles, Support: device.Full},
		device.CapabilitySpec{ID: device.CapDiagnostics, Support: device.Full,
			Features: []device.Feature{device.FeatRawReports, device.FeatTouchpad}},
		device.CapabilitySpec{ID: device.CapFirmwareUpdate, Support: device.Unsupported,
			Note: "TuiSense never installs firmware. Use Sony's official updater (PlayStation Accessories on Windows, or a PS5)."},
		device.CapabilitySpec{ID: device.CapFirmwareCheck, Support: device.Full,
			Note: "Read-only comparison with Sony's published firmware metadata."},
		device.CapabilitySpec{ID: device.CapRemapping, Support: device.Unsupported,
			Note: "Button remapping requires the DualSense Edge."},
		device.CapabilitySpec{ID: device.CapCalibration, Support: device.Unsupported,
			Note: "Stick calibration is not implemented yet (planned)."},
	)
}

var dualSenseActions = []device.Action{
	{ID: "pulse_test", Label: "Test Vibration", Capability: device.CapHaptics,
		Command: device.Command{Type: device.CmdPulseHaptics, PulseLeft: 180, PulseRight: 210, PulseDur: 320 * time.Millisecond}},
}

var dualSenseButtons = []device.ButtonDef{
	{ID: device.ButtonSouth, Label: "✕ Cross", Group: "Face"},
	{ID: device.ButtonEast, Label: "○ Circle", Group: "Face"},
	{ID: device.ButtonWest, Label: "□ Square", Group: "Face"},
	{ID: device.ButtonNorth, Label: "△ Triangle", Group: "Face"},
	{ID: device.ButtonDpadUp, Label: "D-pad ↑", Group: "D-pad"},
	{ID: device.ButtonDpadDown, Label: "D-pad ↓", Group: "D-pad"},
	{ID: device.ButtonDpadLeft, Label: "D-pad ←", Group: "D-pad"},
	{ID: device.ButtonDpadRight, Label: "D-pad →", Group: "D-pad"},
	{ID: device.ButtonL1, Label: "L1", Group: "Shoulder"},
	{ID: device.ButtonR1, Label: "R1", Group: "Shoulder"},
	{ID: device.ButtonL2Digital, Label: "L2 (digital)", Group: "Shoulder"},
	{ID: device.ButtonR2Digital, Label: "R2 (digital)", Group: "Shoulder"},
	{ID: device.ButtonL3, Label: "L3", Group: "Stick"},
	{ID: device.ButtonR3, Label: "R3", Group: "Stick"},
	{ID: device.ButtonMenu, Label: "Options", Group: "System"},
	{ID: device.ButtonView, Label: "Create", Group: "System"},
	{ID: device.ButtonHome, Label: "PS", Group: "System"},
	{ID: device.ButtonTouchpadClick, Label: "Touchpad click", Group: "System"},
	{ID: device.ButtonMicMute, Label: "Mic mute", Group: "System"},
}

func dualSenseInputs(edge bool) device.InputLayout {
	buttons := append([]device.ButtonDef(nil), dualSenseButtons...)
	if edge {
		buttons = append(buttons,
			device.ButtonDef{ID: device.ButtonLeftFn, Label: "Left Fn", Group: "Extra"},
			device.ButtonDef{ID: device.ButtonRightFn, Label: "Right Fn", Group: "Extra"},
			device.ButtonDef{ID: device.ButtonLeftPaddle, Label: "Left back", Group: "Extra"},
			device.ButtonDef{ID: device.ButtonRightPaddle, Label: "Right back", Group: "Extra"},
		)
	}
	return device.InputLayout{
		Buttons:     buttons,
		StickLabels: [2]string{"Left stick", "Right stick"},
		StickRange:  device.AxisRange{Min: 0, Max: 255, Center: 127.5},
		Drift: device.DriftHeuristics{
			CenterTolerance: 4,
			NoiseTolerance:  1.5,
			Rationale: "Heuristic for an 8-bit stick ADC (1 count ≈ 0.8% of half travel; a perfect center reads 127 or 128). " +
				"Not a Sony specification.",
		},
		TriggerLabels:    [2]string{"L2", "R2"},
		TriggerRange:     device.AxisRange{Min: 0, Max: 255, Center: 0},
		TouchpadContacts: 2,
		TouchpadSize:     [2]int{1920, 1080},
		AccelLSBPerG:     8192, // nominal, uncalibrated
		BatteryStep:      10,
		Audio: device.AudioMatch{
			USBIDs:      []string{"054c:0ce6", "054c:0df2"},
			NameHints:   []string{"DualSense", "Wireless Controller"},
			RequiresUSB: true,
		},
	}
}

// Descriptor returns the static model description of the standard DualSense.
func Descriptor() device.Descriptor {
	return device.Descriptor{
		ModelID:        ModelDualSense,
		Family:         Family,
		Brand:          "Sony",
		DisplayName:    "DualSense Wireless Controller",
		ShortName:      "DualSense",
		VisualID:       "dualsense",
		Accent:         device.RGB{R: 70, G: 120, B: 255},
		VendorID:       system.SonyVendorID,
		ProductIDs:     []uint16{system.DualSenseProductID},
		Regions:        dualSenseRegions,
		Inputs:         dualSenseInputs(false),
		Capabilities:   baseCapabilities(),
		Actions:        dualSenseActions,
		DefaultConfig:  device.DefaultConfig,
		DefaultProfile: "Astro Standard",
	}
}

// EdgeDescriptor returns the DualSense Edge model description.
func EdgeDescriptor() device.Descriptor {
	d := Descriptor()
	d.ModelID = ModelDualSenseEdge
	d.DisplayName = "DualSense Edge Wireless Controller"
	d.ShortName = "DualSense Edge"
	d.ProductIDs = []uint16{system.DualSenseEdgePID}
	caps := baseCapabilities()
	caps[device.CapProfiles] = device.CapabilitySpec{ID: device.CapProfiles, Support: device.Full,
		Features: []device.Feature{device.FeatHardwareSlots}}
	caps[device.CapRemapping] = device.CapabilitySpec{ID: device.CapRemapping, Support: device.Limited,
		Note: "Hardware Fn profile slots are detected; on-device remapping editor is planned."}
	d.Capabilities = caps
	d.Inputs = dualSenseInputs(true)
	return d
}

// runtimeCapabilities narrows the baseline for the active transport.
func runtimeCapabilities(base device.CapabilitySet, conn device.ConnectionType) device.CapabilitySet {
	if conn == device.ConnBluetooth {
		return base.WithoutFeature(device.CapAudio, device.FeatSpeaker,
			"Internal speaker routing is only available over USB-C.")
	}
	return base
}
