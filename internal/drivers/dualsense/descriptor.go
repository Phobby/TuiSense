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
	{ID: device.RegionLeftTrigger, Label: "L2 Adaptive Trigger", Badge: "L2 Trigger", Capability: device.CapAdaptiveTriggers,
		Description: "Configure adaptive trigger resistance modes, weapon snap points, and force feedback curves."},
	{ID: device.RegionRightTrigger, Label: "R2 Adaptive Trigger", Badge: "R2 Trigger", Capability: device.CapAdaptiveTriggers,
		Description: "Configure adaptive trigger resistance modes, weapon snap points, and force feedback curves."},
	{ID: device.RegionLight, Label: "Touchpad & Lightbar", Badge: "Lightbar", Capability: device.CapLighting,
		Description: "Customize lightbar RGB color, brightness, animations, and player indicator LEDs."},
	{ID: device.RegionLeftStick, Label: "Left Analog Stick", Badge: "L-Stick", Capability: device.CapSticks,
		Description: "Tune deadzones and response curves, and inspect live stick drift."},
	{ID: device.RegionRightStick, Label: "Right Analog Stick", Badge: "R-Stick", Capability: device.CapSticks,
		Description: "Tune deadzones and response curves, and inspect live stick drift."},
	{ID: device.RegionHaptics, Label: "Haptic Actuators", Badge: "Haptics", Capability: device.CapHaptics,
		Description: "Adjust dual voice-coil actuator intensity and fire live test pulses."},
	{ID: device.RegionAudio, Label: "Microphone & Speaker", Badge: "Mic/Audio", Capability: device.CapAudio,
		Description: "Control the mute LED, microphone gain, speaker, and headset volume."},
	{ID: device.RegionMotion, Label: "6-Axis Motion Sensor", Badge: "Gyro/IMU", Capability: device.CapMotion,
		Description: "Toggle gyroscope/accelerometer, sensitivity, and 3D tilt sync."},
}

// Access constants used by the DualSense declarations. Every setting below is
// sent to the controller in its output report and is not stored on it: it is
// re-applied on every connection.
const (
	readTest    = device.AccessRead | device.AccessTest
	configTest  = device.AccessTest | device.AccessConfigDevice
	hidraw      = "dualsense-hidraw"
	automatedV  = device.VerifiedAutomated
	usbOnlyNote = "Controller audio is a USB audio device; connect with a USB cable."
)

func baseCapabilities() device.CapabilitySet {
	return device.NewCapabilitySet(
		device.CapabilitySpec{ID: device.CapBattery, Support: device.Full, Access: device.AccessRead, Backend: hidraw, Verification: automatedV},
		device.CapabilitySpec{ID: device.CapConnection, Support: device.Full, Access: device.AccessRead, Backend: hidraw, Verification: automatedV},
		device.CapabilitySpec{ID: device.CapFirmwareInfo, Support: device.Full, Access: device.AccessRead, Backend: hidraw, Verification: automatedV},
		device.CapabilitySpec{ID: device.CapHaptics, Support: device.Full, Access: configTest, Backend: hidraw, Verification: automatedV,
			Features: []device.Feature{device.FeatDualMotors, device.FeatTestPulse, device.FeatMotorConfig}},
		device.CapabilitySpec{ID: device.CapAdaptiveTriggers, Support: device.Full, Access: configTest, Backend: hidraw, Verification: automatedV},
		device.CapabilitySpec{ID: device.CapLighting, Support: device.Full, Access: device.AccessConfigDevice, Backend: hidraw, Verification: automatedV,
			Features: []device.Feature{device.FeatLightbarRGB, device.FeatPlayerLEDs, device.FeatMicLED, device.FeatLEDBrightness, device.FeatLEDAnimation}},
		device.CapabilitySpec{ID: device.CapSticks, Support: device.Full, Access: readTest | device.AccessPreview, Backend: hidraw, Verification: automatedV,
			Features: []device.Feature{device.FeatDeadzones, device.FeatResponseCurve},
			Note:     "Deadzones and curves are previews: TuiSense has no input-processing backend, so games receive the raw stick values."},
		device.CapabilitySpec{ID: device.CapMotion, Support: device.Full, Access: readTest | device.AccessPreview, Backend: hidraw, Verification: automatedV,
			Features: []device.Feature{device.FeatGyro, device.FeatAccel}},
		device.CapabilitySpec{ID: device.CapTouchpad, Support: device.Full, Access: readTest, Backend: hidraw, Verification: automatedV},
		device.CapabilitySpec{ID: device.CapAudio, Support: device.Full, Access: configTest, Backend: "dualsense-hidraw + alsa", Verification: automatedV,
			Transports: []device.ConnectionType{device.ConnUSB}, Note: usbOnlyNote,
			Features: []device.Feature{device.FeatSpeaker, device.FeatHeadset, device.FeatMicrophone}},
		device.CapabilitySpec{ID: device.CapProfiles, Support: device.Full, Access: device.AccessConfigDevice, Backend: "tuisense-profiles", Verification: automatedV,
			Note: "Profiles are stored by TuiSense and applied to the controller when it connects."},
		device.CapabilitySpec{ID: device.CapDiagnostics, Support: device.Full, Access: readTest, Backend: hidraw, Verification: automatedV,
			Features: []device.Feature{device.FeatRawReports, device.FeatTouchpad}},
		device.InaccessibleSpec(device.CapFirmwareUpdate, device.RequiresBackend,
			"TuiSense never installs firmware. Use Sony's official updater (PlayStation Accessories on Windows, or a PS5)."),
		device.CapabilitySpec{ID: device.CapFirmwareCheck, Support: device.Full, Access: device.AccessRead, Backend: "sony-firmware-metadata", Verification: automatedV,
			Note: "Read-only comparison with Sony's published firmware metadata."},
		device.AbsentSpec(device.CapRemapping),
		device.AbsentSpec(device.CapRearControls),
		device.InaccessibleSpec(device.CapCalibration, device.RequiresBackend, "Stick calibration is not implemented yet (planned)."),
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
		LinuxDrivers:   []string{"playstation"},
	}
}

// EdgeDescriptor returns the DualSense Edge model description.
func EdgeDescriptor() device.Descriptor {
	d := Descriptor()
	d.ModelID = ModelDualSenseEdge
	d.DisplayName = "DualSense Edge Wireless Controller"
	d.ShortName = "DualSense Edge"
	d.VisualID = "dualsense_edge"
	d.ProductIDs = []uint16{system.DualSenseEdgePID}
	caps := baseCapabilities()
	profiles := caps[device.CapProfiles]
	profiles.Features = []device.Feature{device.FeatHardwareSlots}
	caps[device.CapProfiles] = profiles
	caps[device.CapRemapping] = device.InaccessibleSpec(device.CapRemapping, device.RequiresBackend,
		"On-controller remapping and Fn profile slots use Sony's protocol, which TuiSense does not write; use a PS5 or the PlayStation Accessories app.")
	caps[device.CapRearControls] = device.CapabilitySpec{ID: device.CapRearControls, Support: device.Full, Access: readTest, Backend: hidraw,
		Verification: device.VerifiedAutomated, Note: "Back buttons and Fn buttons are read from the input report (bit layout covered by protocol tests)."}
	d.Capabilities = caps
	d.Inputs = dualSenseInputs(true)
	return d
}

// runtimeCapabilities narrows the baseline for the active transport.
func runtimeCapabilities(base device.CapabilitySet, conn device.ConnectionType) device.CapabilitySet {
	return base.ForConnection(conn)
}
