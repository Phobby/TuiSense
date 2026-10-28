package state

import (
	"math"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/ui/visual"
)

// GalleryState is the optional Controller Gallery: a visual preview of every
// registered model, fully separate from connected hardware. It never adds a
// device to s.Devices and never changes the active device.
type GalleryState struct {
	Active bool
	Index  int // into visual.Gallery()
}

// StageModelID is the visual rendered on the stage: the gallery selection
// while previewing, otherwise the active device's visual.
func (s *AppState) StageModelID() string {
	if s.Gallery.Active {
		if g := visual.Gallery(); len(g) > 0 {
			return g[((s.Gallery.Index%len(g))+len(g))%len(g)].ID
		}
	}
	return s.Descriptor.VisualID
}

// StageSpec returns the spec of the stage model.
func (s *AppState) StageSpec() *visual.ModelSpec { return visual.Lookup(s.StageModelID()).Spec }

// StageComponent returns the selected (or, with hovered, the hovered) component.
func (s *AppState) StageComponent(hovered bool) (visual.Component, bool) {
	id := s.SelectedComponent
	if hovered && s.HoveredComponent != "" {
		id = s.HoveredComponent
	}
	if id == "" {
		return visual.Component{}, false
	}
	return s.StageSpec().Component(id)
}

// ComponentReported reports whether the active driver delivers c's input.
// Gallery previews never report input.
func (s *AppState) ComponentReported(c visual.Component) bool {
	if s.Gallery.Active || !c.Bound {
		return false
	}
	b, ok := s.Descriptor.Inputs.Button(c.Button)
	return ok && b.Group != "Unmapped"
}

// SelectComponent selects a semantic component. On connected hardware a
// component linked to a declared capability region opens that region's
// controls (or, in Diagnostics, its test), exactly like selecting the region.
func (s *AppState) SelectComponent(id string) {
	s.SelectedComponent = id
	if s.Gallery.Active || id == "" {
		return
	}
	c, ok := s.StageSpec().Component(id)
	if !ok {
		return
	}
	if c.Region != device.RegionNone {
		if _, declared := s.Descriptor.Region(c.Region); declared {
			s.SelectRegion(c.Region)
			return
		}
	}
	s.SelectedRegion = device.RegionNone
	s.RegionHint = ""
	if s.ActivePanel == PanelDiagnostics && s.Diag != nil && s.ComponentReported(c) {
		if a, ok := s.Diag.Avail[diag.CatButtons]; ok && a.Status != diag.Unsupported {
			s.Diag.Select(diag.CatButtons)
		}
	}
}

// CycleComponent moves the selection through the stage model's pickable
// components (keyboard access to everything the mouse can pick).
func (s *AppState) CycleComponent(delta int) {
	var ids []string
	for _, c := range s.StageSpec().Components {
		if c.Pickable() {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	idx := -1
	for i, id := range ids {
		if id == s.SelectedComponent {
			idx = i
		}
	}
	if idx < 0 && delta < 0 {
		idx = 0
	}
	next := ((idx+delta)%len(ids) + len(ids)) % len(ids)
	s.SelectComponent(ids[next])
}

// ViewingRear reports whether the camera currently looks at the back.
func (s *AppState) ViewingRear() bool {
	return math.Cos((s.RotY)*math.Pi/180) < 0
}

// SetView animates the camera to the stage model's front or rear preset.
// The model is rotated, never mirrored, so left/right controls keep their
// physical side (a left paddle appears on screen right from behind).
func (s *AppState) SetView(rear bool) {
	spec := s.StageSpec()
	p := spec.Front
	if rear {
		p = spec.Rear
	}
	s.AutoRotate = false
	s.TargetRotX = p.RotX
	// Approach along the shortest arc from the current angle.
	cur := s.RotY
	t := p.RotY
	for t-cur > 180 {
		t -= 360
	}
	for t-cur < -180 {
		t += 360
	}
	s.TargetRotY = t
	s.CameraTween = true
}

// ToggleView flips between the front and rear presets.
func (s *AppState) ToggleView() { s.SetView(!s.ViewingRear()) }

// StepCamera advances a preset transition with exponential easing.
func (s *AppState) StepCamera(dt float64) {
	if !s.CameraTween {
		return
	}
	k := 1 - math.Exp(-dt*9)
	s.RotX += (s.TargetRotX - s.RotX) * k
	s.RotY += (s.TargetRotY - s.RotY) * k
	if math.Abs(s.TargetRotX-s.RotX) < 0.25 && math.Abs(s.TargetRotY-s.RotY) < 0.25 {
		s.RotX, s.RotY = s.TargetRotX, math.Mod(s.TargetRotY+720, 360)
		s.CameraTween = false
	}
}

// Orbit rotates the camera directly (drag / keys), cancelling any transition.
func (s *AppState) Orbit(dx, dy float64) {
	s.AutoRotate = false
	s.CameraTween = false
	s.RotY = math.Mod(s.RotY+dx+360, 360)
	s.RotX = math.Max(-75, math.Min(75, s.RotX+dy))
}

// ZoomBy changes the zoom within the supported range.
func (s *AppState) ZoomBy(d float64) { s.Zoom = math.Max(0.55, math.Min(1.9, s.Zoom+d)) }

// OpenGallery enters the Controller Gallery, starting at the active
// device's model when it has one.
func (s *AppState) OpenGallery() {
	g := visual.Gallery()
	s.Gallery.Index = 0
	for i, spec := range g {
		if spec.ID == s.Descriptor.VisualID {
			s.Gallery.Index = i
		}
	}
	s.Gallery.Active = true
	s.ShowDeviceSelector = false
	s.SelectedComponent, s.HoveredComponent = "", ""
	s.ResetCamera()
}

// CloseGallery returns to the connected controller immediately.
func (s *AppState) CloseGallery() {
	s.Gallery.Active = false
	s.SelectedComponent, s.HoveredComponent = "", ""
	s.ResetCamera()
}

// GalleryStep shows the previous/next gallery model.
func (s *AppState) GalleryStep(delta int) {
	n := len(visual.Gallery())
	if n == 0 {
		return
	}
	s.Gallery.Index = ((s.Gallery.Index+delta)%n + n) % n
	s.SelectedComponent, s.HoveredComponent = "", ""
	s.ResetCamera()
}

// ResetCamera restores the stage model's front preset and default zoom.
func (s *AppState) ResetCamera() {
	p := s.StageSpec().Front
	s.RotX, s.RotY, s.RotZ = p.RotX, p.RotY, 0
	s.TargetRotX, s.TargetRotY = p.RotX, p.RotY
	s.CameraTween = false
	s.Zoom = 1.0
	s.ShowcasePhase = 0.0
}
