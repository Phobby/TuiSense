package visual

import (
	"math"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/ui/geom"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/graphics"
)

type cellColor = cell.Color

// Model is a built, cached controller mesh. Geometry is generated once per
// spec; Frame only re-poses the few animated parts and recolours faces into
// buffers it reuses, so rendering allocates nothing per frame.
//
// A Model is owned by the UI goroutine.
type Model struct {
	Spec       *ModelSpec
	rest       []geom.Vec3 // posed at rest
	verts      []geom.Vec3 // current pose (shared with the returned Model3D)
	faces      [][]int
	faceParts  []int
	parts      []part
	centroids  []geom.Vec3 // per component, at rest
	normals    []geom.Vec3 // outward direction of each component's mounting surface
	half       geom.Vec3   // bounding-box half extents
	compIndex  map[string]int
	colors     []cell.Color
	scratch    []projected // Pick's projection buffer
	partColors []cell.Color
	name       string
}

// FrameContext is the per-frame state a model is posed and coloured with.
type FrameContext struct {
	Telemetry device.Telemetry
	Live      bool // pose sticks/triggers from Telemetry
	// Pressed lights up (and slightly depresses) bound buttons; Reported
	// filters to buttons the active driver actually reports (nil = none).
	Pressed  device.ButtonSet
	Reported func(device.ButtonID) bool
	Selected int // component index, -1 for none
	Hovered  int
	// Region additionally highlights every component linked to a selected
	// capability region (e.g. chosen from a settings panel).
	Region   device.RegionID
	Accent   cell.Color
	Hover    cell.Color
	Pressing cell.Color
	Dim      bool // disconnected: fade toward DimTo
	DimTo    cell.Color
	// Light, when set, is the live colour of "light" materials (lightbar, LEDs).
	Light *RGB
	// MicLED lights the "mic_led" material.
	MicLED bool
}

// Faces returns the face count (for budgets and tests).
func (m *Model) Faces() int { return len(m.faces) }

// Vertices returns the vertex count.
func (m *Model) Vertices() int { return len(m.rest) }

// Index returns the component index of id (-1 when absent).
func (m *Model) Index(id string) int {
	if i, ok := m.compIndex[id]; ok {
		return i
	}
	return -1
}

// ComponentAt returns the component at index i.
func (m *Model) ComponentAt(i int) (Component, bool) {
	if i < 0 || i >= len(m.Spec.Components) {
		return Component{}, false
	}
	return m.Spec.Components[i], true
}

// FacesOf counts the faces generated for component i.
func (m *Model) FacesOf(i int) int {
	n := 0
	for _, p := range m.faceParts {
		if m.parts[p].comp == i {
			n++
		}
	}
	return n
}

// Centroid returns component i's centre at rest (renderer units).
func (m *Model) Centroid(i int) geom.Vec3 { return m.centroids[i] }

// HalfExtents is the bounding-box half size (renderer units).
func (m *Model) HalfExtents() geom.Vec3 { return m.half }

// Frame poses and colours the model. The returned Model3D shares the model's
// vertex and face slices; colours are valid until the next call.
func (m *Model) Frame(ctx FrameContext) (graphics.Model3D, []cell.Color) {
	tel := ctx.Telemetry
	for i := range m.parts {
		p := &m.parts[i]
		if !p.dynamic {
			continue
		}
		var off geom.Vec3
		if p.comp >= 0 && m.pressed(ctx, p.comp) {
			off = p.press
		}
		xf := geom.Identity()
		if ctx.Live {
			switch p.anim {
			case AnimLeftStick:
				xf = tilt(p.pivot, tel.LeftStickX, tel.LeftStickY)
			case AnimRightStick:
				xf = tilt(p.pivot, tel.RightStickX, tel.RightStickY)
			case AnimLeftTrigger:
				xf = pull(p.pivot, tel.L2Pressure)
			case AnimRightTrigger:
				xf = pull(p.pivot, tel.R2Pressure)
			}
		}
		for v := p.first; v < p.end; v++ {
			r := xf.Apply(m.rest[v])
			m.verts[v] = geom.Vec3{X: r.X + off.X, Y: r.Y + off.Y, Z: r.Z + off.Z}
		}
	}
	m.colorize(ctx)
	return graphics.Model3D{Name: m.name, Vertices: m.verts, Faces: m.faces}, m.colors
}

