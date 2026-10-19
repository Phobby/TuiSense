package diag

import (
	"fmt"
	"math"
	"time"

	"github.com/ygzztrk/tuisense/internal/device"
)

// StickPhase is a step of the guided drift test.
type StickPhase int

const (
	StickIdle       StickPhase = iota
	StickStationary            // sticks released: measure resting center & noise
	StickRange                 // sticks rotated: measure reach in 8 directions
	StickRecenter              // released again: verify return to center
	StickDone
)

// StickLive is the real-time view of one stick.
type StickLive struct {
	Raw      [2]float64 // raw counts (X, Y)
	Norm     [2]float64 // normalized [-1,1], no deadzone applied
	Min, Max [2]float64 // raw extremes since reset
	jitter   [2]*Ring   // last ~1 s of raw values
	Trail    [][2]float64
}

// Jitter returns the rolling standard deviation (raw counts) of X and Y.
func (l *StickLive) Jitter() [2]float64 {
	return [2]float64{l.jitter[0].Std(), l.jitter[1].Std()}
}

// StickStats summarizes one stick over a measurement phase.
type StickStats struct {
	X, Y   AxisStats
	MaxDev float64 // largest distance from the nominal center, raw counts
}

// RangeStats tracks reach per 45° direction during the range phase.
type RangeStats struct {
	Octant [8]float64 // max normalized radius reached per direction
	Peak   float64
}

// StickAnalysis is the interpreted result for one stick.
type StickAnalysis struct {
	Status      Status
	Summary     string
	Explanation []string
	OffsetRaw   float64 // resting offset from nominal center, raw counts
	OffsetPct   float64 // as % of half travel
	NoiseRaw    float64 // standard deviation, raw counts
	PeakToPeak  float64
	MaxDevPct   float64
	Samples     int
	ReachedDirs int     // range phase: directions reaching ≥90%
	RecenterRaw float64 // change of resting center between measurements, raw counts
}

// StickTest is the guided drift / range test for all declared sticks.
type StickTest struct {
	Layout   device.InputLayout
	Duration time.Duration // stationary sampling period (configurable)
	Deadzone [2]float64    // TuiSense's configured inner deadzone in % (read-only context)

	Phase      StickPhase
	PhaseStart time.Time
	PhaseEnd   time.Time

	Live       [2]StickLive
	Stationary [2]StickStats
	Recenter   [2]StickStats
	Range      [2]RangeStats
	Analysis   [2]StickAnalysis
	History    [2]*Ring // offset magnitude (%) history for graphs

	scenario func(device.InputScenario)
}

// Durations offered in the UI.
var StickDurations = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

const (
	rangeDuration    = 6 * time.Second
	recenterDuration = 3 * time.Second
	trailLen         = 40
	// settleTime is ignored at the start of resting phases: a released stick
	// needs a moment to spring back to center.
	settleTime = 400 * time.Millisecond
)

// NewStickTest creates the test for a layout.
func NewStickTest(layout device.InputLayout, scenario func(device.InputScenario)) *StickTest {
	t := &StickTest{Layout: layout, Duration: 5 * time.Second, scenario: scenario}
	for i := range t.Live {
		t.Live[i].jitter = [2]*Ring{NewRing(25), NewRing(25)}
		t.History[i] = NewRing(60)
	}
	t.ResetLive()
	return t
}

// ResetLive clears min/max/trails of the live view.
func (t *StickTest) ResetLive() {
	for i := range t.Live {
		l := &t.Live[i]
		l.Min = [2]float64{math.Inf(1), math.Inf(1)}
		l.Max = [2]float64{math.Inf(-1), math.Inf(-1)}
		l.Trail = nil
		l.jitter[0].Reset()
		l.jitter[1].Reset()
	}
}

func (t *StickTest) setScenario(s device.InputScenario) {
	if t.scenario != nil {
		t.scenario(s)
	}
}

// rawSticks returns raw counts, reconstructing from normalized values when the
// backend provides no raw data.
func rawSticks(tel device.Telemetry, r device.AxisRange) [4]float64 {
	if tel.Raw.Valid {
		return [4]float64{float64(tel.Raw.Sticks[0]), float64(tel.Raw.Sticks[1]), float64(tel.Raw.Sticks[2]), float64(tel.Raw.Sticks[3])}
	}
	n := [4]float64{tel.LeftStickX, tel.LeftStickY, tel.RightStickX, tel.RightStickY}
	for i := range n {
		n[i] = r.Center + n[i]*r.HalfSpan()
	}
	return n
}

