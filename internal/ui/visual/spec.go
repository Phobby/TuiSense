// Package visual is TuiSense's 3D controller model system.
//
// A controller visual is pure data — a ModelSpec — describing the body
// silhouette and a list of semantic Components (sticks, buttons, paddles,
// lighting zones, …), each with its own geometry, verified label, input
// binding and capability region. The generator (build.go) turns a spec into a
// cached triangle mesh for Limoni's Viewer3D; the same mesh drives colouring,
// live-input animation and projection-based picking (pick.go), so adding a
// controller never touches the renderer.
//
// Visual specs are separate from hardware drivers: a model existing here says
// nothing about whether TuiSense can talk to that controller. HardwareModelIDs
// links a spec to the driver descriptors that render with it.
//
// Units are millimetres in the spec. Coordinates follow internal/ui/geom:
// +X is the player's right, +Y up, -Z the front face.
package visual

import "github.com/Phobby/TuiSense/internal/device"

// ComponentKind classifies a component for inspectors and diagnostics.
type ComponentKind string

const (
	KindLeftStick      ComponentKind = "left_stick"
	KindRightStick     ComponentKind = "right_stick"
	KindFaceButton     ComponentKind = "face_button"
	KindDPad           ComponentKind = "dpad"
	KindLeftBumper     ComponentKind = "left_bumper"
	KindRightBumper    ComponentKind = "right_bumper"
	KindLeftTrigger    ComponentKind = "left_trigger"
	KindRightTrigger   ComponentKind = "right_trigger"
	KindRearButton     ComponentKind = "rear_button"
	KindRearPaddle     ComponentKind = "rear_paddle"
	KindExtraBumper    ComponentKind = "extra_bumper"
	KindTouchpad       ComponentKind = "touchpad"
	KindSystemButton   ComponentKind = "system_button"
	KindProfileButton  ComponentKind = "profile_button"
	KindLightingZone   ComponentKind = "lighting_zone"
	KindMotionSensor   ComponentKind = "motion_sensor"
	KindAudioComponent ComponentKind = "audio"
	KindGrip           ComponentKind = "grip"
	KindHardwareSwitch ComponentKind = "hardware_switch"
	KindPort           ComponentKind = "port"
	KindIndicator      ComponentKind = "indicator"
	KindDecor          ComponentKind = "" // non-interactive styling geometry
)

// kindNames maps kinds to display names; RegisterKind adds model-specific ones.
var kindNames = map[ComponentKind]string{
	KindLeftStick: "Analog stick", KindRightStick: "Analog stick", KindFaceButton: "Face button",
	KindDPad: "D-pad", KindLeftBumper: "Bumper", KindRightBumper: "Bumper",
	KindLeftTrigger: "Trigger", KindRightTrigger: "Trigger", KindRearButton: "Rear button",
	KindRearPaddle: "Rear paddle", KindExtraBumper: "Extra bumper", KindTouchpad: "Touchpad",
	KindSystemButton: "System button", KindProfileButton: "Profile button",
	KindLightingZone: "Lighting zone", KindMotionSensor: "Motion sensor", KindAudioComponent: "Audio",
	KindGrip: "Grip", KindHardwareSwitch: "Hardware switch", KindPort: "Port", KindIndicator: "Indicator",
}

// RegisterKind declares a model-specific component kind (e.g. "nfc_reader").
func RegisterKind(k ComponentKind, displayName string) { kindNames[k] = displayName }

// KindName returns the English display name of a kind.
func KindName(k ComponentKind) string {
	if n, ok := kindNames[k]; ok {
		return n
	}
	return string(k)
}

// Rear reports whether the kind lives on the back of a controller.
func (k ComponentKind) Rear() bool { return k == KindRearButton || k == KindRearPaddle }

// Surface is where a component's geometry is mounted.
type Surface string

const (
	Front  Surface = "front"
	Rear   Surface = "rear"
	Top    Surface = "top"
	Bottom Surface = "bottom"
	Inside Surface = "inside" // placed at the given absolute coordinates
)

// Side is the player's left/right.
type Side string

const (
	Left   Side = "left"
	Right  Side = "right"
	Center Side = "center"
)

// Shape selects the primitive a Geometry generates.
type Shape int

