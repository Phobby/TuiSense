package copytext

import (
	"strings"
	"testing"

	"github.com/thebanri/limoni/core/buffer"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

func frame() *terminal.Frame {
	return terminal.NewFrame(buffer.NewBuffer(cell.NewRect(0, 0, 60, 20)), terminal.NewFocusManager())
}
func TestValuesMessagesMultilineAndTruncation(t *testing.T) {
	for _, payload := range []string{"0x00000413", "3.200%", "Possible drift detected: 3.2%", "line one\nline two\n\tprecise", strings.Repeat("long diagnostic message ", 30)} {
		t.Run(payload[:min(len(payload), 16)], func(t *testing.T) {
			f := frame()
			var copied string
			m := Model{OnCopy: func(s string) { copied = s }}
			m.Begin("scope")
			CopyableText{ID: "value", Text: payload, Value: payload}.Render(f, cell.NewRect(3, 4, 12, 2), &m)
			if len(f.ClickRegions) == 0 {
				t.Fatal("no hitbox")
			}
			f.ClickRegions[0].Fire(driver.MouseEvent{Button: driver.MouseLeft}, f)
			if copied != payload {
				t.Fatalf("lost underlying text: %q", copied)
			}
			for _, r := range f.ClickRegions {
				if r.Area.X != 3 || r.Area.Y < 4 || r.Area.Y > 5 || r.Area.Width > 12 {
					t.Fatalf("bad hitbox %+v", r.Area)
				}
			}
		})
	}
}
func TestHitboxesKeyboardHoverAndNoDragCopy(t *testing.T) {
	f := frame()
	copies := 0
	m := Model{OnCopy: func(string) { copies++ }}
	m.Begin("device:panel")
	CopyableText{ID: "a", Text: "0x0630", Value: "0x0630"}.Render(f, cell.NewRect(3, 4, 20, 1), &m)
	CopyableText{ID: "b", Text: "250 Hz", Value: "250 Hz"}.Render(f, cell.NewRect(3, 6, 20, 1), &m)
	r := f.ClickRegions[0]
	if r.Area.Contains(2, 4) || r.Area.Contains(9, 4) || !r.Area.Contains(8, 4) || r.Area.Contains(3, 5) {
		t.Fatalf("hitbox includes padding or border: %+v", r.Area)
	}
	for _, ev := range []driver.MouseEvent{{Button: driver.MouseLeft, Drag: true}, {Button: driver.MouseRelease}, {Button: driver.MouseScrollUp}, {Button: driver.MouseNone}} {
		r.Fire(ev, f)
	}
	if copies != 0 {
		t.Fatal("non-click copied text")
	}
	f.FocusManager.SetFocused("a")
	f.FocusManager.Next()
	if f.FocusManager.Focused() != "b" {
		t.Fatal("Tab navigation lost")
	}
	if !m.Activate("b", "device:panel", driver.KeyEvent{Type: driver.KeyEnter}) || copies != 1 {
		t.Fatal("Enter did not copy")
	}
	if m.Activate("b", "other device", driver.KeyEvent{Type: driver.KeyEnter}) {
		t.Fatal("stale scope activated")
	}
	if m.Activate("b", "device:panel", driver.KeyEvent{Type: driver.KeyArrowRight}) {
		t.Fatal("controller keys consumed")
	}
	f.DispatchPointerMove(driver.MouseEvent{X: 4, Y: 4, Button: driver.MouseNone})
	if f.HoveredRegionID() != "a" {
		t.Fatal("hover target missing")
	}
	m.Begin("device:panel")
	if m.Activate("b", "device:panel", driver.KeyEvent{Type: driver.KeyEnter}) {
		t.Fatal("hidden target remained active")
	}
}

func TestEventRouterCopiesOnceAndPreservesScroll(t *testing.T) {
	f := frame()
	copies, scrolls := 0, 0
	m := Model{OnCopy: func(string) { copies++ }}
	m.Begin("test")
	CopyableText{ID: "target", Text: "250 Hz", Value: "250 Hz", OnScroll: func(driver.MouseEvent) { scrolls++ }}.Render(f, cell.NewRect(3, 4, 12, 1), &m)
	if !f.DispatchEventRegions(driver.MouseEvent{Button: driver.MouseLeft, X: 4, Y: 4}) || copies != 1 {
		t.Fatal("semantic mouse router swallowed copy")
	}
	f.DispatchEventRegions(driver.MouseEvent{Button: driver.MouseScrollDown, X: 4, Y: 4})
	if copies != 1 || scrolls != 1 {
		t.Fatal("scroll copied text or lost log navigation")
	}
	m.Disabled = true
	f.DispatchEventRegions(driver.MouseEvent{Button: driver.MouseLeft, X: 4, Y: 4})
	if copies != 1 {
		t.Fatal("background copied through a modal")
	}
}

func TestOverlayRetainsUnderlyingTextAndStyle(t *testing.T) {
	f := frame()
	red := cell.Style{Fg: cell.NewColorRGB(255, 0, 0)}
	f.Buffer.SetStringWithin(3, 4, "error", red, 5)
	m := Model{}
	m.Begin("test")
	CopyableText{ID: "overlay", Text: "error", Value: "full error", Overlay: true}.Render(f, cell.NewRect(3, 4, 5, 1), &m)
	c := f.Buffer.CellAt(3, 4)
	if c.Content != 'e' || c.Style.Fg != red.Fg || c.Style.Modifier&cell.ModifierUnderline == 0 {
		t.Fatal("overlay replaced existing log text or severity color")
	}
}
