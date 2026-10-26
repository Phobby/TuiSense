package diag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/i18n"
)

func simpleFixture() Report {
	r := Report{Device: ReportDevice{Model: "Community Pad", Connection: "USB"}, GeneratedAt: time.Date(2026, 10, 26, 12, 0, 0, 0, time.Local), Algorithm: "algorithm-secret", Mapping: "mapping-secret", SchemaVersion: ReportSchemaVersion}
	r.Entries = []ReportEntry{
		{Category: CatSticks, Status: Warning, Completion: CompletionPartial, Assessments: []Assessment{
			{Component: "Left stick", Test: "snapback", Status: Passed, Evidence: []Measurement{{"Maximum overshoot", 1.3, "%"}}},
			{Component: "Right stick", Test: "snapback", Status: Warning, Evidence: []Measurement{{"Maximum overshoot", 69.8, "%"}, {"Average overshoot", 35.3, "%"}, {"Sudden releases", 5, ""}, {"Releases beyond preview", 3, ""}, {"Preview deadzone", 8, "%"}}},
		}},
		{Category: CatTriggers, Status: NotTested, Completion: CompletionNone},
		{Category: CatBattery, Status: Unavailable, Completion: CompletionUnavailable},
		{Category: CatButtons, Status: Passed, Completion: CompletionDone, Components: []ComponentCompletion{{"A", CompletionDone}, {"B", CompletionDone}}, Result: &Result{Category: CatButtons, Status: Passed}},
	}
	finalizeReport(&r)
	return r
}
func TestSimpleTurkishReportContract(t *testing.T) {
	r := simpleFixture()
	var md bytes.Buffer
	if err := r.WriteMarkdown(&md); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# TuiSense — Kontrolcü Sağlık Raporu", "## Genel Sonuç", "## Olası Sorunlar", "## Test Sonuçları", "## Önerilen İşlemler", "## Son Değerlendirme", "| Bileşen | Sonuç | Açıklama |", "26 Ekim 2026", "%69,8", "5 ani bırakmanın 3 tanesinde %8,0", "2/2 tuş algılandı.", "Test tamamlanmadı", "Kullanılamıyor"} {
		if !strings.Contains(md.String(), want) {
			t.Fatalf("missing %q:\n%s", want, md.String())
		}
	}
	for _, bad := range []string{"algorithm-secret", "mapping-secret", "Comparison basis", "Reason:", "Supporting evidence", "Event:"} {
		if strings.Contains(r.Text(), bad) {
			t.Fatalf("technical field leaked: %s", bad)
		}
	}
	if strings.Count(r.WarningsText(), "Ani bırakmada geri sekme") != 1 || strings.Contains(r.WarningsText(), "Sol çubuk") {
		t.Fatal("warning must isolate the affected component")
	}
	if strings.Contains(r.Text(), "**") || strings.Contains(r.Text(), "##") {
		t.Fatal("plain copy must not contain Markdown styling")
	}
	if len(strings.Fields(r.Text())) > 420 {
		t.Fatal("default report is too long")
	}
	actions := r.simpleActions()
	if len(actions) < 2 || !strings.Contains(actions[0], "Analog") || !strings.Contains(strings.Join(actions[1:], " "), "Tetik") {
		t.Fatalf("warning must precede missing tests: %v", actions)
	}
}
func TestSimpleLocaleIndependentAndScoped(t *testing.T) {
	r := simpleFixture()
	before := i18n.Current()
	defer i18n.Set(before)
	i18n.Set(i18n.English)
	englishUI := r.Text()
	i18n.Set(i18n.Turkish)
	if r.Text() != englishUI {
		t.Fatal("report language depends on UI mode")
	}
	report := r.Scope(CatTriggers).TestText()
	if !strings.Contains(report, "Test: Tetikler") || !strings.Contains(report, "Test edilmedi") || strings.Contains(report, "69,8") || strings.Contains(report, "Pil") {
		t.Fatal(report)
	}
	if strings.Contains(report, "Ölçümler:") {
		t.Fatal("missing measurements must not be invented")
	}
}
func TestTechnicalReportsBoundDuplicateEvents(t *testing.T) {
	r := simpleFixture()
	for i := 0; i < 1000; i++ {
		r.Events = append(r.Events, Event{At: r.GeneratedAt, Category: string(CatConnection), Message: "same-poll"})
	}
	for i := 0; i < 80; i++ {
		r.Events = append(r.Events, Event{At: r.GeneratedAt, Category: string(CatConnection), Message: fmt.Sprintf("unique-event-%d", i)})
	}
	output := r.TechnicalText()
	if !strings.Contains(output, "mapping-secret") || !strings.Contains(output, "Algoritma:") {
		t.Fatal("technical metadata missing")
	}
	if strings.Count(output, "unique-event-") != 32 || strings.Count(output, "same-poll") > 1 {
		t.Fatal("event output is unbounded")
	}
	var js bytes.Buffer
	if err := r.WriteJSON(&js); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(js.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Events) != 32 || len(r.Events) != 1080 {
		t.Fatal("export must bound events without changing original observations")
	}
	r.Events = r.Events[:1000]
	if strings.Count(r.TechnicalText(), "same-poll") != 1 {
		t.Fatal("duplicates were not collapsed")
	}
}
func TestSimpleTelemetryIsNotHardwareHealth(t *testing.T) {
	r := Report{Entries: []ReportEntry{{Category: CatBattery, Status: Passed, Completion: CompletionDone, Result: &Result{Measurements: []Measurement{{"Reported level", 5, "%"}}}}, {Category: CatFirmware, Status: Passed, Completion: CompletionDone, Result: &Result{}}}}
	finalizeReport(&r)
	text := r.Text()
	for _, want := range []string{"Normal gözlem", "pil sağlığı ölçülemiyor", "güncellik doğrulanmadı"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	if strings.Contains(text, "Olası Sorunlar") {
		t.Fatal("normal telemetry invented issues")
	}
}

func TestSnapbackAssessmentCapturesBothComponents(t *testing.T) {
	s := healthSession()
	s.Snapback.done = true
	s.Snapback.PreviewDeadzone = 8
	for i := 0; i < 5; i++ {
		s.Snapback.Events[0] = append(s.Snapback.Events[0], SnapEvent{OvershootPct: 1, Samples: 200})
		s.Snapback.Events[1] = append(s.Snapback.Events[1], SnapEvent{OvershootPct: 40, Samples: 200})
	}
	s.collect(t0)
	r := BuildReport(s, "test", false, t0)
	warnings := r.WarningsText()
	if !strings.Contains(warnings, "Sağ çubuk") || strings.Contains(warnings, "Sol çubuk") || !strings.Contains(warnings, "%40,0") || !strings.Contains(warnings, "Test tamamlanmadı") {
		t.Fatal(warnings)
	}
	entry := reportEntry(t, r, CatSticks)
	if len(entry.Assessments) != 2 {
		t.Fatalf("missing captured snapback assessments: %+v", entry.Assessments)
	}
	s.Snapback.Events[1] = nil
	if r.WarningsText() != warnings {
		t.Fatal("snapshot changed with live test")
	}
}

func TestUnsupportedCategoryDoesNotInventFaultOrAdvice(t *testing.T) {
	r := Report{Entries: []ReportEntry{{Category: CatBattery, Status: Unsupported, Completion: CompletionUnsupported}}}
	finalizeReport(&r)
	if !strings.Contains(r.TestText(), "Kullanılamıyor") || strings.Contains(r.TestText(), "Öneri:") || strings.Contains(r.Text(), "Olası Sorunlar") {
		t.Fatal(r.TestText())
	}
}
