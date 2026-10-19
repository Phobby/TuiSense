package dualsense

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"github.com/ygzztrk/tuisense/internal/device"
)

const (
	USBInputReportID      byte = 0x01
	USBOutputReportID     byte = 0x02
	USBOutputReportSize   int  = 48
	BTReportID            byte = 0x31
	BTOutputReportSize    int  = 78
	BTOutputCRCSeed       byte = 0xA2
	FeatureFirmwareReport byte = 0x20
	FeatureFirmwareSize   int  = 64
	FeaturePairingReport  byte = 0x09
	FeaturePairingSize    int  = 20
)

// ValidFlag0 bit definitions for DualSense output report byte 0 (after report ID).
const (
	flagCompatVibration byte = 0x01
	flagHapticsSelect   byte = 0x02
	flagRightTrigger    byte = 0x04
	flagLeftTrigger     byte = 0x08
	flagHeadphoneVol    byte = 0x10
	flagSpeakerVol      byte = 0x20
	flagMicVol          byte = 0x40
	flagAudioControl    byte = 0x80
)

// ValidFlag1 bit definitions for DualSense output report byte 1 (after report ID).
const (
	flagMicMuteLED       byte = 0x01
	flagPowerSaveMute    byte = 0x02
	flagLightbarColor    byte = 0x04
	flagReleaseLEDs      byte = 0x08
	flagPlayerIndicators byte = 0x10
)

// BuildOutputReport encodes a ControllerConfig into either a 48-byte USB HID Output Report (0x02)
// or a 78-byte Bluetooth HID Output Report (0x31) signed with an IEEE 802.3 CRC32 trailer.
func BuildOutputReport(cfg device.Config, conn device.ConnectionType, btSeq uint8) []byte {
	var payload [47]byte

	// 1. Validity flags
	payload[0] = flagCompatVibration | flagHapticsSelect | flagRightTrigger | flagLeftTrigger |
		flagHeadphoneVol | flagSpeakerVol | flagMicVol
	payload[1] = flagMicMuteLED | flagPowerSaveMute | flagLightbarColor | flagPlayerIndicators

	// 2. Haptic / Rumble motors (scaled by MasterIntensity)
	scale := float64(cfg.Haptics.MasterIntensity) / 100.0
	if scale < 0 {
		scale = 0
	} else if scale > 1 {
		scale = 1
	}
	payload[2] = uint8(float64(cfg.Haptics.RightMotor) * scale) // Right (high-freq)
	payload[3] = uint8(float64(cfg.Haptics.LeftMotor) * scale)  // Left (low-freq)

	// 3. Audio volumes (mapped to DualSense register ranges)
	payload[4] = uint8(cfg.Audio.HeadphoneVolume * 0x7F / 100)
	payload[5] = uint8(0x3D + (cfg.Audio.SpeakerVolume * (0x64 - 0x3D) / 100))
	payload[6] = uint8(cfg.Audio.MicGain * 0x40 / 100)

	// 4. Mic Mute LED & power save audio mute bit
	payload[8] = byte(cfg.Lighting.MicLED)
	if cfg.Audio.ForceMicMute || cfg.Lighting.MicLED != device.MicLEDOff {
		payload[9] = 0x10 // Mute internal mic input path
	}

	// 5. Right Trigger (R2) 11-byte effect block at payload[10..20]
	encodeTriggerBlock(payload[10:21], cfg.RightTrigger)

	// 6. Left Trigger (L2) 11-byte effect block at payload[21..31]
	encodeTriggerBlock(payload[21:32], cfg.LeftTrigger)

	// 7. LED brightness, Player LEDs, and RGB Lightbar
	payload[38] = 0x02 // Allow setting player/lightbar brightness
	payload[41] = 0x02 // Lightbar pulse option: fade in
	payload[42] = byte(cfg.Lighting.Brightness)
	payload[43] = cfg.Lighting.PlayerMask & 0x1F

	if cfg.Lighting.Enabled && cfg.Lighting.Animation != device.AnimOff {
		bFactor := brightnessScale(cfg.Lighting.Brightness)
		payload[44] = uint8(float64(cfg.Lighting.Red) * bFactor)
		payload[45] = uint8(float64(cfg.Lighting.Green) * bFactor)
		payload[46] = uint8(float64(cfg.Lighting.Blue) * bFactor)
	}

	if conn == device.ConnBluetooth {
		pkt := make([]byte, BTOutputReportSize)
		pkt[0] = BTReportID
		pkt[1] = (btSeq << 4) | 0x02 // BT HID tag + sequence nibble
		copy(pkt[2:49], payload[:])

		crc := computeBluetoothCRC32(pkt[:74])
		binary.LittleEndian.PutUint32(pkt[74:78], crc)
		return pkt
	}

	pkt := make([]byte, USBOutputReportSize)
	pkt[0] = USBOutputReportID
	copy(pkt[1:48], payload[:])
	return pkt
}

