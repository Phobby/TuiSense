package visual

import (
	"math"
	"sort"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/thebanri/limoni/core/cell"
)

var wantGallery = []string{
	"dualsense", "dualsense_edge", "xbox_series", "xbox_elite2", "dualshock4",
	"switch_pro", "8bitdo_ultimate_bt", "gamesir_g7se", "razer_wolverine_v3pro", "msi_gc20",
}

func camAt(m *Model, p CameraPreset, view cell.Rect) Camera {
	return Camera{RotX: p.RotX, RotY: p.RotY, Distance: DefaultDistance, Scale: m.FitScale(view.Width, view.Height, 1)}
}

func TestGalleryHasTenModelsThatBuild(t *testing.T) {
	g := Gallery()
	if len(g) != len(wantGallery) {
		t.Fatalf("gallery has %d models, want %d", len(g), len(wantGallery))
	}
	for i, spec := range g {
		if spec.ID != wantGallery[i] {
			t.Errorf("gallery[%d] = %s, want %s", i, spec.ID, wantGallery[i])
		}
		m := Lookup(spec.ID)
		if m.Spec != spec {
			t.Errorf("%s: lookup returned another spec", spec.ID)
		}
		// Low-poly but real geometry: enough detail to read, cheap to raster.
		if m.Faces() < 400 || m.Faces() > 4000 {
			t.Errorf("%s: %d faces outside the terminal budget", spec.ID, m.Faces())
		}
		if spec.Manufacturer == "" || spec.Name == "" || spec.Revision == "" || spec.Attribution == "" || spec.Source != proceduralSource {
			t.Errorf("%s: incomplete descriptor metadata", spec.ID)
		}
		for _, f := range m.faces {
			for _, v := range f {
				if v < 0 || v >= m.Vertices() {
					t.Fatalf("%s: face index out of range", spec.ID)
				}
			}
		}
		seen := map[string]bool{}
		for i, c := range spec.Components {
			if c.ID == "" || seen[c.ID] {
				t.Errorf("%s: empty or duplicate component id %q", spec.ID, c.ID)
			}
			seen[c.ID] = true
			if c.Pickable() && (c.Label == "" || c.Verify == "") {
				t.Errorf("%s/%s: interactive components need a label and verification status", spec.ID, c.ID)
			}
			if c.Pickable() && len(c.Shapes) > 0 && m.FacesOf(i) == 0 {
				t.Errorf("%s/%s: component generated no geometry", spec.ID, c.ID)
			}
		}
	}
	if Lookup("no-such-model").Spec.ID != GenericID {
		t.Fatal("unknown visual IDs must fall back to the generic gamepad")
	}
}

func TestModelsAreCached(t *testing.T) {
	if Lookup("dualsense") != Lookup("dualsense") {
		t.Fatal("geometry must be generated once and cached")
	}
}

// silhouette renders a coarse front mask of a model.
func silhouette(m *Model) []bool {
	const w, h = 160, 100
	img := RenderPreview(m, camAt(m, CameraPreset{}, cell.NewRect(0, 0, w/2, h/4)), FrameContext{Selected: -1, Hovered: -1}, w, h)
	mask := make([]bool, w*h)
	for i := range mask {
		px := img.Pix[i*4 : i*4+3]
		mask[i] = px[0] != 18 || px[1] != 20 || px[2] != 26
	}
	return mask
}

// Every pair of models must differ visibly, not only in colour: their
// front silhouettes and/or their rear controls and component layouts.
func TestModelsAreGeometricallyDistinct(t *testing.T) {
	type sig struct {
		mask   []bool
		layout map[string]bool
	}
	sigs := map[string]sig{}
	for _, id := range wantGallery {
		m := Lookup(id)
		layout := map[string]bool{}
		for _, c := range m.Spec.Components {
			layout[c.ID] = true
		}
		sigs[id] = sig{silhouette(m), layout}
	}
	for i, a := range wantGallery {
		for _, b := range wantGallery[i+1:] {
			sa, sb := sigs[a], sigs[b]
			diff := 0
			for k := range sa.mask {
				if sa.mask[k] != sb.mask[k] {
					diff++
				}
			}
			layoutDiff := 0
			for id := range sa.layout {
				if !sb.layout[id] {
					layoutDiff++
				}
			}
			for id := range sb.layout {
				if !sa.layout[id] {
					layoutDiff++
				}
			}
			if diff < 120 && layoutDiff < 2 {
				t.Errorf("%s and %s are not distinguishable (silhouette diff %d px, layout diff %d)", a, b, diff, layoutDiff)
			}
		}
	}
}

