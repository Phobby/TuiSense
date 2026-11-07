//go:build darwin && cgo

package dualsense

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/hid/IOHIDManager.h>
#include <IOKit/hid/IOHIDDevice.h>
#include <IOKit/hid/IOHIDKeys.h>

typedef struct {
	uint64_t entry;
	int pid;
	int bluetooth;
	char serial[64];
	char product[128];
} ts_dev;

static int ts_int_prop(IOHIDDeviceRef d, CFStringRef key) {
	int v = 0;
	CFTypeRef p = IOHIDDeviceGetProperty(d, key);
	if (p && CFGetTypeID(p) == CFNumberGetTypeID()) CFNumberGetValue((CFNumberRef)p, kCFNumberIntType, &v);
	return v;
}

static void ts_str_prop(IOHIDDeviceRef d, CFStringRef key, char *out, CFIndex n) {
	out[0] = 0;
	CFTypeRef p = IOHIDDeviceGetProperty(d, key);
	if (p && CFGetTypeID(p) == CFStringGetTypeID()) CFStringGetCString((CFStringRef)p, out, n, kCFStringEncodingUTF8);
}

// ts_scan lists Sony gamepad HID interfaces. No device is opened.
static int ts_scan(ts_dev *out, int max) {
	IOHIDManagerRef m = IOHIDManagerCreate(kCFAllocatorDefault, kIOHIDOptionsTypeNone);
	if (!m) return -1;
	CFMutableDictionaryRef match = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int vid = 0x054C;
	CFNumberRef n = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &vid);
	CFDictionarySetValue(match, CFSTR(kIOHIDVendorIDKey), n);
	CFRelease(n);
	IOHIDManagerSetDeviceMatching(m, match);
	CFRelease(match);
	CFSetRef set = IOHIDManagerCopyDevices(m);
	int count = 0;
	if (set) {
		CFIndex total = CFSetGetCount(set);
		const void **devs = malloc(sizeof(void *) * (total ? total : 1));
		CFSetGetValues(set, devs);
		for (CFIndex i = 0; i < total && count < max; i++) {
			IOHIDDeviceRef d = (IOHIDDeviceRef)devs[i];
			if (ts_int_prop(d, CFSTR(kIOHIDPrimaryUsagePageKey)) != 0x01 || ts_int_prop(d, CFSTR(kIOHIDPrimaryUsageKey)) != 0x05) continue;
			ts_dev *o = &out[count];
			memset(o, 0, sizeof(*o));
			o->pid = ts_int_prop(d, CFSTR(kIOHIDProductIDKey));
			char transport[32];
			ts_str_prop(d, CFSTR(kIOHIDTransportKey), transport, sizeof transport);
			o->bluetooth = strncmp(transport, "Bluetooth", 9) == 0;
			ts_str_prop(d, CFSTR(kIOHIDSerialNumberKey), o->serial, sizeof o->serial);
			ts_str_prop(d, CFSTR(kIOHIDProductKey), o->product, sizeof o->product);
			if (IORegistryEntryGetRegistryEntryID(IOHIDDeviceGetService(d), &o->entry) != KERN_SUCCESS) continue;
			count++;
		}
		free(devs);
		CFRelease(set);
	}
	CFRelease(m);
	return count;
}

static IOHIDDeviceRef ts_open(uint64_t entry, int *ret) {
	io_service_t svc = IOServiceGetMatchingService(MACH_PORT_NULL, IORegistryEntryIDMatching(entry));
	if (!svc) { *ret = -1; return NULL; }
	IOHIDDeviceRef d = IOHIDDeviceCreate(kCFAllocatorDefault, svc);
	IOObjectRelease(svc);
	if (!d) { *ret = -1; return NULL; }
	IOReturn r = IOHIDDeviceOpen(d, kIOHIDOptionsTypeNone);
	if (r != kIOReturnSuccess) { CFRelease(d); *ret = r; return NULL; }
	*ret = 0;
	return d;
}

extern void tsGoReport(uintptr_t ctx, uint8_t *data, CFIndex len);
extern void tsGoRemoved(uintptr_t ctx);

static void ts_report_cb(void *ctx, IOReturn result, void *sender, IOHIDReportType type, uint32_t id, uint8_t *report, CFIndex len) {
	tsGoReport((uintptr_t)ctx, report, len);
}
static void ts_removed_cb(void *ctx, IOReturn result, void *sender) { tsGoRemoved((uintptr_t)ctx); }

#define TS_BUF 256

