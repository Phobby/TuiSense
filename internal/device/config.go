package device

// LightbarAnimation defines software-driven or hardware lightbar effects.
type LightbarAnimation string

const (
	AnimSolid   LightbarAnimation = "Solid"
	AnimPulse   LightbarAnimation = "Breathing Pulse"
	AnimRainbow LightbarAnimation = "Spectrum Cycle"
	AnimBattery LightbarAnimation = "Battery Gradient"
	AnimOff     LightbarAnimation = "Off"
)

var AllLightbarAnimations = []LightbarAnimation{
	AnimSolid,
	AnimPulse,
	AnimRainbow,
	AnimBattery,
	AnimOff,
}

// MicLEDMode represents the DualSense mute button LED state.
type MicLEDMode uint8

const (
	MicLEDOff   MicLEDMode = 0
	MicLEDSolid MicLEDMode = 1
	MicLEDPulse MicLEDMode = 2
)

func (m MicLEDMode) String() string {
	switch m {
	case MicLEDSolid:
		return "Solid Amber"
	case MicLEDPulse:
		return "Pulsing Amber"
	default:
		return "Off"
	}
}

// BrightnessLevel represents DualSense player/lightbar LED brightness.
type BrightnessLevel uint8

const (
	BrightnessHigh   BrightnessLevel = 0 // 0x00 in DualSense HID protocol = High
	BrightnessMedium BrightnessLevel = 1 // 0x01 = Medium
	BrightnessLow    BrightnessLevel = 2 // 0x02 = Low
)

func (b BrightnessLevel) String() string {
	switch b {
	case BrightnessMedium:
		return "Medium (65%)"
	case BrightnessLow:
		return "Low (30%)"
	default:
		return "High (100%)"
	}
}

// StickCurve defines the response curve applied to analog stick axes.
type StickCurve string

const (
	CurveLinear     StickCurve = "Linear (1:1)"
	CurvePrecision  StickCurve = "Precision (Fine Center)"
	CurveDynamic    StickCurve = "Dynamic (S-Curve)"
	CurveAggressive StickCurve = "Quick (Fast Ramp)"
)

var AllStickCurves = []StickCurve{
	CurveLinear,
	CurvePrecision,
	CurveDynamic,
	CurveAggressive,
}

// LightingConfig holds RGB lightbar, player LED, and mic LED configuration.
type LightingConfig struct {
	Enabled      bool              `json:"enabled"`
	Red          uint8             `json:"red"`
	Green        uint8             `json:"green"`
	Blue         uint8             `json:"blue"`
	Brightness   BrightnessLevel   `json:"brightness"`
	Animation    LightbarAnimation `json:"animation"`
	PlayerMask   uint8             `json:"player_mask"` // 5-bit bitmask (0x00..0x1F)
	PlayerNumber int               `json:"player_number"`
	MicLED       MicLEDMode        `json:"mic_led"`
}

// HapticsConfig holds dual-motor rumble and audio-haptic attenuation settings.
type HapticsConfig struct {
	LeftMotor       uint8 `json:"left_motor"`       // 0..255 (Low-frequency heavy weight)
	RightMotor      uint8 `json:"right_motor"`      // 0..255 (High-frequency crisp detail)
	LinkMotors      bool  `json:"link_motors"`      // Keep left & right synchronized
	MasterIntensity int   `json:"master_intensity"` // 0..100%
	TriggerRumble   bool  `json:"trigger_rumble"`   // Enable trigger motor coupling
	// Per-trigger rumble motors (Xbox-style impulse triggers); ignored by devices
	// without the FeatTriggerMotors haptics feature.
	LeftTriggerMotor  uint8 `json:"left_trigger_motor,omitempty"`
	RightTriggerMotor uint8 `json:"right_trigger_motor,omitempty"`
}

// StickAxisConfig holds tuning for a single analog stick.
type StickAxisConfig struct {
	InnerDeadzone int        `json:"inner_deadzone"` // 0..40%
	OuterDeadzone int        `json:"outer_deadzone"` // 70..100%
	Sensitivity   int        `json:"sensitivity"`    // 50..150%
	Curve         StickCurve `json:"curve"`
}

// SticksConfig holds tuning for both Left and Right analog sticks.
type SticksConfig struct {
	Left       StickAxisConfig `json:"left"`
	Right      StickAxisConfig `json:"right"`
	LinkSticks bool            `json:"link_sticks"`
}

// MotionConfig holds 6-axis IMU gyroscope and accelerometer settings.
type MotionConfig struct {
	GyroEnabled     bool `json:"gyro_enabled"`
	AccelEnabled    bool `json:"accel_enabled"`
	GyroSensitivity int  `json:"gyro_sensitivity"` // 10..200%
	Sync3DViewTilt  bool `json:"sync_3d_view_tilt"`
	InvertPitch     bool `json:"invert_pitch"`
}

// AudioConfig holds internal controller speaker and microphone settings.
type AudioConfig struct {
	SpeakerVolume   int  `json:"speaker_volume"`   // 0..100%
	HeadphoneVolume int  `json:"headphone_volume"` // 0..100%
	MicGain         int  `json:"mic_gain"`         // 0..100%
	ForceMicMute    bool `json:"force_mic_mute"`
}

// Config is the complete user-configurable state of a controller, serializable
// as a named or per-controller JSON profile. It is a superset grouped by
// capability: each device applies only the sections its CapabilitySet supports,
// so one profile format works across controller families.
type Config struct {
	LeftTrigger  TriggerConfig  `json:"left_trigger"`
	RightTrigger TriggerConfig  `json:"right_trigger"`
	LinkTriggers bool           `json:"link_triggers"`
	Haptics      HapticsConfig  `json:"haptics"`
	Lighting     LightingConfig `json:"lighting"`
	Sticks       SticksConfig   `json:"sticks"`
	Motion       MotionConfig   `json:"motion"`
	Audio        AudioConfig    `json:"audio"`
}

// DefaultConfig returns factory-balanced settings shared by all controller families.
func DefaultConfig() Config {
	return Config{
		LeftTrigger:  DefaultTriggerConfig(),
		RightTrigger: DefaultTriggerConfig(),
		LinkTriggers: false,
		Haptics: HapticsConfig{
			LeftMotor:       0,
			RightMotor:      0,
			LinkMotors:      true,
			MasterIntensity: 80,
			TriggerRumble:   true,
		},
		Lighting: LightingConfig{
			Enabled:      true,
			Red:          0,
			Green:        114,
			Blue:         255, // Iconic PlayStation Astro Blue
			Brightness:   BrightnessHigh,
			Animation:    AnimSolid,
			PlayerMask:   0x04, // Center LED (Player 1)
			PlayerNumber: 1,
			MicLED:       MicLEDOff,
		},
		Sticks: SticksConfig{
			Left: StickAxisConfig{
				InnerDeadzone: 6,
				OuterDeadzone: 96,
				Sensitivity:   100,
				Curve:         CurveLinear,
			},
			Right: StickAxisConfig{
				InnerDeadzone: 5,
				OuterDeadzone: 98,
				Sensitivity:   100,
				Curve:         CurvePrecision,
			},
			LinkSticks: false,
		},
		Motion: MotionConfig{
			GyroEnabled:     true,
			AccelEnabled:    true,
			GyroSensitivity: 100,
			Sync3DViewTilt:  true,
			InvertPitch:     false,
		},
		Audio: AudioConfig{
			SpeakerVolume:   75,
			HeadphoneVolume: 80,
			MicGain:         70,
			ForceMicMute:    false,
		},
	}
}
