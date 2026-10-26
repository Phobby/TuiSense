package dualsense

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system"
)

// realBTReport is a Firmware Info report (0x20) read over Bluetooth from a
// physical DualSense (CFI-ZCT1, 054C:0CE6) with HIDIOCGFEATURE, CRC included.
const realBTReport = "204a756c20203420323032353130" + "3a31303a33320200040013040000" +
	"2a0010015038000000000000000000003006000" + "02a0001000a0002000600000085" + "6c63b7"

func realReport(t *testing.T) []byte {
	t.Helper()
	b, err := hex.DecodeString(realBTReport)
	if err != nil || len(b) != FeatureFirmwareSize {
		t.Fatalf("fixture: %v (%d bytes)", err, len(b))
	}
	return b
}

func detailMap(ds []device.Detail) map[string]device.Detail {
	m := map[string]device.Detail{}
	for _, d := range ds {
		m[d.Key] = d
	}
	return m
}

func btInfo() device.Info {
	return device.Info{VendorID: system.SonyVendorID, ProductID: system.DualSenseProductID, Connection: device.ConnBluetooth,
		HIDRawPath: "/dev/hidraw8", SysfsPath: "/sys/class/hidraw/hidraw8", Connected: true}
}

func kernelSysfs() system.HIDSysfsInfo {
	return system.HIDSysfsInfo{Found: true, Driver: "playstation", BusType: system.BusBluetooth, Name: "DualSense Wireless Controller",
		Attrs: map[string]string{SysfsFirmwareVersion: "0x0110002a", SysfsHardwareVersion: "0x00000413"}}
}

func TestParseRealBluetoothReport(t *testing.T) {
	fi, err := ParseFirmwareInfo(realReport(t), true)
	if err != nil {
		t.Fatalf("a real Bluetooth report must pass its CRC: %v", err)
	}
	want := FirmwareInfo{BuildDate: "Jul  4 2025", BuildTime: "10:10:32", FwType: 0x0002, SwSeries: 0x0004,
		HardwareInfo: 0x00000413, FirmwareVersion: 0x0110002A, UpdateVersion: 0x0630,
		ComponentVersions: [3]uint32{0x0001002A, 0x0002000A, 0x00000006}}
	copy(want.DeviceInfo[:], []byte{0x50, 0x38})
	if fi != want {
		t.Fatalf("decoded\n%+v\nwant\n%+v", fi, want)
	}
	if id := fi.Identity(); id.Target != "" {
		t.Fatalf("no field identifies the firmware target; got %q", id.Target)
	}
}

func TestDetailsSeparateFirmwareAndHardware(t *testing.T) {
	fi, _ := ParseFirmwareInfo(realReport(t), true)
	m := detailMap(BuildDetails(DetailsInput{Info: btInfo(), Sysfs: kernelSysfs(), Firmware: &fi}))

	checks := []struct{ key, value string }{
		{"model", "DualSense Wireless Controller"},
		{"manufacturer", "Sony Interactive Entertainment"},
		{"usb_id", "054C:0CE6"},
		{"connection", string(device.ConnBluetooth)},
		{"fw_version", "0x0630"},     // what Sony's metadata compares
		{"fw_build", "0x0110002A"},   // the kernel's firmware_version
		{"hw_version", "0x00000413"}, // the kernel's hardware_version
		{"fw_date", "Jul  4 2025 10:10:32"},
		{"fw_target", "Unknown"},
		{"revision", "Unknown"},
		{"driver", "hid-playstation"},
	}
	for _, c := range checks {
		if got := m[c.key].Value; got != c.value {
			t.Errorf("%s = %q, want %q", c.key, got, c.value)
		}
	}
	if !strings.Contains(m["fw_build"].Origin, "matches sysfs firmware_version") || !strings.Contains(m["hw_version"].Origin, "matches sysfs hardware_version") {
		t.Errorf("the report and the kernel's sysfs copy should be cross-checked: %q / %q", m["fw_build"].Origin, m["hw_version"].Origin)
	}
	if !m["hw_version"].Raw || m["fw_version"].Raw {
		t.Error("the hardware word has no documented meaning and must be marked raw; the update version is verified")
	}
	// sw_series happens to read 0x0004 on this controller. Without a source
	// linking it to Sony's targets it stays a raw, advanced-only value.
	if d := m["sw_series"]; d.Value != "0x0004" || !d.Raw || !d.Advanced {
		t.Errorf("sw_series must be shown only as a raw advanced value, got %+v", d)
	}
	if _, ok := m["notice_usb"]; ok {
		t.Error("everything was read over Bluetooth; no USB hint is needed")
	}
}

