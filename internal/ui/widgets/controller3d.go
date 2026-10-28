package widgets

import (
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/ui/visual"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// AccentColor converts a descriptor accent into a terminal color.
func AccentColor(d device.Descriptor) cell.Color {
	return cell.NewColorRGB(d.Accent.R, d.Accent.G, d.Accent.B)
}

// StageCallbacks lets the stage drive selection without knowing about the session.
type StageCallbacks struct {
	OnSelectComponent func(id string)
}

// StagePointerID is the event region covering the 3D stage.
const StagePointerID = "stage_3d_pointer"

const doubleClick = 400 * time.Millisecond

// StageCamera returns the camera the stage draws (and picks) with for a
// viewport, including motion-synced tilt on connected hardware.
func StageCamera(s *state.AppState, m *visual.Model, view cell.Rect) visual.Camera {
	rotX, rotY, rotZ := s.RotX, s.RotY, s.RotZ
	if !s.Gallery.Active && s.DeviceInfo.Connected && s.Caps.Supports(device.CapMotion) && s.Config.Motion.GyroEnabled && s.Config.Motion.Sync3DViewTilt {
		rotX += s.Telemetry.GyroPitch * 0.35
		rotZ += s.Telemetry.GyroRoll * 0.35
	}
	return visual.Camera{RotX: rotX, RotY: rotY, RotZ: rotZ, Distance: visual.DefaultDistance,
		Scale: m.FitScale(view.Width, view.Height, s.Zoom)}
}

// StageView is the viewport of the model inside a stage area (one row is
// kept for the contextual component label).
func StageView(area cell.Rect) cell.Rect {
	return cell.NewRect(area.X+1, area.Y, area.Width-2, area.Height-1)
}

// RenderControllerStage draws the centred 3D controller — the focal point of
// TuiSense — with projection-based picking, hover highlighting, a small
// hover tooltip and a single contextual label. It renders the gallery model
// while previewing and the active device's model otherwise.
func RenderControllerStage(f *terminal.Frame, area cell.Rect, s *state.AppState, cb StageCallbacks) {
	if area.Width < 20 || area.Height < 6 {
		return
	}
	p := s.Theme
	if s.ActiveDeviceKey == "" && !s.Gallery.Active {
		renderEmptyBanner(f, area, s)
		return
	}
	view := StageView(area)
	m := visual.Lookup(s.StageModelID())
	accent := AccentColor(s.Descriptor)
	if s.Gallery.Active {
		accent = p.AccentBlue
	}
	model, colors := m.Frame(frameContext(s, m, accent))
	cam := StageCamera(s, m, view)

	marker := widgets.MarkerBraille
	if s.RenderMarker == 1 {
		marker = widgets.MarkerHalfBlock
	} else if s.RenderMarker == 2 {
		marker = widgets.MarkerQuadrant
	}
	viewer := s.Viewer3D
	if viewer == nil {
		viewer = &widgets.Viewer3D{}
		s.Viewer3D = viewer
	}
	viewer.ID = "stage_3d"
	viewer.Model = model
	viewer.RotX, viewer.RotY, viewer.RotZ = cam.RotX, cam.RotY, cam.RotZ
	viewer.Distance = cam.Distance
	viewer.Scale = cam.Scale
	viewer.Shading = s.ShadingMode
	viewer.Wireframe = s.ShadingMode == "Wireframe"
	viewer.WireframeStyle = cell.Style{Fg: p.AccentBlue, Bg: p.BgCanvas}
	viewer.FocusedStyle = cell.Style{Fg: accent, Bg: p.BgCanvas}
	viewer.FaceColors = colors
	viewer.Marker = marker
	viewer.Light = visual.StageLight
	f.RenderWidget(viewer, view)

	if !s.ShowHelp && !s.ShowDeviceSelector && !s.ShowCopyFallback && !s.ShowQuitConfirm {
		registerStagePointer(f, view, s, m, cam, cb)
		registerComponentNodes(f, view, s, m, cam)
	}
	renderTooltip(f, view, s, m, cam)
	if !s.Gallery.Active { // the gallery inspector already names the selection
		renderComponentLabel(f, cell.NewRect(area.X, area.Y+area.Height-1, area.Width, 1), s)
	}
}

// frameContext derives the per-frame model state. Live input is only shown
// for connected devices and only for buttons their driver reports; gallery
// previews never animate.
func frameContext(s *state.AppState, m *visual.Model, accent cell.Color) visual.FrameContext {
	p := s.Theme
	ctx := visual.FrameContext{
		Selected: m.Index(s.SelectedComponent),
		Hovered:  m.Index(s.HoveredComponent),
		Accent:   accent,
		Hover:    p.AccentPurple,
		Pressing: cell.NewColorRGB(255, 236, 140),
	}
	if s.Gallery.Active {
		return ctx
	}
	ctx.Region = s.SelectedRegion
	connected := s.DeviceInfo.Connected
	ctx.Dim, ctx.DimTo = !connected, p.BgSubtle
	if !connected {
		return ctx
	}
	ctx.Live = true
	ctx.Telemetry = s.Telemetry
	ctx.Pressed = s.Telemetry.Buttons | s.Telemetry.ButtonsLatched
	ctx.Reported = func(id device.ButtonID) bool {
		b, ok := s.Descriptor.Inputs.Button(id)
		return ok && b.Group != "Unmapped"
	}
	cfg := s.Config
	if s.Caps.Supports(device.CapLighting) {
		light := visual.RGB{R: 55, G: 70, B: 95}
		if cfg.Lighting.Enabled && cfg.Lighting.Animation != device.AnimOff {
			light = visual.RGB{R: cfg.Lighting.Red, G: cfg.Lighting.Green, B: cfg.Lighting.Blue}
		}
		ctx.Light = &light
	}
	ctx.MicLED = cfg.Lighting.MicLED != device.MicLEDOff || s.Telemetry.MicMutedButton
	return ctx
}

func registerStagePointer(f *terminal.Frame, view cell.Rect, s *state.AppState, m *visual.Model, cam visual.Camera, cb StageCallbacks) {
	pick := func(x, y uint16) string {
		if c, ok := m.ComponentAt(m.Pick(cam, view, x, y)); ok {
			return c.ID
		}
		return ""
	}
	choose := func(id string) {
		if cb.OnSelectComponent != nil {
			cb.OnSelectComponent(id)
		} else {
			s.SelectComponent(id)
		}
	}
	handle := func(ev driver.MouseEvent) {
		switch ev.Button {
		case driver.MouseNone:
			if !ev.Drag {
				s.HoveredComponent = pick(ev.X, ev.Y)
				s.HoveredRegion = device.RegionNone
				if c, ok := s.StageComponent(true); ok && s.HoveredComponent != "" {
					s.HoveredRegion = c.Region
				}
			}
		case driver.MouseScrollUp:
			s.ZoomBy(0.10)
		case driver.MouseScrollDown:
			s.ZoomBy(-0.10)
		case driver.MouseLeft:
			if ev.Drag {
				return
			}
			if f.FocusManager != nil {
				f.FocusManager.SetFocused("stage_3d")
			}
			now := time.Now()
			last := s.StageClick
			s.StageClick.At, s.StageClick.X, s.StageClick.Y = now, ev.X, ev.Y
			if now.Sub(last.At) < doubleClick && absDiff(last.X, ev.X) <= 1 && absDiff(last.Y, ev.Y) <= 1 {
				s.ResetCamera()
				s.StageClick.At = time.Time{}
				return
			}
			// A press on a component selects it at once; a press on empty
			// space that is released without dragging clears the selection.
			hit := pick(ev.X, ev.Y)
			if hit != "" {
				choose(hit)
			}
			lastX, lastY := int(ev.X), int(ev.Y)
			moved := false
			f.CaptureMouse(func(d driver.MouseEvent) {
				switch {
				case d.Button == driver.MouseRelease:
					if !moved && hit == "" && s.SelectedComponent != "" {
						choose("")
					}
				case d.Drag:
					dx, dy := int(d.X)-lastX, int(d.Y)-lastY
					if dx != 0 || dy != 0 {
						moved = true
						s.HoveredComponent = ""
						s.Orbit(float64(dx)*3.0, float64(dy)*3.0)
						lastX, lastY = int(d.X), int(d.Y)
					}
				}
			})
		}
	}
	f.RegisterEventRegion(terminal.EventRegion{
		ID: StagePointerID, Area: view, Phase: terminal.TargetPhase,
		Handler: func(ctx *terminal.EventContext) { handle(ctx.Mouse) },
		OnLeave: func(_ *terminal.EventContext) {
			s.HoveredComponent = ""
			s.HoveredRegion = device.RegionNone
		},
	})
}

func absDiff(a, b uint16) uint16 {
	if a > b {
		return a - b
	}
	return b - a
}

// registerComponentNodes exposes every visible component to accessibility
// tooling (and UI tests) at its projected position.
func registerComponentNodes(f *terminal.Frame, view cell.Rect, s *state.AppState, m *visual.Model, cam visual.Camera) {
	for i, c := range m.Spec.Components {
		if !c.Pickable() {
			continue
		}
		cx, cy, facing := m.ScreenPos(cam, view, i)
		if !facing || cx < float64(view.X) || cy < float64(view.Y) || cx >= float64(view.X+view.Width) || cy >= float64(view.Y+view.Height) {
			continue
		}
		var st accessibility.NodeState
		if c.ID == s.SelectedComponent {
			st |= accessibility.StateSelected
		}
		f.RegisterAccessibility(accessibility.AccessibilityNode{
			ID: "component_" + c.ID, Role: accessibility.RoleButton, Label: i18n.T(c.Label), State: st,
			Value: i18n.T(visual.KindName(c.Kind)), Bounds: cell.NewRect(uint16(cx), uint16(cy), 1, 1),
		})
	}
}

// renderTooltip shows the hovered component's name just above it.
func renderTooltip(f *terminal.Frame, view cell.Rect, s *state.AppState, m *visual.Model, cam visual.Camera) {
	if s.HoveredComponent == "" || s.HoveredComponent == s.SelectedComponent {
		return
	}
	i := m.Index(s.HoveredComponent)
	c, ok := m.ComponentAt(i)
	if !ok {
		return
	}
	cx, cy, _ := m.ScreenPos(cam, view, i)
	text := " " + i18n.T(c.Label) + " "
	w := uint16(cell.StringWidth(text))
	if w >= view.Width {
		return
	}
	x := int(cx) - int(w)/2
	x = max(int(view.X), min(x, int(view.X+view.Width-w)))
	y := int(cy) - 2
	if y < int(view.Y) {
		y = int(cy) + 2
	}
	if y < int(view.Y) || y >= int(view.Y+view.Height) {
		return
	}
	p := s.Theme
	f.Buffer.SetStringWithin(uint16(x), uint16(y), text, cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated}, w)
}

