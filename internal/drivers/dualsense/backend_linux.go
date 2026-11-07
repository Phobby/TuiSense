package dualsense

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// LinuxBackend is the DualSense backend on Linux hidraw.
type LinuxBackend = HIDBackend

const rawHID = true

func scanDevices() ([]system.DiscoveredDevice, error) { return system.ScanDualSenseDevices() }

func openBackend(dev system.DiscoveredDevice, logger *logging.Logger) (device.Device, error) {
	return OpenLinuxBackend(dev, logger)
}

// OpenLinuxBackend opens a discovered DualSense hidraw node, queries firmware/pairing
// feature reports when permitted, and prepares non-blocking I/O.
func OpenLinuxBackend(dev system.DiscoveredDevice, logger *logging.Logger) (*LinuxBackend, error) {
	readOnly := false
	f, err := os.OpenFile(dev.HIDRawPath, os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		if !os.IsPermission(err) {
			return nil, fmt.Errorf("open %s: %w", dev.HIDRawPath, err)
		}
		dev.PermissionDenied = true
		dev.PermissionHint = system.PermissionExplanation(dev.HIDRawPath)
		readOnly = true
		if logger != nil {
			logger.Warn("HIDRAW", "%s", dev.PermissionHint)
		}
		// Attempt read-only open so telemetry still works if read access is permitted
		f, _ = os.OpenFile(dev.HIDRawPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	}
	var port reportPort
	if f != nil {
		port = hidrawPort{f}
	}
	return newHIDBackend(dev, port, readOnly, "the hidraw node could not be opened", logger), nil
}

// hidrawPort is a non-blocking /dev/hidraw* file.
type hidrawPort struct{ f *os.File }

// Read performs one raw read that returns EAGAIN immediately when no report
// is queued. (File.Read on a pollable fd would park in Go's poller and wait
// for the next report instead.)
func (p hidrawPort) Read(buf []byte) (int, error) {
	rc, err := p.f.SyscallConn()
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

func (p hidrawPort) Write(report []byte) error {
	_, err := p.f.Write(report)
	return err
}

// GetFeature issues HIDIOCGFEATURE(len) on the raw fd. SyscallConn, not
// File.Fd: Fd would switch the file to blocking mode and break Read.
func (p hidrawPort) GetFeature(buf []byte) error {
	rc, err := p.f.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	if err := rc.Control(func(fd uintptr) {
		// _IOC(_IOC_READ|_IOC_WRITE, 'H', 0x07, len)
		ioc := uintptr((3 << 30) | (uintptr('H') << 8) | 0x07 | (uintptr(len(buf)) << 16))
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, ioc, uintptr(unsafe.Pointer(&buf[0])))
	}); err != nil {
		return err
	}
	if errno != 0 {
		return errno
	}
	return nil
}

func (p hidrawPort) Close() error { return p.f.Close() }
