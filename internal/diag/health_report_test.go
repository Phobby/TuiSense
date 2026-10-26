package diag

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

func reportEntry(t *testing.T, r Report, category Category) ReportEntry {
	t.Helper()
	for _, entry := range r.Entries {
		if entry.Category == category {
			return entry
		}
	}
	t.Fatalf("missing report category %s", category)
	return ReportEntry{}
}
func healthSession() *Session { return NewSession(dsContext(), nil, nil, nil, nil, t0) }

func TestHealthPartialAndUnsupportedAreNotPasses(t *testing.T) {
	s := healthSession()
	report := BuildReport(s, "test", false, t0)
	if report.SchemaVersion != ReportSchemaVersion || !report.Coverage.Partial {
		t.Fatal("fresh session must produce a versioned partial report")
	}
	for _, entry := range report.Entries {
		if entry.Status == Passed {
			t.Fatalf("unperformed test passed: %s", entry.Category)
		}
		if len(entry.Assessments) == 0 {
			t.Fatalf("%s needs an explanation even when untested", entry.Category)
		}
	}
	if len(report.PotentialIssues()) != 0 {
		t.Fatal("unsupported and untested inputs are not defects")
	}
}

func TestHealthCompletedCoverageWithoutArtificialScore(t *testing.T) {
	layout := device.InputLayout{Buttons: []device.ButtonDef{{ID: device.ButtonSouth, Label: "A"}}}
	desc := device.Descriptor{ModelID: "test.pad", DisplayName: "Minimal Test Pad", Inputs: layout}
	context := DeviceContext{Desc: desc, Info: device.Info{Connected: true, EventDriven: true, Connection: device.ConnUSBWired}, Simulated: true}
	s := NewSession(context, nil, nil, nil, nil, t0)
	s.Buttons.StartGuided()
	s.Feed(device.Telemetry{Buttons: device.ButtonSet(0).With(device.ButtonSouth), PacketsRx: 1}, context.Info, t0)
	s.Feed(device.Telemetry{PacketsRx: 2}, context.Info, t0.Add(4*time.Second))
	report := BuildReport(s, "test", false, t0.Add(4*time.Second))
	if report.Coverage.Partial || report.Coverage.Completed != 2 || report.Coverage.Warnings != 0 {
		t.Fatalf("unexpected coverage: %+v", report.Coverage)
	}
	text := report.Text()
	for _, section := range []string{"Genel Sonuç", "Test Sonuçları", "Önerilen İşlemler", "Son Değerlendirme"} {
		if !strings.Contains(text, section) {
			t.Fatal(section)
		}
	}
	if strings.Contains(strings.ToLower(text), "health: 100") {
		t.Fatal("a coverage report must not become a hardware score")
	}
	if !strings.Contains(text, "SİMÜLASYON") {
		t.Fatal("simulated observations must remain marked")
	}
}

func TestHealthPreliminaryRestIsNotFullStickCompletion(t *testing.T) {
	s := healthSession()
	s.Sticks.Start(t0)
	feedStep(s.Sticks, t0, func(int) [4]int32 { return [4]int32{128, 128, 128, 128} })
	s.collect(t0.Add(4 * time.Second))
	entry := reportEntry(t, BuildReport(s, "test", false, t0.Add(4*time.Second)), CatSticks)
	if entry.Completion != CompletionPartial || entry.Status != Incomplete {
		t.Fatalf("rest alone is incomplete: %+v", entry)
	}
	for _, assessment := range entry.Assessments {
		if assessment.Status == PossibleIssue || assessment.Status == Failed {
			t.Fatal("a centered preliminary reading cannot establish a fault")
		}
	}
}

