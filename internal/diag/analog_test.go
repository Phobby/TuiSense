package diag

import (
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
)

const tick = 40 * time.Millisecond

// feedSticks feeds raw stick samples at 25 Hz (one sample per update, as a
// simulated backend without per-report data would) until `until`.
func feedSticks(t *StickTest, from, until time.Time, gen func(i int) [4]int32) time.Time {
	now := from
	for i := 0; now.Before(until); i++ {
		t.Feed(device.Telemetry{Raw: device.RawInput{Valid: true, Sticks: gen(i)}}, now)
		now = now.Add(tick)
	}
	return now
}

func feedStep(t *StickTest, from time.Time, gen func(i int) [4]int32) time.Time {
	return feedSticks(t, from, t.StepEnd.Add(tick), gen)
}

func noisy(rng *rand.Rand, center float64, sigma float64) int32 {
	return int32(math.Round(center + rng.NormFloat64()*sigma))
}

// wideLayout is an evdev-style 16-bit layout (as reported by xpad).
func wideLayout() device.InputLayout {
	r := device.AxisRange{Min: -32768, Max: 32767, Center: -0.5}
	return device.InputLayout{
		StickLabels: [2]string{"Left stick", "Right stick"},
		StickRange:  r,
		StickAxes:   [4]device.AxisRange{r, r, r, r},
		Drift:       device.DriftHeuristics{CenterTolerance: 0.03 * r.HalfSpan(), NoiseTolerance: 0.01 * r.HalfSpan()},
	}
}

// runMovement drives the movement steps with a stick that reaches every direction.
func runMovement(t *testing.T, st *StickTest, now time.Time) time.Time {
	t.Helper()
	st.Continue(now)
	targets := map[StickStep][2]float64{StickUp: {0, -1}, StickDown: {0, 1}, StickLeft: {-1, 0}, StickRight: {1, 0}}
	for st.Moving() {
		step := st.Step
		var n [2]float64
		if dir, ok := targets[step]; ok {
			n = dir
		}
		ang := 0.0
		gen := func(int) [4]int32 {
			if step == StickRotate {
				ang += 0.45
				n = [2]float64{math.Cos(ang), math.Sin(ang)}
			}
			var out [4]int32
			for s := 0; s < 2; s++ {
				out[2*s] = int32(denormBipolar(st.Layout.StickAxis(2*s), n[0]))
				out[2*s+1] = int32(denormBipolar(st.Layout.StickAxis(2*s+1), n[1]))
			}
			return out
		}
		for i := 0; st.Step == step && i < 400; i++ {
			st.Feed(device.Telemetry{Raw: device.RawInput{Valid: true, Sticks: gen(i)}}, now)
			now = now.Add(tick)
		}
	}
	if st.Step != StickRelease2 {
		t.Fatalf("movement should end in the second release step, got %v", st.Step)
	}
	return now
}

func TestStickCenterNormal(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.Start(t0)
	feedStep(st, t0, func(int) [4]int32 {
		return [4]int32{noisy(rng, 128, 0.4), noisy(rng, 128, 0.4), noisy(rng, 129, 0.4), noisy(rng, 127, 0.4)}
	})
	if st.Step != StickExplain {
		t.Fatalf("after step 1 the explanation step should follow, got %v", st.Step)
	}
	a := st.Analysis[0]
	if a.Verdict != VerdictNormal || a.RestA.N < 60 {
		t.Fatalf("a centered stick is normal (~75 samples), got %v n=%d %q", a.Verdict, a.RestA.N, a.Headline)
	}
	if !strings.Contains(a.Headline, "close to the center") {
		t.Fatalf("plain-language headline expected, got %q", a.Headline)
	}
	if r := st.Result(t0, false); r == nil || r.Status != Passed || !st.Preliminary() {
		t.Fatalf("a preliminary passed result should exist after step 2")
	}
}