// Feed processes one telemetry sample.
func (t *StickTest) Feed(tel device.Telemetry, now time.Time) {
	r := t.Layout.StickRange
	if r.HalfSpan() == 0 {
		return
	}
	raw := rawSticks(tel, r)
	for i := 0; i < 2; i++ {
		x, y := raw[2*i], raw[2*i+1]
		l := &t.Live[i]
		l.Raw = [2]float64{x, y}
		l.Norm = [2]float64{(x - r.Center) / r.HalfSpan(), (y - r.Center) / r.HalfSpan()}
		for a, v := range l.Raw {
			l.Min[a] = math.Min(l.Min[a], v)
			l.Max[a] = math.Max(l.Max[a], v)
			l.jitter[a].Push(v)
		}
		l.Trail = append(l.Trail, l.Norm)
		if len(l.Trail) > trailLen {
			l.Trail = l.Trail[1:]
		}
		t.History[i].Push(100 * math.Hypot(l.Norm[0], l.Norm[1]))

		dev := math.Hypot(x-r.Center, y-r.Center)
		settling := now.Sub(t.PhaseStart) < settleTime
		switch t.Phase {
		case StickStationary:
			if !settling {
				addStick(&t.Stationary[i], x, y, dev)
			}
		case StickRecenter:
			if !settling {
				addStick(&t.Recenter[i], x, y, dev)
			}
		case StickRange:
			rad := math.Hypot(l.Norm[0], l.Norm[1])
			oct := int(math.Mod(math.Atan2(l.Norm[1], l.Norm[0])+2*math.Pi+math.Pi/8, 2*math.Pi) / (math.Pi / 4))
			rs := &t.Range[i]
			rs.Octant[oct%8] = math.Max(rs.Octant[oct%8], rad)
			rs.Peak = math.Max(rs.Peak, rad)
		}
	}
	t.Tick(now)
}

func addStick(s *StickStats, x, y, dev float64) {
	s.X.Add(x)
	s.Y.Add(y)
	s.MaxDev = math.Max(s.MaxDev, dev)
}

// Tick advances timed phases.
func (t *StickTest) Tick(now time.Time) {
	if t.Phase == StickIdle || t.Phase == StickDone || now.Before(t.PhaseEnd) {
		return
	}
	switch t.Phase {
	case StickStationary:
		for i := range t.Analysis {
			t.Analysis[i] = t.analyzeStationary(i, t.Stationary[i])
		}
		t.Phase = StickDone
		t.setScenario(device.ScenarioDemo)
	case StickRange:
		for i := range t.Analysis {
			t.Analysis[i].ReachedDirs = reached(t.Range[i])
		}
		t.begin(StickRecenter, now, recenterDuration)
		t.setScenario(device.ScenarioIdle)
	case StickRecenter:
		for i := range t.Analysis {
			t.Analysis[i] = t.analyzeRecenter(i)
		}
		t.Phase = StickDone
		t.setScenario(device.ScenarioDemo)
	}
}

func (t *StickTest) begin(p StickPhase, now time.Time, d time.Duration) {
	t.Phase, t.PhaseStart, t.PhaseEnd = p, now, now.Add(d)
}

// StartStationary begins step 1 (user has released both sticks).
func (t *StickTest) StartStationary(now time.Time) {
	t.Stationary = [2]StickStats{}
	t.Recenter = [2]StickStats{}
	t.Range = [2]RangeStats{}
	t.Analysis = [2]StickAnalysis{}
	t.begin(StickStationary, now, t.Duration)
	t.setScenario(device.ScenarioIdle)
}

// StartRange begins the optional movement step (after a stationary run).
func (t *StickTest) StartRange(now time.Time) {
	t.Range = [2]RangeStats{}
	t.Recenter = [2]StickStats{}
	t.begin(StickRange, now, rangeDuration)
	t.setScenario(device.ScenarioMove)
}

