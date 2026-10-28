package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/ui/views"
	"github.com/thebanri/limoni/core/buffer"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/uitest"
)

func TestHealthReportUIAndCopyActions(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	var copied []string
	a.Opts.CopyText = func(text string) error { copied = append(copied, text); return nil }
	page.GetByID("diag_generate_health_report").Click()
	if a.State.Diag.Active != diag.CatHealthReport || a.State.Diag.FinalReport == nil {
		t.Fatal("partial report must be available before testing")
	}
	page.GetByID("copy_full_report").Click()
	waitForCopy(t, a)
	if len(copied) != 1 || !strings.Contains(copied[0], "Son Değerlendirme") {
		t.Fatal("copy full report must include complete underlying content")
	}
	page.GetByID("copy_warnings_only").Click()
	waitForCopy(t, a)
	if !strings.Contains(copied[1], "TuiSense — Tanılama Uyarıları") || !strings.Contains(copied[1], "Önemli bir uyarı gözlenmedi") {
		t.Fatal("warnings-only copy must explain empty warning coverage")
	}
	page.GetByID("diag_tab_triggers").Click()
	page.GetByID("copy_test_report").Click()
	waitForCopy(t, a)
	if !strings.Contains(copied[2], "Test: Tetikler") || strings.Contains(copied[2], "Analog çubuklar") {
		t.Fatal("individual report copied unrelated tests")
	}
	page.GetByID("copy_test_summary").Click()
	waitForCopy(t, a)
	if !strings.Contains(copied[3], "Test edilmedi") {
		t.Fatal("unperformed test must remain explicit")
	}
	page.GetByID("diag_show_test_report").Click()
	page.GetByID("copy_detailed_test_report").Click()
	waitForCopy(t, a)
	if !strings.Contains(copied[4], "Değerlendirme:") {
		t.Fatal("detailed report needs assessment criteria")
	}
}

func TestHealthReportEZAdvancedShareSameSnapshotAndKeyboardCopy(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	a.State.ActivePanel = state.PanelDiagnostics
	a.State.Diag.GenerateHealthReport(time.Now())
	ez := views.BuildHealthClipboard(a.State, false, time.Now())
	a.SetMode(state.ModeAdvanced)
	advanced := views.BuildHealthClipboard(a.State, false, time.Now())
	if ez != advanced {
		t.Fatal("UX mode must not change stored measurements or conclusions")
	}
	var copied string
	a.Opts.CopyText = func(text string) error { copied = text; return nil }
	frame := terminal.NewFrame(buffer.NewBuffer(cell.NewRect(0, 0, 160, 60)), terminal.NewFocusManager())
	a.Draw(frame, nil)
	frame.FocusManager.SetFocused("copy_full_report")
	a.HandleKeyEvent(frame, driver.KeyEvent{Type: driver.KeyEnter})
	waitForCopy(t, a)
	if copied != advanced {
		t.Fatal("focused keyboard action must copy the same underlying report")
	}
}

func TestHealthReportHistoryReviewCopyAndExport(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	a.State.Diag.Ctx.Info.FirmwareVersion = "historical-version"
	a.State.Diag.GenerateHealthReport(time.Now())
	original := a.State.Diag.FinalReport.Text()
	a.diagnosticStorage("save")
	waitDiagnosticIO(t, a)
	a.State.Diag.Ctx.Info.FirmwareVersion = "current-version"
	a.State.Diag.GenerateHealthReport(time.Now())
	a.diagnosticStorage("review")
	waitDiagnosticIO(t, a)
	if a.State.Diag.PreviousReport == nil || a.State.Diag.ReportSnapshot(time.Now()).Text() != original {
		t.Fatal("review must preserve historical content, not rebuild it from current measurements")
	}
	var copied string
	a.Opts.CopyText = func(text string) error { copied = text; return nil }
	a.CopyText(views.BuildHealthClipboard(a.State, false, time.Now()))
	waitForCopy(t, a)
	if !strings.Contains(copied, "historical-version") || strings.Contains(copied, "current-version") {
		t.Fatal("previous-report copy used current firmware")
	}
	a.Opts.ReportDir = t.TempDir()
	a.ExportDiagnosticsReport()
	waitDiagnosticIO(t, a)
	files, err := os.ReadDir(a.Opts.ReportDir)
	if err != nil || len(files) != 3 {
		t.Fatal("previous report should export all three formats")
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(a.Opts.ReportDir, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "historical-version") {
			t.Fatal("previous-report export rebuilt current values")
		}
	}
	a.diagnosticStorage("clear")
	waitDiagnosticIO(t, a)
	if a.State.Diag.PreviousReport != nil {
		t.Fatal("clearing history must dismiss the historical report")
	}
}

func TestHealthReportCompactScreenRetainsControllerAndActions(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	a.State.ActivePanel = state.PanelDiagnostics
	a.State.Diag.GenerateHealthReport(time.Now())
	page := uitest.Run(t, 80, 24, a.Draw)
	page.Expect(page.GetByRole("image", "Sony DualSense Wireless Controller")).ToBeVisible()
	page.Expect(page.GetByID("copy_full_report")).ToBeVisible()
	page.Expect(page.GetByID("diag_scroll_down")).ToBeVisible()
}

func TestTechnicalCopyIsExplicitAndKeepsSimpleDefault(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	var copied string
	a.Opts.CopyText = func(text string) error { copied = text; return nil }
	page.GetByID("diag_generate_health_report").Click()
	page.GetByID("copy_technical_report").Click()
	waitForCopy(t, a)
	if !strings.Contains(copied, "Teknik Tanılama Raporu") || !strings.Contains(copied, "Algoritma:") {
		t.Fatal("explicit technical copy missing metadata")
	}
	page.GetByID("show_technical_report").Click()
	if !a.State.Diag.ShowTechnicalReport {
		t.Fatal("technical view was not selected")
	}
	page.GetByID("copy_full_report").Click()
	waitForCopy(t, a)
	if strings.Contains(copied, "Algoritma:") || !strings.Contains(copied, "Kontrolcü Sağlık Raporu") {
		t.Fatal("technical view must not change default copy format")
	}
}
