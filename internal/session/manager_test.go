package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
	"github.com/Phobby/TuiSense/internal/logging"
)

// fakeDriver simulates attachable hardware without touching the OS.
type fakeDriver struct {
	mu      sync.Mutex
	present []string
}

func (d *fakeDriver) ID() string { return "fake" }
func (d *fakeDriver) Models() []device.Descriptor {
	return []device.Descriptor{dualsense.Descriptor()}
}
func (d *fakeDriver) set(keys ...string) {
	d.mu.Lock()
	d.present = keys
	d.mu.Unlock()
}
func (d *fakeDriver) Discover() ([]device.Candidate, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []device.Candidate
	for _, k := range d.present {
		out = append(out, device.Candidate{Key: k, Label: k})
	}
	return out, nil
}
func (d *fakeDriver) Open(c device.Candidate, logger *logging.Logger) (device.Device, error) {
	return &recordingDevice{MockBackend: dualsense.NewMockBackend(logger)}, nil
}

// recordingDevice wraps the DualSense mock and records executed commands.
type recordingDevice struct {
	*dualsense.MockBackend
	mu   sync.Mutex
	cmds []device.CommandType
}

func (r *recordingDevice) ExecuteCommand(ctx context.Context, cmd device.Command) error {
	r.mu.Lock()
	r.cmds = append(r.cmds, cmd.Type)
	r.mu.Unlock()
	return r.MockBackend.ExecuteCommand(ctx, cmd)
}

func newRegistry(drv device.Driver) *device.Registry {
	r := device.NewRegistry()
	if drv != nil {
		_ = r.RegisterDriver(drv)
	}
	_ = r.RegisterVirtual(dualsense.MockVirtual())
	_ = r.RegisterVirtual(xboxmock.Virtual())
	return r
}

type eventLog struct {
	mu  sync.Mutex
	evs []DeviceEvent
}

func (l *eventLog) add(ev DeviceEvent) {
	l.mu.Lock()
	l.evs = append(l.evs, ev)
	l.mu.Unlock()
}

func (l *eventLog) has(kind EventKind, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, ev := range l.evs {
		if ev.Kind == kind && ev.Key == key {
			return true
		}
	}
	return false
}

func TestHardwareIsPreferredAndTestDeviceNeverAutoSelected(t *testing.T) {
	drv := &fakeDriver{}
	drv.set("hw:1")
	m := NewManager(newRegistry(drv), Options{AllowMock: true}, nil, nil)
	if m.ActiveKey() != "hw:1" {
		t.Fatalf("expected hardware to be active, got %q", m.ActiveKey())
	}
	if m.Device(dualsense.MockKey) != nil {
		t.Fatalf("fallback mock must not attach when hardware exists")
	}
	if m.Device(xboxmock.Key) == nil {
		t.Fatalf("auto-attach test device should be listed")
	}
}

func TestMockFallbackAndForceMock(t *testing.T) {
	m := NewManager(newRegistry(&fakeDriver{}), Options{AllowMock: true}, nil, nil)
	if m.ActiveKey() != dualsense.MockKey {
		t.Fatalf("expected mock fallback, got %q", m.ActiveKey())
	}
	drv := &fakeDriver{}
	drv.set("hw:1")
	forced := NewManager(newRegistry(drv), Options{ForceMock: true}, nil, nil)
	if forced.ActiveKey() != dualsense.MockKey || forced.Device("hw:1") != nil {
		t.Fatalf("--mock must skip hardware entirely")
	}
}

func TestNoControllerFallback(t *testing.T) {
	m := NewManager(newRegistry(&fakeDriver{}), Options{AllowMock: false}, nil, nil)
	if m.Active() != nil || m.ActiveKey() != "" {
		t.Fatalf("expected no active device")
	}
	if len(m.Devices()) != 1 {
		t.Fatalf("only the test device should be listed, got %d", len(m.Devices()))
	}
	if dev := m.EnableMock(); dev == nil || m.ActiveKey() != dualsense.MockKey {
		t.Fatalf("EnableMock should attach and select the mock")
	}
}

func TestPreferredKeyAndCycling(t *testing.T) {
	m := NewManager(newRegistry(nil), Options{AllowMock: true, PreferredKey: xboxmock.Key}, nil, nil)
	if m.ActiveKey() != xboxmock.Key {
		t.Fatalf("preferred device should be restored, got %q", m.ActiveKey())
	}
	log := &eventLog{}
	m.onEvent = log.add
	if err := m.SelectNext(1); err != nil {
		t.Fatal(err)
	}
	if m.ActiveKey() != dualsense.MockKey || !log.has(EventSelected, dualsense.MockKey) {
		t.Fatalf("cycling should wrap to the mock and emit EventSelected")
	}
	if err := m.Select("missing"); err == nil {
		t.Fatalf("selecting an unknown key must fail")
	}
}

