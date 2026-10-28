package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/theme"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// The diagnostics view is generic: every section is derived from the active
// device's diag.Session (which is itself built from capabilities and the
// descriptor's InputLayout). Nothing here knows about specific controllers.

// cursor is a small top-to-bottom writer that clips to its rectangle.
type cursor struct {
	f *terminal.Frame
	r cell.Rect
	y uint16
	s *state.AppState
}

func newCursor(f *terminal.Frame, r cell.Rect, s *state.AppState) *cursor {
	return &cursor{f: f, r: r, y: r.Y, s: s}
}

func (c *cursor) ok() bool         { return c.y < c.r.Y+c.r.Height }
func (c *cursor) remaining() int   { return int(c.r.Y+c.r.Height) - int(c.y) }
func (c *cursor) gap()             { c.y++ }
func (c *cursor) limit() uint16    { return c.r.X + c.r.Width }
func (c *cursor) p() theme.Palette { return c.s.Theme }

func (c *cursor) line(text string, st cell.Style) {
	if c.ok() {
		copyValue(c.f, cell.NewRect(c.r.X, c.y, c.r.Width, 1), c.s, text, cleanCopyText(text), st)
		c.y++
	}
}

// wrap writes text word-wrapped to the cursor width, at most maxLines lines.
func (c *cursor) wrap(text string, st cell.Style, maxLines int) {
	lines := wrapCopyText(text, int(c.r.Width))
	visible := min(len(lines), maxLines, c.remaining())
	if visible <= 0 {
		return
	}
	copyValue(c.f, cell.NewRect(c.r.X, c.y, c.r.Width, uint16(visible)), c.s, strings.Join(lines[:visible], "\n"), cleanCopyText(text), st)
	c.y += uint16(visible)
}

// pills renders a pill row, wrapping onto further lines (aligned after the
// label) when the items do not fit.
func (c *cursor) pills(label string, lw uint16, items []choice) {
	if label != "" && uint16(cell.StringWidth(label)+1) > lw {
		lw = uint16(cell.StringWidth(label) + 1)
	}
	first := true
	for len(items) > 0 && c.ok() {
		x := c.r.X + lw
		n := 0
		for n < len(items) {
			w := uint16(cell.StringWidth(items[n].Label) + 3)
			if x+w > c.limit() && n > 0 {
				break
			}
			x += w
			n++
		}
		lbl := ""
		if first {
			lbl = label
		}
		pillRow(c.f, c.r.X, c.y, c.limit(), c.s, lbl, lw, items[:n])
		items = items[n:]
		first = false
		c.y++
	}
}

// kv writes "key value", wrapping a long value onto up to two more lines
// aligned under the value column.
func (c *cursor) kv(key, val string, keyW uint16, valStyle cell.Style) {
	c.kvCopy(key, val, val, keyW, valStyle)
}

func (c *cursor) kvCopy(key, display, value string, keyW uint16, valStyle cell.Style) {
	if !c.ok() {
		return
	}
	c.f.Buffer.SetStringWithin(c.r.X, c.y, key, c.p().MutedStyle(), min(keyW, c.r.Width))
	if c.r.Width <= keyW {
		c.y++
		return
	}
	width := c.r.Width - keyW
	lines := []string{display}
	if cell.StringWidth(display) > int(width) && strings.Contains(display, " ") {
		lines = wrapCopyText(display, int(width))
	}
	visible := min(3, len(lines), c.remaining())
	copyValue(c.f, cell.NewRect(c.r.X+keyW, c.y, width, uint16(visible)), c.s, strings.Join(lines[:visible], "\n"), value, valStyle)
	c.y += uint16(visible)
}

func (c *cursor) gauge(id, label string, ratio float64, valText string, col cell.Color) {
	if !c.ok() {
		return
	}
	p := c.p()
	lw := uint16(10)
	vw := uint16(cell.StringWidth(valText) + 1)
	c.f.Buffer.SetStringWithin(c.r.X, c.y, label, p.MutedStyle(), lw)
	if c.r.Width > lw+vw+4 {
		gw := c.r.Width - lw - vw
		c.f.RenderWidget(widgets.LineGauge{ID: id, Ratio: clamp01(ratio), HideLabel: true,
			FilledStyle:   cell.Style{Fg: col, Bg: p.BgPanel},
			UnfilledStyle: cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
		}, cell.NewRect(c.r.X+lw, c.y, gw, 1))
		copyValue(c.f, cell.NewRect(c.r.X+lw+gw+1, c.y, min(vw, c.limit()-(c.r.X+lw+gw+1)), 1), c.s, valText, valText, p.ValueStyle())
	}
	c.y++
}

