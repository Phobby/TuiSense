package diag

import (
	"fmt"
	"math"
	"time"

	"github.com/ygzztrk/tuisense/internal/device"
)

// TriggerPhase is a step of the guided trigger test.
type TriggerPhase int

const (
	TriggerIdle  TriggerPhase = iota
	TriggerRest               // released: resting value & noise
	TriggerSweep              // pressed fully and released slowly
	TriggerDone
)

const (
	triggerRestDuration  = 2 * time.Second
	triggerSweepDuration = 6 * time.Second
	triggerBins          = 20
	// ResistanceTestDuration bounds the adaptive resistance effect.
	ResistanceTestDuration = 5 * time.Second
)

// TriggerLive is the real-time view of one trigger.
type TriggerLive struct {
	Raw, Norm float64
	Min, Max  float64
	History   *Ring // normalized travel for graphs
}

// TriggerAnalysis is the interpreted result for one trigger.
type TriggerAnalysis struct {
	RestPct, RestNoise, MaxPct, CoveragePct float64
	DigitalAtPct                            float64 // -1 when no digital bit / not observed
	Status                                  Status
	Notes                                   []string
}

// Actuator lets diagnostics drive hardware through the app's dispatcher. Tests
// that actuate always restore CurrentConfig(), the user's untouched settings.
type Actuator interface {
	Dispatch(device.Command)
	CurrentConfig() device.Config
}

// TriggerTest measures analog trigger travel and optionally runs a bounded
// adaptive-resistance check.
type TriggerTest struct {
	Layout device.InputLayout
	Live   [2]TriggerLive
	Phase  TriggerPhase
	start  time.Time
	end    time.Time

	rest     [2]AxisStats
	sweepMin [2]float64
	sweepMax [2]float64
	bins     [2][triggerBins]bool
	digital  [2]float64
	Analysis [2]TriggerAnalysis

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
	t := &TriggerTest{Layout: layout, ResistanceSupported: resistance, act: act, scenario: scenario}
	for i := range t.Live {
		t.Live[i].History = NewRing(60)
		t.Live[i].Min, t.Live[i].Max = math.Inf(1), math.Inf(-1)
	}
	return t
}

func (t *TriggerTest) setScenario(s device.InputScenario) {
	if t.scenario != nil {
		t.scenario(s)
	}
}

// Feed processes one sample.
func (t *TriggerTest) Feed(tel device.Telemetry, now time.Time) {
	r := t.Layout.TriggerRange
	span := r.Span()
	if span == 0 {
		return
	}
	raw := [2]float64{float64(tel.Raw.Triggers[0]), float64(tel.Raw.Triggers[1])}
	if !tel.Raw.Valid {
		raw = [2]float64{r.Min + tel.L2Pressure*span, r.Min + tel.R2Pressure*span}
	}
	digitalIDs := [2]device.ButtonID{device.ButtonL2Digital, device.ButtonR2Digital}
	for i := 0; i < 2; i++ {
		l := &t.Live[i]
		l.Raw = raw[i]
		l.Norm = (raw[i] - r.Min) / span
		l.Min, l.Max = math.Min(l.Min, raw[i]), math.Max(l.Max, raw[i])
		l.History.Push(l.Norm)
		switch t.Phase {
		case TriggerRest:
			t.rest[i].Add(raw[i])
		case TriggerSweep:
			t.sweepMin[i] = math.Min(t.sweepMin[i], raw[i])
			t.sweepMax[i] = math.Max(t.sweepMax[i], raw[i])
			b := int(l.Norm * triggerBins)
			if b >= triggerBins {
				b = triggerBins - 1
			}
			if b >= 0 {
				t.bins[i][b] = true
			}
			if _, ok := t.Layout.Button(digitalIDs[i]); ok && tel.Buttons.Has(digitalIDs[i]) && t.digital[i] < 0 {
				t.digital[i] = 100 * l.Norm
			}
		}
	}
	t.Tick(now)
}

// Start begins the guided test with the rest step.
func (t *TriggerTest) Start(now time.Time) {
	t.rest = [2]AxisStats{}
	t.sweepMin = [2]float64{math.Inf(1), math.Inf(1)}
	t.sweepMax = [2]float64{math.Inf(-1), math.Inf(-1)}
	t.bins = [2][triggerBins]bool{}
	t.digital = [2]float64{-1, -1}
	t.Phase, t.start, t.end = TriggerRest, now, now.Add(triggerRestDuration)
	t.setScenario(device.ScenarioIdle)
}

// Cancel aborts the guided test.
func (t *TriggerTest) Cancel() {
	if t.Phase == TriggerRest || t.Phase == TriggerSweep {
		t.Phase = TriggerIdle
		t.setScenario(device.ScenarioDemo)
	}
}

// Running reports whether a measurement step is active.
func (t *TriggerTest) Running() bool { return t.Phase == TriggerRest || t.Phase == TriggerSweep }

