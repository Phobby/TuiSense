package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/uitest"
)

func waitDiagnosticIO(t *testing.T, a *Application) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for a.diagnosticBusy && time.Now().Before(deadline) {
		a.drainDiagnosticStorage()
		time.Sleep(time.Millisecond)
	}
	if a.diagnosticBusy {
		t.Fatal("diagnostic storage did not complete")
	}
	if a.State.LastError != "" {
		t.Fatal(a.State.LastError)
	}
}
func TestV2ExplicitHistoryLifecycle(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	a.State.Diag.Results[diag.CatSticks] = diag.Result{Category: diag.CatSticks, Status: diag.Passed, At: time.Now(), Measurements: []diag.Measurement{{Name: "Left center offset", Value: 2, Unit: "%"}}, Simulated: true}
	history := filepath.Join(a.Opts.ConfigDir, "diagnostics", "history.json")
	if _, err := os.Stat(history); !os.IsNotExist(err) {
		t.Fatal("polling must not save history")
	}
	a.diagnosticStorage("save")
	waitDiagnosticIO(t, a)
	if _, err := os.Stat(history); err != nil {
		t.Fatal(err)
	}
	a.diagnosticStorage("compare")
	waitDiagnosticIO(t, a)
	if len(a.State.Diag.HistorySummary) == 0 {
		t.Fatal("comparison must be presented")
	}
	a.diagnosticStorage("clear")
	waitDiagnosticIO(t, a)
	if _, err := os.Stat(history); !os.IsNotExist(err) {
		t.Fatal("history must be clearable")
	}
}
func TestV2ViewportScrollAndKeyboardActions(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	a.State.Diag.Select(diag.CatSticks)
	page.Expect(page.GetByID("stick_start")).ToBeVisible()
	page.GetByID("stick_start").Click()
	if !a.State.Diag.Sticks.Running() {
		t.Fatal("translated click should start the test")
	}
	// Keyboard category changes clean up the outgoing measurement.
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyRune, Ch: 'l'})
	if a.State.Diag.Sticks.Running() {
		t.Fatal("category navigation must cancel outgoing tests")
	}
	a.State.Diag.Select("")
	a.State.Diag.HistorySummary = make([]string, 80)
	for i := range a.State.Diag.HistorySummary {
		a.State.Diag.HistorySummary[i] = "Historical measurement"
	}
	page.Press("Tab") // Redraw after changing UI-owned state directly.
	page.Expect(page.GetByID("diag_scroll_down")).ToBeVisible()
	page.GetByID("diag_scroll_down").Click()
	if a.State.DiagnosticsScroll == 0 {
		t.Fatalf("long diagnostics must scroll: max=%d scope=%q active=%q history=%d", a.State.DiagnosticsScrollMax, a.State.DiagnosticsScope, a.State.Diag.Active, len(a.State.Diag.HistorySummary))
	}
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyPageUp})
	if a.State.DiagnosticsScroll != 0 {
		t.Fatal("page up must scroll towards top")
	}
}
func TestV2CompactDiagnosticsCanReachPreviewAndKeepConfiguration(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	a.State.ActivePanel = state.PanelDiagnostics
	a.State.Diag.Select(diag.CatSticks)
	page := uitest.Run(t, 80, 24, a.Draw)
	original := a.State.Config.Sticks
	for i := 0; i < 12; i++ {
		page.GetByID("diag_scroll_down").Click()
	}
	// Preview changes are isolated from persistent controller configuration.
	a.State.Diag.Sticks.PreviewDeadzone = 25
	a.State.Diag.Sticks.AxialPreview = true
	if a.State.Config.Sticks != original {
		t.Fatal("preview must never change configuration")
	}
	if a.State.DiagnosticsScrollMax == 0 {
		t.Fatal("compact content must remain reachable")
	}
}
func TestV2FailedExportNeverClaimsSuccess(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	a.Opts.ReportDir = filepath.Join(blocker, "reports")
	a.ExportDiagnosticsReport()
	deadline := time.Now().Add(3 * time.Second)
	for a.diagnosticBusy && time.Now().Before(deadline) {
		a.drainDiagnosticStorage()
		time.Sleep(time.Millisecond)
	}
	if a.State.LastError == "" || !strings.Contains(a.State.LastError, "not a directory") {
		t.Fatalf("export failure must be visible: %s", a.State.LastError)
	}
}
