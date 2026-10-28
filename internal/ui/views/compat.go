package views

import (
	"strings"

	"github.com/Phobby/TuiSense/internal/compat"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/thebanri/limoni/core/cell"
)

// renderCompat is the Driver & Compatibility section of Device Details: only
// actionable findings for the connected controller, and remedies that run
// solely after the user reviewed and approved the exact commands.
func renderCompat(c *cursor, pc PanelContext) {
	s := pc.S
	p := c.p()
	r := s.Compat
	const kw = 14
	c.kv(i18n.T("Controller"), i18n.T(r.Controller), kw, p.ValueStyle())
	c.kv(i18n.T("System"), r.OS, kw, p.ValueStyle())
	if r.Connection != "" {
		c.kv(i18n.T("Connection"), i18n.T(r.Connection), kw, p.ValueStyle())
	}
	if r.Backend != "" {
		c.kv(i18n.T("Backend"), r.Backend, kw, p.ValueStyle())
	}
	if r.Input != "" {
		c.kv(i18n.T("Input"), i18n.T(r.Input), kw, p.ValueStyle())
	}
	c.gap()
	for _, ch := range r.Checks {
		icon, col := compatIcon(c, ch.Level)
		text := icon + " " + i18n.T(ch.Title) + ": " + i18n.T(ch.Detail)
		if ch.Value != "" {
			text += " (" + ch.Value + ")"
		}
		c.wrap(text, cell.Style{Fg: col, Bg: p.BgCanvas}, 3)
	}
	c.gap()
	items := []choice{{ID: "compat_check", Label: i18n.T("Check Driver Requirements"), OnSelect: func() {
		s.CompatReview = ""
		if pc.CB.OnCheckDrivers != nil {
			pc.CB.OnCheckDrivers()
		}
	}}}
	for _, a := range r.Actions {
		id := a.ID
		items = append(items, choice{ID: "compat_action_" + id, Label: i18n.T(a.Title), Selected: s.CompatReview == id,
			OnSelect: func() { s.CompatReview = id; s.CompatStatus = "" }})
	}
	c.pills("", 0, items)
	if s.CompatStatus != "" {
		c.wrap(i18n.T(s.CompatStatus), p.MutedStyle(), 2)
	}
	if a, ok := r.Action(s.CompatReview); ok {
		renderCompatReview(c, pc, a)
	}
}

// renderCompatReview shows everything an action will do before it can run.
func renderCompatReview(c *cursor, pc PanelContext, a compat.Action) {
	s := pc.S
	p := c.p()
	c.gap()
	c.line(i18n.T(a.Title), p.SectionHeaderStyle())
	c.wrap(i18n.T(a.Explanation), p.ValueStyle(), 4)
	if len(a.Changes) > 0 {
		c.line(i18n.T("Changes to your system:"), p.MutedStyle())
		for _, ch := range a.Changes {
			c.wrap("  • "+i18n.T(ch), p.ValueStyle(), 2)
		}
	}
	if len(a.Commands) > 0 {
		c.line(i18n.T("Exact commands (no shell):"), p.MutedStyle())
		for _, argv := range a.Commands {
			c.wrap("  $ "+strings.Join(argv, " "), p.ValueStyle(), 2)
		}
	}
	for _, in := range a.Instructions {
		c.wrap("  → "+i18n.T(in), p.ValueStyle(), 3)
	}
	if len(a.Rollback) > 0 {
		c.line(i18n.T("To undo:"), p.MutedStyle())
		for _, rb := range a.Rollback {
			c.wrap("  $ "+i18n.T(rb), p.MutedStyle(), 2)
		}
	}
	if a.Reconnect {
		c.wrap(i18n.T("Reconnect the controller afterwards."), p.MutedStyle(), 1)
	}
	var items []choice
	if !a.Manual && !s.CompatBusy {
		id := a.ID
		items = append(items, choice{ID: "compat_approve_" + id, Label: i18n.T("Approve and run"), OnSelect: func() {
			if pc.CB.OnRunCompatAction != nil {
				pc.CB.OnRunCompatAction(id)
			}
		}})
	}
	items = append(items, choice{ID: "compat_cancel", Label: i18n.T("Close"), OnSelect: func() { s.CompatReview = "" }})
	c.pills("", 0, items)
}

func compatIcon(c *cursor, l compat.Level) (string, cell.Color) {
	p := c.p()
	switch l {
	case compat.LevelOK:
		return "✓", p.Success
	case compat.LevelWarning:
		return "!", p.Warning
	case compat.LevelMissing:
		return "✕", p.Danger
	}
	return "·", p.FgSecondary
}
