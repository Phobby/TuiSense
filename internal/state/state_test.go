package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
)

func panelIDs(entries []PanelEntry) map[PanelID]PanelEntry {
	out := map[PanelID]PanelEntry{}
	for _, e := range entries {
		out[e.Spec.ID] = e
	}
	return out
}

func TestVisiblePanelsAreCapabilityDriven(t *testing.T) {
	ds := panelIDs(VisiblePanels(dualsense.Descriptor().Capabilities, ModeAdvanced))
	for _, id := range []PanelID{PanelOverview, PanelTriggers, PanelMotion, PanelLighting, PanelAudio, PanelSticks, PanelDevice, PanelLogs, PanelSettings} {
		if _, ok := ds[id]; !ok {
			t.Errorf("DualSense Advanced should show %s", id)
		}
	}
	if e, ok := ds[PanelFirmware]; !ok || e.Support != device.Full {
		t.Errorf("DualSense Advanced should show the Device Details panel")
	}

	xb := panelIDs(VisiblePanels(xboxmock.Descriptor().Capabilities, ModeAdvanced))
	for _, id := range []PanelID{PanelTriggers, PanelMotion} {
		if _, ok := xb[id]; ok {
			t.Errorf("Xbox test device must not show %s", id)
		}
	}
	if e, ok := xb[PanelLighting]; !ok || e.Support != device.Limited {
		t.Errorf("Xbox lighting should be visible and Limited, got %+v ok=%v", e, ok)
	}
	// Device details exist for every controller; the firmware check inside
	// the panel is offered only by a provider (see app tests).
	if _, ok := xb[PanelFirmware]; !ok {
		t.Errorf("every attached controller should offer Device Details")
	}
	if _, ok := panelIDs(VisiblePanels(device.CapabilitySet{}, ModeAdvanced))[PanelFirmware]; ok {
		t.Errorf("Device Details must be hidden without a controller")
	}
}

func TestEZModeIsGenuinelySmaller(t *testing.T) {
	caps := dualsense.Descriptor().Capabilities
	ez := VisiblePanels(caps, ModeEZ)
	adv := VisiblePanels(caps, ModeAdvanced)
	if len(ez) >= len(adv) {
		t.Fatalf("EZ should expose fewer panels (%d) than Advanced (%d)", len(ez), len(adv))
	}
	ids := panelIDs(ez)
	for _, id := range []PanelID{PanelSticks, PanelMotion, PanelAudio, PanelDevice, PanelLogs} {
		if _, ok := ids[id]; ok {
			t.Errorf("EZ mode should hide %s", id)
		}
	}
	if _, ok := ids[PanelFirmware]; !ok {
		t.Errorf("EZ mode should offer the simple Device Details panel")
	}
	for i, e := range ez {
		if want := []rune("1234567890")[i]; e.Hotkey != want {
			t.Errorf("hotkeys must follow visible order: panel %s got %q want %q", e.Spec.ID, e.Hotkey, want)
		}
	}
	if ez[0].DisplayTitle(ModeEZ) != "Home" {
		t.Errorf("EZ overview should be titled Home")
	}
}

func TestNoControllerShowsOnlyAlwaysAvailablePanels(t *testing.T) {
	got := VisiblePanels(device.PlaceholderDescriptor().Capabilities, ModeAdvanced)
	if len(got) != 2 || got[0].Spec.ID != PanelOverview || got[1].Spec.ID != PanelSettings {
		t.Fatalf("expected only overview+settings without a controller, got %+v", got)
	}
}

func newState(t *testing.T) *AppState {
	t.Helper()
	return NewAppState(t.TempDir(), nil)
}

func activate(s *AppState, key string, desc device.Descriptor) {
	s.ActivateDevice(key, desc, desc.Capabilities, device.Info{Connected: true}, device.Telemetry{}, desc.NewDefaultConfig())
}

