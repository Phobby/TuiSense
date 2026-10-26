package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/testkit"
	"github.com/thebanri/limoni/uitest"
)

func TestNormalStartupHasNoVirtualFactoriesOrImaginaryController(t *testing.T) {
	a := newTestApp(t, newDirs(t), Options{Registry: drivers.NewBuiltinRegistry(drivers.RegistryOptions{})})
	if len(a.Manager.Devices()) != 0 || a.Manager.Active() != nil {
		t.Fatal("normal startup attached a virtual controller")
	}
	term := testkit.NewTerminal(80, 24)
	term.Draw(func(f *terminal.Frame) { a.Draw(f, nil) })
	snapshot := term.Snapshot()
	if !strings.Contains(snapshot, "No controller connected") || !strings.Contains(snapshot, "Connect a supported controller") {
		t.Fatal("clean empty state missing")
	}
	for _, r := range snapshot {
		if r >= 0x2801 && r <= 0x28ff {
			t.Fatal("imaginary controller rendered")
		}
	}
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyRune, Ch: 'D'})
	if a.Manager.Active() != nil {
		t.Fatal("undocumented shortcut enabled mock mode")
	}
}

func TestExplicitDemoHasOnlyOneClearlyMarkedController(t *testing.T) {
	a := newTestApp(t, newDirs(t), Options{ForceMock: true, Registry: drivers.NewBuiltinRegistry(drivers.RegistryOptions{Mock: true})})
	if len(a.State.Devices) != 1 || !a.State.IsSimulatedDevice() {
		t.Fatal("explicit demo must have one simulated controller")
	}
	term := testkit.NewTerminal(80, 24)
	term.Draw(func(f *terminal.Frame) { a.Draw(f, nil) })
	if !strings.Contains(term.Snapshot(), "MOCK") {
		t.Fatal("demo source must be visible")
	}
}

func TestHelpScrollCapturesKeysAndCloses(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 80, 24, a.Draw)
	page.Press("?")
	if !a.State.ShowHelp || a.State.HelpScrollMax == 0 {
		t.Fatal("help must scroll at 80x24")
	}
	original := a.State.RotY
	page.Press("end")
	if a.State.HelpScroll != a.State.HelpScrollMax {
		t.Fatal("End did not reach help bottom")
	}
	page.Press("d")
	if a.State.RotY != original {
		t.Fatal("help leaked camera shortcut")
	}
	page.Press("home")
	if a.State.HelpScroll != 0 {
		t.Fatal("Home did not return to first help section")
	}
	page.Press("escape")
	if a.State.ShowHelp {
		t.Fatal("Escape did not close help")
	}
	page.Press("?")
	page.Press("?")
	if a.State.ShowHelp {
		t.Fatal("? did not toggle help closed")
	}
}

func TestMinimalHomeAndResponsivePages(t *testing.T) {
	for _, size := range [][2]uint16{{80, 24}, {100, 30}, {120, 40}, {160, 45}, {220, 60}} {
		for _, mode := range []state.UXMode{state.ModeEZ, state.ModeAdvanced} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], mode), func(t *testing.T) {
				a := newMockApp(t, mode)
				term := testkit.NewTerminal(size[0], size[1])
				for _, entry := range a.State.VisiblePanels() {
					a.State.ActivePanel = entry.Spec.ID
					term.Draw(func(f *terminal.Frame) {
						a.Draw(f, nil)
						for _, node := range f.AccessibilityTree() {
							if node.Bounds.Width > 0 && node.Bounds.Height > 0 && node.Bounds.Intersection(f.Area()) != node.Bounds {
								t.Errorf("%s: node %s outside terminal: %+v", entry.Spec.ID, node.ID, node.Bounds)
							}
						}
					})
					snap := term.Snapshot()
					lines := strings.Split(strings.TrimSuffix(snap, "\n"), "\n")
					if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "? Help" {
						t.Errorf("%s: footer contains permanent clutter: %q", entry.Spec.ID, lines[len(lines)-1])
					}
					if entry.Spec.ID == state.PanelOverview {
						for _, unwanted := range []string{"MANAGER", "Quick Settings", "VID", "Packets", "Gyro", "Click a part"} {
							if strings.Contains(snap, unwanted) {
								t.Errorf("home leaks %q", unwanted)
							}
						}
						if countBraille(t, a, size[0], size[1]) < 40 {
							t.Fatal("3D visual is not prominent")
						}
					}
					if dir := os.Getenv("TUISENSE_UI_ARTIFACTS"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						name := fmt.Sprintf("%dx%d-%s-%s.txt", size[0], size[1], mode, entry.Spec.ID)
						if err := os.WriteFile(filepath.Join(dir, name), []byte(snap), 0644); err != nil {
							t.Fatal(err)
						}
					}
				}
			})
		}
	}
}

func TestContextualSettingsAndKeyboardActions(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	page := uitest.Run(t, 80, 24, a.Draw)
	page.Expect(page.GetByID("ez_vib_Off")).Not().ToBeVisible()
	page.GetByID("home_customize").Click()
	page.GetByID("nav_haptics").Click()
	page.Expect(page.GetByID("ez_vib_Off")).ToBeVisible()
	page.Expect(page.GetByID("ez_trig_Weapon")).Not().ToBeVisible()
	page.Press("e")
	if a.State.Mode != state.ModeAdvanced {
		t.Fatal("mode shortcut failed")
	}
	page.Press("pgdn")
	if a.State.PanelScroll == 0 {
		t.Fatal("short forms must scroll")
	}
	page.Press("1")
	a.State.SelectRegion(device.RegionRightTrigger)
	page.Press("tab")
	if a.State.ActivePanel != state.PanelTriggers || !a.State.EditingRightTrigger {
		t.Fatal("selected component did not open its own controls")
	}
}

func TestProjectedMouseSelectionAndHover(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	term := testkit.NewTerminal(160, 45)
	term.Draw(func(f *terminal.Frame) {
		a.Draw(f, nil)
		for _, region := range f.EventRegions {
			if region.ID != "hover_"+string(device.RegionLeftTrigger) {
				continue
			}
			ev := driver.MouseEvent{X: region.Area.X + 2, Y: region.Area.Y + 1}
			f.DispatchPointerMove(ev)
			if a.State.HoveredRegion != device.RegionLeftTrigger {
				t.Fatal("projected region did not highlight on hover")
			}
			ev.Button = driver.MouseLeft
			f.DispatchEventRegions(ev)
			if a.State.ActivePanel != state.PanelTriggers {
				t.Fatal("projected click did not open trigger inspector")
			}
			return
		}
		t.Fatal("3D projected interaction region missing")
	})
}
