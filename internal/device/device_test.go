package device

import (
	"context"
	"testing"

	"github.com/Phobby/TuiSense/internal/logging"
)

func TestCapabilitySetLevelsAndFeatures(t *testing.T) {
	caps := NewCapabilitySet(
		CapabilitySpec{ID: CapLighting, Support: Full, Access: AccessConfigDevice, Features: []Feature{FeatLightbarRGB, FeatPlayerLEDs}},
		CapabilitySpec{ID: CapAudio, Support: Limited, Access: AccessTest, Features: []Feature{FeatHeadset}},
	)
	if caps.Level(CapMotion) != Unsupported || caps.Supports(CapMotion) {
		t.Fatalf("absent capability must be Unsupported")
	}
	if !caps.HasFeature(CapLighting, FeatLightbarRGB) || caps.HasFeature(CapLighting, FeatMicLED) {
		t.Fatalf("feature lookup mismatch")
	}
	if caps.Level(CapAudio) != Limited {
		t.Fatalf("expected Limited audio")
	}

	narrowed := caps.WithoutFeature(CapLighting, FeatPlayerLEDs, "no player LEDs")
	if narrowed.HasFeature(CapLighting, FeatPlayerLEDs) || narrowed.Level(CapLighting) != Limited {
		t.Fatalf("WithoutFeature should drop the feature and downgrade to Limited, got %+v", narrowed[CapLighting])
	}
	if !caps.HasFeature(CapLighting, FeatPlayerLEDs) {
		t.Fatalf("WithoutFeature must not mutate the original set")
	}
}

func TestAvailabilityCombinesStateAndSupport(t *testing.T) {
	caps := NewCapabilitySet(CapabilitySpec{ID: CapHaptics, Support: Full, Access: AccessTest})
	connected := Info{Connected: true}

	if lvl, _ := Availability(connected, caps, CapHaptics); lvl != Full {
		t.Fatalf("expected Full, got %v", lvl)
	}
	if lvl, reason := Availability(Info{Connected: false}, caps, CapHaptics); lvl != Unsupported || reason == "" {
		t.Fatalf("disconnected device must be unavailable with a reason, got %v %q", lvl, reason)
	}
	if lvl, _ := Availability(Info{Connected: true, PermissionDenied: true}, caps, CapHaptics); lvl != Limited {
		t.Fatalf("permission denied should limit, got %v", lvl)
	}
	if lvl, _ := Availability(connected, caps, CapMotion); lvl != Unsupported {
		t.Fatalf("undeclared capability must be unsupported")
	}
}

type stubDriver struct{ id string }

func (d stubDriver) ID() string { return d.id }
func (d stubDriver) Models() []Descriptor {
	return []Descriptor{{ModelID: d.id + ".model", Brand: "B"}}
}
func (d stubDriver) Discover() ([]Candidate, error)                  { return nil, nil }
func (d stubDriver) Open(Candidate, *logging.Logger) (Device, error) { return nil, context.Canceled }

