package app

import (
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/compat"
	"github.com/Phobby/TuiSense/internal/demo"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/padmodels"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/update"
	"github.com/thebanri/limoni/uitest"
)

// newWebDemoApp builds the app exactly as `tuisense --web-demo` does, with
// temporary directories and a recording clipboard.
func newWebDemoApp(t *testing.T) (*Application, *[]string) {
	t.Helper()
	dirs := newDirs(t)
	var copied []string
	a := New(Options{WebDemo: true, ProfileDir: dirs.profiles, ConfigDir: dirs.config, ReportDir: t.TempDir(),
		Language: "en", Version: "9.9.9",
		CopyText: func(s string) error { copied = append(copied, s); return nil }})
	a.State.AutoRotate = false
	return a, &copied
}

func demoKey(modelID string) string { return demo.KeyPrefix + modelID }

func TestWebDemoListsTheTenSimulatedControllersAndNoHardware(t *testing.T) {
	a, _ := newWebDemoApp(t)
	if n := len(a.Opts.Registry.Drivers()); n != 0 {
		t.Fatalf("the web demo registry must contain no hardware driver, got %d", n)
	}
	if len(a.State.Devices) != 10 {
		t.Fatalf("expected the ten demo controllers, got %d", len(a.State.Devices))
	}
	for i, m := range demo.Models() {
		d := a.State.Devices[i]
		if d.Key != m.Key || d.Source != device.SourceSimulated || d.Descriptor.ModelID != m.ModelID {
			t.Fatalf("device %d: got %s/%s/%s, want %s", i, d.Key, d.Source, d.Descriptor.ModelID, m.Key)
		}
	}
	if a.State.ActiveDeviceKey != demo.DefaultKey || !a.State.IsSimulatedDevice() {
		t.Fatalf("the demo must open on the simulated DualSense, got %q", a.State.ActiveDeviceKey)
	}
	if a.Opts.AudioBackend != nil {
		t.Fatal("the web demo must not open a host audio backend")
	}
	if a.State.Telemetry.Simulated == false {
		a.pumpTelemetry()
		if !a.State.Telemetry.Simulated {
			t.Fatal("telemetry must be marked simulated")
		}
	}
	snap := snapshotOf(a, 160, 45)
	if !strings.Contains(snap, "WEB DEMO — SIMULATED CONTROLLERS") || !strings.Contains(snap, "v9.9.9") {
		t.Fatalf("the header must show the web demo indicator and the version:\n%s", snap)
	}
}

func TestWebDemoUsesTheRealModelDescriptorsAndVisuals(t *testing.T) {
	a, _ := newWebDemoApp(t)
	want := map[string]string{}
	for id, d := range a.State.HardwareModels {
		want[id] = d.VisualID
	}
	for _, m := range demo.Models() {
		a.SelectDevice(m.Key)
		if a.State.ActiveDeviceKey != m.Key {
			t.Fatalf("could not select %s", m.Key)
		}
		if got := a.State.Descriptor.VisualID; got == "" || got != want[m.ModelID] {
			t.Fatalf("%s: visual %q, the real model uses %q", m.ModelID, got, want[m.ModelID])
		}
		snap := snapshotOf(a, 160, 45)
		if !strings.Contains(snap, a.State.Descriptor.ShortName) {
			t.Fatalf("%s: the header must name the controller", m.ModelID)
		}
	}
}

