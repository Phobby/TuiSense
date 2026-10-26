package diag

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
)

// StickStep is a step of the guided stick test.
type StickStep int

const (
	StickIdle    StickStep = iota
	StickRelease           // 1: sticks released — measure the resting position
	StickExplain           // 2: resting result explained; waiting for the user
	StickUp                // 3: movement range, one direction at a time…
	StickDown
	StickLeft
	StickRight
	StickRotate   // …then around the rim
	StickRelease2 // 3 (end): released again — same resting point?
	StickDone     // 4: final result
)

// Verdict is the human-level conclusion for one stick or trigger. Inputs are
// never declared "failed": a measurement alone cannot prove a hardware defect.
type Verdict string

const (
	VerdictNone           Verdict = ""
	VerdictNormal         Verdict = "normal"
	VerdictPossibleDrift  Verdict = "possible_drift"
	VerdictFurtherTesting Verdict = "further_testing"
	VerdictInconclusive   Verdict = "inconclusive"
	VerdictPossibleIssue  Verdict = "possible_issue" // triggers: e.g. not returning to rest
	VerdictSnapback       Verdict = "snapback"       // sticks: overshoot past the deadzone after a sudden release
)

// Label returns the verdict in the active UI language.
func (v Verdict) Label() string {
	switch v {
	case VerdictNormal:
		return i18n.T("Normal — within tolerance")
	case VerdictPossibleDrift:
		return i18n.T("Possible drift")
	case VerdictFurtherTesting:
		return i18n.T("Further testing recommended")
	case VerdictInconclusive:
		return i18n.T("Inconclusive")
	case VerdictPossibleIssue:
		return i18n.T("Possible issue")
	case VerdictSnapback:
		return i18n.T("Noticeable snapback")
	}
	return i18n.T("Not tested")
}

// Status maps the verdict to a health status.
func (v Verdict) Status() Status {
	switch v {
	case VerdictNormal:
		return Passed
	case VerdictPossibleDrift, VerdictFurtherTesting, VerdictPossibleIssue, VerdictSnapback:
		return Warning
	case VerdictInconclusive:
		return Inconclusive
	}
	return NotTested
}

// Movement directions in StickTest reach arrays.
const (
	DirUp = iota
	DirDown
	DirLeft
	DirRight
)

const (
	stickSectors   = 16
	reachThreshold = 0.85 // a direction counts as reached at 85% of travel
	rimRadius      = 0.70 // rotation coverage counts sectors reached at ≥70%
	rimSectorsGoal = 14   // of 16 sectors

	stickSettle      = 500 * time.Millisecond // ignored while a released stick springs back
	directionTimeout = 8 * time.Second
	rotateTimeout    = 12 * time.Second
	release2Duration = 2 * time.Second
	minRestSamples   = 10
	trailLen         = 40
	historyLen       = 120
)

// StickDurations are the resting-measurement lengths offered in Advanced mode.
var StickDurations = []time.Duration{2 * time.Second, 3 * time.Second, 5 * time.Second, 10 * time.Second}

// StickLive is the real-time view of one stick.
type StickLive struct {
	Valid    bool
	Raw      [2]float64 // raw counts (X, Y)
	Norm     [2]float64 // normalized [-1, 1] with the device's own range; no deadzone
	Min, Max [2]float64 // raw extremes since reset
	Trail    [][2]float64
	HistX    *Ring // normalized X per update, for time-series graphs
	HistY    *Ring
	jitter   [2]*Ring // normalized values of the most recent samples
}

// JitterPct returns the recent standard deviation of X and Y in % of travel.
func (l *StickLive) JitterPct() [2]float64 {
	return [2]float64{100 * l.jitter[0].Std(), 100 * l.jitter[1].Std()}
}

// restStats accumulates one released-stick measurement.
type restStats struct {
	X, Y       AxisStats // normalized
	RawX, RawY AxisStats
	Dev        AxisStats       // distance from nominal center per sample (normalized)
	Half       [2][2]AxisStats // [half][axis] normalized, for stability
	mid        time.Time
	rate       rateCounter
	deviations [4096]float64 // bounded window for the median
	deviationN int
}

