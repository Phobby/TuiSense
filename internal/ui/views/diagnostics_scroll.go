package views

import (
	"fmt"
	"time"

	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/ui/copytext"
	"github.com/thebanri/limoni/core/buffer"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// The immediate-mode diagnostics body uses a bounded, retained offscreen frame.
// Only visible hitboxes and focus targets are forwarded to the real frame.
// The controller stage is outside this viewport and never scrolls or overlaps it.
var diagnosticBody struct{ frame *terminal.Frame }

func renderDiagnosticViewport(c *cursor, d *diag.Session, pc PanelContext, now time.Time) {
	if c.remaining() < 3 || c.r.Width < 8 {
		return
	}
	s := c.s
	scope := CopyScope(s)
	if s.DiagnosticsScope != scope {
		s.DiagnosticsScroll = 0
		s.DiagnosticsScope = scope
	}
	area := cell.NewRect(c.r.X, c.y, c.r.Width, uint16(c.remaining()-1))
	offArea := cell.NewRect(0, 0, area.Width, 512)
	if diagnosticBody.frame == nil {
		diagnosticBody.frame = terminal.NewFrame(buffer.NewBuffer(offArea), nil)
	}
	child := diagnosticBody.frame
	child.Reset()
	child.Buffer.Resize(offArea)
	child.Buffer.Clear()
	child.SetTheme(s.Theme.LimoniTheme())
	body := newCursor(child, offArea, s)
	renderDiagnosticContent(body, d, pc, now)
	s.DiagnosticsScrollMax = max(0, int(body.y)-int(area.Height))
	s.DiagnosticsScroll = min(max(0, s.DiagnosticsScroll), s.DiagnosticsScrollMax)
	offset := s.DiagnosticsScroll
	for y := uint16(0); y < area.Height; y++ {
		for x := uint16(0); x < area.Width; x++ {
			value := child.Buffer.CellAt(x, y+uint16(offset))
			// Blank offscreen cells use the panel's background too.
			if value.Content == ' ' {
				value.Style.Bg = s.Theme.BgPanel
			}
			c.f.Buffer.SetCell(area.X+x, area.Y+y, value)
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
	c.f.RegisterClickHandler(area, func(ev driver.MouseEvent) {
		if ev.Button == driver.MouseScrollUp {
			s.DiagnosticsScroll = max(0, s.DiagnosticsScroll-3)
		}
		if ev.Button == driver.MouseScrollDown {
			s.DiagnosticsScroll = min(s.DiagnosticsScrollMax, s.DiagnosticsScroll+3)
		}
	})
	for _, region := range child.ClickRegions {
		rect := translate(region.Area)
		if rect.Width == 0 || rect.Height == 0 {
			continue
		}
		region := region
		c.f.RegisterClickHandler(rect, func(ev driver.MouseEvent) {
			if ev.Button == driver.MouseScrollUp {
				s.DiagnosticsScroll = max(0, s.DiagnosticsScroll-3)
				return
			}
			if ev.Button == driver.MouseScrollDown {
				s.DiagnosticsScroll = min(s.DiagnosticsScrollMax, s.DiagnosticsScroll+3)
				return
			}
			ev.X -= area.X
			ev.Y = uint16(int(ev.Y) + offset - int(area.Y))
			region.Fire(ev, child)
			if capture := child.TakeMouseCapture(); capture != nil {
				c.f.CaptureMouse(func(event driver.MouseEvent) {
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
		c.f.RegisterAccessibility(node)
		if c.f.IsFocused(node.ID) {
			for y := rect.Y; y < rect.Y+rect.Height; y++ {
				for x := rect.X; x < rect.X+rect.Width; x++ {
					value := c.f.Buffer.CellAt(x, y)
					value.Style.Modifier |= cell.ModifierUnderline
					c.f.Buffer.SetCell(x, y, value)
				}
			}
		}
		if c.f.FocusManager != nil {
			// Text and buttons are both keyboard-accessible in this diagnostics surface.
			c.f.FocusManager.Register(node.ID)
			c.f.FocusManager.RegisterBounds(node.ID, rect)
		}
	}

	y := area.Y + area.Height
	controls := []struct {
		id, label string
		delta     int
	}{{"diag_scroll_up", "↑", -8}, {"diag_scroll_down", "↓", 8}}
	for i, control := range controls {
		control := control
		copytext.CopyableText{ID: control.id, Text: control.label, Style: s.Theme.ButtonStyle(false), OnActivate: func() { s.DiagnosticsScroll = min(max(0, s.DiagnosticsScroll+control.delta), s.DiagnosticsScrollMax) }}.Render(c.f, cell.NewRect(area.X+uint16(i*3), y, 1, 1), &s.Copy)
	}
	text := fmt.Sprintf("PgUp/PgDn · %d/%d", offset, s.DiagnosticsScrollMax)
	c.f.Buffer.SetStringWithin(area.X+7, y, text, s.Theme.MutedStyle(), area.Width-7)
}