func (c *cursor) sparkline(id string, data []float64, col cell.Color, h uint16) {
	if c.remaining() < int(h) || len(data) == 0 {
		return
	}
	c.f.RenderWidget(widgets.Sparkline{ID: id, Data: data, Color: col, Style: c.p().PanelStyle()},
		cell.NewRect(c.r.X, c.y, c.r.Width, h))
	c.y += h
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func statusColor(p theme.Palette, st diag.Status) cell.Color {
	switch st {
	case diag.Passed:
		return p.Success
	case diag.Warning, diag.PossibleIssue:
		return p.Warning
	case diag.Failed:
		return p.Danger
	case diag.Testing:
		return p.AccentCyan
	case diag.Inconclusive, diag.Incomplete:
		return p.AccentPurple
	}
	return p.FgMuted
}

func shortCategory(c diag.Category) string {
	switch c {
	case diag.CatSticks:
		return i18n.T("Sticks")
	case diag.CatAudioIn:
		return i18n.T("Mic")
	case diag.CatAudioOut:
		return i18n.T("Speaker")
	case diag.CatMotion:
		return i18n.T("Motion")
	case diag.CatTouchpad:
		return i18n.T("Touch")
	case diag.CatHaptics:
		return i18n.T("Rumble")
	case diag.CatBattery:
		return i18n.T("Power")
	case diag.CatConnection:
		return i18n.T("Link")
	}
	return c.Label()
}

// renderDiagnosticsPanel is the Diagnostics tab (both modes; content adapts).
func renderDiagnosticsPanel(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	title := i18n.T("Diagnostics 2.0")
	d := s.Diag
	if d != nil && d.Ctx.Simulated {
		title += i18n.T(" · SIMULATED DATA")
	}
	if s.IsPreviewDevice() {
		// Tests measure a real controller's input; a preview has none.
		inner := panelFrame(f, area, s, i18n.T("Diagnostics 2.0"))
		c := newCursor(f, inner, s)
		c.line(i18n.T("Diagnostics measure a connected controller's own input, so they are not part of the settings preview."), p.LabelStyle())
		c.line(i18n.T("Connect this controller to test its sticks, triggers and buttons."), p.MutedStyle())
		return
	}
	inner := panelFrame(f, area, s, title)
	c := newCursor(f, inner, s)
	renderCopyActions(c, true)
	if d == nil {
		c.line(i18n.T("Connect a controller (or choose one with [C]) to run diagnostics."), p.LabelStyle())
		return
	}
	now := time.Now()
	renderDiagTabs(c, d)
	if g := d.Guided; g != nil {
		if cat, ok := g.Current(); ok {
			c.pills(i18n.Tf("Guided %d/%d", g.Idx+1, len(g.Steps)), 13, []choice{
				{ID: "diag_guided_step", Label: cat.Label(), Selected: true},
				{ID: "diag_guided_next", Label: i18n.T("Next ›"), OnSelect: d.NextGuided},
				{ID: "diag_guided_end", Label: i18n.T("■ End"), OnSelect: d.StopGuided},
			})
			c.line(d.Status(cat).Icon()+" "+d.Status(cat).Label(), p.MutedStyle())
			report := diag.BuildReport(d, "", false, now)
			complete := 0
			for _, entry := range report.Entries {
				for _, step := range g.Steps {
					if step == entry.Category && entry.Completion == diag.CompletionDone {
						complete++
					}
				}
			}
			c.line(i18n.Tf("Protocols completed: %d/%d", complete, len(g.Steps)), p.MutedStyle())
		}
	}
	c.gap()
	renderDiagnosticViewport(c, d, pc, now)
}

func renderDiagnosticContent(c *cursor, d *diag.Session, pc PanelContext, now time.Time) {
	p := c.p()
	if d.Active == diag.CatHealthReport {
		renderHealthReport(c, d, pc, d.ReportSnapshot(now))
		return
	}
	if d.Active != "" {
		renderTestReportControls(c, d)
		if d.ShowTestReport {
			renderHealthReport(c, d, pc, d.ReportSnapshot(now).Scope(d.Active))
			return
		}
	}
	if d.Active == "" {
		renderDiagSummary(c, d, pc)
		return
	}
	if a := d.Avail[d.Active]; !a.Available() {
		c.line(a.Status.Icon()+" "+d.Active.Label()+": "+a.Status.Label(), cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold})
		c.wrap(a.Reason, p.LabelStyle(), 4)
		return
	}
	switch d.Active {
	case diag.CatSticks:
		renderStickCategory(c, d, now)
	case diag.CatButtons:
		renderButtonDiag(c, d)
	case diag.CatTriggers:
		renderTriggerDiag(c, d, now)
	case diag.CatTouchpad:
		renderTouchDiag(c, d)
	case diag.CatMotion:
		renderMotionDiag(c, d, now)
	case diag.CatHaptics:
		renderHapticDiag(c, d, now)
	case diag.CatAudioIn:
		renderMicDiag(c, d, now)
	case diag.CatAudioOut:
		renderSpeakerDiag(c, d)
	case diag.CatBattery:
		renderBatteryDiag(c, d, now)
	case diag.CatConnection:
		renderConnectionDiag(c, d, pc, now)
	case diag.CatDeadzone:
		renderStickScopes(c, d.Sticks, c.s.Mode == state.ModeAdvanced)
		renderDeadzonePreview(c, d.Sticks, c.s.Mode == state.ModeAdvanced)
	case diag.CatFirmware:
		c.wrap(i18n.T("Firmware metadata is read-only; it does not establish firmware integrity or hardware health."), p.MutedStyle(), 4)
		c.kv(i18n.T("Firmware"), nonEmptyStr(d.Ctx.Info.FirmwareVersion, i18n.T("Unavailable")), 14, p.ValueStyle())
		c.kv(i18n.T("Hardware"), nonEmptyStr(d.Ctx.Info.HardwareVersion, i18n.T("Unavailable")), 14, p.LabelStyle())
	}
}