func TestHealthRepeatedDriftHasEvidenceImpactAndFollowUp(t *testing.T) {
	s := healthSession()
	s.Sticks.Start(t0)
	values := func(int) [4]int32 { return [4]int32{140, 128, 128, 128} }
	now := feedStep(s.Sticks, t0, values)
	now = runMovement(t, s.Sticks, now)
	feedStep(s.Sticks, now, values)
	s.collect(now.Add(3 * time.Second))
	report := BuildReport(s, "test", false, now.Add(3*time.Second))
	entry := reportEntry(t, report, CatSticks)
	if entry.Completion != CompletionDone || len(entry.Assessments) != 2 {
		t.Fatalf("complete two-stick assessment expected: %+v", entry)
	}
	left := entry.Assessments[0]
	if left.Status != PossibleIssue || left.EvidenceKind != "possible_issue" || len(left.Evidence) < 6 || len(left.Actions) < 2 || left.GameplayImpact == "" || !strings.Contains(left.Basis, "not manufacturer") {
		t.Fatalf("explainable drift assessment missing: %+v", left)
	}
	if entry.Assessments[1].Status != Passed {
		t.Fatal("normal other stick must stay normal")
	}
	warnings := report.WarningsText()
	if !strings.Contains(warnings, "Sol çubuk") || !strings.Contains(warnings, "İlk merkez sapması") || !strings.Contains(warnings, "Öneri") {
		t.Fatal(warnings)
	}
	if strings.Contains(warnings, "Sağ çubuk") {
		t.Fatal("warnings-only copy must omit a normally functioning stick")
	}
}

func TestHealthSingleOffsetRemainsUncertain(t *testing.T) {
	s := healthSession()
	s.Sticks.Start(t0)
	feedStep(s.Sticks, t0, func(int) [4]int32 { return [4]int32{140, 128, 128, 128} })
	s.collect(t0.Add(4 * time.Second))
	entry := reportEntry(t, BuildReport(s, "test", false, t0.Add(4*time.Second)), CatSticks)
	for _, a := range entry.Assessments {
		if a.Status == PossibleIssue || a.Status == Failed {
			t.Fatal("a single resting reading cannot prove drift")
		}
	}
}

func TestHealthTriggerMappingAndIncompleteTravel(t *testing.T) {
	s := healthSession()
	s.Triggers.Start(t0)
	trigSim(s.Triggers, t0, func(step TriggerStep, k int) ([2]int32, device.ButtonSet) {
		switch step {
		case TriggerPressL:
			return [2]int32{255, 0}, 0
		case TriggerPressR:
			return [2]int32{0, 180}, 0
		case TriggerSlowL:
			return [2]int32{slowPress(k), 0}, 0
		case TriggerSlowR:
			return [2]int32{0, min(180, slowPress(k))}, 0
		}
		return [2]int32{}, 0
	})
	s.collect(t0.Add(2 * time.Minute))
	entry := reportEntry(t, BuildReport(s, "test", false, t0.Add(2*time.Minute)), CatTriggers)
	if entry.Completion != CompletionPartial || entry.Assessments[1].Status != Incomplete {
		t.Fatal("partial press is an incomplete protocol")
	}
	right := entry.Assessments[1]
	if !strings.Contains(right.Reason, "not a hardware failure") || !strings.Contains(strings.Join(right.Actions, " "), "mapping") {
		t.Fatalf("mapping follow-up missing: %+v", right)
	}
	if right.Status == Failed || right.Status == PossibleIssue {
		t.Fatal("incomplete press is not evidence of a defective trigger")
	}
}

func TestHealthDigitalTriggerReportDoesNotInventAnalogTests(t *testing.T) {
	s := healthSession()
	layout := s.Ctx.Desc.Inputs
	layout.TriggerKind = device.TriggerDigital
	layout.TriggerRange = device.AxisRange{Min: 0, Max: 1}
	layout.TriggerAxes = [2]device.AxisRange{}
	s.Triggers = NewTriggerTest(layout, false, nil, nil)
	s.Triggers.Start(t0)
	trigSim(s.Triggers, t0, func(step TriggerStep, k int) ([2]int32, device.ButtonSet) {
		if step == TriggerPressL && k < 5 {
			return [2]int32{}, device.ButtonSet(0).With(device.ButtonL2Digital)
		}
		if step == TriggerPressR && k < 5 {
			return [2]int32{}, device.ButtonSet(0).With(device.ButtonR2Digital)
		}
		return [2]int32{}, 0
	})
	s.collect(t0.Add(time.Minute))
	entry := reportEntry(t, BuildReport(s, "test", false, t0.Add(time.Minute)), CatTriggers)
	for _, a := range entry.Assessments {
		if !strings.Contains(a.Basis, "Digital on/off") || strings.Contains(a.Basis, "95%") {
			t.Fatal("digital mapping must not inherit analog expectations")
		}
	}
}