func rearLabels(id string) []string {
	var out []string
	for _, c := range Lookup(id).Spec.RearComponents() {
		out = append(out, c.Label)
	}
	sort.Strings(out)
	return out
}

func TestRearControlsOnlyWhereTheHardwareHasThem(t *testing.T) {
	want := map[string][]string{
		"dualsense":             nil,
		"dualsense_edge":        {"Left back button", "Right back button"},
		"xbox_series":           nil,
		"xbox_elite2":           {"P1", "P2", "P3", "P4"},
		"dualshock4":            nil,
		"switch_pro":            nil,
		"8bitdo_ultimate_bt":    {"Left back paddle", "Right back paddle"},
		"gamesir_g7se":          {"L4", "R4"},
		"razer_wolverine_v3pro": {"Lower left back button", "Lower right back button", "Upper left back button", "Upper right back button"},
		"msi_gc20":              nil,
	}
	for id, labels := range want {
		got := rearLabels(id)
		if len(got) != len(labels) {
			t.Errorf("%s: rear controls %v, want %v", id, got, labels)
			continue
		}
		for i := range got {
			if got[i] != labels[i] {
				t.Errorf("%s: rear controls %v, want %v", id, got, labels)
			}
		}
	}
	// Rear inputs are bound only where a driver can report them one by one:
	// the DualSense Edge (hidraw protocol) and the Elite Series 2 (xpad paddles).
	for _, id := range wantGallery {
		for _, c := range Lookup(id).Spec.RearComponents() {
			if c.Bound != (id == "dualsense_edge" || id == "xbox_elite2") {
				t.Errorf("%s/%s: rear input binding %v is not backed by a driver", id, c.ID, c.Bound)
			}
		}
	}
}

// Rear controls must sit on the back (+Z) and front controls on the front.
func TestComponentsSitOnTheirSurfaces(t *testing.T) {
	for _, id := range wantGallery {
		m := Lookup(id)
		for i, c := range m.Spec.Components {
			if len(c.Shapes) == 0 || !c.Pickable() {
				continue
			}
			z := m.Centroid(i).Z
			switch {
			case c.Surface == Rear && z <= 0:
				t.Errorf("%s/%s: rear control at z=%.2f is not on the back", id, c.ID, z)
			case c.Surface == Front && z >= 0:
				t.Errorf("%s/%s: front control at z=%.2f is not on the front", id, c.ID, z)
			}
			x := m.Centroid(i).X
			if (c.Side == Left && x >= 0) || (c.Side == Right && x <= 0) {
				t.Errorf("%s/%s: %s-side component at x=%.2f", id, c.ID, c.Side, x)
			}
		}
	}
}

// pickRate projects every facing component of a view and checks that
// picking at its centre returns it.
func pickRate(t *testing.T, m *Model, cam Camera, view cell.Rect, filter func(Component) bool) (hit, total int) {
	t.Helper()
	m.Frame(FrameContext{Selected: -1, Hovered: -1})
	for i, c := range m.Spec.Components {
		if !c.Pickable() || len(c.Shapes) == 0 || !filter(c) {
			continue
		}
		cx, cy, facing := m.ScreenPos(cam, view, i)
		if !facing || cx < float64(view.X) || cy < float64(view.Y) {
			continue
		}
		total++
		if m.Pick(cam, view, uint16(cx), uint16(cy)) == i {
			hit++
		}
	}
	return
}

