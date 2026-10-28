package gamepad

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FindPowerSupply returns the kernel power_supply directory that belongs to
// the input device at sysfsInput (its HID or USB parent), or "". Drivers such
// as hid-playstation, hid-nintendo, xpad (wireless) and xpadneo register the
// controller battery there; it is a read-only, documented sysfs interface.
func FindPowerSupply(sysfsInput string) string {
	if sysfsInput == "" {
		return ""
	}
	dir := filepath.Dir(sysfsInput) // …/input
	for i := 0; i < 4 && strings.HasPrefix(dir, "/") && filepath.Base(dir) != "devices"; i++ {
		matches, _ := filepath.Glob(filepath.Join(dir, "power_supply", "*", "capacity"))
		if len(matches) > 0 {
			return filepath.Dir(matches[0])
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// ReadPowerSupply reads capacity (percent) and status from a power_supply directory.
func ReadPowerSupply(dir string) (percent int, charging, full, ok bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "capacity"))
	if err != nil {
		return -1, false, false, false
	}
	percent, err = strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || percent < 0 || percent > 100 {
		return -1, false, false, false
	}
	status, _ := os.ReadFile(filepath.Join(dir, "status"))
	switch strings.TrimSpace(string(status)) {
	case "Charging":
		charging = true
	case "Full":
		full = true
	}
	return percent, charging, full, true
}