func TestBluetoothReadFailureSuggestsUSB(t *testing.T) {
	m := detailMap(BuildDetails(DetailsInput{Info: btInfo(), Sysfs: kernelSysfs(), FirmwareErr: "firmware info report failed its CRC check"}))
	if d := m["fw_version"]; !d.Unavailable || d.Note != "firmware info report failed its CRC check" {
		t.Errorf("the update version is unknown and must say why, got %+v", d)
	}
	// The kernel's own copy is still shown, attributed to sysfs.
	if d := m["fw_build"]; d.Value != "0x0110002A" || d.Origin != "sysfs firmware_version" {
		t.Errorf("fw_build should fall back to sysfs, got %+v", d)
	}
	if d := m["hw_version"]; d.Value != "0x00000413" || d.Unavailable {
		t.Errorf("hardware version is available from sysfs over Bluetooth, got %+v", d)
	}
	if d, ok := m["notice_usb"]; !ok || d.Value != USBHint {
		t.Error("a failed Bluetooth read should suggest USB")
	}

	usb := btInfo()
	usb.Connection = device.ConnUSB
	m = detailMap(BuildDetails(DetailsInput{Info: usb, Sysfs: kernelSysfs(), FirmwareErr: "x"}))
	if _, ok := m["notice_usb"]; ok {
		t.Error("never suggest USB to a controller that is already on USB")
	}
}

func TestPermissionDeniedUsesSysfsOnly(t *testing.T) {
	info := btInfo()
	info.PermissionDenied = true
	m := detailMap(BuildDetails(DetailsInput{Info: info, Sysfs: kernelSysfs(), FirmwareErr: "the hidraw node could not be opened"}))
	if _, ok := m["notice_permission"]; !ok {
		t.Error("missing permission notice")
	}
	if _, ok := m["notice_usb"]; ok {
		t.Error("USB does not fix a permission problem")
	}
	if m["fw_build"].Value != "0x0110002A" {
		t.Error("sysfs values are readable without hidraw access")
	}
}

func TestMissingSysfsNeverInventsValues(t *testing.T) {
	m := detailMap(BuildDetails(DetailsInput{Info: btInfo(), FirmwareErr: "feature report 0x20 could not be read: EIO"}))
	for _, k := range []string{"fw_version", "fw_build", "hw_version"} {
		if d := m[k]; !d.Unavailable {
			t.Errorf("%s must be unavailable without the report and sysfs, got %+v", k, d)
		}
	}
	for _, k := range []string{"driver", "sysfs_attrs", "sw_series", "fw_date", "hw_fields"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s must not appear when it was not read", k)
		}
	}
	fw, build, hw := versionStrings(nil, system.HIDSysfsInfo{})
	if fw != "" || build != "" || hw != "" {
		t.Fatalf("no version strings may be invented: %q %q %q", fw, build, hw)
	}
	// Malformed sysfs content is ignored, not parsed loosely.
	_, build, hw = versionStrings(nil, system.HIDSysfsInfo{Attrs: map[string]string{SysfsFirmwareVersion: "garbage", SysfsHardwareVersion: "413"}})
	if build != "" || hw != "" {
		t.Fatalf("malformed sysfs values must be dropped: %q %q", build, hw)
	}
}

func TestSysfsMismatchIsReported(t *testing.T) {
	fi, _ := ParseFirmwareInfo(realReport(t), true)
	sys := kernelSysfs()
	sys.Attrs[SysfsFirmwareVersion] = "0x0110001f"
	m := detailMap(BuildDetails(DetailsInput{Info: btInfo(), Sysfs: sys, Firmware: &fi}))
	if d, ok := m["fw_build_sysfs"]; !ok || d.Value != "0x0110001F" {
		t.Fatalf("a disagreement between the report and sysfs must be shown, got %+v", d)
	}
	if strings.Contains(m["fw_build"].Origin, "matches") {
		t.Fatal("mismatching values must not be labeled as matching")
	}
}

func TestVersionStringsPreferTheReport(t *testing.T) {
	fi, _ := ParseFirmwareInfo(realReport(t), true)
	fw, build, hw := versionStrings(&fi, kernelSysfs())
	if fw != "0x0630" || build != "0x0110002A" || hw != "0x00000413" {
		t.Fatalf("got %q %q %q", fw, build, hw)
	}
	fi.UpdateVersion = 0
	if fw, _, _ = versionStrings(&fi, kernelSysfs()); fw != "" {
		t.Fatal("a zero update version is unknown, not 0x0000")
	}
}

func TestMockDetailsAreMarkedSimulated(t *testing.T) {
	m := detailMap(NewMockBackend(nil).Details())
	if _, ok := m["notice_simulated"]; !ok {
		t.Fatal("mock details must say they are simulated")
	}
	if m["fw_target"].Value != "Unknown" {
		t.Fatal("even the mock must not claim a firmware target")
	}
}
