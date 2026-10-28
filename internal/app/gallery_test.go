package app

import (
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/ui/visual"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/testkit"
	"github.com/thebanri/limoni/uitest"
)

func draw(a *Application, w, h uint16) (*testkit.Terminal, string) {
	term := testkit.NewTerminal(w, h)
	term.Draw(func(f *terminal.Frame) { a.Draw(f, nil) })
	return term, term.Snapshot()
}

// The gallery works without any controller and never injects devices.
func TestGalleryWorksWithoutHardwareAndInjectsNoDevices(t *testing.T) {
	a := newTestApp(t, newDirs(t), Options{Registry: drivers.NewBuiltinRegistry(drivers.RegistryOptions{})})
	_, snap := draw(a, 120, 40)
	if !strings.Contains(snap, "controller gallery") {
		t.Fatal("empty state should point to the gallery")
	}
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyRune, Ch: 'G'})
	if !a.State.Gallery.Active {
		t.Fatal("G must open the gallery")
	}
	seen := map[string]bool{}
	for i := 0; i < len(visual.Gallery()); i++ {
		_, snap = draw(a, 120, 40)
		if !strings.Contains(snap, "VISUAL PREVIEW") || !strings.Contains(snap, "Controller Gallery") {
			t.Fatal("previews must be labelled as visual previews")
		}
		if countBraille(t, a, 120, 40) < 150 {
			t.Fatalf("model %s is not rendered prominently", a.State.StageModelID())
		}
		seen[a.State.StageModelID()] = true
		a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyArrowRight})
	}
	if len(seen) != 10 {
		t.Fatalf("browsed %d distinct models, want 10", len(seen))
	}
	if len(a.Manager.Devices()) != 0 || len(a.State.Devices) != 0 || a.State.ActiveDeviceKey != "" {
		t.Fatal("the gallery injected a device")
	}
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyEsc})
	if a.State.Gallery.Active {
		t.Fatal("Esc must return from the gallery")
	}
	if _, snap = draw(a, 120, 40); !strings.Contains(snap, "No controller connected") {
		t.Fatal("leaving the gallery must restore the empty state")
	}
}

// Real-hardware support claims come from drivers; demo-only models say so.
func TestGalleryDistinguishesSupportedHardwareFromPreviews(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	active := a.State.ActiveDeviceKey
	a.State.OpenGallery()
	for i, spec := range visual.Gallery() {
		a.State.Gallery.Index = i
		_, snap := draw(a, 160, 45)
		supported := len(spec.HardwareModelIDs) > 0
		if strings.Contains(snap, "supported by a TuiSense driver") != supported {
			t.Errorf("%s: hardware support line wrong (supported=%v)", spec.ID, supported)
		}
		if strings.Contains(snap, "Visual preview only") == supported {
			t.Errorf("%s: preview-only line wrong", spec.ID)
		}
	}
	a.State.CloseGallery()
	if a.State.ActiveDeviceKey != active {
		t.Fatal("returning from the gallery must keep the connected device")
	}
}

func TestGalleryRearViewAndComponentInspector(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 160, 45, a.Draw)
	page.Press("G")
	for a.State.StageModelID() != "xbox_elite2" {
		page.Press("right")
	}
	page.Press("v")
	for i := 0; i < 40 && a.State.CameraTween; i++ {
		a.State.StepCamera(1.0 / 30)
	}
	if !a.State.ViewingRear() {
		t.Fatal("V must rotate to the rear preset")
	}
	page.Expect(page.GetByID("component_paddle_p1")).ToBeVisible()
	page.GetByID("component_paddle_p1").Click()
	if a.State.SelectedComponent != "paddle_p1" {
		t.Fatalf("clicking P1 selected %q", a.State.SelectedComponent)
	}
	if _, snap := draw(a, 160, 45); !strings.Contains(snap, "P1 · Rear paddle · Rear · input mapped at connection time") {
		t.Fatalf("inspector must state how P1 is read:\n%s", snap)
	}
	page.Press("v")
	for i := 0; i < 40 && a.State.CameraTween; i++ {
		a.State.StepCamera(1.0 / 30)
	}
	if a.State.ViewingRear() {
		t.Fatal("V must return to the front")
	}
}

