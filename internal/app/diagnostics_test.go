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
	page.Press("2")
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

	page.Press("c")
	page.GetByID("selector_item_1").Click()
	d := a.State.Diag
	if d == nil || d.Ctx.Desc.ModelID != xboxmock.ModelID || !d.Ctx.Simulated {
		t.Fatalf("diagnostics must switch to the simulated Xbox session")
	}
	page.Expect(page.GetByRole("image", "Microsoft Xbox Wireless Controller")).ToBeVisible()
	page.Expect(page.GetByID("diag_tab_touchpad")).Not().ToBeVisible()
	page.Expect(page.GetByID("diag_tab_motion")).Not().ToBeVisible()
	page.Expect(page.GetByID("diag_tab_audio_input")).Not().ToBeVisible()
	if d.Status(diag.CatTouchpad) != diag.Unsupported {
		t.Fatalf("touchpad must be unsupported on the Xbox test device")
	}

	// Clicking a region on the visual opens its diagnostic test.
	a.State.SelectRegion(device.RegionLeftTrigger)
	page.Press("tab")
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

func TestXboxMockStickTestFlagsSimulatedOffsetWithoutFaultClaim(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	a.SelectDevice(xboxmock.Key)
	a.State.ActivePanel = state.PanelDiagnostics
	d := a.State.Diag
	d.Sticks.Duration = 2 * time.Second
	a.tickDiagnostics(time.Now())
	d.Sticks.Start(time.Now())
	deadline := time.Now().Add(4 * time.Second)
	for d.Sticks.Running() && time.Now().Before(deadline) {
		a.pumpTelemetry()
		time.Sleep(40 * time.Millisecond)
	}
	if d.Sticks.Step != diag.StickExplain {
		t.Fatalf("after the resting step the explanation step should show, got %v", d.Sticks.Step)
	}
	r, ok := d.Results[diag.CatSticks]
	if !ok {
		t.Fatalf("the resting step should produce a preliminary result")
	}
	right := d.Sticks.Analysis[1]
	if !r.Simulated || r.Status != diag.Warning || right.Verdict != diag.VerdictFurtherTesting {
		t.Fatalf("simulated right-stick offset should ask for further testing (simulated), got %v %v %q", r.Status, right.Verdict, right.Headline)
	}
	if d.Sticks.Analysis[0].Verdict != diag.VerdictNormal {
		t.Fatalf("left stick of the mock rests centered and should be normal, got %q", d.Sticks.Analysis[0].Headline)
	}
	if r.Status == diag.Failed {
		t.Fatal("a single measurement must never be a failure")
	}
}

func TestLeavingDiagnosticsRestoresTriggersAndStopsEffects(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	d := a.State.Diag
	d.Active = diag.CatTriggers
	for range 6 { // the button sits below the fold, as a user would scroll to it
		page.Press("pgdn")
	}
	page.GetByID("resistance_start").Click()
	a.Dispatcher.FlushNow()
	mock := a.Manager.Active()
	if mock.CurrentConfig().LeftTrigger.Mode != device.TriggerModeResistance {
		t.Fatalf("test resistance should be applied after the explicit click")
	}
	d.Haptics.Play("both", time.Now())

	page.Press("1") // back to Home → leaving diagnostics
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
	deadline := time.Now().Add(3 * time.Second)
	for a.diagnosticBusy && time.Now().Before(deadline) {
		a.drainDiagnosticStorage()
		time.Sleep(time.Millisecond)
	}
	entries, _ := os.ReadDir(a.Opts.ReportDir)
	var jsonFile string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			jsonFile = filepath.Join(a.Opts.ReportDir, e.Name())
		}
	}
	if len(entries) != 3 || jsonFile == "" {
		t.Fatalf("expected JSON, Markdown and text reports, got %d files", len(entries))
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
	page.Expect(page.GetByID("diag_export")).ToBeVisible() // sharing is available in both modes
	page.GetByID("diag_run_guided").Click()
	d := a.State.Diag
	if d.Guided == nil || d.Active != diag.CatConnection {
		t.Fatalf("guided run should start at the connection step")
	}
	page.GetByID("diag_guided_next").Click()
	if d.Active != diag.CatButtons {
		t.Fatalf("expected buttons as second guided step, got %s", d.Active)
	}
	// Switching mode keeps the diagnostics session and its state.
	page.Press("e")
	if a.State.Diag != d || d.Guided == nil {
		t.Fatalf("switching modes must not discard diagnostics state")
	}
}

func TestSnapbackTestOnSimulatedDualSense(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeEZ)
	d := a.State.Diag
	page.Expect(page.GetByID("diag_tab_snapback")).Not().ToBeVisible()
	page.GetByID("diag_tab_sticks").Click()
	page.Expect(page.GetByID("stick_sub_deadzone")).ToBeVisible()
	page.Expect(page.GetByID("stick_start")).ToBeVisible()
	page.GetByID("stick_sub_snapback").Click()
	if d.Active != diag.CatSticks || d.StickSubTest != diag.StickSubSnapback {
		t.Fatalf("the snapback test lives inside the stick category, got %s/%s", d.Active, d.StickSubTest)
	}
	page.GetByID("snap_start").Click()
	if !d.Snapback.Running() {
		t.Fatal("snapback collection should start on the explicit click")
	}
	deadline := time.Now().Add(6 * time.Second)
	for len(d.Snapback.Events[0]) < 3 && time.Now().Before(deadline) {
		a.pumpTelemetry()
		time.Sleep(40 * time.Millisecond)
	}
	page.GetByID("snap_finish").Click()
	r, ok := d.Results[diag.CatSticks]
	if !ok || !r.Simulated {
		t.Fatalf("a simulated snapback result is expected")
	}
	if left := d.Snapback.Analyze(0); left.Verdict != diag.VerdictSnapback {
		t.Fatalf("the mock's 14%% overshoot should be noticeable, got %v %q", left.Verdict, left.Headline)
	}
	if right := d.Snapback.Analyze(1); right.Verdict == diag.VerdictNormal {
		t.Fatalf("at the mock's 25 Hz poll rate a clean result must not be claimed, got %q", right.Headline)
	}
	page.Expect(page.GetByID("snap_again")).ToBeVisible()
}
