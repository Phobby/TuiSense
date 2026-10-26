package diag

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
)

func TestV2MedianAndBoundedRestWindow(t *testing.T) {
	var rest restStats
	for i := 0; i < 10000; i++ {
		rest.add(device.RawSample{At: t0.Add(time.Duration(i) * time.Millisecond)}, [2]float64{128, 128}, [2]float64{.03, .04})
	}
	summary := summarizeRest(rest, dualsense.Descriptor().Inputs, 0)
	if summary.N != 10000 || math.Abs(summary.MedianDevPct-5) > 1e-9 || math.Abs(summary.OffsetPct-5) > 1e-9 || summary.NoisePct > 1e-9 {
		t.Fatalf("incorrect statistics: %+v", summary)
	}
	if len(rest.deviations) != 4096 {
		t.Fatal("median storage must be bounded")
	}
}
func TestV2CircularityDescribesSquareWithoutFault(t *testing.T) {
	st := NewStickTest(dualsense.Descriptor().Inputs, nil)
	st.StartCircularity(t0)
	for i := 0; i < 720 && st.Running(); i++ {
		angle := 2 * math.Pi * float64(i) / 120
		x, y := math.Cos(angle), math.Sin(angle)
		scale := 1 / math.Max(math.Abs(x), math.Abs(y))
		var raw [4]int32
		for k := 0; k < 2; k++ {
			raw[k*2] = int32(denormBipolar(st.Layout.StickAxis(k*2), x*scale))
			raw[k*2+1] = int32(denormBipolar(st.Layout.StickAxis(k*2+1), y*scale))
		}
		st.Feed(device.Telemetry{Raw: device.RawInput{Valid: true, Sticks: raw}}, t0.Add(time.Duration(i)*10*time.Millisecond))
	}
	result := st.Result(t0, false)
	if result == nil || result.Status != Passed || strings.Contains(result.Summary, "drift") {
		t.Fatalf("square gate should be descriptive: %+v", result)
	}
	stats := st.Travel[0].Circularity()
	if stats.Covered < 14 || stats.RadiusSpreadPct < 10 {
		t.Fatalf("expected observed square shape: %+v", stats)
	}
	st.StartCircularity(t0)
	st.Tick(t0.Add(13 * time.Second))
	if got := st.Result(t0, false); got == nil || got.Status != Incomplete {
		t.Fatalf("no rotation is incomplete: %+v", got)
	}
}
func TestV2AxialAndRadialPreview(t *testing.T) {
	raw := [2]float64{.06, .06}
	if PreviewDeadzone(raw, 8).Ignored || !PreviewAxialDeadzone(raw, 8).Ignored {
		t.Fatal("radial and axial must differ on diagonals")
	}
	v := PreviewAxialDeadzone([2]float64{.5, -1}, 20)
	if math.Abs(v.Effective[0]-.375) > 1e-12 || v.Effective[1] != -1 || raw != [2]float64{.06, .06} {
		t.Fatalf("incorrect preview %+v", v)
	}
}
func TestV2MappingChangeResetsAndCleansEffects(t *testing.T) {
	desc := dualsense.Descriptor()
	ctx := DeviceContext{Key: "fake", Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true}, Simulated: true}
	act := &fakeActuator{cfg: device.DefaultConfig()}
	s := NewSession(ctx, act, nil, nil, nil, t0)
	s.Haptics.Play("both", t0)
	s.Results[CatSticks] = Result{Category: CatSticks, Status: Warning}
	ctx.Desc.Inputs.TriggerKind = device.TriggerDigital
	s.SetContext(ctx)
	if s.Haptics.Running != nil || len(s.Results) != 0 || s.Triggers.Kind != device.TriggerDigital || len(s.Events) != 1 {
		t.Fatal("old mapping/results must not survive a change")
	}
	last := act.cmds[len(act.cmds)-1]
	if last.Type != device.CmdSetHaptics || last.Config.Haptics.LeftMotor != 0 {
		t.Fatal("mapping change must clean effects")
	}
}
func TestV2ResultTimestampAndEventBuffer(t *testing.T) {
	desc := dualsense.Descriptor()
	s := NewSession(DeviceContext{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true}}, nil, nil, nil, nil, t0)
	s.Sticks.Start(t0)
	feedStep(s.Sticks, t0, func(int) [4]int32 { return [4]int32{128, 128, 128, 128} })
	s.collect(t0.Add(4 * time.Second))
	at := s.Results[CatSticks].At
	s.collect(t0.Add(8 * time.Second))
	if !s.Results[CatSticks].At.Equal(at) {
		t.Fatal("finished timestamp must not move each frame")
	}
	for i := 0; i < 300; i++ {
		s.record("test", "bounded", t0)
	}
	if len(s.Events) != 100 {
		t.Fatal("events must be bounded")
	}
}
func TestV2ButtonReportTransitionsAndSkippedStatus(t *testing.T) {
	layout := device.InputLayout{Buttons: []device.ButtonDef{{ID: device.ButtonSouth, Label: "A"}}}
	bt := NewButtonTest(layout, nil)
	pressed := device.ButtonSet(0).With(device.ButtonSouth)
	bt.Feed(device.Telemetry{Samples: []device.RawSample{{At: t0, Buttons: pressed}, {At: t0.Add(time.Millisecond)}, {At: t0.Add(2 * time.Millisecond), Buttons: pressed}}}, t0)
	if bt.Counts[device.ButtonSouth] != 2 {
		t.Fatal("must count transitions within the batch")
	}
	bt.StartGuided()
	bt.SkipCurrent()
	if bt.Result(t0, false).Status != Incomplete {
		t.Fatal("skipped is not a warning")
	}
}
func TestV2HapticsLateTickDoesNotReplayExpiredPulses(t *testing.T) {
	act := &fakeActuator{cfg: device.DefaultConfig()}
	act.cfg.Haptics.LeftMotor = 200
	ht := NewHapticTest(dualsense.Descriptor().Capabilities, act)
	ht.Play("pulse", t0)
	ht.Tick(t0.Add(time.Minute))
	if ht.Running != nil || len(act.cmds) != 2 {
		t.Fatalf("late tick must only stop: %+v", act.cmds)
	}
	last := act.cmds[1]
	if last.Type != device.CmdSetHaptics || last.Config.Haptics.LeftMotor != 0 || act.cfg.Haptics.LeftMotor != 200 {
		t.Fatal("stop must silence effect without changing user configuration")
	}
}
func TestV2HistoryPrivacyCompatibilityAndClear(t *testing.T) {
	store := HistoryStore{Dir: t.TempDir()}
	report := Report{Algorithm: AlgorithmVersion, Mapping: "map", Method: "3s", Version: "test", GeneratedAt: t0, Device: ReportDevice{Model: "controller", ModelID: "test", Connection: "USB"}, Entries: []ReportEntry{{Status: Passed, Result: &Result{Category: CatSticks, Status: Passed, Measurements: []Measurement{{"offset", 2, "%"}}}}}}
	prev, err := store.Snapshot(report, "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(prev); err != nil {
		t.Fatal(err)
	}
	current, err := store.Snapshot(report, "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	current.Results[0].Measurements = []Measurement{{"offset", 3, "%"}}
	lines := strings.Join(CompareHistory(prev, current), "\n")
	if !strings.Contains(lines, "+1.000") || !strings.Contains(lines, "do not establish") {
		t.Fatal(lines)
	}
	current.Mapping = "new"
	if !strings.Contains(CompareHistory(prev, current)[0], "not directly comparable") {
		t.Fatal("mapping warning required")
	}
	data, _ := os.ReadFile(filepath.Join(store.Dir, "history.json"))
	if bytes.Contains(data, []byte("AA:BB")) {
		t.Fatal("identity leaked")
	}
	other := HistoryStore{Dir: t.TempDir()}
	different, _ := other.Snapshot(report, "AA:BB:CC:DD:EE:FF")
	if different.Identity == prev.Identity {
		t.Fatal("identities must be installation-specific")
	}
	if err = store.Clear(); err != nil {
		t.Fatal(err)
	}
	entries, err := store.Load()
	if err != nil || len(entries) != 0 {
		t.Fatal("clear must remove snapshots")
	}
}
func TestV2MarkdownJSONAndEmbeddedRedaction(t *testing.T) {
	desc := dualsense.Descriptor()
	s := NewSession(DeviceContext{Desc: desc, Caps: desc.Capabilities, Simulated: true, Info: device.Info{Connected: true, MACAddress: "AA:BB:CC:DD:EE:FF"}}, nil, nil, nil, nil, t0)
	s.Results[CatSticks] = Result{Category: CatSticks, Status: Incomplete, Summary: "serial: secret123 at AA:BB:CC:DD:EE:FF", At: t0, Measurements: []Measurement{{"X | Y", 3, "%"}}}
	report := BuildReport(s, "test", false, t0)
	var md, js bytes.Buffer
	if err := report.WriteMarkdown(&md); err != nil {
		t.Fatal(err)
	}
	if err := report.WriteJSON(&js); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{md.String(), js.String()} {
		if strings.Contains(text, "secret123") || strings.Contains(text, "AA:BB") {
			t.Fatalf("identifier leak: %s", text)
		}
		if text == md.String() && (!strings.Contains(text, "Eksik test") || !strings.Contains(text, "SİMÜLASYON")) {
			t.Fatal("coverage and simulation must be explicit")
		}
	}
	var parsed Report
	if err := json.Unmarshal(js.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Entries[0].Status != Incomplete {
		t.Fatal("status must survive report round-trip")
	}
	var technical bytes.Buffer
	_ = report.WriteTechnicalMarkdown(&technical)
	if !strings.Contains(technical.String(), `X \| Y`) {
		t.Fatal("markdown table separators must be escaped")
	}
}

func TestV2MissingAxisRangesAreUnavailable(t *testing.T) {
	desc := dualsense.Descriptor()
	desc.Inputs.StickRange = device.AxisRange{}
	desc.Inputs.StickAxes = [4]device.AxisRange{}
	desc.Inputs.TriggerRange = device.AxisRange{}
	desc.Inputs.TriggerAxes = [2]device.AxisRange{}
	plan := Plan(DeviceContext{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true}})
	if plan[CatSticks].Status != Unavailable || plan[CatTriggers].Status != Unavailable {
		t.Fatal("invalid mappings must not produce synthetic centered measurements")
	}
}
func TestV2SkippedSlowTriggerTestIsIncomplete(t *testing.T) {
	tt := NewTriggerTest(dualsense.Descriptor().Inputs, false, nil, nil)
	tt.Start(t0)
	trigSim(tt, t0, func(step TriggerStep, k int) ([2]int32, device.ButtonSet) {
		switch step {
		case TriggerPressL:
			return [2]int32{255, 0}, 0
		case TriggerPressR:
			return [2]int32{0, 255}, 0
		}
		return [2]int32{}, 0
	})
	if result := tt.Result(t0, false); result.Status != Incomplete {
		t.Fatalf("unobserved slow movement must be incomplete: %+v", result)
	}
}
