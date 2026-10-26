package widgets

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"math"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/ui/visual"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/graphics"
	"github.com/thebanri/limoni/widgets"
)

// AccentColor converts a descriptor accent into a terminal color.
func AccentColor(d device.Descriptor) cell.Color {
	return cell.NewColorRGB(d.Accent.R, d.Accent.G, d.Accent.B)
}

// StageCallbacks lets the stage drive selection without knowing about the session.
type StageCallbacks struct {
	OnSelectRegion func(device.RegionID)
	OnSelectDevice func(key string)
	OnCycleDevice  func(delta int)
	OnOpenSelector func()
}

// RenderControllerStage draws the centered controller visual — the focal point
// of TuiSense — with projected picking and a single contextual region label.
// Everything shown is derived from the active device's Descriptor.
func RenderControllerStage(f *terminal.Frame, area cell.Rect, s *state.AppState, cb StageCallbacks) {
	if area.Width < 20 || area.Height < 6 {
		return
	}
	p := s.Theme
	accent := AccentColor(s.Descriptor)

	if s.ActiveDeviceKey == "" {
		renderEmptyBanner(f, area, s)
		return
	}
	viewArea := cell.NewRect(area.X+1, area.Y, area.Width-2, area.Height-1)
	v := visual.Lookup(s.Descriptor.VisualID)
	model, faceColors := v.Build(visual.Context{
		Palette:   p,
		Accent:    accent,
		Telemetry: s.Telemetry,
		Config:    s.Config,
		Connected: s.DeviceInfo.Connected,
		Selected:  s.SelectedRegion,
		Hovered:   s.HoveredRegion,
		// In diagnostics, physical buttons light up while pressed.
		Pressed:     s.Telemetry.Buttons | s.Telemetry.ButtonsLatched,
		ShowPressed: s.ActivePanel == state.PanelDiagnostics,
	})

	rotX, rotY, rotZ := s.RotX, s.RotY, s.RotZ
	if s.DeviceInfo.Connected && s.Caps.Supports(device.CapMotion) && s.Config.Motion.GyroEnabled && s.Config.Motion.Sync3DViewTilt {
		rotX += s.Telemetry.GyroPitch * 0.35
		rotZ += s.Telemetry.GyroRoll * 0.35
	}

	// Viewer3D scales by viewport height. Empirically the model spans ~62% of
	// the height and ~0.57·H cells of width at scale 1.55, so cap the scale by
	// both axes: tall stages stay height-bound, wide short stages grow.
	fit := math.Min(1.95, 0.85*float64(viewArea.Width)/float64(max(viewArea.Height, 1)))
	viewerScale := math.Max(0.9, fit) * s.Zoom
	const dist = 3.1

	marker := widgets.MarkerBraille
	if s.RenderMarker == 1 {
		marker = widgets.MarkerHalfBlock
	} else if s.RenderMarker == 2 {
		marker = widgets.MarkerQuadrant
	}

	viewer := s.Viewer3D
	if viewer == nil {
		viewer = &widgets.Viewer3D{ID: "stage_3d"}
		s.Viewer3D = viewer
	}
	viewer.ID = "stage_3d"
	viewer.Model = model
	viewer.RotX, viewer.RotY, viewer.RotZ = rotX, rotY, rotZ
	viewer.Distance = dist
	viewer.Scale = viewerScale
	viewer.Shading = s.ShadingMode
	viewer.Wireframe = s.ShadingMode == "Wireframe"
	viewer.WireframeStyle = cell.Style{Fg: p.AccentBlue, Bg: p.BgCanvas}
	viewer.FocusedStyle = cell.Style{Fg: accent, Bg: p.BgCanvas}
	viewer.FaceColors = faceColors
	viewer.Marker = marker
	viewer.Light = graphics.Light{
		Direction: graphics.Vector3D{X: 0.30, Y: 0.55, Z: -0.90}.Normalize(),
		Ambient:   0.48,
		Diffuse:   0.52,
	}
	f.RenderWidget(viewer, viewArea)

	// Must match Viewer3D's internal projection so mouse picking is 1:1.
	projScale := float64(viewArea.Height) * 4.0 * 0.40 * viewerScale
	if !s.ShowHelp && !s.ShowDeviceSelector && !s.ShowCopyFallback {
		register3DStageMouse(f, viewArea, s, v.Anchors(), rotX, rotY, rotZ, dist, projScale, cb.OnSelectRegion)
	}

	region := s.SelectedRegion
	if s.HoveredRegion != device.RegionNone {
		region = s.HoveredRegion
	}
	if r, ok := s.Descriptor.Region(region); ok {
		renderCentered(f, cell.NewRect(area.X, area.Y+area.Height-1, area.Width, 1), i18n.T(r.Label), p.MutedStyle())
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
	f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "empty_state_banner", Role: accessibility.RoleGeneric, Label: i18n.T("No controller connected"), Bounds: viewArea})
}

