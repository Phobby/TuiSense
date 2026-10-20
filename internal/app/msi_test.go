package app

import (
	"context"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/system/evdev"
	"github.com/thebanri/limoni/uitest"
)

// fakeMSI stands in for an opened MSI Force GC20 V2 (XInput mode, as Linux's
// xpad driver reports it) so the UI can be tested without hardware. Its
// descriptor is built by the real MSI model from an inferred mapping.
type fakeMSI struct {
	desc device.Descriptor
	info device.Info
	tel  device.Telemetry
	cmds []device.Command
}

func xpadCaps() (evdev.InputDevice, map[uint16]evdev.AbsInfo) {
	set := func(codes ...uint16) evdev.Bits {
		b := make(evdev.Bits, 13)
		for _, c := range codes {
			b[c/64] |= 1 << (c % 64)
		}
		return b
	}
	d := evdev.InputDevice{Name: "Microsoft X-Box 360 pad", Bus: evdev.BusUSB, Vendor: 0x045e, Product: 0x028e, Version: 0x0210, KernelDriver: "xpad",
		EventPath: "/dev/input/event23", EventName: "event23",
		Caps: evdev.Capabilities{
			EV: set(evdev.EvSyn, evdev.EvKey, evdev.EvAbs, evdev.EvFF),
			Key: set(evdev.BtnSouth, evdev.BtnEast, evdev.BtnNorth, evdev.BtnWest, evdev.BtnTL, evdev.BtnTR, evdev.BtnSelect,
				evdev.BtnStart, evdev.BtnMode, evdev.BtnThumbL, evdev.BtnThumbR),
			Abs: set(evdev.AbsX, evdev.AbsY, evdev.AbsZ, evdev.AbsRX, evdev.AbsRY, evdev.AbsRZ, evdev.AbsHat0X, evdev.AbsHat0Y),
			FF:  set(evdev.FFRumble),
		}}
	st := evdev.AbsInfo{Minimum: -32768, Maximum: 32767}
	tr := evdev.AbsInfo{Minimum: 0, Maximum: 255}
	h := evdev.AbsInfo{Minimum: -1, Maximum: 1}
	return d, map[uint16]evdev.AbsInfo{evdev.AbsX: st, evdev.AbsY: st, evdev.AbsRX: st, evdev.AbsRY: st, evdev.AbsZ: tr, evdev.AbsRZ: tr, evdev.AbsHat0X: h, evdev.AbsHat0Y: h}
}

func newFakeMSI(_ *logging.Logger) device.Device {
	d, abs := xpadCaps()
	m := gamepad.InferMapping(d, abs)
	f := &fakeMSI{desc: msigc20.Describe(m, d, true)}
	f.info = device.Info{ID: "event23", Name: d.Name, Model: "MSI Force GC20 V2", VendorID: 0x045e, ProductID: 0x028e,
		InputDevicePath: d.EventPath, Connection: device.ConnUSBWired, Connected: true, KernelDriver: "xpad", EventDriven: true,
		IdentityKey: d.HardwareKey(), IdentityState: "probable", Identification: "USB ID 045e:028e version 0210 matches the SDL community entry for the MSI GC20 V2 (shared with other Xbox 360-compatible pads)",
		IdentityCandidateID: msigc20.ModelID, IdentityCandidateName: "MSI Force GC20 V2",
		InputMode: msigc20.ModeHint(m, d), Details: []device.InfoField{{Label: "Kernel driver", Value: "xpad"}, {Label: "USB ID", Value: "045e:028e, bcdDevice 2.10"}}}
	f.tel = device.Telemetry{Raw: device.RawInput{Valid: true}, Samples: []device.RawSample{}}
	return f
}

func (f *fakeMSI) Descriptor() device.Descriptor      { return f.desc }
func (f *fakeMSI) Info() device.Info                  { return f.info }
func (f *fakeMSI) Capabilities() device.CapabilitySet { return f.desc.Capabilities }
func (f *fakeMSI) Telemetry() device.Telemetry        { return f.tel }
func (f *fakeMSI) CurrentConfig() device.Config       { return device.DefaultConfig() }
func (f *fakeMSI) Poll(context.Context) (device.Telemetry, error) {
	f.tel.Samples = []device.RawSample{{At: time.Now(), Held: true}}
	return f.tel, nil
}
func (f *fakeMSI) ApplyConfig(context.Context, device.Config) error { return nil }
func (f *fakeMSI) ExecuteCommand(_ context.Context, c device.Command) error {
	f.cmds = append(f.cmds, c)
	return nil
}
func (f *fakeMSI) Close() error { return nil }

const msiKey = "evdev:/dev/input/event23#msi.force_gc20_v2"

func newMSIApp(t *testing.T, mode state.UXMode) *Application {
	t.Helper()
	reg := device.NewRegistry()
	_ = reg.RegisterVirtual(dualsense.MockVirtual())
	_ = reg.RegisterVirtual(xboxmock.Virtual())
	_ = reg.RegisterVirtual(device.VirtualDevice{Key: msiKey, Source: device.SourceTest, AutoAttach: true,
		Descriptor: msigc20.Describe(gamepad.Mapping{}, evdev.InputDevice{}, true), New: newFakeMSI})
	a := newTestApp(t, newDirs(t), Options{ForceMock: true, AllowMock: true, Registry: reg})
	a.State.SetMode(mode)
	a.SelectDevice(msiKey)
	return a
}