// renderDiagTabs draws "Summary" plus one status-colored pill per visible category.
func renderDiagTabs(c *cursor, d *diag.Session) {
	if !c.ok() {
		return
	}
	p := c.p()
	x := c.r.X
	draw := func(id, label string, st cell.Style, onClick func()) bool {
		w := uint16(cell.StringWidth(label))
		if x+w > c.limit() {
			c.y++
			x = c.r.X
			if !c.ok() {
				return false
			}
		}
		rect := cell.NewRect(x, c.y, w, 1)
		c.f.Buffer.SetStringWithin(x, c.y, label, st, w)
		c.f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { onClick() })
		c.f.RegisterAccessibility(accessibility.AccessibilityNode{ID: id, Role: accessibility.RoleTab, Label: strings.TrimSpace(label), Bounds: rect})
		x += w + 1
		return true
	}
	sumStyle := cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}
	if d.Active == "" {
		sumStyle = p.ButtonStyle(true)
	}
	draw("diag_tab_summary", i18n.T(" ◎ All "), sumStyle, func() { d.Select("") })
	draw("diag_tab_health_report", " "+i18n.T("Health Report")+" ", p.ButtonStyle(d.Active == diag.CatHealthReport), func() { d.Select(diag.CatHealthReport); d.GenerateHealthReport(time.Now()) })
	if c.r.Height < 16 {
		if d.Active != "" && d.Active != diag.CatHealthReport {
			draw("diag_tab_"+string(d.Active), " "+d.Active.Label()+" ", p.ButtonStyle(true), func() {})
		}
		cycle := func(delta int) {
			categories := append([]diag.Category{"", diag.CatHealthReport}, d.Visible()...)
			index := 0
			for i, category := range categories {
				if category == d.Active {
					index = i
				}
			}
			d.Select(categories[(index+delta+len(categories))%len(categories)])
		}
		draw("diag_category_prev", " ‹ ", p.ButtonStyle(false), func() { cycle(-1) })
		draw("diag_category_next", " › ", p.ButtonStyle(false), func() { cycle(1) })
		c.y++
		return
	}
	for _, cat := range d.Visible() {
		st := d.Status(cat)
		style := cell.Style{Fg: statusColor(p, st), Bg: p.BgElevated}
		if st == diag.Unavailable {
			style = cell.Style{Fg: p.FgMuted, Bg: p.BgPanel}
		}
		if d.Active == cat {
			style = p.ButtonStyle(true)
		}
		cat := cat
		if !draw("diag_tab_"+string(cat), " "+st.Icon()+" "+shortCategory(cat)+" ", style, func() { d.Select(cat) }) {
			return
		}
	}
	c.y++
}

