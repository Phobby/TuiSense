package geom

import "math"

// Ellipse returns an n-gon approximating an ellipse, counter-clockwise.
func Ellipse(rx, ry float64, n int) []Vec2 {
	out := make([]Vec2, n)
	for i := range out {
		s, c := math.Sincos(2 * math.Pi * float64(i) / float64(n))
		out[i] = Vec2{X: c * rx, Y: s * ry}
	}
	return out
}

// RoundedRect returns a w×h rectangle with corner radius r (n segments per corner).
func RoundedRect(w, h, r float64, n int) []Vec2 {
	r = math.Min(r, math.Min(w, h)/2)
	if n < 1 || r <= 0 {
		return []Vec2{{-w / 2, -h / 2}, {w / 2, -h / 2}, {w / 2, h / 2}, {-w / 2, h / 2}}
	}
	cx, cy := w/2-r, h/2-r
	var out []Vec2
	corners := [4]Vec2{{cx, -cy}, {cx, cy}, {-cx, cy}, {-cx, -cy}}
	for k, c := range corners {
		start := -math.Pi/2 + float64(k)*math.Pi/2
		for i := 0; i <= n; i++ {
			s, co := math.Sincos(start + float64(i)*(math.Pi/2)/float64(n))
			out = append(out, Vec2{X: c.X + co*r, Y: c.Y + s*r})
		}
	}
	return out
}

// Cross returns a plus-shaped outline with arm span `span` and arm width `w`.
func Cross(span, w float64) []Vec2 {
	a, h := span/2, w/2
	return []Vec2{
		{h, -a}, {h, -h}, {a, -h}, {a, h}, {h, h}, {h, a},
		{-h, a}, {-h, h}, {-a, h}, {-a, -h}, {-h, -h}, {-h, -a},
	}
}

// SignedArea is positive for counter-clockwise outlines.
func SignedArea(p []Vec2) float64 {
	a := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i].X*p[j].Y - p[j].X*p[i].Y
	}
	return a / 2
}

// CCW returns the outline in counter-clockwise order.
func CCW(p []Vec2) []Vec2 {
	if SignedArea(p) >= 0 {
		return p
	}
	out := make([]Vec2, len(p))
	for i := range p {
		out[i] = p[len(p)-1-i]
	}
	return out
}

// Mirror turns a right-half profile, listed from the top centre clockwise to
// the bottom centre (x ≥ 0), into a closed symmetric outline.
func Mirror(half []Vec2) []Vec2 {
	out := append([]Vec2(nil), half...)
	for i := len(half) - 1; i >= 0; i-- {
		p := half[i]
		if p.X <= 1e-9 { // centre-line points are shared
			continue
		}
		out = append(out, Vec2{X: -p.X, Y: p.Y})
	}
	return out
}

// Smooth subdivides a closed outline with a centripetal-free (uniform)
// Catmull-Rom spline, `steps` points per original edge.
func Smooth(p []Vec2, steps int) []Vec2 {
	if steps <= 1 || len(p) < 4 {
		return p
	}
	n := len(p)
	out := make([]Vec2, 0, n*steps)
	for i := 0; i < n; i++ {
		p0, p1, p2, p3 := p[(i-1+n)%n], p[i], p[(i+1)%n], p[(i+2)%n]
		for s := 0; s < steps; s++ {
			t := float64(s) / float64(steps)
			t2, t3 := t*t, t*t*t
			f := func(a, b, c, d float64) float64 {
				return 0.5 * (2*b + (-a+c)*t + (2*a-5*b+4*c-d)*t2 + (-a+3*b-3*c+d)*t3)
			}
			out = append(out, Vec2{X: f(p0.X, p1.X, p2.X, p3.X), Y: f(p0.Y, p1.Y, p2.Y, p3.Y)})
		}
	}
	return out
}

