package visual

import (
	"image"
	"image/color"
	"math"

	"github.com/Phobby/TuiSense/internal/ui/geom"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/graphics"
)

// StageLight is the light the stage renders with; previews use the same one.
var StageLight = graphics.Light{
	Direction: graphics.Vector3D{X: 0.30, Y: 0.55, Z: -0.90}.Normalize(),
	Ambient:   0.46,
	Diffuse:   0.56,
}

// RenderPreview rasterises a model into a w×h image with the stage's
// projection and Lambert lighting. Contributors use it (through
// `go test ./internal/ui/visual -run Preview` with TUISENSE_MODEL_PREVIEWS
// set) to check a new model at full resolution without a terminal.
func RenderPreview(m *Model, cam Camera, ctx FrameContext, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{18, 20, 26, 255}
	for i := range img.Pix {
		img.Pix[i] = [4]uint8{bg.R, bg.G, bg.B, bg.A}[i%4]
	}
	model, colors := m.Frame(ctx)
	// A w×h image is the dot grid of a (w/2)×(h/4) cell viewport.
	p := newProjector(cam, cell.NewRect(0, 0, uint16(w/2), uint16(h/4)))
	depth := make([]float64, w*h)
	for i := range depth {
		depth[i] = math.Inf(1)
	}
	type pt struct{ x, y, z float64 }
	pts := make([]pt, len(model.Vertices))
	rot := make([]geom.Vec3, len(model.Vertices))
	for i, v := range model.Vertices {
		x, y, z, _ := p.project(v)
		pts[i] = pt{x, y, z}
		rot[i] = p.rot.apply(v)
	}
	for f, face := range model.Faces {
		for k := 1; k+1 < len(face); k++ {
			a, b, c := pts[face[0]], pts[face[k]], pts[face[k+1]]
			area := (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x)
			if area >= 0 {
				continue
			}
			n := geom.OutwardNormal(rot[face[0]], rot[face[k]], rot[face[k+1]])
			shade := StageLight.CalculateIntensity(graphics.Vector3D{X: n.X, Y: n.Y, Z: n.Z})
			col := graphics.ApplyShade(colors[f], shade)
			r, g, bl := rgbOf(col)
			minX, maxX := int(math.Max(0, math.Floor(math.Min(a.x, math.Min(b.x, c.x))))), int(math.Min(float64(w-1), math.Ceil(math.Max(a.x, math.Max(b.x, c.x)))))
			minY, maxY := int(math.Max(0, math.Floor(math.Min(a.y, math.Min(b.y, c.y))))), int(math.Min(float64(h-1), math.Ceil(math.Max(a.y, math.Max(b.y, c.y)))))
			for y := minY; y <= maxY; y++ {
				for x := minX; x <= maxX; x++ {
					sx, sy := float64(x)+0.5, float64(y)+0.5
					w0 := ((b.x-sx)*(c.y-sy) - (b.y-sy)*(c.x-sx)) / area
					w1 := ((c.x-sx)*(a.y-sy) - (c.y-sy)*(a.x-sx)) / area
					w2 := 1 - w0 - w1
					if w0 < 0 || w1 < 0 || w2 < 0 {
						continue
					}
					z := w0*a.z + w1*b.z + w2*c.z
					if z >= depth[y*w+x] {
						continue
					}
					depth[y*w+x] = z
					img.SetRGBA(x, y, color.RGBA{r, g, bl, 255})
				}
			}
		}
	}
	return img
}

func rgbOf(c cell.Color) (uint8, uint8, uint8) {
	if c.Type() == cell.ColorRGB {
		return uint8(c >> 16), uint8(c >> 8), uint8(c)
	}
	return 128, 128, 128
}
