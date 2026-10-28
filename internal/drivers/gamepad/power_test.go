package gamepad

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The kernel registers the controller battery next to its HID device:
// …/0005:054C:09CC.0003/power_supply/<name>/{capacity,status} with the input
// node at …/0005:054C:09CC.0003/input/input20.
func TestPowerSupplyIsFoundAndRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux sysfs paths contain ':', which Windows file names cannot")
	}
	hid := filepath.Join(t.TempDir(), "devices", "bt", "0005:054C:09CC.0003")
	input := filepath.Join(hid, "input", "input20")
	ps := filepath.Join(hid, "power_supply", "ps-controller-battery-aa")
	for _, dir := range []string{input, ps} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(ps, "capacity"), []byte("65\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ps, "status"), []byte("Charging\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := FindPowerSupply(input)
	if dir != ps {
		t.Fatalf("found %q, want %q", dir, ps)
	}
	pct, charging, full, ok := ReadPowerSupply(dir)
	if !ok || pct != 65 || !charging || full {
		t.Fatalf("read %d %v %v %v", pct, charging, full, ok)
	}
	if FindPowerSupply(filepath.Join(t.TempDir(), "devices", "usb", "input", "input3")) != "" {
		t.Fatal("a wired pad without a power_supply has no battery")
	}
	if err := os.WriteFile(filepath.Join(ps, "capacity"), []byte("unknown"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := ReadPowerSupply(dir); ok {
		t.Fatal("unparsable capacity must not be reported")
	}
}

// DualSense and DualSense Edge stay with the hidraw protocol driver; other
// Sony pads (DualShock 4) are served by the evdev driver.
func TestOnlyDualSenseIDsAreExcluded(t *testing.T) {
	for _, id := range [][2]uint16{{0x054c, 0x0ce6}, {0x054c, 0x0df2}} {
		if !excludedIDs[id] {
			t.Errorf("%04x:%04x must be left to the DualSense driver", id[0], id[1])
		}
	}
	for _, id := range [][2]uint16{{0x054c, 0x05c4}, {0x054c, 0x09cc}} {
		if excludedIDs[id] {
			t.Errorf("DualShock 4 %04x:%04x must be discoverable", id[0], id[1])
		}
	}
}
