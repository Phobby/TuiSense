package widgets

import (
	"fmt"
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
// of TuiSense — with its title, region callouts, picking, and device switcher.
// Everything shown is derived from the active device's Descriptor.
func RenderControllerStage(f *terminal.Frame, area cell.Rect, s *state.AppState, cb StageCallbacks) {
	if area.Width < 20 || area.Height < 10 {
		return
	}
	p := s.Theme
	accent := AccentColor(s.Descriptor)

	// Row 0: centered device title + source tag (click opens the device selector)
	title := s.Descriptor.DisplayName
	tag := sourceTag(s)
	titleW := uint16(cell.StringWidth(title) + cell.StringWidth(tag) + 1)
	tx := area.X
	if titleW < area.Width {
		tx = area.X + (area.Width-titleW)/2
	}
	w := f.Buffer.SetStringWithin(tx, area.Y, title, cell.Style{Fg: p.FgPrimary, Bg: p.BgCanvas, Modifier: cell.ModifierBold}, area.Width-(tx-area.X))
	if tag != "" && tx+w+1 < area.X+area.Width {
		f.Buffer.SetStringWithin(tx+w+1, area.Y, tag, cell.Style{Fg: p.BgCanvas, Bg: accent, Modifier: cell.ModifierBold}, area.X+area.Width-tx-w-1)
	}
	if cb.OnOpenSelector != nil {
		f.RegisterClickHandler(cell.NewRect(tx, area.Y, min(titleW, area.Width), 1), func(_ driver.MouseEvent) { cb.OnOpenSelector() })
	}

	top := area.Y + 1
	if s.Mode == state.ModeAdvanced && area.Height >= 16 {
		renderStageToolbar(f, cell.NewRect(area.X+2, top, area.Width-4, 1), s)
		top++
	}

	// Bottom rows: region description + device switcher
	footerRows := uint16(2)
	viewH := area.Y + area.Height - footerRows - top
	if viewH < 6 {
		footerRows = 0
		viewH = area.Y + area.Height - top
	}
	viewArea := cell.NewRect(area.X+1, top, area.Width-2, viewH)

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
	register3DStageMouse(f, viewArea, s, v.Anchors(), rotX, rotY, rotZ, dist, projScale, cb.OnSelectRegion)
	renderRegionHotspotPills(f, viewArea, s, accent, cb.OnSelectRegion)

	if !s.DeviceInfo.Connected {
		renderEmptyBanner(f, viewArea, s)
	}

	if footerRows == 2 {
		y := area.Y + area.Height - 2
		line := ""
		if r, ok := s.Descriptor.Region(s.SelectedRegion); ok {
			line = "● " + r.Label + " — " + r.Description
		} else if s.DeviceInfo.Connected {
			line = "Select a highlighted region to configure it"
		}
		renderCentered(f, cell.NewRect(area.X+1, y, area.Width-2, 1), line, cell.Style{Fg: p.FgSecondary, Bg: p.BgCanvas})
		renderDeviceSwitcher(f, cell.NewRect(area.X, y+1, area.Width, 1), s, accent, cb)
	}
}

func sourceTag(s *state.AppState) string {
	for _, d := range s.Devices {
		if d.Key == s.ActiveDeviceKey {
			switch d.Source {
			case device.SourceMock:
				return " MOCK "
			case device.SourceTest:
				return " TEST "
			}
		}
	}
	return ""
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
	if viewArea.Height < 8 || viewArea.Width < 36 {
		return
	}
	p := s.Theme
	bw := min(viewArea.Width-6, uint16(50))
	bh := uint16(5)
	bx := viewArea.X + (viewArea.Width-bw)/2
	by := viewArea.Y + (viewArea.Height-bh)/2
	f.RenderWidget(widgets.Block{
		Title:         " NO CONTROLLER CONNECTED ",
		Borders:       widgets.BorderAll,
		BorderSymbols: widgets.SymbolsRounded,
		BorderStyle:   cell.Style{Fg: p.Warning, Bg: p.BgElevated},
		TitleStyle:    cell.Style{Fg: p.Warning, Bg: p.BgElevated, Modifier: cell.ModifierBold},
		Style:         cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated},
	}, cell.NewRect(bx, by, bw, bh))
	f.Buffer.SetStringWithin(bx+2, by+1, "Connect a controller via USB or Bluetooth.", cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated}, bw-4)
	f.Buffer.SetStringWithin(bx+2, by+2, "Hot-plug detection is active.", cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}, bw-4)
	f.Buffer.SetStringWithin(bx+2, by+3, "[D] simulate  ·  [C] choose another device", cell.Style{Fg: p.AccentCyan, Bg: p.BgElevated}, bw-4)
	f.RegisterAccessibility(accessibility.AccessibilityNode{
		ID:     "empty_state_banner",
		Role:   accessibility.RoleGeneric,
		Label:  "No controller connected",
		Bounds: cell.NewRect(bx, by, bw, bh),
	})
}