// renderComponentLabel is the single contextual line under the model: the
// selected component (or capability region) and whether its input is live.
func renderComponentLabel(f *terminal.Frame, row cell.Rect, s *state.AppState) {
	text := ""
	if c, ok := s.StageComponent(false); ok {
		text = i18n.T(c.Label) + " · " + i18n.T(visual.KindName(c.Kind))
		switch {
		case s.ComponentReported(c):
			text += " · " + i18n.T("live input")
		case c.Bound || c.Kind.Rear():
			text += " · " + i18n.T("visual only")
		}
		if s.Mode == state.ModeAdvanced {
			text += " · " + c.ID
		}
	} else if r, ok := s.Descriptor.Region(s.SelectedRegion); ok && !s.Gallery.Active {
		text = i18n.T(r.Label)
	}
	if text != "" {
		renderCentered(f, row, text, s.Theme.MutedStyle())
	}
}

func renderCentered(f *terminal.Frame, area cell.Rect, text string, st cell.Style) {
	w := uint16(cell.StringWidth(text))
	x := area.X
	if w < area.Width {
		x = area.X + (area.Width-w)/2
	}
	f.Buffer.SetStringWithin(x, area.Y, text, st, area.Width-(x-area.X))
}

func renderEmptyBanner(f *terminal.Frame, viewArea cell.Rect, s *state.AppState) {
	y := viewArea.Y + viewArea.Height/2
	renderCentered(f, cell.NewRect(viewArea.X, y, viewArea.Width, 1), i18n.T("No controller connected"), s.Theme.ValueStyle())
	if y+2 < viewArea.Y+viewArea.Height {
		renderCentered(f, cell.NewRect(viewArea.X, y+2, viewArea.Width, 1), i18n.T("Connect a supported controller to get started."), s.Theme.MutedStyle())
	}
	if y+3 < viewArea.Y+viewArea.Height {
		renderCentered(f, cell.NewRect(viewArea.X, y+3, viewArea.Width, 1), i18n.T("Press G to browse the controller gallery."), s.Theme.MutedStyle())
	}
	f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "empty_state_banner", Role: accessibility.RoleGeneric, Label: i18n.T("No controller connected"), Bounds: viewArea})
}
