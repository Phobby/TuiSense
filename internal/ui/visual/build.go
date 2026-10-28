package visual

import (
	"math"

	"github.com/Phobby/TuiSense/internal/ui/geom"
)

// unitsPerMM converts spec millimetres to renderer units. Every model shares
// it, so relative proportions between controllers are preserved; the camera
// frames each model from its own bounding box.
const unitsPerMM = 1.0 / 60.0

const bodyPart = -1 // part component index of the shell when it is not a component

// part is one primitive of the built mesh: a contiguous vertex range with a
// component, a material and an optional animation.
type part struct {
	comp       int // component index, or bodyPart
	material   string
	first, end int // vertex range
	anim       Anim
	pivot      geom.Vec3 // animation pivot (renderer units)
	press      geom.Vec3 // inward offset applied while the bound button is held
	dynamic    bool
}

// generator turns a spec into a mesh in millimetres.
type generator struct {
	spec    *ModelSpec
	b       geom.Builder
	outline []geom.Vec2
	parts   []part
	grips   []Geometry // ellipsoids used to find the rear surface
}

func (g *generator) begin(comp int, material string) int {
	g.parts = append(g.parts, part{comp: comp, material: material, first: len(g.b.Verts)})
	g.b.Part = len(g.parts) - 1
	return g.b.Part
}

func (g *generator) end() { g.parts[len(g.parts)-1].end = len(g.b.Verts) }

func (g *generator) halfDepth() float64 { return g.spec.Body.Depth / 2 }

// topY / bottomY follow the silhouette at x (clamped to the outline's span).
func (g *generator) topY(x float64) float64 {
	for d := 0.0; d < 40; d += 1 {
		for _, xx := range []float64{x - d, x + d} {
			if t, _, ok := geom.EdgeY(g.outline, xx); ok {
				return t
			}
		}
	}
	return 0
}

func (g *generator) bottomY(x float64) float64 {
	for d := 0.0; d < 40; d += 1 {
		for _, xx := range []float64{x - d, x + d} {
			if _, b, ok := geom.EdgeY(g.outline, xx); ok {
				return b
			}
		}
	}
	return 0
}

func (g *generator) slope(top bool, x float64) float64 {
	f := g.bottomY
	if top {
		f = g.topY
	}
	return math.Atan2(f(x+2)-f(x-2), 4) * 180 / math.Pi
}

// rearZ is the outermost back surface at (x, y): the slab or a grip bulge.
func (g *generator) rearZ(x, y float64) float64 {
	z := g.halfDepth()
	for _, e := range g.grips {
		rx, ry, rz := e.Size[0]/2, e.Size[1]/2, e.Size[2]/2
		dx, dy := (x-e.At[0])/rx, (y-e.At[1])/ry
		if q := dx*dx + dy*dy; q < 1 {
			z = math.Max(z, e.At[2]+rz*math.Sqrt(1-q))
		}
	}
	b := g.spec.Body
	if b.SpineDepth > 0 && math.Abs(x) < b.SpineWidth/2 && y > g.topY(x)-b.SpineHeight {
		z = math.Max(z, g.halfDepth()-2+b.SpineDepth)
	}
	return z
}

// place returns the mm transform of a shape and the inward unit normal of
// its mounting surface (used for press feedback).
func (g *generator) place(c Component, s Geometry) (geom.Xform, geom.Vec3) {
	x, y, lift := s.At[0], s.At[1], s.At[2]
	h := s.Size[2] // extent along the mounting axis
	surface := c.Surface
	if s.Surface != "" {
		surface = s.Surface
	}
	if surface == "" {
		surface = Front
	}
	rot := func(xf geom.Xform) geom.Xform { return xf.Rotate(s.Rot[0], s.Rot[1], s.Rot[2]) }
	switch surface {
	case Rear:
		z := g.rearZ(x, y) + 0.2*h + lift
		// World rotation first, then turn the primitive's front cap to +Z.
		return rot(geom.At(x, y, z)).Rotate(0, 180, 0), geom.Vec3{Z: -1}
	case Top, Bottom:
		top := surface == Top
		edge, sign := g.bottomY(x), -1.0
		if top {
			edge, sign = g.topY(x), 1.0
		}
		slope := g.slope(top, x)
		switch s.Shape {
		case ShapeBumper, ShapeTrigger, ShapeBox, ShapeSlider, ShapeEllipsoid:
			// Boxes sit on the edge (y offset by half their height), tilted to its slope.
			yy := edge + sign*(s.Size[1]/2*0.6+y)
			return rot(geom.At(x, yy, lift).Rotate(0, 0, slope)), geom.Vec3{Y: -sign}
		default:
			// Caps face outward along ±Y.
			yy := edge + sign*(0.2*h+y)
			return rot(geom.At(x, yy, lift).Rotate(0, 0, slope).Rotate(sign*90, 0, 0)), geom.Vec3{Y: -sign}
		}
	case Inside:
		return rot(geom.At(x, y, lift)), geom.Vec3{}
	}
	z := -g.halfDepth() - 0.2*h - lift
	return rot(geom.At(x, y, z)), geom.Vec3{Z: 1}
}