func TestHealthCancelledProtocolRetainsPartialEvidence(t *testing.T) {
	s := healthSession()
	s.Sticks.Start(t0)
	s.Sticks.Feed(device.Telemetry{Raw: device.RawInput{Valid: true, Sticks: [4]int32{130, 128, 128, 128}}}, t0.Add(time.Second))
	s.Select(CatHealthReport)
	s.GenerateHealthReport(t0.Add(time.Second))
	entry := reportEntry(t, *s.FinalReport, CatSticks)
	if entry.Status != Incomplete || entry.Completion != CompletionPartial || len(entry.Result.Measurements) == 0 {
		t.Fatalf("canceled observations must remain partial: %+v", entry)
	}
}

func TestHealthGuidedFinishAutomaticallyShowsPartialReport(t *testing.T) {
	s := healthSession()
	s.StartGuided()
	for s.Guided != nil {
		s.NextGuided()
	}
	if s.Active != CatHealthReport || s.FinalReport == nil {
		t.Fatal("guided completion must show the final health report")
	}
	if !s.FinalReport.Coverage.Partial {
		t.Fatal("skipping steps must not establish coverage")
	}
}

func TestHealthCapabilityDrivenMSIReport(t *testing.T) {
	input := evdev.InputDevice{Vendor: 0x045e, Product: 0x028e}
	desc := msigc20.Describe(gamepad.Mapping{}, input, false)
	s := NewSession(DeviceContext{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true, Connection: device.ConnUSBWired}}, nil, nil, nil, nil, t0)
	report := BuildReport(s, "test", false, t0)
	for _, category := range []Category{CatBattery, CatMotion, CatAudioIn, CatTouchpad, CatFirmware} {
		entry := reportEntry(t, report, category)
		if entry.Completion != CompletionUnsupported || entry.Assessments[0].Status != Unsupported {
			t.Fatalf("MSI %s must be unsupported, not defective", category)
		}
	}
	if len(report.PotentialIssues()) != 0 {
		t.Fatal("absent MSI capabilities are not warnings")
	}
}

func TestHealthMissingMeasurementsAndMultipleWarnings(t *testing.T) {
	s := healthSession()
	s.Avail[CatAudioIn] = Availability{Category: CatAudioIn, Status: NotTested}
	s.Results[CatAudioIn] = Result{Category: CatAudioIn, Status: Failed, Summary: "Audio endpoint permission error", Details: []string{"access denied"}, At: t0}
	s.Results[CatHaptics] = Result{Category: CatHaptics, Status: Warning, Summary: "Pattern was not felt", UserConfirmed: true, At: t0}
	s.Results[CatButtons] = Result{Category: CatButtons, Status: Warning, Summary: "Long button state", Measurements: []Measurement{{"hold", 7, "s"}}, At: t0}
	report := BuildReport(s, "test", false, t0)
	audio := reportEntry(t, report, CatAudioIn)
	if audio.Assessments[0].Status != Inconclusive || audio.Completion == CompletionDone {
		t.Fatal("backend error without hardware measurements must not prove a physical defect")
	}
	if len(report.PotentialIssues()) < 2 {
		t.Fatal("all simultaneous warnings must remain visible")
	}
	for _, a := range audio.Assessments {
		if len(a.Evidence) > 0 {
			t.Fatal("missing microphone measurements must stay missing")
		}
	}
}

