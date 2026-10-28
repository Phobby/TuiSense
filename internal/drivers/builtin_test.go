package drivers

import (
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/xinput"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/ui/visual"
)

func TestBuiltinRegistryContents(t *testing.T) {
	r := NewBuiltinRegistry(RegistryOptions{Hardware: true, TestDevices: true})
	want := []string{"dualsense", "evdev_gamepad"}
	if runtime.GOOS == "windows" {
		want = append(want, "xinput")
	}
	var got []string
	for _, d := range r.Drivers() {
		got = append(got, d.ID())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("hardware drivers %v, want %v", got, want)
	}
	keys := map[string]device.Source{}
	for _, v := range r.Virtuals() {
		keys[v.Key] = v.Source
	}
	if keys["mock:dualsense"] != device.SourceMock || keys["test:xbox"] != device.SourceTest {
		t.Fatalf("unexpected virtual devices: %v", keys)
	}
	if len(NewBuiltinRegistry(RegistryOptions{}).Virtuals()) != 0 {
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
		spec, ok := visual.Spec(d.VisualID)
		if !ok {
			t.Fatalf("%s: no visual registered for %q", d.ModelID, d.VisualID)
		}
		anchors := map[device.RegionID]bool{}
		for _, c := range spec.Components {
			if c.Pickable() {
				anchors[c.Region] = true
			}
		}
		for _, reg := range d.Regions {
			if _, ok := state.PanelForCapability(reg.Capability); !ok {
				t.Errorf("%s: region %s capability %s has no panel", d.ModelID, reg.ID, reg.Capability)
			}
			if !anchors[reg.ID] {
				t.Errorf("%s: region %s has no selectable component on visual %s", d.ModelID, reg.ID, d.VisualID)
			}
			if reg.Badge == "" || reg.Label == "" {
				t.Errorf("%s: region %s missing labels", d.ModelID, reg.ID)
			}
		}
		// Every button the descriptor declares must exist as a bound component,
		// so live input can light it up.
		for _, b := range d.Inputs.Buttons {
			found := false
			for _, c := range spec.Components {
				found = found || (c.Bound && c.Button == b.ID)
			}
			if !found {
				t.Errorf("%s: button %q has no component on visual %s", d.ModelID, b.Label, d.VisualID)
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

var lowerWord = regexp.MustCompile(`[a-z]{2,}`) // codes like "L1", "X", "LB" need no translation

// TestDescriptorTextIsTranslated keeps controller descriptors in sync with the
// Turkish catalog: every user-visible descriptor string needs a translation.
func TestDescriptorTextIsTranslated(t *testing.T) {
	r := NewBuiltinRegistry(RegistryOptions{Hardware: true, TestDevices: true})
	// The XInput driver is only registered on Windows; check its text everywhere.
	for _, d := range append(r.Models(), xinput.Descriptor(true)) {
		texts := []string{d.DisplayName, d.ShortName}
		for _, reg := range d.Regions {
			texts = append(texts, reg.Label, reg.Badge, reg.Description)
		}
		for _, b := range d.Inputs.Buttons {
			texts = append(texts, b.Label)
		}
		texts = append(texts, d.Inputs.StickLabels[0], d.Inputs.StickLabels[1])
		for _, a := range d.Actions {
			texts = append(texts, a.Label)
		}
		for _, c := range d.Capabilities {
			texts = append(texts, c.Note)
		}
		for _, s := range texts {
			if lowerWord.MatchString(s) && !i18n.Has(i18n.Turkish, s) {
				t.Errorf("%s: no Turkish translation for %q", d.ModelID, s)
			}
		}
	}
}

// TestMSIIsRealHardwareAndIsolatedFromXboxMock checks that the MSI Force GC20
// V2 is registered through a hardware driver, never as a virtual device, and
// that the mock Xbox stays a separate simulated test device.
func TestMSIIsRealHardwareAndIsolatedFromXboxMock(t *testing.T) {
	r := NewBuiltinRegistry(RegistryOptions{Hardware: true, TestDevices: true})
	var msiDriver string
	for _, d := range r.Drivers() {
		for _, m := range d.Models() {
			if m.ModelID == "msi.force_gc20_v2" {
				msiDriver = d.ID()
			}
		}
	}
	if msiDriver != "evdev_gamepad" {
		t.Fatalf("MSI GC20 V2 must come from the evdev hardware driver, got %q", msiDriver)
	}
	for _, v := range r.Virtuals() {
		if v.Descriptor.ModelID == "msi.force_gc20_v2" || v.Descriptor.Family == "msi_gc20" {
			t.Fatal("MSI must not be registered as a mock/test device")
		}
		if v.Key == "test:xbox" && (v.Source != device.SourceTest || v.Descriptor.Brand != "Microsoft") {
			t.Fatal("the Xbox test device must stay a separate simulated device")
		}
	}
	if len(NewBuiltinRegistry(RegistryOptions{TestDevices: true}).Drivers()) != 0 {
		t.Fatal("hardware drivers must be optional (tests run without touching devices)")
	}
}

func TestExplicitDualSenseTestDeviceIsOptionalAndSimulated(t *testing.T) {
	registry := NewBuiltinRegistry(RegistryOptions{TestDevices: true})
	virtual, ok := registry.Virtual("test:dualsense")
	if !ok || virtual.Source != device.SourceTest || !virtual.AutoAttach {
		t.Fatal("DualSense test device must be explicitly registered and auto-attached")
	}
	controller := virtual.New(nil)
	defer controller.Close()
	if controller.Descriptor().ShortName != "DualSense (Test)" || controller.Descriptor().VisualID != "dualsense" {
		t.Fatal("test device must use original DualSense visual and a clear test label")
	}
	if _, ok := controller.(device.ScenarioSimulator); !ok {
		t.Fatal("test device must support simulated diagnostic input")
	}
	if _, ok := NewBuiltinRegistry(RegistryOptions{}).Virtual("test:dualsense"); ok {
		t.Fatal("test device must respect disabled test devices")
	}
}

// TestVisualHardwareLinks keeps the gallery honest: a visual may only claim
// hardware support for real driver models that actually render with it, and
// the two production hardware visuals must keep their links.
func TestVisualHardwareLinks(t *testing.T) {
	hw := map[string]device.Descriptor{}
	for _, drv := range NewBuiltinRegistry(RegistryOptions{Hardware: true}).Drivers() {
		for _, d := range drv.Models() {
			hw[d.ModelID] = d
		}
	}
	claimed := map[string]string{}
	for _, spec := range visual.Gallery() {
		for _, id := range spec.HardwareModelIDs {
			d, ok := hw[id]
			if !ok {
				t.Errorf("visual %s claims unknown hardware model %s", spec.ID, id)
				continue
			}
			if d.VisualID != spec.ID {
				t.Errorf("visual %s claims %s, which renders with %s", spec.ID, id, d.VisualID)
			}
			claimed[id] = spec.ID
		}
	}
	for id, want := range map[string]string{"sony.dualsense": "dualsense", "sony.dualsense_edge": "dualsense_edge", "msi.force_gc20_v2": "msi_gc20"} {
		if claimed[id] != want {
			t.Errorf("hardware model %s must be linked to visual %s, got %q", id, want, claimed[id])
		}
	}
}