func (m *Model) pressed(ctx FrameContext, comp int) bool {
	c := m.Spec.Components[comp]
	return c.Bound && ctx.Reported != nil && ctx.Reported(c.Button) && ctx.Pressed.Has(c.Button)
}

// tilt rotates about pivot following a stick (negative Y = up).
func tilt(pivot geom.Vec3, x, y float64) geom.Xform {
	x, y = clamp1(x), clamp1(y)
	return geom.At(-pivot.X, -pivot.Y, -pivot.Z).Then(geom.Identity().Rotate(-y*20, -x*20, 0)).Then(geom.At(pivot.X, pivot.Y, pivot.Z))
}

// pull swings a trigger back about its top-front edge.
func pull(pivot geom.Vec3, v float64) geom.Xform {
	v = math.Max(0, math.Min(1, v))
	return geom.At(-pivot.X, -pivot.Y, -pivot.Z).Then(geom.Identity().Rotate(-v*24, 0, 0)).Then(geom.At(pivot.X, pivot.Y, pivot.Z))
}

func clamp1(v float64) float64 { return math.Max(-1, math.Min(1, v)) }

func (m *Model) material(name string, ctx FrameContext) cell.Color {
	switch name {
	case "light":
		if ctx.Light != nil {
			return rgb(*ctx.Light)
		}
	case "mic_led":
		if ctx.MicLED {
			return rgb(RGB{255, 170, 0})
		}
	case "switch_knob":
		if c, ok := m.Spec.Materials[name]; ok {
			return rgb(c)
		}
		return cell.NewColorRGB(150, 156, 168)
	}
	if c, ok := m.Spec.Materials[name]; ok {
		return rgb(c)
	}
	return cell.NewColorRGB(128, 132, 140)
}

func (m *Model) colorize(ctx FrameContext) {
	// Resolve each part once, then fan out to its faces.
	if cap(m.partColors) < len(m.parts) {
		m.partColors = make([]cell.Color, len(m.parts))
	}
	pcs := m.partColors[:len(m.parts)]
	for pi, p := range m.parts {
		pcs[pi] = m.partColor(p, ctx)
	}
	for f, pi := range m.faceParts {
		m.colors[f] = pcs[pi]
	}
}

func (m *Model) partColor(p part, ctx FrameContext) cell.Color {
	col := m.material(p.material, ctx)
	if p.comp >= 0 && m.Spec.Components[p.comp].Pickable() {
		c := m.Spec.Components[p.comp]
		switch {
		case p.comp == ctx.Selected || (ctx.Region != device.RegionNone && c.Region == ctx.Region):
			col = graphics.InterpolateColor(col, ctx.Accent, 0.55)
		case p.comp == ctx.Hovered:
			col = graphics.InterpolateColor(col, ctx.Hover, 0.38)
		}
		if m.pressed(ctx, p.comp) {
			col = graphics.InterpolateColor(col, ctx.Pressing, 0.6)
		}
	}
	if ctx.Dim {
		col = graphics.InterpolateColor(col, ctx.DimTo, 0.55)
	}
	return col
}

// rgb tone-maps a material colour for terminal rendering: a lifted floor
// keeps black plastics readable on dark terminal backgrounds while light
// shells keep their brightness.
func rgb(c RGB) cell.Color {
	lift := func(v uint8) uint8 { return uint8(math.Min(255, 46+float64(v)*0.82)) }
	return cell.NewColorRGB(lift(c.R), lift(c.G), lift(c.B))
}