func TestRegistryRejectsDuplicatesAndListsModels(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterDriver(stubDriver{id: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterDriver(stubDriver{id: "a"}); err == nil {
		t.Fatalf("duplicate driver ID must be rejected")
	}
	v := VirtualDevice{Key: "test:x", Source: SourceTest, Descriptor: Descriptor{ModelID: "x.model", Brand: "A"}, New: func(*logging.Logger) Device { return nil }}
	if err := r.RegisterVirtual(v); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterVirtual(v); err == nil {
		t.Fatalf("duplicate virtual key must be rejected")
	}
	if err := r.RegisterVirtual(VirtualDevice{Key: "broken"}); err == nil {
		t.Fatalf("virtual device without factory must be rejected")
	}
	models := r.Models()
	if len(models) != 2 || models[0].ModelID != "x.model" {
		t.Fatalf("expected 2 models sorted by brand, got %+v", models)
	}
	if _, ok := r.Virtual("test:x"); !ok {
		t.Fatalf("virtual lookup failed")
	}
}

func TestDescriptorRegionsAndPlaceholder(t *testing.T) {
	d := Descriptor{Regions: []Region{{ID: RegionLeftStick}, {ID: RegionHaptics}}}
	if _, ok := d.Region(RegionHaptics); !ok {
		t.Fatalf("expected haptics region")
	}
	if _, ok := d.Region(RegionMotion); ok {
		t.Fatalf("undeclared region must not resolve")
	}
	if got := d.RegionIDs(); len(got) != 2 || got[0] != RegionLeftStick {
		t.Fatalf("unexpected region order %v", got)
	}
	ph := PlaceholderDescriptor()
	if len(ph.Capabilities.Supported()) != 0 || ph.VisualID == "" {
		t.Fatalf("placeholder must have a visual and no capabilities")
	}
}

func TestAxisNormalization(t *testing.T) {
	ds := AxisRange{Min: 0, Max: 255, Center: 128}
	for raw, want := range map[float64]float64{0: -1, 128: 0, 255: 1, 64: -0.5} {
		if got := ds.NormalizeBipolar(raw); got != want {
			t.Errorf("8-bit %v → %v, want %v", raw, got, want)
		}
	}
	wide := AxisRange{Min: -32768, Max: 32767, Center: -0.5}
	if wide.NormalizeBipolar(-32768) != -1 || wide.NormalizeBipolar(32767) != 1 || wide.NormalizeBipolar(-0.5) != 0 {
		t.Error("16-bit range must reach ±1 at both ends")
	}
	if v := wide.NormalizeBipolar(40000); v != 1 {
		t.Errorf("out-of-range values clamp, got %v", v)
	}
	inv := AxisRange{Min: 0, Max: 1023, Center: 511.5, Invert: true}
	if inv.NormalizeBipolar(1023) != -1 || inv.NormalizeUnipolar(1023) != 0 || inv.NormalizeUnipolar(0) != 1 {
		t.Error("inverted axes flip direction")
	}
	trig := AxisRange{Min: 0, Max: 1023}
	if trig.NormalizeUnipolar(1023) != 1 || trig.NormalizeUnipolar(0) != 0 || trig.NormalizeUnipolar(511.5) != 0.5 {
		t.Error("unipolar trigger normalization wrong")
	}
	if (AxisRange{}).NormalizeBipolar(5) != 0 {
		t.Error("an invalid range must not produce values")
	}
	l := InputLayout{StickRange: ds, StickAxes: [4]AxisRange{3: wide}}
	if l.StickAxis(0) != ds || l.StickAxis(3) != wide {
		t.Error("per-axis ranges override the shared range")
	}
}

// Physical presence and TuiSense's ability to use a feature are separate facts.
func TestPresenceAndAccessAreSeparate(t *testing.T) {
	caps := NewCapabilitySet(
		AbsentSpec(CapBattery),
		InaccessibleSpec(CapMotion, RequiresBackend, "separate node"),
		CapabilitySpec{ID: CapSticks, Support: Full, Access: AccessRead | AccessTest | AccessPreview},
		CapabilitySpec{ID: CapLighting, Support: Full}, // declared without any access
	)
	for c, want := range map[Capability]SupportState{
		CapBattery: StateHardwareAbsent, CapMotion: StateDetected, CapSticks: StatePreviewOnly, CapLighting: StateReadOnly,
	} {
		if got := caps.Spec(c).State(); got != want && !(c == CapLighting && got == StateDetected) {
			t.Errorf("%s: state %s, want %s", c, got, want)
		}
	}
	for _, c := range []Capability{CapBattery, CapMotion, CapLighting} {
		if caps.Supports(c) {
			t.Errorf("%s must not be usable", c)
		}
	}
	if caps.Spec(CapMotion).Physical() != Present || caps.Spec(CapBattery).Physical() != Absent {
		t.Fatal("presence lost")
	}
	if !caps.HasAccess(CapSticks, AccessPreview) || caps.HasAccess(CapSticks, AccessConfigSoftware) {
		t.Fatal("a preview must never count as a software configuration")
	}
}

func TestForConnectionNarrowsTransportBoundCapabilities(t *testing.T) {
	caps := NewCapabilitySet(CapabilitySpec{ID: CapAudio, Support: Full, Access: AccessTest, Transports: []ConnectionType{ConnUSB}})
	if !caps.ForConnection(ConnUSB).Supports(CapAudio) || !caps.ForConnection(ConnUSBWired).Supports(CapAudio) {
		t.Fatal("audio must stay available over USB")
	}
	bt := caps.ForConnection(ConnBluetooth).Spec(CapAudio)
	if bt.Accessible() || bt.Requires != RequiresConnection || bt.Physical() != Present {
		t.Fatalf("over Bluetooth audio is present but needs another connection: %+v", bt)
	}
	if !caps.Supports(CapAudio) {
		t.Fatal("ForConnection must not mutate the original set")
	}
}
