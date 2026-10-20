// Package session owns the set of attached controllers (real, mock, and test),
// the active-device selection, telemetry polling, hot-plug, and command dispatch.
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/logging"
)

// EventKind identifies a device lifecycle notification emitted by Manager.
type EventKind string

const (
	EventConnected    EventKind = "connected"
	EventDisconnected EventKind = "disconnected"
	EventTelemetry    EventKind = "telemetry"
	EventSelected     EventKind = "selected"
	EventError        EventKind = "error"
	EventListChanged  EventKind = "list_changed" // a device was attached without becoming active
)

// DeviceEvent carries a controller lifecycle or telemetry update.
type DeviceEvent struct {
	Kind      EventKind
	Key       string
	Info      device.Info
	Telemetry device.Telemetry
	Err       error
}

// Summary is a UI-facing snapshot of one attached device for the selector.
type Summary struct {
	Key        string
	Descriptor device.Descriptor
	Source     device.Source
	Connected  bool
	Active     bool
}

type entry struct {
	key    string
	source device.Source
	driver device.Driver // nil for virtual devices
	dev    device.Device
}

// Options configures device attachment policy.
type Options struct {
	ForceMock    bool   // skip hardware, attach the simulated DualSense
	AllowMock    bool   // attach the simulated DualSense when no hardware is found
	PreferredKey string // device to select at startup if attached (persisted choice)
}

// Manager discovers, attaches, selects, hot-plugs, and polls controllers.
type Manager struct {
	mu        sync.RWMutex
	registry  *device.Registry
	opts      Options
	entries   []*entry
	activeKey string
	autoMock  bool // the simulated DualSense was attached only as a fallback
	logger    *logging.Logger
	onEvent   func(DeviceEvent)
	rescan    chan struct{}
}

// NewManager creates a manager and performs initial discovery synchronously.
func NewManager(reg *device.Registry, opts Options, logger *logging.Logger, onEvent func(DeviceEvent)) *Manager {
	m := &Manager{registry: reg, opts: opts, logger: logger, onEvent: onEvent, rescan: make(chan struct{}, 1)}
	m.discoverInitial()
	return m
}

func (m *Manager) logInfo(tag, format string, args ...any) {
	if m.logger != nil {
		m.logger.Info(tag, format, args...)
	}
}

func (m *Manager) logWarn(tag, format string, args ...any) {
	if m.logger != nil {
		m.logger.Warn(tag, format, args...)
	}
}

func (m *Manager) emit(ev DeviceEvent) {
	if m.onEvent != nil {
		m.onEvent(ev)
	}
}

func (m *Manager) discoverInitial() {
	if !m.opts.ForceMock {
		for _, drv := range m.registry.Drivers() {
			cands, err := drv.Discover()
			if err != nil {
				m.logWarn("MANAGER", "%s discovery warning: %v", drv.ID(), err)
			}
			for _, c := range cands {
				dev, err := drv.Open(c, m.logger)
				if err != nil {
					m.logWarn("MANAGER", "Failed to open %s: %v", c.Label, err)
					continue
				}
				m.entries = append(m.entries, &entry{key: c.Key, source: device.SourceHardware, driver: drv, dev: dev})
				m.logInfo("MANAGER", "Attached %s", c.Label)
			}
		}
	}

	hasHardware := len(m.entries) > 0
	for _, v := range m.registry.Virtuals() {
		switch {
		case v.Source == device.SourceMock && (m.opts.ForceMock || (m.opts.AllowMock && !hasHardware)):
			m.attachVirtualLocked(v)
			m.autoMock = !m.opts.ForceMock
		case v.AutoAttach:
			m.attachVirtualLocked(v)
		}
	}

	m.activeKey = m.defaultActiveLocked()
	if m.activeKey == "" {
		m.logWarn("MANAGER", "No controller detected. Waiting for hot-plug...")
	}
}

