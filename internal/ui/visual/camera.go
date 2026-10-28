package visual

import (
	"math"

	"github.com/Phobby/TuiSense/internal/ui/geom"
	"github.com/thebanri/limoni/core/cell"
)

// DefaultDistance is the camera distance used for every model.
const DefaultDistance = 4.2

// Camera is the orbit used to draw (and therefore pick) a model. It mirrors
// Limoni's Viewer3D exactly: rotation Y, then X, then Z (degrees), a camera
// at -Distance on Z, and a perspective scale of viewportHeight·4·0.40·Scale
// in Braille dots.
type Camera struct {
	RotX, RotY, RotZ float64
	Distance         float64
	Scale            float64
}

// FitScale returns the Viewer3D Scale that frames m inside a w×h cell
// viewport at the given zoom, from the model's own bounding box. Proportions
// are never altered; only the framing changes.
func (m *Model) FitScale(w, h uint16, zoom float64) float64 {
	if w == 0 || h == 0 {
		return 1
	}
	vw, vh := float64(w)*2, float64(h)*4
	// Allow for the default downward tilt and perspective of the nearest face.
	tilt := 22 * math.Pi / 180
	hx := math.Max(m.half.X, math.Hypot(m.half.X, m.half.Z)*0.92)
	hy := m.half.Y*math.Cos(tilt) + m.half.Z*math.Sin(tilt)
	near := DefaultDistance - math.Max(m.half.Z, m.half.X*0.35)
	sx := 0.47 * vw * near / (hx * vh * 0.40)
	sy := 0.45 * vh * near / (hy * vh * 0.40)
	return math.Min(sx, sy) * zoom
}

type rotation struct{ sx, cx, sy, cy, sz, cz float64 }

func newRotation(x, y, z float64) rotation {
	const rad = math.Pi / 180
	var r rotation
	r.sx, r.cx = math.Sincos(x * rad)
	r.sy, r.cy = math.Sincos(y * rad)
	r.sz, r.cz = math.Sincos(z * rad)
	return r
}

func (r rotation) apply(v geom.Vec3) geom.Vec3 {
	v = geom.Vec3{X: v.X*r.cy + v.Z*r.sy, Y: v.Y, Z: -v.X*r.sy + v.Z*r.cy}
	v = geom.Vec3{X: v.X, Y: v.Y*r.cx - v.Z*r.sx, Z: v.Y*r.sx + v.Z*r.cx}
	return geom.Vec3{X: v.X*r.cz - v.Y*r.sz, Y: v.X*r.sz + v.Y*r.cz, Z: v.Z}
}

// projector maps model space to Braille-dot coordinates of a viewport.
type projector struct {
	rot          rotation
	vw, vh       float64
	dist, factor float64
}

func newProjector(cam Camera, area cell.Rect) projector {
	dist := cam.Distance
	if dist <= 0.1 {
		dist = DefaultDistance
	}
	scale := cam.Scale
	if scale <= 0 {
		scale = 1
	}
	vh := float64(area.Height) * 4
	return projector{rot: newRotation(cam.RotX, cam.RotY, cam.RotZ), vw: float64(area.Width) * 2, vh: vh, dist: dist, factor: vh * 0.40 * scale}
}

// project returns dot coordinates and camera-space depth.
func (p projector) project(v geom.Vec3) (x, y, z float64, ok bool) {
	r := p.rot.apply(v)
	if r.Z+p.dist <= 0.11 {
		return 0, 0, 0, false
	}
	f := p.factor / (r.Z + p.dist)
	return p.vw/2 + r.X*f, p.vh/2 - r.Y*f, r.Z, true
}

// ScreenPos returns the terminal cell of component i's centre and whether it
// currently faces the camera: its mounting surface turns toward the viewer
// (components inside the body use the direction from the model centre).
func (m *Model) ScreenPos(cam Camera, area cell.Rect, i int) (cx, cy float64, facing bool) {
	if i < 0 || i >= len(m.centroids) {
		return 0, 0, false
	}
	p := newProjector(cam, area)
	x, y, _, ok := p.project(m.centroids[i])
	if !ok {
		return 0, 0, false
	}
	n := m.normals[i]
	if n == (geom.Vec3{}) {
		n = m.centroids[i]
	}
	return float64(area.X) + x/2, float64(area.Y) + y/4, p.rot.apply(n).Z < -0.05
}

// Pick returns the pickable component under terminal cell (mx, my), or -1.
// It rasterises nothing: each front-facing triangle of the current pose is
// tested against sample dots of the cell and the nearest hit wins, so the
// result follows rotation, zoom, animation and viewport size exactly.
func (m *Model) Pick(cam Camera, area cell.Rect, mx, my uint16) int {
	if !area.Contains(mx, my) {
		return -1
	}
	p := newProjector(cam, area)
	n := len(m.verts)
	if cap(m.scratch) < n {
		m.scratch = make([]projected, n)
	}
	pts := m.scratch[:n]
	for i, v := range m.verts {
		x, y, z, ok := p.project(v)
		pts[i] = projected{x, y, z, ok}
	}
	cx, cy := float64(mx-area.X)*2, float64(my-area.Y)*4
	samples := [4][2]float64{{cx + 0.5, cy + 1}, {cx + 1.5, cy + 1}, {cx + 0.5, cy + 3}, {cx + 1.5, cy + 3}}
	best, bestZ := -1, math.Inf(1)
	for f, face := range m.faces {
		for k := 1; k+1 < len(face); k++ {
			a, b, c := pts[face[0]], pts[face[k]], pts[face[k+1]]
			if !a.ok || !b.ok || !c.ok {
				continue
			}
			area2 := (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x)
			if area2 >= 0 { // back-facing (or degenerate), as Viewer3D culls it
				continue
			}
			for _, s := range samples {
				w0 := ((b.x-s[0])*(c.y-s[1]) - (b.y-s[1])*(c.x-s[0])) / area2
				w1 := ((c.x-s[0])*(a.y-s[1]) - (c.y-s[1])*(a.x-s[0])) / area2
				w2 := 1 - w0 - w1
				if w0 < 0 || w1 < 0 || w2 < 0 {
					continue
				}
				if z := w0*a.z + w1*b.z + w2*c.z; z < bestZ {
					bestZ, best = z, f
				}
			}
		}
	}
	if best < 0 {
		return -1
	}
	comp := m.parts[m.faceParts[best]].comp
	if comp < 0 || !m.Spec.Components[comp].Pickable() {
		return -1
	}
	return comp
}

type projected struct {
	x, y, z float64
	ok      bool
}