// Cancel aborts any running phase without producing results.
func (t *StickTest) Cancel() {
	if t.Phase != StickIdle && t.Phase != StickDone {
		t.Phase = StickIdle
		t.setScenario(device.ScenarioDemo)
	}
}

// Running reports whether a measurement phase is active.
func (t *StickTest) Running() bool {
	return t.Phase == StickStationary || t.Phase == StickRange || t.Phase == StickRecenter
}

// Progress returns 0..1 of the current phase.
func (t *StickTest) Progress(now time.Time) float64 {
	total := t.PhaseEnd.Sub(t.PhaseStart)
	if total <= 0 {
		return 0
	}
	return math.Min(1, float64(now.Sub(t.PhaseStart))/float64(total))
}

func reached(rs RangeStats) int {
	n := 0
	for _, r := range rs.Octant {
		if r >= 0.9 {
			n++
		}
	}
	return n
}

func (t *StickTest) tolerances() (center, noise float64) {
	hs := t.Layout.StickRange.HalfSpan()
	center, noise = t.Layout.Drift.CenterTolerance, t.Layout.Drift.NoiseTolerance
	if center <= 0 {
		center = 0.02 * hs
	}
	if noise <= 0 {
		noise = 0.01 * hs
	}
	return center, noise
}

func (t *StickTest) analyzeStationary(i int, st StickStats) StickAnalysis {
	r := t.Layout.StickRange
	hs := r.HalfSpan()
	tolC, tolN := t.tolerances()
	name := t.Layout.StickLabels[i]
	a := StickAnalysis{Samples: st.X.N}
	dx, dy := st.X.Mean()-r.Center, st.Y.Mean()-r.Center
	a.OffsetRaw = math.Hypot(dx, dy)
	a.OffsetPct = 100 * a.OffsetRaw / hs
	a.NoiseRaw = math.Max(st.X.Std(), st.Y.Std())
	a.PeakToPeak = math.Max(st.X.PeakToPeak(), st.Y.PeakToPeak())
	a.MaxDevPct = 100 * st.MaxDev / hs
	dzRaw := t.Deadzone[i] / 100 * hs
	moveLimit := math.Max(8*tolN, 0.08*hs)

	switch {
	case a.Samples < 10:
		a.Status, a.Summary = Inconclusive, "Too few samples — keep the controller connected and retry."
	case a.PeakToPeak > moveLimit:
		a.Status = Inconclusive
		a.Summary = fmt.Sprintf("%s moved during the measurement (range %.0f counts).", name, a.PeakToPeak)
		a.Explanation = append(a.Explanation, "Release the stick completely and keep the controller still, then retest.")
	case a.OffsetRaw <= tolC && a.NoiseRaw <= tolN:
		a.Status = Passed
		a.Summary = fmt.Sprintf("%s rests %.1f%% from center with low noise — no sign of drift at rest.", name, a.OffsetPct)
		a.Explanation = append(a.Explanation, "Small constant offsets like this are normal manufacturing/calibration differences.")
	case a.OffsetRaw <= tolC:
		a.Status = Warning
		a.Summary = fmt.Sprintf("%s is centered but noisy (σ %.1f counts).", name, a.NoiseRaw)
		a.Explanation = append(a.Explanation, "Jitter without a center offset usually indicates electrical noise or a worn potentiometer track, not drift.")
	case dzRaw > 0 && a.OffsetRaw < dzRaw:
		a.Status = Warning
		a.Summary = fmt.Sprintf("%s rests %.1f%% off-center — above typical tolerance but hidden by your %.0f%% deadzone.", name, a.OffsetPct, t.Deadzone[i])
		a.Explanation = append(a.Explanation,
			"This can be a calibration difference or early drift. Run the movement step to see whether the center is stable.")
	default:
		a.Status = Warning
		a.Summary = fmt.Sprintf("Possible drift: %s rests %.1f%% off-center, beyond your %.0f%% deadzone.", name, a.OffsetPct, t.Deadzone[i])
		a.Explanation = append(a.Explanation,
			"Games using a similar or smaller deadzone may register movement while the stick is released.",
			"Run the movement step: an offset that persists (or changes) after moving the stick strengthens the drift indication.")
	}
	return a
}

