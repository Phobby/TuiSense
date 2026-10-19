package app

import (
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/testkit"
	"github.com/thebanri/limoni/uitest"
)

type testDirs struct{ profiles, config string }

func newDirs(t *testing.T) testDirs {
	return testDirs{profiles: t.TempDir(), config: t.TempDir()}
}

// newTestApp builds an app that never touches real hardware or the user's
// config directory: the registry has no hardware driver and dirs are temporary.
func newTestApp(t *testing.T, dirs testDirs, opts Options) *Application {
	t.Helper()
	opts.ProfileDir = dirs.profiles
	opts.ConfigDir = dirs.config
	if opts.Registry == nil {
		opts.Registry = drivers.NewBuiltinRegistry(drivers.RegistryOptions{TestDevices: true})
	}
	a := New(opts)
	a.State.AutoRotate = false
	return a
}

func newMockApp(t *testing.T, mode state.UXMode) *Application {
	a := newTestApp(t, newDirs(t), Options{ForceMock: true, AllowMock: true})
	a.State.SetMode(mode)
	return a
}

func TestDashboardStartupAndRegionSelection(t *testing.T) {
	application := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 120, 40, application.Draw)

	page.Expect(page.GetByID("status_widget")).ToBeVisible()
	page.Expect(page.GetByID("device_summary_widget")).ToBeVisible()
	page.Expect(page.GetByID("battery_gauge")).ToBeVisible()

	page.GetByRole("button", "L2 Trigger").Click()
	if application.State.SelectedRegion != device.RegionLeftTrigger {
		t.Fatalf("expected SelectedRegion=%s, got %s", device.RegionLeftTrigger, application.State.SelectedRegion)
	}
	if application.State.ActivePanel != state.PanelTriggers {
		t.Fatalf("expected ActivePanel=triggers, got %v", application.State.ActivePanel)
	}
	page.Expect(page.GetByID("slider_l2_strength")).ToBeVisible()

	page.GetByRole("button", "Haptics").Click()
	if application.State.ActivePanel != state.PanelHaptics {
		t.Fatalf("expected haptics panel, got %v", application.State.ActivePanel)
	}
	page.Expect(page.GetByID("btn_test_heavy_rumble")).ToBeVisible()

	page.GetByRole("button", "L-Stick").Click()
	if application.State.ActivePanel != state.PanelSticks {
		t.Fatalf("expected sticks panel, got %v", application.State.ActivePanel)
	}
	page.Expect(page.GetByID("slider_lstick_inner_dz")).ToBeVisible()

	page.GetByRole("button", "Gyro/IMU").Click()
	if application.State.ActivePanel != state.PanelMotion {
		t.Fatalf("expected motion panel, got %v", application.State.ActivePanel)
	}
	page.Expect(page.GetByID("cb_gyro_enabled")).ToBeVisible()

	page.GetByRole("button", "Mic/Audio").Click()
	if application.State.ActivePanel != state.PanelAudio {
		t.Fatalf("expected audio panel, got %v", application.State.ActivePanel)
	}
	page.Expect(page.GetByID("slider_speaker_vol")).ToBeVisible()
}

func TestProfileSaveLoadAndReset(t *testing.T) {
	application := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 120, 40, application.Draw)

	page.Press("p")
	if application.State.ActivePanel != state.PanelProfiles {
		t.Fatalf("expected profiles panel, got %v", application.State.ActivePanel)
	}

	application.State.Config.Lighting.Red = 250
	application.State.Config.Lighting.Green = 10
	application.State.Config.Lighting.Blue = 90
	application.State.ProfileNameInput.SetValue("Neon Drift")

	page.GetByID("btn_profile_save").Click()
	if application.State.ActiveProfile != "Neon Drift" {
		t.Fatalf("expected ActiveProfile='Neon Drift', got %q", application.State.ActiveProfile)
	}

	page.GetByID("btn_profile_reset").Click()
	if application.State.Config.Lighting.Red == 250 {
		t.Fatalf("expected factory reset to restore default lighting color")
	}

	saved, ok := application.State.ProfileMgr.Get("Neon Drift")
	if !ok || saved.Config.Lighting.Red != 250 || saved.Family != dualsense.Family {
		t.Fatalf("expected saved DualSense-family profile 'Neon Drift' with Red=250, got ok=%v %+v", ok, saved.Family)
	}
}