// renderDeviceSwitcher draws "‹ ◆ DualSense  ◇ Xbox (Test) ›" chips under the visual.
func renderDeviceSwitcher(f *terminal.Frame, area cell.Rect, s *state.AppState, accent cell.Color, cb StageCallbacks) {
	p := s.Theme
	if len(s.Devices) == 0 {
		renderCentered(f, area, "No devices attached", cell.Style{Fg: p.FgMuted, Bg: p.BgCanvas})
		return
	}
	labels := make([]string, len(s.Devices))
	totalW := 4 // arrows
	for i, d := range s.Devices {
		mark := "◇"
		if d.Key == s.ActiveDeviceKey {
			mark = "◆"
		}
		labels[i] = fmt.Sprintf(" %s %s ", mark, d.Descriptor.ShortName)
		totalW += cell.StringWidth(labels[i]) + 1
	}
	x := area.X
	if uint16(totalW) < area.Width {
		x = area.X + (area.Width-uint16(totalW))/2
	}
	limit := area.X + area.Width
	arrowStyle := cell.Style{Fg: p.FgMuted, Bg: p.BgCanvas, Modifier: cell.ModifierBold}

	f.Buffer.SetStringWithin(x, area.Y, "‹ ", arrowStyle, 2)
	if cb.OnCycleDevice != nil {
		f.RegisterClickHandler(cell.NewRect(x, area.Y, 2, 1), func(_ driver.MouseEvent) { cb.OnCycleDevice(-1) })
	}
	x += 2

	for i, d := range s.Devices {
		cw := uint16(cell.StringWidth(labels[i]))
		if x+cw > limit {
			break
		}
		active := d.Key == s.ActiveDeviceKey
		st := cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}
		if active {
			st = cell.Style{Fg: p.BgCanvas, Bg: accent, Modifier: cell.ModifierBold}
		}
		rect := cell.NewRect(x, area.Y, cw, 1)
		f.Buffer.SetStringWithin(x, area.Y, labels[i], st, cw)
		key := d.Key
		if cb.OnSelectDevice != nil {
			f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { cb.OnSelectDevice(key) })
		}
		nodeState := accessibility.NodeState(0)
		if active {
			nodeState |= accessibility.StateSelected
		}
		f.RegisterAccessibility(accessibility.AccessibilityNode{
			ID:     fmt.Sprintf("device_chip_%d", i),
			Role:   accessibility.RoleButton,
			Label:  d.Descriptor.ShortName,
			Value:  d.Descriptor.DisplayName,
			State:  nodeState,
			Bounds: rect,
		})
		x += cw + 1
	}
	if x+2 <= limit {
		f.Buffer.SetStringWithin(x, area.Y, " ›", arrowStyle, 2)
		if cb.OnCycleDevice != nil {
			f.RegisterClickHandler(cell.NewRect(x, area.Y, 2, 1), func(_ driver.MouseEvent) { cb.OnCycleDevice(1) })
		}
	}
}

