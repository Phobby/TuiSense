// Package geom builds low-poly triangle/quad meshes for Limoni's Viewer3D.
//
// Conventions (shared by every TuiSense controller model):
//   - +X is the controller's right as seen by the player holding it, +Y is up,
//     and -Z is the front face (the camera looks from -Z toward +Z).
//   - Faces wind counter-clockwise when seen from outside, which is what
//     Viewer3D treats as front-facing. Its outward normal is -(e1 × e2).
//   - Every face carries a part index so a renderer can colour, animate and
//     pick semantic components without knowing how they were built.
package geom

import (
	"math"

	"github.com/thebanri/limoni/graphics"
)

// Vec2 is a point of a 2D outline (X right, Y up).
type Vec2 struct{ X, Y float64 }

// Vec3 aliases Limoni's vertex type so meshes need no conversion.
type Vec3 = graphics.Vertex3D

// Mesh is an indexed polygon mesh whose faces are tagged with a part index.
type Mesh struct {
	Verts []Vec3
	Faces [][]int
	Parts []int // part index of every face
}

// Xform is a rotation followed by a translation.
type Xform struct {
	m [3][3]float64
	T Vec3
}

// Identity returns the identity transform.
func Identity() Xform { return Xform{m: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}} }

// At returns a translation.
func At(x, y, z float64) Xform {
	xf := Identity()
	xf.T = Vec3{X: x, Y: y, Z: z}
	return xf
}

// Rotate returns xf rotated (in its local frame) by Euler angles in degrees,
// applied X, then Y, then Z.
func (xf Xform) Rotate(xDeg, yDeg, zDeg float64) Xform {
	r := mul(rotZ(zDeg), mul(rotY(yDeg), rotX(xDeg)))
	xf.m = mul(xf.m, r)
	return xf
}

// Then composes: the result applies xf first, then next.
func (xf Xform) Then(next Xform) Xform {
	return Xform{m: mul(next.m, xf.m), T: next.Apply(xf.T)}
}

// Apply transforms a point.
func (xf Xform) Apply(v Vec3) Vec3 {
	m := xf.m
	return Vec3{
		X: m[0][0]*v.X + m[0][1]*v.Y + m[0][2]*v.Z + xf.T.X,
		Y: m[1][0]*v.X + m[1][1]*v.Y + m[1][2]*v.Z + xf.T.Y,
		Z: m[2][0]*v.X + m[2][1]*v.Y + m[2][2]*v.Z + xf.T.Z,
	}
}

func mul(a, b [3][3]float64) [3][3]float64 {
	var out [3][3]float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = a[i][0]*b[0][j] + a[i][1]*b[1][j] + a[i][2]*b[2][j]
		}
	}
	return out
}

func rotX(d float64) [3][3]float64 {
	s, c := math.Sincos(d * math.Pi / 180)
	return [3][3]float64{{1, 0, 0}, {0, c, -s}, {0, s, c}}
}

func rotY(d float64) [3][3]float64 {
	s, c := math.Sincos(d * math.Pi / 180)
	return [3][3]float64{{c, 0, s}, {0, 1, 0}, {-s, 0, c}}
}

func rotZ(d float64) [3][3]float64 {
	s, c := math.Sincos(d * math.Pi / 180)
	return [3][3]float64{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}
}

// Builder appends primitives to a mesh under the current part index.
type Builder struct {
	Mesh
	Part int
}

func (b *Builder) addVerts(xf Xform, local []Vec3) int {
	base := len(b.Verts)
	for _, v := range local {
		b.Verts = append(b.Verts, xf.Apply(v))
	}
	return base
}

func (b *Builder) face(idx ...int) {
	b.Faces = append(b.Faces, idx)
	b.Parts = append(b.Parts, b.Part)
}

// Box adds a cuboid of size sx×sy×sz centred on xf.
func (b *Builder) Box(xf Xform, sx, sy, sz float64) {
	hx, hy, hz := sx/2, sy/2, sz/2
	o := b.addVerts(xf, []Vec3{
		{X: -hx, Y: -hy, Z: -hz}, {X: hx, Y: -hy, Z: -hz}, {X: hx, Y: hy, Z: -hz}, {X: -hx, Y: hy, Z: -hz},
		{X: -hx, Y: -hy, Z: hz}, {X: hx, Y: -hy, Z: hz}, {X: hx, Y: hy, Z: hz}, {X: -hx, Y: hy, Z: hz},
	})
	for _, f := range [6][4]int{{0, 1, 2, 3}, {5, 4, 7, 6}, {1, 5, 6, 2}, {4, 0, 3, 7}, {3, 2, 6, 7}, {4, 5, 1, 0}} {
		b.face(o+f[0], o+f[1], o+f[2], o+f[3])
	}
}

// Cylinder adds an n-sided cylinder along local Z whose front cap (at
// -depth/2) faces -Z. rx and ry allow elliptical sections.
func (b *Builder) Cylinder(xf Xform, rx, ry, depth float64, n int) {
	b.Prism(xf, Ellipse(rx, ry, n), depth)
}

// Frustum is a cylinder whose front cap radius differs from its back cap.
func (b *Builder) Frustum(xf Xform, rFront, rBack, depth float64, n int) {
	front := Ellipse(rFront, rFront, n)
	back := Ellipse(rBack, rBack, n)
	b.rings(xf, [][]Vec2{front, back}, []float64{-depth / 2, depth / 2}, front)
}

