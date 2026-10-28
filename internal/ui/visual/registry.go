package visual

import (
	"fmt"
	"sync"
)

var (
	regMu   sync.Mutex
	specs   = map[string]*ModelSpec{}
	order   []string
	builtMu sync.Mutex
	built   = map[string]*Model{}
)

// GenericID is the fallback visual for unidentified gamepads.
const GenericID = "generic"

// Register adds a spec (called from init in each model file). Contributors
// add a controller by registering a new spec; the renderer needs no change.
func Register(s ModelSpec) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := specs[s.ID]; dup {
		panic(fmt.Sprintf("visual: duplicate model id %q", s.ID))
	}
	if s.Source == "" {
		s.Source = proceduralSource
	}
	if s.Attribution == "" {
		s.Attribution = originalAttribution
	}
	if s.Front == (CameraPreset{}) {
		s.Front = CameraPreset{RotX: -18, RotY: 12}
	}
	if s.Rear == (CameraPreset{}) {
		s.Rear = CameraPreset{RotX: -16, RotY: 192}
	}
	cp := s
	specs[s.ID] = &cp
	order = append(order, s.ID)
}

// Spec returns the registered spec for id.
func Spec(id string) (*ModelSpec, bool) {
	regMu.Lock()
	defer regMu.Unlock()
	s, ok := specs[id]
	return s, ok
}

// Lookup returns the built (cached) model for id, falling back to the
// generic gamepad. Geometry is generated once per process.
func Lookup(id string) *Model {
	s, ok := Spec(id)
	if !ok {
		s, _ = Spec(GenericID)
		id = GenericID
	}
	builtMu.Lock()
	defer builtMu.Unlock()
	if m, ok := built[id]; ok {
		return m
	}
	m := build(s)
	built[id] = m
	return m
}

// IDs lists every registered visual ID in registration order.
func IDs() []string {
	regMu.Lock()
	defer regMu.Unlock()
	return append([]string(nil), order...)
}

// Gallery lists the specs shown in the Controller Gallery, in a stable order.
func Gallery() []*ModelSpec {
	regMu.Lock()
	defer regMu.Unlock()
	var out []*ModelSpec
	for _, id := range galleryOrder {
		if s, ok := specs[id]; ok && s.Gallery {
			out = append(out, s)
		}
	}
	for _, id := range order { // models registered by contributors after the built-in ten
		if s := specs[id]; s.Gallery && !contains(galleryOrder, id) {
			out = append(out, s)
		}
	}
	return out
}

var galleryOrder = []string{
	"dualsense", "dualsense_edge", "xbox_series", "xbox_elite2", "dualshock4",
	"switch_pro", "8bitdo_ultimate_bt", "gamesir_g7se", "razer_wolverine_v3pro", "msi_gc20",
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