func renderStageToolbar(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	p := s.Theme
	type item struct {
		text    string
		active  bool
		onClick func()
	}
	markerNames := [3]string{"Braille", "HalfBlock", "Quadrant"}
	items := []item{
		{fmt.Sprintf(" ⟳ Orbit %s ", onOff(s.AutoRotate)), s.AutoRotate, func() { s.AutoRotate = !s.AutoRotate }},
		{fmt.Sprintf(" ◐ %s ", s.ShadingMode), false, func() { CycleShading(s) }},
		{fmt.Sprintf(" ❖ %s ", markerNames[s.RenderMarker%3]), false, func() { s.RenderMarker = (s.RenderMarker + 1) % 3 }},
		{" ⌖ Reset ", false, s.ResetCamera},
	}
	totalW := 0
	for _, it := range items {
		totalW += cell.StringWidth(it.text) + 1
	}
	x := area.X
	if uint16(totalW) < area.Width {
		x = area.X + (area.Width-uint16(totalW))/2
	}
	for _, it := range items {
		w := uint16(cell.StringWidth(it.text))
		if x+w > area.X+area.Width {
			break
		}
		st := cell.Style{Fg: p.FgMuted, Bg: p.BgCanvas}
		if it.active {
			st = cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}
		}
		f.Buffer.SetStringWithin(x, area.Y, it.text, st, w)
		fn := it.onClick
		f.RegisterClickHandler(cell.NewRect(x, area.Y, w, 1), func(_ driver.MouseEvent) { fn() })
		x += w + 1
	}
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// CycleShading advances the stage shading mode.
func CycleShading(s *state.AppState) {
	switch s.ShadingMode {
	case "Lambert":
		s.ShadingMode = "Gouraud"
	case "Gouraud":
		s.ShadingMode = "Flat"
	case "Flat":
		s.ShadingMode = "Wireframe"
	default:
		s.ShadingMode = "Lambert"
	}
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

	f.RegisterClickHandler(viewArea, func(ev driver.MouseEvent) {
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
	})
}

// renderRegionHotspotPills places one callout per declared region at its slot.
func renderRegionHotspotPills(
	f *terminal.Frame,
	viewArea cell.Rect,
	s *state.AppState,
	accent cell.Color,
	onSelectRegion func(device.RegionID),
) {
	if viewArea.Width < 34 || viewArea.Height < 8 {
		return
	}
	p := s.Theme
	left := viewArea.X + 1
	right := viewArea.X + viewArea.Width - 1
	centerX := viewArea.X + viewArea.Width/2
	topY := viewArea.Y
	midY := viewArea.Y + 3
	lowY := viewArea.Y + viewArea.Height - 3
	botY := viewArea.Y + viewArea.Height - 1

	// Track occupied spans per row so callouts never overlap on narrow stages.
	occupied := map[uint16][][2]uint16{}
	fits := func(x, y, w uint16) bool {
		if x < viewArea.X || x+w > viewArea.X+viewArea.Width {
			return false
		}
		for _, span := range occupied[y] {
			if x < span[1]+1 && span[0] < x+w+1 {
				return false
			}
		}
		return true
	}

	for _, region := range s.Descriptor.Regions {
		label := " ◉ " + region.Badge + " "
		w := uint16(cell.StringWidth(label))
		var x, y uint16
		switch region.Slot {
		case device.SlotTopLeft:
			x, y = left, topY
		case device.SlotTopCenter:
			x, y = centerX-w/2, topY
		case device.SlotTopRight:
			x, y = right-w, topY
		case device.SlotMidLeft:
			x, y = left, midY
		case device.SlotMidRight:
			x, y = right-w, midY
		case device.SlotLowLeft:
			x, y = left, lowY
		case device.SlotLowRight:
			x, y = right-w, lowY
		default:
			x, y = centerX-w/2, botY
		}
		if x < viewArea.X {
			x = viewArea.X
		}
		if !fits(x, y, w) {
			continue
		}
		occupied[y] = append(occupied[y], [2]uint16{x, x + w})
		rect := cell.NewRect(x, y, w, 1)

		active := s.SelectedRegion == region.ID
		st := cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}
		switch {
		case active:
			st = cell.Style{Fg: p.BgCanvas, Bg: accent, Modifier: cell.ModifierBold}
		case !s.Caps.Supports(region.Capability):
			st = cell.Style{Fg: p.FgMuted, Bg: p.BgCanvas}
		}
		f.Buffer.SetStringWithin(rect.X, rect.Y, label, st, rect.Width)

		id := region.ID
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) {
			if onSelectRegion != nil {
				onSelectRegion(id)
			}
		})

		nodeState := accessibility.NodeState(0)
		if active {
			nodeState |= accessibility.StateSelected
		}
		f.RegisterAccessibility(accessibility.AccessibilityNode{
			ID:     "region_" + string(region.ID),
			Role:   accessibility.RoleButton,
			Label:  region.Badge,
			Value:  region.Label,
			State:  nodeState,
			Bounds: rect,
		})
	}
}