func TestHotplugAndTransportSimulation(t *testing.T) {
	application := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 120, 40, application.Draw)
	application.State.ActivePanel = state.PanelSettings

	page.GetByID("btn_sim_hotplug").Click()
	if application.State.DeviceInfo.Connected {
		t.Fatalf("expected controller to be disconnected after toggling hotplug")
	}
	page.GetByID("btn_sim_hotplug").Click()
	if !application.State.DeviceInfo.Connected {
		t.Fatalf("expected controller to be reconnected")
	}

	page.GetByID("btn_sim_transport").Click()
	if application.State.DeviceInfo.Connection != device.ConnBluetooth {
		t.Fatalf("expected Bluetooth connection, got %s", application.State.DeviceInfo.Connection)
	}
	// Runtime capabilities follow the transport: no internal speaker over Bluetooth.
	if application.State.Caps.HasFeature(device.CapAudio, device.FeatSpeaker) {
		t.Fatalf("speaker feature should be removed over Bluetooth")
	}
}

func countBraille(t *testing.T, a *Application, w, h uint16) int {
	t.Helper()
	term := testkit.NewTerminal(w, h)
	term.Draw(func(f *terminal.Frame) { a.Draw(f, nil) })
	n := 0
	for _, r := range term.Snapshot() {
		if r >= 0x2801 && r <= 0x28FF {
			n++
		}
	}
	return n
}

func Test3DControllerMeshRendersVisibly(t *testing.T) {
	application := newMockApp(t, state.ModeEZ)
	if n := countBraille(t, application, 120, 40); n < 120 {
		t.Fatalf("expected at least 120 non-empty 3D Braille cells for DualSense, got %d", n)
	}
	application.SelectDevice(xboxmock.Key)
	if n := countBraille(t, application, 120, 40); n < 120 {
		t.Fatalf("expected at least 120 non-empty 3D Braille cells for Xbox, got %d", n)
	}
}

func TestSwitchingDeviceChangesVisualInfoAndPanels(t *testing.T) {
	application := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 120, 40, application.Draw)

	page.Expect(page.GetByRole("image", "Sony DualSense 3D")).ToBeVisible()
	application.State.Config.Lighting.Red = 42 // DualSense-specific change

	page.GetByRole("button", "Xbox (Test)").Click()
	s := application.State
	if s.ActiveDeviceKey != xboxmock.Key || s.Descriptor.VisualID != "xbox" {
		t.Fatalf("expected Xbox test device to be active, got %q", s.ActiveDeviceKey)
	}
	page.Expect(page.GetByRole("image", "Xbox-style Gamepad 3D")).ToBeVisible()
	page.Expect(page.GetByID("device_summary_widget")).ToContainValue("Xbox")
	page.Expect(page.GetByRole("button", "LT")).ToBeVisible()
	page.Expect(page.GetByRole("button", "Gyro/IMU")).Not().ToBeVisible()
	if state.PanelVisible(s.VisiblePanels(), state.PanelTriggers) || state.PanelVisible(s.VisiblePanels(), state.PanelMotion) {
		t.Fatalf("Xbox test device must not offer adaptive triggers or motion panels")
	}

	// Xbox LT is an impulse trigger: it opens haptics with trigger motor sliders.
	page.GetByRole("button", "LT").Click()
	if s.ActivePanel != state.PanelHaptics {
		t.Fatalf("expected haptics panel for Xbox LT, got %s", s.ActivePanel)
	}
	page.Expect(page.GetByID("slider_hap_lt")).ToBeVisible()

	page.Press(",")
	if s.ActiveDeviceKey != dualsense.MockKey {
		t.Fatalf("expected ',' to cycle back to DualSense, got %q", s.ActiveDeviceKey)
	}
	if s.Config.Lighting.Red != 42 {
		t.Fatalf("DualSense config should be restored after switching back, got red=%d", s.Config.Lighting.Red)
	}
	page.Expect(page.GetByRole("image", "Sony DualSense 3D")).ToBeVisible()
}

func TestDeviceSelectorModal(t *testing.T) {
	application := newMockApp(t, state.ModeEZ)
	page := uitest.Run(t, 120, 40, application.Draw)

	page.Press("c")
	page.Expect(page.GetByID("selector_item_1")).ToBeVisible()
	page.Press("down")
	page.Press("enter")
	if application.State.ActiveDeviceKey != xboxmock.Key || application.State.ShowDeviceSelector {
		t.Fatalf("expected Xbox selected and modal closed, got %q open=%v", application.State.ActiveDeviceKey, application.State.ShowDeviceSelector)
	}
}