func TestWebDemoCapabilitiesFollowTheSelectedModel(t *testing.T) {
	a, _ := newWebDemoApp(t)
	panels := func(key string) map[state.PanelID]bool {
		a.SelectDevice(key)
		out := map[state.PanelID]bool{}
		for _, p := range a.State.VisiblePanels() {
			out[p.Spec.ID] = true
		}
		return out
	}

	msi := panels(demoKey(msigc20.ModelID))
	for _, c := range []device.Capability{device.CapBattery, device.CapTouchpad, device.CapAdaptiveTriggers, device.CapLighting, device.CapMotion, device.CapAudio} {
		if a.State.Caps.Supports(c) {
			t.Fatalf("MSI Force GC20 V2 must not offer %s", c)
		}
	}
	if msi[state.PanelLighting] || msi[state.PanelTriggers] || msi[state.PanelAudio] || msi[state.PanelMotion] {
		t.Fatalf("MSI Force GC20 V2 shows panels for absent hardware: %v", msi)
	}
	a.State.ActivePanel = state.PanelDiagnostics
	for _, c := range a.State.Diag.Visible() {
		if c == diag.CatBattery || c == diag.CatTouchpad || c == diag.CatAudioIn || c == diag.CatMotion {
			t.Fatalf("MSI Force GC20 V2 diagnostics list %s", c)
		}
	}
	if !a.State.Caps.Supports(device.CapHaptics) {
		t.Fatal("the GC20 V2 has two rumble motors")
	}

	ds := panels(demoKey(dualsense.ModelDualSense))
	if !ds[state.PanelTriggers] || !ds[state.PanelLighting] {
		t.Fatalf("the DualSense must offer adaptive triggers and lighting: %v", ds)
	}
	if a.State.Caps.Supports(device.CapRearControls) {
		t.Fatal("the standard DualSense has no rear buttons")
	}
	panels(demoKey(dualsense.ModelDualSenseEdge))
	if !a.State.Caps.Supports(device.CapRearControls) {
		t.Fatal("the DualSense Edge reports its back buttons")
	}

	panels(demoKey(padmodels.XboxElite2ID))
	if !a.State.Caps.Supports(device.CapRearControls) {
		t.Fatal("the Elite Series 2 paddles are reported by xpad")
	}
	paddles := 0
	for _, b := range a.State.Descriptor.Inputs.Buttons {
		if b.Group == "Rear" {
			paddles++
		}
	}
	if paddles != 4 {
		t.Fatalf("the Elite Series 2 has four paddles, got %d", paddles)
	}
	if a.State.Caps.Supports(device.CapAdaptiveTriggers) {
		t.Fatal("Xbox controllers have no adaptive triggers")
	}

	panels(demoKey(padmodels.SwitchProID))
	if a.State.Descriptor.Inputs.TriggerKind != device.TriggerDigital {
		t.Fatal("ZL/ZR are digital on the Switch Pro Controller")
	}
	if !a.State.Caps.Supports(device.CapBattery) || a.State.Caps.Supports(device.CapAudio) {
		t.Fatal("the Switch Pro has a battery and no audio")
	}

	// Rear controls a model has but cannot report stay honest: present, not reachable.
	panels(demoKey(padmodels.WolverineID))
	if a.State.Caps.Supports(device.CapRearControls) || a.State.Caps.Spec(device.CapRearControls).Physical() != device.Present {
		t.Fatal("the Wolverine's back buttons exist but are not reported separately")
	}
}

func TestWebDemoSwitchesModesAndModelsThroughTheUI(t *testing.T) {
	a, _ := newWebDemoApp(t)
	page := uitest.Run(t, 160, 45, a.Draw)
	page.GetByID("mode_advanced").Click()
	if a.State.Mode != state.ModeAdvanced {
		t.Fatal("the ADVANCED switch must work")
	}
	page.GetByID("mode_ez").Click()
	if a.State.Mode != state.ModeEZ {
		t.Fatal("the EZ switch must work")
	}
	page.Press("c")
	if !a.State.ShowDeviceSelector {
		t.Fatal("c opens the controller selector")
	}
	page.Press("down")
	page.Press("enter")
	if a.State.ActiveDeviceKey != demoKey(dualsense.ModelDualSenseEdge) {
		t.Fatalf("the selector must switch to the Edge, got %s", a.State.ActiveDeviceKey)
	}
	page.Press("?")
	if !a.State.ShowHelp {
		t.Fatal("? opens help")
	}
	page.Press("?")
	page.Press("escape")
	if !a.State.ShowQuitConfirm {
		t.Fatal("Esc asks before quitting, as in the native app")
	}
}

func TestWebDemoDiagnosticsUseSimulatedInputAndSayItInTheReport(t *testing.T) {
	a, _ := newWebDemoApp(t)
	a.SelectDevice(demoKey(padmodels.XboxSeriesID))
	a.State.ActivePanel = state.PanelDiagnostics
	d := a.State.Diag
	if d == nil || !d.Ctx.Simulated {
		t.Fatal("diagnostics must run on a simulated session")
	}
	d.Sticks.Duration = time.Second
	a.tickDiagnostics(time.Now())
	d.Sticks.Start(time.Now())
	deadline := time.Now().Add(3 * time.Second)
	for d.Sticks.Running() && time.Now().Before(deadline) {
		a.pumpTelemetry()
		time.Sleep(40 * time.Millisecond)
	}
	if _, ok := d.Results[diag.CatSticks]; !ok {
		t.Fatal("the stick test must produce a result from simulated input")
	}
	r := diag.BuildReport(d, a.Opts.Version, false, time.Now())
	if !r.Device.Simulated {
		t.Fatal("the report must be marked simulated")
	}
	found := false
	for _, n := range r.Notes {
		found = found || strings.Contains(n, "SIMULATED DEVICE")
	}
	if !found {
		t.Fatal("the report must say its values are not hardware measurements")
	}
}

