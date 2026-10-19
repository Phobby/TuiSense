// Package simulate generates deterministic, scenario-driven input streams for
// mock and test devices. Every value it produces is synthetic; telemetry built
// from it must carry Telemetry.Simulated = true.
package simulate

import (
	"math"
	"math/rand"

	"github.com/Phobby/TuiSense/internal/device"
)

// Profile tunes the synthetic imperfections of a simulated controller.
type Profile struct {
	StickOffset [4]float64 // resting offset per axis, normalized [-1,1]
	StickNoise  float64    // standard deviation in raw counts
	TriggerRest [2]float64 // resting trigger value, normalized
	GyroNoise   float64    // raw counts
	GyroBias    [3]float64 // raw counts
	GravityAxis int        // accel axis carrying gravity when resting (0..2)
	GravitySign float64
}

// Frame is one simulated input sample in normalized units.
type Frame struct {
	Sticks   [4]float64 // [-1,1]
	Triggers [2]float64 // [0,1]
	Buttons  device.ButtonSet
	Touch    [2]device.TouchPoint
	GyroDPS  [3]float64 // angular rate, °/s
	AccelG   [3]float64 // g
	Tilt     [3]float64 // pitch/yaw/roll degrees for the 3D visual
}

// Generator produces frames for a scenario.
type Generator struct {
	Layout  device.InputLayout
	Profile Profile
	rng     *rand.Rand
}

// New creates a generator with a fixed seed (deterministic noise).
func New(layout device.InputLayout, profile Profile, seed int64) *Generator {
	return &Generator{Layout: layout, Profile: profile, rng: rand.New(rand.NewSource(seed))}
}

// Frame computes the sample at time t (seconds since scenario start).
func (g *Generator) Frame(scn device.InputScenario, t float64) Frame {
	var f Frame
	p := g.Profile
	f.Sticks = p.StickOffset
	f.Triggers = p.TriggerRest
	f.AccelG[clampAxis(p.GravityAxis)] = p.GravitySign

	switch scn {
	case device.ScenarioDemo, "":
		f.Sticks = [4]float64{0.38 * math.Sin(t*1.1), 0.28 * math.Cos(t*1.7), 0.45 * math.Cos(t*0.9), 0.35 * math.Sin(t*1.4)}
		f.Triggers = [2]float64{0.5 + 0.48*math.Sin(t*0.85), 0.5 + 0.48*math.Sin(t*0.85+1.2)}
		f.Tilt = [3]float64{11 * math.Sin(t*0.75), 15 * math.Cos(t*0.55), 8.5 * math.Sin(t*0.95)}
		f.GyroDPS = [3]float64{8.2 * math.Cos(t*0.75), -8.2 * math.Sin(t*0.55), 8.1 * math.Cos(t*0.95)}
		if g.Layout.TouchpadContacts > 0 && int(t)%4 != 0 {
			f.Touch[0] = g.touchAt(0, t)
		}
	case device.ScenarioIdle, device.ScenarioStill:
		// released: profile offsets only
	case device.ScenarioMove:
		a := t * math.Pi // one revolution per 2 s
		f.Sticks = [4]float64{math.Cos(a), math.Sin(a), math.Cos(-a + 0.7), math.Sin(-a + 0.7)}
	case device.ScenarioButtons:
		if len(g.Layout.Buttons) > 0 {
			step := int(t / 0.6)
			if math.Mod(t, 0.6) < 0.4 {
				f.Buttons = f.Buttons.With(g.Layout.Buttons[step%len(g.Layout.Buttons)].ID)
			}
		}
	case device.ScenarioTriggers:
		phase := math.Mod(t, 3) / 3 // 0..1
		v := 1 - math.Abs(2*phase-1)
		f.Triggers = [2]float64{v, v}
	case device.ScenarioTouch:
		if g.Layout.TouchpadContacts > 0 {
			f.Touch[0] = g.touchAt(0, t)
			if g.Layout.TouchpadContacts > 1 && math.Mod(t, 4) > 2 {
				f.Touch[1] = g.touchAt(1, t+1.3)
			}
			if math.Mod(t, 2) < 0.25 {
				f.Buttons = f.Buttons.With(device.ButtonTouchpadClick)
			}
		}
	case device.ScenarioMotion:
		axis := int(t/2) % 3
		rate := 90 * math.Sin(math.Mod(t, 2)*math.Pi)
		f.GyroDPS[axis] = rate
		f.Tilt[axis] = 20 * math.Sin(math.Mod(t, 2)*math.Pi)
	}

	// Digital trigger bits follow analog travel, as on real controllers.
	if f.Triggers[0] > 0.5 {
		if _, ok := g.Layout.Button(device.ButtonL2Digital); ok {
			f.Buttons = f.Buttons.With(device.ButtonL2Digital)
		}
	}
	if f.Triggers[1] > 0.5 {
		if _, ok := g.Layout.Button(device.ButtonR2Digital); ok {
			f.Buttons = f.Buttons.With(device.ButtonR2Digital)
		}
	}
	return f
}

func (g *Generator) touchAt(id int, t float64) device.TouchPoint {
	w, h := float64(g.Layout.TouchpadSize[0]), float64(g.Layout.TouchpadSize[1])
	return device.TouchPoint{
		Active: true,
		ID:     uint8(10 + id),
		X:      uint16(w/2 + w*0.34*math.Sin(t*1.2+float64(id))),
		Y:      uint16(h/2 + h*0.31*math.Cos(t*1.6+float64(id))),
	}
}

func clampAxis(a int) int {
	if a < 0 || a > 2 {
		return 1
	}
	return a
}

// Raw converts a frame to raw device units using the layout ranges, adding the
// profile's noise. Gyro/accel use the layout's nominal scales (or 16.4 LSB/°/s
// and 8192 LSB/g when unknown) — they are synthetic anyway.
func (g *Generator) Raw(f Frame) device.RawInput {
	r := device.RawInput{Valid: true}
	sr := g.Layout.StickRange
	for i := 0; i < 4; i++ {
		v := sr.Center + f.Sticks[i]*sr.HalfSpan() + g.rng.NormFloat64()*g.Profile.StickNoise
		r.Sticks[i] = int32(math.Round(clamp(v, sr.Min, sr.Max)))
	}
	tr := g.Layout.TriggerRange
	for i := 0; i < 2; i++ {
		v := tr.Min + f.Triggers[i]*tr.Span()
		r.Triggers[i] = int32(math.Round(clamp(v, tr.Min, tr.Max)))
	}
	gs := g.Layout.GyroLSBPerDPS
	if gs == 0 {
		gs = 16.4
	}
	as := g.Layout.AccelLSBPerG
	if as == 0 {
		as = 8192
	}
	for i := 0; i < 3; i++ {
		r.Gyro[i] = int16(clamp(f.GyroDPS[i]*gs+g.Profile.GyroBias[i]+g.rng.NormFloat64()*g.Profile.GyroNoise, -32768, 32767))
		r.Accel[i] = int16(clamp(f.AccelG[i]*as+g.rng.NormFloat64()*g.Profile.GyroNoise, -32768, 32767))
	}
	return r
}

// Normalize converts raw stick counts back to [-1,1] (no deadzone), so the
// normalized telemetry is consistent with the simulated raw values.
func Normalize(raw int32, r device.AxisRange) float64 {
	return clamp((float64(raw)-r.Center)/r.HalfSpan(), -1, 1)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