// Progress returns 0..1 of the current step.
func (t *TriggerTest) Progress(now time.Time) float64 {
	total := t.end.Sub(t.start)
	if total <= 0 {
		return 0
	}
	return math.Min(1, float64(now.Sub(t.start))/float64(total))
}

// Tick advances steps and enforces the resistance time limit.
func (t *TriggerTest) Tick(now time.Time) {
	if t.ResistanceActive && !now.Before(t.ResistanceUntil) {
		t.StopResistance()
	}
	if !t.Running() || now.Before(t.end) {
		return
	}
	if t.Phase == TriggerRest {
		t.Phase, t.start, t.end = TriggerSweep, now, now.Add(triggerSweepDuration)
		t.setScenario(device.ScenarioTriggers)
		return
	}
	t.Phase = TriggerDone
	t.setScenario(device.ScenarioDemo)
	r := t.Layout.TriggerRange
	for i := 0; i < 2; i++ {
		a := TriggerAnalysis{DigitalAtPct: t.digital[i]}
		a.RestPct = 100 * (t.rest[i].Mean() - r.Min) / r.Span()
		a.RestNoise = t.rest[i].Std()
		if !math.IsInf(t.sweepMax[i], -1) {
			a.MaxPct = 100 * (t.sweepMax[i] - r.Min) / r.Span()
		}
		covered := 0
		for _, b := range t.bins[i] {
			if b {
				covered++
			}
		}
		a.CoveragePct = 100 * float64(covered) / triggerBins
		name := t.Layout.TriggerLabels[i]
		switch {
		case t.rest[i].N < 5:
			a.Status = Inconclusive
			a.Notes = append(a.Notes, name+": too few samples.")
		case a.RestPct > 5:
			a.Status = Warning
			a.Notes = append(a.Notes, fmt.Sprintf("%s does not fully return to rest (reads %.0f%% when released) — sticking or not released.", name, a.RestPct))
		case a.MaxPct < 30:
			a.Status = Inconclusive
			a.Notes = append(a.Notes, name+" was barely pressed during the sweep step.")
		case a.MaxPct < 95:
			a.Status = Warning
			a.Notes = append(a.Notes, fmt.Sprintf("%s reached only %.0f%% of its travel — press fully, or check for an obstruction.", name, a.MaxPct))
		default:
			a.Status = Passed
			a.Notes = append(a.Notes, fmt.Sprintf("%s: rest %.1f%%, full travel %.0f%%.", name, a.RestPct, a.MaxPct))
		}
		if a.CoveragePct < 70 && a.MaxPct >= 30 {
			a.Notes = append(a.Notes, fmt.Sprintf("%s skipped parts of its travel (%.0f%% of positions seen) — release more slowly; persistent gaps can indicate a worn sensor.", name, a.CoveragePct))
		}
		t.Analysis[i] = a
	}
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
	if t.Phase != TriggerDone {
		return nil
	}
	res := &Result{Category: CatTriggers, Simulated: simulated, At: now}
	var st []Status
	for i, a := range t.Analysis {
		st = append(st, a.Status)
		res.Details = append(res.Details, a.Notes...)
		lbl := t.Layout.TriggerLabels[i]
		res.Measurements = append(res.Measurements,
			Measurement{lbl + " resting value", a.RestPct, "%"},
			Measurement{lbl + " resting noise σ", a.RestNoise, "counts"},
			Measurement{lbl + " max travel", a.MaxPct, "%"},
			Measurement{lbl + " travel positions seen", a.CoveragePct, "%"},
		)
		if a.DigitalAtPct >= 0 {
			res.Measurements = append(res.Measurements, Measurement{lbl + " digital activation", a.DigitalAtPct, "% travel"})
		}
	}
	if d := math.Abs(t.Analysis[0].MaxPct - t.Analysis[1].MaxPct); d > 10 {
		res.Details = append(res.Details, fmt.Sprintf("%s and %s differ by %.0f%% in maximum travel.", t.Layout.TriggerLabels[0], t.Layout.TriggerLabels[1], d))
	}
	if t.ResistanceFelt != nil {
		res.UserConfirmed = true
		if *t.ResistanceFelt {
			res.Details = append(res.Details, "Adaptive resistance: felt by the user.")
		} else {
			res.Details = append(res.Details, "Adaptive resistance: NOT felt by the user (check that the profile/firmware allows trigger effects).")
			st = append(st, Warning)
		}
	}
	res.Status = worst(st...)
	res.Summary = map[Status]string{
		Passed:       "Both triggers return to rest and reach full travel.",
		Warning:      "Trigger findings need attention — see details.",
		Inconclusive: "Trigger measurement inconclusive — retest.",
	}[res.Status]
	res.Limitations = []string{
		"Travel is sampled at ~25 Hz; very fast presses can skip positions.",
		"Adaptive resistance can only be confirmed by feel; the controller reports no force data.",
	}
	return res
}

// LastResistance reports whether a resistance run has happened (feedback can be given).
func (t *TriggerTest) LastResistance() bool { return t.resistanceRan && !t.ResistanceActive }