// encodeTriggerBlock writes the 11-byte adaptive trigger effect descriptor.
func encodeTriggerBlock(dst []byte, tc device.TriggerConfig) {
	for i := range dst {
		dst[i] = 0
	}
	switch tc.Mode {
	case device.TriggerModeOff:
		dst[0] = 0x05 // Off / Release tension

	case device.TriggerModeResistance:
		dst[0] = 0x01 // Continuous resistance
		dst[1] = tc.StartPosition
		dst[2] = tc.Strength

	case device.TriggerModeWeapon:
		dst[0] = 0x02 // Sectional / Weapon snap resistance
		dst[1] = tc.StartPosition
		dst[2] = tc.EndPosition
		dst[3] = tc.Strength

	case device.TriggerModeFeedback:
		dst[0] = 0x21 // Enchanced zone feedback
		dst[1] = tc.StartPosition
		dst[2] = tc.EndPosition
		dst[3] = tc.Strength

	case device.TriggerModeVibration:
		dst[0] = 0x26 // Automatic / Rapid vibration pulse
		dst[1] = tc.StartPosition
		dst[2] = tc.Strength
		dst[3] = tc.Frequency
		if dst[3] == 0 {
			dst[3] = 25
		}

	case device.TriggerModeCustom:
		dst[0] = 0x02
		dst[1] = tc.StartPosition
		dst[2] = tc.EndPosition
		dst[3] = tc.Strength
		dst[4] = tc.Frequency

	default:
		dst[0] = 0x05
	}
}

func brightnessScale(b device.BrightnessLevel) float64 {
	switch b {
	case device.BrightnessMedium:
		return 0.65
	case device.BrightnessLow:
		return 0.30
	default:
		return 1.0
	}
}

func computeBluetoothCRC32(reportWithoutCRC []byte) uint32 {
	h := crc32.NewIEEE()
	_, _ = h.Write([]byte{BTOutputCRCSeed})
	_, _ = h.Write(reportWithoutCRC)
	return h.Sum32()
}

