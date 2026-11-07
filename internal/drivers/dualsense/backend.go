//go:build linux || (darwin && cgo)

package dualsense

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// HIDBackend implements device.Device for a physical Sony DualSense or
// DualSense Edge over a raw HID report port: Linux /dev/hidraw* or macOS IOKit.
// Everything above the port (parsing, output reports, pulses) is shared.
type HIDBackend struct {
	mu      sync.RWMutex
	desc    device.Descriptor
	info    device.Info
	cfg     device.Config
	tel     device.Telemetry
	port    reportPort // nil when the device could not be opened
	btSeq   uint8
	logger  *logging.Logger
	readBuf [128]byte

	powerSupplyDir string
	lastSeq        uint8
	haveSeq        bool
	droppedReports uint64
	readErrors     uint64

	fwIdentity device.FirmwareIdentity
	details    []device.Detail
}

// reportPort is the OS-specific raw HID access. Reports and feature buffers
// start with their report ID, as on Linux hidraw.
type reportPort interface {
	// Read returns one queued input report without waiting, or
	// syscall.EAGAIN when none is queued.
	Read(buf []byte) (int, error)
	Write(report []byte) error
	// GetFeature reads the feature report whose ID is buf[0] into buf.
	GetFeature(buf []byte) error
	Close() error
}

// newHIDBackend prepares a backend for an opened (or, with port nil, an
// unopenable) controller and reads its firmware and pairing feature reports.
func newHIDBackend(dev system.DiscoveredDevice, port reportPort, readOnly bool, unopenedMsg string, logger *logging.Logger) *HIDBackend {
	desc := Descriptor()
	if dev.ProductID == system.DualSenseEdgePID {
		desc = EdgeDescriptor()
	}

	inMode := "HID Report 0x01 (USB 64B)"
	outMode := "HID Report 0x02 (USB 48B)"
	if dev.Connection == device.ConnBluetooth {
		inMode = "HID Report 0x31 (BT 78B)"
		outMode = "HID Report 0x31 (BT 78B + CRC32)"
	}

	b := &HIDBackend{
		desc: desc,
		info: device.Info{
			ID:               dev.HIDRawName,
			Name:             dev.Name,
			Model:            dev.Model,
			VendorID:         dev.VendorID,
			ProductID:        dev.ProductID,
			MACAddress:       dev.MACAddress,
			HIDRawPath:       dev.HIDRawPath,
			InputDevicePath:  dev.InputDevicePath,
			SysfsPath:        dev.SysfsPath,
			Connection:       dev.Connection,
			InputMode:        inMode,
			OutputMode:       outMode,
			Connected:        true,
			ReadOnly:         readOnly,
			PermissionDenied: dev.PermissionDenied,
			PermissionHint:   dev.PermissionHint,
			LastSeen:         time.Now(),
		},
		powerSupplyDir: dev.PowerSupplyDir,
		cfg:            device.DefaultConfig(),
		port:           port,
		logger:         logger,
		tel: device.Telemetry{
			BatteryPercent: -1, // unknown until a real HID report or sysfs sample arrives
		},
	}

	if dev.HasSysfsBattery {
		b.tel.BatteryPercent = dev.SysfsBatteryPct
		b.tel.Charging = dev.SysfsCharging
	}

	var fi *FirmwareInfo
	fwErr := unopenedMsg
	if port != nil {
		fi, fwErr = b.queryFeatureReports()
	}
	if fi != nil {
		b.fwIdentity = fi.Identity()
	} else {
		b.fwIdentity = device.FirmwareIdentity{Source: "HID feature report 0x20", Error: fwErr}
		if logger != nil {
			logger.Warn("FIRMWARE", "%s", fwErr)
		}
	}

	// On Linux the kernel's own copy of the firmware words is available in
	// sysfs even when the hidraw node is not readable.
	sys := system.ReadHIDSysfs(dev.SysfsPath, SysfsFirmwareVersion, SysfsHardwareVersion)
	b.info.KernelDriver = sys.Driver
	b.applyVersions(fi, sys)
	b.details = BuildDetails(DetailsInput{Info: b.info, Sysfs: sys, Firmware: fi, FirmwareErr: fwErr})
	return b
}

// featureCalibration is read over Bluetooth only for its side effect: the
// controller then switches from its short report to the full 0x31 report, as
// the Linux kernel driver does when it connects. Nothing is written.
const (
	featureCalibration     byte = 0x05
	featureCalibrationSize      = 41
)

