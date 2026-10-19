package drivers

import (
	"testing"

	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/state"
	"github.com/ygzztrk/tuisense/internal/ui/visual"
)

func TestBuiltinRegistryContents(t *testing.T) {
	r := NewBuiltinRegistry(RegistryOptions{Hardware: true, TestDevices: true})
	if len(r.Drivers()) != 1 || r.Drivers()[0].ID() != "dualsense" {
		t.Fatalf("expected the DualSense hardware driver")
	}
	keys := map[string]device.Source{}
	for _, v := range r.Virtuals() {
		keys[v.Key] = v.Source
	}
	if keys["mock:dualsense"] != device.SourceMock || keys["test:xbox"] != device.SourceTest {
		t.Fatalf("unexpected virtual devices: %v", keys)
	}
	if len(NewBuiltinRegistry(RegistryOptions{}).Virtuals()) != 1 {
		t.Fatalf("test devices must be optional")
	}
}

// TestDescriptorsAreConsistent guards the contract a contributor must satisfy
// when adding a controller: every region maps to a panel and a pickable anchor
// on its visual, and every declared capability is a known one.
func TestDescriptorsAreConsistent(t *testing.T) {
	r := NewBuiltinRegistry(RegistryOptions{Hardware: true, TestDevices: true})
	for _, d := range r.Models() {
		if d.ModelID == "" || d.Family == "" || d.ShortName == "" || d.VisualID == "" {
			t.Errorf("%s: incomplete identity", d.ModelID)
		}
		v := visual.Lookup(d.VisualID)
		if v == nil {
			t.Fatalf("%s: no visual registered for %q", d.ModelID, d.VisualID)
		}
		anchors := map[device.RegionID]bool{}
		for _, a := range v.Anchors() {
			anchors[a.Region] = true
		}
		for _, reg := range d.Regions {
			if _, ok := state.PanelForCapability(reg.Capability); !ok {
				t.Errorf("%s: region %s capability %s has no panel", d.ModelID, reg.ID, reg.Capability)
			}
			if !anchors[reg.ID] {
				t.Errorf("%s: region %s has no 3D anchor on visual %s", d.ModelID, reg.ID, d.VisualID)
			}
			if reg.Badge == "" || reg.Label == "" {
				t.Errorf("%s: region %s missing labels", d.ModelID, reg.ID)
			}
		}
		for c := range d.Capabilities {
			known := false
			for _, k := range device.AllCapabilities {
				known = known || k == c
			}
			if !known {
				t.Errorf("%s: unknown capability %q", d.ModelID, c)
			}
		}
	}
}