// ts_start, ts_run and ts_stop run on one locked OS thread (its run loop).
static uint8_t *ts_start(IOHIDDeviceRef d, uintptr_t ctx) {
	uint8_t *buf = malloc(TS_BUF);
	IOHIDDeviceRegisterInputReportCallback(d, buf, TS_BUF, ts_report_cb, (void *)ctx);
	IOHIDDeviceRegisterRemovalCallback(d, ts_removed_cb, (void *)ctx);
	IOHIDDeviceScheduleWithRunLoop(d, CFRunLoopGetCurrent(), kCFRunLoopDefaultMode);
	return buf;
}
static void ts_run(double seconds) { CFRunLoopRunInMode(kCFRunLoopDefaultMode, seconds, false); }
static void ts_stop(IOHIDDeviceRef d, uint8_t *buf) {
	IOHIDDeviceRegisterInputReportCallback(d, buf, TS_BUF, NULL, NULL);
	IOHIDDeviceRegisterRemovalCallback(d, NULL, NULL);
	IOHIDDeviceUnscheduleFromRunLoop(d, CFRunLoopGetCurrent(), kCFRunLoopDefaultMode);
	IOHIDDeviceClose(d, kIOHIDOptionsTypeNone);
	CFRelease(d);
	free(buf);
}

// Reports and feature buffers start with their report ID (as hidapi does).
static IOReturn ts_set_output(IOHIDDeviceRef d, uint8_t *data, CFIndex len) {
	return IOHIDDeviceSetReport(d, kIOHIDReportTypeOutput, data[0], data, len);
}
static IOReturn ts_get_feature(IOHIDDeviceRef d, uint8_t *data, CFIndex *len) {
	return IOHIDDeviceGetReport(d, kIOHIDReportTypeFeature, data[0], data, len);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/cgo"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/system"
)

// kIOReturnNotPermitted: macOS privacy settings refused the HID open.
const ioReturnNotPermitted = -536870174 // 0xe00002e2

// MacInputMonitoringHint explains the one permission macOS may require.
const MacInputMonitoringHint = "macOS did not allow access to the controller. Allow your terminal app in System Settings → Privacy & Security → Input Monitoring, then quit and reopen the terminal."

const iokitPrefix = "IOService:"

const rawHID = true

func scanDevices() ([]system.DiscoveredDevice, error) {
	var buf [16]C.ts_dev
	n := int(C.ts_scan(&buf[0], C.int(len(buf))))
	if n < 0 {
		return nil, errors.New("dualsense: IOKit HID manager unavailable")
	}
	var out []system.DiscoveredDevice
	for _, d := range buf[:n] {
		pid := uint16(d.pid)
		if pid != system.DualSenseProductID && pid != system.DualSenseEdgePID {
			continue
		}
		dev := system.DiscoveredDevice{
			HIDRawName: fmt.Sprintf("iokit-%x", uint64(d.entry)),
			HIDRawPath: fmt.Sprintf("%s%d", iokitPrefix, uint64(d.entry)),
			VendorID:   system.SonyVendorID,
			ProductID:  pid,
			Name:       C.GoString(&d.product[0]),
			Model:      "DualSense",
			Connection: device.ConnUSB,
			Readable:   true,
			Writable:   true,
		}
		if pid == system.DualSenseEdgePID {
			dev.Model = "DualSense Edge"
		}
		if d.bluetooth != 0 {
			dev.Connection = device.ConnBluetooth
			dev.MACAddress = macFromSerial(C.GoString(&d.serial[0]))
		}
		out = append(out, dev)
	}
	return out, nil
}

// macFromSerial accepts the "a0-ab-51-..." form IOKit reports for Bluetooth.
func macFromSerial(s string) string {
	if len(s) != 17 {
		return ""
	}
	b := []byte(s)
	for i := 2; i < 17; i += 3 {
		b[i] = ':'
	}
	return string(b)
}

func openBackend(dev system.DiscoveredDevice, logger *logging.Logger) (device.Device, error) {
	var entry uint64
	if _, err := fmt.Sscanf(dev.HIDRawPath, iokitPrefix+"%d", &entry); err != nil {
		return nil, fmt.Errorf("dualsense: unexpected IOKit path %q", dev.HIDRawPath)
	}
	port, code := openIOKitPort(entry)
	if port == nil {
		if code != ioReturnNotPermitted {
			return nil, fmt.Errorf("dualsense: IOHIDDeviceOpen failed (IOReturn 0x%x)", uint32(code))
		}
		dev.PermissionDenied = true
		dev.PermissionHint = MacInputMonitoringHint
		if logger != nil {
			logger.Warn("IOKIT", "%s", MacInputMonitoringHint)
		}
		return newHIDBackend(dev, nil, true, "macOS did not allow the controller to be opened", logger), nil
	}
	return newHIDBackend(dev, port, false, "", logger), nil
}

