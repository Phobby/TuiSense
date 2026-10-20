package diag

import (
	"math"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
)

// inputSamples returns the raw samples carried by one telemetry update.
//
// Real backends attach every report received since the previous poll
// (tel.Samples, possibly empty when nothing new arrived). An empty slice
// yields no samples, so a controller that has not reported yet can never
// contribute fabricated "perfectly centered" readings. Backends without
// per-report data (simulated devices) contribute their newest raw values, or
// values reconstructed from the normalized telemetry.
func inputSamples(tel device.Telemetry, l device.InputLayout, now time.Time) []device.RawSample {
	if tel.Samples != nil {
		return tel.Samples
	}
	if tel.Raw.Valid {
		return []device.RawSample{{At: now, Sticks: tel.Raw.Sticks, Triggers: tel.Raw.Triggers, Buttons: tel.Buttons}}
	}
	s := device.RawSample{At: now, Buttons: tel.Buttons}
	n := [4]float64{tel.LeftStickX, tel.LeftStickY, tel.RightStickX, tel.RightStickY}
	for i := range n {
		s.Sticks[i] = int32(math.Round(denormBipolar(l.StickAxis(i), n[i])))
	}
	tp := [2]float64{tel.L2Pressure, tel.R2Pressure}
	for i := range tp {
		s.Triggers[i] = int32(math.Round(denormUnipolar(l.TriggerAxis(i), tp[i])))
	}
	return []device.RawSample{s}
}

// denormBipolar is the inverse of AxisRange.NormalizeBipolar.
func denormBipolar(r device.AxisRange, v float64) float64 {
	if r.Invert {
		v = -v
	}
	c := r.Center
	if c < r.Min || c > r.Max {
		c = (r.Min + r.Max) / 2
	}
	if v >= 0 {
		return c + v*(r.Max-c)
	}
	return c + v*(c-r.Min)
}

// denormUnipolar is the inverse of AxisRange.NormalizeUnipolar.
func denormUnipolar(r device.AxisRange, v float64) float64 {
	if r.Invert {
		v = 1 - v
	}
	return r.Min + v*r.Span()
}

// rateCounter measures how many fresh (non-repeated) samples arrive per second.
type rateCounter struct {
	first, last time.Time
	n           int
}

func (r *rateCounter) add(s device.RawSample) {
	if s.Held {
		return
	}
	if r.n == 0 {
		r.first = s.At
	}
	r.last = s.At
	r.n++
}

// Hz returns the observed fresh-sample rate (0 when unknown).
func (r rateCounter) Hz() float64 {
	dt := r.last.Sub(r.first).Seconds()
	if r.n < 2 || dt <= 0 {
		return 0
	}
	return float64(r.n-1) / dt
}
