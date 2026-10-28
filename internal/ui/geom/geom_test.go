package geom

import (
	"math"
	"testing"
)

func centroid(m Mesh, f []int) Vec3 {
	var c Vec3
	for _, i := range f {
		c.X += m.Verts[i].X
		c.Y += m.Verts[i].Y
		c.Z += m.Verts[i].Z
	}
	n := float64(len(f))
	return Vec3{X: c.X / n, Y: c.Y / n, Z: c.Z / n}
}

// Every face of a convex solid must face away from its centre, or Viewer3D
// would cull the visible side and light the hidden one.
func assertOutward(t *testing.T, name string, m Mesh, centre Vec3) {
	t.Helper()
	for i, f := range m.Faces {
		n := OutwardNormal(m.Verts[f[0]], m.Verts[f[1]], m.Verts[f[2]])
		c := centroid(m, f)
		d := (c.X-centre.X)*n.X + (c.Y-centre.Y)*n.Y + (c.Z-centre.Z)*n.Z
		if d <= 0 {
			t.Fatalf("%s: face %d winds inward (dot %.4f)", name, i, d)
		}
	}
}

func TestPrimitivesWindOutward(t *testing.T) {
	xf := At(0.3, -0.2, 0.1).Rotate(20, -35, 10)
	centre := xf.Apply(Vec3{})
	cases := map[string]func(*Builder){
		"box":       func(b *Builder) { b.Box(xf, 1, 0.5, 0.3) },
		"cylinder":  func(b *Builder) { b.Cylinder(xf, 0.4, 0.3, 0.2, 10) },
		"frustum":   func(b *Builder) { b.Frustum(xf, 0.4, 0.25, 0.3, 10) },
		"ellipsoid": func(b *Builder) { b.Ellipsoid(xf, 0.5, 0.3, 0.4, 10, 6) },
		"slab":      func(b *Builder) { b.Slab(xf, RoundedRect(1, 0.6, 0.15, 3), 0.3, 0.06) },
	}
	for name, add := range cases {
		var b Builder
		add(&b)
		assertOutward(t, name, b.Mesh, centre)
	}
}

func TestFrontCapFacesCamera(t *testing.T) {
	var b Builder
	b.Cylinder(Identity(), 1, 1, 0.2, 8)
	// The first cap face after the side wall is the front (-Z) cap.
	for i, f := range b.Faces {
		c := centroid(b.Mesh, f)
		if c.Z < -0.099 {
			n := OutwardNormal(b.Verts[f[0]], b.Verts[f[1]], b.Verts[f[2]])
			if n.Z > -0.99 {
				t.Fatalf("front cap face %d normal %+v, want -Z", i, n)
			}
		}
	}
}

func TestTriangulateConcaveOutline(t *testing.T) {
	// A controller-like silhouette with a concave notch between the grips.
	half := []Vec2{{0, 1}, {1, 1}, {1.4, 0}, {1.2, -1}, {0.8, -1.1}, {0.5, -0.4}, {0, -0.3}}
	p := CCW(Smooth(Mirror(half), 3))
	tris := Triangulate(p)
	if len(tris) != len(p)-2 {
		t.Fatalf("ear clipping produced %d triangles for %d points", len(tris), len(p))
	}
	area := 0.0
	for _, tr := range tris {
		a := cross(p[tr[0]], p[tr[1]], p[tr[2]]) / 2
		if a < -1e-9 {
			t.Fatal("triangle wound clockwise")
		}
		area += a
	}
	if math.Abs(area-SignedArea(p)) > 1e-6 {
		t.Fatalf("triangles cover %.4f, outline %.4f", area, SignedArea(p))
	}
}

func TestXformComposition(t *testing.T) {
	a := At(1, 0, 0).Rotate(0, 0, 90)
	b := At(0, 2, 0)
	v := a.Then(b).Apply(Vec3{X: 1})
	if math.Abs(v.X-1) > 1e-9 || math.Abs(v.Y-3) > 1e-9 {
		t.Fatalf("composed transform: %+v", v)
	}
}

func TestEdgeQueries(t *testing.T) {
	p := RoundedRect(4, 2, 0, 0)
	top, bottom, ok := EdgeY(p, 0.5)
	if !ok || top != 1 || bottom != -1 {
		t.Fatalf("EdgeY: %v %v %v", top, bottom, ok)
	}
	if r, ok := EdgeX(p, 0.2); !ok || r != 2 {
		t.Fatalf("EdgeX: %v %v", r, ok)
	}
}
