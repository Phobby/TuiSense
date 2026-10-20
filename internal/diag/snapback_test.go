package diag

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
)

// snapStream generates 250 Hz samples of the left stick: hold at the edge in
// direction ang, release over fall, overshoot past center by `over`, settle.
func snapStream(from time.Time, ang, over float64, fall time.Duration) ([]device.RawSample, time.Time) {
	const dt = 4 * time.Millisecond
	l := dualsense.Descriptor().Inputs
	raw := func(p float64) [4]int32 {
		x := denormBipolar(l.StickAxis(0), p*math.Cos(ang))
		y := denormBipolar(l.StickAxis(1), p*math.Sin(ang))
		return [4]int32{int32(math.Round(x)), int32(math.Round(y)), 128, 128}
	}
	var out []device.RawSample
	at := from
	add := func(p float64) {
		out = append(out, device.RawSample{At: at, Sticks: raw(p)})
		at = at.Add(dt)
	}
	for i := 0; i < 50; i++ { // 200 ms at the edge
		add(1)
	}
	steps := int(fall / dt)
	for i := 1; i <= steps; i++ {
		add(1 - float64(i)/float64(steps))
	}
	for i := 0; i < 5; i++ { // 20 ms swing to the opposite side
		add(-over * math.Sin(math.Pi*float64(i+1)/6))
	}
	for i := 0; i < 3; i++ {
		add(over / 4)
	}
	for i := 0; i < 100; i++ { // 400 ms at rest
		add(0)
	}
	return out, at
}

func runSnap(t *testing.T, over float64, fall time.Duration, releases int) *SnapbackTest {
	t.Helper()
	sb := NewSnapbackTest(dualsense.Descriptor().Inputs, nil)
	sb.Start(t0)
	now := t0
	for k := 0; k < releases; k++ {
		batch, end := snapStream(now, float64(k)*math.Pi/2, over, fall)
		for i := 0; i < len(batch); i += 10 { // delivered in 40 ms polls
			j := min(i+10, len(batch))
			sb.Feed(device.Telemetry{Samples: batch[i:j]}, batch[j-1].At)
		}
		now = end
	}
	sb.Finish()
	return sb
}

func TestSnapbackDetected(t *testing.T) {
	sb := runSnap(t, 0.14, 12*time.Millisecond, 5)
	a := sb.Analyze(0)
	if a.Events != 5 || a.Verdict != VerdictSnapback || a.Status() != Warning {
		t.Fatalf("14%% overshoot should be a noticeable snapback, got %+v", a)
	}
	if math.Abs(a.MaxOvershootPct-14) > 2 || a.OverDeadzone != 5 || a.RateHz < 200 {
		t.Fatalf("overshoot %.1f%% (want ≈14), over=%d, rate %.0f", a.MaxOvershootPct, a.OverDeadzone, a.RateHz)
	}
	if a.MeanSettleMs <= 0 || a.MeanSettleMs > 100 {
		t.Fatalf("settle time should be measured, got %.0f ms", a.MeanSettleMs)
	}
	if !strings.Contains(strings.Join(a.Explanation, " "), "not a defect") {
		t.Fatal("snapback must be explained as common mechanics, not a defect")
	}
	if r := sb.Result(t0, false); r.Status != Warning || r.Status == Failed {
		t.Fatalf("result status %v", r.Status)
	}
	// The right stick was never touched: not tested, and not part of the status.
	if b := sb.Analyze(1); b.Events != 0 || b.Verdict != VerdictNone {
		t.Fatalf("untouched stick must not get a verdict, got %+v", b)
	}
	// A larger preview deadzone hides the same overshoot.
	sb.PreviewDeadzone = 15
	if a := sb.Analyze(0); a.Verdict != VerdictNormal || a.OverDeadzone != 0 {
		t.Fatalf("15%% preview should hide a 14%% overshoot, got %v", a.Verdict)
	}
}

func TestSnapbackSmallOvershootIsNormal(t *testing.T) {
	a := runSnap(t, 0.03, 12*time.Millisecond, 4).Analyze(0)
	if a.Verdict != VerdictNormal || a.MaxOvershootPct > 5 {
		t.Fatalf("3%% overshoot is clean, got %v %.1f%%", a.Verdict, a.MaxOvershootPct)
	}
}

