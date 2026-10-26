package views

import (
	"github.com/thebanri/limoni/core/buffer"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// renderPanelViewport keeps every control reachable on short terminals.
func renderPanelViewport(f *terminal.Frame, viewport cell.Rect, pc PanelContext, render PanelRenderer) {
	s := pc.S
	scope := CopyScope(s) + string(s.Mode)
	if scope != s.PanelScrollScope {
		s.PanelScrollScope = scope
		s.PanelScroll = 0
	}
	area := viewport
	area.Height--
	height := uint16(40)
	if s.Mode == "ez" {
		height = 24
	}
	offArea := cell.NewRect(0, 0, area.Width, max(area.Height, height))
	child := terminal.NewFrame(buffer.NewBuffer(offArea), nil)
	child.SetTheme(s.Theme.LimoniTheme())
	render(child, offArea, pc)
	s.PanelScrollMax = max(0, int(offArea.Height)-int(area.Height))
	s.PanelScroll = min(max(0, s.PanelScroll), s.PanelScrollMax)
	offset := s.PanelScroll
	for y := uint16(0); y < area.Height; y++ {
		for x := uint16(0); x < area.Width; x++ {
			value := child.Buffer.CellAt(x, y+uint16(offset))
			// Blank offscreen cells use the panel's background too.
			if value.Content == ' ' {
				value.Style.Bg = s.Theme.BgPanel
			}
			f.Buffer.SetCell(area.X+x, area.Y+y, value)
		}
	}
	translate := func(r cell.Rect) cell.Rect {
		y0 := max(int(r.Y), offset)
		y1 := min(int(r.Y+r.Height), offset+int(area.Height))
		if y1 <= y0 {
			return cell.Rect{}
		}
		return cell.NewRect(area.X+r.X, area.Y+uint16(y0-offset), min(r.Width, area.Width-min(r.X, area.Width)), uint16(y1-y0))
	}
	f.RegisterClickHandler(area, func(ev driver.MouseEvent) {
		if ev.Button == driver.MouseScrollUp {
			s.PanelScroll = max(0, s.PanelScroll-3)
		}
		if ev.Button == driver.MouseScrollDown {
			s.PanelScroll = min(s.PanelScrollMax, s.PanelScroll+3)
		}
	})
	for _, region := range child.ClickRegions {
		rect := translate(region.Area)
		if rect.Width == 0 || rect.Height == 0 {
			continue
		}
		region := region
		f.RegisterClickHandler(rect, func(ev driver.MouseEvent) {
			if ev.Button == driver.MouseScrollUp {
				s.PanelScroll = max(0, s.PanelScroll-3)
				return
			}
			if ev.Button == driver.MouseScrollDown {
				s.PanelScroll = min(s.PanelScrollMax, s.PanelScroll+3)
				return
			}
			ev.X -= area.X
			ev.Y = uint16(int(ev.Y) + offset - int(area.Y))
			region.Fire(ev, child)
			if capture := child.TakeMouseCapture(); capture != nil {
				f.CaptureMouse(func(event driver.MouseEvent) {
					event.X -= area.X
					event.Y = uint16(int(event.Y) + offset - int(area.Y))
					capture(event)
				})
			}
		})
	}
	for _, node := range child.AccessibilityTree() {
		rect := translate(node.Bounds)
		if rect.Width == 0 || rect.Height == 0 {
			continue
		}
		node.Bounds = rect
		if _, ok := s.Copy.Actions[node.ID]; !ok {
			original := cell.NewRect(rect.X-area.X, rect.Y-area.Y+uint16(offset), rect.Width, rect.Height)
			for _, region := range child.ClickRegions {
				if region.Area.Intersection(original).Width > 0 && region.Area.Intersection(original).Height > 0 {
					region := region
					s.Copy.Actions[node.ID] = func() { region.Fire(driver.MouseEvent{Button: driver.MouseLeft}, child) }
					break
				}
			}
		}
		f.RegisterAccessibility(node)
		if f.IsFocused(node.ID) {
			for y := rect.Y; y < rect.Y+rect.Height; y++ {
				for x := rect.X; x < rect.X+rect.Width; x++ {
					value := f.Buffer.CellAt(x, y)
					value.Style.Modifier |= cell.ModifierUnderline
					f.Buffer.SetCell(x, y, value)
				}
			}
		}
		if f.FocusManager != nil {
			// Text and buttons are both keyboard-accessible in this diagnostics surface.
			f.FocusManager.Register(node.ID)
			f.FocusManager.RegisterBounds(node.ID, rect)
		}
	}

	if s.PanelScrollMax > 0 {
		f.Buffer.SetStringWithin(area.X+2, area.Y+area.Height, "↑ ↓", s.Theme.MutedStyle(), area.Width-2)
	}
}
