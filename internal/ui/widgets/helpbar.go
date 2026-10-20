package widgets

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// RenderHelpBar draws the bottom keybinding & mouse hint bar, plus an inline
// recoverable error banner if a permission or I/O warning is active.
func RenderHelpBar(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width == 0 || area.Height == 0 {
		return
	}
	p := s.Theme

	// Fill background
	barStyle := cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}
	for y := area.Y; y < area.Y+area.Height; y++ {
		for x := area.X; x < area.X+area.Width; x++ {
			f.Buffer.SetCell(x, y, cell.Cell{Content: ' ', Style: barStyle})
		}
	}

	// If an inline error or permission warning is present and area has 2 rows, show it on row 0
	helpRowY := area.Y
	if area.Height >= 2 {
		helpRowY = area.Y + area.Height - 1
		errY := area.Y
		if s.LastError != "" {
			errStyle := cell.Style{Fg: p.FgPrimary, Bg: p.Danger, Modifier: cell.ModifierBold}
			for x := area.X; x < area.X+area.Width; x++ {
				f.Buffer.SetCell(x, errY, cell.Cell{Content: ' ', Style: errStyle})
			}
			f.Buffer.SetStringWithin(area.X+1, errY, " ⚠ "+s.LastError+i18n.T("  [Click or Esc to dismiss]"), errStyle, area.Width-2)
			f.RegisterClickHandler(cell.NewRect(area.X, errY, area.Width, 1), func(_ driver.MouseEvent) {
				s.ClearError()
			})
		} else if s.DeviceInfo.PermissionDenied && s.DeviceInfo.PermissionHint != "" {
			warnStyle := cell.Style{Fg: p.BgCanvas, Bg: p.Warning, Modifier: cell.ModifierBold}
			for x := area.X; x < area.X+area.Width; x++ {
				f.Buffer.SetCell(x, errY, cell.Cell{Content: ' ', Style: warnStyle})
			}
			f.Buffer.SetStringWithin(area.X+1, errY, " 🔒 "+s.DeviceInfo.PermissionHint, warnStyle, area.Width-2)
		} else {
			// Show latest log line on upper status strip
			if latest, ok := s.Logger.Latest(); ok {
				f.Buffer.SetStringWithin(area.X+1, errY, " ▸ "+latest.Formatted, cell.Style{Fg: p.FgMuted, Bg: p.BgCanvas}, area.Width-2)
			}
		}
	}

	type hintItem struct {
		key  string
		desc string
	}
	hints := []hintItem{
		{i18n.T("Click"), i18n.T("Select Region")},
		{"1-9", i18n.T("Panels")},
		{"C", i18n.T("Controller")},
		{"E", i18n.T("Advanced Mode")},
		{i18n.T("Tab"), i18n.T("Focus")},
		{"←/→", i18n.T("Adjust")},
		{"?", i18n.T("Help")},
		{"Q", i18n.T("Quit")},
	}
	if s.Mode == state.ModeAdvanced {
		hints = []hintItem{
			{i18n.T("Click"), i18n.T("Select Region")},
			{"1-9/0", i18n.T("Panels")},
			{"C / ,.", i18n.T("Controller")},
			{"E", i18n.T("EZ Mode")},
			{i18n.T("Tab"), i18n.T("Focus")},
			{"←/→", i18n.T("Adjust")},
			{i18n.T("WASD"), i18n.T("Orbit")},
			{"T", i18n.T("Test Rumble")},
			{"D/B/U", i18n.T("Simulate")},
			{"?", i18n.T("Help")},
			{"Q", i18n.T("Quit")},
		}
	}

	x := area.X + 1
	limit := area.X + area.Width - 1
	keyStyle := cell.Style{Fg: p.FgPrimary, Bg: p.BgSubtle, Modifier: cell.ModifierBold}
	descStyle := cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}
	sepStyle := cell.Style{Fg: p.FgMuted, Bg: p.BgElevated}

	for i, h := range hints {
		if i > 0 {
			if x+3 >= limit {
				break
			}
			x += f.Buffer.SetStringWithin(x, helpRowY, " │ ", sepStyle, limit-x)
		}
		kText := " " + h.key + " "
		dText := " " + h.desc
		if x+uint16(cell.StringWidth(kText)+cell.StringWidth(dText)) > limit {
			break
		}
		x += f.Buffer.SetStringWithin(x, helpRowY, kText, keyStyle, limit-x)
		x += f.Buffer.SetStringWithin(x, helpRowY, dText, descStyle, limit-x)
	}
}
