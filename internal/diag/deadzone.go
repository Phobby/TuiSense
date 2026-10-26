package diag

import "math"

// DeadzonePresets are the preview sizes offered in the UI (percent of stick
// travel). They are illustrations, not hardware standards: every game picks
// its own deadzone.
var DeadzonePresets = []float64{0, 3, 5, 8, 10, 15, 20, 25}

// DefaultPreviewDeadzone is the preview deadzone selected initially.
const DefaultPreviewDeadzone = 8.0

// DeadzonePreview shows what a game using a radial deadzone of SizePct would
// receive for one stick position. It is a pure calculation: TuiSense has no
// input-processing backend, so nothing here changes what games receive.
type DeadzonePreview struct {
	SizePct   float64    // deadzone radius in % of travel
	Raw       [2]float64 // normalized stick position [-1, 1]
	RawPct    float64    // distance from center in % of travel
	Effective [2]float64 // what a game would see after the deadzone
	Ignored   bool       // the position is inside the deadzone (game sees 0)
}

// PreviewDeadzone applies a scaled radial deadzone: positions inside the
// circle become (0, 0); positions outside are rescaled so the output still
// starts at 0 at the deadzone edge and reaches 1 at full travel.
func PreviewDeadzone(n [2]float64, sizePct float64) DeadzonePreview {
	p := DeadzonePreview{SizePct: sizePct, Raw: n}
	r := math.Hypot(n[0], n[1])
	p.RawPct = 100 * r
	dz := math.Max(0, math.Min(sizePct/100, 0.99))
	if r == 0 {
		return p
	}
	if r <= dz {
		p.Ignored = true // a real deflection a game would discard
		return p
	}
	scale := (math.Min(r, 1) - dz) / (1 - dz) / r
	p.Effective = [2]float64{n[0] * scale, n[1] * scale}
	return p
}

// FilteredBy reports whether a constant offset of offsetPct would be hidden
// by a deadzone of sizePct.
func FilteredBy(offsetPct, sizePct float64) bool { return offsetPct <= sizePct }

// PreviewAxialDeadzone filters and rescales each axis independently. This is
// a software illustration and never changes controller or game configuration.
func PreviewAxialDeadzone(n [2]float64, sizePct float64) DeadzonePreview {
	p := DeadzonePreview{SizePct: sizePct, Raw: n, RawPct: 100 * math.Hypot(n[0], n[1])}
	dz := math.Max(0, math.Min(sizePct/100, .99))
	for i, v := range n {
		if math.Abs(v) > dz {
			p.Effective[i] = math.Copysign((math.Min(1, math.Abs(v))-dz)/(1-dz), v)
		}
	}
	p.Ignored = p.RawPct > 0 && p.Effective == [2]float64{}
	return p
}
