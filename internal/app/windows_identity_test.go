package app

import (
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/winpad"
	"github.com/Phobby/TuiSense/internal/drivers/xinput"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/system/winhid"
	"github.com/thebanri/limoni/uitest"
)

// oneSlot is an XInput API with one wired controller in slot 1.
type oneSlot struct{}

func (oneSlot) GetState(slot uint32) (xinput.State, error) {
	if slot == 0 {
		return xinput.State{}, nil
	}
	return xinput.State{}, xinput.ErrNotConnected
}
func (oneSlot) SetState(uint32, uint16, uint16) error { return nil }
func (oneSlot) Battery(uint32) (uint8, uint8, error)  { return xinput.BatteryWired, 0, nil }

func windowsApp(t *testing.T, hid []winhid.Device, withXInput bool) *Application {
	t.Helper()
	gamepad.SetOverrides(nil)
	list := func() ([]winhid.Device, error) { return hid, nil }
	reg := device.NewRegistry()
	if withXInput {
		_ = reg.RegisterDriver(xinput.NewDriverWith(oneSlot{}, list, drivers.WindowsModels()...))
	}
	_ = reg.RegisterDriver(winpad.NewDriverWith(drivers.WindowsModels(), list))
	a := newTestApp(t, newDirs(t), Options{Registry: reg})
	a.State.SetMode(state.ModeAdvanced)
	return a
}

// The reported Windows bug: an MSI Force GC20 V2 in Default (XInput) mode
// must be detected, must show neither DualSense nor Microsoft branding, and
// must offer the MSI only as a confirmation.
func TestWindowsMSIInXInputModeNeverShowsADualSense(t *testing.T) {
	a := windowsApp(t, []winhid.Device{{Path: `\\?\hid#vid_045e&pid_028e&ig_00#1`, VendorID: 0x045e, ProductID: 0x028e, Version: 0x0210,
		UsagePage: 1, Usage: 5, Product: "Controller (XBOX 360 For Windows)"}}, true)
	page := uitest.Run(t, 160, 48, a.Draw)
	page.Expect(page.GetByRole("image", "Generic Gamepad")).ToBeVisible()
	if a.State.Descriptor.VisualID != "generic" || strings.Contains(a.State.Descriptor.DisplayName, "DualSense") {
		t.Fatalf("got %s / %s", a.State.Descriptor.DisplayName, a.State.Descriptor.VisualID)
	}
	if a.State.DeviceInfo.IdentityCandidateName != "MSI Force GC20 V2" {
		t.Fatal("the MSI is offered for confirmation")
	}
	for _, p := range a.State.VisiblePanels() {
		if p.Spec.ID == state.PanelCustomize {
			t.Fatal("no DualSense-style settings for an XInput pad")
		}
	}
	page.Expect(page.GetByRole("tab", "2 Diagnostics")).ToBeVisible()
}

// In DirectInput/Android mode the MSI is invisible to XInput. It is still
// detected and named from its USB strings, but nothing pretends to work.
func TestWindowsMSIInHIDModeIsDetectedWithoutFakeFeatures(t *testing.T) {
	a := windowsApp(t, []winhid.Device{{Path: `\\?\hid#vid_0db0&pid_0001#1`, VendorID: 0x0db0, ProductID: 0x0001, Version: 1,
		UsagePage: 1, Usage: 4, Manufacturer: "MSI", Product: "Force GC20 V2"}}, false)
	page := uitest.Run(t, 160, 48, a.Draw)
	page.Expect(page.GetByRole("image", "MSI Force GC20 V2")).ToBeVisible()
	for _, p := range a.State.VisiblePanels() {
		switch p.Spec.ID {
		case state.PanelDiagnostics, state.PanelCustomize:
			t.Fatalf("panel %s needs input TuiSense cannot read here", p.Spec.ID)
		}
	}
	if !strings.Contains(page.Screen(), "Controller detected, but input access is unavailable.") {
		t.Fatalf("the missing input access must be explained on the home screen:\n%s", page.Screen())
	}
}

// With nothing connected the stage is the neutral empty state, not a model.
func TestWindowsWithoutControllerShowsNoModel(t *testing.T) {
	a := windowsApp(t, nil, false)
	page := uitest.Run(t, 160, 48, a.Draw)
	page.Expect(page.GetByID("empty_state_banner")).ToBeVisible()
	if a.State.ActiveDeviceKey != "" || a.State.Descriptor.VisualID != "generic" {
		t.Fatalf("no device may be invented: %q %q", a.State.ActiveDeviceKey, a.State.Descriptor.VisualID)
	}
}
