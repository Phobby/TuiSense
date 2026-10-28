package app

import (
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/uitest"
)

// galleryTo opens the gallery and steps to the model with the given visual ID.
func galleryTo(t *testing.T, a *Application, page *uitest.Page, id string) {
	t.Helper()
	page.Press("G")
	for i := 0; a.State.StageModelID() != id; i++ {
		if i > 20 {
			t.Fatalf("model %s is not in the gallery", id)
		}
		page.Press("right")
	}
}

// With no controller at all (macOS, Windows without XInput pads), the
// gallery opens any model's settings as a preview, and Esc returns.
func TestGalleryPreviewWithoutController(t *testing.T) {
	a := newTestApp(t, newDirs(t), Options{Version: "0.8.1", Registry: drivers.NewBuiltinRegistry(drivers.RegistryOptions{})})
	a.State.SetMode(state.ModeAdvanced)
	page := uitest.Run(t, 140, 40, a.Draw)

	galleryTo(t, a, page, "dualsense")
	page.GetByID("gallery_preview").Click()
	s := a.State
	if !s.IsPreviewDevice() || s.Descriptor.Family != "dualsense" || s.Gallery.Active {
		t.Fatalf("preview not active: key %q model %q", s.ActiveDeviceKey, s.Descriptor.ModelID)
	}
	if s.ActivePanel != state.PanelCustomize {
		t.Fatalf("the preview opens the settings, got %s", s.ActivePanel)
	}
	page.Expect(page.GetByID("exit_preview")).ToBeVisible()
	if first := strings.Split(snapshotOf(a, 140, 40), "\n")[0]; !strings.Contains(first, "PREVIEW") {
		t.Fatalf("the header must say PREVIEW: %q", first)
	}
	if s.Caps.Supports(device.CapBattery) || s.Caps.Supports(device.CapFirmwareCheck) {
		t.Fatal("a preview cannot read a battery or check firmware")
	}

	page.Press("2")
	if snap := snapshotOf(a, 140, 40); !strings.Contains(snap, "not part of the settings preview") {
		t.Fatal("diagnostics must explain that they need the real controller")
	}

	page.Press("esc")
	if s.IsPreviewDevice() || s.ActiveDeviceKey != "" || len(s.Devices) != 0 || s.ShowQuitConfirm {
		t.Fatalf("Esc leaves the preview: key %q devices %d dialog %v", s.ActiveDeviceKey, len(s.Devices), s.ShowQuitConfirm)
	}

	// A model with nothing TuiSense can configure previews what a connected
	// one shows: no Customize tab, just as with the real controller.
	a.escAt = time.Time{}
	galleryTo(t, a, page, "xbox_elite2")
	page.Press("enter")
	if !s.IsPreviewDevice() || s.Descriptor.ModelID != "microsoft.xbox_elite_series2" || !state.PanelVisible(s.VisiblePanels(), s.ActivePanel) {
		t.Fatalf("Xbox Elite preview: model %q panel %s", s.Descriptor.ModelID, s.ActivePanel)
	}
}

// Clicking a part in the gallery and previewing opens that part's settings:
// a DualSense trigger opens the adaptive-trigger editor, and changing it
// never touches the connected controller.
func TestGalleryPreviewOpensTheSelectedPart(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 140, 40, a.Draw)
	real := a.State.ActiveDeviceKey
	realCfg := a.Manager.Active().CurrentConfig()

	galleryTo(t, a, page, "dualsense")
	a.State.SelectComponent("left_trigger")
	page.GetByID("gallery_preview").Click()
	s := a.State
	if !s.IsPreviewDevice() || s.SelectedRegion != device.RegionLeftTrigger || s.ActivePanel != state.PanelTriggers {
		t.Fatalf("trigger settings not opened: preview %v region %q panel %s", s.IsPreviewDevice(), s.SelectedRegion, s.ActivePanel)
	}
	if snap := snapshotOf(a, 140, 40); strings.Contains(snap, "applied to the controller") || !strings.Contains(snap, "not sent to any controller") {
		t.Fatalf("a preview must not claim settings reach a controller:\n%s", snap)
	}

	trig := s.Config.LeftTrigger
	trig.Mode = device.TriggerModeResistance
	a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, LeftTrigger: &trig})
	a.Dispatcher.FlushNow()

	page.GetByID("exit_preview").Click()
	if s.ActiveDeviceKey != real || s.IsPreviewDevice() {
		t.Fatalf("exit returns to %q, got %q", real, s.ActiveDeviceKey)
	}
	if got := a.Manager.Active().CurrentConfig(); got.LeftTrigger != realCfg.LeftTrigger {
		t.Fatalf("the preview changed the connected controller: %+v", got.LeftTrigger)
	}
	prefs, _ := a.Prefs.Load()
	if strings.HasPrefix(prefs.LastDevice, "preview:") {
		t.Fatal("a preview is never remembered as the last device")
	}
}