func TestModeSwitchingChangesVisibleControls(t *testing.T) {
	application := newMockApp(t, state.ModeEZ)
	page := uitest.Run(t, 120, 40, application.Draw)
	s := application.State

	page.Expect(page.GetByID("ez_vib_Medium")).ToBeVisible()
	page.Expect(page.GetByRole("tab", "1 Home")).ToBeVisible()
	ezTabs := len(s.VisiblePanels())

	page.GetByID("ez_vib_Off").Click()
	if s.Config.Haptics.MasterIntensity != 0 {
		t.Fatalf("EZ vibration Off should set master intensity to 0")
	}
	page.GetByID("ez_trig_Weapon").Click()
	if s.Config.LeftTrigger.Mode != device.TriggerModeWeapon || s.Config.RightTrigger.Mode != device.TriggerModeWeapon {
		t.Fatalf("EZ trigger feel should apply to both triggers")
	}

	page.GetByID("mode_advanced").Click()
	if s.Mode != state.ModeAdvanced || len(s.VisiblePanels()) <= ezTabs {
		t.Fatalf("Advanced mode should expose more panels")
	}
	page.Expect(page.GetByID("ez_vib_Medium")).Not().ToBeVisible()
	page.Expect(page.GetByRole("button", "⚡ Test Vibration")).ToBeVisible()

	page.GetByRole("button", "L-Stick").Click()
	if s.ActivePanel != state.PanelSticks {
		t.Fatalf("expected sticks panel in Advanced mode")
	}
	page.Press("e")
	if s.Mode != state.ModeEZ || s.ActivePanel != state.PanelOverview {
		t.Fatalf("EZ has no sticks panel; expected fallback to Home, got %s", s.ActivePanel)
	}

	// In EZ, Advanced-only regions keep the user on Home with a hint.
	page.GetByRole("button", "L-Stick").Click()
	if s.ActivePanel != state.PanelOverview || !strings.Contains(s.RegionHint, "Advanced") {
		t.Fatalf("expected EZ hint for stick region, got panel=%s hint=%q", s.ActivePanel, s.RegionHint)
	}
}

func TestModeDeviceAndProfilePersistence(t *testing.T) {
	dirs := newDirs(t)
	first := newTestApp(t, dirs, Options{ForceMock: true})
	if first.State.Mode != state.ModeEZ {
		t.Fatalf("first launch should default to EZ mode")
	}
	first.SetMode(state.ModeAdvanced)

	prof, ok := first.State.ProfileMgr.Get("Competitive FPS")
	if !ok {
		t.Fatal("missing built-in profile")
	}
	first.State.Config = prof.Config
	first.State.ActiveProfile = prof.Name
	first.savePrefs()
	first.SelectDevice(xboxmock.Key)

	second := newTestApp(t, dirs, Options{ForceMock: true})
	if second.State.Mode != state.ModeAdvanced {
		t.Fatalf("mode should persist, got %s", second.State.Mode)
	}
	if second.State.ActiveDeviceKey != xboxmock.Key {
		t.Fatalf("selected device should persist, got %q", second.State.ActiveDeviceKey)
	}
	second.SelectDevice(dualsense.MockKey)
	if second.State.ActiveProfile != "Competitive FPS" || second.State.Config.Lighting.Red != prof.Config.Lighting.Red {
		t.Fatalf("DualSense active profile should persist, got %q", second.State.ActiveProfile)
	}
}

func TestNoControllerFallback(t *testing.T) {
	application := newTestApp(t, newDirs(t), Options{AllowMock: false})
	s := application.State
	if s.ActiveDeviceKey != "" || s.DeviceInfo.Connected {
		t.Fatalf("expected no active controller")
	}
	panels := s.VisiblePanels()
	if len(panels) != 2 {
		t.Fatalf("without a controller only overview and settings should be offered, got %d", len(panels))
	}

	page := uitest.Run(t, 120, 40, application.Draw)
	page.Expect(page.GetByID("empty_state_banner")).ToBeVisible()
	page.Expect(page.GetByRole("button", "Xbox (Test)")).ToBeVisible()

	page.Press("D")
	if s.ActiveDeviceKey != dualsense.MockKey || !s.DeviceInfo.Connected {
		t.Fatalf("[D] should attach the simulated DualSense, got %q", s.ActiveDeviceKey)
	}
	page.Expect(page.GetByID("empty_state_banner")).Not().ToBeVisible()
}