// shape emits one primitive; for sticks it emits the static well as its own part.
func (g *generator) shape(ci int, c Component, s Geometry) {
	mat := s.Material
	if mat == "" {
		mat = c.Material
	}
	xf, inward := g.place(c, s)
	seg := s.Segments
	if seg == 0 {
		seg = 14
	}
	w, h, d := s.Size[0], s.Size[1], s.Size[2]
	pi := g.begin(ci, mat)
	pr := &g.parts[pi]
	if c.Bound {
		pr.press = geom.Vec3{X: inward.X * 0.7, Y: inward.Y * 0.7, Z: inward.Z * 0.7}
		pr.dynamic = true
	}
	switch s.Shape {
	case ShapeDisc:
		ry := h
		if ry == 0 {
			ry = w
		}
		g.b.Cylinder(xf, w, ry, d, seg)
	case ShapeDome:
		g.b.Frustum(xf, w*0.8, w, d, seg)
	case ShapeBox, ShapeBumper, ShapePaddle:
		g.b.Box(xf, w, h, d)
	case ShapeTrigger:
		g.b.Box(xf, w, h, d)
		pr.pivot = xf.Apply(geom.Vec3{Y: h / 2, Z: -d / 2})
		pr.anim, pr.dynamic = c.Anim, c.Anim != AnimNone
	case ShapePad:
		g.b.Slab(xf, geom.RoundedRect(w, h, math.Min(w, h)*0.32, 3), d, d*0.3)
	case ShapeEllipsoid:
		g.b.Ellipsoid(xf, w/2, h/2, d/2, 12, 7)
	case ShapeCross:
		g.b.Prism(xf, geom.Cross(w, h), d)
	case ShapeSlider:
		g.b.Box(xf, w, h, d*0.4)
		g.end()
		g.begin(ci, "switch_knob")
		g.b.Box(geom.At(-w*0.25, 0, -d*0.3).Then(xf), w*0.4, h*1.1, d*0.7)
	case ShapeStick:
		// Static well, then the moving shaft + cap pivoting at the surface.
		well := w * 1.35
		g.b.Cylinder(geom.At(0, 0, d*0.2).Then(xf), well, well, 1.6, seg)
		g.end()
		pi = g.begin(ci, mat)
		pr = &g.parts[pi]
		pr.pivot = xf.Apply(geom.Vec3{Z: d * 0.2})
		pr.anim, pr.dynamic = c.Anim, c.Anim != AnimNone
		if c.Bound {
			pr.press = geom.Vec3{X: inward.X * 1.2, Y: inward.Y * 1.2, Z: inward.Z * 1.2}
			pr.dynamic = true
		}
		g.b.Cylinder(geom.At(0, 0, -d*0.25).Then(xf), w*0.42, w*0.42, d*0.75, 10)
		g.b.Frustum(geom.At(0, 0, -d*0.62).Then(xf), w*0.86, w, d*0.32, seg)
	}
	g.end()
}