func TestActivateDeviceSwitchesContextAndRemembersConfig(t *testing.T) {
	s := newState(t)
	s.Mode = ModeAdvanced
	activate(s, "ds", dualsense.Descriptor())
	s.Config.Lighting.Red = 11
	s.ActivePanel = PanelTriggers

	activate(s, "xb", xboxmock.Descriptor())
	if s.Descriptor.VisualID != "xbox_series" || s.ActiveDeviceKey != "xb" {
		t.Fatalf("context not switched: %+v", s.Descriptor.ModelID)
	}
	if s.ActivePanel != PanelOverview {
		t.Fatalf("triggers panel is unavailable on Xbox; expected fallback to overview, got %s", s.ActivePanel)
	}
	if s.Config.Lighting.Red == 11 {
		t.Fatalf("Xbox must get its own config")
	}

	activate(s, "ds", dualsense.Descriptor())
	if s.Config.Lighting.Red != 11 {
		t.Fatalf("switching back must restore the DualSense config, got red=%d", s.Config.Lighting.Red)
	}
}

func TestSelectRegionRoutesByCapabilityAndMode(t *testing.T) {
	s := newState(t)
	s.Mode = ModeAdvanced
	activate(s, "ds", dualsense.Descriptor())

	s.SelectRegion(device.RegionRightTrigger)
	if s.ActivePanel != PanelTriggers || !s.EditingRightTrigger {
		t.Fatalf("R2 should open triggers editing right, got %s", s.ActivePanel)
	}

	activate(s, "xb", xboxmock.Descriptor())
	s.SelectRegion(device.RegionLeftTrigger)
	if s.ActivePanel != PanelHaptics {
		t.Fatalf("Xbox LT is an impulse trigger and should open haptics, got %s", s.ActivePanel)
	}

	s.SetMode(ModeEZ)
	s.SelectRegion(device.RegionLeftStick)
	if s.ActivePanel != PanelOverview || s.RegionHint == "" {
		t.Fatalf("stick tuning is Advanced-only; EZ should stay on Home with a hint, got %s %q", s.ActivePanel, s.RegionHint)
	}
}

func TestSetModeFallsBackWhenPanelHidden(t *testing.T) {
	s := newState(t)
	s.Mode = ModeAdvanced
	activate(s, "ds", dualsense.Descriptor())
	s.ActivePanel = PanelSticks
	s.SetMode(ModeEZ)
	if s.ActivePanel != PanelOverview {
		t.Fatalf("expected fallback to overview, got %s", s.ActivePanel)
	}
	s.ActivePanel = PanelLighting
	s.SetMode(ModeAdvanced)
	if s.ActivePanel != PanelLighting {
		t.Fatalf("lighting exists in both modes and should stay active")
	}
}

func TestPrefsRoundTripAndCorruptFile(t *testing.T) {
	dir := t.TempDir()
	ps := NewPrefsStore(dir)
	p, err := ps.Load()
	if err != nil || ParseMode(p.Mode) != ModeEZ {
		t.Fatalf("missing file should yield EZ defaults, got %+v %v", p, err)
	}
	want := Preferences{Mode: string(ModeAdvanced), LastDevice: "test:xbox", ActiveProfiles: map[string]string{"dualsense": "Competitive FPS"}}
	if err := ps.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := ps.Load()
	if err != nil || got.Mode != want.Mode || got.LastDevice != want.LastDevice || got.ActiveProfiles["dualsense"] != "Competitive FPS" {
		t.Fatalf("round trip mismatch: %+v %v", got, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = ps.Load()
	if err == nil || ParseMode(got.Mode) != ModeEZ {
		t.Fatalf("corrupt file should report an error and fall back to EZ, got %+v %v", got, err)
	}
}

func TestProfilesFilteredByFamily(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	if _, err := pm.Save("Xbox Only", "", "", "xbox", device.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	has := func(list []Profile, name string) bool {
		for _, p := range list {
			if p.Name == name {
				return true
			}
		}
		return false
	}
	if !has(pm.ListFor("xbox"), "Xbox Only") || has(pm.ListFor("dualsense"), "Xbox Only") {
		t.Fatalf("family-scoped profile leaked or missing")
	}
	if !has(pm.ListFor("xbox"), "Astro Standard") {
		t.Fatalf("universal built-ins must be listed for every family")
	}
}
