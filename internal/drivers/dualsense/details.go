package dualsense

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system"
)

// Sysfs attributes the Linux hid-playstation driver adds to the HID device.
// They mirror bytes 28..31 and 24..27 of the Firmware Info report, which the
// kernel reads itself when the controller connects.
const (
	SysfsFirmwareVersion = "firmware_version"
	SysfsHardwareVersion = "hardware_version"
)

// USBHint is shown when Bluetooth could not provide firmware data that a
// wired connection may.
const USBHint = "Connect your DualSense using USB to retrieve additional firmware information."

// DetailsInput is everything the details builder needs. It is plain data so
// it can be tested without hardware.
type DetailsInput struct {
	Info  device.Info
	Sysfs system.HIDSysfsInfo
	// Firmware is the decoded report 0x20 (nil when it could not be read).
	Firmware *FirmwareInfo
	// FirmwareErr explains why report 0x20 is missing.
	FirmwareErr string
	// FirmwareSource names where Firmware came from.
	FirmwareSource string
	Simulated      bool
}

// parseSysfsWord parses a hid-playstation sysfs attribute ("0x%08x").
func parseSysfsWord(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return 0, false
	}
	v, err := strconv.ParseUint(s[2:], 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// modelName names a Sony controller by USB product ID.
func modelName(pid uint16) string {
	switch pid {
	case system.DualSenseProductID:
		return "DualSense Wireless Controller"
	case system.DualSenseEdgePID:
		return "DualSense Edge Wireless Controller"
	}
	return ""
}

// BuildDetails lists the controller's identification data, each value with
// its origin. Values whose meaning is not verified are marked Raw; nothing is
// inferred or defaulted. The firmware update target is always reported as
// unknown because the controller does not expose it in a documented form.
func BuildDetails(in DetailsInput) []device.Detail {
	info := in.Info
	var out []device.Detail
	add := func(d device.Detail) { out = append(out, d) }
	unavailable := func(g device.DetailGroup, key, label, why string, adv bool) {
		add(device.Detail{Group: g, Key: key, Label: label, Value: "unavailable", Note: why, Unavailable: true, Advanced: adv})
	}
	fi := in.Firmware
	bt := info.Connection == device.ConnBluetooth

	// Device.
	if name := modelName(info.ProductID); name != "" {
		add(device.Detail{Group: device.DetailDevice, Key: "model", Label: "Model", Value: name, Origin: "USB product ID"})
	}
	if info.VendorID == system.SonyVendorID {
		add(device.Detail{Group: device.DetailDevice, Key: "manufacturer", Label: "Manufacturer", Value: "Sony Interactive Entertainment", Origin: "USB vendor ID"})
	}
	idOrigin := "device discovery"
	if in.Sysfs.Found {
		idOrigin = "sysfs uevent HID_ID"
	}
	add(device.Detail{Group: device.DetailDevice, Key: "usb_id", Label: "USB VID:PID", Value: fmt.Sprintf("%04X:%04X", info.VendorID, info.ProductID), Origin: idOrigin})
	add(device.Detail{Group: device.DetailDevice, Key: "connection", Label: "Connection", Value: string(info.Connection), Origin: "HID bus type"})
	if info.HIDRawPath != "" {
		add(device.Detail{Group: device.DetailDevice, Key: "hidraw", Label: "HID device", Value: info.HIDRawPath, Origin: "device discovery"})
	}
	if info.MACAddress != "" {
		add(device.Detail{Group: device.DetailDevice, Key: "mac", Label: "Bluetooth MAC", Value: info.MACAddress, Origin: "sysfs HID_UNIQ or feature report 0x09", Advanced: true})
	}

	// Firmware.
	reportOrigin := nonEmptyOr(in.FirmwareSource, "HID feature report 0x20")
	readWhy := in.FirmwareErr
	if readWhy == "" {
		readWhy = "the Firmware Info report was not read"
	}
	switch {
	case fi != nil && fi.UpdateVersion != 0:
		add(device.Detail{Group: device.DetailFirmware, Key: "fw_version", Label: "Firmware version",
			Value: FormatUpdateVersion(fi.UpdateVersion), Origin: reportOrigin + ", bytes 44–45",
			Note: "The update version that Sony's firmware metadata lists (the Linux driver calls it the feature version)."})
	case fi != nil:
		unavailable(device.DetailFirmware, "fw_version", "Firmware version", "The controller reported no update version.", false)
	default:
		unavailable(device.DetailFirmware, "fw_version", "Firmware version", readWhy, false)
	}

	sysFW, hasSysFW := parseSysfsWord(in.Sysfs.Attrs[SysfsFirmwareVersion])
	sysHW, hasSysHW := parseSysfsWord(in.Sysfs.Attrs[SysfsHardwareVersion])
	buildNote := "Main firmware version word. It is a different number from the firmware version above and is not compared with Sony's metadata."
	switch {
	case fi != nil:
		origin := reportOrigin + ", bytes 28–31"
		if hasSysFW && sysFW == fi.FirmwareVersion {
			origin += " (matches sysfs " + SysfsFirmwareVersion + ")"
		}
		add(device.Detail{Group: device.DetailFirmware, Key: "fw_build", Label: "Firmware build", Value: FormatWord(fi.FirmwareVersion), Origin: origin, Note: buildNote})
		if hasSysFW && sysFW != fi.FirmwareVersion {
			add(device.Detail{Group: device.DetailFirmware, Key: "fw_build_sysfs", Label: "Firmware build (sysfs)", Value: FormatWord(sysFW),
				Origin: "sysfs " + SysfsFirmwareVersion, Note: "Differs from the value read now; the kernel read it when the controller connected.", Advanced: true})
		}
	case hasSysFW:
		add(device.Detail{Group: device.DetailFirmware, Key: "fw_build", Label: "Firmware build", Value: FormatWord(sysFW), Origin: "sysfs " + SysfsFirmwareVersion, Note: buildNote})
	default:
		unavailable(device.DetailFirmware, "fw_build", "Firmware build", readWhy, false)
	}
	if fi != nil && fi.BuildDate != "" {
		add(device.Detail{Group: device.DetailFirmware, Key: "fw_date", Label: "Firmware build date", Value: fi.BuildDate + " " + fi.BuildTime, Origin: reportOrigin + ", bytes 1–19"})
	}
	add(device.Detail{Group: device.DetailFirmware, Key: "fw_target", Label: "Firmware target", Value: "Unknown",
		Note: "The controller does not report its Sony firmware update target (0004, 000B, 000E…) in any documented field, and TuiSense does not guess it."})
	if fi != nil {
		rawNote := "Field name from community reverse engineering (dualsensectl); its meaning is not documented by Sony and TuiSense does not use it."
		add(device.Detail{Group: device.DetailFirmware, Key: "fw_type", Label: "Firmware type (raw)", Value: fmt.Sprintf("0x%04X", fi.FwType), Origin: reportOrigin + ", bytes 20–21", Note: rawNote, Raw: true, Advanced: true})
		add(device.Detail{Group: device.DetailFirmware, Key: "sw_series", Label: "Software series (raw)", Value: fmt.Sprintf("0x%04X", fi.SwSeries), Origin: reportOrigin + ", bytes 22–23",
			Note: "Field name from dualsensectl. No source documents a link between this value and Sony's firmware targets, so it is not used to select one.", Raw: true, Advanced: true})
		add(device.Detail{Group: device.DetailFirmware, Key: "fw_components", Label: "Component versions (raw)",
			Value:  fmt.Sprintf("%s · %s · %s", FormatWord(fi.ComponentVersions[0]), FormatWord(fi.ComponentVersions[1]), FormatWord(fi.ComponentVersions[2])),
			Origin: reportOrigin + ", bytes 48–59", Note: "dualsensectl calls these the SBL, Venom and Spider DSP firmware versions (unverified).", Raw: true, Advanced: true})
		add(device.Detail{Group: device.DetailFirmware, Key: "update_image", Label: "Update image info (raw)", Value: fmt.Sprintf("0x%02X", fi.UpdateImageInfo), Origin: reportOrigin + ", byte 46", Note: rawNote, Raw: true, Advanced: true})
	}

	// Hardware.
	hwNote := "Hardware information word. Sony does not document its encoding, and it does not identify the firmware target."
	var hw uint32
	hasHW := false
	switch {
	case fi != nil:
		hw, hasHW = fi.HardwareInfo, true
		origin := reportOrigin + ", bytes 24–27"
		if hasSysHW && sysHW == hw {
			origin += " (matches sysfs " + SysfsHardwareVersion + ")"
		}
		add(device.Detail{Group: device.DetailHardware, Key: "hw_version", Label: "Hardware version (raw)", Value: FormatWord(hw), Origin: origin, Note: hwNote, Raw: true})
	case hasSysHW:
		hw, hasHW = sysHW, true
		add(device.Detail{Group: device.DetailHardware, Key: "hw_version", Label: "Hardware version (raw)", Value: FormatWord(hw), Origin: "sysfs " + SysfsHardwareVersion, Note: hwNote, Raw: true})
	default:
		unavailable(device.DetailHardware, "hw_version", "Hardware version (raw)", readWhy, false)
	}
	add(device.Detail{Group: device.DetailHardware, Key: "revision", Label: "Board revision", Value: "Unknown",
		Note: "Not reported in a documented form. The model number printed on the board (e.g. BDM-030) cannot be read over HID."})
	if hasHW {
		add(device.Detail{Group: device.DetailHardware, Key: "hw_fields", Label: "Hardware word fields (raw)",
			Value:  fmt.Sprintf("bits 16–23: 0x%02X · bits 8–15: 0x%02X · bits 0–7: 0x%02X", (hw>>16)&0xFF, (hw>>8)&0xFF, hw&0xFF),
			Origin: "decoded from the hardware version", Note: "dualsensectl's comments call these variation, generation and trial fields; this is unverified.", Raw: true, Advanced: true})
	}
	if fi != nil {
		add(device.Detail{Group: device.DetailHardware, Key: "device_info", Label: "Device info bytes (raw)", Value: strings.ToUpper(hex.EncodeToString(fi.DeviceInfo[:])),
			Origin: reportOrigin + ", bytes 32–43", Note: "Undocumented.", Raw: true, Advanced: true})
	}

	// Linux driver.
	if in.Sysfs.Driver != "" {
		v, origin := in.Sysfs.Driver, "sysfs device driver"
		if v == "playstation" {
			v, origin = "hid-playstation", "sysfs device driver (\"playstation\")"
		}
		add(device.Detail{Group: device.DetailDriver, Key: "driver", Label: "Kernel driver", Value: v, Origin: origin})
	}
	if in.Sysfs.Name != "" {
		add(device.Detail{Group: device.DetailDriver, Key: "hid_name", Label: "HID name", Value: in.Sysfs.Name, Origin: "sysfs uevent HID_NAME", Advanced: true})
	}
	if in.Sysfs.Found {
		attrs := "none"
		var have []string
		for _, a := range []string{SysfsFirmwareVersion, SysfsHardwareVersion} {
			if _, ok := in.Sysfs.Attrs[a]; ok {
				have = append(have, a)
			}
		}
		if len(have) > 0 {
			attrs = strings.Join(have, ", ")
		}
		add(device.Detail{Group: device.DetailDriver, Key: "sysfs_attrs", Label: "Driver attributes", Value: attrs, Origin: "sysfs", Advanced: true})
	}
	if info.SysfsPath != "" {
		add(device.Detail{Group: device.DetailDriver, Key: "sysfs", Label: "sysfs path", Value: info.SysfsPath, Origin: "device discovery", Advanced: true})
	}
	if in.Sysfs.KernelRelease != "" {
		add(device.Detail{Group: device.DetailDriver, Key: "kernel", Label: "Linux kernel", Value: in.Sysfs.KernelRelease, Origin: "/proc/sys/kernel/osrelease", Advanced: true})
	}

	// Notices.
	if fi == nil && bt && !info.PermissionDenied {
		add(device.Detail{Group: device.DetailFirmware, Key: "notice_usb", Label: "Note", Value: USBHint})
	}
	if fi == nil && info.PermissionDenied {
		add(device.Detail{Group: device.DetailFirmware, Key: "notice_permission", Label: "Note",
			Value: "TuiSense has no read access to the controller's HID device, so only sysfs values are shown. Grant read access to see every value."})
	}
	if in.Simulated {
		add(device.Detail{Group: device.DetailDevice, Key: "notice_simulated", Label: "Note", Value: "Simulated controller — values are not from hardware."})
	}
	return out
}

// versionStrings returns the summary firmware version, firmware build and
// hardware version, preferring the report just read and falling back to the
// kernel's sysfs copy. Empty strings mean "not read".
func versionStrings(fi *FirmwareInfo, sys system.HIDSysfsInfo) (fw, build, hw string) {
	if fi != nil {
		if fi.UpdateVersion != 0 {
			fw = FormatUpdateVersion(fi.UpdateVersion)
		}
		return fw, FormatWord(fi.FirmwareVersion), FormatWord(fi.HardwareInfo)
	}
	if v, ok := parseSysfsWord(sys.Attrs[SysfsFirmwareVersion]); ok {
		build = FormatWord(v)
	}
	if v, ok := parseSysfsWord(sys.Attrs[SysfsHardwareVersion]); ok {
		hw = FormatWord(v)
	}
	return fw, build, hw
}

func nonEmptyOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