// ParseInputReport decodes a raw DualSense USB (0x01) or Bluetooth (0x31) HID input report
// into normalized ControllerTelemetry.
func ParseInputReport(raw []byte) (device.Telemetry, error) {
	if len(raw) < 10 {
		return device.Telemetry{}, fmt.Errorf("input report too short (%d bytes)", len(raw))
	}

	offset := 0
	switch raw[0] {
	case USBInputReportID:
		offset = 1
	case BTReportID:
		// Full Bluetooth input report starts payload at byte 2
		offset = 2
	default:
		// Short BT report (0x01 over BT) or unprefixed payload
		offset = 1
	}

	body := raw[offset:]
	if len(body) < 54 {
		return device.Telemetry{}, fmt.Errorf("input report body too short (%d bytes)", len(body))
	}

	tel := device.Telemetry{
		LeftStickX:  normalizeAxis(body[0]),
		LeftStickY:  normalizeAxis(body[1]),
		RightStickX: normalizeAxis(body[2]),
		RightStickY: normalizeAxis(body[3]),
		L2Pressure:  float64(body[4]) / 255.0,
		R2Pressure:  float64(body[5]) / 255.0,
		UpdatedAt:   time.Now(),
	}

	// Buttons: body[7], body[8], body[9]
	tel.ButtonsMask = uint32(body[7]) | (uint32(body[8]) << 8) | (uint32(body[9]) << 16)
	tel.MicMutedButton = (body[9] & 0x04) != 0
	tel.Buttons = DecodeButtons(body[7], body[8], body[9])
	tel.ButtonsLatched = tel.Buttons

	// 6-axis IMU: Gyroscope (body[15..20]) & Accelerometer (body[21..26])
	gyroPitch := int16(binary.LittleEndian.Uint16(body[15:17]))
	gyroYaw := int16(binary.LittleEndian.Uint16(body[17:19]))
	gyroRoll := int16(binary.LittleEndian.Uint16(body[19:21]))

	accelX := int16(binary.LittleEndian.Uint16(body[21:23]))
	accelY := int16(binary.LittleEndian.Uint16(body[23:25]))
	accelZ := int16(binary.LittleEndian.Uint16(body[25:27]))

	tel.GyroPitch = clampFloat(float64(gyroPitch)/180.0, -45.0, 45.0)
	tel.GyroYaw = clampFloat(float64(gyroYaw)/180.0, -45.0, 45.0)
	tel.GyroRoll = clampFloat(float64(gyroRoll)/180.0, -45.0, 45.0)

	tel.Raw = device.RawInput{
		Valid:    true,
		Sticks:   [4]int32{int32(body[0]), int32(body[1]), int32(body[2]), int32(body[3])},
		Triggers: [2]int32{int32(body[4]), int32(body[5])},
		Gyro:     [3]int16{gyroPitch, gyroYaw, gyroRoll},
		Accel:    [3]int16{accelX, accelY, accelZ},
	}

	tel.AccelX = float64(accelX) / 8192.0
	tel.AccelY = float64(accelY) / 8192.0
	tel.AccelZ = float64(accelZ) / 8192.0

	// Touchpad points: body[32..35] and body[36..39]. Bit 7 of the contact
	// byte is 0 while the finger touches (active-low); bits 0..6 are the contact ID.
	for i := 0; i < 2; i++ {
		tel.Touch[i] = decodeTouchPoint(body[32+4*i : 36+4*i])
	}
	tel.TouchActive = tel.Touch[0].Active
	tel.TouchX, tel.TouchY = tel.Touch[0].X, tel.Touch[0].Y

	// Battery & Charging status: body[52]
	// Lower nibble = 0..10 level, Upper nibble = power state (0x0=discharging, 0x1=charging, 0x2=full)
	batByte := body[52]
	rawLevel := int(batByte & 0x0F)
	powerState := (batByte >> 4) & 0x0F

	pct := rawLevel*10 + 5
	if pct > 100 {
		pct = 100
	}
	if powerState == 0x02 {
		pct = 100
		tel.FullyCharged = true
	}
	tel.BatteryPercent = pct
	tel.Charging = powerState == 0x01 || powerState == 0x02

	previewLen := len(raw)
	if previewLen > 24 {
		previewLen = 24
	}
	tel.LastInputHex = FormatHexPreview(raw[:previewLen])
	return tel, nil
}

// ReportSequence returns the 8-bit sequence counter of an input report (body[6]),
// used to detect dropped reports.
func ReportSequence(raw []byte) (uint8, bool) {
	offset := 1
	if len(raw) > 0 && raw[0] == BTReportID {
		offset = 2
	}
	if len(raw) <= offset+6 {
		return 0, false
	}
	return raw[offset+6], true
}

func decodeTouchPoint(b []byte) device.TouchPoint {
	tp := device.TouchPoint{Active: b[0]&0x80 == 0, ID: b[0] & 0x7F}
	if tp.Active {
		tp.X = uint16(b[1]) | (uint16(b[2]&0x0F) << 8)
		tp.Y = (uint16(b[2]) >> 4) | (uint16(b[3]) << 4)
	}
	return tp
}

