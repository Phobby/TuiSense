package winhid

import "testing"

func TestClassification(t *testing.T) {
	xusb := Device{Path: `\\?\hid#vid_045e&pid_028e&ig_00#8&2b3f1a7c&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`,
		VendorID: 0x045e, ProductID: 0x028e, Version: 0x0210, UsagePage: 1, Usage: 5}
	bt := Device{Path: `\\?\hid#{00001124-0000-1000-8000-00805f9b34fb}_vid&0002054c_pid&0ce6#9&1a&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`,
		VendorID: 0x054c, ProductID: 0x0ce6, UsagePage: 1, Usage: 5}
	kbd := Device{Path: `\\?\hid#vid_046d&pid_c31c&mi_00#7&1&0&0000#{...}`, UsagePage: 1, Usage: 6}

	if !xusb.XInput() || bt.XInput() || kbd.XInput() {
		t.Fatal("XInput classification must follow the IG_ marker only")
	}
	if !bt.Bluetooth() || xusb.Bluetooth() {
		t.Fatal("Bluetooth classification")
	}
	if got := GameControllers([]Device{xusb, bt, kbd}); len(got) != 2 {
		t.Fatalf("keyboards are not game controllers: %v", got)
	}
	if xusb.HardwareKey() != "045e:028e:0210" {
		t.Fatalf("hardware key %q", xusb.HardwareKey())
	}
	if xusb.ShortID() == bt.ShortID() || len(xusb.ShortID()) != 8 {
		t.Fatal("short IDs must be distinct and short")
	}
	in := xusb.AsInputDevice()
	if !in.IsGamepad() || in.HardwareKey() != xusb.HardwareKey() {
		t.Fatal("a HID game controller must satisfy the gamepad matchers")
	}
	if kbd.AsInputDevice().IsGamepad() {
		t.Fatal("a keyboard must never look like a gamepad")
	}
}

// TestEnumerateNeverFails runs on every CI OS; on Windows it exercises the
// real SetupAPI/hid.dll calls (runners have no controller, so the list of
// game controllers is normally empty).
func TestEnumerateNeverFails(t *testing.T) {
	all, err := Enumerate()
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	for _, d := range GameControllers(all) {
		t.Logf("game controller %s usage %02x:%02x", d.HardwareKey(), d.UsagePage, d.Usage)
	}
}
