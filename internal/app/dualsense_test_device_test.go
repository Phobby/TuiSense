package app

import (
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/uitest"
)

func TestDualSenseTestDeviceSelectableWithoutMockFallback(t *testing.T) {
	a := newTestApp(t, newDirs(t), Options{AllowMock: false})
	if a.Manager.Device(dualsense.TestKey) == nil {
		t.Fatal("test DualSense must be listed without fallback")
	}
	if a.State.ActiveDeviceKey != "" {
		t.Fatal("test devices must never pretend to be automatically detected hardware")
	}
	page := uitest.Run(t, 120, 40, a.Draw)
	page.Press("c")
	page.GetByID("selector_item_1").Click()
	if a.State.ActiveDeviceKey != dualsense.TestKey || a.State.Descriptor.VisualID != "dualsense" || !a.State.Diag.Ctx.Simulated {
		t.Fatal("selection must activate the simulated DualSense with the original model")
	}
	for _, entry := range a.State.Devices {
		if entry.Key == dualsense.TestKey && entry.Source != device.SourceTest {
			t.Fatal("source must remain Test")
		}
	}
	a.State.ActivePanel = state.PanelDiagnostics
	d := a.State.Diag
	if !d.Avail[diag.CatSticks].Available() || !d.Avail[diag.CatTriggers].Available() || !d.Avail[diag.CatButtons].Available() {
		t.Fatal("DualSense input diagnostics must be available")
	}
	a.SelectDevice(dualsense.TestKey)
	if a.Manager.Device(dualsense.MockKey) != nil {
		t.Fatal("selecting the test device must not attach the fallback mock")
	}
}
