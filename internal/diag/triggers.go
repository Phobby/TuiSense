package diag

import (
	"math"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
)

// TriggerStep is a step of the guided trigger test.
type TriggerStep int

const (
	TriggerIdle   TriggerStep = iota
	TriggerRest               // A: both released — resting value
	TriggerPressL             // B: press the left trigger fully
	TriggerPressR             //    …then the right one
	TriggerSlowL              // C: press and release slowly (analog only)
	TriggerSlowR
	TriggerReturn // D: released again — back to the baseline?
	TriggerDone
)

const (
	triggerSettle      = 500 * time.Millisecond
	triggerRestTime    = 1500 * time.Millisecond
	triggerPressTime   = 10 * time.Second
	triggerSlowTime    = 12 * time.Second
	triggerLinger      = 500 * time.Millisecond // keep a completed press on screen briefly
	triggerBins        = 20
	triggerFull        = 0.95 // analog: "fully pressed" at 95% of the declared range
	triggerSharedFull  = 0.90 // shared axis: 90% of one half
	triggerRestLimit   = 0.05 // released value above 5% counts as "not at rest"
	triggerNoiseLimit  = 0.02
	triggerJumpLimit   = 0.20
	triggerSlowPeak    = 0.80
	triggerMoved       = 0.10
	triggerReturnDelta = 0.03
	// ResistanceTestDuration bounds the adaptive resistance effect.
	ResistanceTestDuration = 5 * time.Second
)

// TriggerLive is the real-time view of one trigger.
type TriggerLive struct {
	Raw, Norm float64 // Norm: 0 = released, 1 = fully pressed
	Min, Max  float64 // raw extremes since the view opened
	History   *Ring   // normalized travel for graphs
}

// CheckState is the outcome of one line of a result checklist.
type CheckState int

const (
	CheckOK CheckState = iota
	CheckWarn
	CheckNotObserved
	CheckNotApplicable
)

// Icon returns a compact glyph for the check state.
func (c CheckState) Icon() string { return [...]string{"✓", "!", "?", "–"}[c] }

// Check is one human-readable line of a result checklist.
type Check struct {
	Label string
	State CheckState
	Text  string
}

// TriggerAnalysis is the interpreted result for one trigger.
type TriggerAnalysis struct {
	Verdict     Verdict
	Headline    string
	Checks      []Check
	Explanation []string
	Advice      []string
	Kind        device.TriggerKind

	RestPct, RestNoisePct, ReturnPct float64
	MaxPct, CoveragePct, MaxJumpPct  float64
	SampleRateHz                     float64
	Detected, FullRange, Returned    bool
}

// Status returns the health status of the analysis.
func (a TriggerAnalysis) Status() Status { return a.Verdict.Status() }

type triggerTrack struct {
	restA, restB AxisStats
	rate         rateCounter
	max          float64
	reachedAt    time.Time
	pressed      bool // digital: a press was seen in the press step
	released     bool // digital: released again after the press
	bins         [triggerBins]bool
	slowPeak     bool
	slowDone     bool
	maxJump      float64
	prev         float64
	havePrev     bool
	moved        bool
}

// Actuator lets diagnostics drive hardware through the app's dispatcher. Tests
// that actuate always restore CurrentConfig(), the user's untouched settings.
type Actuator interface {
	Dispatch(device.Command)
	CurrentConfig() device.Config
}

// TriggerTest is the guided trigger test. It adapts to how the current input
// mode exposes the triggers (analog axes, digital buttons, or one shared axis)
// and optionally runs a bounded adaptive-resistance check.
type TriggerTest struct {
	Layout device.InputLayout
	Kind   device.TriggerKind
	Live   [2]TriggerLive
	// SharedAxis is the normalized shared-axis value in [-1, 1] (shared kind only).
	SharedAxis float64

	Step      TriggerStep
	StepStart time.Time
	StepEnd   time.Time
	steps     []TriggerStep
	track     [2]triggerTrack
	lSign     float64 // shared axis: direction the left trigger moves it (learned)
	Analysis  [2]TriggerAnalysis

	ResistanceSupported bool
	ResistanceActive    bool
	ResistanceUntil     time.Time
	ResistanceFelt      *bool // user feedback after a run
	resistanceRan       bool

	act      Actuator
	scenario func(device.InputScenario)
}

