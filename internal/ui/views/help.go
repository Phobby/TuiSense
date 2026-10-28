package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

type helpLine struct{ key, text string }

func helpLines(s *state.AppState) []helpLine {
	lines := []helpLine{{"", "General navigation"}, {"Tab / Shift+Tab", "Move focus"}, {"Esc", "Close / clear selection / ask to quit"}, {"?", "Open Help"}}
	for _, p := range s.PrimaryPanels() {
		lines = append(lines, helpLine{string(p.Hotkey), p.DisplayTitle(s.Mode)})
	}
	if len(s.Devices) > 0 {
		lines = append(lines, helpLine{"", "Controller selection"}, helpLine{"C", "Open the controller selector"}, helpLine{", / .", "Previous / next controller"})
	}
	if s.ActiveDeviceKey != "" || s.Gallery.Active {
		lines = append(lines, []helpLine{{"", "3D viewer controls"}, {"W A S d / drag", "Orbit camera"}, {"V", "Front / rear view"},
			{"[ / ] / click", "Select component"}, {"+ / - / scroll", "Zoom in / out"}, {"R / double-click", "Reset camera"}, {"Space / M", "Auto-orbit / shading"}}...)
	}
	lines = append(lines, helpLine{"", "Controller Gallery"}, helpLine{"G", "Browse every controller model (visual preview, no hardware)"})
	if s.Gallery.Active {
		lines = append(lines, helpLine{"← / → , / .", "Previous / next model"}, helpLine{"Esc / G", "Back to the connected controller"})
	}
	lines = append(lines, helpLine{"", "Settings and interaction"}, helpLine{"E", "Toggle EZ ↔ Advanced mode"}, helpLine{"Arrows / Enter", "Adjust focused control / activate"})
	if state.PanelVisible(s.VisiblePanels(), state.PanelFirmware) {
		lines = append(lines, helpLine{"F", "Device details & firmware check (read-only)"})
	}
	if state.PanelVisible(s.VisiblePanels(), state.PanelProfiles) {
		lines = append(lines, helpLine{"", "Profiles"}, helpLine{"P", "Open Profiles"})
	}
	if s.ActivePanel == state.PanelDiagnostics {
		lines = append(lines, []helpLine{{"", "Diagnostics"}, {"h / l", "Previous / next test category"}, {"o / g / n / x", "Start test / guided check / next step / stop all"}, {"PgUp / PgDn", "Scroll details"}, {"", "Copying information"}, {"y / Y", "Copy current test / all diagnostics"}}...)
	} else {
		lines = append(lines, helpLine{"", "Copying information"}, helpLine{"y / Y", "Device report (safe / full)"})
	}
	if s.Caps.Supports(device.CapHaptics) {
		lines = append(lines, helpLine{"T", "Test vibration"})
	}
	lines = append(lines, helpLine{"PgUp / PgDn", "Scroll details"})
	if s.IsSimulatedDevice() {
		lines = append(lines, helpLine{"", "Developer simulation"}, helpLine{"D / B / U", "Simulate hot-plug / transport / permission"})
	}
	return append(lines, helpLine{"", "Application controls"}, helpLine{"Q", "Quit"}, helpLine{"Esc / ?", "Close Help"}, helpLine{"↑ ↓ / PgUp PgDn", "Scroll help"})
}

func renderHelpModal(f *terminal.Frame, total cell.Rect, s *state.AppState) {
	if total.Width < 24 || total.Height < 8 {
		return
	}
	w, h := min(total.Width-2, uint16(84)), min(total.Height-2, uint16(32))
	area := cell.NewRect(total.X+(total.Width-w)/2, total.Y+(total.Height-h)/2, w, h)
	f.RegisterModal("help_modal", area, func() { s.ShowHelp = false })
	f.BeginLayer("help_modal")
	defer f.EndLayer()
	f.RenderWidget(widgets.Block{Title: " " + i18n.T("Help") + " ", Borders: widgets.BorderAll, BorderSymbols: widgets.SymbolsRounded, Style: s.Theme.PanelStyle(), BorderStyle: s.Theme.CardBorder()}, area)
	// Wrap descriptions so every implemented binding is readable even at 80x24.
	var rows []helpLine
	keyW := min(22, int(w)/3)
	for _, line := range helpLines(s) {
		text := i18n.T(line.text)
		available := int(w) - keyW - 5
		if line.key == "" {
			rows = append(rows, helpLine{"", text})
			continue
		}
		key := line.key
		var part string
		for _, r := range text {
			if cell.StringWidth(part+string(r)) > available {
				rows = append(rows, helpLine{key, part})
				key = " "
				part = ""
			}
			part += string(r)
		}
		rows = append(rows, helpLine{key, part})
	}
	visible := int(h) - 4
	s.HelpScrollMax = max(0, len(rows)-visible)
	s.HelpScroll = min(max(0, s.HelpScroll), s.HelpScrollMax)
	for i := 0; i < visible && i+s.HelpScroll < len(rows); i++ {
		line := rows[i+s.HelpScroll]
		y := area.Y + 2 + uint16(i)
		if line.key == "" {
			f.Buffer.SetStringWithin(area.X+2, y, line.text, s.Theme.SectionHeaderStyle(), w-4)
		} else {
			f.Buffer.SetStringWithin(area.X+2, y, line.key, s.Theme.ValueStyle(), uint16(keyW))
			f.Buffer.SetStringWithin(area.X+uint16(keyW)+3, y, line.text, s.Theme.LabelStyle(), w-uint16(keyW)-5)
		}
	}
	f.Buffer.SetStringWithin(area.X+2, area.Y+h-2, fmt.Sprintf("↑ ↓  PgUp/PgDn   %d/%d   Esc", s.HelpScroll+1, s.HelpScrollMax+1), s.Theme.MutedStyle(), w-4)
	f.RegisterClickHandler(area, func(ev driver.MouseEvent) {
		switch ev.Button {
		case driver.MouseScrollUp:
			s.HelpScroll = max(0, s.HelpScroll-3)
		case driver.MouseScrollDown:
			s.HelpScroll = min(s.HelpScrollMax, s.HelpScroll+3)
		}
	})
}
