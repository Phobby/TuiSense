// Package visual holds the procedural 2.5D/3D controller models. Each
// controller family registers a Visual keyed by Descriptor.VisualID; the stage
// widget renders whichever visual the active device declares.
package visual

import (
	"math"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/theme"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/graphics"
)

// Context carries everything a visual needs to build a frame's mesh.
type Context struct {
	Palette   theme.Palette
	Accent    cell.Color
	Telemetry device.Telemetry
	Config    device.Config
	Connected bool
	Selected  device.RegionID
	Hovered   device.RegionID
	// Pressed highlights physical buttons (button diagnostics); ShowPressed enables it.
	Pressed     device.ButtonSet
	ShowPressed bool
}

// Anchor is a 3D point used for projected region picking on the stage.
// Note: the Limoni camera looks from -Z toward +Z, so negative Z is the front face.
type Anchor struct {
	Region device.RegionID
	Center graphics.Vertex3D
	Radius float64
}

// Visual builds a controller mesh and exposes pickable anchors.
type Visual interface {
	Name() string
	Build(ctx Context) (graphics.Model3D, []cell.Color)
	Anchors() []Anchor
}

var registry = map[string]Visual{}

// Register adds a visual under id (called from init in each visual file).
func Register(id string, v Visual) { registry[id] = v }

// Lookup returns the visual for id, falling back to the generic gamepad.
func Lookup(id string) Visual {
	if v, ok := registry[id]; ok {
		return v
	}
	return registry["generic"]
}

// faceTag associates a mesh face with its interactive region and material.
type faceTag struct {
	region   device.RegionID
	material string
	button   device.ButtonID
	isButton bool
}

// meshBuilder accumulates parametric primitives into a Model3D.
type meshBuilder struct {
	verts []graphics.Vertex3D
	faces [][]int
	tags  []faceTag
	btn   *device.ButtonID // when set, new faces belong to this physical button
}

// asButton tags every face added by fn with the given button.
func (b *meshBuilder) asButton(id device.ButtonID, fn func()) {
	b.btn = &id
	fn()
	b.btn = nil
}

func (b *meshBuilder) tag(region device.RegionID, mat string) faceTag {
	t := faceTag{region: region, material: mat}
	if b.btn != nil {
		t.button, t.isButton = *b.btn, true
	}
	return t
}

func (b *meshBuilder) transform(v graphics.Vertex3D, cx, cy, cz, tiltX, tiltY, tiltZ float64) graphics.Vertex3D {
	if tiltX != 0 {
		v = v.RotateX(tiltX)
	}
	if tiltY != 0 {
		v = v.RotateY(tiltY)
	}
	if tiltZ != 0 {
		v = v.RotateZ(tiltZ)
	}
	v.X += cx
	v.Y += cy
	v.Z += cz
	return v
}

// box adds an oriented cuboid.
func (b *meshBuilder) box(cx, cy, cz, sx, sy, sz, tiltX, tiltY, tiltZ float64, region device.RegionID, mat string) {
	hx, hy, hz := sx*0.5, sy*0.5, sz*0.5
	local := [8]graphics.Vertex3D{
		{X: -hx, Y: -hy, Z: -hz}, {X: hx, Y: -hy, Z: -hz}, {X: hx, Y: hy, Z: -hz}, {X: -hx, Y: hy, Z: -hz},
		{X: -hx, Y: -hy, Z: hz}, {X: hx, Y: -hy, Z: hz}, {X: hx, Y: hy, Z: hz}, {X: -hx, Y: hy, Z: hz},
	}
	base := len(b.verts)
	for _, v := range local {
		b.verts = append(b.verts, b.transform(v, cx, cy, cz, tiltX, tiltY, tiltZ))
	}
	// Counter-clockwise outward winding (matching graphics.NewCube)
	boxFaces := [6][4]int{{0, 1, 2, 3}, {5, 4, 7, 6}, {1, 5, 6, 2}, {4, 0, 3, 7}, {3, 2, 6, 7}, {4, 5, 1, 0}}
	for _, bf := range boxFaces {
		b.faces = append(b.faces, []int{base + bf[0], base + bf[1], base + bf[2], base + bf[3]})
		b.tags = append(b.tags, b.tag(region, mat))
	}
}

// disk adds an n-sided cylinder along Z (facing -Z) for sticks and round buttons.
func (b *meshBuilder) disk(cx, cy, cz, radius, depth, tiltX, tiltY float64, region device.RegionID, mat string) {
	const segments = 8
	hz := depth * 0.5
	base := len(b.verts)
	for ring := 0; ring < 2; ring++ {
		z := -hz
		if ring == 1 {
			z = hz
		}
		for i := 0; i < segments; i++ {
			ang := float64(i) * (2.0 * math.Pi / float64(segments))
			v := graphics.Vertex3D{X: math.Cos(ang) * radius, Y: math.Sin(ang) * radius, Z: z}
			b.verts = append(b.verts, b.transform(v, cx, cy, cz, tiltX, tiltY, 0))
		}
	}
	front := make([]int, segments)
	back := make([]int, segments)
	for i := 0; i < segments; i++ {
		front[i] = base + i
		back[i] = base + segments + (segments - 1 - i)
	}
	b.faces = append(b.faces, front, back)
	b.tags = append(b.tags, b.tag(region, mat), b.tag(region, mat))
	for i := 0; i < segments; i++ {
		next := (i + 1) % segments
		b.faces = append(b.faces, []int{base + next, base + i, base + segments + i, base + segments + next})
		b.tags = append(b.tags, b.tag(region, mat))
	}
}

// colorize resolves per-face colors from materials, then applies selection/hover tints.
func (b *meshBuilder) colorize(ctx Context, material func(string) cell.Color) []cell.Color {
	out := make([]cell.Color, len(b.faces))
	for i, tag := range b.tags {
		col := material(tag.material)
		if tag.region != device.RegionNone {
			switch tag.region {
			case ctx.Selected:
				col = graphics.InterpolateColor(col, ctx.Accent, 0.62)
			case ctx.Hovered:
				col = graphics.InterpolateColor(col, ctx.Palette.AccentPurple, 0.45)
			}
		}
		if ctx.ShowPressed && tag.isButton && ctx.Pressed.Has(tag.button) {
			col = graphics.InterpolateColor(col, rgb(255, 236, 120), 0.85)
		}
		if !ctx.Connected {
			col = graphics.InterpolateColor(col, ctx.Palette.BgSubtle, 0.55)
		}
		out[i] = col
	}
	return out
}

func (b *meshBuilder) model(name string) graphics.Model3D {
	return graphics.Model3D{Name: name, Vertices: b.verts, Faces: b.faces}
}

func rgb(r, g, bl uint8) cell.Color { return cell.NewColorRGB(r, g, bl) }
