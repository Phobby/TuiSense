package views

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// panelFrame draws a calm inspector card and returns its padded inner area.
func panelFrame(f *terminal.Frame, area cell.Rect, s *state.AppState, title string) cell.Rect {
	p := s.Theme
	f.RenderWidget(widgets.Block{
		Title:         " " + title + " ",
		TitleStyle:    cell.Style{Fg: uiwidgets.AccentColor(s.Descriptor), Bg: p.BgPanel, Modifier: cell.ModifierBold},
		Borders:       widgets.BorderAll,
		BorderSymbols: widgets.SymbolsRounded,
		BorderStyle:   p.CardBorder(),
		Style:         p.PanelStyle(),
	}, area)
	return cell.NewRect(area.X+2, area.Y+1, area.Width-4, area.Height-2)
}

// supportBanner prints a one-line notice when capability c is limited or
// unavailable on the active device, returning the next free row.
func supportBanner(f *terminal.Frame, inner cell.Rect, y uint16, s *state.AppState, c device.Capability) uint16 {
	if y >= inner.Y+inner.Height {
		return y
	}
	p := s.Theme
	level, reason := device.Availability(s.DeviceInfo, s.Caps, c)
	switch {
	case level == device.Full:
		return y
	case level == device.Limited:
		msg := i18n.T("◐ Limited support")
		if reason != "" {
			msg += " — " + reason
		}
		f.Buffer.SetStringWithin(inner.X, y, msg, cell.Style{Fg: p.Warning, Bg: p.BgPanel}, inner.Width)
	default:
		f.Buffer.SetStringWithin(inner.X, y, "✕ "+reason, cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold}, inner.Width)
	}
	f.RegisterAccessibility(accessibility.AccessibilityNode{
		ID:     "support_banner_" + string(c),
		Role:   accessibility.RoleGeneric,
		Label:  i18n.T("Support notice"),
		Value:  reason,
		Bounds: cell.NewRect(inner.X, y, inner.Width, 1),
	})
	return y + 1
}

// choice is one option in a pill row.
type choice struct {
	ID       string
	Label    string
	Selected bool
	OnSelect func()
}

// pillRow renders a horizontal row of selectable pills after an optional
// label and returns the x after the last pill drawn.
func pillRow(f *terminal.Frame, x, y, limit uint16, s *state.AppState, label string, labelW uint16, items []choice) uint16 {
	p := s.Theme
	if label != "" && uint16(cell.StringWidth(label)+1) > labelW {
		labelW = uint16(cell.StringWidth(label) + 1) // translated labels may be longer
	}
	if labelW > 0 {
		if label != "" {
			f.Buffer.SetStringWithin(x, y, label, p.MutedStyle(), labelW)
		}
		x += labelW
	}
	for _, it := range items {
		lbl := " " + it.Label + " "
		w := uint16(cell.StringWidth(lbl))
		if x+w > limit {
			break
		}
		rect := cell.NewRect(x, y, w, 1)
		f.Buffer.SetStringWithin(x, y, lbl, p.ButtonStyle(it.Selected), w)
		if it.OnSelect != nil {
			fn := it.OnSelect
			f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { fn() })
		}
		if it.ID != "" {
			st := accessibility.NodeState(0)
			if it.Selected {
				st |= accessibility.StateSelected
			}
			f.RegisterAccessibility(accessibility.AccessibilityNode{
				ID: it.ID, Role: accessibility.RoleButton, Label: strings.TrimSpace(it.Label), State: st, Bounds: rect,
			})
		}
		x += w + 1
	}
	return x
}

func nonEmptyStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
