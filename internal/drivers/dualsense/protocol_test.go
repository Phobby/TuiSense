package dualsense

import (
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
)

func TestBuildOutputReportUSB(t *testing.T) {
	cfg := device.DefaultConfig()
	cfg.Lighting.Red = 140
	cfg.Lighting.Green = 40
	cfg.Lighting.Blue = 250
	cfg.Lighting.MicLED = device.MicLEDSolid
	cfg.LeftTrigger = device.TriggerConfig{
		Mode:          device.TriggerModeResistance,
		StartPosition: 30,
		Strength:      200,
	}
	cfg.RightTrigger = device.TriggerConfig{
		Mode:          device.TriggerModeWeapon,
		StartPosition: 70,
		EndPosition:   160,
		Strength:      220,
	}

	pkt := BuildOutputReport(cfg, device.ConnUSB, 0)
	if len(pkt) != USBOutputReportSize {
		t.Fatalf("expected USB output report size %d, got %d", USBOutputReportSize, len(pkt))
	}
	if pkt[0] != USBOutputReportID {
		t.Fatalf("expected report ID 0x02, got 0x%02X", pkt[0])
	}
	if pkt[9] != byte(device.MicLEDSolid) {
		t.Errorf("expected MicLED byte 1, got %d", pkt[9])
	}
	// Right trigger starts at byte 11
	if pkt[11] != 0x02 || pkt[12] != 70 || pkt[13] != 160 || pkt[14] != 220 {
		t.Errorf("unexpected R2 weapon trigger block: %v", pkt[11:16])
	}
	// Left trigger starts at byte 22
	if pkt[22] != 0x01 || pkt[23] != 30 || pkt[24] != 200 {
		t.Errorf("unexpected L2 resistance trigger block: %v", pkt[22:26])
	}
	// Lightbar RGB at bytes 45, 46, 47
	if pkt[45] != 140 || pkt[46] != 40 || pkt[47] != 250 {
		t.Errorf("unexpected RGB bytes: %d, %d, %d", pkt[45], pkt[46], pkt[47])
	}
}

func TestBuildOutputReportBluetoothCRC32(t *testing.T) {
	cfg := device.DefaultConfig()
	pkt := BuildOutputReport(cfg, device.ConnBluetooth, 3)
	if len(pkt) != BTOutputReportSize {
		t.Fatalf("expected BT output report size %d, got %d", BTOutputReportSize, len(pkt))
	}
	if pkt[0] != BTReportID {
		t.Fatalf("expected BT report ID 0x31, got 0x%02X", pkt[0])
	}

	gotCRC := binary.LittleEndian.Uint32(pkt[74:78])
	h := crc32.NewIEEE()
	_, _ = h.Write([]byte{BTOutputCRCSeed})
	_, _ = h.Write(pkt[:74])
	wantCRC := h.Sum32()

	if gotCRC != wantCRC {
		t.Fatalf("BT CRC32 mismatch: got 0x%08X, want 0x%08X", gotCRC, wantCRC)
	}
}

func TestParseInputReport(t *testing.T) {
	raw := make([]byte, 64)
	raw[0] = USBInputReportID
	raw[1] = 255   // LeftStickX = +1.0
	raw[2] = 0     // LeftStickY = -1.0
	raw[3] = 128   // RightStickX ~ 0
	raw[4] = 128   // RightStickY ~ 0
	raw[5] = 255   // L2 = 1.0
	raw[6] = 128   // R2 ~ 0.5
	raw[10] = 0x04 // Mic mute button pressed
	raw[33] = 0x00 // Touch 1 active (bit 7 == 0)
	raw[53] = 0x18 // Charging (0x1_) + level 8 (85%)

	tel, err := ParseInputReport(raw)
	if err != nil {
		t.Fatalf("ParseInputReport failed: %v", err)
	}
	if tel.LeftStickX < 0.99 || tel.LeftStickY > -0.99 {
		t.Errorf("unexpected stick coords: LX=%.2f LY=%.2f", tel.LeftStickX, tel.LeftStickY)
	}
	if tel.L2Pressure < 0.99 {
		t.Errorf("expected L2Pressure 1.0, got %.2f", tel.L2Pressure)
	}
	if !tel.MicMutedButton {
		t.Errorf("expected MicMutedButton true")
	}
	if !tel.TouchActive {
		t.Errorf("expected TouchActive true")
	}
	if !tel.Charging || tel.BatteryPercent != 85 {
		t.Errorf("expected Charging=true BatteryPercent=85, got Charging=%v Pct=%d", tel.Charging, tel.BatteryPercent)
	}
}
