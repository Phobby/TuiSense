package views

import (
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/theme"
	"github.com/thebanri/limoni/core/buffer"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

func reportState() *state.AppState {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	s := &state.AppState{ActivePanel: state.PanelDiagnostics, Descriptor: device.Descriptor{DisplayName: "Universal Controller", Brand: "Example", ModelID: "universal"}, DeviceInfo: device.Info{FirmwareVersion: "0x0630", HardwareVersion: "0x00000413", Connection: device.ConnBluetooth, MACAddress: "AA:BB:CC:DD:EE:FF", ID: "SERIAL-12345", InputDevicePath: "/dev/input/event7"}, LastError: "Disconnected AA:BB:CC:DD:EE:FF serial: SERIAL-12345", LastErrorAt: now}
	s.Firmware.Details = []device.Detail{{Key: "serial_number", Label: "Serial number", Value: "SERIAL-12345"}, {Key: "sw_series", Label: "Software series", Value: "0x0123"}, {Key: "revision", Label: "Board revision", Unavailable: true, Value: "not reported"}}
	s.Firmware.Result = &firmware.Result{Status: firmware.StatusUndetermined, Summary: "Cannot verify installed firmware", CheckedAt: now}
	s.Diag = &diag.Session{Ctx: diag.DeviceContext{Desc: s.Descriptor, Info: s.DeviceInfo}, Active: diag.CatSticks, Avail: map[diag.Category]diag.Availability{}, Results: map[diag.Category]diag.Result{diag.CatSticks: {Category: diag.CatSticks, Status: diag.Warning, Summary: "Possible drift detected", Measurements: []diag.Measurement{{Name: "Left center offset", Value: 3.2, Unit: "%"}}, Details: []string{"Release sticks before measuring"}, Limitations: []string{"A preview is not hardware calibration"}, At: now}}}
	for _, cat := range diag.AllCategories {
		s.Diag.Avail[cat] = diag.Availability{Category: cat, Status: diag.Unsupported, Reason: "Capability not reported"}
	}
	s.Diag.Avail[diag.CatSticks] = diag.Availability{Category: diag.CatSticks, Status: diag.NotTested}
	s.Diag.Avail[diag.CatButtons] = diag.Availability{Category: diag.CatButtons, Status: diag.NotTested}
	return s
}
func TestClipboardReportFormattingMissingValuesAndPrivacy(t *testing.T) {
	s := reportState()
	report := BuildClipboardReport(s, false, "test", s.LastErrorAt)
	if strings.Contains(report, "Capability not reported") {
		t.Errorf("hardware the controller does not have must not appear in reports:\n%s", report)
	}
	for _, want := range []string{"TuiSense Diagnostic Report", "Universal Controller", "Example", "0x0630", "0x00000413", "3.20 %", "Possible drift detected", "Release sticks before measuring", "Not measured", "Unavailable: not reported", "2026-10-25T10:00:00Z", "0x0123", "/dev/input/event7"} {
		if !strings.Contains(report, want) {
			t.Errorf("missing %q:\n%s", want, report)
		}
	}
	for _, secret := range []string{"AA:BB:CC:DD:EE:FF", "SERIAL-12345"} {
		if strings.Contains(report, secret) {
			t.Errorf("leaked %s", secret)
		}
	}
	if strings.Contains(report, "╭") || strings.Contains(report, "⧉") {
		t.Fatal("decorative UI in report")
	}
	s.IncludeCopyIdentifiers = true
	report = BuildClipboardReport(s, false, "", s.LastErrorAt)
	if !strings.Contains(report, "AA:BB:CC:DD:EE:FF") || !strings.Contains(report, "SERIAL-12345") {
		t.Fatal("explicit opt-in ignored")
	}
}
func TestCurrentTestReportAndNoSession(t *testing.T) {
	s := reportState()
	report := BuildClipboardReport(s, true, "", s.LastErrorAt)
	if strings.Contains(report, "Buttons:") || !strings.Contains(report, "3.20 %") {
		t.Fatal("current report includes unrelated tests or loses measurement")
	}
	s.ActivePanel = state.PanelFirmware
	report = BuildClipboardReport(s, true, "", s.LastErrorAt)
	if strings.Contains(report, "3.20 %") || !strings.Contains(report, "Cannot verify installed firmware") {
		t.Fatal("firmware report scope incorrect")
	}
	s.Diag = nil
	if report = BuildClipboardReport(s, false, "", s.LastErrorAt); !strings.Contains(report, "no measurements available") {
		t.Fatal("missing state fabricated")
	}
}
func TestCursorCopiesExactValueAndEntireWrappedMessage(t *testing.T) {
	f := terminal.NewFrame(buffer.NewBuffer(cell.NewRect(0, 0, 50, 12)), terminal.NewFocusManager())
	s := &state.AppState{Theme: theme.Default()}
	var got string
	s.Copy.OnCopy = func(text string) { got = text }
	s.Copy.Begin("test")
	c := newCursor(f, cell.NewRect(2, 2, 20, 8), s)
	c.kv("Firmware", "0x0630", 10, s.Theme.ValueStyle())
	f.ClickRegions[0].Fire(driver.MouseEvent{Button: driver.MouseLeft}, f)
	if got != "0x0630" {
		t.Fatalf("label or formatting copied: %q", got)
	}
	full := "Diagnostic measurement: 3.200%\nComplete explanation that is far too long to display."
	c.wrap(full, s.Theme.LabelStyle(), 2)
	f.ClickRegions[len(f.ClickRegions)-1].Fire(driver.MouseEvent{Button: driver.MouseLeft}, f)
	if got != full {
		t.Fatalf("wrapped/truncated message lost: %q", got)
	}
}

func TestCircularityPrimaryActionUsesSelectedTest(t *testing.T) {
	desc := dualsense.Descriptor()
	d := diag.NewSession(diag.DeviceContext{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true}, Simulated: true}, nil, nil, nil, nil, time.Now())
	d.Active = diag.CatSticks
	d.StickSubTest = diag.StickSubCircularity
	DiagPrimaryAction(d, time.Now())
	if !d.Sticks.CircularityOnly || d.Sticks.Step != diag.StickRotate {
		t.Fatal("keyboard primary action must start circularity")
	}
	DiagPrimaryAction(d, time.Now())
	if d.Sticks.Running() {
		t.Fatal("second primary action must cancel circularity")
	}
}
