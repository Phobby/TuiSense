package winpad_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/winpad"
	"github.com/Phobby/TuiSense/internal/system/winhid"
)

func open(t *testing.T, devs ...winhid.Device) []device.Device {
	t.Helper()
	gamepad.SetOverrides(nil)
	d := winpad.NewDriverWith(drivers.WindowsModels(), func() ([]winhid.Device, error) { return devs, nil })
	cands, err := d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	var out []device.Device
	for _, c := range cands {
		dev, err := d.Open(c, nil)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, dev)
	}
	return out
}

func pad(path string, vid, pid, ver uint16, mfr, product string) winhid.Device {
	return winhid.Device{Path: path, VendorID: vid, ProductID: pid, Version: ver, UsagePage: 1, Usage: 5, Manufacturer: mfr, Product: product}
}

// accessibleOnlyConnection: nothing beyond the connection facts is offered,
// so no settings panel, diagnostic, command or fake success can appear.
func accessibleOnlyConnection(t *testing.T, dev device.Device) {
	t.Helper()
	for c, s := range dev.Capabilities() {
		if s.Accessible() && c != device.CapConnection {
			t.Errorf("%s must not be offered without an input backend", c)
		}
	}
	if err := dev.ExecuteCommand(context.Background(), device.Command{Type: device.CmdPulseHaptics}); err == nil {
		t.Error("commands must be refused, never reported as applied")
	}
	if dev.Info().PermissionHint != winpad.NoInputNote {
		t.Error("the user is told that input is unavailable")
	}
}

// MSI GC20 V2 in a DirectInput or Android mode is a plain HID gamepad that
// XInput cannot see. Named by its USB strings, it is identified as the MSI;
// it is never a DualSense.
func TestMSIInHIDModeIsDetectedAndNamedFromItsStrings(t *testing.T) {
	devs := open(t, pad(`\\?\hid#vid_0db0&pid_0000#1`, 0x0db0, 0x0001, 1, "MSI", "Force GC20 V2"))
	if len(devs) != 1 {
		t.Fatal("the controller must be detected")
	}
	d := devs[0]
	if d.Descriptor().ModelID != msigc20.ModelID || d.Descriptor().VisualID != "msi_gc20" || d.Info().IdentityState != "identified" {
		t.Fatalf("got %s %s %s", d.Descriptor().ModelID, d.Descriptor().VisualID, d.Info().IdentityState)
	}
	accessibleOnlyConnection(t, d)
	if len(d.Descriptor().ModeGuide) == 0 {
		t.Fatal("the MSI mode guide explains how to reach Default (XInput) mode")
	}
}

func TestUnknownHIDGamepadIsGenericNeverDualSense(t *testing.T) {
	d := open(t, pad(`\\?\hid#vid_0079&pid_0006#1`, 0x0079, 0x0006, 0x0107, "DragonRise Inc.", "Generic   USB  Joystick"))[0]
	desc := d.Descriptor()
	if desc.ModelID != winpad.ModelID || desc.DisplayName != "Generic Gamepad" || desc.VisualID != "generic" {
		t.Fatalf("got %+v", desc)
	}
	if strings.Contains(desc.DisplayName+desc.Brand+d.Info().Model, "Sony") || strings.Contains(desc.DisplayName+d.Info().Model, "DualSense") {
		t.Fatal("unknown hardware never shows Sony branding")
	}
	if d.Info().Name != "Generic   USB  Joystick" {
		t.Fatal("the raw product string is shown as the device name only")
	}
	accessibleOnlyConnection(t, d)
}

// A real DualSense is identified by Sony's ID, but its Linux-only features
// are declared inaccessible on Windows, never offered.
func TestDualSenseOnWindowsIsIdentifiedButNotDriven(t *testing.T) {
	d := open(t, pad(`\\?\hid#{00001124-0000-1000-8000-00805f9b34fb}_vid&0002054c_pid&0ce6#1`, 0x054c, 0x0ce6, 0x0100, "Sony Interactive Entertainment", "DualSense Wireless Controller"))[0]
	if d.Descriptor().ModelID != "sony.dualsense" || d.Info().Connection != device.ConnBluetooth {
		t.Fatalf("got %s %s", d.Descriptor().ModelID, d.Info().Connection)
	}
	accessibleOnlyConnection(t, d)
	if s := d.Capabilities().Spec(device.CapAdaptiveTriggers); s.Physical() != device.Present || s.Requires != device.RequiresPlatform {
		t.Fatalf("adaptive triggers exist but need a Windows backend: %+v", s)
	}
}

// XInput-class HID entries belong to the XInput driver: one physical
// controller is never listed twice, and keyboards are never listed.
func TestXInputEntriesAndNonGamepadsAreSkipped(t *testing.T) {
	devs := open(t,
		pad(`\\?\hid#vid_045e&pid_028e&ig_00#1`, 0x045e, 0x028e, 0x0210, "", ""),
		winhid.Device{Path: `\\?\hid#vid_046d&pid_c31c#1`, UsagePage: 1, Usage: 6},
	)
	if len(devs) != 0 {
		t.Fatalf("expected nothing, got %d", len(devs))
	}
}

func TestTwoIdenticalHIDControllersStaySeparate(t *testing.T) {
	devs := open(t,
		pad(`\\?\hid#vid_0db0&pid_0001#7&a`, 0x0db0, 1, 1, "MSI", "Force GC20 V2"),
		pad(`\\?\hid#vid_0db0&pid_0001#7&b`, 0x0db0, 1, 1, "MSI", "Force GC20 V2"))
	if len(devs) != 2 || devs[0].Info().ID == devs[1].Info().ID {
		t.Fatal("identical controllers must remain separately selectable")
	}
	if strings.Contains(devs[0].Info().ID, `\\?\`) {
		t.Fatal("the device path (instance IDs, Bluetooth address) never becomes the visible ID")
	}
}