func (m *Manager) attachVirtualLocked(v device.VirtualDevice) *entry {
	if e := m.findLocked(v.Key); e != nil {
		return e
	}
	e := &entry{key: v.Key, source: v.Source, dev: v.New(m.logger)}
	m.entries = append(m.entries, e)
	m.logInfo("MANAGER", "Attached %s device %s", v.Source, v.Descriptor.DisplayName)
	return e
}

// defaultActiveLocked picks: preferred → first hardware → first mock → none.
// Test devices are never auto-selected; the user chooses them explicitly.
func (m *Manager) defaultActiveLocked() string {
	if m.opts.PreferredKey != "" && m.findLocked(m.opts.PreferredKey) != nil {
		return m.opts.PreferredKey
	}
	for _, src := range []device.Source{device.SourceHardware, device.SourceMock} {
		for _, e := range m.entries {
			if e.source == src {
				return e.key
			}
		}
	}
	return ""
}

func (m *Manager) findLocked(key string) *entry {
	for _, e := range m.entries {
		if e.key == key {
			return e
		}
	}
	return nil
}

// Active returns the active device, or nil when none is selected.
func (m *Manager) Active() device.Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e := m.findLocked(m.activeKey); e != nil {
		return e.dev
	}
	return nil
}

// ActiveKey returns the registry key of the active device ("" if none).
func (m *Manager) ActiveKey() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeKey
}

// Device returns the attached device for key.
func (m *Manager) Device(key string) device.Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e := m.findLocked(key); e != nil {
		return e.dev
	}
	return nil
}

// Devices returns selector summaries for every attached device.
func (m *Manager) Devices() []Summary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Summary, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, Summary{
			Key:        e.key,
			Descriptor: e.dev.Descriptor(),
			Source:     e.source,
			Connected:  e.dev.Info().Connected,
			Active:     e.key == m.activeKey,
		})
	}
	return out
}

// Select makes key the active device.
func (m *Manager) Select(key string) error {
	m.mu.Lock()
	e := m.findLocked(key)
	if e == nil {
		m.mu.Unlock()
		return fmt.Errorf("device %q is not attached", key)
	}
	m.activeKey = key
	dev := e.dev
	m.mu.Unlock()

	m.logInfo("MANAGER", "Selected %s", dev.Descriptor().DisplayName)
	m.emit(DeviceEvent{Kind: EventSelected, Key: key, Info: dev.Info(), Telemetry: dev.Telemetry()})
	return nil
}

// SelectNext cycles the active device by delta through attached devices.
func (m *Manager) SelectNext(delta int) error {
	m.mu.RLock()
	n := len(m.entries)
	if n == 0 {
		m.mu.RUnlock()
		return fmt.Errorf("no devices attached")
	}
	idx := -1
	for i, e := range m.entries {
		if e.key == m.activeKey {
			idx = i
		}
	}
	if idx < 0 && delta < 0 {
		idx = 0
	}
	next := m.entries[((idx+delta)%n+n)%n].key
	m.mu.RUnlock()
	return m.Select(next)
}

// ActiveSimulator returns the active device's Simulator, if it implements one.
func (m *Manager) ActiveSimulator() device.Simulator {
	if sim, ok := m.Active().(device.Simulator); ok {
		return sim
	}
	return nil
}

// EnableMock attaches (if needed) and selects the first registered mock device.
func (m *Manager) EnableMock() device.Device {
	var target *device.VirtualDevice
	for _, v := range m.registry.Virtuals() {
		if v.Source == device.SourceMock {
			v := v
			target = &v
			break
		}
	}
	if target == nil {
		return nil
	}
	m.mu.Lock()
	e := m.attachVirtualLocked(*target)
	m.mu.Unlock()
	_ = m.Select(e.key)
	m.emit(DeviceEvent{Kind: EventConnected, Key: e.key, Info: e.dev.Info(), Telemetry: e.dev.Telemetry()})
	return e.dev
}

