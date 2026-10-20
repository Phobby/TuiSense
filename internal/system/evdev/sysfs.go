package evdev

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Bits is a kernel capability bitmap (bit n set = code n supported).
type Bits []uint64

// Has reports whether bit n is set.
func (b Bits) Has(n uint16) bool {
	w := int(n) / 64
	return w < len(b) && b[w]&(1<<(uint(n)%64)) != 0
}

// Set returns the indices of all set bits in ascending order.
func (b Bits) Set() []uint16 {
	var out []uint16
	for w, word := range b {
		for i := 0; i < 64; i++ {
			if word&(1<<uint(i)) != 0 {
				out = append(out, uint16(w*64+i))
			}
		}
	}
	return out
}

// ParseBits parses a sysfs capability bitmap ("1b 0 3" = words, most
// significant first, each one unsigned long of the running kernel).
func ParseBits(s string) Bits {
	fields := strings.Fields(s)
	out := make(Bits, len(fields))
	for i, f := range fields {
		v, err := strconv.ParseUint(f, 16, 64)
		if err != nil {
			continue
		}
		out[len(fields)-1-i] = v
	}
	return out
}

// Capabilities are the event bitmaps an input device advertises.
type Capabilities struct {
	EV, Key, Abs, FF Bits
}

// USBInfo describes the USB device an input node belongs to. Serial numbers
// are deliberately never read.
type USBInfo struct {
	VendorID, ProductID, BCDDevice uint16
	Manufacturer, Product          string
	SpeedMbps                      string
	USBVersion                     string
	BusNum, DevNum                 int
	Path                           string // sysfs path of the USB device
}

// InputDevice is one /dev/input/event* node discovered through sysfs.
type InputDevice struct {
	EventPath    string // /dev/input/eventN
	EventName    string // eventN
	Name         string
	Phys         string
	Bus, Vendor  uint16
	Product      uint16
	Version      uint16
	Caps         Capabilities
	SysfsPath    string // resolved inputX directory
	KernelDriver string // driver of the parent device, e.g. "xpad", "hid-generic"
	USB          *USBInfo
}

// HardwareKey is a non-unique identity used to remember a model choice:
// "vid:pid:version" in lowercase hex. It contains no serial number.
func (d InputDevice) HardwareKey() string {
	return strings.ToLower(hex4(d.Vendor) + ":" + hex4(d.Product) + ":" + hex4(d.Version))
}

func hex4(v uint16) string {
	s := strconv.FormatUint(uint64(v), 16)
	return strings.Repeat("0", 4-len(s)) + s
}

// IsGamepad reports whether the device looks like a gamepad or joystick:
// gamepad/joystick buttons plus at least an X/Y axis pair or a hat.
func (d InputDevice) IsGamepad() bool {
	if !d.Caps.EV.Has(EvKey) || !d.Caps.EV.Has(EvAbs) {
		return false
	}
	buttons := false
	for c := BtnJoystickFirst; c <= BtnThumbR; c++ {
		if d.Caps.Key.Has(c) {
			buttons = true
			break
		}
	}
	axes := (d.Caps.Abs.Has(AbsX) && d.Caps.Abs.Has(AbsY)) || (d.Caps.Abs.Has(AbsHat0X) && d.Caps.Abs.Has(AbsHat0Y))
	return buttons && axes
}

// Virtual reports whether the node was created in software (uinput, e.g.
// Steam Input's virtual pads) rather than by a physical device.
func (d InputDevice) Virtual() bool { return strings.Contains(d.SysfsPath, "/devices/virtual/") }

// HasFF reports whether force-feedback code c is supported.
func (d InputDevice) HasFF(c uint16) bool { return d.Caps.EV.Has(EvFF) && d.Caps.FF.Has(c) }

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readHex16(path string) uint16 {
	v, _ := strconv.ParseUint(readTrim(path), 16, 16)
	return uint16(v)
}

// Scan enumerates every event node under root (normally "/sys"). devRoot is
// the directory holding the device nodes (normally "/dev/input").
func Scan(root, devRoot string) ([]InputDevice, error) {
	classDir := filepath.Join(root, "class", "input")
	entries, err := os.ReadDir(classDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []InputDevice
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "event") {
			continue
		}
		if d, ok := readDevice(filepath.Join(classDir, name), name, devRoot); ok {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return eventNum(out[i].EventName) < eventNum(out[j].EventName) })
	return out, nil
}

func eventNum(name string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(name, "event"))
	return n
}

func readDevice(eventDir, name, devRoot string) (InputDevice, bool) {
	inputDir, err := filepath.EvalSymlinks(filepath.Join(eventDir, "device"))
	if err != nil {
		return InputDevice{}, false
	}
	d := InputDevice{
		EventPath: filepath.Join(devRoot, name),
		EventName: name,
		Name:      readTrim(filepath.Join(inputDir, "name")),
		Phys:      readTrim(filepath.Join(inputDir, "phys")),
		Bus:       readHex16(filepath.Join(inputDir, "id", "bustype")),
		Vendor:    readHex16(filepath.Join(inputDir, "id", "vendor")),
		Product:   readHex16(filepath.Join(inputDir, "id", "product")),
		Version:   readHex16(filepath.Join(inputDir, "id", "version")),
		SysfsPath: inputDir,
		Caps: Capabilities{
			EV:  ParseBits(readTrim(filepath.Join(inputDir, "capabilities", "ev"))),
			Key: ParseBits(readTrim(filepath.Join(inputDir, "capabilities", "key"))),
			Abs: ParseBits(readTrim(filepath.Join(inputDir, "capabilities", "abs"))),
			FF:  ParseBits(readTrim(filepath.Join(inputDir, "capabilities", "ff"))),
		},
	}
	if parent, err := filepath.EvalSymlinks(filepath.Join(inputDir, "device")); err == nil {
		if drv, err := filepath.EvalSymlinks(filepath.Join(parent, "driver")); err == nil {
			d.KernelDriver = filepath.Base(drv)
		}
		d.USB = findUSB(parent)
	}
	return d, true
}

// findUSB walks up from dir to the USB device (the directory with idVendor).
func findUSB(dir string) *USBInfo {
	for i := 0; i < 8 && dir != "/" && dir != "." && dir != ""; i++ {
		if _, err := os.Stat(filepath.Join(dir, "idVendor")); err == nil {
			u := &USBInfo{
				VendorID:     readHex16(filepath.Join(dir, "idVendor")),
				ProductID:    readHex16(filepath.Join(dir, "idProduct")),
				BCDDevice:    readHex16(filepath.Join(dir, "bcdDevice")),
				Manufacturer: readTrim(filepath.Join(dir, "manufacturer")),
				Product:      readTrim(filepath.Join(dir, "product")),
				SpeedMbps:    readTrim(filepath.Join(dir, "speed")),
				USBVersion:   readTrim(filepath.Join(dir, "version")),
				Path:         dir,
			}
			u.BusNum, _ = strconv.Atoi(readTrim(filepath.Join(dir, "busnum")))
			u.DevNum, _ = strconv.Atoi(readTrim(filepath.Join(dir, "devnum")))
			return u
		}
		dir = filepath.Dir(dir)
	}
	return nil
}