func renderDiagSummary(c *cursor, d *diag.Session, pc PanelContext) {
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	if d.Ctx.Simulated {
		c.line(i18n.T("⚠ Simulated device — every value below is mock data, not a hardware measurement."), cell.Style{Fg: p.Warning, Bg: p.BgPanel})
	}
	actions := []choice{
		{ID: "diag_run_guided", Label: i18n.T("▶ Run guided check"), OnSelect: d.StartGuided},
		{ID: "diag_generate_health_report", Label: i18n.T("Generate Health Report"), OnSelect: func() { d.Select(diag.CatHealthReport); d.GenerateHealthReport(time.Now()) }},
		{ID: "diag_reset", Label: i18n.T("↻ Reset results"), OnSelect: func() { d.Reset(time.Now()) }},
	}
	if pc.CB.OnExportReport != nil {
		actions = append(actions, choice{ID: "diag_export", Label: i18n.T("⇩ Export report"), OnSelect: pc.CB.OnExportReport})
	}
	c.pills("", 0, actions)
	var history []choice
	if pc.CB.OnSaveDiagnosticHistory != nil {
		history = append(history, choice{ID: "diag_history_save", Label: "Save snapshot", OnSelect: pc.CB.OnSaveDiagnosticHistory})
	}
	if pc.CB.OnCompareDiagnosticHistory != nil {
		history = append(history, choice{ID: "diag_history_compare", Label: "Compare history", OnSelect: pc.CB.OnCompareDiagnosticHistory})
	}
	if pc.CB.OnClearDiagnosticHistory != nil {
		history = append(history, choice{ID: "diag_history_clear", Label: "Clear history", OnSelect: pc.CB.OnClearDiagnosticHistory})
	}
	if pc.CB.OnReviewDiagnosticHistory != nil {
		history = append(history, choice{ID: "diag_history_review", Label: i18n.T("Review Previous Report"), OnSelect: pc.CB.OnReviewDiagnosticHistory})
	}
	c.pills("", 0, history)
	for _, line := range d.HistorySummary {
		c.wrap(line, p.LabelStyle(), 3)
	}
	c.gap()
	labelW := uint16(22)
	for _, cat := range diag.AllCategories {
		if !c.ok() {
			break
		}
		st := d.Status(cat)
		if d.Avail[cat].Hidden() {
			continue // absent hardware, or explained in Driver & Compatibility
		}
		text := st.Label()
		if r, ok := d.Results[cat]; ok && (st != diag.Unsupported && st != diag.Unavailable) {
			text += " · " + r.At.Format("15:04:05") + " — " + r.Summary
		} else if a := d.Avail[cat]; !a.Available() && adv {
			text += " — " + a.Reason
		}
		rowY := c.y
		c.f.Buffer.SetStringWithin(c.r.X, c.y, st.Icon()+" "+cat.Label(), cell.Style{Fg: statusColor(p, st), Bg: p.BgPanel, Modifier: cell.ModifierBold}, labelW)
		if c.r.Width > labelW {
			payload := text
			if result, ok := d.Results[cat]; ok {
				payload = result.CopyText()
			}
			copyValue(c.f, cell.NewRect(c.r.X+labelW, c.y, c.r.Width-labelW, 1), c.s, text, payload, p.LabelStyle())
		}
		cat := cat
		if st != diag.Unsupported {
			c.f.RegisterClickHandler(cell.NewRect(c.r.X, rowY, min(labelW, c.r.Width), 1), func(_ driver.MouseEvent) { d.Select(cat) })
		}
		c.f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "diag_summary_" + string(cat), Role: accessibility.RoleGeneric,
			Label: cat.Label(), Value: st.String(), Bounds: cell.NewRect(c.r.X, rowY, c.r.Width, 1)})
		c.y++
	}
	if adv && len(d.Events) > 0 && c.remaining() > 3 {
		c.line("Recent diagnostic events", p.SectionHeaderStyle())
		for i := max(0, len(d.Events)-3); i < len(d.Events); i++ {
			event := d.Events[i]
			c.wrap(event.At.Format("15:04:05")+" "+event.Message, p.MutedStyle(), 2)
		}
	}
	if c.remaining() > 1 {
		c.gap()
		c.line(i18n.T("Passed only appears after a real measurement or your explicit confirmation."), p.MutedStyle())
	}
}

