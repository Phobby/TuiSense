package visual

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebanri/limoni/core/cell"
)

// TestPreviewImages writes front and rear PNG previews of every model when
// TUISENSE_MODEL_PREVIEWS names a directory (a contributor tool; skipped otherwise).
func TestPreviewImages(t *testing.T) {
	dir := os.Getenv("TUISENSE_MODEL_PREVIEWS")
	if dir == "" {
		t.Skip("set TUISENSE_MODEL_PREVIEWS=<dir> to export model previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := FrameContext{Selected: -1, Hovered: -1}
	for _, id := range IDs() {
		m := Lookup(id)
		for _, view := range []struct {
			name string
			p    CameraPreset
		}{{"front", m.Spec.Front}, {"rear", m.Spec.Rear}, {"side", CameraPreset{RotX: -10, RotY: 90}}, {"toprear", CameraPreset{RotX: -55, RotY: 160}}} {
			const w, h = 640, 400
			cam := Camera{RotX: view.p.RotX, RotY: view.p.RotY, Distance: DefaultDistance, Scale: m.FitScale(w/2, h/4, 1)}
			img := RenderPreview(m, cam, ctx, w, h)
			f, err := os.Create(filepath.Join(dir, id+"-"+view.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
	_ = cell.Rect{}
}