// NewTriggerTest creates the trigger test.
func NewTriggerTest(layout device.InputLayout, resistance bool, act Actuator, scenario func(device.InputScenario)) *TriggerTest {
	t := &TriggerTest{Layout: layout, Kind: layout.TriggerKind, ResistanceSupported: resistance && layout.TriggerKind == device.TriggerAnalog, act: act, scenario: scenario}
	for i := range t.Live {
		t.Live[i].History = NewRing(historyLen)
		t.Live[i].Min, t.Live[i].Max = math.Inf(1), math.Inf(-1)
	}
	switch t.Kind {
	case device.TriggerAnalog:
		t.steps = []TriggerStep{TriggerRest, TriggerPressL, TriggerPressR, TriggerSlowL, TriggerSlowR, TriggerReturn}
	default:
		t.steps = []TriggerStep{TriggerRest, TriggerPressL, TriggerPressR, TriggerReturn}
	}
	return t
}

func (t *TriggerTest) setScenario(s device.InputScenario) {
	if t.scenario != nil {
		t.scenario(s)
	}
}

// Steps returns the guided steps for the current trigger kind.
func (t *TriggerTest) Steps() []TriggerStep { return t.steps }

// StepIndex returns the 1-based position of the current step (0 when idle).
func (t *TriggerTest) StepIndex() int {
	for i, s := range t.steps {
		if s == t.Step {
			return i + 1
		}
	}
	return 0
}

// values converts one sample to per-trigger travel in [0, 1].
func (t *TriggerTest) values(s device.RawSample) (v [2]float64, raw [2]float64) {
	raw = [2]float64{float64(s.Triggers[0]), float64(s.Triggers[1])}
	switch t.Kind {
	case device.TriggerDigital:
		ids := [2]device.ButtonID{device.ButtonL2Digital, device.ButtonR2Digital}
		for i, id := range ids {
			if s.Buttons.Has(id) {
				v[i] = 1
			}
			raw[i] = v[i]
		}
	case device.TriggerSharedAxis:
		axis := t.Layout.TriggerAxis(0).NormalizeBipolar(raw[0])
		t.SharedAxis = axis
		sign := t.lSign
		if sign == 0 {
			sign = 1
		}
		v[0], v[1] = math.Max(0, axis*sign), math.Max(0, -axis*sign)
		raw[1] = raw[0]
	default:
		for i := range v {
			v[i] = t.Layout.TriggerAxis(i).NormalizeUnipolar(raw[i])
		}
	}
	return v, raw
}

// Feed processes one telemetry update.
// Steps advance on each sample's own timestamp (see StickTest.Feed).
func (t *TriggerTest) Feed(tel device.Telemetry, now time.Time) {
	samples := inputSamples(tel, t.Layout, now)
	for _, s := range samples {
		t.Tick(s.At)
		t.feedSample(s)
	}
	if len(samples) > 0 {
		for i := range t.Live {
			t.Live[i].History.Push(t.Live[i].Norm)
		}
	}
	if len(samples) > 0 {
		t.Tick(samples[len(samples)-1].At)
	} else {
		t.Tick(now) // no data: time still runs out for the step
	}
}

func (t *TriggerTest) feedSample(s device.RawSample) {
	if t.Kind == device.TriggerSharedAxis && t.Step == TriggerPressL && t.lSign == 0 {
		axis := t.Layout.TriggerAxis(0).NormalizeBipolar(float64(s.Triggers[0]))
		if math.Abs(axis) > 0.5 {
			t.lSign = math.Copysign(1, axis)
		}
	}
	v, raw := t.values(s)
	settled := !s.At.Before(t.StepStart.Add(triggerSettle))
	for i := 0; i < 2; i++ {
		l := &t.Live[i]
		l.Raw, l.Norm = raw[i], v[i]
		l.Min, l.Max = math.Min(l.Min, raw[i]), math.Max(l.Max, raw[i])
		tr := &t.track[i]
		own := (i == 0 && (t.Step == TriggerPressL || t.Step == TriggerSlowL)) || (i == 1 && (t.Step == TriggerPressR || t.Step == TriggerSlowR))
		switch t.Step {
		case TriggerRest:
			if settled {
				tr.restA.Add(v[i])
				tr.rate.add(s)
			}
		case TriggerReturn:
			if settled {
				tr.restB.Add(v[i])
			}
		case TriggerPressL, TriggerPressR, TriggerSlowL, TriggerSlowR:
			if v[i] > triggerMoved {
				tr.moved = true
			}
			if !own {
				break
			}
			tr.max = math.Max(tr.max, v[i])
			tr.rate.add(s)
			full := triggerFull
			if t.Kind == device.TriggerSharedAxis {
				full = triggerSharedFull
			}
			if (t.Step == TriggerPressL || t.Step == TriggerPressR) && v[i] >= full && tr.reachedAt.IsZero() {
				tr.reachedAt = s.At
			}
			if t.Kind == device.TriggerDigital {
				if v[i] > 0 {
					tr.pressed = true
				} else if tr.pressed {
					tr.released = true
				}
			}
			if t.Step == TriggerSlowL || t.Step == TriggerSlowR {
				b := min(int(v[i]*triggerBins), triggerBins-1)
				tr.bins[b] = true
				if v[i] >= triggerSlowPeak {
					tr.slowPeak = true
				}
				if tr.slowPeak && v[i] < triggerMoved {
					tr.slowDone = true
				}
				if tr.havePrev && !s.Held {
					tr.maxJump = math.Max(tr.maxJump, math.Abs(v[i]-tr.prev))
				}
				tr.prev, tr.havePrev = v[i], true
			}
		}
	}
}

