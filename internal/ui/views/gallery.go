package views

import (
	"fmt"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/support"
	"github.com/Phobby/TuiSense/internal/ui"
	"github.com/Phobby/TuiSense/internal/ui/visual"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// renderGallery draws the Controller Gallery: a visual preview of every
// registered model, clearly separated from connected hardware.
func renderGallery(f *terminal.Frame, grid ui.DashboardLayout, s *state.AppState, cb Callbacks) {
	specs := visual.Gallery()
	if len(specs) == 0 {
		return
	}
	spec := s.StageSpec()
	renderGalleryTitle(f, grid.TabsArea, s, len(specs))
	uiwidgets.RenderControllerStage(f, grid.Stage3DArea, s, uiwidgets.StageCallbacks{OnSelectComponent: s.SelectComponent})
	renderGalleryInspector(f, grid.InspectorArea, s, spec, cb)
}

func renderGalleryTitle(f *terminal.Frame, area cell.Rect, s *state.AppState, n int) {
	if area.Width < 20 {
		return
	}
	p := s.Theme
	spec := s.StageSpec()
	idx := ((s.Gallery.Index % n) + n) % n
	title := fmt.Sprintf(" %s %s · %d/%d ", spec.Manufacturer, spec.Name, idx+1, n)
	tw := uint16(cell.StringWidth(title))
	total := tw + 6
	x := area.X
	if total < area.Width {
		x += (area.Width - total) / 2
	}
	arrow := func(x uint16, label, id string, delta int) {
		rect := cell.NewRect(x, area.Y, 3, 1)
		f.Buffer.SetStringWithin(x, area.Y, label, cell.Style{Fg: p.FgPrimary, Bg: p.BgSubtle, Modifier: cell.ModifierBold}, 3)
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { s.GalleryStep(delta) })
		f.RegisterAccessibility(accessibility.AccessibilityNode{ID: id, Role: accessibility.RoleButton, Label: i18n.T(label), Bounds: rect})
	}
	arrow(x, " ‹ ", "gallery_prev", -1)
	f.Buffer.SetStringWithin(x+3, area.Y, title, cell.Style{Fg: p.FgPrimary, Bg: p.BgCanvas, Modifier: cell.ModifierBold}, min(tw, area.Width-6))
	arrow(x+3+min(tw, area.Width-6), " › ", "gallery_next", 1)
}

// hardwareSupport resolves the real driver descriptors that render with spec.
func hardwareSupport(s *state.AppState, spec *visual.ModelSpec) []device.Descriptor {
	var out []device.Descriptor
	for _, id := range spec.HardwareModelIDs {
		if d, ok := s.HardwareModels[id]; ok {
			out = append(out, d)
		}
	}
	return out
}

