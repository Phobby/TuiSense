package dualsense

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func firmwareReport(update uint16) []byte {
	buf := make([]byte, FeatureFirmwareSize)
	buf[0] = FeatureFirmwareReport
	copy(buf[1:12], "Jun  5 2024")
	copy(buf[12:20], "11:24:13")
	binary.LittleEndian.PutUint32(buf[24:28], 0x00000611)
	binary.LittleEndian.PutUint32(buf[28:32], 0x0110002A)
	binary.LittleEndian.PutUint16(buf[44:46], update)
	return buf
}

func signBT(buf []byte) {
	h := crc32.NewIEEE()
	_, _ = h.Write([]byte{BTFeatureCRCSeed})
	_, _ = h.Write(buf[:FeatureFirmwareSize-4])
	binary.LittleEndian.PutUint32(buf[FeatureFirmwareSize-4:], h.Sum32())
}

func TestParseFirmwareInfoUSB(t *testing.T) {
	fi, err := ParseFirmwareInfo(firmwareReport(0x0630), false)
	if err != nil {
		t.Fatal(err)
	}
	if fi.UpdateVersion != 0x0630 || fi.HardwareInfo != 0x611 || fi.FirmwareVersion != 0x0110002A {
		t.Fatalf("decoded %+v", fi)
	}
	if fi.BuildDate != "Jun  5 2024" || fi.BuildTime != "11:24:13" {
		t.Fatalf("build timestamp %q %q", fi.BuildDate, fi.BuildTime)
	}
	id := fi.Identity()
	if !id.Read || !id.HasUpdateVersion || id.UpdateVersion != 0x0630 {
		t.Fatalf("identity %+v", id)
	}
}

func TestParseFirmwareInfoBluetoothCRC(t *testing.T) {
	buf := firmwareReport(0x0641)
	signBT(buf)
	if _, err := ParseFirmwareInfo(buf, true); err != nil {
		t.Fatalf("valid BT report rejected: %v", err)
	}
	buf[44] ^= 0x01 // corrupted in transit
	if _, err := ParseFirmwareInfo(buf, true); err == nil {
		t.Fatal("a report with a bad CRC must never be trusted")
	}
}

func TestParseFirmwareInfoRejectsBadReports(t *testing.T) {
	if _, err := ParseFirmwareInfo(make([]byte, 10), false); err == nil {
		t.Fatal("short report accepted")
	}
	buf := firmwareReport(0x0630)
	buf[0] = 0x09
	if _, err := ParseFirmwareInfo(buf, false); err == nil {
		t.Fatal("wrong report ID accepted")
	}
}

func TestFirmwareIdentityNeverInventsValues(t *testing.T) {
	buf := firmwareReport(0) // update version not reported
	copy(buf[1:20], "\x00\x01garbage-not-a-date")
	fi, err := ParseFirmwareInfo(buf, false)
	if err != nil {
		t.Fatal(err)
	}
	id := fi.Identity()
	if id.HasUpdateVersion {
		t.Fatal("a zero update version must be reported as unknown")
	}
	if fi.BuildDate != "" {
		t.Fatalf("non-ASCII build date must be dropped, got %q", fi.BuildDate)
	}
}

func TestMockExposesSimulatedFirmwareIdentity(t *testing.T) {
	id := NewMockBackend(nil).FirmwareIdentity()
	if !id.Read || !id.HasUpdateVersion {
		t.Fatalf("mock identity %+v", id)
	}
}