func TestWebDemoRefusesHostActions(t *testing.T) {
	a, copied := newWebDemoApp(t)
	before := a.Updater.Snapshot()
	if before.Settings.Check || before.Settings.AutoDownload {
		t.Fatal("update checks must be off in the web demo")
	}
	a.CheckForUpdates()
	a.DownloadUpdate()
	a.SetUpdateSettings(update.Settings{Check: true, AutoDownload: true})
	time.Sleep(50 * time.Millisecond)
	if after := a.Updater.Snapshot(); after.Status != before.Status || after.Settings.Check {
		t.Fatalf("the web demo must never check for or stage updates: %+v", after)
	}
	if msg := a.ApplyStagedUpdate(); msg != "" {
		t.Fatalf("the web demo must never install an update: %q", msg)
	}

	if _, ok := a.Opts.CompatEnv.(demo.CompatEnv); !ok {
		t.Fatal("Driver & Compatibility must not read the server")
	}
	if err := a.Opts.CompatRunner.Run([]string{"pkexec", "true"}); err == nil {
		t.Fatal("the web demo runner must refuse every command")
	}
	a.State.Compat = compat.Report{Actions: []compat.Action{{ID: compat.ActionUdevDualSense, Commands: [][]string{{"true"}}}}}
	a.State.CompatReview = compat.ActionUdevDualSense
	a.RunCompatAction(compat.ActionUdevDualSense)
	if a.State.CompatBusy || a.State.CompatStatus != webDemoDisabled {
		t.Fatalf("driver actions must be refused, status %q", a.State.CompatStatus)
	}

	if err := a.Opts.OpenURL("https://www.playstation.com/"); err == nil {
		t.Fatal("the web demo must never start a browser on the server")
	}
	if len(a.Opts.FirmwareProviders) != 0 {
		t.Fatal("firmware metadata is never fetched by the web demo")
	}

	a.CopyText("hello")
	deadline := time.Now().Add(2 * time.Second)
	for len(*copied) == 0 && time.Now().Before(deadline) {
		a.drainCopies()
		time.Sleep(10 * time.Millisecond)
	}
	if len(*copied) != 1 || (*copied)[0] != "hello" {
		t.Fatalf("copy must reach the clipboard adapter, got %v", *copied)
	}
}

func TestWebDemoClipboardIsTheBrowserTerminal(t *testing.T) {
	opts := webDemoOptions(Options{})
	svc, ok := opts.Clipboard.(interface {
		Available() []string
	})
	if !ok {
		t.Fatalf("unexpected clipboard %T", opts.Clipboard)
	}
	if tools := svc.Available(); len(tools) != 0 {
		t.Fatalf("no host clipboard tool may be used, got %v", tools)
	}
}

func TestWebDemoSimulatedControlsWork(t *testing.T) {
	a, _ := newWebDemoApp(t)
	a.SelectDevice(demoKey(padmodels.DualShock4ID))
	a.ToggleSimulatedConnection()
	if a.State.DeviceInfo.Connected {
		t.Fatal("the simulated controller must unplug")
	}
	a.ToggleSimulatedConnection()
	a.CycleSimulatedTransport()
	if a.State.DeviceInfo.Connection != device.ConnBluetooth {
		t.Fatalf("the DualShock 4 switches to Bluetooth, got %s", a.State.DeviceInfo.Connection)
	}
	if a.State.Caps.Supports(device.CapAudio) {
		t.Fatal("DualShock 4 audio is USB-only")
	}
	a.SelectDevice(demoKey(msigc20.ModelID))
	a.CycleSimulatedTransport()
	if a.State.DeviceInfo.Connection == device.ConnBluetooth {
		t.Fatal("the wired-only GC20 V2 never switches to Bluetooth")
	}
	a.TestPulse()
	a.Dispatcher.FlushNow()
	if tel := a.Manager.Active().Telemetry(); !strings.Contains(tel.LastOutputHex, "no vibration") {
		t.Fatalf("a rumble test must only be recorded, got %q", tel.LastOutputHex)
	}
}
