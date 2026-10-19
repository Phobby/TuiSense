package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/uitest"
)

func openDiagnostics(t *testing.T, mode state.UXMode) (*Application, *uitest.Page) {
	a := newMockApp(t, mode)
	a.Opts.ReportDir = t.TempDir()
	page := uitest.Run(t, 120, 40, a.Draw)
	label := "Diagnostics"
	if mode == state.ModeEZ {
		label = "2 Diagnostics" // EZ has few enough tabs to show hotkeys
	}
	page.GetByRole("tab", label).Click()
	if a.State.ActivePanel != state.PanelDiagnostics || a.State.Diag == nil {
		t.Fatalf("Diagnostics tab should open with a session")
	}
	return a, page
}

func TestDiagnosticsAdaptToSelectedController(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	for _, id := range []string{"diag_tab_sticks", "diag_tab_buttons", "diag_tab_triggers", "diag_tab_touchpad", "diag_tab_motion"} {
		page.Expect(page.GetByID(id)).ToBeVisible()
	}

	page.GetByRole("button", "Xbox (Test)").Click()
	d := a.State.Diag
	if d == nil || d.Ctx.Desc.ModelID != xboxmock.ModelID || !d.Ctx.Simulated {
		t.Fatalf("diagnostics must switch to the simulated Xbox session")
	}
	page.Expect(page.GetByRole("image", "Xbox-style Gamepad 3D")).ToBeVisible()
	page.Expect(page.GetByID("diag_tab_touchpad")).Not().ToBeVisible()
	page.Expect(page.GetByID("diag_tab_motion")).Not().ToBeVisible()
	page.Expect(page.GetByID("diag_tab_audio_input")).Not().ToBeVisible()
	if d.Status(diag.CatTouchpad) != diag.Unsupported {
		t.Fatalf("touchpad must be unsupported on the Xbox test device")
	}

	// Clicking a region on the visual opens its diagnostic test.
	page.GetByRole("button", "LT").Click()
	if a.State.ActivePanel != state.PanelDiagnostics || d.Active != diag.CatTriggers {
		t.Fatalf("LT should open trigger diagnostics, got panel=%s active=%s", a.State.ActivePanel, d.Active)
	}
	page.Expect(page.GetByID("triggers_start")).ToBeVisible()
	page.Expect(page.GetByID("resistance_start")).Not().ToBeVisible() // no adaptive triggers on Xbox

	// Xbox-specific labels in the button tester.
	d.Active = diag.CatButtons
	page.Expect(page.GetByLabel("A")).ToBeVisible()
	page.Expect(page.GetByLabel("≡ Menu")).ToBeVisible()
}

func TestXboxMockDriftTestReportsSimulatedPossibleDrift(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	a.SelectDevice(xboxmock.Key)
	a.State.ActivePanel = state.PanelDiagnostics
	d := a.State.Diag
	d.Sticks.Duration = 2 * time.Second
	a.tickDiagnostics(time.Now())
	d.Sticks.StartStationary(time.Now())
	deadline := time.Now().Add(4 * time.Second)
	for d.Sticks.Running() && time.Now().Before(deadline) {
		a.pumpTelemetry()
		time.Sleep(40 * time.Millisecond)
	}
	r, ok := d.Results[diag.CatSticks]
	if !ok {
		t.Fatalf("drift test should produce a result")
	}
	if !r.Simulated || r.Status != diag.Warning || !strings.Contains(d.Sticks.Analysis[1].Summary, "Possible drift") {
		t.Fatalf("simulated right-stick offset should be flagged as possible drift (simulated), got %v %q", r.Status, d.Sticks.Analysis[1].Summary)
	}
	if d.Sticks.Analysis[0].Status != diag.Passed {
		t.Fatalf("left stick of the mock rests centered and should pass, got %q", d.Sticks.Analysis[0].Summary)
	}
}

func TestLeavingDiagnosticsRestoresTriggersAndStopsEffects(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	d := a.State.Diag
	d.Active = diag.CatTriggers
	page.GetByID("resistance_start").Click()
	a.Dispatcher.FlushNow()
	mock := a.Manager.Active()
	if mock.CurrentConfig().LeftTrigger.Mode != device.TriggerModeResistance {
		t.Fatalf("test resistance should be applied after the explicit click")
	}
	d.Haptics.Play("both", time.Now())

	page.Press("esc") // back to overview → leaving diagnostics
	a.Dispatcher.FlushNow()
	if d.Triggers.ResistanceActive || d.Haptics.Running != nil {
		t.Fatalf("leaving the tab must stop resistance and haptics")
	}
	if got, want := mock.CurrentConfig().LeftTrigger, a.State.Config.LeftTrigger; got != want {
		t.Fatalf("triggers must be restored to the user's settings: got %+v want %+v", got, want)
	}
	if a.State.Config.LeftTrigger.Mode == device.TriggerModeResistance && a.State.Config.LeftTrigger.Strength == 150 {
		t.Fatalf("diagnostics must never modify the user's config")
	}
}

func TestDisconnectDuringDiagnostics(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	d := a.State.Diag
	d.Active = diag.CatSticks
	page.GetByID("stick_start").Click()
	if !d.Sticks.Running() {
		t.Fatalf("drift test should be running")
	}
	page.Press("D") // simulated unplug
	if d.Sticks.Running() || d.Status(diag.CatSticks) != diag.Unavailable {
		t.Fatalf("disconnect must cancel the measurement and mark tests unavailable, got %v", d.Status(diag.CatSticks))
	}
	if d.Conn.Interruptions != 1 {
		t.Fatalf("the interruption should be counted, got %d", d.Conn.Interruptions)
	}
	page.Expect(page.GetByID("stick_start")).Not().ToBeVisible()
}

func TestReportExportFromDiagnostics(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	for i := 0; i < 3; i++ {
		a.pumpTelemetry()
	}
	page.GetByID("diag_export").Click()
	entries, _ := os.ReadDir(a.Opts.ReportDir)
	var jsonFile string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			jsonFile = filepath.Join(a.Opts.ReportDir, e.Name())
		}
	}
	if len(entries) != 2 || jsonFile == "" {
		t.Fatalf("expected a JSON and a text report, got %d files", len(entries))
	}
	data, _ := os.ReadFile(jsonFile)
	out := string(data)
	for _, want := range []string{`"model_id": "` + dualsense.ModelDualSense + `"`, `"simulated": true`, "SIMULATED DEVICE", `"battery"`} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %s", want)
		}
	}
	if strings.Contains(out, "E8:47:3A") {
		t.Errorf("MAC address must not be exported by default")
	}
}

func TestEZDiagnosticsAreGuidedAndPlain(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeEZ)
	page.Expect(page.GetByID("diag_run_guided")).ToBeVisible()
	page.Expect(page.GetByID("diag_export")).Not().ToBeVisible() // export is an Advanced tool
	page.GetByID("diag_run_guided").Click()
	d := a.State.Diag
	if d.Guided == nil || d.Active != diag.CatConnection {
		t.Fatalf("guided run should start at the connection step")
	}
	page.GetByID("diag_guided_next").Click()
	if d.Active != diag.CatBattery {
		t.Fatalf("expected battery as second guided step, got %s", d.Active)
	}
	// Switching mode keeps the diagnostics session and its state.
	page.Press("e")
	if a.State.Diag != d || d.Guided == nil {
		t.Fatalf("switching modes must not discard diagnostics state")
	}
}