func (r *restStats) add(s device.RawSample, raw, n [2]float64) {
	r.X.Add(n[0])
	r.Y.Add(n[1])
	r.RawX.Add(raw[0])
	r.RawY.Add(raw[1])
	dev := math.Hypot(n[0], n[1])
	r.Dev.Add(dev)
	r.deviations[r.deviationN%len(r.deviations)] = dev
	r.deviationN++
	h := 0
	if !s.At.Before(r.mid) {
		h = 1
	}
	r.Half[h][0].Add(n[0])
	r.Half[h][1].Add(n[1])
	r.rate.add(s)
}

// RestSummary is one resting measurement in human terms (all % of travel).
type RestSummary struct {
	N            int
	RateHz       float64
	Mean         [2]float64 // normalized mean position
	OffsetPct    float64    // distance of the mean from the nominal center
	OffsetRaw    float64    // the same in raw counts
	AvgDevPct    float64    // average distance of individual samples
	MedianDevPct float64
	MaxDevPct    float64
	NoisePct     float64 // larger of the X/Y standard deviations
	NoiseRaw     float64
	P2PPct       float64 // largest X/Y peak-to-peak
	StableDiff   float64 // shift of the mean between the first and second half
}

func summarizeRest(r restStats, l device.InputLayout, stick int) RestSummary {
	s := RestSummary{N: r.X.N, RateHz: r.rate.Hz()}
	if s.N == 0 {
		return s
	}
	s.Mean = [2]float64{r.X.Mean(), r.Y.Mean()}
	s.OffsetPct = 100 * math.Hypot(s.Mean[0], s.Mean[1])
	ax, ay := l.StickAxis(2*stick), l.StickAxis(2*stick+1)
	s.OffsetRaw = math.Hypot(r.RawX.Mean()-ax.Center, r.RawY.Mean()-ay.Center)
	s.AvgDevPct = 100 * r.Dev.Mean()
	s.MaxDevPct = 100 * r.Dev.Max
	count := min(r.deviationN, len(r.deviations))
	if count > 0 {
		values := append([]float64(nil), r.deviations[:count]...)
		sort.Float64s(values)
		s.MedianDevPct = 50 * (values[(count-1)/2] + values[count/2])
	}
	s.NoisePct = 100 * math.Max(r.X.Std(), r.Y.Std())
	s.NoiseRaw = math.Max(r.RawX.Std(), r.RawY.Std())
	s.P2PPct = 100 * math.Max(r.X.PeakToPeak(), r.Y.PeakToPeak())
	if r.Half[0][0].N > 0 && r.Half[1][0].N > 0 {
		s.StableDiff = 100 * math.Hypot(r.Half[1][0].Mean()-r.Half[0][0].Mean(), r.Half[1][1].Mean()-r.Half[0][1].Mean())
	}
	return s
}

// TravelStats tracks how far a stick moved during the movement steps.
type TravelStats struct {
	Reach  [4]float64            // max normalized reach up/down/left/right
	Sector [stickSectors]float64 // max radius per 22.5° sector (outer boundary)
}

// Coverage returns how many sectors reached the rim radius.
func (t TravelStats) Coverage() int {
	n := 0
	for _, r := range t.Sector {
		if r >= rimRadius {
			n++
		}
	}
	return n
}