func TestStickSingleOffsetIsNotDrift(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.Start(t0)
	// 10 counts off (≈7.8%) in the first measurement only.
	now := feedStep(st, t0, func(int) [4]int32 { return [4]int32{noisy(rng, 138, 0.4), 128, 128, 128} })
	if v := st.Analysis[0].Verdict; v != VerdictFurtherTesting {
		t.Fatalf("one off-center measurement is only 'further testing', got %v", v)
	}
	now = runMovement(t, st, now)
	feedStep(st, now, func(int) [4]int32 { return [4]int32{noisy(rng, 128, 0.4), 128, 128, 128} })
	a := st.Analysis[0]
	if st.Step != StickDone || a.Verdict != VerdictFurtherTesting {
		t.Fatalf("offset that does not repeat must not be called drift, got %v %q", a.Verdict, a.Headline)
	}
	for _, r := range []*Result{st.Result(t0, false)} {
		if r.Status == Failed {
			t.Fatal("stick results are never Failed")
		}
	}
}

func TestStickConsistentOffsetIsPossibleDrift(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.PreviewDeadzone = 5
	drifting := func(int) [4]int32 { return [4]int32{128, 128, 128, noisy(rng, 118, 0.4)} } // right Y ≈ 7.8% up
	st.Start(t0)
	now := feedStep(st, t0, drifting)
	now = runMovement(t, st, now)
	feedStep(st, now, drifting)
	r, a := st.Analysis[1], st.Result(t0, false)
	if r.Verdict != VerdictPossibleDrift || a.Status != Warning {
		t.Fatalf("an offset repeated in both measurements is possible drift (Warning), got %v / %v", r.Verdict, a.Status)
	}
	if st.Analysis[0].Verdict != VerdictNormal {
		t.Fatalf("the centered left stick stays normal, got %v", st.Analysis[0].Verdict)
	}
	joined := strings.Join(r.Explanation, " ")
	if !strings.Contains(joined, "would still reach the game") || !strings.Contains(joined, "not proof of a hardware defect") {
		t.Fatalf("explanation must relate to the deadzone preview and avoid fault claims: %q", joined)
	}
	// With a larger preview deadzone the same data is described as filtered.
	st.PreviewDeadzone = 10
	st.analyzeAll(true)
	if !strings.Contains(strings.Join(st.Analysis[1].Explanation, " "), "would be filtered out") {
		t.Fatalf("10%% preview should filter a 7.8%% offset: %v", st.Analysis[1].Explanation)
	}
	if len(st.Analysis[1].Missed) != 0 || st.Analysis[1].Travel.Coverage() < 14 {
		t.Fatalf("movement should reach all directions, missed=%v coverage=%d", st.Analysis[1].Missed, st.Analysis[1].Travel.Coverage())
	}
}

func TestStickJitterIsFurtherTesting(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.Start(t0)
	feedStep(st, t0, func(int) [4]int32 { return [4]int32{noisy(rng, 128, 2.5), 128, 128, 128} })
	a := st.Analysis[0]
	if a.Verdict != VerdictFurtherTesting || a.RestA.NoisePct < 1.2 {
		t.Fatalf("centered but noisy stick → further testing, got %v noise %.2f%%", a.Verdict, a.RestA.NoisePct)
	}
	if j := st.Live[0].JitterPct(); j[0] < 1 {
		t.Fatalf("live jitter should reflect the noise, got %.2f", j[0])
	}
}

func TestStickMovementDuringRestIsInconclusive(t *testing.T) {
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.Start(t0)
	feedStep(st, t0, func(i int) [4]int32 { return [4]int32{int32(128 + 40*math.Sin(float64(i)/5)), 128, 128, 128} })
	if st.Analysis[0].Verdict != VerdictInconclusive || !strings.Contains(st.Analysis[0].Headline, "without touching") {
		t.Fatalf("movement must make the test inconclusive, got %v %q", st.Analysis[0].Verdict, st.Analysis[0].Headline)
	}
}

