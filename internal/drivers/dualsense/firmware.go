package dualsense

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"regexp"

	"github.com/Phobby/TuiSense/internal/device"
)

// BTFeatureCRCSeed prefixes the CRC32 of Bluetooth feature reports (the Linux
// hid-playstation driver's PS_FEATURE_CRC32_SEED).
const BTFeatureCRCSeed byte = 0xA3

// FirmwareInfo is the decoded Firmware Info feature report (0x20).
//
// Layout sources:
//   - Linux hid-playstation (dualsense_get_firmware_info): hardware info at
//     bytes 24..27, firmware version at 28..31, update ("feature") version at
//     44..45. These are the only fields used for decisions; the kernel also
//     exposes the first two as the sysfs attributes hardware_version and
//     firmware_version.
//   - dualsensectl (struct dualsense_feature_report_firmware): build date and
//     time, fw_type, sw_series, device_info, update_image_info and three
//     component versions. Their names come from community reverse
//     engineering; their meaning is not documented by Sony, so they are shown
//     as raw values only.
type FirmwareInfo struct {
	BuildDate string // e.g. "Jun  5 2024" (empty when not plain ASCII)
	BuildTime string // e.g. "11:24:13"

	FwType   uint16 // bytes 20..21 (raw)
	SwSeries uint16 // bytes 22..23 (raw)

	HardwareInfo    uint32 // bytes 24..27 (kernel: hw_version)
	FirmwareVersion uint32 // bytes 28..31 (kernel: fw_version)

	DeviceInfo [12]byte // bytes 32..43 (raw)

	UpdateVersion   uint16 // bytes 44..45 (kernel: update_version)
	UpdateImageInfo uint8  // byte 46 (raw)

	// Component versions, bytes 48..59 (raw; dualsensectl calls them the
	// SBL, Venom and Spider DSP firmware versions).
	ComponentVersions [3]uint32
}

var (
	buildDateRe = regexp.MustCompile(`^[A-Z][a-z]{2} [ 0-9][0-9] [0-9]{4}$`)
	buildTimeRe = regexp.MustCompile(`^[0-9]{2}:[0-9]{2}:[0-9]{2}$`)
)

// ParseFirmwareInfo validates and decodes a Firmware Info feature report as
// returned by HIDIOCGFEATURE (report ID in byte 0). Bluetooth reports carry
// a CRC32 trailer that must match, so a corrupted read is never trusted.
func ParseFirmwareInfo(buf []byte, bluetooth bool) (FirmwareInfo, error) {
	if len(buf) < FeatureFirmwareSize {
		return FirmwareInfo{}, fmt.Errorf("firmware info report too short (%d bytes)", len(buf))
	}
	buf = buf[:FeatureFirmwareSize]
	if buf[0] != FeatureFirmwareReport {
		return FirmwareInfo{}, fmt.Errorf("unexpected report ID 0x%02X", buf[0])
	}
	if bluetooth {
		h := crc32.NewIEEE()
		_, _ = h.Write([]byte{BTFeatureCRCSeed})
		_, _ = h.Write(buf[:FeatureFirmwareSize-4])
		if h.Sum32() != binary.LittleEndian.Uint32(buf[FeatureFirmwareSize-4:]) {
			return FirmwareInfo{}, errors.New("firmware info report failed its CRC check")
		}
	}
	le16 := binary.LittleEndian.Uint16
	le32 := binary.LittleEndian.Uint32
	fi := FirmwareInfo{
		FwType:          le16(buf[20:22]),
		SwSeries:        le16(buf[22:24]),
		HardwareInfo:    le32(buf[24:28]),
		FirmwareVersion: le32(buf[28:32]),
		UpdateVersion:   le16(buf[44:46]),
		UpdateImageInfo: buf[46],
		ComponentVersions: [3]uint32{
			le32(buf[48:52]), le32(buf[52:56]), le32(buf[56:60]),
		},
	}
	copy(fi.DeviceInfo[:], buf[32:44])
	if d, t := string(buf[1:12]), string(buf[12:20]); buildDateRe.MatchString(d) && buildTimeRe.MatchString(t) {
		fi.BuildDate, fi.BuildTime = d, t
	}
	return fi, nil
}

// FormatUpdateVersion renders an update version the way Sony's metadata does.
func FormatUpdateVersion(v uint16) string { return fmt.Sprintf("0x%04X", v) }

// FormatWord renders a 32-bit firmware or hardware word.
func FormatWord(v uint32) string { return fmt.Sprintf("0x%08X", v) }

// Identity converts a decoded report into the driver-neutral firmware
// identity. A zero update version is treated as "not reported". The firmware
// update target is left empty: no field of this report is documented to
// identify it.
func (fi FirmwareInfo) Identity() device.FirmwareIdentity {
	id := device.FirmwareIdentity{Read: true, Source: "HID feature report 0x20 (Firmware Info)"}
	if fi.UpdateVersion != 0 {
		id.UpdateVersion, id.HasUpdateVersion = uint32(fi.UpdateVersion), true
	}
	return id
}