// stepComplete reports whether the current interactive step has finished.
func (t *TriggerTest) stepComplete(now time.Time) bool {
	switch t.Step {
	case TriggerPressL, TriggerPressR:
		tr := t.track[int(t.Step-TriggerPressL)]
		if t.Kind == device.TriggerDigital {
			return tr.released
		}
		return !tr.reachedAt.IsZero() && now.Sub(tr.reachedAt) >= triggerLinger
	case TriggerSlowL, TriggerSlowR:
		return t.track[int(t.Step-TriggerSlowL)].slowDone
	}
	return false
}

// Tick advances steps and enforces the resistance time limit.
func (t *TriggerTest) Tick(now time.Time) {
	if t.ResistanceActive && !now.Before(t.ResistanceUntil) {
		t.StopResistance()
	}
	if !t.Running() {
		return
	}
	if now.Before(t.StepEnd) && !t.stepComplete(now) {
		return
	}
	t.next(now)
}

func (t *TriggerTest) next(now time.Time) {
	idx := t.StepIndex()
	if idx >= len(t.steps) {
		t.finish()
		return
	}
	step := t.steps[idx]
	d := triggerPressTime
	switch step {
	case TriggerSlowL, TriggerSlowR:
		d = triggerSlowTime
	case TriggerReturn:
		d = triggerSettle + triggerRestTime
	}
	t.Step, t.StepStart, t.StepEnd = step, now, now.Add(d)
	switch step {
	case TriggerReturn:
		t.setScenario(device.ScenarioIdle)
	default:
		t.setScenario(device.ScenarioTriggers)
	}
}

// Start begins step A.
func (t *TriggerTest) Start(now time.Time) {
	t.track = [2]triggerTrack{}
	t.Analysis = [2]TriggerAnalysis{}
	t.lSign = 0
	t.Step, t.StepStart, t.StepEnd = TriggerRest, now, now.Add(triggerSettle+triggerRestTime)
	t.setScenario(device.ScenarioIdle)
}

// SkipStep ends the current interactive step early (counted as not observed).
func (t *TriggerTest) SkipStep(now time.Time) {
	switch t.Step {
	case TriggerPressL, TriggerPressR, TriggerSlowL, TriggerSlowR:
		t.next(now)
	}
}

// Cancel aborts the guided test without producing results.
func (t *TriggerTest) Cancel() {
	if t.Running() {
		t.Step = TriggerIdle
		t.setScenario(device.ScenarioDemo)
	}
}

// Running reports whether a step is active.
func (t *TriggerTest) Running() bool { return t.Step > TriggerIdle && t.Step < TriggerDone }

// Settling reports whether a resting step is still ignoring samples.
func (t *TriggerTest) Settling(now time.Time) bool {
	return (t.Step == TriggerRest || t.Step == TriggerReturn) && now.Before(t.StepStart.Add(triggerSettle))
}

// Progress returns 0..1 of the current step's time budget.
func (t *TriggerTest) Progress(now time.Time) float64 {
	total := t.StepEnd.Sub(t.StepStart)
	if total <= 0 {
		return 0
	}
	return clamp01(float64(now.Sub(t.StepStart)) / float64(total))
}