// Scale multiplies every point.
func Scale(p []Vec2, s float64) []Vec2 {
	out := make([]Vec2, len(p))
	for i, v := range p {
		out[i] = Vec2{X: v.X * s, Y: v.Y * s}
	}
	return out
}

// Inset offsets a counter-clockwise outline inward by d (mitred, clamped).
func Inset(p []Vec2, d float64) []Vec2 {
	n := len(p)
	out := make([]Vec2, n)
	for i := range p {
		a, b, c := p[(i-1+n)%n], p[i], p[(i+1)%n]
		n1 := leftNormal(a, b)
		n2 := leftNormal(b, c)
		m := Vec2{X: n1.X + n2.X, Y: n1.Y + n2.Y}
		l := math.Hypot(m.X, m.Y)
		if l < 1e-9 {
			out[i] = b
			continue
		}
		m = Vec2{X: m.X / l, Y: m.Y / l}
		cos := m.X*n1.X + m.Y*n1.Y
		k := d / math.Max(cos, 0.35)
		out[i] = Vec2{X: b.X + m.X*k, Y: b.Y + m.Y*k}
	}
	return out
}

// leftNormal is the unit normal on the left of a→b (inward for CCW outlines).
func leftNormal(a, b Vec2) Vec2 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return Vec2{}
	}
	return Vec2{X: -dy / l, Y: dx / l}
}

// Triangulate ear-clips a simple polygon (CCW) into counter-clockwise triangles.
func Triangulate(p []Vec2) [][3]int {
	n := len(p)
	if n < 3 {
		return nil
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	var out [][3]int
	guard := 0
	for len(idx) > 3 && guard < n*n {
		guard++
		clipped := false
		for k := range idx {
			i0, i1, i2 := idx[(k-1+len(idx))%len(idx)], idx[k], idx[(k+1)%len(idx)]
			a, b, c := p[i0], p[i1], p[i2]
			if cross(a, b, c) <= 1e-12 {
				continue // reflex or degenerate
			}
			ear := true
			for _, j := range idx {
				if j == i0 || j == i1 || j == i2 {
					continue
				}
				if inTriangle(p[j], a, b, c) {
					ear = false
					break
				}
			}
			if !ear {
				continue
			}
			out = append(out, [3]int{i0, i1, i2})
			idx = append(idx[:k], idx[k+1:]...)
			clipped = true
			break
		}
		if !clipped {
			break // numerically stuck: fan the rest
		}
	}
	for k := 1; k+1 < len(idx); k++ {
		out = append(out, [3]int{idx[0], idx[k], idx[k+1]})
	}
	return out
}

func cross(a, b, c Vec2) float64 { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }

func inTriangle(p, a, b, c Vec2) bool {
	return cross(a, b, p) >= 0 && cross(b, c, p) >= 0 && cross(c, a, p) >= 0
}

// EdgeY returns the highest (top) and lowest (bottom) Y where the vertical
// line at x crosses the outline; ok is false when it misses.
func EdgeY(p []Vec2, x float64) (top, bottom float64, ok bool) {
	top, bottom = math.Inf(-1), math.Inf(1)
	for i := range p {
		a, b := p[i], p[(i+1)%len(p)]
		if (a.X <= x && b.X > x) || (b.X <= x && a.X > x) {
			t := (x - a.X) / (b.X - a.X)
			y := a.Y + t*(b.Y-a.Y)
			top, bottom = math.Max(top, y), math.Min(bottom, y)
			ok = true
		}
	}
	return
}

// EdgeX returns the outermost X (right side) where the horizontal line at y
// crosses the outline.
func EdgeX(p []Vec2, y float64) (right float64, ok bool) {
	right = math.Inf(-1)
	for i := range p {
		a, b := p[i], p[(i+1)%len(p)]
		if (a.Y <= y && b.Y > y) || (b.Y <= y && a.Y > y) {
			t := (y - a.Y) / (b.Y - a.Y)
			right = math.Max(right, a.X+t*(b.X-a.X))
			ok = true
		}
	}
	return
}
