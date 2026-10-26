package device

import (
	"fmt"
	"sort"
	"sync"

	"github.com/Phobby/TuiSense/internal/logging"
)

// Candidate is a physical device found by a driver's discovery scan.
// Key must be stable for as long as the device stays attached (e.g. a hidraw path).
type Candidate struct {
	Key      string
	Endpoint string // changes when a stable physical identity moves to another node
	Label    string
	Payload  any // driver-private discovery data handed back to Open
}

// Driver discovers and opens real hardware for one controller family.
// Contributors add a controller by implementing Driver and registering it.
type Driver interface {
	ID() string
	// Models lists every Descriptor this driver can produce.
	Models() []Descriptor
	// Discover enumerates currently attached devices. It must be cheap enough
	// to call every ~1.5s and return (nil, nil) on unsupported platforms.
	Discover() ([]Candidate, error)
	Open(c Candidate, logger *logging.Logger) (Device, error)
}

// VirtualDevice registers a mock or test device that can be instantiated on demand.
type VirtualDevice struct {
	Key        string
	Source     Source // SourceMock or SourceTest
	Descriptor Descriptor
	// AutoAttach means the device is listed from startup (test devices); otherwise
	// it is only attached as a fallback or when explicitly enabled (mocks).
	AutoAttach bool
	New        func(logger *logging.Logger) Device
}

// Registry is the central catalog of drivers and virtual devices.
type Registry struct {
	mu       sync.RWMutex
	drivers  []Driver
	virtuals []VirtualDevice
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// RegisterDriver adds a hardware driver. Duplicate IDs are rejected.
func (r *Registry) RegisterDriver(d Driver) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.drivers {
		if existing.ID() == d.ID() {
			return fmt.Errorf("driver %q already registered", d.ID())
		}
	}
	r.drivers = append(r.drivers, d)
	return nil
}

// RegisterVirtual adds a mock/test device factory. Duplicate keys are rejected.
func (r *Registry) RegisterVirtual(v VirtualDevice) error {
	if v.Key == "" || v.New == nil {
		return fmt.Errorf("virtual device requires Key and New")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.virtuals {
		if existing.Key == v.Key {
			return fmt.Errorf("virtual device %q already registered", v.Key)
		}
	}
	r.virtuals = append(r.virtuals, v)
	return nil
}

// Drivers returns registered drivers in registration order.
func (r *Registry) Drivers() []Driver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Driver(nil), r.drivers...)
}

// Virtuals returns registered virtual devices in registration order.
func (r *Registry) Virtuals() []VirtualDevice {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]VirtualDevice(nil), r.virtuals...)
}

// Virtual looks up a virtual device by key.
func (r *Registry) Virtual(key string) (VirtualDevice, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.virtuals {
		if v.Key == key {
			return v, true
		}
	}
	return VirtualDevice{}, false
}

// Models returns every known descriptor (drivers + virtuals), de-duplicated by
// ModelID and sorted by brand then name.
func (r *Registry) Models() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	var out []Descriptor
	add := func(d Descriptor) {
		if !seen[d.ModelID] {
			seen[d.ModelID] = true
			out = append(out, d)
		}
	}
	for _, d := range r.drivers {
		for _, m := range d.Models() {
			add(m)
		}
	}
	for _, v := range r.virtuals {
		add(v.Descriptor)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Brand != out[j].Brand {
			return out[i].Brand < out[j].Brand
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out
}
