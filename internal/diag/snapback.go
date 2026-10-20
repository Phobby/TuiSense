package diag

import (
	"math"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
)

// Snapback is the stick's spring overshooting the center when the stick is
// released suddenly from the edge: for a few milliseconds the reading swings
// to the opposite side, which a game can read as a brief opposite input.

const (
	snapEdge      = 0.90                   // the stick is "at the edge" from 90% of travel
	snapCenter    = 0.25                   // a release has reached the center region
	snapMaxFall   = 120 * time.Millisecond // edge → center faster than this = a sudden release
	snapWindow    = 300 * time.Millisecond // overshoot and settling are observed this long
	snapTail      = 60 * time.Millisecond  // the window's tail gives the resting point
	snapBand      = 0.03                   // settled once within 3% of the resting point
	snapMinEvents = 3
	snapMinRateHz = 100 // below this, short overshoots can be missed
	snapTimeout   = 60 * time.Second
	// SnapbackTarget is the number of sudden releases requested per stick.
	SnapbackTarget = 5
)

// SnapPoint is one sample of a release trace.
type SnapPoint struct {
	At  time.Time
	Pos [2]float64 // normalized position
}

// SnapEvent is one measured sudden release.
type SnapEvent struct {
	At           time.Time  // last sample at the edge (release start)
	Dir          [2]float64 // unit push direction
	OvershootPct float64    // largest swing past the resting point, opposite to the push
	SettleMs     float64    // time from release until the reading stays at the resting point
	Crossings    int        // times the reading crossed the resting point
	Samples      int        // fresh samples inside the window
	Trace        []SnapPoint
}

// Projection returns the trace projected onto the push direction (positive =
// push side), relative to the resting point, for graphs.
func (e SnapEvent) Projection() []float64 {
	rest := e.rest()
	out := make([]float64, len(e.Trace))
	for i, p := range e.Trace {
		out[i] = (p.Pos[0]-rest[0])*e.Dir[0] + (p.Pos[1]-rest[1])*e.Dir[1]
	}
	return out
}

func (e SnapEvent) rest() [2]float64 {
	if len(e.Trace) == 0 {
		return [2]float64{}
	}
	end := e.Trace[len(e.Trace)-1].At
	var sum [2]float64
	n := 0
	for _, p := range e.Trace {
		if end.Sub(p.At) <= snapTail {
			sum[0] += p.Pos[0]
			sum[1] += p.Pos[1]
			n++
		}
	}
	return [2]float64{sum[0] / float64(n), sum[1] / float64(n)}
}

// finish computes the event's measurements from its trace.
func (e *SnapEvent) finish() {
	rest := e.rest()
	prev := 1.0
	var lastOut time.Time
	for _, p := range e.Trace {
		d := [2]float64{p.Pos[0] - rest[0], p.Pos[1] - rest[1]}
		proj := d[0]*e.Dir[0] + d[1]*e.Dir[1]
		e.OvershootPct = math.Max(e.OvershootPct, -100*proj)
		if (proj < -0.01 && prev > 0.01) || (proj > 0.01 && prev < -0.01) {
			e.Crossings++
		}
		if math.Abs(proj) > 0.01 {
			prev = proj
		}
		if math.Hypot(d[0], d[1]) > snapBand {
			lastOut = p.At
		}
	}
	if !lastOut.IsZero() {
		e.SettleMs = float64(lastOut.Sub(e.At).Microseconds()) / 1000
	}
}

type snapTracker struct {
	atEdge  bool
	edgeAt  time.Time
	edgeDir [2]float64
	active  *SnapEvent
}

// SnapbackAnalysis is the interpreted result for one stick.
type SnapbackAnalysis struct {
	Verdict          Verdict
	Headline         string
	Explanation      []string
	Advice           []string
	Events           int
	SlowReleases     int
	MaxOvershootPct  float64
	MeanOvershootPct float64
	MeanSettleMs     float64
	MaxSettleMs      float64
	OverDeadzone     int
	RateHz           float64
}

// Status returns the health status of the analysis.
func (a SnapbackAnalysis) Status() Status { return a.Verdict.Status() }

// SnapbackTest detects sudden releases of either stick and measures how far
// the reading swings past the resting point and how long it takes to settle.
type SnapbackTest struct {
	Layout          device.InputLayout
	PreviewDeadzone float64

	Live    [2][2]float64
	Events  [2][]SnapEvent
	Slow    [2]int
	running bool
	done    bool
	start   time.Time
	until   time.Time
	track   [2]snapTracker

	scenario func(device.InputScenario)
}

// NewSnapbackTest creates the test for a layout.
func NewSnapbackTest(layout device.InputLayout, scenario func(device.InputScenario)) *SnapbackTest {
	return &SnapbackTest{Layout: layout, PreviewDeadzone: DefaultPreviewDeadzone, scenario: scenario}
}

func (t *SnapbackTest) setScenario(s device.InputScenario) {
	if t.scenario != nil {
		t.scenario(s)
	}
}