func register3DStageMouse(
	f *terminal.Frame,
	viewArea cell.Rect,
	s *state.AppState,
	anchors []visual.Anchor,
	rotX, rotY, rotZ, dist, projScale float64,
	onSelectRegion func(device.RegionID),
) {
	virtW := float64(viewArea.Width) * 2.0
	virtH := float64(viewArea.Height) * 4.0

	// Project anchors with the same RotateY -> RotateX -> RotateZ order as Viewer3D.
	type projAnchor struct {
		region       device.RegionID
		cellX, cellY float64
		radius       float64
	}
	var projected []projAnchor
	for _, a := range anchors {
		if _, declared := s.Descriptor.Region(a.Region); !declared {
			continue
		}
		v := a.Center.RotateY(rotY).RotateX(rotX).RotateZ(rotZ)
		px, py, vis := graphics.Project(v, virtW, virtH, dist, projScale)
		if !vis {
			continue
		}
		projected = append(projected, projAnchor{
			region: a.Region,
			cellX:  float64(viewArea.X) + px/2.0,
			cellY:  float64(viewArea.Y) + py/4.0,
			radius: math.Max(4.0, (a.Radius*projScale/(v.Z+dist))/2.5),
		})
	}

	pickRegionAt := func(mx, my uint16) device.RegionID {
		best := device.RegionNone
		bestScore := 1e9
		fx, fy := float64(mx), float64(my)
		for _, pa := range projected {
			d := math.Hypot(fx-pa.cellX, (fy-pa.cellY)*1.8) // terminal cell aspect correction
			if d <= pa.radius && d < bestScore {
				bestScore = d
				best = pa.region
			}
		}
		return best
	}

	var dragStartX, dragStartY int
	var movedDuringDrag bool

	handlePointer := func(ev driver.MouseEvent) {
		switch ev.Button {
		case driver.MouseScrollUp:
			s.Zoom = math.Min(1.75, s.Zoom+0.10)
		case driver.MouseScrollDown:
			s.Zoom = math.Max(0.55, s.Zoom-0.10)
		case driver.MouseLeft:
			if ev.Drag {
				return
			}
			if f.FocusManager != nil {
				f.FocusManager.SetFocused("stage_3d")
			}
			dragStartX, dragStartY = int(ev.X), int(ev.Y)
			movedDuringDrag = false
			if r := pickRegionAt(ev.X, ev.Y); r != device.RegionNone && onSelectRegion != nil {
				onSelectRegion(r)
			}
			lastX, lastY := int(ev.X), int(ev.Y)
			f.CaptureMouse(func(dragEv driver.MouseEvent) {
				if dragEv.Button == driver.MouseRelease {
					if !movedDuringDrag {
						if r := pickRegionAt(uint16(dragStartX), uint16(dragStartY)); r != device.RegionNone && onSelectRegion != nil {
							onSelectRegion(r)
						}
					}
					return
				}
				if dragEv.Drag {
					dx := int(dragEv.X) - lastX
					dy := int(dragEv.Y) - lastY
					if dx != 0 || dy != 0 {
						movedDuringDrag = true
						s.AutoRotate = false
						s.RotY = math.Mod(s.RotY+float64(dx)*2.0+360.0, 360.0)
						s.RotX = math.Max(-75.0, math.Min(75.0, s.RotX+float64(dy)*2.0))
						lastX, lastY = int(dragEv.X), int(dragEv.Y)
					}
				}
			})
		}
	}
	f.RegisterClickHandler(viewArea, handlePointer)
	for _, anchor := range projected {
		x := max(int(viewArea.X), int(anchor.cellX)-2)
		y := max(int(viewArea.Y), int(anchor.cellY)-1)
		rect := cell.NewRect(uint16(x), uint16(y), 5, 3).Intersection(viewArea)
		region := anchor.region
		f.RegisterEventRegion(terminal.EventRegion{ID: "hover_" + string(region), Area: rect, Phase: terminal.TargetPhase,
			OnEnter: func(_ *terminal.EventContext) { s.HoveredRegion = region },
			OnLeave: func(_ *terminal.EventContext) { s.HoveredRegion = device.RegionNone },
			Handler: func(ctx *terminal.EventContext) { handlePointer(ctx.Mouse) },
		})
	}

}