// queryFeatureReports issues GET_FEATURE for Firmware Info (0x20) and, when
// the MAC is unknown, Pairing Info (0x09). These are the same reads the
// kernel driver performs when the controller connects; they change nothing on
// the controller. No feature report is ever sent.
func (b *HIDBackend) queryFeatureReports() (*FirmwareInfo, string) {
	if b.info.Connection == device.ConnBluetooth {
		var cal [featureCalibrationSize]byte
		cal[0] = featureCalibration
		_ = b.port.GetFeature(cal[:])
	}
	var fi *FirmwareInfo
	fwErr := ""
	var fwBuf [FeatureFirmwareSize]byte
	fwBuf[0] = FeatureFirmwareReport
	if err := b.port.GetFeature(fwBuf[:]); err != nil {
		fwErr = fmt.Sprintf("feature report 0x20 could not be read: %v", err)
	} else if parsed, err := ParseFirmwareInfo(fwBuf[:], b.info.Connection == device.ConnBluetooth); err != nil {
		fwErr = err.Error()
	} else {
		fi = &parsed
	}

	if b.info.MACAddress == "" {
		var macBuf [FeaturePairingSize]byte
		macBuf[0] = FeaturePairingReport
		if err := b.port.GetFeature(macBuf[:]); err == nil {
			b.info.MACAddress = fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
				macBuf[6], macBuf[5], macBuf[4], macBuf[3], macBuf[2], macBuf[1])
		}
	}
	return fi, fwErr
}

// applyVersions fills the summary version fields from what was read.
func (b *HIDBackend) applyVersions(fi *FirmwareInfo, sys system.HIDSysfsInfo) {
	b.info.FirmwareVersion, b.info.FirmwareBuild, b.info.HardwareVersion = versionStrings(fi, sys)
}

func (b *HIDBackend) Descriptor() device.Descriptor { return b.desc }

// FirmwareIdentity returns the firmware data read when the device was opened.
func (b *HIDBackend) FirmwareIdentity() device.FirmwareIdentity {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.fwIdentity
}

// Details returns the identification data read when the device was opened.
func (b *HIDBackend) Details() []device.Detail {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.details
}

func (b *HIDBackend) Capabilities() device.CapabilitySet {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return runtimeCapabilities(b.desc.Capabilities, b.info.Connection)
}

func (b *HIDBackend) Info() device.Info {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.info
}

func (b *HIDBackend) Telemetry() device.Telemetry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.tel
}

func (b *HIDBackend) CurrentConfig() device.Config {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cfg
}

func (b *HIDBackend) Poll(_ context.Context) (device.Telemetry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.port == nil {
		return b.tel, nil
	}

	// Drain every queued report (bounded) so telemetry reflects the newest state
	// and no button press between polls is lost: buttons from all drained
	// reports are OR-ed into ButtonsLatched. DualSense sends ~250 Hz over USB
	// while the session polls at 25 Hz.
	var latched device.ButtonSet
	var samples []device.RawSample
	got := false
	for i := 0; i < maxDrain; i++ {
		n, err := b.port.Read(b.readBuf[:])
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				break
			}
			b.readErrors++
			b.tel.ReadErrors = b.readErrors
			b.info.Connected = false
			return b.tel, err
		}
		if n <= 0 {
			break
		}
		report := b.readBuf[:n]
		parsed, perr := ParseInputReport(report)
		if perr != nil {
			b.readErrors++
			continue
		}
		if seq, ok := ReportSequence(report); ok {
			if b.haveSeq {
				if gap := uint8(seq - b.lastSeq - 1); gap > 0 && gap < 128 {
					b.droppedReports += uint64(gap)
				}
			}
			b.lastSeq, b.haveSeq = seq, true
		}
		latched |= parsed.Buttons
		samples = append(samples, device.RawSample{At: parsed.UpdatedAt, Sticks: parsed.Raw.Sticks, Triggers: parsed.Raw.Triggers, Buttons: parsed.Buttons})
		parsed.LastOutputHex = b.tel.LastOutputHex
		parsed.PacketsRx = b.tel.PacketsRx + 1
		parsed.PacketsTx = b.tel.PacketsTx
		b.tel = parsed
		b.info.LastSeen = parsed.UpdatedAt
		got = true
	}
	if got {
		b.tel.ButtonsLatched = latched
	} else {
		b.tel.ButtonsLatched = b.tel.Buttons
	}
	// Every report since the last poll, so diagnostics see the full ~250 Hz
	// stream (an empty, non-nil slice means "no new report").
	if samples == nil {
		samples = []device.RawSample{}
	}
	b.tel.Samples = samples
	b.tel.ReportsDropped = b.droppedReports
	b.tel.ReadErrors = b.readErrors
	return b.tel, nil
}

// maxDrain bounds the reports read per poll (≈0.5 s of USB input at 250 Hz).
const maxDrain = 128

