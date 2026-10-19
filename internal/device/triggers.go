package device

import "math"

// TriggerMode defines the DualSense adaptive trigger effect mode.
type TriggerMode string

const (
	TriggerModeOff        TriggerMode = "Off"
	TriggerModeResistance TriggerMode = "Continuous Resistance"
	TriggerModeWeapon     TriggerMode = "Weapon (Snap Release)"
	TriggerModeFeedback   TriggerMode = "Sectional Feedback"
	TriggerModeVibration  TriggerMode = "Rapid Pulse / Automatic"
	TriggerModeCustom     TriggerMode = "Custom Curve"
)

// AllTriggerModes lists all supported adaptive trigger modes in UI display order.
var AllTriggerModes = []TriggerMode{
	TriggerModeOff,
	TriggerModeResistance,
	TriggerModeWeapon,
	TriggerModeFeedback,
	TriggerModeVibration,
	TriggerModeCustom,
}

// TriggerPreset represents a named quick-select preset for adaptive triggers.
type TriggerPreset struct {
	Name        string
	Description string
	Config      TriggerConfig
}

// BuiltinTriggerPresets provides curated PlayStation-style adaptive trigger profiles.
var BuiltinTriggerPresets = []TriggerPreset{
	{
		Name:        "Linear Free (Off)",
		Description: "Zero motor engagement; smooth linear spring travel.",
		Config: TriggerConfig{
			Mode:          TriggerModeOff,
			StartPosition: 0,
			EndPosition:   255,
			Strength:      0,
			Frequency:     0,
		},
	},
	{
		Name:        "Heavy Bow Draw",
		Description: "Continuous resistance starting early in the pull.",
		Config: TriggerConfig{
			Mode:          TriggerModeResistance,
			StartPosition: 25,
			EndPosition:   255,
			Strength:      210,
			Frequency:     0,
		},
	},
	{
		Name:        "Crisp Pistol Break",
		Description: "Distinct wall at 30% with a sharp break at 65%.",
		Config: TriggerConfig{
			Mode:          TriggerModeWeapon,
			StartPosition: 72,
			EndPosition:   165,
			Strength:      230,
			Frequency:     0,
		},
	},
	{
		Name:        "ABS Brake Pedal",
		Description: "Firm zone resistance with high-frequency ABS shudder.",
		Config: TriggerConfig{
			Mode:          TriggerModeVibration,
			StartPosition: 60,
			EndPosition:   230,
			Strength:      195,
			Frequency:     38,
		},
	},
	{
		Name:        "Half-Pull Detent",
		Description: "Tactile step at 50% travel for dual-stage actions.",
		Config: TriggerConfig{
			Mode:          TriggerModeFeedback,
			StartPosition: 110,
			EndPosition:   160,
			Strength:      200,
			Frequency:     0,
		},
	},
}

// TriggerConfig stores parameters for a single adaptive trigger (L2 or R2).
type TriggerConfig struct {
	Mode          TriggerMode `json:"mode"`
	StartPosition uint8       `json:"start_position"` // 0..255
	EndPosition   uint8       `json:"end_position"`   // 0..255
	Strength      uint8       `json:"strength"`       // 0..255
	Frequency     uint8       `json:"frequency"`      // 0..100 Hz (used in Vibration/Custom modes)
}

// DefaultTriggerConfig returns a default disengaged trigger configuration.
func DefaultTriggerConfig() TriggerConfig {
	return TriggerConfig{
		Mode:          TriggerModeOff,
		StartPosition: 40,
		EndPosition:   180,
		Strength:      160,
		Frequency:     25,
	}
}

// ForceAt computes the normalized resistive force [0.0, 1.0] at a normalized
// trigger pull position t in [0.0, 1.0]. Used both for the live Braille
// force-curve graph and 3D trigger tension visualization.
func (tc TriggerConfig) ForceAt(t float64) float64 {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	start := float64(tc.StartPosition) / 255.0
	end := float64(tc.EndPosition) / 255.0
	if end <= start+0.05 {
		end = math.Min(1.0, start+0.15)
	}
	strength := float64(tc.Strength) / 255.0

	switch tc.Mode {
	case TriggerModeOff:
		// Baseline passive spring tension only
		return 0.06 + 0.12*t

	case TriggerModeResistance:
		if t < start {
			return 0.06 + 0.10*t
		}
		ramp := math.Min(1.0, (t-start)/0.08)
		return math.Min(1.0, 0.12+ramp*strength*0.88)

	case TriggerModeWeapon:
		if t < start {
			return 0.08 + 0.12*t
		}
		if t <= end {
			// Rising wall up to the break point
			progress := (t - start) / (end - start)
			return math.Min(1.0, 0.20+strength*(0.55+0.25*progress))
		}
		// Sharp snap drop-off after break point
		return 0.10 + 0.08*(1.0-t)

	case TriggerModeFeedback:
		if t >= start && t <= end {
			center := (start + end) * 0.5
			halfSpan := (end - start) * 0.5
			dist := math.Abs(t-center) / math.Max(0.01, halfSpan)
			bell := math.Max(0, 1.0-dist*dist)
			return math.Min(1.0, 0.12+strength*0.85*bell)
		}
		return 0.08 + 0.10*t

	case TriggerModeVibration:
		if t < start {
			return 0.08 + 0.10*t
		}
		freq := float64(tc.Frequency)
		if freq < 4 {
			freq = 20
		}
		osc := 0.5 + 0.5*math.Sin(t*freq*math.Pi*0.6)
		return math.Min(1.0, 0.15+strength*(0.45+0.40*osc))

	case TriggerModeCustom:
		if t < start {
			return 0.08
		}
		if t > end {
			return 0.15 + 0.25*strength
		}
		norm := (t - start) / (end - start)
		wave := 0.0
		if tc.Frequency > 0 {
			wave = 0.18 * math.Sin(norm*float64(tc.Frequency)*0.5)
		}
		return math.Min(1.0, math.Max(0.05, 0.12+strength*(0.3+0.55*norm+wave)))

	default:
		return 0.08
	}
}

// ApplyStickCurve maps a raw stick coordinate [-1.0, 1.0] through the configured
// inner/outer deadzones, sensitivity multiplier, and response curve.
func ApplyStickCurve(raw float64, cfg StickAxisConfig) float64 {
	sign := 1.0
	if raw < 0 {
		sign = -1.0
	}
	mag := math.Abs(raw)
	inner := float64(cfg.InnerDeadzone) / 100.0
	outer := float64(cfg.OuterDeadzone) / 100.0
	if outer <= inner+0.05 {
		outer = math.Min(1.0, inner+0.10)
	}
	if mag <= inner {
		return 0
	}
	norm := (mag - inner) / (outer - inner)
	if norm > 1.0 {
		norm = 1.0
	}

	var curved float64
	switch cfg.Curve {
	case CurvePrecision:
		curved = math.Pow(norm, 1.65)
	case CurveDynamic:
		// Smoothstep S-curve
		curved = norm * norm * (3.0 - 2.0*norm)
	case CurveAggressive:
		curved = math.Pow(norm, 0.65)
	default:
		curved = norm
	}

	sens := float64(cfg.Sensitivity) / 100.0
	if sens <= 0 {
		sens = 1.0
	}
	out := curved * sens
	if out > 1.0 {
		out = 1.0
	}
	return sign * out
}