// Start polls the active device (25Hz) and scans for hot-plugged hardware
// (every 1.5s) until ctx is cancelled.
func (m *Manager) Start(ctx context.Context) {
	pollTicker := time.NewTicker(40 * time.Millisecond)
	hotplugTicker := time.NewTicker(1500 * time.Millisecond)
	defer pollTicker.Stop()
	defer hotplugTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			for _, e := range m.entries {
				_ = e.dev.Close()
			}
			m.mu.Unlock()
			return

		case <-pollTicker.C:
			m.pollActive(ctx)

		case <-hotplugTicker.C:
			if !m.opts.ForceMock {
				m.checkHotplug()
			}

		case <-m.rescan:
			if !m.opts.ForceMock {
				m.checkHotplug()
			}
		}
	}
}

func (m *Manager) pollActive(ctx context.Context) {
	key := m.ActiveKey()
	dev := m.Device(key)
	if dev == nil {
		return
	}
	tel, err := dev.Poll(ctx)
	if err != nil {
		m.logWarn("POLL", "Controller disconnected or read failed: %v", err)
		m.detach(key, err)
		return
	}
	m.emit(DeviceEvent{Kind: EventTelemetry, Key: key, Info: dev.Info(), Telemetry: tel})
}

// detach closes and removes a hardware entry, clearing the selection if it was active.
func (m *Manager) detach(key string, cause error) {
	m.mu.Lock()
	var removed *entry
	kept := m.entries[:0]
	for _, e := range m.entries {
		if e.key == key && e.source == device.SourceHardware {
			removed = e
			continue
		}
		kept = append(kept, e)
	}
	m.entries = kept
	wasActive := m.activeKey == key
	if removed != nil && wasActive {
		m.activeKey = ""
	}
	m.mu.Unlock()

	if removed == nil {
		return
	}
	info := removed.dev.Info()
	info.Connected = false
	_ = removed.dev.Close()
	m.logWarn("HOTPLUG", "%s removed", removed.dev.Descriptor().DisplayName)
	if wasActive {
		m.emit(DeviceEvent{Kind: EventDisconnected, Key: key, Info: info, Err: cause})
	} else {
		m.emit(DeviceEvent{Kind: EventListChanged, Key: key})
	}
}

func (m *Manager) checkHotplug() {
	for _, drv := range m.registry.Drivers() {
		cands, err := drv.Discover()
		if err != nil {
			continue
		}
		present := map[string]bool{}
		for _, c := range cands {
			present[c.Key] = true
		}

		// Removals first: a device whose key changed (re-enumerated after an
		// input-mode switch, or re-identified as another model) is detached
		// before its replacement attaches, so the replacement can take over
		// the active selection.
		m.mu.RLock()
		var gone []string
		for _, e := range m.entries {
			if e.driver == drv && !present[e.key] {
				gone = append(gone, e.key)
			}
		}
		m.mu.RUnlock()
		for _, k := range gone {
			m.detach(k, nil)
		}

		for _, c := range cands {
			if m.Device(c.Key) != nil {
				continue
			}
			dev, err := drv.Open(c, m.logger)
			if err != nil {
				continue
			}
			m.mu.Lock()
			m.entries = append([]*entry{{key: c.Key, source: device.SourceHardware, driver: drv, dev: dev}}, m.entries...)
			// Real hardware takes over from "nothing" or from a fallback mock.
			takeOver := m.activeKey == "" || (m.autoMock && m.isMockLocked(m.activeKey))
			if takeOver {
				m.activeKey = c.Key
			}
			m.mu.Unlock()

			m.logInfo("HOTPLUG", "Attached %s", c.Label)
			if takeOver {
				m.emit(DeviceEvent{Kind: EventConnected, Key: c.Key, Info: dev.Info(), Telemetry: dev.Telemetry()})
			} else {
				m.emit(DeviceEvent{Kind: EventListChanged, Key: c.Key})
			}
		}
	}
}

// Rescan asks the polling goroutine for an immediate hot-plug scan (e.g.
// after the user confirmed a device's model, which changes its registry key).
func (m *Manager) Rescan() {
	select {
	case m.rescan <- struct{}{}:
	default:
	}
}

func (m *Manager) isMockLocked(key string) bool {
	e := m.findLocked(key)
	return e != nil && e.source == device.SourceMock
}