func TestHotplugTakeoverAndRemoval(t *testing.T) {
	drv := &fakeDriver{}
	log := &eventLog{}
	m := NewManager(newRegistry(drv), Options{AllowMock: true}, nil, log.add)
	if m.ActiveKey() != dualsense.MockKey {
		t.Fatalf("expected fallback mock first")
	}

	drv.set("hw:7")
	m.checkHotplug()
	if m.ActiveKey() != "hw:7" || !log.has(EventConnected, "hw:7") {
		t.Fatalf("hot-plugged hardware should take over from the fallback mock")
	}

	drv.set()
	m.checkHotplug()
	if m.ActiveKey() != "" || !log.has(EventDisconnected, "hw:7") || m.Device("hw:7") != nil {
		t.Fatalf("removed hardware should be detached and deselected")
	}
}

func TestUserChoiceIsNotOverriddenByHotplug(t *testing.T) {
	drv := &fakeDriver{}
	m := NewManager(newRegistry(drv), Options{AllowMock: true}, nil, nil)
	_ = m.Select(xboxmock.Key)
	drv.set("hw:2")
	m.checkHotplug()
	if m.ActiveKey() != xboxmock.Key {
		t.Fatalf("an explicitly selected test device must stay active, got %q", m.ActiveKey())
	}
}

func TestDispatcherRoutesToDeviceActiveAtDispatchTime(t *testing.T) {
	drv := &fakeDriver{}
	drv.set("hw:1")
	m := NewManager(newRegistry(drv), Options{}, nil, nil)
	d := NewDispatcher(m, nil, nil)

	d.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: device.DefaultConfig()})
	_ = m.Select(xboxmock.Key) // switch before the queue is flushed
	d.flushBatch(context.Background())

	hw := m.Device("hw:1").(*recordingDevice)
	hw.mu.Lock()
	defer hw.mu.Unlock()
	if len(hw.cmds) != 1 || hw.cmds[0] != device.CmdSetHaptics {
		t.Fatalf("command should reach the device that was active when dispatched, got %v", hw.cmds)
	}
}

func TestStartPollsActiveDevice(t *testing.T) {
	log := &eventLog{}
	m := NewManager(newRegistry(nil), Options{ForceMock: true}, nil, log.add)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	m.Start(ctx)
	if !log.has(EventTelemetry, dualsense.MockKey) {
		t.Fatalf("expected telemetry events from the active mock")
	}
}

// TestReenumerationKeepsTheControllerActive models an input-mode switch (or a
// confirmed re-identification): the device disappears under one key and
// reappears under another in the same scan. The replacement must become
// active instead of leaving the user with no controller.
func TestReenumerationKeepsTheControllerActive(t *testing.T) {
	drv := &fakeDriver{}
	log := &eventLog{}
	m := NewManager(newRegistry(drv), Options{}, nil, log.add)
	drv.set("evdev:/dev/input/event23#generic")
	m.checkHotplug()
	if m.ActiveKey() != "evdev:/dev/input/event23#generic" {
		t.Fatalf("first pad should become active, got %q", m.ActiveKey())
	}
	drv.set("evdev:/dev/input/event24#msi")
	m.checkHotplug()
	if m.ActiveKey() != "evdev:/dev/input/event24#msi" || m.Device("evdev:/dev/input/event23#generic") != nil {
		t.Fatalf("the re-enumerated controller must replace the old one, active=%q", m.ActiveKey())
	}
	if !log.has(EventDisconnected, "evdev:/dev/input/event23#generic") || !log.has(EventConnected, "evdev:/dev/input/event24#msi") {
		t.Fatal("both lifecycle events must be emitted")
	}
}

func TestRescanRunsOnThePollingGoroutine(t *testing.T) {
	drv := &fakeDriver{}
	m := NewManager(newRegistry(drv), Options{}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Start(ctx)
	drv.set("hw:9")
	m.Rescan()
	deadline := time.Now().Add(time.Second)
	for m.ActiveKey() != "hw:9" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if m.ActiveKey() != "hw:9" {
		t.Fatal("Rescan should trigger an immediate discovery")
	}
}