// build generates the mesh for spec, centred on its bounding box and scaled
// to renderer units.
func build(spec *ModelSpec) *Model {
	g := &generator{spec: spec}
	half := make([]geom.Vec2, len(spec.Body.Half))
	for i, p := range spec.Body.Half {
		half[i] = geom.Vec2{X: p[0], Y: p[1]}
	}
	steps := spec.Body.Smooth
	if steps < 1 {
		steps = 1
	}
	g.outline = geom.CCW(geom.Smooth(geom.Mirror(half), steps))
	for _, c := range spec.Components {
		for _, s := range c.Shapes {
			if s.Shape == ShapeEllipsoid && c.Surface == Inside {
				g.grips = append(g.grips, s)
			}
		}
	}

	bodyComp := bodyPart
	compIndex := map[string]int{}
	for i, c := range spec.Components {
		compIndex[c.ID] = i
	}
	if i, ok := compIndex[spec.Body.ComponentID]; ok {
		bodyComp = i
	}
	g.begin(bodyComp, spec.Body.Material)
	g.b.Slab(geom.Identity(), g.outline, spec.Body.Depth, spec.Body.Bevel)
	g.end()
	if b := spec.Body; b.SpineDepth > 0 {
		g.begin(bodyComp, b.Material)
		top := g.topY(0)
		// Rounded-rect profile in the XZ plane, extruded along Y.
		xf := geom.At(0, top-b.SpineHeight/2-1, g.halfDepth()-2+b.SpineDepth/2).Rotate(90, 0, 0)
		g.b.Slab(xf, geom.RoundedRect(b.SpineWidth, b.SpineDepth, b.SpineDepth*0.45, 3), b.SpineHeight, 2)
		g.end()
	}
	for i, c := range spec.Components {
		for _, s := range c.Shapes {
			g.shape(i, c, s)
		}
	}

	m := &Model{Spec: spec, name: spec.Manufacturer + " " + spec.Name, faces: g.b.Faces, faceParts: g.b.Parts, parts: g.parts, compIndex: compIndex}
	lo, hi := geom.Bounds(g.b.Verts)
	ctr := geom.Vec3{X: (lo.X + hi.X) / 2, Y: (lo.Y + hi.Y) / 2, Z: (lo.Z + hi.Z) / 2}
	conv := func(v geom.Vec3) geom.Vec3 {
		return geom.Vec3{X: (v.X - ctr.X) * unitsPerMM, Y: (v.Y - ctr.Y) * unitsPerMM, Z: (v.Z - ctr.Z) * unitsPerMM}
	}
	m.rest = make([]geom.Vec3, len(g.b.Verts))
	for i, v := range g.b.Verts {
		m.rest[i] = conv(v)
	}
	m.verts = append([]geom.Vec3(nil), m.rest...)
	for i := range m.parts {
		p := &m.parts[i]
		p.pivot = conv(p.pivot)
		p.press = geom.Vec3{X: p.press.X * unitsPerMM, Y: p.press.Y * unitsPerMM, Z: p.press.Z * unitsPerMM}
	}
	m.half = geom.Vec3{X: (hi.X - lo.X) / 2 * unitsPerMM, Y: (hi.Y - lo.Y) / 2 * unitsPerMM, Z: (hi.Z - lo.Z) / 2 * unitsPerMM}
	m.normals = make([]geom.Vec3, len(spec.Components))
	for i, c := range spec.Components {
		surface := c.Surface
		if len(c.Shapes) > 0 && c.Shapes[0].Surface != "" {
			surface = c.Shapes[0].Surface
		}
		switch surface {
		case Rear:
			m.normals[i] = geom.Vec3{Z: 1}
		case Top:
			m.normals[i] = geom.Vec3{Y: 1}
		case Bottom:
			m.normals[i] = geom.Vec3{Y: -1}
		case Front:
			m.normals[i] = geom.Vec3{Z: -1}
		}
	}
	m.centroids = make([]geom.Vec3, len(spec.Components))
	counts := make([]int, len(spec.Components))
	for _, p := range m.parts {
		if p.comp < 0 {
			continue
		}
		for v := p.first; v < p.end; v++ {
			c := &m.centroids[p.comp]
			c.X += m.rest[v].X
			c.Y += m.rest[v].Y
			c.Z += m.rest[v].Z
			counts[p.comp]++
		}
	}
	for i, n := range counts {
		if n > 0 {
			c := &m.centroids[i]
			c.X, c.Y, c.Z = c.X/float64(n), c.Y/float64(n), c.Z/float64(n)
		}
	}
	m.colors = make([]cellColor, len(m.faces))
	return m
}