// iokitPort reads input reports through a run loop on a dedicated OS thread
// and queues them for the backend's non-blocking Poll.
type iokitPort struct {
	devMu sync.Mutex // dev; held across synchronous IOKit calls
	dev   C.IOHIDDeviceRef

	qmu     sync.Mutex // taken by the run-loop callbacks
	queue   [][]byte
	removed bool
	closed  bool

	stop   chan struct{}
	done   chan struct{}
	handle cgo.Handle
}

// maxQueued bounds memory if nothing polls (about 2 s of USB input).
const maxQueued = 512

func openIOKitPort(entry uint64) (*iokitPort, int) {
	p := &iokitPort{stop: make(chan struct{}), done: make(chan struct{})}
	p.handle = cgo.NewHandle(p)
	ready := make(chan int, 1)
	go func() {
		runtime.LockOSThread() // the run loop belongs to this thread
		defer runtime.UnlockOSThread()
		defer close(p.done)
		var ret C.int
		d := C.ts_open(C.uint64_t(entry), &ret)
		if d == 0 {
			ready <- int(ret)
			return
		}
		p.dev = d
		buf := C.ts_start(d, C.uintptr_t(p.handle))
		ready <- 0
		for {
			select {
			case <-p.stop:
				p.devMu.Lock()
				p.dev = 0
				p.devMu.Unlock()
				C.ts_stop(d, buf)
				return
			default:
				C.ts_run(0.05)
			}
		}
	}()
	if code := <-ready; code != 0 {
		p.handle.Delete()
		return nil, code
	}
	return p, 0
}

//export tsGoReport
func tsGoReport(ctx C.uintptr_t, data *C.uint8_t, n C.CFIndex) {
	p := cgo.Handle(ctx).Value().(*iokitPort)
	report := C.GoBytes(unsafe.Pointer(data), C.int(n))
	p.qmu.Lock()
	if len(p.queue) >= maxQueued {
		p.queue = p.queue[1:]
	}
	p.queue = append(p.queue, report)
	p.qmu.Unlock()
}

//export tsGoRemoved
func tsGoRemoved(ctx C.uintptr_t) {
	p := cgo.Handle(ctx).Value().(*iokitPort)
	p.qmu.Lock()
	p.removed = true
	p.qmu.Unlock()
}

var errRemoved = errors.New("the controller was disconnected")

func (p *iokitPort) Read(buf []byte) (int, error) {
	p.qmu.Lock()
	defer p.qmu.Unlock()
	if len(p.queue) == 0 {
		if p.removed || p.closed {
			return 0, errRemoved
		}
		return 0, syscall.EAGAIN
	}
	n := copy(buf, p.queue[0])
	p.queue = p.queue[1:]
	return n, nil
}

func (p *iokitPort) Write(report []byte) error {
	if len(report) == 0 {
		return errors.New("empty report")
	}
	if p.gone() {
		return errRemoved
	}
	p.devMu.Lock()
	defer p.devMu.Unlock()
	if p.dev == 0 {
		return errRemoved
	}
	c := C.CBytes(report)
	defer C.free(c)
	if r := C.ts_set_output(p.dev, (*C.uint8_t)(c), C.CFIndex(len(report))); r != 0 {
		return fmt.Errorf("IOHIDDeviceSetReport: IOReturn 0x%x", uint32(r))
	}
	return nil
}

func (p *iokitPort) GetFeature(buf []byte) error {
	if p.gone() {
		return errRemoved
	}
	p.devMu.Lock()
	defer p.devMu.Unlock()
	if p.dev == 0 {
		return errRemoved
	}
	c := (*C.uint8_t)(C.malloc(C.size_t(len(buf))))
	defer C.free(unsafe.Pointer(c))
	*c = C.uint8_t(buf[0])
	n := C.CFIndex(len(buf))
	if r := C.ts_get_feature(p.dev, c, &n); r != 0 {
		return fmt.Errorf("IOHIDDeviceGetReport: IOReturn 0x%x", uint32(r))
	}
	got := C.GoBytes(unsafe.Pointer(c), C.int(n))
	copy(buf, got)
	return nil
}

func (p *iokitPort) gone() bool {
	p.qmu.Lock()
	defer p.qmu.Unlock()
	return p.removed || p.closed
}

func (p *iokitPort) Close() error {
	p.qmu.Lock()
	if p.closed {
		p.qmu.Unlock()
		return nil
	}
	p.closed = true
	p.qmu.Unlock()
	close(p.stop)
	<-p.done
	p.handle.Delete()
	return nil
}
