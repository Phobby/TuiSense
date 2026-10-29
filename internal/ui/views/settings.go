package views

import (
	"fmt"
	"strings"

	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/theme"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
)

// renderSettings is the Settings page in both modes: General and updates on
// the left, the 3D view (and, for a simulated device, the simulator) on the
// right, stacked in one column when the panel is narrow. Advanced adds the
// renderer options.
func renderSettings(f *terminal.Frame, area cell.Rect, pc PanelContext, advanced bool) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	s := pc.S
	inner := panelFrame(f, area, s, i18n.T("Settings"))
	lc, rc := splitColumns(newCursor(f, inner, s), s, 120)
	left, right := lc.r, rc.r

	y := sectionHeader(f, s, left, left.Y, i18n.T("General"))
	y = renderModeChooser(f, left, y+1, s, pc.CB) // +1: the frame's top edge
	y = renderLanguageChooser(f, left, y, s, pc.CB)
	y = renderUpdates(f, left, y, s, pc.CB, advanced)

	if right.X != left.X {
		y = right.Y
	}
	limit := right.X + right.Width
	y = sectionHeader(f, s, right, y, i18n.T("3D view & accessibility"))
	y++ // the frame's top edge
	pillRow(f, right.X, y, limit, s, "", 0, []choice{{
		ID: "btn_open_gallery", Label: i18n.T("Controller Gallery (visual preview)"), OnSelect: s.OpenGallery,
	}})
	y += pillRowH
	orbit, contrast := i18n.T("Slowly rotate the controller"), i18n.T("High-contrast colors")
	if advanced {
		orbit, contrast = i18n.T("Continuous 3D Auto-Orbit"), i18n.T("High-Contrast Accessibility Theme")
	}
	y = pillRows(f, right.X, y, limit, s, "", 0, []choice{
		{ID: "cb_settings_autorot", Kind: kindToggle, Label: orbit, Selected: s.AutoRotate, OnSelect: func() { s.AutoRotate = !s.AutoRotate }},
		{ID: "cb_settings_high_contrast", Kind: kindToggle, Label: contrast, Selected: s.HighContrast, OnSelect: func() {
			s.HighContrast = !s.HighContrast
			if s.HighContrast {
				s.Theme = theme.HighContrast()
			} else {
				s.Theme = theme.Default()
			}
			pc.prefsChanged()
		}},
	})
	if !advanced {
		return
	}

	var shading, markers []choice
	for _, mode := range []string{"Gouraud", "Lambert", "Flat", "Wireframe"} {
		target := mode
		shading = append(shading, choice{Kind: kindOption, ID: "shading_" + strings.ToLower(mode), Label: mode, Selected: s.ShadingMode == mode, OnSelect: func() { s.ShadingMode = target }})
	}
	for i, name := range []string{i18n.T("Braille (2x4 Sub-Cell)"), i18n.T("Half-Block (1x2 Solid)"), i18n.T("Quadrant (2x2 Block)")} {
		target := i
		markers = append(markers, choice{Kind: kindOption, ID: fmt.Sprintf("marker_%d", i), Label: name, Selected: s.RenderMarker == i, OnSelect: func() { s.RenderMarker = target }})
	}
	y = pillRows(f, right.X, y, limit, s, i18n.T("3D Shading:"), 12, shading)
	y = pillRows(f, right.X, y, limit, s, i18n.T("3D Marker:"), 12, markers)

	// Simulation controls only belong to an explicitly selected virtual device.
	if !s.IsSimulatedDevice() {
		return
	}
	y = sectionHeader(f, s, right, y, i18n.T("Simulator (mock & test devices)"))
	pillRows(f, right.X, y+1, limit, s, "", 0, []choice{
		{ID: "btn_sim_hotplug", Label: i18n.T("Toggle Connect / Disconnect [D]"), OnSelect: pc.CB.OnToggleConnect},
		{ID: "btn_sim_transport", Label: i18n.T("ᛒ Switch USB-C ↔ Bluetooth [B]"), OnSelect: pc.CB.OnCycleTransport},
		{ID: "btn_sim_permission", Label: i18n.T("Simulate udev Permission Error [U]"), OnSelect: pc.CB.OnTogglePermission},
	})
}

// renderModeChooser draws the EZ / Advanced segmented choice used by both settings variants.
func renderModeChooser(f *terminal.Frame, inner cell.Rect, y uint16, s *state.AppState, cb Callbacks) uint16 {
	if y >= inner.Y+inner.Height {
		return y
	}
	set := func(m state.UXMode) func() {
		return func() {
			if cb.OnSetMode != nil {
				cb.OnSetMode(m)
			} else {
				s.SetMode(m)
			}
		}
	}
	pillRow(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Mode:"), 11, []choice{ // the same column as Language
		{Kind: kindOption, ID: "settings_mode_ez", Label: i18n.T("EZ · simple"), Selected: s.Mode == state.ModeEZ, OnSelect: set(state.ModeEZ)},
		{Kind: kindOption, ID: "settings_mode_advanced", Label: i18n.T("Advanced · everything"), Selected: s.Mode == state.ModeAdvanced, OnSelect: set(state.ModeAdvanced)},
	})
	return y + pillRowH
}

// renderLanguageChooser draws the clickable UI language switch (both modes).
// Language names are always shown in their own language.
func renderLanguageChooser(f *terminal.Frame, inner cell.Rect, y uint16, s *state.AppState, cb Callbacks) uint16 {
	if y >= inner.Y+inner.Height {
		return y
	}
	var items []choice
	for _, l := range i18n.Languages {
		l := l
		items = append(items, choice{Kind: kindOption, ID: "settings_lang_" + string(l), Label: l.Name(), Selected: s.Language == l, OnSelect: func() {
			if cb.OnSetLanguage != nil {
				cb.OnSetLanguage(l)
			} else {
				s.Language = l
				i18n.Set(l)
			}
		}})
	}
	pillRow(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Language:"), 11, items)
	return y + pillRowH
}