// DecodeButtons maps the DualSense button bytes (body[7..9]) to semantic buttons.
// The D-pad is a hat switch in the low nibble of b0 (0=N … 7=NW, 8=released).
func DecodeButtons(b0, b1, b2 byte) device.ButtonSet {
	var set device.ButtonSet
	switch b0 & 0x0F {
	case 0:
		set = set.With(device.ButtonDpadUp)
	case 1:
		set = set.With(device.ButtonDpadUp).With(device.ButtonDpadRight)
	case 2:
		set = set.With(device.ButtonDpadRight)
	case 3:
		set = set.With(device.ButtonDpadDown).With(device.ButtonDpadRight)
	case 4:
		set = set.With(device.ButtonDpadDown)
	case 5:
		set = set.With(device.ButtonDpadDown).With(device.ButtonDpadLeft)
	case 6:
		set = set.With(device.ButtonDpadLeft)
	case 7:
		set = set.With(device.ButtonDpadUp).With(device.ButtonDpadLeft)
	}
	bits := []struct {
		b    byte
		mask byte
		id   device.ButtonID
	}{
		{b0, 0x10, device.ButtonWest}, {b0, 0x20, device.ButtonSouth}, {b0, 0x40, device.ButtonEast}, {b0, 0x80, device.ButtonNorth},
		{b1, 0x01, device.ButtonL1}, {b1, 0x02, device.ButtonR1}, {b1, 0x04, device.ButtonL2Digital}, {b1, 0x08, device.ButtonR2Digital},
		{b1, 0x10, device.ButtonView}, {b1, 0x20, device.ButtonMenu}, {b1, 0x40, device.ButtonL3}, {b1, 0x80, device.ButtonR3},
		{b2, 0x01, device.ButtonHome}, {b2, 0x02, device.ButtonTouchpadClick}, {b2, 0x04, device.ButtonMicMute},
		{b2, 0x10, device.ButtonLeftFn}, {b2, 0x20, device.ButtonRightFn}, {b2, 0x40, device.ButtonLeftPaddle}, {b2, 0x80, device.ButtonRightPaddle},
	}
	for _, bt := range bits {
		if bt.b&bt.mask != 0 {
			set = set.With(bt.id)
		}
	}
	return set
}

// EncodeButtons is the inverse of DecodeButtons (used by the mock to build realistic reports).
func EncodeButtons(set device.ButtonSet) (b0, b1, b2 byte) {
	up, down, left, right := set.Has(device.ButtonDpadUp), set.Has(device.ButtonDpadDown), set.Has(device.ButtonDpadLeft), set.Has(device.ButtonDpadRight)
	hat := byte(8)
	switch {
	case up && right:
		hat = 1
	case down && right:
		hat = 3
	case down && left:
		hat = 5
	case up && left:
		hat = 7
	case up:
		hat = 0
	case right:
		hat = 2
	case down:
		hat = 4
	case left:
		hat = 6
	}
	b0 = hat
	for i := 0; i < 24; i++ {
		probe := [3]byte{}
		probe[i/8] = 1 << (i % 8)
		if i < 4 {
			continue // hat nibble
		}
		dec := DecodeButtons(probe[0]|8, probe[1], probe[2])
		for id := device.ButtonID(0); id < 64; id++ {
			if dec.Has(id) && set.Has(id) {
				switch i / 8 {
				case 0:
					b0 |= probe[0]
				case 1:
					b1 |= probe[1]
				case 2:
					b2 |= probe[2]
				}
			}
		}
	}
	return b0, b1, b2
}

func normalizeAxis(b byte) float64 {
	v := (float64(b) - 127.5) / 127.5
	if v > -0.008 && v < 0.008 {
		return 0
	}
	return clampFloat(v, -1.0, 1.0)
}

func clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// FormatHexPreview formats bytes as uppercase space-separated hex pairs.
func FormatHexPreview(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) > 24 {
		b = b[:24]
	}
	h := strings.ToUpper(hex.EncodeToString(b))
	var sb strings.Builder
	sb.Grow(len(b)*3 - 1)
	for i := 0; i < len(h); i += 2 {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(h[i : i+2])
	}
	return sb.String()
}
