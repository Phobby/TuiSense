package dualsense

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// LinuxBackend implements device.Device for a physical Sony DualSense or
// DualSense Edge attached via Linux /dev/hidraw*.
type LinuxBackend struct {
	mu      sync.RWMutex
	desc    device.Descriptor
	info    device.Info
	cfg     device.Config
	tel     device.Telemetry
	file    *os.File
	btSeq   uint8
	logger  *logging.Logger
	readBuf [128]byte

	powerSupplyDir string
	lastSeq        uint8
	haveSeq        bool
	droppedReports uint64
	readErrors     uint64
}

// OpenLinuxBackend opens a discovered DualSense hidraw node, queries firmware/pairing
// feature reports when permitted, and prepares non-blocking I/O.
func OpenLinuxBackend(dev system.DiscoveredDevice, logger *logging.Logger) (*LinuxBackend, error) {
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

	info := device.Info{
		ID:               dev.HIDRawName,
		Name:             dev.Name,
		Model:            dev.Model,
		VendorID:         dev.VendorID,
		ProductID:        dev.ProductID,
		FirmwareVersion:  "0458",
		HardwareVersion:  "0311",
		MACAddress:       dev.MACAddress,
		HIDRawPath:       dev.HIDRawPath,
		InputDevicePath:  dev.InputDevicePath,
		SysfsPath:        dev.SysfsPath,
		Connection:       dev.Connection,
		InputMode:        inMode,
		OutputMode:       outMode,
		Connected:        true,
		PermissionDenied: dev.PermissionDenied,
		PermissionHint:   dev.PermissionHint,
		LastSeen:         time.Now(),
	}

	var f *os.File
	var err error
	f, err = os.OpenFile(dev.HIDRawPath, os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		if os.IsPermission(err) {
			info.PermissionDenied = true
			info.ReadOnly = true
			info.PermissionHint = system.PermissionExplanation(dev.HIDRawPath)
			if logger != nil {
				logger.Warn("HIDRAW", "%s", info.PermissionHint)
			}
			// Attempt read-only open so telemetry still works if read access is permitted
			f, _ = os.OpenFile(dev.HIDRawPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		} else {
			return nil, fmt.Errorf("open %s: %w", dev.HIDRawPath, err)
		}
	}

	b := &LinuxBackend{
		desc:           desc,
		info:           info,
		powerSupplyDir: dev.PowerSupplyDir,
		cfg:            device.DefaultConfig(),
		file:           f,
		logger:         logger,
		tel: device.Telemetry{
			BatteryPercent: 80,
			Charging:       dev.Connection == device.ConnUSB,
			UpdatedAt:      time.Now(),
		},
	}

	if dev.HasSysfsBattery {
		b.tel.BatteryPercent = dev.SysfsBatteryPct
		b.tel.Charging = dev.SysfsCharging
	}

	if f != nil {
		b.queryFeatureReports()
	}

	return b, nil
}

// queryFeatureReports attempts HIDIOCGFEATURE ioctls for Firmware Info (0x20) and Pairing/MAC (0x09).
func (b *LinuxBackend) queryFeatureReports() {
	if b.file == nil {
		return
	}
	// Use the raw fd via SyscallConn: calling File.Fd() would switch the file
	// to blocking mode and break the non-blocking report drain in Poll.
	rc, err := b.file.SyscallConn()
	if err != nil {
		return
	}
	var fd uintptr
	_ = rc.Control(func(raw uintptr) { fd = raw })

	// Feature Report 0x20 (Firmware version)
	var fwBuf [FeatureFirmwareSize]byte
	fwBuf[0] = FeatureFirmwareReport
	if err := hidGetFeature(fd, fwBuf[:]); err == nil {
		hwVer := binary.LittleEndian.Uint32(fwBuf[24:28])
		fwVer := binary.LittleEndian.Uint32(fwBuf[28:32])
		if fwVer > 0 {
			b.info.FirmwareVersion = fmt.Sprintf("%04X", fwVer&0xFFFF)
		}
		if hwVer > 0 {
			b.info.HardwareVersion = fmt.Sprintf("%04X", hwVer&0xFFFF)
		}
	}

	// Feature Report 0x09 (Bluetooth MAC address if not already known via sysfs)
	if b.info.MACAddress == "" {
		var macBuf [FeaturePairingSize]byte
		macBuf[0] = FeaturePairingReport
		if err := hidGetFeature(fd, macBuf[:]); err == nil {
			b.info.MACAddress = fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
				macBuf[6], macBuf[5], macBuf[4], macBuf[3], macBuf[2], macBuf[1])
		}
	}
}