func TestStickNoSamplesIsInconclusive(t *testing.T) {
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.Start(t0)
	// A real backend that has not received a report yet: empty sample batch,
	// zero-valued (uninitialized) telemetry. Nothing may be counted.
	now := t0
	for now.Before(st.StepEnd.Add(tick)) {
		st.Feed(device.Telemetry{Samples: []device.RawSample{}}, now)
		now = now.Add(tick)
	}
	if a := st.Analysis[0]; a.Verdict != VerdictInconclusive || a.RestA.N != 0 {
		t.Fatalf("no readings → inconclusive, got %v n=%d", a.Verdict, a.RestA.N)
	}
}

func TestStickSameRelativeOffsetAcrossRanges(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	// 8-bit DualSense: 10 counts ≈ 7.8%; 16-bit evdev: the same 7.8%.
	narrow := NewStickTest(dualsense.Descriptor().Inputs, nil)
	narrow.Start(t0)
	feedStep(narrow, t0, func(int) [4]int32 { return [4]int32{noisy(rng, 138, 0.3), 128, 128, 128} })
	wide := NewStickTest(wideLayout(), nil)
	wide.Start(t0)
	off := int32(2556) // 7.8% of 32767
	feedStep(wide, t0, func(int) [4]int32 { return [4]int32{off + int32(rng.NormFloat64()*60), 0, 0, 0} })
	if narrow.Analysis[0].Verdict != wide.Analysis[0].Verdict {
		t.Fatalf("same relative offset must give the same verdict: %v vs %v", narrow.Analysis[0].Verdict, wide.Analysis[0].Verdict)
	}
	if d := math.Abs(narrow.Analysis[0].RestA.OffsetPct - wide.Analysis[0].RestA.OffsetPct); d > 0.5 {
		t.Fatalf("normalized offsets differ by %.2f%%", d)
	}
	if wide.Analysis[1].Verdict != VerdictNormal {
		t.Fatalf("16-bit centered stick (center -0.5) must be normal, got %v %.3f%%", wide.Analysis[1].Verdict, wide.Analysis[1].RestA.OffsetPct)
	}
}

func TestInvertedAxisMovement(t *testing.T) {
	l := wideLayout()
	for i := range l.StickAxes {
		if i%2 == 1 {
			l.StickAxes[i].Invert = true // this device reports +Y = up
		}
	}
	st := NewStickTest(l, nil)
	st.Start(t0)
	now := feedStep(st, t0, func(int) [4]int32 { return [4]int32{0, 0, 0, 0} })
	st.Continue(now)
	// Raw maximum on an inverted Y axis is "up".
	st.Feed(device.Telemetry{Raw: device.RawInput{Valid: true, Sticks: [4]int32{0, 32767, 0, 32767}}}, now.Add(tick))
	if st.Step != StickDown || st.Travel[0].Reach[DirUp] < 0.99 {
		t.Fatalf("raw max on an inverted axis must count as up, step=%v reach=%v", st.Step, st.Travel[0].Reach)
	}
}

func TestStickCancelProducesNoResult(t *testing.T) {
	var scenarios []device.InputScenario
	st := NewStickTest(dualsense.Descriptor().Inputs, func(s device.InputScenario) { scenarios = append(scenarios, s) })
	st.Start(t0)
	st.Cancel()
	if st.Running() || st.Result(t0, false) != nil {
		t.Fatalf("cancelled test must not produce a result")
	}
	if scenarios[len(scenarios)-1] != device.ScenarioDemo {
		t.Fatalf("cancel should restore the demo scenario on simulated devices")
	}
}