func TestMSIControllerUIIsCapabilityDriven(t *testing.T) {
	a := newMSIApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 160, 48, a.Draw)
	page.Expect(page.GetByRole("image", "MSI Force GC20 V2 3D")).ToBeVisible()

	// Only panels backed by real functionality are offered.
	page.Expect(page.GetByRole("tab", "2 Diagnostics")).ToBeVisible()
	for _, p := range a.State.VisiblePanels() {
		switch p.Spec.ID {
		case state.PanelOverview, state.PanelDiagnostics, state.PanelDevice, state.PanelLogs, state.PanelSettings:
		default:
			t.Fatalf("panel %s has no real functionality on the GC20 V2", p.Spec.ID)
		}
	}
	page.GetByRole("tab", "2 Diagnostics").Click()
	d := a.State.Diag
	if d == nil || d.Ctx.Desc.ModelID != msigc20.ModelID {
		t.Fatal("diagnostics should run for the MSI controller")
	}
	for _, c := range []diag.Category{diag.CatTouchpad, diag.CatMotion, diag.CatAudioIn, diag.CatAudioOut, diag.CatBattery} {
		if d.Status(c) != diag.Unsupported {
			t.Fatalf("%s must be unsupported on the GC20 V2, got %v", c, d.Status(c))
		}
	}
	for _, id := range []string{"diag_tab_sticks", "diag_tab_buttons", "diag_tab_triggers", "diag_tab_haptics", "diag_tab_connection"} {
		page.Expect(page.GetByID(id)).ToBeVisible()
	}
	page.Expect(page.GetByID("diag_tab_touchpad")).Not().ToBeVisible()

	// Xbox-style labels and no adaptive-trigger test.
	d.Active = diag.CatTriggers
	page.Expect(page.GetByID("triggers_start")).ToBeVisible()
	page.Expect(page.GetByID("resistance_start")).Not().ToBeVisible()
	d.Active = diag.CatButtons
	page.Expect(page.GetByLabel("A")).ToBeVisible()
	page.Expect(page.GetByLabel("LB")).ToBeVisible()

	// Connection view shows the identification and lets the user confirm it.
	d.Active = diag.CatConnection
	page.Expect(page.GetByID("identity_yes")).ToBeVisible()
	page.GetByID("identity_yes").Click()
	if a.confirmed["045e:028e:0210"] != msigc20.ModelID || gamepad.Overrides()["045e:028e:0210"] != msigc20.ModelID {
		t.Fatalf("confirmation must be remembered, got %v", a.confirmed)
	}
	prefs, _ := a.Prefs.Load()
	if prefs.ConfirmedModels["045e:028e:0210"] != msigc20.ModelID {
		t.Fatal("confirmation must be persisted in settings.json")
	}
	gamepad.SetOverrides(nil)
}

func TestSwitchingBetweenMSIDualSenseAndXbox(t *testing.T) {
	a := newMSIApp(t, state.ModeEZ)
	page := uitest.Run(t, 160, 48, a.Draw)
	page.Expect(page.GetByRole("image", "MSI Force GC20 V2 3D")).ToBeVisible()
	a.SelectDevice(dualsense.MockKey)
	page.Expect(page.GetByRole("image", "Sony DualSense 3D")).ToBeVisible()
	if !a.State.Caps.Supports(device.CapAdaptiveTriggers) {
		t.Fatal("DualSense capabilities must come back after switching")
	}
	a.SelectDevice(xboxmock.Key)
	if a.State.Descriptor.Family != "xbox" || a.State.Diag == nil || !a.State.Diag.Ctx.Simulated {
		t.Fatal("the Xbox mock stays a simulated, separate device")
	}
	a.SelectDevice(msiKey)
	if a.State.Descriptor.Family != "msi_gc20" || a.State.Caps.Supports(device.CapLighting) {
		t.Fatal("switching back to MSI must restore its own capability set")
	}
}

func TestMSIRumbleTestUsesBoundedPulses(t *testing.T) {
	a := newMSIApp(t, state.ModeAdvanced)
	a.State.ActivePanel = state.PanelDiagnostics
	d := a.State.Diag
	if d.Haptics == nil || !d.Haptics.Separate {
		t.Fatal("rumble-capable MSI should offer the per-motor vibration test")
	}
	d.Haptics.Play("left", time.Now())
	for i := 0; i < 20 && d.Haptics.Running != nil; i++ {
		d.Tick(time.Now().Add(time.Duration(i) * 100 * time.Millisecond))
	}
	a.Dispatcher.FlushNow()
	dev := a.Manager.Active().(*fakeMSI)
	if len(dev.cmds) == 0 {
		t.Fatal("the vibration test must dispatch a pulse")
	}
	for _, c := range dev.cmds {
		if c.Type == device.CmdPulseHaptics && c.PulseDur > 1500*time.Millisecond {
			t.Fatalf("pulse too long: %s", c.PulseDur)
		}
	}
}