// hidGetFeature issues a Linux HIDIOCGFEATURE(len) ioctl on a hidraw file descriptor.
func hidGetFeature(fd uintptr, buf []byte) error {
	// _IOC(_IOC_READ|_IOC_WRITE, 'H', 0x07, len)
	ioc := uintptr((3 << 30) | (uintptr('H') << 8) | 0x07 | (uintptr(len(buf)) << 16))
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioc, uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return errno
	}
	return nil
}

func (b *LinuxBackend) Descriptor() device.Descriptor { return b.desc }

func (b *LinuxBackend) Capabilities() device.CapabilitySet {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return runtimeCapabilities(b.desc.Capabilities, b.info.Connection)
}

func (b *LinuxBackend) Info() device.Info {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.info
}

func (b *LinuxBackend) Telemetry() device.Telemetry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.tel
}

func (b *LinuxBackend) CurrentConfig() device.Config {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cfg
}

func (b *LinuxBackend) Poll(_ context.Context) (device.Telemetry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.file == nil {
		return b.tel, nil
	}

	// Drain every queued report (bounded) so telemetry reflects the newest state
	// and no button press between polls is lost: buttons from all drained
	// reports are OR-ed into ButtonsLatched. DualSense sends ~250 Hz over USB
	// while the session polls at 25 Hz.
	var latched device.ButtonSet
	got := false
	for i := 0; i < 128; i++ {
		n, err := b.readNonBlocking(b.readBuf[:])
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
	b.tel.ReportsDropped = b.droppedReports
	b.tel.ReadErrors = b.readErrors
	return b.tel, nil
}

// readNonBlocking performs one raw read that returns EAGAIN immediately when
// no report is queued. (File.Read on a pollable fd would park in Go's poller
// and wait for the next report instead.)
func (b *LinuxBackend) readNonBlocking(buf []byte) (int, error) {
	rc, err := b.file.SyscallConn()
	if err != nil {
		return 0, err
	}
	var n int
	var rerr error
	if err := rc.Read(func(fd uintptr) bool {
		n, rerr = syscall.Read(int(fd), buf)
		return true // never wait for readiness
	}); err != nil {
		return 0, err
	}
	if n < 0 {
		n = 0
	}
	return n, rerr
}

// BatteryDetails returns the battery attributes the kernel exposes for this
// controller (hid-playstation typically provides only capacity and status).
func (b *LinuxBackend) BatteryDetails() []device.BatteryAttribute {
	var out []device.BatteryAttribute
	for _, a := range system.ReadPowerSupplyAttrs(b.powerSupplyDir) {
		out = append(out, device.BatteryAttribute{Name: a.Name, Value: a.Value, Source: "sysfs power_supply"})
	}
	return out
}

func (b *LinuxBackend) ApplyConfig(_ context.Context, cfg device.Config) error {
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
	return b.sendCurrentConfig()
}

func (b *LinuxBackend) ExecuteCommand(ctx context.Context, cmd device.Command) error {
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

func (b *LinuxBackend) TriggerPulse(ctx context.Context, left, right uint8, duration time.Duration) error {
	return b.pulse(ctx, left, right, duration, false)
}

// maxPulse bounds any single pulse, whatever the caller asks for.
const maxPulse = 1500 * time.Millisecond

func (b *LinuxBackend) pulse(ctx context.Context, left, right uint8, duration time.Duration, absolute bool) error {
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

func (b *LinuxBackend) sendCurrentConfig() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	pkt := BuildOutputReport(b.cfg, b.info.Connection, b.btSeq)
	b.btSeq = (b.btSeq + 1) & 0x0F
	b.tel.LastOutputHex = FormatHexPreview(pkt)

	if b.info.PermissionDenied || b.file == nil || b.info.ReadOnly {
		return fmt.Errorf("cannot write HID report: %s", b.info.PermissionHint)
	}

	if _, err := b.file.Write(pkt); err != nil {
		if os.IsPermission(err) {
			b.info.PermissionDenied = true
			b.info.PermissionHint = system.PermissionExplanation(b.info.HIDRawPath)
		}
		return fmt.Errorf("hidraw write %s: %w", b.info.HIDRawPath, err)
	}
	b.tel.PacketsTx++
	b.info.LastSeen = time.Now()
	return nil
}

func (b *LinuxBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.file != nil {
		err := b.file.Close()
		b.file = nil
		return err
	}
	return nil
}
