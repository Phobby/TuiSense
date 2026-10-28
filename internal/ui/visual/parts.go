package visual

import "github.com/Phobby/TuiSense/internal/device"

// Reusable component builders. They only produce data; every model can
// override any field afterwards, and nothing here assumes a brand.

func at(x, y, z float64) [3]float64 { return [3]float64{x, y, z} }
func sz(x, y, z float64) [3]float64 { return [3]float64{x, y, z} }

// stick is an analog thumbstick with its click button.
func stick(side Side, label string, x, y, capR float64, region device.RegionID) Component {
	c := Component{Kind: KindLeftStick, Label: label, Side: side, Surface: Front, Material: "stick",
		Button: device.ButtonL3, Bound: true, Region: region, Anim: AnimLeftStick, Verify: Verified,
		Shapes: []Geometry{{Shape: ShapeStick, At: at(x, y, 0), Size: sz(capR, 0, 13)}}}
	c.ID = "left_stick"
	if side == Right {
		c.ID, c.Kind, c.Button, c.Anim = "right_stick", KindRightStick, device.ButtonR3, AnimRightStick
	}
	return c
}

// dpad returns four direction arms (each selectable and individually lit)
// around a centre, plus a decorative hub.
func dpad(cx, cy, span, width float64, material string) []Component {
	arm := (span - width) / 2
	off := width/2 + arm/2
	mk := func(id, label string, btn device.ButtonID, x, y, w, h float64) Component {
		return Component{ID: id, Kind: KindDPad, Label: label, Side: Left, Surface: Front, Material: material,
			Button: btn, Bound: true, Verify: Verified,
			Shapes: []Geometry{{Shape: ShapeBox, At: at(x, y, 0), Size: sz(w, h, 3)}}}
	}
	return []Component{
		mk("dpad_up", "D-pad up", device.ButtonDpadUp, cx, cy+off, width, arm),
		mk("dpad_down", "D-pad down", device.ButtonDpadDown, cx, cy-off, width, arm),
		mk("dpad_left", "D-pad left", device.ButtonDpadLeft, cx-off, cy, arm, width),
		mk("dpad_right", "D-pad right", device.ButtonDpadRight, cx+off, cy, arm, width),
		{ID: "dpad_hub", Kind: KindDecor, Surface: Front, Material: material,
			Shapes: []Geometry{{Shape: ShapeBox, At: at(cx, cy, 0), Size: sz(width, width, 3)}}},
	}
}

