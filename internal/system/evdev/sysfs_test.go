package evdev

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseBits(t *testing.T) {
	// "1 0" = word1 (bits 64..127) = 1, word0 = 0 → bit 64 set.
	b := ParseBits("1 0")
	if !b.Has(64) || b.Has(0) {
		t.Fatalf("bits %v", b)
	}
	b = ParseBits("3")
	if !b.Has(0) || !b.Has(1) || b.Has(2) {
		t.Fatalf("bits %v", b)
	}
	if got := ParseBits("").Set(); len(got) != 0 {
		t.Fatalf("empty bitmap set %v", got)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeSysfs builds a minimal /sys tree with one xpad-style gamepad attached
// through a USB interface, and returns the sysfs root.
func fakeSysfs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	usbDev := filepath.Join(root, "devices", "pci0000:00", "usb1", "1-2")
	write(t, filepath.Join(usbDev, "idVendor"), "045e")
	write(t, filepath.Join(usbDev, "idProduct"), "028e")
	write(t, filepath.Join(usbDev, "bcdDevice"), "0210")
	write(t, filepath.Join(usbDev, "manufacturer"), "Example Maker")
	write(t, filepath.Join(usbDev, "product"), "Example Pad")
	write(t, filepath.Join(usbDev, "speed"), "12")
	write(t, filepath.Join(usbDev, "version"), " 2.00")
	write(t, filepath.Join(usbDev, "busnum"), "1")
	write(t, filepath.Join(usbDev, "devnum"), "9")
	write(t, filepath.Join(usbDev, "serial"), "SHOULD-NOT-BE-READ")
	intf := filepath.Join(usbDev, "1-2:1.0")
	drvDir := filepath.Join(root, "bus", "usb", "drivers", "xpad")
	if err := os.MkdirAll(drvDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(intf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(drvDir, filepath.Join(intf, "driver")); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(intf, "input", "input30")
	write(t, filepath.Join(input, "name"), "Microsoft X-Box 360 pad")
	write(t, filepath.Join(input, "id", "bustype"), "0003")
	write(t, filepath.Join(input, "id", "vendor"), "045e")
	write(t, filepath.Join(input, "id", "product"), "028e")
	write(t, filepath.Join(input, "id", "version"), "0210")
	write(t, filepath.Join(input, "capabilities", "ev"), "20000b")
	// BTN_A..BTN_THUMBR (0x130..0x13e) live in word 4 (bits 256..319): bits 48..62.
	write(t, filepath.Join(input, "capabilities", "key"), "7cdb000000000000 0 0 0 0")
	// ABS_X,Y,Z,RX,RY,RZ (0..5) + HAT0X/Y (16, 17).
	write(t, filepath.Join(input, "capabilities", "abs"), "3003f")
	write(t, filepath.Join(input, "capabilities", "ff"), "107030000 0") // FF_RUMBLE (0x50 = bit 80) etc.
	if err := os.Symlink(intf, filepath.Join(input, "device")); err != nil {
		t.Fatal(err)
	}
	ev := filepath.Join(input, "event23")
	if err := os.MkdirAll(ev, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(input, filepath.Join(ev, "device")); err != nil {
		t.Fatal(err)
	}
	class := filepath.Join(root, "class", "input")
	if err := os.MkdirAll(class, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ev, filepath.Join(class, "event23")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestScanFakeSysfs(t *testing.T) {
	root := fakeSysfs(t)
	devs, err := Scan(root, "/dev/input")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 {
		t.Fatalf("got %d devices", len(devs))
	}
	d := devs[0]
	if d.EventPath != "/dev/input/event23" || d.Name != "Microsoft X-Box 360 pad" || d.KernelDriver != "xpad" {
		t.Fatalf("unexpected device %+v", d)
	}
	if d.Vendor != 0x045e || d.Product != 0x028e || d.Version != 0x0210 || d.HardwareKey() != "045e:028e:0210" {
		t.Fatalf("ids %+v key %s", d, d.HardwareKey())
	}
	if !d.IsGamepad() {
		t.Fatal("xpad device must be a gamepad")
	}
	if !d.Caps.Key.Has(BtnSouth) || !d.Caps.Key.Has(BtnThumbR) || d.Caps.Key.Has(BtnTL2) {
		t.Fatalf("key bits wrong: %v", d.Caps.Key.Set())
	}
	if !d.Caps.Abs.Has(AbsRZ) || !d.Caps.Abs.Has(AbsHat0Y) || d.Caps.Abs.Has(AbsGas) {
		t.Fatalf("abs bits wrong: %v", d.Caps.Abs.Set())
	}
	if !d.HasFF(FFRumble) {
		t.Fatalf("FF_RUMBLE expected: %v", d.Caps.FF.Set())
	}
	if d.USB == nil || d.USB.Manufacturer != "Example Maker" || d.USB.SpeedMbps != "12" || d.USB.DevNum != 9 {
		t.Fatalf("usb %+v", d.USB)
	}
}

func TestNonGamepadIsIgnored(t *testing.T) {
	mouse := InputDevice{Caps: Capabilities{EV: ParseBits("17"), Key: ParseBits("1f0000 0 0 0 0"), Abs: ParseBits("0")}}
	if mouse.IsGamepad() {
		t.Fatal("a mouse is not a gamepad")
	}
}

func TestScanRealSysfsDoesNotFail(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	devs, err := Scan("/sys", "/dev/input")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devs {
		if d.EventPath == "" || d.EventName == "" {
			t.Fatalf("incomplete device %+v", d)
		}
		t.Logf("%s %q bus=%04x %04x:%04x driver=%s gamepad=%v", d.EventName, d.Name, d.Bus, d.Vendor, d.Product, d.KernelDriver, d.IsGamepad())
	}
}