// renderResult prints a result block: badge + summary, then details for Advanced.
func renderResult(c *cursor, r diag.Result, adv bool) {
	if !c.ok() {
		return
	}
	p := c.p()
	tag := ""
	if r.Simulated {
		tag = i18n.T(" [simulated]")
	}
	if r.UserConfirmed {
		tag += i18n.T(" [your confirmation]")
	}
	message := r.CopyText()
	copyValue(c.f, cell.NewRect(c.r.X, c.y, c.r.Width, 1), c.s, fmt.Sprintf("%s %s — %s%s", r.Status.Icon(), r.Status.Label(), r.Summary, tag), message, cell.Style{Fg: statusColor(p, r.Status), Bg: p.BgPanel, Modifier: cell.ModifierBold})
	c.y++
	maxDetails := 2
	if adv {
		maxDetails = 8
	}
	for i, d := range r.Details {
		if i >= maxDetails {
			break
		}
		c.wrap("· "+d, p.LabelStyle(), 2)
	}
	if adv {
		var parts []string
		for _, m := range r.Measurements {
			parts = append(parts, fmt.Sprintf("%s %.2f%s", m.Name, m.Value, unitSuffix(m.Unit)))
		}
		if len(parts) > 0 {
			c.wrap(strings.Join(parts, " · "), p.MutedStyle(), 4)
		}
		for _, l := range r.Limitations {
			c.wrap("ⓘ "+l, p.MutedStyle(), 2)
		}
	}
}

func unitSuffix(u string) string {
	if u == "" {
		return ""
	}
	return " " + u
}

// renderHealthCard replaces the side card while the Diagnostics tab is open.
func renderHealthCard(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	p := s.Theme
	d := s.Diag
	f.RenderWidget(widgets.Block{
		Title: i18n.T(" Health "), TitleStyle: p.CardTitle(),
		Borders: widgets.BorderAll, BorderSymbols: widgets.SymbolsRounded,
		BorderStyle: p.CardBorder(), Style: p.PanelStyle(),
	}, area)
	c := newCursor(f, cell.NewRect(area.X+2, area.Y+1, area.Width-4, area.Height-2), s)
	if d == nil {
		c.line(i18n.T("No controller"), p.MutedStyle())
		return
	}
	for _, cat := range diag.AllCategories {
		st := d.Status(cat)
		if d.Avail[cat].Hidden() {
			continue
		}
		if !c.ok() {
			return
		}
		rowY := c.y
		c.f.Buffer.SetStringWithin(c.r.X, c.y, st.Icon()+" "+shortCategory(cat), cell.Style{Fg: statusColor(p, st), Bg: p.BgPanel}, 12)
		if c.r.Width > 12 {
			c.f.Buffer.SetStringWithin(c.r.X+12, c.y, st.Label(), p.MutedStyle(), c.r.Width-12)
		}
		if st != diag.Unsupported {
			cat := cat
			f.RegisterClickHandler(cell.NewRect(c.r.X, rowY, c.r.Width, 1), func(_ driver.MouseEvent) { d.Select(cat) })
		}
		c.y++
	}
}
