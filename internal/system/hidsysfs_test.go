package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHIDSysfs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hidraw8")
	dev := filepath.Join(dir, "device")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dev, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("uevent", "DRIVER=playstation\nHID_ID=0005:0000054C:00000CE6\nHID_NAME=DualSense Wireless Controller\nHID_UNIQ=02:00:00:00:00:04\n")
	write("firmware_version", "0x0110002a\n")
	write("hardware_version", "0x00000413\n")

	got := ReadHIDSysfs(dir, "firmware_version", "hardware_version", "not_there")
	if !got.Found || got.Driver != "playstation" || got.BusType != BusBluetooth || got.Name != "DualSense Wireless Controller" || got.Uniq != "02:00:00:00:00:04" {
		t.Fatalf("uevent: %+v", got)
	}
	if got.Attrs["firmware_version"] != "0x0110002a" || got.Attrs["hardware_version"] != "0x00000413" {
		t.Fatalf("attrs: %+v", got.Attrs)
	}
	if _, ok := got.Attrs["not_there"]; ok {
		t.Fatal("missing attributes must be absent, not empty")
	}
}

func TestReadHIDSysfsMissing(t *testing.T) {
	if got := ReadHIDSysfs(filepath.Join(t.TempDir(), "nope"), "firmware_version"); got.Found || got.Attrs != nil {
		t.Fatalf("a missing device must report nothing: %+v", got)
	}
	if got := ReadHIDSysfs(""); got.Found {
		t.Fatal("empty path")
	}
	// A device directory without driver attributes (e.g. hid-generic).
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "device"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ReadHIDSysfs(dir, "firmware_version"); !got.Found || len(got.Attrs) != 0 || got.Driver != "" {
		t.Fatalf("got %+v", got)
	}
}