// BatteryDetails returns the battery attributes the kernel exposes for this
// controller (hid-playstation typically provides only capacity and status).
func (b *HIDBackend) BatteryDetails() []device.BatteryAttribute {
	var out []device.BatteryAttribute
	for _, a := range system.ReadPowerSupplyAttrs(b.powerSupplyDir) {
		out = append(out, device.BatteryAttribute{Name: a.Name, Value: a.Value, Source: "sysfs power_supply"})
	}
	return out
}

func (b *HIDBackend) ApplyConfig(_ context.Context, cfg device.Config) error {
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
	return b.sendCurrentConfig()
}

func (b *HIDBackend) ExecuteCommand(ctx context.Context, cmd device.Command) error {
	b.mu.Lock()
	switch cmd.Type {
	case device.CmdApplyFullConfig:
		b.cfg = cmd.Config
	case device.CmdSetLighting:
		if cmd.Lighting != nil {
			b.cfg.Lighting = *cmd.Lighting
		} else {
			b.cfg.Lighting = cmd.Config.Lighting
		}
	case device.CmdSetTrigger:
		if cmd.LeftTrigger != nil {
			b.cfg.LeftTrigger = *cmd.LeftTrigger
		} else {
			b.cfg.LeftTrigger = cmd.Config.LeftTrigger
		}
		if cmd.RightTrigger != nil {
			b.cfg.RightTrigger = *cmd.RightTrigger
		} else {
			b.cfg.RightTrigger = cmd.Config.RightTrigger
		}
	case device.CmdSetHaptics:
		if cmd.Haptics != nil {
			b.cfg.Haptics = *cmd.Haptics
		} else {
			b.cfg.Haptics = cmd.Config.Haptics
		}
	case device.CmdSetMicLED:
		if cmd.MicLED != nil {
			b.cfg.Lighting.MicLED = *cmd.MicLED
		}
	case device.CmdSetSpeakerVol:
		b.cfg.Audio = cmd.Config.Audio
	case device.CmdPulseHaptics:
		b.mu.Unlock()
		return b.pulse(ctx, cmd.PulseLeft, cmd.PulseRight, cmd.PulseDur, cmd.PulseAbsolute)
	}
	b.mu.Unlock()

	return b.sendCurrentConfig()
}

func (b *HIDBackend) TriggerPulse(ctx context.Context, left, right uint8, duration time.Duration) error {
	return b.pulse(ctx, left, right, duration, false)
}

// maxPulse bounds any single pulse, whatever the caller asks for.
const maxPulse = 1500 * time.Millisecond

func (b *HIDBackend) pulse(ctx context.Context, left, right uint8, duration time.Duration, absolute bool) error {
	if duration <= 0 {
		duration = 280 * time.Millisecond
	}
	if duration > maxPulse {
		duration = maxPulse
	}
	b.mu.Lock()
	origLeft, origRight := b.cfg.Haptics.LeftMotor, b.cfg.Haptics.RightMotor
	origMaster := b.cfg.Haptics.MasterIntensity
	b.cfg.Haptics.LeftMotor = left
	b.cfg.Haptics.RightMotor = right
	if absolute {
		b.cfg.Haptics.MasterIntensity = 100
	}
	b.mu.Unlock()

	if err := b.sendCurrentConfig(); err != nil {
		return err
	}

	go func() {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		b.mu.Lock()
		b.cfg.Haptics.LeftMotor = origLeft
		b.cfg.Haptics.RightMotor = origRight
		b.cfg.Haptics.MasterIntensity = origMaster
		b.mu.Unlock()
		_ = b.sendCurrentConfig()
	}()
	return nil
}

func (b *HIDBackend) sendCurrentConfig() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	pkt := BuildOutputReport(b.cfg, b.info.Connection, b.btSeq)
	b.btSeq = (b.btSeq + 1) & 0x0F
	b.tel.LastOutputHex = FormatHexPreview(pkt)

	if b.info.PermissionDenied || b.port == nil || b.info.ReadOnly {
		return fmt.Errorf("cannot write HID report: %s", b.info.PermissionHint)
	}

	if err := b.port.Write(pkt); err != nil {
		if os.IsPermission(err) {
			b.info.PermissionDenied = true
			b.info.PermissionHint = system.PermissionExplanation(b.info.HIDRawPath)
		}
		return fmt.Errorf("HID write %s: %w", b.info.HIDRawPath, err)
	}
	b.tel.PacketsTx++
	b.info.LastSeen = time.Now()
	return nil
}

func (b *HIDBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.port != nil {
		err := b.port.Close()
		b.port = nil
		return err
	}
	return nil
}

var (
	_ device.Device                   = (*HIDBackend)(nil)
	_ device.BatteryDetailsProvider   = (*HIDBackend)(nil)
	_ device.FirmwareIdentityProvider = (*HIDBackend)(nil)
	_ device.DetailsProvider          = (*HIDBackend)(nil)
)
