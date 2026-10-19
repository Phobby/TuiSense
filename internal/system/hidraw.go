package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/ygzztrk/tuisense/internal/device"
)

const (
	SonyVendorID       uint16 = 0x054C
	DualSenseProductID uint16 = 0x0CE6
	DualSenseEdgePID   uint16 = 0x0DF2
	BusUSB             uint16 = 0x0003
	BusBluetooth       uint16 = 0x0005
)

// DiscoveredDevice holds sysfs and devfs metadata for a detected DualSense controller.
type DiscoveredDevice struct {
	HIDRawName       string
	HIDRawPath       string
	SysfsPath        string
	InputDevicePath  string
	VendorID         uint16
	ProductID        uint16
	BusType          uint16
	Name             string
	Model            string
	MACAddress       string
	Connection       device.ConnectionType
	Readable         bool
	Writable         bool
	PermissionDenied bool
	PermissionHint   string
	PowerSupplyDir   string // /sys/.../power_supply/<name> when the kernel exposes one
	SysfsBatteryPct  int
	SysfsCharging    bool
	HasSysfsBattery  bool
}

// ScanDualSenseDevices inspects /sys/class/hidraw to locate connected Sony DualSense
// and DualSense Edge controllers via USB or Bluetooth.
func ScanDualSenseDevices() ([]DiscoveredDevice, error) {
	const sysHidrawDir = "/sys/class/hidraw"
	entries, err := os.ReadDir(sysHidrawDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", sysHidrawDir, err)
	}

	var devices []DiscoveredDevice
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "hidraw") {
			continue
		}
		sysPath := filepath.Join(sysHidrawDir, name)
		ueventPath := filepath.Join(sysPath, "device", "uevent")
		data, err := os.ReadFile(ueventPath)
		if err != nil {
			continue
		}

		meta := parseUevent(string(data))
		bus, vid, pid, ok := parseHIDID(meta["HID_ID"])
		if !ok || vid != SonyVendorID {
			continue
		}
		if pid != DualSenseProductID && pid != DualSenseEdgePID {
			continue
		}

		devPath := filepath.Join("/dev", name)
		conn := device.ConnUSB
		if bus == BusBluetooth {
			conn = device.ConnBluetooth
		}

		model := "DualSense Wireless Controller"
		if pid == DualSenseEdgePID {
			model = "DualSense Edge Wireless Controller"
		}
		hidName := meta["HID_NAME"]
		if hidName == "" {
			hidName = "Sony Interactive Entertainment " + model
		}

		mac := strings.ToUpper(strings.TrimSpace(meta["HID_UNIQ"]))
		if mac == "" {
			mac = readSysfsAttr(filepath.Join(sysPath, "device", "uniq"))
		}

		readable, writable, permDenied := checkDeviceAccess(devPath)
		var permHint string
		if permDenied {
			permHint = PermissionExplanation(devPath)
		}

		inputPath := findInputEventPath(filepath.Join(sysPath, "device", "input"))
		psRoot := filepath.Join(sysPath, "device", "power_supply")
		batPct, charging, hasBat := readSysfsBattery(psRoot)

		devices = append(devices, DiscoveredDevice{
			HIDRawName:       name,
			HIDRawPath:       devPath,
			SysfsPath:        sysPath,
			InputDevicePath:  inputPath,
			VendorID:         vid,
			ProductID:        pid,
			BusType:          bus,
			Name:             hidName,
			Model:            model,
			MACAddress:       mac,
			Connection:       conn,
			Readable:         readable,
			Writable:         writable,
			PermissionDenied: permDenied,
			PermissionHint:   permHint,
			PowerSupplyDir:   firstPowerSupply(psRoot),
			SysfsBatteryPct:  batPct,
			SysfsCharging:    charging,
			HasSysfsBattery:  hasBat,
		})
	}
	return devices, nil
}

func parseUevent(content string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.IndexByte(line, '='); idx > 0 {
			out[line[:idx]] = line[idx+1:]
		}
	}
	return out
}

// parseHIDID parses "0003:0000054C:00000CE6" into bus, vendor, product.
func parseHIDID(raw string) (bus, vid, pid uint16, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	b, err1 := strconv.ParseUint(parts[0], 16, 16)
	v, err2 := strconv.ParseUint(parts[1], 16, 32)
	p, err3 := strconv.ParseUint(parts[2], 16, 32)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return uint16(b), uint16(v), uint16(p), true
}

func checkDeviceAccess(devPath string) (readable, writable, permDenied bool) {
	f, err := os.OpenFile(devPath, os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err == nil {
		_ = f.Close()
		return true, true, false
	}
	if os.IsPermission(err) {
		// Check if at least read-only access works
		rf, rerr := os.OpenFile(devPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if rerr == nil {
			_ = rf.Close()
			return true, false, true
		}
		return false, false, true
	}
	return false, false, false
}

func findInputEventPath(inputDir string) string {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "input") {
			subEntries, err := os.ReadDir(filepath.Join(inputDir, e.Name()))
			if err != nil {
				continue
			}
			for _, sub := range subEntries {
				if strings.HasPrefix(sub.Name(), "event") {
					return filepath.Join("/dev/input", sub.Name())
				}
			}
		}
	}
	return ""
}

func readSysfsBattery(powerSupplyDir string) (pct int, charging bool, ok bool) {
	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil {
		return 0, false, false
	}
	for _, e := range entries {
		base := filepath.Join(powerSupplyDir, e.Name())
		capStr := readSysfsAttr(filepath.Join(base, "capacity"))
		if capStr == "" {
			continue
		}
		val, err := strconv.Atoi(capStr)
		if err != nil {
			continue
		}
		status := strings.ToLower(readSysfsAttr(filepath.Join(base, "status")))
		isCharging := strings.Contains(status, "charging") && !strings.Contains(status, "discharging")
		return val, isCharging, true
	}
	return 0, false, false
}

func readSysfsAttr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func firstPowerSupply(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		return filepath.Join(root, e.Name())
	}
	return ""
}

// PowerSupplyAttr is one attribute file of a Linux power_supply class device.
type PowerSupplyAttr struct {
	Name, Value string
}

// ReadPowerSupplyAttrs returns every readable, single-line attribute in a
// power_supply directory (e.g. capacity, status, voltage_now). Only values the
// kernel actually exposes are returned; nothing is derived or estimated.
func ReadPowerSupplyAttrs(dir string) []PowerSupplyAttr {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	skip := map[string]bool{"uevent": true, "type": true}
	var out []PowerSupplyAttr
	for _, e := range entries {
		if e.IsDir() || skip[e.Name()] {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o444 == 0 {
			continue
		}
		v := readSysfsAttr(filepath.Join(dir, e.Name()))
		if v == "" || strings.Contains(v, "\n") || len(v) > 64 {
			continue
		}
		out = append(out, PowerSupplyAttr{Name: e.Name(), Value: v})
	}
	return out
}