// faceButtons places four round buttons (north, east, south, west).
func faceButtons(cx, cy, spacing, r float64, ids, labels, materials [4]string) []Component {
	btns := [4]device.ButtonID{device.ButtonNorth, device.ButtonEast, device.ButtonSouth, device.ButtonWest}
	pos := [4][2]float64{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	out := make([]Component, 4)
	for i := range out {
		out[i] = Component{ID: ids[i], Kind: KindFaceButton, Label: labels[i], Side: Right, Surface: Front,
			Material: materials[i], Button: btns[i], Bound: true, Verify: Verified,
			Shapes: []Geometry{{Shape: ShapeDome, At: at(cx+pos[i][0]*spacing, cy+pos[i][1]*spacing, 0), Size: sz(r, 0, 4)}}}
	}
	return out
}

// xboxFaceButtons is the Y/B/A/X diamond used by Xbox-layout pads.
func xboxFaceButtons(cx, cy, spacing, r float64, materials [4]string) []Component {
	return faceButtons(cx, cy, spacing, r, [4]string{"btn_y", "btn_b", "btn_a", "btn_x"}, [4]string{"Y", "B", "A", "X"}, materials)
}

// nintendoFaceButtons is the X/A/B/Y diamond of Nintendo-layout pads: the
// south button is B and the east button is A.
func nintendoFaceButtons(cx, cy, spacing, r float64, materials [4]string) []Component {
	return faceButtons(cx, cy, spacing, r, [4]string{"btn_x", "btn_a", "btn_b", "btn_y"}, [4]string{"X", "A", "B", "Y"}, materials)
}

// shoulders returns bumpers and triggers for both sides. x is the distance
// from the centre; labels are (left bumper, right bumper, left trigger, right trigger).
func shoulders(x, bumperW, triggerW float64, labels [4]string, triggerRegion bool) []Component {
	var lr, rr device.RegionID
	if triggerRegion {
		lr, rr = device.RegionLeftTrigger, device.RegionRightTrigger
	}
	bumper := func(id, label string, kind ComponentKind, side Side, sx float64, btn device.ButtonID) Component {
		return Component{ID: id, Kind: kind, Label: label, Side: side, Surface: Top, Material: "bumper",
			Button: btn, Bound: true, Verify: Verified,
			Shapes: []Geometry{{Shape: ShapeBumper, At: at(sx, 0, -3), Size: sz(bumperW, 5, 12)}}}
	}
	trigger := func(id, label string, kind ComponentKind, side Side, sx float64, btn device.ButtonID, anim Anim, region device.RegionID) Component {
		return Component{ID: id, Kind: kind, Label: label, Side: side, Surface: Top, Material: "trigger",
			Button: btn, Bound: true, Anim: anim, Region: region, Verify: Verified,
			Shapes: []Geometry{{Shape: ShapeTrigger, At: at(sx, -6, 12), Size: sz(triggerW, 15, 8), Rot: [3]float64{-58, 0, 0}}}}
	}
	return []Component{
		bumper("left_bumper", labels[0], KindLeftBumper, Left, -x, device.ButtonL1),
		bumper("right_bumper", labels[1], KindRightBumper, Right, x, device.ButtonR1),
		trigger("left_trigger", labels[2], KindLeftTrigger, Left, -x, device.ButtonL2Digital, AnimLeftTrigger, lr),
		trigger("right_trigger", labels[3], KindRightTrigger, Right, x, device.ButtonR2Digital, AnimRightTrigger, rr),
	}
}

// grips are the rear bulges of the handles; region links them to haptics.
func grips(x, y, w, h, d, z float64, label [2]string, material string, region device.RegionID) []Component {
	mk := func(id, l string, side Side, sx float64, tilt float64) Component {
		return Component{ID: id, Kind: KindGrip, Label: l, Side: side, Surface: Inside, Material: material,
			Region: region, Verify: Approximate, Note: "Grip volume is a stylised interpretation.",
			Shapes: []Geometry{{Shape: ShapeEllipsoid, At: at(sx, y, z), Size: sz(w, h, d), Rot: [3]float64{0, 0, tilt}}}}
	}
	return []Component{mk("left_grip", label[0], Left, -x, -16), mk("right_grip", label[1], Right, x, 16)}
}

// button is a small round or pill button on the front.
func button(id, label string, kind ComponentKind, side Side, btn device.ButtonID, bound bool, g Geometry, material string) Component {
	return Component{ID: id, Kind: kind, Label: label, Side: side, Surface: Front, Material: material,
		Button: btn, Bound: bound, Verify: Verified, Shapes: []Geometry{g}}
}

func disc(x, y, r, h float64) Geometry {
	return Geometry{Shape: ShapeDisc, At: at(x, y, 0), Size: sz(r, 0, h)}
}
func pill(x, y, w, h, t float64) Geometry {
	return Geometry{Shape: ShapePad, At: at(x, y, 0), Size: sz(w, h, t)}
}

// port is a connector on an edge (no input; informational).
func port(id, label string, surface Surface, x, w, h float64, kind ComponentKind, region device.RegionID, v Verification, note string) Component {
	return Component{ID: id, Kind: kind, Label: label, Side: Center, Surface: surface, Material: "port", Region: region,
		Verify: v, Note: note, Shapes: []Geometry{{Shape: ShapeBox, At: at(x, -1.2, 0), Size: sz(w, 3, h)}}}
}

// rearControl is a back button, paddle or switch.
func rearControl(id, label string, kind ComponentKind, side Side, g Geometry, material string, v Verification, note string) Component {
	return Component{ID: id, Kind: kind, Label: label, Side: side, Surface: Rear, Material: material,
		Verify: v, Note: note, Shapes: []Geometry{g}}
}

// decor is non-interactive styling geometry on the front.
func decor(id string, material string, g ...Geometry) Component {
	return Component{ID: id, Kind: KindDecor, Surface: Front, Material: material, Shapes: g}
}