func TestPickingFollowsRotationZoomAndViewport(t *testing.T) {
	for _, id := range wantGallery {
		m := Lookup(id)
		for _, view := range []cell.Rect{cell.NewRect(0, 0, 200, 56), cell.NewRect(10, 4, 150, 40)} {
			front := camAt(m, m.Spec.Front, view)
			hit, total := pickRate(t, m, front, view, func(c Component) bool { return c.Surface == Front })
			if total == 0 || float64(hit) < 0.8*float64(total) {
				t.Errorf("%s %v front: picked %d/%d components at their centres", id, view, hit, total)
			}
			// Zoomed in and rotated to the back: rear controls stay pickable.
			rear := camAt(m, m.Spec.Rear, view)
			rear.Scale *= 1.3
			hit, total = pickRate(t, m, rear, view, func(c Component) bool { return c.Kind.Rear() })
			if hit != total {
				t.Errorf("%s %v rear: picked %d/%d rear controls", id, view, hit, total)
			}
		}
	}
}

// From behind, the player's left controls appear on the right of the screen
// — the model is rotated, never mirrored — and keep their own identity.
func TestRearViewDoesNotMirrorOrSwapSides(t *testing.T) {
	view := cell.NewRect(0, 0, 200, 56)
	mid := float64(view.Width) / 2
	for _, id := range []string{"dualsense_edge", "xbox_elite2", "gamesir_g7se", "razer_wolverine_v3pro", "8bitdo_ultimate_bt"} {
		m := Lookup(id)
		rear := camAt(m, CameraPreset{RotX: 0, RotY: 180}, view)
		front := camAt(m, CameraPreset{}, view)
		for i, c := range m.Spec.Components {
			if c.Side == Center || !c.Pickable() || len(c.Shapes) == 0 {
				continue
			}
			fx, _, _ := m.ScreenPos(front, view, i)
			rx, _, facing := m.ScreenPos(rear, view, i)
			if (c.Side == Left) != (fx < mid) {
				t.Errorf("%s/%s: front view puts a %s control on the wrong side", id, c.ID, c.Side)
			}
			if c.Kind.Rear() && facing && (c.Side == Left) != (rx > mid) {
				t.Errorf("%s/%s: rear view mirrored a %s control", id, c.ID, c.Side)
			}
		}
	}
}

func TestFramingCentresAndFitsEveryModel(t *testing.T) {
	for _, id := range append(wantGallery, GenericID) {
		m := Lookup(id)
		for _, view := range []cell.Rect{cell.NewRect(0, 0, 76, 14), cell.NewRect(0, 0, 118, 26), cell.NewRect(0, 0, 218, 50)} {
			for _, p := range []CameraPreset{m.Spec.Front, m.Spec.Rear} {
				cam := camAt(m, p, view)
				pr := newProjector(cam, view)
				minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
				for _, v := range m.rest {
					x, y, _, _ := pr.project(v)
					minX, maxX = math.Min(minX, x), math.Max(maxX, x)
					minY, maxY = math.Min(minY, y), math.Max(maxY, y)
				}
				if minX < 0 || minY < 0 || maxX > pr.vw || maxY > pr.vh {
					t.Errorf("%s %dx%d: model leaves the viewport (%.0f..%.0f × %.0f..%.0f of %.0f×%.0f)", id, view.Width, view.Height, minX, maxX, minY, maxY, pr.vw, pr.vh)
				}
				if (maxX-minX) < 0.6*pr.vw && (maxY-minY) < 0.6*pr.vh {
					t.Errorf("%s %dx%d: model is not prominent", id, view.Width, view.Height)
				}
				if c := (minX + maxX) / 2; math.Abs(c-pr.vw/2) > 0.08*pr.vw {
					t.Errorf("%s %dx%d: model is off-centre (%.0f vs %.0f)", id, view.Width, view.Height, c, pr.vw/2)
				}
			}
		}
	}
}

