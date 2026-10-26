package system

import (
	"os"
	"path/filepath"
	"strings"
)

// HIDSysfsInfo is what Linux exposes about a HID device in sysfs. Every field
// is read with plain file reads; nothing is written and no device I/O happens.
type HIDSysfsInfo struct {
	Found bool // the device directory exists

	Driver  string // bound kernel driver (e.g. "playstation")
	BusType uint16 // HID bus from HID_ID (0x0003 USB, 0x0005 Bluetooth)
	Name    string // HID_NAME
	Uniq    string // HID_UNIQ (the MAC address for Bluetooth/DualSense)
	Phys    string // HID_PHYS

	// Attrs holds driver-specific attributes that exist on the device (for
	// hid-playstation: firmware_version and hardware_version).
	Attrs map[string]string

	KernelRelease string // /proc/sys/kernel/osrelease
}

// ReadHIDSysfs reads the sysfs view of a hidraw node. hidrawDir is the
// /sys/class/hidraw/hidrawN directory; attrs lists the driver attributes to
// read from its device directory. Missing files are simply left empty.
func ReadHIDSysfs(hidrawDir string, attrs ...string) HIDSysfsInfo {
	var out HIDSysfsInfo
	if hidrawDir == "" {
		return out
	}
	dev := filepath.Join(hidrawDir, "device")
	if _, err := os.Stat(dev); err != nil {
		return out
	}
	out.Found = true
	if data, err := os.ReadFile(filepath.Join(dev, "uevent")); err == nil {
		meta := parseUevent(string(data))
		out.Driver = meta["DRIVER"]
		out.Name = meta["HID_NAME"]
		out.Uniq = meta["HID_UNIQ"]
		out.Phys = meta["HID_PHYS"]
		if bus, _, _, ok := parseHIDID(meta["HID_ID"]); ok {
			out.BusType = bus
		}
	}
	if out.Driver == "" {
		if link, err := os.Readlink(filepath.Join(dev, "driver")); err == nil {
			out.Driver = filepath.Base(link)
		}
	}
	for _, a := range attrs {
		if v := readSysfsAttr(filepath.Join(dev, a)); v != "" {
			if out.Attrs == nil {
				out.Attrs = map[string]string{}
			}
			out.Attrs[a] = v
		}
	}
	out.KernelRelease = KernelRelease()
	return out
}

// KernelRelease returns the running kernel release ("" when unknown).
func KernelRelease() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
