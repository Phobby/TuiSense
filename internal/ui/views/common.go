package views

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"strings"
	"time"

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

// sliderColumn sizes the label column of a group of slider or gauge rows: the
// widest label, a value of valueW columns, and a gap before the track, so every
// track in the group starts in the same column whatever the language.
func sliderColumn(valueW int, labels ...string) uint16 {
	w := 0
	for _, l := range labels {
		w = max(w, cell.StringWidth(l))
	}
	return uint16(w + 1 + valueW + 2)
}

// sliderLabel draws a row's label with its value right-aligned to the column.
func sliderLabel(f *terminal.Frame, r cell.Rect, s *state.AppState, label, value string) {
	pad := max(1, int(r.Width)-2-cell.StringWidth(label)-cell.StringWidth(value))
	copyValue(f, r, s, label+strings.Repeat(" ", pad)+value, value, s.Theme.LabelStyle())
}

// choiceKind says what a button does, and so how it looks.
type choiceKind int

const (
	kindAction choiceKind = iota // does something: │ Check now │
	kindOption                   // one of a group; the chosen one is filled
	kindToggle                   // on or off: │ [x] Pre-releases │
)

// choice is one button in a pill row.
type choice struct {
	ID       string
	Label    string
	Kind     choiceKind
	Selected bool       // the chosen option, a toggle that is on, or a highlighted action
	Disabled bool       // drawn dimmed, not clickable or focusable
	Fg       cell.Color // optional label colour, e.g. a colour swatch
	OnSelect func()
}

// Buttons are rounded frames three rows tall, centred on the label row:
//
//	╭──────────╮
//	│ Raw Logs │   ← y
//	╰──────────╯
//
// so a caller places them by their label row and leaves the rows above and
// below free; stacked button rows are pillRowH apart.
const pillRowH = 3

// buttonLabel is the text inside a button's frame; a toggle shows its state.
func buttonLabel(it choice) string {
	if it.Kind != kindToggle {
		return it.Label
	}
	if it.Selected {
		return "[x] " + it.Label
	}
	return "[ ] " + it.Label
}

// pillWidth is the width of one button frame.
func pillWidth(it choice) uint16 { return uint16(cell.StringWidth(buttonLabel(it)) + 4) }

// pillRow renders a horizontal row of buttons after an optional label and
// returns the x after the last button drawn.
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
		w := pillWidth(it)
		if x+w > limit {
			break
		}
		rect := cell.NewRect(x, y, w, 1)
		if y > 0 {
			rect = cell.NewRect(x, y-1, w, pillRowH)
		}
		focused := it.OnSelect != nil && !it.Disabled && f.IsFocused(it.ID)
		drawButton(f, s, x, y, w, it, focused)
		if it.OnSelect != nil && !it.Disabled {
			if s.Copy.Actions == nil {
				s.Copy.Actions = make(map[string]func())
			}
			s.Copy.Actions[it.ID] = it.OnSelect
			if f.FocusManager != nil {
				f.FocusManager.Register(it.ID)
				f.FocusManager.RegisterBounds(it.ID, rect)
			}
			fn := it.OnSelect
			f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { fn() })
		}
		if it.ID != "" {
			st := accessibility.NodeState(0)
			if it.Selected {
				st |= accessibility.StateSelected
			}
			if it.Disabled {
				st |= accessibility.StateDisabled
			}
			f.RegisterAccessibility(accessibility.AccessibilityNode{
				ID: it.ID, Role: accessibility.RoleButton, Label: strings.TrimSpace(it.Label), State: st, Bounds: rect,
			})
		}
		x += w + 1
	}
	return x
}

// pillRows lays items out like pillRow, wrapping onto further button rows
// aligned after the label, and returns the label row after the last one.
func pillRows(f *terminal.Frame, x, y, limit uint16, s *state.AppState, label string, labelW uint16, items []choice) uint16 {
	if label != "" && uint16(cell.StringWidth(label)+1) > labelW {
		labelW = uint16(cell.StringWidth(label) + 1)
	}
	for first := true; len(items) > 0; first = false {
		bx, n := x+labelW, 0
		for n < len(items) {
			w := pillWidth(items[n])
			if bx+w > limit && n > 0 {
				break
			}
			bx += w + 1
			n++
		}
		lbl := ""
		if first {
			lbl = label
		}
		pillRow(f, x, y, limit, s, lbl, labelW, items[:n])
		items = items[n:]
		y += pillRowH
	}
	return y
}

