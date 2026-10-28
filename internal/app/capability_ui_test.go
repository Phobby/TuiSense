package app

import (
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/compat"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/testkit"
)

func snapshotOf(a *Application, w, h uint16) string {
	term := testkit.NewTerminal(w, h)
	term.Draw(func(f *terminal.Frame) { a.Draw(f, nil) })
	return term.Snapshot()
}

// A wired controller without battery, audio, motion or touchpad never shows
// those sections anywhere: home, diagnostics, reports or settings.
func TestBatterylessControllerShowsNoAbsentHardware(t *testing.T) {
	a := newMSIApp(t, state.ModeAdvanced)
	home := snapshotOf(a, 160, 45)
	for _, word := range []string{"Battery", "Charging"} {
		if strings.Contains(home, word) {
			t.Fatalf("home mentions %q for a wired-only controller", word)
		}
	}
	a.State.ActivePanel = state.PanelDiagnostics
	for _, c := range a.State.Diag.Visible() {
		switch c {
		case diag.CatBattery, diag.CatMotion, diag.CatTouchpad, diag.CatAudioIn, diag.CatAudioOut:
			t.Fatalf("diagnostics list %s for the GC20 V2", c)
		}
	}
	diagScreen := snapshotOf(a, 160, 45)
	for _, word := range []string{"Battery", "Microphone", "Touchpad", "Motion", "Unsupported"} {
		if strings.Contains(diagScreen, word) {
			t.Fatalf("diagnostics screen mentions %q:\n%s", word, diagScreen)
		}
	}
	report := diag.BuildReport(a.State.Diag, "test", false, a.lastFrame)
	for _, e := range report.Entries {
		if e.Category == diag.CatBattery || e.Category == diag.CatAudioIn || e.Category == diag.CatMotion || e.Category == diag.CatTouchpad {
			t.Fatalf("report contains absent %s", e.Category)
		}
	}
	for _, p := range a.State.VisiblePanels() {
		if p.Spec.ID == state.PanelLighting || p.Spec.ID == state.PanelTriggers || p.Spec.ID == state.PanelAudio || p.Spec.ID == state.PanelMotion {
			t.Fatalf("settings panel %s offered for hardware the GC20 V2 lacks", p.Spec.ID)
		}
	}
}

// Changing transport refreshes capabilities, settings and diagnostics.
func TestTransportChangeRefreshesCapabilities(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	if a.State.DeviceInfo.Connection != device.ConnUSB {
		t.Skip("mock starts on another transport")
	}
	if !state.PanelVisible(a.State.VisiblePanels(), state.PanelAudio) {
		t.Fatal("over USB the DualSense audio settings are available")
	}
	a.CycleSimulatedTransport()
	if a.State.DeviceInfo.Connection != device.ConnBluetooth {
		t.Fatalf("expected Bluetooth, got %s", a.State.DeviceInfo.Connection)
	}
	if a.State.Caps.Supports(device.CapAudio) || state.PanelVisible(a.State.VisiblePanels(), state.PanelAudio) {
		t.Fatal("over Bluetooth the audio settings must disappear")
	}
	if a.State.Caps.Spec(device.CapAudio).Requires != device.RequiresConnection {
		t.Fatal("audio is present but needs USB, not absent")
	}
	a.State.ActivePanel = state.PanelDiagnostics
	for _, c := range a.State.Diag.Visible() {
		if c == diag.CatAudioIn || c == diag.CatAudioOut {
			t.Fatal("audio diagnostics must be hidden over Bluetooth")
		}
	}
	a.CycleSimulatedTransport()
	if !a.State.Caps.Supports(device.CapAudio) {
		t.Fatal("audio must come back over USB")
	}
}

// Selecting a 3D component opens only real, supported functionality.
func TestComponentSelectionFollowsCapabilities(t *testing.T) {
	a := newMSIApp(t, state.ModeAdvanced)
	a.State.ActivePanel = state.PanelOverview
	a.State.SelectComponent("left_stick")
	if a.State.ActivePanel != state.PanelDiagnostics {
		t.Fatalf("the MSI stick opens its diagnostics, got %s", a.State.ActivePanel)
	}
	a.State.ActivePanel = state.PanelOverview
	a.State.SelectComponent("mode")
	if a.State.ActivePanel != state.PanelOverview || a.State.SelectedRegion != device.RegionNone {
		t.Fatal("a hardware-only switch must not open any settings")
	}
	if c, _ := a.State.StageSpec().Component("mode"); a.State.ComponentReported(c) {
		t.Fatal("the GC20 mode button is not reported to the host")
	}
}

func TestDriverCompatibilitySectionIsActionable(t *testing.T) {
	a := newMSIApp(t, state.ModeEZ)
	a.Opts.CompatEnv = fakeLinux{}
	a.refreshCompat()
	if a.State.Compat.Input != "Simulated" && a.State.Compat.Input != "Working" {
		t.Fatalf("unexpected input status %q", a.State.Compat.Input)
	}
	a.State.ActivePanel = state.PanelFirmware
	a.State.Firmware.Section = state.FirmwareCompat
	snap := snapshotOf(a, 160, 45)
	if !strings.Contains(snap, "Driver & Compatibility") || !strings.Contains(snap, "Check Driver Requirements") {
		t.Fatalf("Driver & Compatibility section missing:\n%s", snap)
	}
	// An action can only run while it is being reviewed.
	a.State.Compat.Actions = []compat.Action{{ID: compat.ActionUdevDualSense, Title: "x", Commands: [][]string{{"true"}}}}
	a.RunCompatAction(compat.ActionUdevDualSense)
	if a.State.CompatBusy {
		t.Fatal("an unreviewed action must not start")
	}
}

type fakeLinux struct{}

func (fakeLinux) GOOS() string                    { return "linux" }
func (fakeLinux) ModuleLoaded(string) bool        { return false }
func (fakeLinux) Writable(string) bool            { return false }
func (fakeLinux) Exists(string) bool              { return false }
func (fakeLinux) LookPath(string) (string, error) { return "", errNoTool }

var errNoTool = &toolErr{}

type toolErr struct{}

func (*toolErr) Error() string { return "not found" }
