//go:build windows

package winhid

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GUID_DEVINTERFACE_HID.
var hidGUID = windows.GUID{Data1: 0x4D1E55B2, Data2: 0xF16F, Data3: 0x11CF, Data4: [8]byte{0x88, 0xCB, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30}}

var (
	hid                  = windows.NewLazySystemDLL("hid.dll")
	procGetAttributes    = hid.NewProc("HidD_GetAttributes")
	procGetPreparsedData = hid.NewProc("HidD_GetPreparsedData")
	procFreePreparsed    = hid.NewProc("HidD_FreePreparsedData")
	procGetCaps          = hid.NewProc("HidP_GetCaps")
	procGetManufacturer  = hid.NewProc("HidD_GetManufacturerString")
	procGetProduct       = hid.NewProc("HidD_GetProductString")
)

const hidpStatusSuccess = 0x00110000

// Hot-plug scans run every 1.5 s: each path is queried once while present.
var cache = struct {
	sync.Mutex
	m map[string]Device
}{m: map[string]Device{}}

// Available reports whether hid.dll (part of every Windows install) loads.
func Available() bool { return hid.Load() == nil }

// Enumerate lists every present HID collection with its identity.
func Enumerate() ([]Device, error) {
	if err := hid.Load(); err != nil {
		return nil, err
	}
	paths, err := windows.CM_Get_Device_Interface_List("", &hidGUID, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil {
		return nil, err
	}
	cache.Lock()
	defer cache.Unlock()
	next := make(map[string]Device, len(paths))
	out := make([]Device, 0, len(paths))
	for _, p := range paths {
		d, ok := cache.m[p]
		if !ok {
			if d, ok = query(p); !ok {
				continue
			}
		}
		next[p] = d
		out = append(out, d)
	}
	cache.m = next
	return out, nil
}

// query opens the collection without read/write access (allowed even for
// devices another program holds exclusively) and reads its attributes.
func query(path string) (Device, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Device{}, false
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return Device{}, false
	}
	defer windows.CloseHandle(h)
	d := Device{Path: path}
	var attr struct {
		Size                          uint32
		VendorID, ProductID, Revision uint16
	}
	attr.Size = uint32(unsafe.Sizeof(attr))
	if r, _, _ := procGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&attr))); r == 0 {
		return Device{}, false
	}
	d.VendorID, d.ProductID, d.Version = attr.VendorID, attr.ProductID, attr.Revision
	var pre uintptr
	if r, _, _ := procGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&pre))); r != 0 {
		var caps [32]uint16 // HIDP_CAPS: Usage, UsagePage, then lengths and counts
		if st, _, _ := procGetCaps.Call(pre, uintptr(unsafe.Pointer(&caps))); st == hidpStatusSuccess {
			d.Usage, d.UsagePage = caps[0], caps[1]
		}
		procFreePreparsed.Call(pre)
	}
	// Strings are read only for game controllers (keyboards and mice are skipped).
	if d.IsGameController() {
		d.Manufacturer = hidString(procGetManufacturer, h)
		d.Product = hidString(procGetProduct, h)
	}
	return d, true
}

func hidString(proc *windows.LazyProc, h windows.Handle) string {
	var buf [256]uint16
	if r, _, _ := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2)); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:])
}