func TestLiveInputHighlightsOnlyReportedButtons(t *testing.T) {
	m := Lookup("dualsense_edge")
	base := FrameContext{Selected: -1, Hovered: -1, Pressing: cell.NewColorRGB(255, 236, 140)}
	_, idle := m.Frame(base)
	idleCopy := append([]cell.Color(nil), idle...)
	idleVerts := append(m.verts[:0:0], m.verts...)

	ctx := base
	ctx.Pressed = device.ButtonSet(0).With(device.ButtonLeftPaddle)
	ctx.Reported = func(device.ButtonID) bool { return false }
	_, cols := m.Frame(ctx)
	for i := range cols {
		if cols[i] != idleCopy[i] {
			t.Fatal("an input the driver does not report must not light up")
		}
	}
	ctx.Reported = func(device.ButtonID) bool { return true }
	_, cols = m.Frame(ctx)
	paddle := m.Index("back_left")
	lit, other := 0, 0
	for f, p := range m.faceParts {
		if cols[f] != idleCopy[f] {
			if m.parts[p].comp == paddle {
				lit++
			} else {
				other++
			}
		}
	}
	if lit == 0 || other != 0 {
		t.Fatalf("pressing the left back button lit %d of its faces and %d others", lit, other)
	}
	moved := false
	for i := range m.verts {
		moved = moved || m.verts[i] != idleVerts[i]
	}
	if !moved {
		t.Fatal("a pressed button should depress slightly")
	}

	ctx = base
	ctx.Live = true
	ctx.Telemetry.LeftStickX, ctx.Telemetry.L2Pressure = 1, 1
	m.Frame(ctx)
	ls := m.Index("left_stick")
	lt := m.Index("left_trigger")
	movedStick, movedTrigger := false, false
	for _, p := range m.parts {
		for v := p.first; v < p.end; v++ {
			if m.verts[v] != idleVerts[v] {
				movedStick = movedStick || p.comp == ls
				movedTrigger = movedTrigger || p.comp == lt
			}
		}
	}
	if !movedStick || !movedTrigger {
		t.Fatalf("live telemetry must pose the stick (%v) and trigger (%v)", movedStick, movedTrigger)
	}
}

func TestFrameDoesNotAllocate(t *testing.T) {
	m := Lookup("xbox_elite2")
	ctx := FrameContext{Selected: 3, Hovered: 5, Live: true}
	ctx.Telemetry.RightStickY = -0.7
	allocs := testing.AllocsPerRun(50, func() { m.Frame(ctx) })
	if allocs > 0 {
		t.Fatalf("Frame allocates %.0f times per call", allocs)
	}
}

// Every user-visible model string must be translated (the UI is bilingual).
func TestModelTextIsTranslated(t *testing.T) {
	var texts []string
	for _, k := range kindNames {
		texts = append(texts, k)
	}
	for _, id := range IDs() {
		s := Lookup(id).Spec
		texts = append(texts, s.Revision) // product names stay untranslated
		texts = append(texts, s.Caveats...)
		for _, c := range s.Components {
			if c.Pickable() {
				texts = append(texts, c.Label, c.Note)
			}
		}
	}
	for _, s := range texts {
		if s != "" && hasWord(s) && !i18n.Has(i18n.Turkish, s) {
			t.Errorf("no Turkish translation for %q", s)
		}
	}
}

func hasWord(s string) bool {
	n := 0
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			n++
			if n >= 2 {
				return true
			}
		} else {
			n = 0
		}
	}
	return false
}

func BenchmarkFrame(b *testing.B) {
	m := Lookup("dualsense_edge")
	ctx := FrameContext{Selected: 3, Hovered: 5, Live: true}
	ctx.Telemetry.LeftStickX = 0.5
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Frame(ctx)
	}
}

func BenchmarkPick(b *testing.B) {
	m := Lookup("xbox_elite2")
	view := cell.NewRect(0, 0, 160, 40)
	cam := camAt(m, m.Spec.Rear, view)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Pick(cam, view, 70, 22)
	}
}