func renderGalleryInspector(f *terminal.Frame, area cell.Rect, s *state.AppState, spec *visual.ModelSpec, cb Callbacks) {
	if area.Width < 20 || area.Height < 3 {
		return
	}
	p := s.Theme
	x, w := area.X+2, area.Width-4
	y := area.Y
	line := func(text string, st cell.Style) {
		if y < area.Y+area.Height {
			f.Buffer.SetStringWithin(x, y, text, st, w)
			y++
		}
	}

	// 1. Preview badge + identity.
	badge := " " + i18n.T("VISUAL PREVIEW") + " "
	bw := uint16(cell.StringWidth(badge))
	f.Buffer.SetStringWithin(x, y, badge, cell.Style{Fg: p.BgCanvas, Bg: p.AccentBlue, Modifier: cell.ModifierBold}, min(bw, w))
	ident := " " + spec.Manufacturer + " " + spec.Name
	if spec.Revision != "" {
		ident += " · " + i18n.T(spec.Revision)
	}
	if bw < w {
		f.Buffer.SetStringWithin(x+bw, y, ident, p.ValueStyle(), w-bw)
	}
	back := " " + i18n.T("Back to controller") + " "
	backW := uint16(cell.StringWidth(back))
	if bw+uint16(cell.StringWidth(ident))+backW+2 < w {
		rect := cell.NewRect(x+w-backW, y, backW, 1)
		f.Buffer.SetStringWithin(rect.X, y, back, cell.Style{Fg: p.FgPrimary, Bg: p.BgSubtle}, backW)
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { s.CloseGallery() })
		f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "gallery_close", Role: accessibility.RoleButton, Label: i18n.T("Back to controller"), Bounds: rect})
	}
	y++

	// 2. What TuiSense can actually do with this controller.
	if row, ok := support.ForVisual(spec.ID); ok && len(hardwareSupport(s, spec)) > 0 {
		var names []string
		for _, n := range row.Implemented() {
			names = append(names, i18n.T(n))
		}
		line(i18n.T("Connected hardware: supported by a TuiSense driver")+" ("+row.Platforms+") · "+strings.Join(names, ", "), cell.Style{Fg: p.Success, Bg: p.BgCanvas})
		if s.Mode == state.ModeAdvanced {
			line(i18n.T("Detection")+": "+row.Detection+" · "+i18n.T(row.Verification), p.MutedStyle())
		}
		// Explore this model's settings without owning it; nothing is sent.
		label := i18n.T("▶ Preview settings")
		if c, ok := s.StageComponent(false); ok && c.Region != device.RegionNone {
			label = i18n.Tf("▶ Preview %s settings", i18n.T(c.Label))
		}
		if y < area.Y+area.Height && cb.OnPreviewModel != nil {
			pillRow(f, x, y, x+w, s, "", 0, []choice{{ID: "gallery_preview", Label: label, OnSelect: cb.OnPreviewModel}})
			hint := "  " + i18n.T("settings only · nothing is sent to a controller")
			used := uint16(cell.StringWidth(label)) + 2
			if used < w {
				f.Buffer.SetStringWithin(x+used, y, hint, p.MutedStyle(), w-used)
			}
			y++
		}
	} else {
		line(i18n.T("Visual preview only — TuiSense cannot communicate with this controller."), cell.Style{Fg: p.Warning, Bg: p.BgCanvas})
	}

	// 3. Selected / hovered component.
	if c, ok := s.StageComponent(true); ok {
		text := i18n.T(c.Label) + " · " + i18n.T(visual.KindName(c.Kind)) + " · " + i18n.T(surfaceName(c.Surface))
		text += " · " + i18n.T(componentSupport(s, spec, c))
		if c.Verify != visual.Verified {
			text += " · " + i18n.T(string(c.Verify))
		}
		line(text, p.ValueStyle())
		if c.Note != "" && (s.Mode == state.ModeAdvanced || c.Verify != visual.Verified) {
			line(i18n.T(c.Note), p.MutedStyle())
		}
	} else {
		line(rearSummary(spec), p.ValueStyle())
	}

	// 4. Contextual hint (gallery only; the main UI has no shortcut bar).
	if s.Mode == state.ModeAdvanced && len(spec.Caveats) > 0 && y < area.Y+area.Height-1 {
		line(i18n.T(spec.Caveats[0]), p.MutedStyle())
	}
	if y < area.Y+area.Height {
		y = area.Y + area.Height - 1
		line(i18n.T("←/→ model · Enter preview settings · V front/rear · drag to rotate · Esc back"), p.MutedStyle())
	}
}

func surfaceName(sf visual.Surface) string {
	switch sf {
	case visual.Rear:
		return "Rear"
	case visual.Top:
		return "Top edge"
	case visual.Bottom:
		return "Bottom edge"
	case visual.Inside:
		return "Body"
	}
	return "Front"
}

// componentSupport states honestly what TuiSense does with a component.
func componentSupport(s *state.AppState, spec *visual.ModelSpec, c visual.Component) string {
	hw := hardwareSupport(s, spec)
	if len(hw) == 0 {
		return "visual only"
	}
	if c.Bound {
		for _, d := range hw {
			if _, ok := d.Inputs.Button(c.Button); ok {
				return "input read by the driver"
			}
		}
		if len(hw[0].Inputs.Buttons) == 0 {
			return "input mapped at connection time"
		}
		return "not reported by the driver"
	}
	if c.Region != device.RegionNone {
		return "settings available when connected"
	}
	return "visual only"
}

func rearSummary(spec *visual.ModelSpec) string {
	rear := spec.RearComponents()
	if len(rear) == 0 {
		return i18n.T("No rear buttons or paddles.")
	}
	labels := make([]string, len(rear))
	for i, c := range rear {
		labels[i] = i18n.T(c.Label)
	}
	return i18n.Tf("Rear controls: %s (press V to view the back)", strings.Join(labels, ", "))
}