func TestHealthJSONMarkdownTextPrivacyAndWriterFailure(t *testing.T) {
	s := healthSession()
	s.Ctx.Info.MACAddress = "AA:BB:CC:DD:EE:FF"
	s.Ctx.Info.ID = "SECRET-1234"
	s.Results[CatButtons] = Result{Category: CatButtons, Status: Warning, Summary: "serial: SECRET-1234 on AA:BB:CC:DD:EE:FF", Measurements: []Measurement{{"X | Y", 2, "%"}, {"unknown", math.NaN(), ""}}, At: t0}
	report := BuildReport(s, "test", false, t0)
	if s.Results[CatButtons].Summary == report.Entries[1].Result.Summary {
		t.Fatal("sanitizing must not mutate original results")
	}
	var md, js bytes.Buffer
	if err := report.WriteTechnicalMarkdown(&md); err != nil {
		t.Fatal(err)
	}
	if err := report.WriteJSON(&js); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{report.Text(), report.WarningsText(), md.String(), js.String()} {
		for _, secret := range []string{"SECRET-1234", "AA:BB:CC:DD:EE:FF", "NaN"} {
			if strings.Contains(text, secret) {
				t.Fatalf("sensitive/invalid value leaked: %s", secret)
			}
		}
	}
	if !strings.Contains(md.String(), `X \| Y`) {
		t.Fatal("Markdown measurements must escape table columns")
	}
	var decoded Report
	if err := json.Unmarshal(js.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != ReportSchemaVersion || len(decoded.Entries[1].Assessments) == 0 {
		t.Fatal("structured assessment did not round-trip")
	}
	if err := report.WriteText(failingReportWriter{}); err == nil {
		t.Fatal("writer failures must propagate")
	}
	dir := t.TempDir()
	_, _, err := report.Export(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatal("TXT, Markdown and JSON must all be exported")
	}
	if _, _, err = report.Export(dir); err != nil {
		t.Fatal("repeat export must not overwrite or fail on a saved report")
	}
}

type failingReportWriter struct{}

func (failingReportWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

var _ io.Writer = failingReportWriter{}

func TestHealthHistoryKeepsOriginalReportAndLegacyLimits(t *testing.T) {
	s := healthSession()
	s.Ctx.Info.FirmwareVersion = "observed-version"
	report := BuildReport(s, "test", false, t0)
	store := HistoryStore{Dir: t.TempDir()}
	snapshot, err := store.Snapshot(report, "local-device")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(snapshot); err != nil {
		t.Fatal(err)
	}
	entries, err := store.Load()
	if err != nil || len(entries) != 1 {
		t.Fatal(err)
	}
	stored := entries[0].StoredReport()
	if stored.Text() != report.Text() {
		t.Fatal("saved reports must retain original coverage and assessment")
	}
	current := snapshot
	current.Algorithm = "other-algorithm"
	if !strings.Contains(CompareHistory(snapshot, current)[0], "not directly comparable") {
		t.Fatal("changed algorithms cannot establish a trend")
	}
	legacy := HistoryEntry{Model: "Old pad", At: t0, Results: []Result{{Category: CatSticks, Status: Passed, Measurements: []Measurement{{"offset", 1, "%"}}}}}
	old := legacy.StoredReport()
	if old.Coverage.Completed != 0 || !old.Coverage.Partial || old.Entries[0].Status != Inconclusive {
		t.Fatal("legacy snapshots cannot invent protocol completion")
	}
	raw, err := os.ReadFile(filepath.Join(store.Dir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("local-device")) {
		t.Fatal("history must use a private local identity")
	}
}

func TestHealthReportGenerationDoesNotActuate(t *testing.T) {
	actuator := &fakeActuator{cfg: device.DefaultConfig()}
	s := NewSession(dsContext(), actuator, nil, nil, nil, t0)
	config := actuator.cfg
	report := BuildReport(s, "test", false, t0)
	_ = report.Text()
	_ = report.WarningsText()
	s.GenerateHealthReport(t0)
	if len(actuator.cmds) != 0 || actuator.cfg != config {
		t.Fatal("report generation must never change the real controller configuration")
	}
}

func TestHealthDeadzoneAndFirmwareReportsUseOnlyObservedData(t *testing.T) {
	s := healthSession()
	before := BuildReport(s, "test", false, t0)
	if reportEntry(t, before, CatDeadzone).Result != nil || reportEntry(t, before, CatFirmware).Result != nil {
		t.Fatal("unavailable observations must not be invented")
	}
	s.Sticks.Feed(device.Telemetry{Raw: device.RawInput{Valid: true, Sticks: [4]int32{140, 128, 128, 128}}}, t0)
	s.Ctx.Info.FirmwareVersion = "0x0630"
	after := BuildReport(s, "test", false, t0)
	preview := reportEntry(t, after, CatDeadzone)
	if preview.Completion != CompletionDone || preview.Status == Passed || !strings.Contains(preview.Result.Summary, "game input is unchanged") {
		t.Fatal("preview must remain separate from hardware health")
	}
	firmware := reportEntry(t, after, CatFirmware)
	if firmware.Completion != CompletionDone || !strings.Contains(firmware.Result.Summary, "0x0630") {
		t.Fatal("actual firmware metadata should be reportable")
	}
}

// A device's appearance or UX mode is intentionally absent from assessment rules.
func TestHealthReportUsesDeclaredCapabilitiesForDualSense(t *testing.T) {
	desc := dualsense.Descriptor()
	s := healthSession()
	for _, category := range []Category{CatSticks, CatTriggers, CatButtons, CatTouchpad, CatMotion, CatBattery, CatFirmware} {
		if !s.Avail[category].Available() {
			t.Fatalf("%s should expose %s", desc.DisplayName, category)
		}
	}
	report := BuildReport(s, "test", false, t0).Scope(CatTriggers)
	if len(report.Entries) != 1 || report.Entries[0].Category != CatTriggers || strings.Contains(report.Text(), "Analog sticks:") {
		t.Fatal("individual reports must be independently scoped")
	}
}

func TestHealthNormalFullStickProtocol(t *testing.T) {
	s := healthSession()
	s.Sticks.Start(t0)
	center := func(int) [4]int32 { return [4]int32{128, 128, 128, 128} }
	now := feedStep(s.Sticks, t0, center)
	now = runMovement(t, s.Sticks, now)
	feedStep(s.Sticks, now, center)
	s.collect(now.Add(3 * time.Second))
	entry := reportEntry(t, BuildReport(s, "test", false, now.Add(3*time.Second)), CatSticks)
	if entry.Completion != CompletionDone || entry.Status != Passed {
		t.Fatalf("complete normal protocol should pass: %+v", entry)
	}
	for _, a := range entry.Assessments {
		if a.Status != Passed {
			t.Fatal("normal stick should not produce a warning")
		}
	}
}

func TestHealthTwoStickWarningsRemainSeparate(t *testing.T) {
	s := healthSession()
	s.Sticks.Start(t0)
	values := func(int) [4]int32 { return [4]int32{140, 128, 140, 128} }
	now := feedStep(s.Sticks, t0, values)
	now = runMovement(t, s.Sticks, now)
	feedStep(s.Sticks, now, values)
	s.collect(now.Add(3 * time.Second))
	report := BuildReport(s, "test", false, now.Add(3*time.Second)).Scope(CatSticks)
	if report.Coverage.Warnings != 2 || len(report.PotentialIssues()) != 2 {
		t.Fatal("each affected component needs an independent explanation")
	}
}

func TestHealthConnectedEntryWithoutReportsDoesNotPass(t *testing.T) {
	s := healthSession()
	s.collect(t0.Add(5 * time.Second))
	entry := reportEntry(t, BuildReport(s, "test", false, t0.Add(5*time.Second)), CatConnection)
	if entry.Status == Passed || entry.Completion == CompletionDone {
		t.Fatal("connection metadata alone must not establish input continuity")
	}
	if len(entry.Result.Measurements) > 0 {
		t.Fatal("unavailable report frequency must not become a zero-valued measurement")
	}
}
