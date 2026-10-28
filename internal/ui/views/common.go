package views

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// panelFrame draws a calm inspector card and returns its padded inner area.
func panelFrame(f *terminal.Frame, area cell.Rect, s *state.AppState, title string) cell.Rect {
	if area.Width < 5 || area.Height < 3 {
		return cell.Rect{}
	}
	f.Buffer.SetStringWithin(area.X+2, area.Y, title, s.Theme.SectionHeaderStyle(), area.Width-4)
	// Settings panels state what their controls change: the controller,
	// software input processing, or only a preview inside TuiSense.
	if scope := scopeLabel(s); scope != "" {
		tw := uint16(cell.StringWidth(title))
		if area.Width > tw+8 {
			f.Buffer.SetStringWithin(area.X+2+tw+2, area.Y, scope, s.Theme.MutedStyle(), area.Width-tw-6)
		}
	}
	return cell.NewRect(area.X+2, area.Y+2, area.Width-4, area.Height-2)
}

// scopeLabel describes the effect of the active settings panel's controls.
func scopeLabel(s *state.AppState) string {
	var c device.Capability
	for _, spec := range state.PanelCatalog {
		if spec.ID == s.ActivePanel && spec.Access != 0 {
			c = spec.Capability
		}
	}
	if c == "" || s.Gallery.Active {
		return ""
	}
	if s.IsPreviewDevice() {
		return i18n.T("· preview — not sent to any controller")
	}
	scope, ok := device.SettingScope(s.Caps, c)
	if !ok {
		return ""
	}
	switch scope {
	case device.ScopeDevice:
		if s.Caps.Spec(c).Persistent {
			return i18n.T("· saved on the controller")
		}
		return i18n.T("· applied to the controller (re-applied on connect)")
	case device.ScopeSoftware:
		return i18n.T("· software input processing")
	case device.ScopePreview:
		return i18n.T("· preview only — games are not affected")
	}
	return ""
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
		copyValue(f, cell.NewRect(inner.X, y, inner.Width, 1), s, msg, msg, cell.Style{Fg: p.Warning, Bg: p.BgPanel})
	default:
		copyValue(f, cell.NewRect(inner.X, y, inner.Width, 1), s, "✕ "+reason, reason, cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold})
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
			if s.Copy.Actions == nil {
				s.Copy.Actions = make(map[string]func())
			}
			s.Copy.Actions[it.ID] = it.OnSelect
			if f.FocusManager != nil {
				f.FocusManager.Register(it.ID)
				f.FocusManager.RegisterBounds(it.ID, rect)
			}
			if f.IsFocused(it.ID) {
				f.Buffer.SetStringWithin(x, y, lbl, p.ButtonFocusedStyle(), w)
			}
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

// registerControl gives compact custom controls the same keyboard and mouse action.
func registerControl(f *terminal.Frame, rect cell.Rect, s *state.AppState, id, label string, fn func(driver.MouseEvent)) {
	if s.Copy.Actions == nil {
		s.Copy.Actions = make(map[string]func())
	}
	s.Copy.Actions[id] = func() { fn(driver.MouseEvent{Button: driver.MouseLeft, X: rect.X, Y: rect.Y}) }
	if f.FocusManager != nil {
		f.FocusManager.Register(id)
		f.FocusManager.RegisterBounds(id, rect)
	}
	f.RegisterClickHandler(rect, func(ev driver.MouseEvent) {
		if ev.Button == driver.MouseLeft {
			fn(ev)
		}
	})
	f.RegisterAccessibility(accessibility.AccessibilityNode{ID: id, Role: accessibility.RoleButton, Label: label, Bounds: rect})
	if f.IsFocused(id) {
		for x := rect.X; x < rect.X+rect.Width; x++ {
			v := f.Buffer.CellAt(x, rect.Y)
			v.Style.Modifier |= cell.ModifierUnderline
			f.Buffer.SetCell(x, rect.Y, v)
		}
	}
}