const (
	ShapeDisc      Shape = iota // flat cylinder: Size = (radiusX, radiusY, height); radiusY 0 = round
	ShapeDome                   // tapered button: Size = (radius, _, height)
	ShapeBox                    // Size = (w, h, d)
	ShapePad                    // rounded slab: Size = (w, h, thickness)
	ShapeStick                  // well + shaft + cap: Size = (cap radius, _, height)
	ShapeBumper                 // box hugging the top edge: Size = (w, h, d)
	ShapeTrigger                // angled box behind the top edge, pivots on pull
	ShapeEllipsoid              // Size = full extents (w, h, d)
	ShapePaddle                 // thin plate on the back: Size = (w, h, thickness)
	ShapeSlider                 // switch track with a knob: Size = (w, h, height)
	ShapeCross                  // plus shape: Size = (span, arm width, height)
)

// Geometry is one primitive of a component, in millimetres.
//
// For Front and Rear surfaces At.X/At.Y are the position on the face and At.Z
// lifts the primitive outward from the surface; for Top and Bottom At.X is
// the position along the edge, At.Y an offset from the edge and At.Z the
// depth. Inside uses At verbatim. Rot is a local rotation in degrees.
type Geometry struct {
	Shape    Shape
	At       [3]float64
	Size     [3]float64
	Rot      [3]float64
	Segments int
	Material string  // overrides the component material
	Surface  Surface // overrides the component surface
}

// Anim is how live telemetry moves a component.
type Anim int

const (
	AnimNone Anim = iota
	AnimLeftStick
	AnimRightStick
	AnimLeftTrigger
	AnimRightTrigger
)

// Verification records how well a detail is backed by references.
type Verification string

const (
	Verified    Verification = "verified"    // manufacturer documentation
	Approximate Verification = "approximate" // exists, but position/shape is an interpretation
	Unverified  Verification = "unverified"  // could not be confirmed; shown as such
)

// Component is one semantic, selectable part of a controller.
type Component struct {
	ID       string
	Kind     ComponentKind
	Label    string // manufacturer label where verified, otherwise a neutral description
	Side     Side
	Surface  Surface
	Material string
	Shapes   []Geometry

	// Button is the input event this component produces, when Bound. It is a
	// *physical* binding; whether a driver reports it is decided at runtime by
	// the active descriptor's InputLayout.
	Button device.ButtonID
	Bound  bool
	// Region links the component to the capability region (and panel) of a
	// connected device's descriptor.
	Region device.RegionID
	Anim   Anim

	Verify Verification
	Note   string // source or caveat, shown in Advanced mode and the gallery
}

// Pickable reports whether the component is interactive.
func (c Component) Pickable() bool { return c.Kind != KindDecor }

// CameraPreset is an orbit orientation in degrees.
type CameraPreset struct{ RotX, RotY float64 }

// Body is the controller shell.
type Body struct {
	// Half is the right half of the front silhouette, from the top centre
	// clockwise to the bottom centre (x ≥ 0).
	Half   [][2]float64
	Smooth int     // spline points per profile edge (1 keeps hard facets)
	Depth  float64 // slab thickness
	Bevel  float64
	// Material of the shell; ComponentID optionally makes the shell itself a
	// component (e.g. the internal motion sensor).
	Material    string
	ComponentID string
	// Spine is the rear shoulder block that carries the triggers.
	SpineWidth, SpineHeight, SpineDepth float64
}

// RGB is a 24-bit material colour.
type RGB struct{ R, G, B uint8 }

// ModelSpec is the complete, data-only description of a controller visual.
type ModelSpec struct {
	ID           string // stable visual ID referenced by device.Descriptor.VisualID
	Manufacturer string
	Name         string
	Revision     string // model number / generation
	// Gallery lists the spec in the Controller Gallery.
	Gallery bool
	// Dimensions are nominal W×H×D in mm (see DimensionsNote).
	Dimensions     [3]float64
	DimensionsNote string
	Body           Body
	Components     []Component
	Materials      map[string]RGB
	Front, Rear    CameraPreset
	// HardwareModelIDs are driver descriptor ModelIDs rendered with this
	// visual; empty means TuiSense has no driver for the controller.
	HardwareModelIDs []string
	// Source and Attribution document where the geometry comes from.
	Source      string
	Attribution string
	References  []string
	// Caveats lists details that are approximate or unverified.
	Caveats []string
}

// Component returns the component with id.
func (s *ModelSpec) Component(id string) (Component, bool) {
	for _, c := range s.Components {
		if c.ID == id {
			return c, true
		}
	}
	return Component{}, false
}

// RearComponents lists the rear buttons and paddles.
func (s *ModelSpec) RearComponents() []Component {
	var out []Component
	for _, c := range s.Components {
		if c.Kind.Rear() {
			out = append(out, c)
		}
	}
	return out
}

const proceduralSource = "procedural"

const originalAttribution = "Original low-poly procedural interpretation by TuiSense contributors (MIT). " +
	"Not an official manufacturer 3D asset; proportions follow public product references."
