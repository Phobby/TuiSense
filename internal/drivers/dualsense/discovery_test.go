package dualsense

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system"
	"os"
	"path/filepath"
	"testing"
)

func TestPhysicalIdentityFiltersDuplicateInterfaces(t *testing.T) {
	bt := system.DiscoveredDevice{HIDRawPath: "/dev/hidraw1", MACAddress: "a0:b1:c2:d3:e4:f5", Connection: device.ConnBluetooth, Readable: true, Writable: true}
	usb := bt
	usb.HIDRawPath = "/dev/hidraw2"
	usb.MACAddress = "A0:B1:C2:D3:E4:F5"
	usb.Connection = device.ConnUSB
	other := usb
	other.HIDRawPath = "/dev/hidraw3"
	other.MACAddress = "a0:b1:c2:d3:e4:f6"
	got := uniqueDevices([]system.DiscoveredDevice{bt, usb, other, usb})
	if len(got) != 2 || got[0].HIDRawPath != usb.HIDRawPath {
		t.Fatalf("expected one USB endpoint plus distinct identical model: %+v", got)
	}
	if physicalKey(bt) != physicalKey(usb) {
		t.Fatal("identity should survive transport changes")
	}
	usb.HIDRawPath = "/dev/hidraw9"
	if physicalKey(bt) != physicalKey(usb) {
		t.Fatal("identity should survive node renumbering")
	}
}

func TestMissingOrUnreliableIdentityNeverMergesModels(t *testing.T) {
	for _, id := range []string{"", "000000", "00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "DualSense"} {
		a := system.DiscoveredDevice{HIDRawPath: "/dev/hidraw1", MACAddress: id, Model: "DualSense"}
		b := a
		b.HIDRawPath = "/dev/hidraw2"
		if len(uniqueDevices([]system.DiscoveredDevice{a, b})) != 2 {
			t.Fatalf("unreliable identity %q merged real devices", id)
		}
	}
}

func TestPhysicalUSBAncestorFiltersInterfacesWithoutSerial(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "usb", "1-2")
	if err := os.MkdirAll(parent, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"idVendor", "idProduct"} {
		if err := os.WriteFile(filepath.Join(parent, name), []byte("054c"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var in []system.DiscoveredDevice
	for i, name := range []string{"interface0", "interface1"} {
		target := filepath.Join(parent, name)
		if err := os.MkdirAll(target, 0755); err != nil {
			t.Fatal(err)
		}
		raw := filepath.Join(root, name)
		if err := os.MkdirAll(raw, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(raw, "device")); err != nil {
			t.Fatal(err)
		}
		in = append(in, system.DiscoveredDevice{HIDRawPath: fmt.Sprintf("/dev/hidraw%d", i), SysfsPath: raw})
	}
	if len(uniqueDevices(in)) != 1 {
		t.Fatal("interfaces of a known physical USB device must merge")
	}
}