func TestDeadzonePreview(t *testing.T) {
	cases := []struct {
		n       [2]float64
		dz      float64
		ignored bool
		eff     float64
	}{
		{[2]float64{0, 0}, 8, false, 0},
		{[2]float64{0.05, 0}, 8, true, 0},
		{[2]float64{0.05, 0}, 0, false, 0.05},
		{[2]float64{0.5, 0}, 20, false, 0.375},
		{[2]float64{0, -1}, 20, false, 1},
		{[2]float64{0.06, 0.06}, 8, false, (math.Hypot(0.06, 0.06) - 0.08) / 0.92},
	}
	for _, c := range cases {
		p := PreviewDeadzone(c.n, c.dz)
		if p.Ignored != c.ignored || math.Abs(math.Hypot(p.Effective[0], p.Effective[1])-c.eff) > 1e-9 {
			t.Fatalf("PreviewDeadzone(%v, %.0f) = %+v, want ignored=%v eff=%.3f", c.n, c.dz, p, c.ignored, c.eff)
		}
	}
	if !FilteredBy(3.2, 8) || FilteredBy(7.8, 5) {
		t.Fatal("FilteredBy mismatch")
	}
	if suggestDeadzone(7.8) != 10 {
		t.Fatalf("7.8%% offset should suggest the 10%% preset, got %.0f", suggestDeadzone(7.8))
	}
}

// ---- Triggers ----

// trigSim drives the trigger test with a function of the current step.
func trigSim(tt *TriggerTest, from time.Time, val func(step TriggerStep, k int) ([2]int32, device.ButtonSet)) time.Time {
	now := from
	k := 0
	last := tt.Step
	for i := 0; tt.Running() && i < 5000; i++ {
		if tt.Step != last {
			last, k = tt.Step, 0
		}
		tr, b := val(tt.Step, k)
		tt.Feed(device.Telemetry{Raw: device.RawInput{Valid: true, Triggers: tr}, Buttons: b}, now)
		now = now.Add(tick)
		k++
	}
	return now
}

// slowPress ramps 0→255→0 over ~3 s (75 updates at 25 Hz).
func slowPress(k int) int32 {
	p := float64(k%75) / 74
	return int32(255 * (1 - math.Abs(2*p-1)))
}

func TestTriggerAnalogGuidedNormal(t *testing.T) {
	tt := NewTriggerTest(dualsense.Descriptor().Inputs, false, nil, nil)
	tt.Start(t0)
	trigSim(tt, t0, func(step TriggerStep, k int) ([2]int32, device.ButtonSet) {
		switch step {
		case TriggerPressL:
			return [2]int32{255, 0}, 0
		case TriggerPressR:
			return [2]int32{0, 255}, 0
		case TriggerSlowL:
			return [2]int32{slowPress(k), 0}, 0
		case TriggerSlowR:
			return [2]int32{0, slowPress(k)}, 0
		}
		return [2]int32{0, 0}, 0
	})
	for i, a := range tt.Analysis {
		if a.Verdict != VerdictNormal || !a.FullRange || !a.Returned || !a.Detected {
			t.Fatalf("trigger %d should be normal: %+v", i, a)
		}
	}
	if r := tt.Result(t0, false); r.Status != Passed {
		t.Fatalf("expected Passed, got %v", r.Status)
	}
}

func TestTriggerNotFullyPressedIsIncomplete(t *testing.T) {
	tt := NewTriggerTest(dualsense.Descriptor().Inputs, false, nil, nil)
	tt.Start(t0)
	trigSim(tt, t0, func(step TriggerStep, k int) ([2]int32, device.ButtonSet) {
		switch step {
		case TriggerPressL:
			return [2]int32{255, 0}, 0
		case TriggerPressR, TriggerSlowR:
			return [2]int32{0, 204}, 0 // R only ever reaches 80%
		case TriggerSlowL:
			return [2]int32{slowPress(k), 0}, 0
		}
		return [2]int32{0, 0}, 0
	})
	a := tt.Analysis[1]
	if a.Verdict != VerdictInconclusive || !strings.HasPrefix(a.Headline, "Test incomplete") {
		t.Fatalf("a press that never reaches full travel is 'Test incomplete', got %v %q", a.Verdict, a.Headline)
	}
	r := tt.Result(t0, false)
	if r.Status == Failed || r.Status == Warning {
		t.Fatalf("an incomplete press must not be reported as a fault, got %v", r.Status)
	}
}

