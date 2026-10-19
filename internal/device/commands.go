package device

import (
	"fmt"
	"time"
)

// CommandType identifies the kind of device write operation.
type CommandType string

const (
	CmdApplyFullConfig CommandType = "apply_full_config"
	CmdSetLighting     CommandType = "set_lighting"
	CmdSetTrigger      CommandType = "set_trigger"
	CmdSetHaptics      CommandType = "set_haptics"
	CmdPulseHaptics    CommandType = "pulse_haptics"
	CmdSetMicLED       CommandType = "set_mic_led"
	CmdSetSpeakerVol   CommandType = "set_speaker_volume"
)

// Command represents an atomic action dispatched to the active controller backend.
type Command struct {
	Type         CommandType
	Config       Config
	LeftTrigger  *TriggerConfig
	RightTrigger *TriggerConfig
	Lighting     *LightingConfig
	Haptics      *HapticsConfig
	MicLED       *MicLEDMode
	PulseLeft    uint8
	PulseRight   uint8
	PulseDur     time.Duration
	// PulseAbsolute makes a pulse ignore the profile's master haptic gain, so a
	// diagnostic pulse has a known, bounded intensity.
	PulseAbsolute bool
}

// CoalesceKey returns a key used by ActionDispatcher to coalesce rapid slider updates
// of the same category without dropping distinct command device.
func (c Command) CoalesceKey() string {
	return string(c.Type)
}

// Describe returns a concise human-readable description for logging.
func (c Command) Describe() string {
	switch c.Type {
	case CmdApplyFullConfig:
		return fmt.Sprintf("ApplyFullConfig(RGB=#%02X%02X%02X, L2=%s, R2=%s)",
			c.Config.Lighting.Red, c.Config.Lighting.Green, c.Config.Lighting.Blue,
			c.Config.LeftTrigger.Mode, c.Config.RightTrigger.Mode)
	case CmdSetLighting:
		if c.Lighting != nil {
			return fmt.Sprintf("SetLighting(RGB=#%02X%02X%02X, Bri=%s, Anim=%s, Mic=%s)",
				c.Lighting.Red, c.Lighting.Green, c.Lighting.Blue,
				c.Lighting.Brightness, c.Lighting.Animation, c.Lighting.MicLED)
		}
		return "SetLighting"
	case CmdSetTrigger:
		return fmt.Sprintf("SetTriggers(L2=%s[str:%d], R2=%s[str:%d])",
			c.Config.LeftTrigger.Mode, c.Config.LeftTrigger.Strength,
			c.Config.RightTrigger.Mode, c.Config.RightTrigger.Strength)
	case CmdSetHaptics:
		return fmt.Sprintf("SetHaptics(L=%d, R=%d, Master=%d%%)",
			c.Config.Haptics.LeftMotor, c.Config.Haptics.RightMotor, c.Config.Haptics.MasterIntensity)
	case CmdPulseHaptics:
		return fmt.Sprintf("PulseHaptics(L=%d, R=%d, Dur=%s)", c.PulseLeft, c.PulseRight, c.PulseDur)
	case CmdSetMicLED:
		if c.MicLED != nil {
			return fmt.Sprintf("SetMicLED(%s)", *c.MicLED)
		}
		return "SetMicLED"
	case CmdSetSpeakerVol:
		return fmt.Sprintf("SetAudio(Speaker=%d%%, MicGain=%d%%)", c.Config.Audio.SpeakerVolume, c.Config.Audio.MicGain)
	default:
		return string(c.Type)
	}
}