// Target returns the trigger the current step asks about (-1 = both).
func (t *TriggerTest) Target() int {
	switch t.Step {
	case TriggerPressL, TriggerSlowL:
		return 0
	case TriggerPressR, TriggerSlowR:
		return 1
	}
	return -1
}

func (t *TriggerTest) finish() {
	t.Step = TriggerDone
	t.setScenario(device.ScenarioDemo)
	for i := 0; i < 2; i++ {
		t.Analysis[i] = t.analyze(i)
	}
}

func (t *TriggerTest) kindLabel() string {
	switch t.Kind {
	case device.TriggerDigital:
		return i18n.T("digital button (on/off)")
	case device.TriggerSharedAxis:
		return i18n.T("shared analog axis (both triggers on one axis)")
	}
	return i18n.T("analog axis")
}

func (t *TriggerTest) analyze(i int) TriggerAnalysis {
	tr := t.track[i]
	name := t.Layout.TriggerLabels[i]
	a := TriggerAnalysis{Kind: t.Kind, SampleRateHz: tr.rate.Hz()}
	a.RestPct, a.RestNoisePct = 100*tr.restA.Mean(), 100*tr.restA.Std()
	a.ReturnPct = 100 * tr.restB.Mean()
	a.MaxPct = 100 * tr.max
	covered := 0
	for _, b := range tr.bins {
		if b {
			covered++
		}
	}
	a.CoveragePct = 100 * float64(covered) / triggerBins
	a.MaxJumpPct = 100 * tr.maxJump
	a.Detected = tr.moved || tr.max > triggerMoved
	a.FullRange = !tr.reachedAt.IsZero()
	restElevated := tr.restA.N > 0 && tr.restA.Mean() > triggerRestLimit
	returnElevated := tr.restB.N > 0 && tr.restB.Mean() > triggerRestLimit
	a.Returned = tr.restB.N > 0 && !returnElevated && math.Abs(tr.restB.Mean()-tr.restA.Mean()) <= math.Max(triggerReturnDelta, 4*tr.restA.Std())
	repeat := i18n.Tf("Run the trigger test again and press %s all the way down when asked.", name)

	add := func(label string, st CheckState, text string) {
		a.Checks = append(a.Checks, Check{Label: label, State: st, Text: text})
	}
	yesNo := func(ok bool, observed bool) CheckState {
		switch {
		case !observed:
			return CheckNotObserved
		case ok:
			return CheckOK
		}
		return CheckWarn
	}

	add(i18n.T("Detected"), yesNo(a.Detected, a.Detected), map[bool]string{true: i18n.T("yes — the trigger changed the reported value"), false: i18n.T("no movement seen during the test")}[a.Detected])
	add(i18n.T("Input type"), CheckOK, t.kindLabel())
	switch t.Kind {
	case device.TriggerDigital:
		add(i18n.T("Full range"), CheckNotApplicable, i18n.T("not applicable — a digital button has no travel range"))
	default:
		add(i18n.T("Full range"), yesNo(a.FullRange, a.Detected), i18n.Tf("highest value seen %.0f%%", a.MaxPct))
	}
	add(i18n.T("Resting value"), yesNo(!restElevated && a.RestNoisePct <= 100*triggerNoiseLimit, tr.restA.N > 0), i18n.Tf("%.1f%% when released (σ %.1f%%)", a.RestPct, a.RestNoisePct))
	add(i18n.T("Return to rest"), yesNo(a.Returned, tr.restB.N > 0), i18n.Tf("%.1f%% after release", a.ReturnPct))
	if t.Kind == device.TriggerAnalog {
		ran := tr.slowPeak
		add(i18n.T("Smooth movement"), yesNo(a.MaxJumpPct <= 100*triggerJumpLimit && ran, ran), i18n.Tf("largest jump between reports %.0f%%, %.0f%% of positions seen", a.MaxJumpPct, a.CoveragePct))
	}

	switch {
	case tr.restA.N == 0:
		a.Verdict = VerdictInconclusive
		a.Headline = i18n.Tf("The result is inconclusive: no readings arrived from %s.", name)
		a.Advice = append(a.Advice, i18n.T("Keep the controller connected and run the test again."))
	case restElevated && returnElevated:
		a.Verdict = VerdictPossibleIssue
		a.Headline = i18n.Tf("%s reads %.0f%% and %.0f%% while released. A trigger that does not return fully can cause unwanted input in games.", name, a.RestPct, a.ReturnPct)
		a.Explanation = append(a.Explanation, i18n.T("Expected: close to 0% when released. Possible causes are debris, a weak return spring or calibration — or a finger resting on the trigger."))
		a.Advice = append(a.Advice, i18n.T("Make sure nothing touches the triggers and repeat the test."))
	case !a.Detected:
		a.Verdict = VerdictInconclusive
		a.Headline = i18n.Tf("Test incomplete: no movement was detected from %s.", name)
		a.Explanation = append(a.Explanation, i18n.T("This does not necessarily indicate a hardware problem. If the trigger never responds, the controller may be in an input mode that reports it differently."))
		a.Advice = append(a.Advice, repeat)
	case t.Kind == device.TriggerDigital && !tr.released:
		a.Verdict = VerdictInconclusive
		a.Headline = i18n.Tf("Test incomplete: a full press and release of %s was not observed.", name)
		a.Advice = append(a.Advice, repeat)
	case t.Kind != device.TriggerDigital && !a.FullRange:
		a.Verdict = VerdictInconclusive
		a.Headline = i18n.Tf("Test incomplete: %s reached %.0f%% of its expected range during this test.", name, a.MaxPct)
		a.Explanation = append(a.Explanation, i18n.T("This does not necessarily indicate a hardware problem. Try pressing the trigger fully and running the test again."))
		a.Advice = append(a.Advice, repeat)
	case restElevated || returnElevated || a.RestNoisePct > 100*triggerNoiseLimit || (tr.restB.N > 0 && !a.Returned):
		a.Verdict = VerdictFurtherTesting
		a.Headline = i18n.Tf("%s did not settle at the same resting value each time (%.1f%%, then %.1f%%).", name, a.RestPct, a.ReturnPct)
		a.Advice = append(a.Advice, i18n.T("Release both triggers completely and repeat; a single elevated reading is often a finger still touching the trigger."))
	case t.Kind == device.TriggerAnalog && tr.slowPeak && a.MaxJumpPct > 100*triggerJumpLimit:
		a.Verdict = VerdictFurtherTesting
		a.Headline = i18n.Tf("%s jumped by %.0f%% between two reports during the slow press.", name, a.MaxJumpPct)
		a.Explanation = append(a.Explanation, i18n.T("Pressing quickly causes jumps like this. Persistent jumps during very slow presses can point to a worn sensor."))
		a.Advice = append(a.Advice, i18n.T("Repeat the slow step, pressing and releasing over about two seconds."))
	default:
		a.Verdict = VerdictNormal
		switch t.Kind {
		case device.TriggerDigital:
			a.Headline = i18n.Tf("%s works as a digital button (on/off) in this input mode; analog range checks do not apply.", name)
		case device.TriggerSharedAxis:
			a.Headline = i18n.Tf("%s moves the shared trigger axis to %.0f%% and returns to rest.", name, a.MaxPct)
		default:
			a.Headline = i18n.Tf("%s works normally: it rests at %.0f%%, reaches %.0f%% when fully pressed and returns to rest.", name, a.RestPct, a.MaxPct)
		}
	}
	if t.Kind == device.TriggerAnalog && !tr.slowPeak && a.Verdict == VerdictNormal {
		a.Explanation = append(a.Explanation, i18n.T("The slow-movement step was skipped, so smoothness was not checked."))
	}
	return a
}