func TestTriggerRestElevated(t *testing.T) {
	run := func(restB int32) TriggerAnalysis {
		tt := NewTriggerTest(dualsense.Descriptor().Inputs, false, nil, nil)
		tt.Start(t0)
		trigSim(tt, t0, func(step TriggerStep, k int) ([2]int32, device.ButtonSet) {
			switch step {
			case TriggerRest:
				return [2]int32{30, 0}, 0 // ≈12% while "released"
			case TriggerReturn:
				return [2]int32{restB, 0}, 0
			case TriggerPressL:
				return [2]int32{255, 0}, 0
			case TriggerPressR:
				return [2]int32{0, 255}, 0
			case TriggerSlowL:
				return [2]int32{max(30, slowPress(k)), 0}, 0
			case TriggerSlowR:
				return [2]int32{30, slowPress(k)}, 0
			}
			return [2]int32{}, 0
		})
		return tt.Analysis[0]
	}
	if a := run(30); a.Verdict != VerdictPossibleIssue {
		t.Fatalf("elevated in both resting measurements → possible issue, got %v %q", a.Verdict, a.Headline)
	}
	if a := run(0); a.Verdict != VerdictFurtherTesting {
		t.Fatalf("elevated only once → further testing, got %v %q", a.Verdict, a.Headline)
	}
}

func TestTriggerNoInputIsInconclusive(t *testing.T) {
	tt := NewTriggerTest(dualsense.Descriptor().Inputs, false, nil, nil)
	tt.Start(t0)
	trigSim(tt, t0, func(TriggerStep, int) ([2]int32, device.ButtonSet) { return [2]int32{}, 0 })
	for _, a := range tt.Analysis {
		if a.Verdict != VerdictInconclusive || a.Detected {
			t.Fatalf("no input → inconclusive, got %v", a.Verdict)
		}
	}
}

func TestTriggerDigitalFallback(t *testing.T) {
	l := device.InputLayout{
		TriggerLabels: [2]string{"LT", "RT"},
		TriggerKind:   device.TriggerDigital,
		TriggerRange:  device.AxisRange{Min: 0, Max: 1},
		Buttons:       []device.ButtonDef{{ID: device.ButtonL2Digital, Label: "LT"}, {ID: device.ButtonR2Digital, Label: "RT"}},
	}
	tt := NewTriggerTest(l, true, nil, nil)
	if tt.ResistanceSupported {
		t.Fatal("digital triggers never offer adaptive resistance")
	}
	for _, s := range tt.Steps() {
		if s == TriggerSlowL || s == TriggerSlowR {
			t.Fatal("digital triggers skip the analog smoothness step")
		}
	}
	tt.Start(t0)
	trigSim(tt, t0, func(step TriggerStep, k int) ([2]int32, device.ButtonSet) {
		switch {
		case step == TriggerPressL && k < 5:
			return [2]int32{}, device.ButtonSet(0).With(device.ButtonL2Digital)
		case step == TriggerPressR && k < 5:
			return [2]int32{}, device.ButtonSet(0).With(device.ButtonR2Digital)
		}
		return [2]int32{}, 0
	})
	for i, a := range tt.Analysis {
		if a.Verdict != VerdictNormal || !strings.Contains(a.Headline, "digital button") {
			t.Fatalf("digital trigger %d should pass as a button: %v %q", i, a.Verdict, a.Headline)
		}
		for _, c := range a.Checks {
			if c.Label == "Full range" && c.State != CheckNotApplicable {
				t.Fatal("analog range check must not apply to digital triggers")
			}
		}
	}
}