func TestSnapbackSlowReturnIgnored(t *testing.T) {
	sb := runSnap(t, 0.14, 400*time.Millisecond, 4)
	a := sb.Analyze(0)
	if a.Events != 0 || a.SlowReleases != 4 {
		t.Fatalf("hand-guided returns are not snapback measurements: events=%d slow=%d", a.Events, a.SlowReleases)
	}
	if r := sb.Result(t0, false); r.Status != Inconclusive {
		t.Fatalf("no releases → inconclusive, got %v", r.Status)
	}
}

func TestSnapbackTooFewReleases(t *testing.T) {
	a := runSnap(t, 0.14, 12*time.Millisecond, 2).Analyze(0)
	if a.Verdict != VerdictInconclusive || !strings.HasPrefix(a.Headline, "Test incomplete") {
		t.Fatalf("two releases are not enough, got %v %q", a.Verdict, a.Headline)
	}
}

func TestSnapbackLowRateCannotClaimNormal(t *testing.T) {
	// One sample per 40 ms poll (a backend without per-report data).
	sb := NewSnapbackTest(dualsense.Descriptor().Inputs, nil)
	sb.Start(t0)
	now := t0
	for k := 0; k < 4; k++ {
		batch, end := snapStream(now, 0, 0.02, 12*time.Millisecond)
		for i := 0; i < len(batch); i += 10 {
			sb.Feed(device.Telemetry{Samples: batch[i : i+1]}, batch[i].At)
		}
		now = end
	}
	sb.Finish()
	if a := sb.Analyze(0); a.Events > 0 && a.Verdict == VerdictNormal {
		t.Fatalf("a low sample rate must not report a clean result, got %+v", a)
	}
}

func TestSnapbackSessionIntegration(t *testing.T) {
	s := NewSession(dsContext(), &fakeActuator{cfg: device.DefaultConfig()}, nil, nil, nil, t0)
	if s.Snapback == nil || !s.Avail[CatSticks].Available() {
		t.Fatal("snapback test should be available wherever sticks are")
	}
	for _, c := range s.Visible() {
		if c == "snapback" {
			t.Fatal("snapback is a sub-test of the stick category, not its own category")
		}
	}
	s.Snapback.Start(t0)
	if s.Status(CatSticks) != Testing {
		t.Fatal("collecting should show as testing")
	}
	s.Feed(device.Telemetry{}, device.Info{Connected: false, Connection: device.ConnUSB}, t0.Add(time.Second))
	if s.Snapback.Running() {
		t.Fatal("disconnect must stop collecting")
	}
	s.Sticks.PreviewDeadzone = 15
	s.Tick(t0.Add(2 * time.Second))
	if s.Snapback.PreviewDeadzone != 15 {
		t.Fatal("the deadzone preview is shared with the stick test")
	}
}

func TestStickCategoryMergesDeadzoneAndSnapbackResults(t *testing.T) {
	s := NewSession(dsContext(), &fakeActuator{cfg: device.DefaultConfig()}, nil, nil, nil, t0)
	s.Snapback = runSnap(t, 0.14, 12*time.Millisecond, 4)
	s.Tick(t0)
	r, ok := s.Results[CatSticks]
	if !ok || r.Status != Warning || !strings.Contains(r.Summary, "snapback") {
		t.Fatalf("snapback alone should produce the stick result, got %+v", r)
	}
	s.Sticks.Start(t0)
	feedStep(s.Sticks, t0, func(int) [4]int32 { return [4]int32{128, 128, 128, 128} })
	s.Tick(t0.Add(5 * time.Second))
	r = s.Results[CatSticks]
	if !strings.Contains(r.Summary, "rest close to the center") || !strings.Contains(r.Summary, "Snapback: ") {
		t.Fatalf("both sub-tests must appear in one result, got %q", r.Summary)
	}
	if r.Status != Warning {
		t.Fatalf("the merged status is the worse of both, got %v", r.Status)
	}
}