// OuterRadiusPct returns the mean outer radius over covered sectors (100 = full circle).
func (t TravelStats) OuterRadiusPct() float64 {
	sum, n := 0.0, 0
	for _, r := range t.Sector {
		if r >= rimRadius {
			sum += r
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return 100 * sum / float64(n)
}

func (t *TravelStats) add(n [2]float64) {
	reach := [4]float64{-n[1], n[1], -n[0], n[0]}
	for d, v := range reach {
		t.Reach[d] = math.Max(t.Reach[d], v)
	}
	r := math.Hypot(n[0], n[1])
	if r < 0.05 {
		return
	}
	ang := math.Mod(math.Atan2(n[1], n[0])+2*math.Pi, 2*math.Pi)
	sec := int(ang/(2*math.Pi)*stickSectors) % stickSectors
	t.Sector[sec] = math.Max(t.Sector[sec], r)
}

// StickAnalysis is the interpreted result for one stick.
type StickAnalysis struct {
	Verdict     Verdict
	Headline    string   // one plain-language sentence
	Explanation []string // why, and what it means for gameplay
	Advice      []string // how to repeat or what to try
	RestA       RestSummary
	RestB       RestSummary // zero until the second resting measurement
	HasRestB    bool
	ShiftPct    float64 // difference between the two resting points
	Travel      TravelStats
	TravelDone  bool
	Missed      []int // directions not reached during the movement steps
}

// Status returns the health status of the analysis.
func (a StickAnalysis) Status() Status { return a.Verdict.Status() }

// StickTest is the guided stick test for all declared sticks.
type StickTest struct {
	Layout          device.InputLayout
	Duration        time.Duration // resting measurement length (after settling)
	CircularityOnly bool
	AxialPreview    bool
	PreviewDeadzone float64 // deadzone preview size in %, never applied to input
	SourceNote      string  // set by the session, e.g. event-driven input caveat

	Step      StickStep
	StepStart time.Time
	StepEnd   time.Time

	Live     [2]StickLive
	Rest     [2]restStats
	Rest2    [2]restStats
	Travel   [2]TravelStats
	Analysis [2]StickAnalysis
	DirDone  [4]bool // a direction step completed (reached by both sticks)
	DirSkip  [4]bool // a direction step timed out or was skipped
	RotateOK bool

	scenario func(device.InputScenario)
}

// NewStickTest creates the test for a layout.
func NewStickTest(layout device.InputLayout, scenario func(device.InputScenario)) *StickTest {
	t := &StickTest{Layout: layout, Duration: 3 * time.Second, PreviewDeadzone: DefaultPreviewDeadzone, scenario: scenario}
	for i := range t.Live {
		t.Live[i].jitter = [2]*Ring{NewRing(100), NewRing(100)}
		t.Live[i].HistX = NewRing(historyLen)
		t.Live[i].HistY = NewRing(historyLen)
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

// Normalize converts raw stick counts to [-1, 1] using the declared ranges.
func (t *StickTest) Normalize(stick int, raw [2]float64) [2]float64 {
	return [2]float64{
		t.Layout.StickAxis(2 * stick).NormalizeBipolar(raw[0]),
		t.Layout.StickAxis(2*stick + 1).NormalizeBipolar(raw[1]),
	}
}

// Feed processes one telemetry update (which may carry many reports).
//
// Steps advance on each sample's own timestamp, so a backlog of reports
// (e.g. after a slow UI frame) is attributed to the step it was measured in
// instead of ending the step early with too few samples.
func (t *StickTest) Feed(tel device.Telemetry, now time.Time) {
	samples := inputSamples(tel, t.Layout, now)
	for _, s := range samples {
		t.Tick(s.At)
		t.feedSample(s)
	}
	if len(samples) > 0 {
		for i := range t.Live {
			t.Live[i].HistX.Push(t.Live[i].Norm[0])
			t.Live[i].HistY.Push(t.Live[i].Norm[1])
		}
	}
	if len(samples) > 0 {
		t.Tick(samples[len(samples)-1].At)
	} else {
		t.Tick(now) // no data: time still runs out for the step
	}
}

func (t *StickTest) feedSample(s device.RawSample) {
	settled := !s.At.Before(t.StepStart.Add(stickSettle))
	for i := 0; i < 2; i++ {
		raw := [2]float64{float64(s.Sticks[2*i]), float64(s.Sticks[2*i+1])}
		n := t.Normalize(i, raw)
		l := &t.Live[i]
		l.Raw, l.Norm = raw, n
		l.Valid = true
		for a := range raw {
			l.Min[a] = math.Min(l.Min[a], raw[a])
			l.Max[a] = math.Max(l.Max[a], raw[a])
			l.jitter[a].Push(n[a])
		}
		l.Trail = append(l.Trail, n)
		if len(l.Trail) > trailLen {
			l.Trail = l.Trail[1:]
		}
		switch t.Step {
		case StickRelease:
			if settled {
				t.Rest[i].add(s, raw, n)
			}
		case StickRelease2:
			if settled {
				t.Rest2[i].add(s, raw, n)
			}
		case StickUp, StickDown, StickLeft, StickRight, StickRotate:
			t.Travel[i].add(n)
		}
	}
}

// stepReached reports whether both sticks completed the current movement step.
func (t *StickTest) stepReached() bool {
	switch t.Step {
	case StickUp, StickDown, StickLeft, StickRight:
		d := int(t.Step - StickUp)
		return t.Travel[0].Reach[d] >= reachThreshold && t.Travel[1].Reach[d] >= reachThreshold
	case StickRotate:
		return t.Travel[0].Coverage() >= rimSectorsGoal && t.Travel[1].Coverage() >= rimSectorsGoal
	}
	return false
}

// Tick advances timed steps and completes movement steps.
func (t *StickTest) Tick(now time.Time) {
	switch t.Step {
	case StickRelease:
		if !now.Before(t.StepEnd) {
			t.analyzeAll(false)
			t.Step = StickExplain
			t.setScenario(device.ScenarioDemo)
		}
	case StickUp, StickDown, StickLeft, StickRight, StickRotate:
		if t.CircularityOnly && (!now.Before(t.StepEnd) || t.stepReached()) {
			t.Step = StickDone
			t.setScenario(device.ScenarioDemo)
			return
		}
		reached := t.stepReached()
		if reached || !now.Before(t.StepEnd) {
			if t.Step == StickRotate {
				t.RotateOK = reached
			} else {
				d := int(t.Step - StickUp)
				t.DirDone[d], t.DirSkip[d] = reached, !reached
			}
			t.advanceMovement(now)
		}
	case StickRelease2:
		if !now.Before(t.StepEnd) {
			t.analyzeAll(true)
			t.Step = StickDone
			t.setScenario(device.ScenarioDemo)
		}
	}
}

func (t *StickTest) begin(step StickStep, now time.Time, d time.Duration) {
	t.Step, t.StepStart, t.StepEnd = step, now, now.Add(d)
}

func (t *StickTest) advanceMovement(now time.Time) {
	switch t.Step {
	case StickUp, StickDown, StickLeft:
		t.begin(t.Step+1, now, directionTimeout)
	case StickRight:
		t.begin(StickRotate, now, rotateTimeout)
	case StickRotate:
		t.begin(StickRelease2, now, stickSettle+release2Duration)
		for i := range t.Rest2 {
			t.Rest2[i].mid = now.Add(stickSettle + release2Duration/2)
		}
		t.setScenario(device.ScenarioIdle)
	}
}

// Start begins step 1: the user has released both sticks.
func (t *StickTest) Start(now time.Time) {
	t.CircularityOnly = false
	t.Rest, t.Rest2 = [2]restStats{}, [2]restStats{}
	t.Travel = [2]TravelStats{}
	t.Analysis = [2]StickAnalysis{}
	t.DirDone, t.DirSkip, t.RotateOK = [4]bool{}, [4]bool{}, false
	t.begin(StickRelease, now, stickSettle+t.Duration)
	mid := now.Add(stickSettle + t.Duration/2)
	for i := range t.Rest {
		t.Rest[i].mid = mid
	}
	t.setScenario(device.ScenarioIdle)
}

// Continue moves from the explanation (step 2) to the movement steps (step 3).
func (t *StickTest) Continue(now time.Time) {
	if t.Step != StickExplain {
		return
	}
	t.Travel = [2]TravelStats{}
	t.Rest2 = [2]restStats{}
	t.begin(StickUp, now, directionTimeout)
	t.setScenario(device.ScenarioMove)
}

// Finish ends the test after step 2 with the preliminary (resting-only) result.
func (t *StickTest) Finish() {
	if t.Step == StickExplain {
		t.Step = StickDone
	}
}

// SkipStep skips the current movement step (counted as not reached).
func (t *StickTest) SkipStep(now time.Time) {
	switch t.Step {
	case StickUp, StickDown, StickLeft, StickRight:
		t.DirSkip[int(t.Step-StickUp)] = true
		t.advanceMovement(now)
	case StickRotate:
		t.advanceMovement(now)
	}
}

// Cancel aborts any running step without producing results.
// A finished step-2 explanation is kept as the (preliminary) result.
func (t *StickTest) Cancel() {
	switch {
	case t.Step == StickExplain:
		t.Step = StickDone
	case t.Running():
		t.Step = StickIdle
		t.setScenario(device.ScenarioDemo)
	}
}

// Running reports whether a measurement step is active.
func (t *StickTest) Running() bool {
	return t.Step == StickRelease || (t.Step >= StickUp && t.Step <= StickRelease2)
}

// Moving reports whether a movement step (3) is active.
func (t *StickTest) Moving() bool { return t.Step >= StickUp && t.Step <= StickRotate }

// Progress returns 0..1 of the current step.
func (t *StickTest) Progress(now time.Time) float64 {
	switch t.Step {
	case StickUp, StickDown, StickLeft, StickRight:
		d := int(t.Step - StickUp)
		return math.Min(1, math.Min(t.Travel[0].Reach[d], t.Travel[1].Reach[d])/reachThreshold)
	case StickRotate:
		return math.Min(1, float64(min(t.Travel[0].Coverage(), t.Travel[1].Coverage()))/rimSectorsGoal)
	}
	total := t.StepEnd.Sub(t.StepStart)
	if total <= 0 {
		return 0
	}
	return clamp01(float64(now.Sub(t.StepStart)) / float64(total))
}

// Settling reports whether the current resting step is still ignoring samples.
func (t *StickTest) Settling(now time.Time) bool {
	return (t.Step == StickRelease || t.Step == StickRelease2) && now.Before(t.StepStart.Add(stickSettle))
}

// Tolerances returns the center and noise tolerances in % of travel. They
// come from the driver's declared heuristics (raw counts), converted with the
// device's own range; 3% / 1% when the driver declares none.
func (t *StickTest) Tolerances() (centerPct, noisePct float64) {
	hs := t.Layout.StickAxis(0).HalfSpan()
	centerPct, noisePct = 3, 1
	if hs > 0 && t.Layout.Drift.CenterTolerance > 0 {
		centerPct = 100 * t.Layout.Drift.CenterTolerance / hs
	}
	if hs > 0 && t.Layout.Drift.NoiseTolerance > 0 {
		noisePct = 100 * t.Layout.Drift.NoiseTolerance / hs
	}
	return centerPct, noisePct
}

func (t *StickTest) analyzeAll(final bool) {
	for i := range t.Analysis {
		t.Analysis[i] = t.analyze(i, final)
	}
}

// DirectionLabel names a movement direction.
func DirectionLabel(d int) string {
	return [...]string{i18n.T("up"), i18n.T("down"), i18n.T("left"), i18n.T("right")}[d]
}

func (t *StickTest) analyze(i int, final bool) StickAnalysis {
	tolC, tolN := t.Tolerances()
	moveLimit := math.Max(8*tolN, 8)
	name := inSentence(i18n.T(t.Layout.StickLabels[i]))
	a := StickAnalysis{RestA: summarizeRest(t.Rest[i], t.Layout, i), Travel: t.Travel[i]}
	ra := a.RestA
	retry := i18n.T("Release the stick completely, put the controller on a table, and run the test again.")

	switch {
	case ra.N < minRestSamples:
		a.Verdict = VerdictInconclusive
		a.Headline = i18n.Tf("The result is inconclusive: too few readings arrived from the %s.", name)
		a.Advice = append(a.Advice, i18n.T("Keep the controller connected and run the test again."))
		return a
	case ra.P2PPct > moveLimit:
		a.Verdict = VerdictInconclusive
		a.Headline = i18n.Tf("The result is inconclusive. Try the test again without touching the %s.", name)
		a.Explanation = append(a.Explanation, i18n.Tf("The reading moved by %.0f%% of travel while the stick should have been released.", ra.P2PPct))
		a.Advice = append(a.Advice, retry)
		return a
	}

	overA := ra.OffsetPct > tolC
	noisy := ra.NoisePct > tolN
	offset := ra.OffsetPct

	if !final {
		switch {
		case !overA && !noisy:
			a.Verdict = VerdictNormal
			a.Headline = i18n.Tf("Your %s is resting close to the center (%.1f%% away).", name, ra.OffsetPct)
		case overA:
			a.Verdict = VerdictFurtherTesting
			a.Headline = i18n.Tf("Your %s rested %.1f%% away from the center. Continue with the movement test to see whether this repeats.", name, ra.OffsetPct)
		default:
			a.Verdict = VerdictFurtherTesting
			a.Headline = i18n.Tf("Your %s is centered, but its reading flickers slightly at rest (±%.1f%%).", name, ra.NoisePct)
		}
	} else {
		a.HasRestB = true
		a.RestB = summarizeRest(t.Rest2[i], t.Layout, i)
		rb := a.RestB
		a.TravelDone = true
		for d := 0; d < 4; d++ {
			if a.Travel.Reach[d] < reachThreshold {
				a.Missed = append(a.Missed, d)
			}
		}
		if rb.N < minRestSamples || rb.P2PPct > moveLimit {
			a.Verdict = VerdictInconclusive
			a.Headline = i18n.Tf("The result is inconclusive: the %s was moving during the second resting measurement.", name)
			a.Advice = append(a.Advice, retry)
			return a
		}
		overB := rb.OffsetPct > tolC
		a.ShiftPct = 100 * math.Hypot(rb.Mean[0]-ra.Mean[0], rb.Mean[1]-ra.Mean[1])
		consistent := a.ShiftPct <= math.Max(tolC, 0.5*math.Max(ra.OffsetPct, rb.OffsetPct))
		offset = math.Max(ra.OffsetPct, rb.OffsetPct)
		switch {
		case overA && overB && consistent:
			a.Verdict = VerdictPossibleDrift
			a.Headline = i18n.Tf("Your %s shows some center movement: it rested %.1f%% and %.1f%% away from the center in two measurements. This may cause unwanted movement with very small deadzones.", name, ra.OffsetPct, rb.OffsetPct)
		case overA || overB:
			a.Verdict = VerdictFurtherTesting
			a.Headline = i18n.Tf("Your %s did not rest at the same point each time (%.1f%%, then %.1f%% from the center). Repeat the test to see whether this persists.", name, ra.OffsetPct, rb.OffsetPct)
		case noisy || rb.NoisePct > tolN:
			a.Verdict = VerdictFurtherTesting
			a.Headline = i18n.Tf("Your %s is centered, but its reading flickers slightly at rest (±%.1f%%).", name, math.Max(ra.NoisePct, rb.NoisePct))
		default:
			a.Verdict = VerdictNormal
			a.Headline = i18n.Tf("Your %s is resting close to the center (%.1f%% away) and returns to the same point after movement.", name, offset)
		}
	}

	// Why it matters, in terms of the deadzone preview.
	dz := t.PreviewDeadzone
	if offset > tolC {
		if FilteredBy(offset, dz) {
			a.Explanation = append(a.Explanation, i18n.Tf("The measured deviation was %.1f%%. At the selected %.0f%% deadzone preview, this movement would be filtered out.", offset, dz))
		} else {
			a.Explanation = append(a.Explanation, i18n.Tf("The measured deviation was %.1f%%. At the selected %.0f%% deadzone preview, this movement would still reach the game; a deadzone of about %.0f%% would hide it.", offset, dz, suggestDeadzone(offset)))
		}
		a.Explanation = append(a.Explanation, i18n.Tf("Expected: a released stick rests within about %.1f%% of the center (an interpretation heuristic, not a manufacturer specification).", tolC))
	}
	if noisy {
		a.Explanation = append(a.Explanation, i18n.T("A flickering reading at rest is usually electrical noise; most games hide it with their deadzone."))
	}
	switch a.Verdict {
	case VerdictPossibleDrift:
		a.Explanation = append(a.Explanation, i18n.T("This is not proof of a hardware defect: dirt, a slightly worn sensor or calibration differences can cause it."))
		a.Advice = append(a.Advice,
			i18n.T("Repeat the test under the same conditions. If unwanted movement persists in games, seek device-specific support."))
	case VerdictFurtherTesting:
		a.Advice = append(a.Advice, i18n.T("Run the test again; a result that changes between runs is not a reliable sign of a fault."))
	}
	if final {
		if len(a.Missed) == 0 {
			a.Explanation = append(a.Explanation, i18n.T("Full movement range reached in all four directions."))
		} else {
			var dirs []string
			for _, d := range a.Missed {
				dirs = append(dirs, i18n.Tf("%s %.0f%%", DirectionLabel(d), 100*a.Travel.Reach[d]))
			}
			a.Explanation = append(a.Explanation, i18n.Tf("The edge was not reached in every direction (%s). Push the stick all the way and repeat; this alone does not indicate a fault.", strings.Join(dirs, ", ")))
		}
	}
	return a
}

// inSentence lowercases a label used mid-sentence in English ("your left
// stick"); other languages keep the label as written.
func inSentence(label string) string {
	if i18n.Current() == i18n.English {
		return strings.ToLower(label)
	}
	return label
}

// suggestDeadzone returns the smallest preview preset that hides offsetPct.
func suggestDeadzone(offsetPct float64) float64 {
	for _, p := range DeadzonePresets {
		if p >= offsetPct+0.5 {
			return p
		}
	}
	return math.Ceil(offsetPct + 1)
}

// Preliminary reports whether the shown result is based on step 1 only.
func (t *StickTest) Preliminary() bool {
	return (t.Step == StickExplain || t.Step == StickDone) && !t.Analysis[0].HasRestB
}

// Result combines both sticks into a category result (nil before step 1 ends).
func (t *StickTest) Result(now time.Time, simulated bool) *Result {
	if t.CircularityOnly {
		if t.Step != StickDone {
			return nil
		}
		r := &Result{Category: CatSticks, Status: Passed, Simulated: simulated, At: now, Summary: "Circularity envelope recorded; shape is descriptive, not a hardware fault score."}
		for i, tr := range t.Travel {
			v := tr.Circularity()
			if v.Covered < rimSectorsGoal {
				r.Status = Incomplete
			}
			name := t.Layout.StickLabels[i]
			r.Measurements = append(r.Measurements, Measurement{name + " circularity coverage", float64(v.Covered), "of 16"}, Measurement{name + " radius spread", v.RadiusSpreadPct, "%"}, Measurement{name + " radius standard deviation", v.RadiusStdPct, "%"}, Measurement{name + " range asymmetry", v.AsymmetryPct, "%"})
		}
		r.Limitations = []string{"Coverage counts 16 sectors reached at 70% of declared travel; 14 sectors is a completion heuristic, not a hardware specification.", "Square and other gate shapes are valid. Incomplete travel cannot establish a fault."}
		return r
	}
	if t.Step != StickExplain && t.Step != StickDone {
		return nil
	}
	res := &Result{Category: CatSticks, Simulated: simulated, At: now}
	var st []Status
	var drift, retest []string
	for i, a := range t.Analysis {
		st = append(st, a.Status())
		lbl := i18n.T(t.Layout.StickLabels[i])
		switch a.Verdict {
		case VerdictPossibleDrift:
			drift = append(drift, lbl)
		case VerdictFurtherTesting:
			retest = append(retest, lbl)
		}
		res.Details = append(res.Details, a.Headline)
		res.Details = append(res.Details, a.Explanation...)
		res.Details = append(res.Details, a.Advice...)
		ra := a.RestA
		if ra.N == 0 {
			res.Measurements = append(res.Measurements, Measurement{lbl + " samples", 0, ""})
			res.Details = append(res.Details, lbl+": resting measurements unavailable; no samples arrived.")
			continue
		}
		res.Measurements = append(res.Measurements,
			Measurement{i18n.Tf("%s center offset", lbl), ra.OffsetPct, i18n.T("% of travel")},
			Measurement{i18n.Tf("%s center offset (raw)", lbl), ra.OffsetRaw, i18n.T("counts")},
			Measurement{i18n.Tf("%s average resting deviation", lbl), ra.AvgDevPct, i18n.T("% of travel")},
			Measurement{i18n.Tf("%s median resting deviation (bounded window)", lbl), ra.MedianDevPct, i18n.T("% of travel")},
			Measurement{i18n.Tf("%s maximum resting deviation", lbl), ra.MaxDevPct, i18n.T("% of travel")},
			Measurement{i18n.Tf("%s noise σ", lbl), ra.NoisePct, i18n.T("% of travel")},
			Measurement{i18n.Tf("%s noise σ (raw)", lbl), ra.NoiseRaw, i18n.T("counts")},
			Measurement{i18n.Tf("%s stability (mean shift)", lbl), ra.StableDiff, i18n.T("% of travel")},
			Measurement{i18n.Tf("%s samples", lbl), float64(ra.N), ""},
		)
		if ra.RateHz > 0 {
			res.Measurements = append(res.Measurements, Measurement{i18n.Tf("%s sample rate", lbl), ra.RateHz, i18n.T("Hz")})
		}
		if a.HasRestB {
			res.Measurements = append(res.Measurements,
				Measurement{i18n.Tf("%s second center offset", lbl), a.RestB.OffsetPct, i18n.T("% of travel")},
				Measurement{i18n.Tf("%s return-to-center shift", lbl), a.ShiftPct, i18n.T("% of travel")},
				Measurement{i18n.Tf("%s travel coverage", lbl), 100 * float64(a.Travel.Coverage()) / stickSectors, "%"},
				Measurement{i18n.Tf("%s outer radius", lbl), a.Travel.OuterRadiusPct(), "%"},
			)
			for d := 0; d < 4; d++ {
				res.Measurements = append(res.Measurements, Measurement{i18n.Tf("%s reach %s", lbl, DirectionLabel(d)), 100 * a.Travel.Reach[d], "%"})
			}
		}
	}
	res.Status = worst(st...)
	if res.Status == Passed && !t.Preliminary() && (!t.RotateOK || len(t.Analysis[0].Missed) > 0 || len(t.Analysis[1].Missed) > 0) {
		res.Status = Incomplete
	}
	switch {
	case len(drift) > 0:
		res.Summary = i18n.Tf("Possible drift: %s. Not a confirmed hardware fault.", strings.Join(drift, ", "))
	case res.Status == Inconclusive:
		res.Summary = i18n.T("Inconclusive — repeat the test without touching the sticks.")
	case len(retest) > 0:
		res.Summary = i18n.Tf("Further testing recommended: %s.", strings.Join(retest, ", "))
	case res.Status == Incomplete:
		res.Summary = i18n.T("Resting measurements completed; movement coverage is incomplete. Repeat the movement test.")
	case t.Preliminary():
		res.Summary = i18n.T("Both sticks rest close to the center (movement step not run).")
	default:
		res.Summary = i18n.T("Both sticks rest close to the center and cover their full range.")
	}
	tolC, tolN := t.Tolerances()
	res.Limitations = []string{
		i18n.Tf("Tolerances (center %.1f%%, noise %.1f%% of travel) are interpretation heuristics, not manufacturer specifications. ", tolC, tolN) + i18n.T(t.Layout.Drift.Rationale),
		i18n.T("The first 0.5 s of each resting step is ignored while the stick springs back."),
		i18n.T("Deadzone preview only — TuiSense does not change game input."),
		i18n.T("Stick calibration is never read from or written to the controller."),
	}
	if t.SourceNote != "" {
		res.Limitations = append(res.Limitations, t.SourceNote)
	}
	return res
}

// Preview returns the selected simulation. No hardware configuration is written.
func (t *StickTest) Preview(n [2]float64) DeadzonePreview {
	if t.AxialPreview {
		return PreviewAxialDeadzone(n, t.PreviewDeadzone)
	}
	return PreviewDeadzone(n, t.PreviewDeadzone)
}

// Circularity describes the observed sector envelope, without imposing a
// circular gate on devices that intentionally use square or other gates.
type Circularity struct {
	Covered                                                    int
	MeanRadiusPct, RadiusStdPct, RadiusSpreadPct, AsymmetryPct float64
}

func (t TravelStats) Circularity() Circularity {
	var radii AxisStats
	for _, r := range t.Sector {
		if r >= rimRadius {
			radii.Add(100 * r)
		}
	}
	c := Circularity{Covered: radii.N, MeanRadiusPct: radii.Mean(), RadiusStdPct: radii.Std(), RadiusSpreadPct: radii.PeakToPeak()}
	c.AsymmetryPct = 100 * math.Max(math.Abs(t.Reach[DirLeft]-t.Reach[DirRight]), math.Abs(t.Reach[DirUp]-t.Reach[DirDown]))
	return c
}

// StartCircularity collects the movement envelope without a resting drift test.
func (t *StickTest) StartCircularity(now time.Time) {
	t.Start(now)
	t.CircularityOnly = true
	t.Step = StickRotate
	t.StepStart = now
	t.StepEnd = now.Add(rotateTimeout)
	t.setScenario(device.ScenarioMove)
}