func TestTriggerSharedAxis(t *testing.T) {
	r := device.AxisRange{Min: 0, Max: 255, Center: 127.5}
	l := device.InputLayout{
		TriggerLabels: [2]string{"LT", "RT"},
		TriggerKind:   device.TriggerSharedAxis,
		TriggerRange:  r,
		TriggerAxes:   [2]device.AxisRange{r, r},
	}
	tt := NewTriggerTest(l, false, nil, nil)
	tt.Start(t0)
	trigSim(tt, t0, func(step TriggerStep, k int) ([2]int32, device.ButtonSet) {
		switch step {
		case TriggerPressL:
			return [2]int32{0, 0}, 0 // LT pushes the shared axis to its minimum on this device
		case TriggerPressR:
			return [2]int32{255, 255}, 0
		}
		return [2]int32{128, 128}, 0
	})
	if tt.lSign != -1 {
		t.Fatalf("the left trigger's direction should be learned from the press step, got %v", tt.lSign)
	}
	for i, a := range tt.Analysis {
		if a.Verdict != VerdictNormal {
			t.Fatalf("shared-axis trigger %d: %v %q", i, a.Verdict, a.Headline)
		}
	}
	res := tt.Result(t0, false)
	if !strings.Contains(strings.Join(res.Details, " "), "share one axis") {
		t.Fatal("result must explain that independent measurement is impossible")
	}
}

func TestTriggerResistanceCleanup(t *testing.T) {
	act := &fakeActuator{cfg: device.DefaultConfig()}
	tt := NewTriggerTest(dualsense.Descriptor().Inputs, true, act, nil)
	if !tt.StartResistance(t0) || len(act.cmds) != 1 || act.cmds[0].Config.LeftTrigger.Mode != device.TriggerModeResistance {
		t.Fatalf("resistance should be applied once")
	}
	tt.Tick(t0.Add(ResistanceTestDuration))
	if tt.ResistanceActive || len(act.cmds) != 2 || act.cmds[1].Config.LeftTrigger != act.cfg.LeftTrigger {
		t.Fatalf("resistance must be restored to the user's trigger settings after the time limit")
	}
}

func TestHighRateSamplesAreUsed(t *testing.T) {
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.Start(t0)
	now := t0
	for now.Before(st.StepEnd.Add(tick)) {
		batch := make([]device.RawSample, 10) // 10 reports per 40 ms = 250 Hz
		for k := range batch {
			batch[k] = device.RawSample{At: now.Add(time.Duration(k) * 4 * time.Millisecond), Sticks: [4]int32{128, 128, 128, 128}}
		}
		st.Feed(device.Telemetry{Samples: batch}, now)
		now = now.Add(tick)
	}
	a := st.Analysis[0]
	if a.RestA.N < 600 || a.RestA.RateHz < 200 {
		t.Fatalf("every report must be measured: n=%d rate=%.0f Hz", a.RestA.N, a.RestA.RateHz)
	}
}

// TestBacklogIsAttributedToTheRightStep: when the UI falls behind, a batch of
// queued reports must not end the resting step early ("too few readings").
func TestBacklogIsAttributedToTheRightStep(t *testing.T) {
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.Start(t0)
	var batch []device.RawSample
	for at := t0; at.Before(t0.Add(4 * time.Second)); at = at.Add(tick) {
		batch = append(batch, device.RawSample{At: at, Sticks: [4]int32{128, 128, 128, 128}})
	}
	st.Feed(device.Telemetry{Samples: batch}, t0.Add(4*time.Second))
	if st.Step != StickExplain || st.Analysis[0].Verdict != VerdictNormal || st.Analysis[0].RestA.N < 60 {
		t.Fatalf("backlog must be measured in step 1: step=%v verdict=%v n=%d", st.Step, st.Analysis[0].Verdict, st.Analysis[0].RestA.N)
	}
}
