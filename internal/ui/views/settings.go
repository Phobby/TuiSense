package views

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/theme"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// RenderSettingsPanel renders application preferences, 3D rendering options,
// accessibility theme switching, and Mock Hardware Simulator controls.
func RenderSettingsPanel(f *terminal.Frame, area cell.Rect, s *state.AppState, sim Callbacks) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, i18n.T("Settings"))
	y := inner.Y

	y = renderModeChooser(f, inner, y, s, sim)
	y = renderLanguageChooser(f, inner, y, s, sim)
	y++

	// 1. 3D Stage & Visual Preferences
	f.Buffer.SetStringWithin(inner.X, y, i18n.T("3D Viewport & Accessibility"), p.SectionHeaderStyle(), inner.Width)
	y++

	f.RenderWidget(widgets.Checkbox{
		ID:           "cb_settings_autorot",
		Checked:      &s.AutoRotate,
		Label:        i18n.T("Continuous 3D Auto-Orbit"),
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}, cell.NewRect(inner.X, y, inner.Width, 1))
	y++

	// High Contrast Theme Toggle
	prevHC := s.HighContrast
	f.RenderWidget(widgets.Checkbox{
		ID:           "cb_settings_high_contrast",
		Checked:      &s.HighContrast,
		Label:        i18n.T("High-Contrast Accessibility Theme"),
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}, cell.NewRect(inner.X, y, inner.Width, 1))
	if s.HighContrast != prevHC {
		if s.HighContrast {
			s.Theme = theme.HighContrast()
		} else {
			s.Theme = theme.Default()
		}
	}
	y += 2

	// 3D Shading Mode Selector
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.T("3D Shading:"), p.MutedStyle(), 12)
		sx := inner.X + 12
		for _, mode := range []string{"Gouraud", "Lambert", "Flat", "Wireframe"} {
			lbl := " " + mode + " "
			mw := uint16(cell.StringWidth(lbl))
			if sx+mw > inner.X+inner.Width {
				break
			}
			mRect := cell.NewRect(sx, y, mw, 1)
			f.Buffer.SetStringWithin(sx, y, lbl, p.ButtonStyle(s.ShadingMode == mode), mw)
			target := mode
			f.RegisterClickHandler(mRect, func(_ driver.MouseEvent) {
				s.ShadingMode = target
			})
			sx += mw + 1
		}
		y++
	}

	// Sub-Cell Marker Resolution Selector
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.T("3D Marker:"), p.MutedStyle(), 12)
		mx := inner.X + 12
		markers := []struct {
			idx  int
			name string
		}{
			{0, i18n.T("Braille (2x4 Sub-Cell)")},
			{1, i18n.T("Half-Block (1x2 Solid)")},
			{2, i18n.T("Quadrant (2x2 Block)")},
		}
		for _, m := range markers {
			lbl := " " + m.name + " "
			mw := uint16(cell.StringWidth(lbl))
			if mx+mw > inner.X+inner.Width {
				break
			}
			mRect := cell.NewRect(mx, y, mw, 1)
			f.Buffer.SetStringWithin(mx, y, lbl, p.ButtonStyle(s.RenderMarker == m.idx), mw)
			targetIdx := m.idx
			f.RegisterClickHandler(mRect, func(_ driver.MouseEvent) {
				s.RenderMarker = targetIdx
			})
			mx += mw + 1
		}
		y += 2
	}

	// 2. Interactive Hardware Simulator (for testing hot-plug, transport, and udev permissions)
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.T("Simulator (mock & test devices)"), p.SectionHeaderStyle(), inner.Width)
		y++
	}

	if y < inner.Y+inner.Height {
		btnHotplug := widgets.Button{
			ID:           "btn_sim_hotplug",
			Label:        i18n.T("🔌 Toggle Connect / Disconnect [D]"),
			Style:        p.ButtonStyle(s.DeviceInfo.Connected),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress:      sim.OnToggleConnect,
		}
		w, _ := btnHotplug.SizeHint(inner)
		uiwidgets.RenderButton(f, cell.NewRect(inner.X, y, w, 1), btnHotplug)
		y++
	}

	if y < inner.Y+inner.Height {
		btnTrans := widgets.Button{
			ID:           "btn_sim_transport",
			Label:        i18n.T("ᛒ Switch USB-C ↔ Bluetooth [B]"),
			Style:        p.ButtonStyle(false),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress:      sim.OnCycleTransport,
		}
		w, _ := btnTrans.SizeHint(inner)
		uiwidgets.RenderButton(f, cell.NewRect(inner.X, y, w, 1), btnTrans)
		y++
	}

	if y < inner.Y+inner.Height {
		btnPerm := widgets.Button{
			ID:           "btn_sim_permission",
			Label:        i18n.T("🔒 Simulate udev Permission Error [U]"),
			Style:        p.ButtonStyle(s.DeviceInfo.PermissionDenied),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress:      sim.OnTogglePermission,
		}
		w, _ := btnPerm.SizeHint(inner)
		uiwidgets.RenderButton(f, cell.NewRect(inner.X, y, w, 1), btnPerm)
	}
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
	pillRow(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Mode:"), 7, []choice{
		{ID: "settings_mode_ez", Label: i18n.T("EZ · simple"), Selected: s.Mode == state.ModeEZ, OnSelect: set(state.ModeEZ)},
		{ID: "settings_mode_advanced", Label: i18n.T("Advanced · everything"), Selected: s.Mode == state.ModeAdvanced, OnSelect: set(state.ModeAdvanced)},
	})
	return y + 1
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
		items = append(items, choice{ID: "settings_lang_" + string(l), Label: l.Name(), Selected: s.Language == l, OnSelect: func() {
			if cb.OnSetLanguage != nil {
				cb.OnSetLanguage(l)
			} else {
				s.Language = l
				i18n.Set(l)
			}
		}})
	}
	pillRow(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Language:"), 11, items)
	return y + 1
}