// StartResistance applies a moderate continuous resistance to both triggers
// for ResistanceTestDuration. It is only ever called from an explicit user action.
func (t *TriggerTest) StartResistance(now time.Time) bool {
	if !t.ResistanceSupported || t.act == nil || t.ResistanceActive {
		return false
	}
	cfg := t.act.CurrentConfig()
	tc := device.TriggerConfig{Mode: device.TriggerModeResistance, StartPosition: 40, EndPosition: 255, Strength: 150}
	cfg.LeftTrigger, cfg.RightTrigger = tc, tc
	t.act.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: cfg})
	t.ResistanceActive = true
	t.ResistanceUntil = now.Add(ResistanceTestDuration)
	t.ResistanceFelt = nil
	t.resistanceRan = true
	return true
}

// StopResistance restores the user's own trigger settings. Safe to call anytime.
func (t *TriggerTest) StopResistance() {
	if !t.ResistanceActive || t.act == nil {
		return
	}
	t.act.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: t.act.CurrentConfig()})
	t.ResistanceActive = false
}

// Result returns the trigger result once the guided test finished.
func (t *TriggerTest) Result(now time.Time, simulated bool) *Result {
	if t.Step != TriggerDone && t.ResistanceFelt == nil {
		return nil
	}
	res := &Result{Category: CatTriggers, Simulated: simulated, At: now}
	var st []Status
	var issues, retest []string
	if t.Step == TriggerDone {
		for i, a := range t.Analysis {
			lbl := t.Layout.TriggerLabels[i]
			st = append(st, a.Status())
			switch a.Verdict {
			case VerdictPossibleIssue:
				issues = append(issues, lbl)
			case VerdictFurtherTesting:
				retest = append(retest, lbl)
			}
			res.Details = append(res.Details, a.Headline)
			res.Details = append(res.Details, a.Explanation...)
			res.Details = append(res.Details, a.Advice...)
			res.Measurements = append(res.Measurements,
				Measurement{i18n.Tf("%s resting value", lbl), a.RestPct, "%"},
				Measurement{i18n.Tf("%s resting noise σ", lbl), a.RestNoisePct, "%"},
				Measurement{i18n.Tf("%s value after release", lbl), a.ReturnPct, "%"},
			)
			if t.Kind != device.TriggerDigital {
				res.Measurements = append(res.Measurements, Measurement{i18n.Tf("%s max travel", lbl), a.MaxPct, "%"})
			}
			if t.Kind == device.TriggerAnalog {
				res.Measurements = append(res.Measurements,
					Measurement{i18n.Tf("%s travel positions seen", lbl), a.CoveragePct, "%"},
					Measurement{i18n.Tf("%s largest jump", lbl), a.MaxJumpPct, "%"},
				)
			}
			if a.SampleRateHz > 0 {
				res.Measurements = append(res.Measurements, Measurement{i18n.Tf("%s sample rate", lbl), a.SampleRateHz, i18n.T("Hz")})
			}
		}
		if t.Kind == device.TriggerSharedAxis {
			res.Details = append(res.Details, i18n.T("In this input mode both triggers share one axis: pressing both at once cancels out, so they cannot be measured independently."))
		}
	}
	if t.ResistanceFelt != nil {
		res.UserConfirmed = true
		if *t.ResistanceFelt {
			res.Details = append(res.Details, i18n.T("Adaptive resistance: felt by the user."))
			if t.Step != TriggerDone {
				st = append(st, Passed)
			}
		} else {
			res.Details = append(res.Details, i18n.T("Adaptive resistance: NOT felt by the user (check that the profile/firmware allows trigger effects)."))
			st = append(st, Warning)
		}
	}
	res.Status = worst(st...)
	switch {
	case len(issues) > 0:
		res.Summary = i18n.Tf("Possible trigger issue: %s — see details.", strings.Join(issues, ", "))
	case res.Status == Inconclusive:
		res.Summary = i18n.T("Test incomplete — press each trigger fully when asked and repeat.")
	case len(retest) > 0:
		res.Summary = i18n.Tf("Further testing recommended: %s.", strings.Join(retest, ", "))
	case t.Step != TriggerDone:
		res.Summary = i18n.T("Adaptive resistance check recorded.")
	case t.Kind == device.TriggerDigital:
		res.Summary = i18n.T("Both triggers respond as digital buttons in this input mode.")
	case t.Kind == device.TriggerSharedAxis:
		res.Summary = i18n.T("Both triggers move the shared axis and return to rest.")
	default:
		res.Summary = i18n.T("Both triggers were detected, rest correctly and reach their full range.")
	}
	res.Limitations = []string{
		i18n.T("\"Test incomplete\" means the expected input was not observed — not that the trigger is broken."),
		i18n.T("Thresholds (95% for a full press, 5% for rest) are interpretation heuristics, not manufacturer specifications."),
	}
	if t.ResistanceSupported {
		res.Limitations = append(res.Limitations, i18n.T("Adaptive resistance can only be confirmed by feel; the controller reports no force data."))
	}
	return res
}

// LastResistance reports whether a resistance run has happened (feedback can be given).
func (t *TriggerTest) LastResistance() bool { return t.resistanceRan && !t.ResistanceActive }
