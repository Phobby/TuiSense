// Package copytext integrates reusable copy targets with Limoni mouse and focus handling.
package copytext

import (
	"fmt"
	"strings"

	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// Model stores only the current frame's actions; device and panel switches cannot
// activate stale values. Begin must run after keyboard processing, before drawing.
type Model struct {
	OnCopy   func(string)
	Actions  map[string]func()
	Scope    string
	Disabled bool
}

func (m *Model) Begin(scope string) { m.Scope = scope; m.Actions = make(map[string]func()) }
func (m *Model) Activate(id, scope string, key driver.KeyEvent) bool {
	if m.Disabled || key.Release || scope != m.Scope || (key.Type != driver.KeyEnter && key.Type != driver.KeySpace) {
		return false
	}
	if fn := m.Actions[id]; fn != nil {
		fn()
		return true
	}
	return false
}

// CopyableText keeps the exact payload separate from its clipped/wrapped display.
// Each visible line has its own hitbox and copies the complete payload.
type CopyableText struct {
	ID, Text, Value string
	Style           cell.Style
	Indicator       bool
	// Overlay preserves text and styles already drawn by another Limoni widget.
	Overlay    bool
	OnActivate func()
	OnScroll   func(driver.MouseEvent)
}

func (w CopyableText) Render(f *terminal.Frame, area cell.Rect, m *Model) {
	area = area.Intersection(f.Area())
	if area.Width == 0 || area.Height == 0 {
		return
	}
	if w.ID == "" {
		w.ID = fmt.Sprintf("copy_%d_%d", area.X, area.Y)
	}
	action := w.OnActivate
	if action == nil {
		action = func() {
			if m.OnCopy != nil {
				m.OnCopy(w.Value)
			}
		}
	}
	if m.Actions == nil {
		m.Actions = make(map[string]func())
	}
	rawAction := action
	action = func() {
		if !m.Disabled {
			rawAction()
		}
	}
	if m.Disabled {
		if !w.Overlay {
			for i, line := range strings.Split(w.Text, "\n") {
				if i >= int(area.Height) {
					break
				}
				f.Buffer.SetStringWithin(area.X, area.Y+uint16(i), line, w.Style, area.Width)
			}
		}
		return
	}
	m.Actions[w.ID] = action
	if f.FocusManager != nil {
		f.FocusManager.Register(w.ID)
		f.FocusManager.RegisterBounds(w.ID, area)
	}
	st := w.Style
	if f.IsFocused(w.ID) || f.HoveredRegionID() == w.ID {
		st.Modifier |= cell.ModifierUnderline | cell.ModifierBold
	}
	text := w.Text
	if w.Indicator {
		text += " ⧉"
	}
	for i, line := range strings.Split(text, "\n") {
		if i >= int(area.Height) {
			break
		}
		width := uint16(min(int(area.Width), cell.StringWidth(line)))
		if width == 0 {
			continue
		}
		rect := cell.NewRect(area.X, area.Y+uint16(i), width, 1)
		if !w.Overlay {
			f.Buffer.SetStringWithin(rect.X, rect.Y, line, st, rect.Width)
		} else if f.IsFocused(w.ID) || f.HoveredRegionID() == w.ID {
			for x := rect.X; x < rect.X+rect.Width; x++ {
				c := f.Buffer.CellAt(x, rect.Y)
				c.Style.Modifier |= cell.ModifierUnderline | cell.ModifierBold
				f.Buffer.SetCell(x, rect.Y, c)
			}
		}
		f.RegisterClickHandler(rect, func(ev driver.MouseEvent) {
			if ev.Button != driver.MouseLeft || ev.Drag {
				return
			}
			if f.FocusManager != nil {
				f.FocusManager.SetFocused(w.ID)
			}
			action()
		})
		f.RegisterEventRegion(terminal.EventRegion{ID: w.ID, Area: rect, Phase: terminal.TargetPhase, Handler: func(ctx *terminal.EventContext) {
			if (ctx.Mouse.Button == driver.MouseScrollUp || ctx.Mouse.Button == driver.MouseScrollDown) && w.OnScroll != nil {
				w.OnScroll(ctx.Mouse)
			}
			if ctx.Mouse.Button == driver.MouseLeft && !ctx.Mouse.Drag {
				if f.FocusManager != nil {
					f.FocusManager.SetFocused(w.ID)
				}
				action()
			}
		}})
	}
	f.RegisterAccessibility(accessibility.AccessibilityNode{ID: w.ID, Role: accessibility.RoleButton, Label: "Copy " + w.Text, Value: w.Value, Bounds: area})
}