func (t *StickTest) analyzeRecenter(i int) StickAnalysis {
	first := t.analyzeStationary(i, t.Stationary[i])
	second := t.analyzeStationary(i, t.Recenter[i])
	r := t.Layout.StickRange
	tolC, _ := t.tolerances()
	a := first
	a.ReachedDirs = reached(t.Range[i])
	a.RecenterRaw = math.Hypot(t.Recenter[i].X.Mean()-t.Stationary[i].X.Mean(), t.Recenter[i].Y.Mean()-t.Stationary[i].Y.Mean())
	name := t.Layout.StickLabels[i]
	dzRaw := t.Deadzone[i] / 100 * r.HalfSpan()
	threshold := math.Max(tolC, dzRaw)

	switch {
	case first.Status == Inconclusive || second.Status == Inconclusive:
		a.Status = Inconclusive
		a.Summary = name + ": a measurement was disturbed by movement — retest."
	case first.OffsetRaw > threshold && second.OffsetRaw > threshold:
		a.Status = Failed
		a.Summary = fmt.Sprintf("Drift confirmed: %s rested %.1f%% and %.1f%% off-center in two measurements.", name, first.OffsetPct, second.OffsetPct)
		a.Explanation = append(a.Explanation, "Both measurements exceed the declared tolerance and your deadzone.")
	case a.RecenterRaw > tolC:
		a.Status = worst(first.Status, Warning)
		a.Summary = fmt.Sprintf("%s does not return to the same center (shifted %.0f counts after movement).", name, a.RecenterRaw)
		a.Explanation = append(a.Explanation, "An inconsistent return to center is typical of worn sticks and can cause intermittent drift.")
	default:
		a.Status = worst(first.Status, second.Status)
		a.Explanation = append(a.Explanation, fmt.Sprintf("Center is stable after movement (shift %.0f counts).", a.RecenterRaw))
	}
	if a.ReachedDirs < 8 {
		a.Explanation = append(a.Explanation, fmt.Sprintf("Full travel reached in %d/8 directions (move the stick to the rim; a low count can also mean it was not pushed fully).", a.ReachedDirs))
	} else {
		a.Explanation = append(a.Explanation, "Full travel reached in all 8 directions.")
	}
	return a
}

// Result combines both sticks into a category result (nil before a run).
func (t *StickTest) Result(now time.Time, simulated bool) *Result {
	if t.Phase != StickDone {
		return nil
	}
	res := &Result{Category: CatSticks, Simulated: simulated, At: now}
	var st []Status
	for i, a := range t.Analysis {
		st = append(st, a.Status)
		res.Details = append(res.Details, a.Summary)
		res.Details = append(res.Details, a.Explanation...)
		lbl := t.Layout.StickLabels[i]
		res.Measurements = append(res.Measurements,
			Measurement{lbl + " resting offset", a.OffsetPct, "% of half travel"},
			Measurement{lbl + " resting offset (raw)", a.OffsetRaw, "counts"},
			Measurement{lbl + " noise σ", a.NoiseRaw, "counts"},
			Measurement{lbl + " peak-to-peak", a.PeakToPeak, "counts"},
			Measurement{lbl + " max deviation", a.MaxDevPct, "%"},
			Measurement{lbl + " samples", float64(a.Samples), ""},
		)
		if t.Range[i].Peak > 0 {
			res.Measurements = append(res.Measurements, Measurement{lbl + " directions at full travel", float64(a.ReachedDirs), "of 8"})
		}
	}
	res.Status = worst(st...)
	switch res.Status {
	case Passed:
		res.Summary = "No signs of stick drift at rest."
	case Failed:
		res.Summary = "Stick drift confirmed by repeated measurements."
	case Warning:
		res.Summary = "Stick findings need attention — see details."
	default:
		res.Summary = "Measurement inconclusive — retest with sticks released."
	}
	res.Limitations = []string{
		"Samples are taken at the host poll rate (~25 Hz) using the newest report per poll; the first 0.4 s of each resting step is ignored while the stick settles.",
		"Thresholds are interpretation heuristics, not manufacturer specifications: " + t.Layout.Drift.Rationale,
		fmt.Sprintf("Deadzones shown (L %.0f%%, R %.0f%%) are TuiSense's configured values; each game applies its own.", t.Deadzone[0], t.Deadzone[1]),
		"Stick calibration and deadzones are never read from or written to the controller by this test.",
	}
	return res
}