// Start begins collecting releases (explicit user action).
func (t *SnapbackTest) Start(now time.Time) {
	t.Events = [2][]SnapEvent{}
	t.Slow = [2]int{}
	t.track = [2]snapTracker{}
	t.running, t.done = true, false
	t.start, t.until = now, now.Add(snapTimeout)
	t.setScenario(device.ScenarioSnapback)
}

// Finish ends collection and keeps the measured releases.
func (t *SnapbackTest) Finish() {
	if !t.running {
		return
	}
	t.running, t.done = false, true
	t.setScenario(device.ScenarioDemo)
}

// Cancel stops collecting. Releases measured so far are kept as the result
// (collecting drives no hardware, so there is nothing to undo).
func (t *SnapbackTest) Cancel() {
	if !t.running {
		return
	}
	if len(t.Events[0])+len(t.Events[1]) > 0 {
		t.Finish()
		return
	}
	t.running = false
	t.setScenario(device.ScenarioDemo)
}

// Running reports whether releases are being collected.
func (t *SnapbackTest) Running() bool { return t.running }

// Done reports whether a finished measurement exists.
func (t *SnapbackTest) Done() bool { return t.done }

// Progress returns 0..1 based on the requested releases.
func (t *SnapbackTest) Progress() float64 {
	n := min(len(t.Events[0]), SnapbackTarget) + min(len(t.Events[1]), SnapbackTarget)
	return float64(n) / (2 * SnapbackTarget)
}

// Feed processes one telemetry update; every report is examined.
func (t *SnapbackTest) Feed(tel device.Telemetry, now time.Time) {
	samples := inputSamples(tel, t.Layout, now)
	for _, s := range samples {
		t.feedSample(s)
	}
	at := now
	if len(samples) > 0 {
		at = samples[len(samples)-1].At
	}
	t.Tick(at)
}

// Tick ends the collection at the target count or the time limit.
func (t *SnapbackTest) Tick(now time.Time) {
	if !t.running {
		return
	}
	if (len(t.Events[0]) >= SnapbackTarget && len(t.Events[1]) >= SnapbackTarget) || !now.Before(t.until) {
		t.Finish()
	}
}

func (t *SnapbackTest) feedSample(s device.RawSample) {
	for i := 0; i < 2; i++ {
		n := [2]float64{
			t.Layout.StickAxis(2 * i).NormalizeBipolar(float64(s.Sticks[2*i])),
			t.Layout.StickAxis(2*i + 1).NormalizeBipolar(float64(s.Sticks[2*i+1])),
		}
		t.Live[i] = n
		if !t.running {
			continue
		}
		tr := &t.track[i]
		r := math.Hypot(n[0], n[1])
		if ev := tr.active; ev != nil {
			if s.At.Sub(ev.At) <= snapWindow && r < snapEdge {
				ev.Trace = append(ev.Trace, SnapPoint{At: s.At, Pos: n})
				if !s.Held {
					ev.Samples++
				}
				continue
			}
			ev.finish()
			t.Events[i] = append(t.Events[i], *ev)
			tr.active = nil
		}
		switch {
		case r >= snapEdge:
			tr.atEdge, tr.edgeAt, tr.edgeDir = true, s.At, [2]float64{n[0] / r, n[1] / r}
		case tr.atEdge && r < snapCenter:
			tr.atEdge = false
			if s.At.Sub(tr.edgeAt) <= snapMaxFall {
				tr.active = &SnapEvent{At: tr.edgeAt, Dir: tr.edgeDir, Trace: []SnapPoint{{At: s.At, Pos: n}}, Samples: 1}
			} else {
				t.Slow[i]++ // guided back by hand: not a snapback measurement
			}
		}
	}
}