func TestStageDragRotatesAndDoubleClickResets(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	term := testkit.NewTerminal(120, 40)
	term.Draw(func(f *terminal.Frame) {
		a.Draw(f, nil)
		var area accessibility.AccessibilityNode
		for _, n := range f.AccessibilityTree() {
			if n.ID == "stage_3d" {
				area = n
			}
		}
		for _, region := range f.EventRegions {
			if region.ID != uiwidgets.StagePointerID {
				continue
			}
			x, y := area.Bounds.X+2, area.Bounds.Y+2
			before := a.State.RotY
			f.DispatchEventRegions(driver.MouseEvent{Button: driver.MouseLeft, X: x, Y: y})
			capture := f.TakeMouseCapture()
			capture(driver.MouseEvent{Button: driver.MouseLeft, Drag: true, X: x + 10, Y: y})
			capture(driver.MouseEvent{Button: driver.MouseRelease, X: x + 10, Y: y})
			if a.State.RotY == before {
				t.Fatal("dragging must rotate the model")
			}
			a.State.StageClick.At = time.Time{}
			f.DispatchEventRegions(driver.MouseEvent{Button: driver.MouseLeft, X: x, Y: y})
			f.TakeMouseCapture()
			f.DispatchEventRegions(driver.MouseEvent{Button: driver.MouseLeft, X: x, Y: y})
			if a.State.RotY != a.State.StageSpec().Front.RotY {
				t.Fatal("double-click must reset the camera")
			}
			f.DispatchEventRegions(driver.MouseEvent{Button: driver.MouseScrollUp, X: x, Y: y})
			if a.State.Zoom <= 1 {
				t.Fatal("the wheel must zoom")
			}
			return
		}
		t.Fatal("stage pointer region missing")
	})
}

// Only inputs the active driver reports are treated as live; gallery
// previews never are, even for the same model.
func TestLiveInputBindingFollowsTheDriver(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	cross, _ := a.State.StageSpec().Component("btn_cross")
	if !a.State.ComponentReported(cross) {
		t.Fatal("the DualSense driver reports Cross; the component must be live")
	}
	a.State.OpenGallery()
	if a.State.StageModelID() != "dualsense" {
		t.Fatalf("the gallery should open on the connected model, got %s", a.State.StageModelID())
	}
	if a.State.ComponentReported(cross) {
		t.Fatal("gallery previews must never claim live input")
	}
}

func TestSettingsOpensTheGallery(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	page := uitest.Run(t, 120, 40, a.Draw)
	a.State.ActivePanel = state.PanelSettings
	page.GetByID("btn_open_gallery").Click()
	if !a.State.Gallery.Active {
		t.Fatal("the settings entry must open the gallery")
	}
}

func BenchmarkHomeFrame(b *testing.B) {
	t := &testing.T{}
	a := newMockApp(t, state.ModeEZ)
	term := testkit.NewTerminal(160, 45)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		term.Draw(func(f *terminal.Frame) { a.Draw(f, nil) })
	}
}

// The gallery stays centred and inside the terminal from 80×24 to 220×60,
// in both modes, from any panel and in front and rear views.
func TestGalleryResponsiveLayouts(t *testing.T) {
	for _, size := range [][2]uint16{{80, 24}, {100, 30}, {120, 40}, {160, 45}, {220, 60}} {
		for _, mode := range []state.UXMode{state.ModeEZ, state.ModeAdvanced} {
			a := newMockApp(t, mode)
			a.State.ActivePanel = state.PanelDiagnostics
			a.State.OpenGallery()
			for i := range visual.Gallery() {
				a.State.Gallery.Index = i
				for _, rear := range []bool{false, true} {
					a.State.SetView(rear)
					a.State.RotX, a.State.RotY, a.State.CameraTween = a.State.TargetRotX, a.State.TargetRotY, false
					term := testkit.NewTerminal(size[0], size[1])
					term.Draw(func(f *terminal.Frame) {
						a.Draw(f, nil)
						for _, node := range f.AccessibilityTree() {
							if node.Bounds.Width > 0 && node.Bounds.Intersection(f.Area()) != node.Bounds {
								t.Errorf("%v %s: node %s outside the terminal", size, mode, node.ID)
							}
						}
					})
					if n := countBraille(t, a, size[0], size[1]); n < 60 {
						t.Errorf("%v %s %s rear=%v: model barely visible (%d cells)", size, mode, a.State.StageModelID(), rear, n)
					}
				}
			}
		}
	}
}
