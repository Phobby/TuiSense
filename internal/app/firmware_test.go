package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/firmware/sony"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/uitest"
)

const mockCatalog = `{"FwUpdate0004LatestVersion":"0x0630","FwUpdate000BLatestVersion":"0x0630",
	"FwUpdate000ELatestVersion":"0x0641","FwUpdate0044LatestVersion":"0x0217"}`

// countingFetcher serves mock metadata, optionally blocking until released.
type countingFetcher struct {
	calls   atomic.Int32
	release chan struct{}
}

func (c *countingFetcher) Fetch(ctx context.Context) (sony.Catalog, error) {
	c.calls.Add(1)
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return sony.Catalog{}, ctx.Err()
		}
	}
	return sony.ParseCatalog([]byte(mockCatalog))
}

// expectScreen waits until the rendered screen contains text.
func expectScreen(t *testing.T, page *uitest.Page, text string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(page.Screen(), text) {
		if time.Now().After(deadline) {
			t.Fatalf("screen never showed %q:\n%s", text, page.Screen())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func newFirmwareApp(t *testing.T, mode state.UXMode, f *countingFetcher, opts Options) *Application {
	t.Helper()
	opts.ForceMock, opts.AllowMock = true, true
	opts.FirmwareProviders = []firmware.Provider{sony.New(f)}
	a := newTestApp(t, newDirs(t), opts)
	a.State.SetMode(mode)
	return a
}

func TestFirmwareCheckDoesNotBlockTheUI(t *testing.T) {
	f := &countingFetcher{release: make(chan struct{})}
	a := newFirmwareApp(t, state.ModeEZ, f, Options{})
	a.Manager.Active().(*dualsense.MockBackend).SetFirmwareIdentity(
		device.FirmwareIdentity{Read: true, UpdateVersion: 0x0458, HasUpdateVersion: true})
	a.refreshFirmware()
	page := uitest.Run(t, 120, 40, a.Draw)

	page.Press("f")
	if a.State.ActivePanel != state.PanelFirmware {
		t.Fatalf("EZ Device Details tab should open the panel, got %s", a.State.ActivePanel)
	}
	page.Expect(page.GetByID("firmware_status")).ToHaveValue("not_checked")
	page.GetByID("btn_firmware_check").Click()

	// The fetch is blocked, yet frames keep rendering and show progress.
	start := time.Now()
	page.Expect(page.GetByID("firmware_status")).ToHaveValue("checking")
	if time.Since(start) > 2*time.Second {
		t.Fatal("the UI must keep rendering while the network check runs")
	}
	page.GetByID("btn_firmware_check").Click() // ignored while running
	close(f.release)

	page.Expect(page.GetByID("firmware_status")).ToHaveValue("update_available")
	expectScreen(t, page, "Simulated controller")
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("expected exactly one metadata fetch, got %d", n)
	}
	r := a.State.Firmware.Result
	if r == nil || r.Installed != "0x0458" || r.Available != "0x0630–0x0641" || r.Notify {
		t.Fatalf("unexpected result %+v (simulated devices never notify)", r)
	}
	if a.State.Firmware.Last == nil {
		t.Fatal("a successful check must be persisted")
	}
}

func TestFirmwareAdvancedGuideAndDetails(t *testing.T) {
	f := &countingFetcher{}
	var opened, copied []string
	a := newFirmwareApp(t, state.ModeAdvanced, f, Options{
		OpenURL:  func(u string) error { opened = append(opened, u); return nil },
		CopyText: func(s string) error { copied = append(copied, s); return nil },
	})
	a.State.ActivePanel = state.PanelFirmware
	page := uitest.Run(t, 140, 50, a.Draw)

	page.GetByID("fw_section_details").Click()
	expectScreen(t, page, "Software series (raw)")
	expectScreen(t, page, "Read from simulated report 0x20, bytes 44–45.")
	page.GetByID("fw_detail_sw_series").Click()
	expectScreen(t, page, "About Software series (raw)")
	expectScreen(t, page, "No source documents a link")
	page.Expect(page.GetByID("fw_autocheck_on")).ToBeVisible()
	page.GetByID("fw_group_hardware").Click()
	expectScreen(t, page, "Board revision")
	expectScreen(t, page, "Hardware version (raw)")

	page.GetByID("fw_section_guide").Click()
	if runtime.GOOS == "linux" {
		expectScreen(t, page, "Sony does not currently provide")
	}
	expectScreen(t, page, "PS5 console")
	page.GetByID("fw_section_links").Click()
	page.GetByID("btn_firmware_open_0").Click()
	page.GetByID("btn_firmware_copy_1").Click()
	waitForCopy(t, a)
	if len(opened) != 1 || opened[0] != sony.PCAccessoriesURL {
		t.Fatalf("open: %v", opened)
	}
	if len(copied) != 1 || copied[0] != sony.SupportURL {
		t.Fatalf("copy: %v", copied)
	}
	a.OpenOfficialLink("https://example.com/flash-tool")
	if len(opened) != 1 {
		t.Fatal("only official links may be opened")
	}
	if f.calls.Load() != 0 {
		t.Fatal("opening instructions must never start a check or a download")
	}
}

func TestFirmwareOnlyForSupportedDevices(t *testing.T) {
	f := &countingFetcher{}
	a := newFirmwareApp(t, state.ModeAdvanced, f, Options{})
	if !state.PanelVisible(state.VisiblePanels(a.State.Caps, state.ModeAdvanced), state.PanelFirmware) || !a.State.Firmware.Supported {
		t.Fatal("the mock DualSense supports the firmware check")
	}
	a.SelectDevice(xboxmock.Key)
	if !state.PanelVisible(state.VisiblePanels(a.State.Caps, state.ModeAdvanced), state.PanelFirmware) || a.State.Firmware.Supported {
		t.Fatal("the Xbox test device shows device details but must not offer a firmware check")
	}
	a.State.ActivePanel = state.PanelFirmware
	page := uitest.Run(t, 140, 50, a.Draw)
	expectScreen(t, page, "Firmware update checks are not available")
	page.Expect(page.GetByID("btn_firmware_check")).Not().ToBeVisible()
	a.CheckFirmware()
	time.Sleep(20 * time.Millisecond)
	if f.calls.Load() != 0 {
		t.Fatal("no check may run for an unsupported device")
	}
}

func TestFirmwareWithNoConnectedController(t *testing.T) {
	f := &countingFetcher{}
	a := newTestApp(t, newDirs(t), Options{
		Registry:          drivers.NewBuiltinRegistry(drivers.RegistryOptions{}),
		FirmwareProviders: []firmware.Provider{sony.New(f)},
	})
	page := uitest.Run(t, 120, 40, a.Draw)
	page.Expect(page.GetByRole("tab", "Device Details")).Not().ToBeVisible()
	a.CheckFirmware()
	time.Sleep(20 * time.Millisecond)
	if a.State.Firmware.Supported || f.calls.Load() != 0 {
		t.Fatal("nothing to check without a controller")
	}
}

func TestFirmwareUpdateNotification(t *testing.T) {
	a := newFirmwareApp(t, state.ModeEZ, &countingFetcher{}, Options{})
	countToasts := func() int {
		n := 0
		for _, ti := range a.State.Toasts.Toasts {
			if ti.Title == "Firmware update available" {
				n++
			}
		}
		return n
	}
	res := firmware.Result{Key: a.State.ActiveDeviceKey, Status: firmware.StatusUpdateAvailable,
		Installed: "0x0154", Available: "0x0217", CatalogOK: true}
	a.queueFirmwareResult(res) // already notified earlier: Notify=false
	a.drainFirmware()
	if countToasts() != 0 {
		t.Fatal("no notification without Notify")
	}
	res.Notify = true
	a.queueFirmwareResult(res)
	a.drainFirmware()
	if countToasts() != 1 {
		t.Fatal("a verified, new update must show one notification")
	}
	if a.State.ActivePanel == state.PanelFirmware {
		t.Fatal("a notification must not switch panels or start anything")
	}
}

func TestFirmwareAutoCheckSkipsSimulatedAndPersistsPreference(t *testing.T) {
	f := &countingFetcher{}
	dirs := newDirs(t)
	a := newTestApp(t, dirs, Options{ForceMock: true, AllowMock: true, FirmwareProviders: []firmware.Provider{sony.New(f)}})
	a.started = true
	a.maybeAutoCheck()
	time.Sleep(20 * time.Millisecond)
	if f.calls.Load() != 0 {
		t.Fatal("automatic checks never run for simulated controllers")
	}
	a.SetFirmwareAutoCheck(false)
	data, err := os.ReadFile(filepath.Join(dirs.config, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p state.Preferences
	if err := json.Unmarshal(data, &p); err != nil || p.FirmwareAutoCheck == nil || *p.FirmwareAutoCheck {
		t.Fatalf("auto-check preference not persisted: %s", data)
	}
	b := newTestApp(t, dirs, Options{ForceMock: true, AllowMock: true})
	if b.fwAutoCheck {
		t.Fatal("the disabled auto-check must survive a restart")
	}
}

func TestUpdateStatusRendering(t *testing.T) {
	a := newFirmwareApp(t, state.ModeEZ, &countingFetcher{}, Options{})
	a.State.ActivePanel = state.PanelFirmware
	page := uitest.Run(t, 120, 46, a.Draw)
	expectScreen(t, page, "Installed version")
	expectScreen(t, page, "Firmware target")
	expectScreen(t, page, "Unknown")
	expectScreen(t, page, "Not checked")

	key := a.State.ActiveDeviceKey
	conn := a.Manager.Active().Info().Connection
	cases := []struct {
		r    firmware.Result
		want []string
	}{
		{firmware.Result{Status: firmware.StatusUpdateAvailable, Installed: "0x0630", Available: "0x0641"}, []string{"⬆ Update available", "0x0641", "requires Sony's official updater"}},
		{firmware.Result{Status: firmware.StatusUpToDate, Installed: "0x0630", Available: "0x0630"}, []string{"✓ Up to date"}},
		{firmware.Result{Status: firmware.StatusUndetermined, Installed: "0x0630", Basis: "target 000E lists a newer version"}, []string{"? Unable to verify", "not verified", "target 000E lists a newer version", "To be sure"}},
		{firmware.Result{Status: firmware.StatusUnsupported, Installed: "0x0630"}, []string{"✕ Unsupported device revision"}},
		{firmware.Result{Status: firmware.StatusNetworkUnavailable, Installed: "0x0630"}, []string{"⚠ Network error"}},
		{firmware.Result{Status: firmware.StatusSourceUnavailable, Installed: "0x0630"}, []string{"⚠ Metadata unavailable"}},
	}
	for _, c := range cases {
		c.r.Key, c.r.Connection = key, conn
		a.queueFirmwareResult(c.r)
		page.Expect(page.GetByID("firmware_status")).ToHaveValue(string(c.r.Status))
		for _, w := range c.want {
			expectScreen(t, page, w)
		}
	}
}

func TestStaleResultIsDroppedAfterReconnect(t *testing.T) {
	a := newFirmwareApp(t, state.ModeEZ, &countingFetcher{}, Options{})
	key := a.State.ActiveDeviceKey
	a.queueFirmwareResult(firmware.Result{Key: key, Status: firmware.StatusUpToDate, Installed: "0x0630", Connection: device.ConnBluetooth})
	a.drainFirmware()
	if a.State.Firmware.Result != nil {
		t.Fatal("a result from another connection type no longer applies")
	}
	a.queueFirmwareResult(firmware.Result{Key: key, Status: firmware.StatusUpToDate, Installed: "0x0500", Connection: device.ConnUSB})
	a.drainFirmware()
	if a.State.Firmware.Result != nil {
		t.Fatal("a result for another installed version no longer applies")
	}
	a.queueFirmwareResult(firmware.Result{Key: key, Status: firmware.StatusUpToDate, Installed: "0x0630", Connection: device.ConnUSB})
	a.drainFirmware()
	if a.State.Firmware.Result == nil {
		t.Fatal("a matching result must be shown")
	}
}

func TestActiveControllerSwitchingUpdatesDetails(t *testing.T) {
	a := newFirmwareApp(t, state.ModeAdvanced, &countingFetcher{}, Options{})
	a.State.ActivePanel = state.PanelFirmware
	if d, ok := a.State.Firmware.Detail("fw_version"); !ok || d.Value != "0x0630" || !a.State.Firmware.Supported {
		t.Fatalf("DualSense details: %+v", a.State.Firmware.Details)
	}
	a.SelectDevice(xboxmock.Key)
	fv := a.State.Firmware
	if fv.Supported {
		t.Fatal("the Xbox test device has no firmware check")
	}
	if d, ok := fv.Detail("model"); !ok || !strings.Contains(d.Value, "Xbox") {
		t.Fatalf("details must follow the active controller, got %+v", fv.Details)
	}
	if _, ok := fv.Detail("sw_series"); ok {
		t.Fatal("DualSense-only readings must not leak to another controller")
	}
	page := uitest.Run(t, 120, 46, a.Draw)
	expectScreen(t, page, "Firmware update checks are not available")
	a.SelectDevice(dualsense.MockKey)
	if !a.State.Firmware.Supported {
		t.Fatal("switching back must restore the DualSense check")
	}
	page.Expect(page.GetByID("btn_firmware_check")).ToBeVisible()
}