// Prism extrudes a convex or concave outline (any winding) along local Z.
func (b *Builder) Prism(xf Xform, outline []Vec2, depth float64) {
	outline = CCW(outline)
	b.rings(xf, [][]Vec2{outline, outline}, []float64{-depth / 2, depth / 2}, outline)
}

// Slab extrudes an outline with bevelled front and back edges, the main body
// primitive: the caps are inset by bevel and the side wall has three bands.
func (b *Builder) Slab(xf Xform, outline []Vec2, depth, bevel float64) {
	outline = CCW(outline)
	if bevel <= 0 || bevel*2 >= depth {
		b.Prism(xf, outline, depth)
		return
	}
	inset := Inset(outline, bevel)
	h := depth / 2
	b.rings(xf, [][]Vec2{inset, outline, outline, inset}, []float64{-h, -h + bevel, h - bevel, h}, outline)
}

// rings stitches outline rings (all the same length, CCW) at the given local
// depths into a closed solid; the caps are triangulated from capShape.
func (b *Builder) rings(xf Xform, rings [][]Vec2, zs []float64, capShape []Vec2) {
	n := len(rings[0])
	if n < 3 {
		return
	}
	starts := make([]int, len(rings))
	for r, ring := range rings {
		local := make([]Vec3, n)
		for i, p := range ring {
			local[i] = Vec3{X: p.X, Y: p.Y, Z: zs[r]}
		}
		starts[r] = b.addVerts(xf, local)
	}
	for r := 0; r+1 < len(rings); r++ {
		a, c := starts[r], starts[r+1]
		for i := 0; i < n; i++ {
			j := (i + 1) % n
			b.face(a+i, c+i, c+j, a+j)
		}
	}
	tris := Triangulate(capShape)
	front, back := starts[0], starts[len(starts)-1]
	for _, t := range tris {
		b.face(front+t[0], front+t[1], front+t[2])
		b.face(back+t[2], back+t[1], back+t[0])
	}
}

// Ellipsoid adds a UV ellipsoid with radii rx, ry, rz.
func (b *Builder) Ellipsoid(xf Xform, rx, ry, rz float64, seg, rings int) {
	if seg < 3 || rings < 2 {
		return
	}
	var local []Vec3
	local = append(local, Vec3{Y: ry}) // north pole
	for r := 1; r < rings; r++ {
		phi := math.Pi * float64(r) / float64(rings)
		sp, cp := math.Sincos(phi)
		for s := 0; s < seg; s++ {
			th := 2 * math.Pi * float64(s) / float64(seg)
			st, ct := math.Sincos(th)
			local = append(local, Vec3{X: rx * sp * ct, Y: ry * cp, Z: rz * sp * st})
		}
	}
	local = append(local, Vec3{Y: -ry}) // south pole
	o := b.addVerts(xf, local)
	north, south := o, o+len(local)-1
	ring := func(r, s int) int { return o + 1 + (r-1)*seg + (s % seg) }
	for s := 0; s < seg; s++ {
		b.face(north, ring(1, s), ring(1, s+1))
		b.face(south, ring(rings-1, s+1), ring(rings-1, s))
	}
	for r := 1; r+1 < rings; r++ {
		for s := 0; s < seg; s++ {
			b.face(ring(r, s), ring(r+1, s), ring(r+1, s+1), ring(r, s+1))
		}
	}
}

// Append copies another mesh into the builder, keeping its part indices
// offset by partBase.
func (b *Builder) Append(m Mesh, partBase int) {
	base := len(b.Verts)
	b.Verts = append(b.Verts, m.Verts...)
	for i, f := range m.Faces {
		nf := make([]int, len(f))
		for k, idx := range f {
			nf[k] = idx + base
		}
		b.Faces = append(b.Faces, nf)
		b.Parts = append(b.Parts, m.Parts[i]+partBase)
	}
}

// OutwardNormal is the normal Viewer3D lights a face with.
func OutwardNormal(v0, v1, v2 Vec3) Vec3 {
	e1 := Vec3{X: v1.X - v0.X, Y: v1.Y - v0.Y, Z: v1.Z - v0.Z}
	e2 := Vec3{X: v2.X - v0.X, Y: v2.Y - v0.Y, Z: v2.Z - v0.Z}
	n := Vec3{X: -(e1.Y*e2.Z - e1.Z*e2.Y), Y: -(e1.Z*e2.X - e1.X*e2.Z), Z: -(e1.X*e2.Y - e1.Y*e2.X)}
	l := math.Sqrt(n.X*n.X + n.Y*n.Y + n.Z*n.Z)
	if l == 0 {
		return n
	}
	return Vec3{X: n.X / l, Y: n.Y / l, Z: n.Z / l}
}

// Bounds returns the axis-aligned bounding box of vertices.
func Bounds(vs []Vec3) (lo, hi Vec3) {
	if len(vs) == 0 {
		return
	}
	lo, hi = vs[0], vs[0]
	for _, v := range vs[1:] {
		lo.X, lo.Y, lo.Z = math.Min(lo.X, v.X), math.Min(lo.Y, v.Y), math.Min(lo.Z, v.Z)
		hi.X, hi.Y, hi.Z = math.Max(hi.X, v.X), math.Max(hi.Y, v.Y), math.Max(hi.Z, v.Z)
	}
	return
}