// Analyze interprets the releases of stick i with the current deadzone preview.
func (t *SnapbackTest) Analyze(i int) SnapbackAnalysis {
	evs := t.Events[i]
	name := inSentence(i18n.T(t.Layout.StickLabels[i]))
	a := SnapbackAnalysis{Events: len(evs), SlowReleases: t.Slow[i]}
	if len(evs) == 0 {
		a.Headline = i18n.Tf("No sudden releases of the %s were detected.", name)
		if t.Slow[i] > 0 {
			a.Advice = append(a.Advice, i18n.T("Let go of the stick instantly at the edge instead of guiding it back to the center."))
		}
		return a
	}
	dz := t.PreviewDeadzone
	var sumO, sumS, sumRate float64
	for _, e := range evs {
		sumO += e.OvershootPct
		sumS += e.SettleMs
		sumRate += float64(e.Samples) / snapWindow.Seconds()
		a.MaxOvershootPct = math.Max(a.MaxOvershootPct, e.OvershootPct)
		a.MaxSettleMs = math.Max(a.MaxSettleMs, e.SettleMs)
		if e.OvershootPct > dz {
			a.OverDeadzone++
		}
	}
	n := float64(len(evs))
	a.MeanOvershootPct, a.MeanSettleMs, a.RateHz = sumO/n, sumS/n, sumRate/n
	lowRate := a.RateHz < snapMinRateHz

	switch {
	case len(evs) < snapMinEvents:
		a.Verdict = VerdictInconclusive
		a.Headline = i18n.Tf("Test incomplete: only %d sudden release(s) of the %s were detected; at least %d are needed.", len(evs), name, snapMinEvents)
		a.Advice = append(a.Advice, i18n.T("Push the stick fully to the edge and let go instantly, several times in different directions."))
	case a.OverDeadzone > 0:
		a.Verdict = VerdictSnapback
		a.Headline = i18n.Tf("Your %s snaps back: after a sudden release it swings up to %.1f%% to the opposite side (%d of %d releases go past the %.0f%% deadzone preview).", name, a.MaxOvershootPct, a.OverDeadzone, len(evs), dz)
		a.Explanation = append(a.Explanation, i18n.T("A game using that deadzone can read this as a brief input in the opposite direction — for example the character glancing the other way for a moment."))
		a.Advice = append(a.Advice,
			i18n.T("Let the stick glide back instead of flicking it off, or use a slightly larger deadzone or the game's anti-snapback option if it has one."),
			i18n.T("Overshoot that grows over weeks can point to a worn centering spring."))
	case lowRate:
		a.Verdict = VerdictInconclusive
		a.Headline = i18n.Tf("The %s overshot at most %.1f%%, but the controller reported only %.0f samples/s, so a short overshoot may have been missed.", name, a.MaxOvershootPct, a.RateHz)
	default:
		a.Verdict = VerdictNormal
		a.Headline = i18n.Tf("Your %s settles cleanly: after a sudden release it overshoots at most %.1f%%, inside the %.0f%% deadzone preview.", name, a.MaxOvershootPct, dz)
	}
	if len(evs) >= snapMinEvents {
		a.Explanation = append(a.Explanation, i18n.Tf("On average it overshoots %.1f%% and settles within %.0f ms.", a.MeanOvershootPct, a.MeanSettleMs))
		if lowRate && a.Verdict != VerdictInconclusive {
			a.Explanation = append(a.Explanation, i18n.Tf("Only %.0f samples/s were reported; the real overshoot may be larger.", a.RateHz))
		}
	}
	a.Explanation = append(a.Explanation, i18n.T("Snapback is the stick's spring overshooting the center when it is released quickly. Many controllers do this; on its own it is not a defect."))
	return a
}

// Result returns the category result once the collection has finished.
func (t *SnapbackTest) Result(now time.Time, simulated bool) *Result {
	if !t.done {
		return nil
	}
	res := &Result{Category: CatSticks, Simulated: simulated, At: now}
	var st []Status
	var snap []string
	for i := 0; i < 2; i++ {
		a := t.Analyze(i)
		lbl := i18n.T(t.Layout.StickLabels[i])
		res.Details = append(res.Details, a.Headline)
		res.Details = append(res.Details, a.Explanation...)
		res.Details = append(res.Details, a.Advice...)
		if a.Events == 0 {
			continue
		}
		st = append(st, a.Status())
		if a.Verdict == VerdictSnapback {
			snap = append(snap, lbl)
		}
		res.Measurements = append(res.Measurements,
			Measurement{i18n.Tf("%s sudden releases", lbl), float64(a.Events), ""},
			Measurement{i18n.Tf("%s maximum overshoot", lbl), a.MaxOvershootPct, i18n.T("% of travel")},
			Measurement{i18n.Tf("%s average overshoot", lbl), a.MeanOvershootPct, i18n.T("% of travel")},
			Measurement{i18n.Tf("%s average settle time", lbl), a.MeanSettleMs, "ms"},
			Measurement{i18n.Tf("%s releases past the deadzone preview", lbl), float64(a.OverDeadzone), ""},
			Measurement{i18n.Tf("%s sample rate during releases", lbl), a.RateHz, i18n.T("Hz")},
		)
	}
	res.Status = worst(st...)
	switch {
	case len(st) == 0:
		res.Status = Inconclusive
		res.Summary = i18n.T("No sudden releases were detected — push a stick to the edge and let go instantly.")
	case len(snap) > 0:
		res.Summary = i18n.Tf("Noticeable snapback: %s. Common on many controllers; not a confirmed defect.", strings.Join(snap, ", "))
	case res.Status == Inconclusive:
		res.Summary = i18n.T("Inconclusive — more sudden releases are needed.")
	default:
		res.Summary = i18n.T("The sticks settle cleanly after sudden releases.")
	}
	res.Limitations = []string{
		i18n.Tf("Overshoot is compared with the %.0f%% deadzone preview; each game uses its own deadzone and some filter snapback.", t.PreviewDeadzone),
		i18n.T("Releases guided back by hand are ignored; only edge-to-center returns faster than 120 ms are measured."),
	}
	return res
}