// drawButton draws one rounded button with its label on row y. The chosen
// option (or a highlighted action) is filled with the accent; a toggle that
// is on gets an accent frame, its [x] showing the state.
func drawButton(f *terminal.Frame, s *state.AppState, x, y, w uint16, it choice, focused bool) {
	p := s.Theme
	bg := f.Buffer.CellAt(x, y).Style.Bg // blend into panels and dialogs alike
	border := cell.Style{Fg: p.FgMuted, Bg: bg}
	text := cell.Style{Fg: p.FgPrimary, Bg: bg}
	if it.Fg.Type() != cell.ColorDefault {
		text.Fg, text.Modifier = it.Fg, cell.ModifierBold
	}
	switch {
	case focused:
		border.Fg, text = p.AccentPurple, cell.Style{Fg: p.AccentPurple, Bg: bg, Modifier: cell.ModifierBold}
	case it.Selected && (it.Kind == kindToggle || it.Fg.Type() != cell.ColorDefault):
		border.Fg, text.Modifier = p.AccentCyan, cell.ModifierBold
	case it.Selected:
		border.Fg, text = p.AccentCyan, cell.Style{Fg: p.BgCanvas, Bg: p.AccentCyan, Modifier: cell.ModifierBold}
	}
	if it.Disabled {
		border.Modifier |= cell.ModifierDim
		text.Modifier |= cell.ModifierDim
	}
	bar := strings.Repeat("─", int(w)-2)
	if y > 0 {
		f.Buffer.SetStringWithin(x, y-1, "╭"+bar+"╮", border, w)
	}
	f.Buffer.SetStringWithin(x, y, "│", border, 1)
	f.Buffer.SetStringWithin(x+1, y, " "+buttonLabel(it)+" ", text, w-2)
	f.Buffer.SetStringWithin(x+w-1, y, "│", border, 1)
	f.Buffer.SetStringWithin(x, y+1, "╰"+bar+"╯", border, w)
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

// mix blends a toward b by t (0..1); non-RGB colours are returned unchanged.
func mix(a, b cell.Color, t float64) cell.Color {
	if a.Type() != cell.ColorRGB || b.Type() != cell.ColorRGB {
		return a
	}
	ar, ag, ab := a.RGB()
	br, bg, bb := b.RGB()
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return cell.NewColorRGB(lerp(ar, br), lerp(ag, bg), lerp(ab, bb))
}

// hoverFade is how long a control takes to light up or dim.
const hoverFade = 120 * time.Millisecond

// renderHover lights up the control under the mouse pointer, fading in on
// enter and out on leave. It works from the frame's semantic tree, so every
// button, tab and checkbox gets it, including those inside scrolled panels.
func renderHover(f *terminal.Frame, s *state.AppState, first int, now time.Time) {
	tree := f.AccessibilityTree()
	nodes := tree[min(first, len(tree)):]
	id, area := "", cell.Rect{}
	if s.PointerIn {
		// The last node drawn is on top.
		for i := len(nodes) - 1; i >= 0; i-- {
			n := nodes[i]
			if hoverable(n) && n.Bounds.Contains(s.PointerX, s.PointerY) {
				id, area = n.ID, n.Bounds
				break
			}
		}
	}
	if id != s.HoverID {
		s.LastHoverID, s.LastHoverAt = s.HoverID, now
		s.HoverID, s.HoverAt = id, now
	}
	for _, n := range nodes {
		if s.LastHoverID != "" && n.ID == s.LastHoverID && n.ID != id {
			if t := 1 - float64(now.Sub(s.LastHoverAt))/float64(hoverFade); t > 0 {
				lighten(f, s, n.Bounds, t)
			}
		}
	}
	if id != "" {
		lighten(f, s, area, min(1, float64(now.Sub(s.HoverAt))/float64(hoverFade)))
	}
}

func hoverable(n accessibility.AccessibilityNode) bool {
	if n.ID == "" || n.State&accessibility.StateDisabled != 0 {
		return false
	}
	switch n.Role {
	case accessibility.RoleButton, accessibility.RoleTab, accessibility.RoleCheckbox, accessibility.RoleRadioButton:
		// Copyable values are buttons too, but they carry their text as Value
		// and already underline on hover.
		return n.Value == ""
	}
	return false
}

// lighten moves a control toward its hover look by t: a button frame's
// border turns accent and the inside fills; plain controls get the fill only.
func lighten(f *terminal.Frame, s *state.AppState, r cell.Rect, t float64) {
	for y := r.Y; y < r.Y+r.Height; y++ {
		for x := r.X; x < r.X+r.Width; x++ {
			c := f.Buffer.CellAt(x, y)
			if c.Content >= '─' && c.Content <= '╿' {
				c.Style.Fg = mix(c.Style.Fg, s.Theme.AccentCyan, t)
			} else {
				c.Style.Bg = mix(c.Style.Bg, s.Theme.AccentBlue, 0.35*t)
				if t > 0.5 {
					c.Style.Modifier |= cell.ModifierBold
				}
			}
			f.Buffer.SetCell(x, y, c)
		}
	}
}

// tabChipWidth is the width of one "[ text ]" tab.
func tabChipWidth(text string) uint16 { return uint16(cell.StringWidth(text) + 4) }

// drawTabChip draws a tab as "[ text ]": faint brackets around text in fg;
// the selected tab gets accent brackets and an accent fill.
func drawTabChip(f *terminal.Frame, s *state.AppState, x, y uint16, text string, fg cell.Color, selected bool) {
	p := s.Theme
	bg := f.Buffer.CellAt(x, y).Style.Bg
	bracket := cell.Style{Fg: p.BgSubtle, Bg: bg}
	inner := cell.Style{Fg: fg, Bg: bg}
	if selected {
		bracket = cell.Style{Fg: p.AccentCyan, Bg: bg, Modifier: cell.ModifierBold}
		inner = cell.Style{Fg: p.BgCanvas, Bg: p.AccentCyan, Modifier: cell.ModifierBold}
	}
	w := tabChipWidth(text)
	f.Buffer.SetStringWithin(x, y, "[", bracket, 1)
	f.Buffer.SetStringWithin(x+1, y, " "+text+" ", inner, w-2)
	f.Buffer.SetStringWithin(x+w-1, y, "]", bracket, 1)
}

// tabItem draws one navigation tab and returns its width. Tabs are not
// buttons: bracketed chips, filled with the accent only when selected.
func tabItem(f *terminal.Frame, s *state.AppState, x, y uint16, id, label string, fg cell.Color, selected bool, onSelect func()) uint16 {
	w := tabChipWidth(label)
	rect := cell.NewRect(x, y, w, 1)
	drawTabChip(f, s, x, y, label, fg, selected)
	if f.IsFocused(id) {
		for cx := x; cx < x+w; cx++ {
			c := f.Buffer.CellAt(cx, y)
			c.Style.Modifier |= cell.ModifierUnderline
			f.Buffer.SetCell(cx, y, c)
		}
	}
	if s.Copy.Actions == nil {
		s.Copy.Actions = make(map[string]func())
	}
	s.Copy.Actions[id] = onSelect
	if f.FocusManager != nil {
		f.FocusManager.Register(id)
		f.FocusManager.RegisterBounds(id, rect)
	}
	f.RegisterClickHandler(rect, func(ev driver.MouseEvent) {
		if ev.Button == driver.MouseLeft {
			onSelect()
		}
	})
	state := accessibility.NodeState(0)
	if selected {
		state |= accessibility.StateSelected
	}
	f.RegisterAccessibility(accessibility.AccessibilityNode{ID: id, Role: accessibility.RoleTab, Label: label, State: state, Bounds: rect})
	return w
}

// sectionHeader draws "Title ────────" across a column and returns the first
// row of the section's content, one blank row below.
func sectionHeader(f *terminal.Frame, s *state.AppState, col cell.Rect, y uint16, title string) uint16 {
	t := title + " "
	tw := uint16(cell.StringWidth(t))
	f.Buffer.SetStringWithin(col.X, y, t, s.Theme.SectionHeaderStyle(), col.Width)
	if tw < col.Width {
		f.Buffer.SetStringWithin(col.X+tw, y, strings.Repeat("─", int(col.Width-tw)), cell.Style{Fg: s.Theme.BgSubtle, Bg: s.Theme.BgPanel}, col.Width-tw)
	}
	return y + 2
}
