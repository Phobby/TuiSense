package views

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/widgets"
)

// Analog diagnostics (sticks, deadzone preview, triggers). Measurements and
// conclusions come from package diag; this file only presents them.

func cellStyle(bg, fg cell.Color) cell.Style {
	return cell.Style{Fg: fg, Bg: bg, Modifier: cell.ModifierBold}
}

func verdictColor(c *cursor, v diag.Verdict) cell.Color {
	return statusColor(c.p(), v.Status())
}

// stepHeader prints "Step n of total · title".
func stepHeader(c *cursor, n, total int, title string) {
	c.wrap(i18n.Tf("Step %d of %d · %s", n, total, title), c.p().SectionHeaderStyle(), 2)
}

// instruction prints a prominent instruction for the user.
func instruction(c *cursor, text string) {
	p := c.p()
	c.wrap("▸ "+text, cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold}, 3)
}

// ---- Sticks ----------------------------------------------------------------

func stickStepTitle(st *diag.StickTest) (int, string) {
	switch st.Step {
	case diag.StickRelease:
		return 1, i18n.T("Release the sticks")
	case diag.StickExplain:
		return 2, i18n.T("What the resting position means")
	case diag.StickUp, diag.StickDown, diag.StickLeft, diag.StickRight, diag.StickRotate, diag.StickRelease2:
		return 3, i18n.T("Full stick movement")
	case diag.StickDone:
		return 4, i18n.T("Result")
	}
	return 0, ""
}

func renderStickDiag(c *cursor, d *diag.Session, now time.Time) {
	st := d.Sticks
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	if n, title := stickStepTitle(st); n > 0 {
		stepHeader(c, n, 4, title)
	}
	switch st.Step {
	case diag.StickIdle:
		c.wrap(i18n.T("This test checks whether your sticks rest at the center when released (no \"drift\") and whether they move through their full range. It takes about half a minute."), p.LabelStyle(), 5)
		var items []choice
		if adv {
			for _, dur := range diag.StickDurations {
				dur := dur
				items = append(items, choice{Kind: kindOption, ID: fmt.Sprintf("stick_dur_%d", int(dur.Seconds())), Label: fmt.Sprintf("%ds", int(dur.Seconds())),
					Selected: st.Duration == dur, OnSelect: func() { st.Duration = dur }})
			}
		}
		items = append(items, choice{ID: "stick_start", Label: i18n.T("▶ Start stick test"), OnSelect: func() { st.Start(time.Now()) }})
		if adv {
			items = append(items, choice{ID: "stick_reset_live", Label: i18n.T("Reset min/max"), OnSelect: st.ResetLive})
		}
		c.pills("", 0, items)
	case diag.StickRelease, diag.StickRelease2:
		if st.Step == diag.StickRelease {
			instruction(c, i18n.T("Release both analog sticks and do not touch them."))
		} else {
			instruction(c, i18n.T("Release both sticks again and do not touch them."))
		}
		label := i18n.T("Measuring…")
		if st.Settling(now) {
			label = i18n.T("Waiting for the sticks to settle…")
		}
		progressLine(c, label, st.Progress(now), st.Cancel)
	case diag.StickExplain:
		renderStickExplain(c, st, adv)
	case diag.StickUp, diag.StickDown, diag.StickLeft, diag.StickRight, diag.StickRotate:
		renderStickMovement(c, st, now)
	case diag.StickDone:
		renderStickResults(c, d, st, adv)
	}

	if st.Step != diag.StickDone {
		renderStickScopes(c, st, adv)
	}
	if st.Step == diag.StickIdle || st.Step == diag.StickExplain || st.Step == diag.StickDone {
		renderDeadzonePreview(c, st, adv)
	}
	if adv {
		renderStickGraphs(c, st)
	}
}

func renderStickExplain(c *cursor, st *diag.StickTest, adv bool) {
	p := c.p()
	for i := 0; i < 2; i++ {
		a := st.Analysis[i]
		c.wrap(a.Verdict.Label()+": "+a.Headline, cellStyle(p.BgPanel, verdictColor(c, a.Verdict)), 5)
	}
	if !adv {
		c.wrap(i18n.T("● The center of each circle is the ideal neutral position."), p.LabelStyle(), 2)
		c.wrap(i18n.T("● The bright dot is where the stick actually is."), p.LabelStyle(), 2)
		c.wrap(i18n.T("● The yellow ring is the deadzone preview."), p.LabelStyle(), 2)
		c.wrap(i18n.T("Sticks rarely report exactly zero: a small offset of a few percent is a normal sensor tolerance. Drift matters when the stick rests outside the deadzone a game uses — then the camera or character moves on its own."), p.MutedStyle(), 6)
	}
	c.pills("", 0, []choice{
		{ID: "stick_continue", Label: i18n.T("Continue: movement test ›"), Selected: true, OnSelect: func() { st.Continue(time.Now()) }},
		{ID: "stick_finish", Label: i18n.T("Finish here"), OnSelect: st.Finish},
	})
}

func renderStickMovement(c *cursor, st *diag.StickTest, now time.Time) {
	switch st.Step {
	case diag.StickUp:
		instruction(c, i18n.T("Push both sticks all the way UP and hold."))
	case diag.StickDown:
		instruction(c, i18n.T("Push both sticks all the way DOWN and hold."))
	case diag.StickLeft:
		instruction(c, i18n.T("Push both sticks all the way LEFT and hold."))
	case diag.StickRight:
		instruction(c, i18n.T("Push both sticks all the way RIGHT and hold."))
	case diag.StickRotate:
		instruction(c, i18n.T("Rotate both sticks slowly around the outer edge."))
	}
	progressLine(c, i18n.T("Reached"), st.Progress(now), st.Cancel)
	var parts []string
	for i := 0; i < 2; i++ {
		lbl := i18n.T(st.Layout.StickLabels[i])
		if st.Step == diag.StickRotate {
			parts = append(parts, i18n.Tf("%s: edge %d/16", lbl, st.Travel[i].Coverage()))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %.0f%%", lbl, 100*st.Travel[i].Reach[int(st.Step-diag.StickUp)]))
	}
	c.line(strings.Join(parts, "   "), c.p().LabelStyle())
	c.pills("", 0, []choice{{ID: "stick_skip", Label: i18n.T("Skip this step ›"), OnSelect: func() { st.SkipStep(time.Now()) }}})
}

func renderStickResults(c *cursor, d *diag.Session, st *diag.StickTest, adv bool) {
	p := c.p()
	for i := 0; i < 2; i++ {
		a := st.Analysis[i]
		if a.Headline == "" {
			continue
		}
		c.line("● "+i18n.T(st.Layout.StickLabels[i])+" — "+a.Verdict.Label(), cellStyle(p.BgPanel, verdictColor(c, a.Verdict)))
		c.wrap(a.Headline, p.LabelStyle(), 6)
		if adv {
			ra := a.RestA
			c.wrap(i18n.Tf("offset %.2f%% (%.1f counts) · avg %.2f%% · max %.2f%% · σ %.2f%% · n=%d · %.0f Hz",
				ra.OffsetPct, ra.OffsetRaw, ra.AvgDevPct, ra.MaxDevPct, ra.NoisePct, ra.N, ra.RateHz), p.ValueStyle(), 3)
			if a.HasRestB {
				c.wrap(i18n.Tf("2nd rest %.2f%% · shift %.2f%% · coverage %d/16 · outer radius %.0f%%",
					a.RestB.OffsetPct, a.ShiftPct, a.Travel.Coverage(), a.Travel.OuterRadiusPct()), p.ValueStyle(), 2)
			}
		}
		for k, e := range a.Explanation {
			if k >= 2 {
				break
			}
			c.wrap("· "+e, p.MutedStyle(), 5)
		}
		for _, e := range a.Advice {
			c.wrap("→ "+e, p.MutedStyle(), 4)
		}
	}
	if st.Preliminary() {
		c.wrap(i18n.T("Only the resting step was run; the movement step makes the result more reliable."), p.MutedStyle(), 2)
	}
	if r, ok := d.Results[diag.CatSticks]; ok && adv {
		for _, l := range r.Limitations {
			c.wrap("ⓘ "+l, p.MutedStyle(), 2)
		}
	}
	c.pills("", 0, []choice{{ID: "stick_again", Label: i18n.T("↻ Run the test again"), OnSelect: func() { st.Start(time.Now()) }}})
	renderStickScopes(c, st, adv)
}

// renderStickScopes draws both sticks side by side with a few live values
// (beside each scope when there is room, otherwise below it).
func renderStickScopes(c *cursor, st *diag.StickTest, adv bool) {
	p := c.p()
	colW := c.r.Width / 2
	scopeH := uint16(min(max(c.remaining()-6, 3), 8))
	if c.remaining() < int(scopeH)+2 {
		return
	}
	type infoLine struct {
		text  string
		style cell.Style
	}
	info := func(i int) []infoLine {
		l := st.Live[i]
		dz := st.Preview(l.Norm)
		if !l.Valid {
			return []infoLine{{i18n.T("Unavailable"), p.MutedStyle()}}
		}
		if !adv {
			state := i18n.T("● at rest")
			if dz.RawPct > 10 {
				state = i18n.T("● moving")
			}
			out := []infoLine{{state, p.LabelStyle()}, {i18n.Tf("%.0f%% off", dz.RawPct), p.MutedStyle()}}
			if dz.Ignored {
				out = append(out, infoLine{i18n.T("in deadzone"), cell.Style{Fg: p.Warning, Bg: p.BgPanel}})
			}
			return out
		}
		j := l.JitterPct()
		out := []infoLine{
			{i18n.Tf("raw %.0f %.0f", l.Raw[0], l.Raw[1]), p.ValueStyle()},
			{fmt.Sprintf("x %+.3f y %+.3f", l.Norm[0], l.Norm[1]), p.LabelStyle()},
			{i18n.Tf("%.1f%% off center", dz.RawPct), p.LabelStyle()},
			{i18n.Tf("jitter %.2f/%.2f%%", j[0], j[1]), p.MutedStyle()},
		}
		if !math.IsInf(l.Min[0], 1) {
			out = append(out, infoLine{fmt.Sprintf("X %.0f…%.0f  Y %.0f…%.0f", l.Min[0], l.Max[0], l.Min[1], l.Max[1]), p.MutedStyle()})
		}
		if names := st.Layout.AxisNames; names[2*i] != "" {
			out = append(out, infoLine{names[2*i] + "/" + names[2*i+1], p.MutedStyle()})
		}
		return out
	}
	top := c.y
	below := 0
	for i := 0; i < 2; i++ {
		x := c.r.X + uint16(i)*colW
		c.f.Buffer.SetStringWithin(x, top, i18n.T(st.Layout.StickLabels[i]), p.SectionHeaderStyle(), colW)
		// Braille dots are square (2×4 per 1×2-ratio cell): a circle needs width = 2×height.
		sw := min(colW-2, scopeH*2+1)
		drawStickScope(c, cell.NewRect(x, top+1, sw, scopeH), st, i, adv)
		lines := info(i)
		side := cell.NewRect(x+sw+1, top+1, colW-sw-2, scopeH)
		if side.Width >= 14 {
			ic := newCursor(c.f, side, c.s)
			for _, ln := range lines {
				ic.line(ln.text, ln.style)
			}
			continue
		}
		n := min(len(lines), max(c.remaining()-int(scopeH)-1, 0))
		for k := 0; k < n; k++ {
			copyValue(c.f, cell.NewRect(x, top+1+scopeH+uint16(k), colW-1, 1), c.s, lines[k].text, cleanCopyText(lines[k].text), lines[k].style)
		}
		below = max(below, n)
	}
	c.y = top + 1 + scopeH + uint16(below)
}

// drawStickScope plots: rim, deadzone preview ring, outer boundary reached,
// center cross, trail, the raw position and (Advanced) the effective position.
func drawStickScope(c *cursor, area cell.Rect, st *diag.StickTest, i int, adv bool) {
	if area.Width < 6 || area.Height < 3 {
		return
	}
	p := c.p()
	cv := namedCanvas(fmt.Sprintf("stick_%d", i))
	cv.Reset(area.Width, area.Height)
	vw, vh := int(area.Width)*2, int(area.Height)*4
	cx, cy := vw/2, vh/2
	rx, ry := float64(vw/2-1), float64(vh/2-1)
	plot := func(nx, ny float64, s cell.Style) { cv.Set(cx+int(math.Round(nx*rx)), cy+int(math.Round(ny*ry)), s) }
	rim := cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
	dzStyle := cell.Style{Fg: p.Warning, Bg: p.BgPanel}
	reach := cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel}
	tr := st.Travel[i]
	for a := 0.0; a < 2*math.Pi; a += 0.05 {
		plot(math.Cos(a), math.Sin(a), rim)
		if r := st.PreviewDeadzone / 100; r > 0 {
			if st.AxialPreview {
				x, y := math.Cos(a), math.Sin(a)
				scale := r / math.Max(math.Abs(x), math.Abs(y))
				plot(scale*x, scale*y, dzStyle)
			} else {
				plot(r*math.Cos(a), r*math.Sin(a), dzStyle)
			}
		}
		sec := int(a/(2*math.Pi)*16) % 16
		if r := tr.Sector[sec]; r >= 0.3 {
			r = math.Min(r, 1)
			plot(r*math.Cos(a), r*math.Sin(a), reach)
		}
	}
	for k := -2; k <= 2; k++ {
		cv.Set(cx+k, cy, rim)
		cv.Set(cx, cy+k, rim)
	}
	trail := cell.Style{Fg: p.AccentBlue, Bg: p.BgPanel}
	for _, pt := range st.Live[i].Trail {
		plot(pt[0], pt[1], trail)
	}
	if st.Analysis[i].RestA.N > 0 {
		center := st.Analysis[i].RestA.Mean
		plot(center[0], center[1], cell.Style{Fg: p.AccentPink, Bg: p.BgPanel})
	}
	n := st.Live[i].Norm
	if !st.Live[i].Valid {
		c.f.RenderWidget(cv, area)
		return
	}
	{
		eff := st.Preview(n).Effective
		plot(eff[0], eff[1], cell.Style{Fg: p.Success, Bg: p.BgPanel, Modifier: cell.ModifierBold})
	}
	dot := cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}
	px, py := cx+int(math.Round(n[0]*rx)), cy+int(math.Round(n[1]*ry))
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			cv.Set(px+dx, py+dy, dot)
		}
	}
	c.f.RenderWidget(cv, area)
}

// renderDeadzonePreview explains raw input vs. deadzone vs. effective input.
func renderDeadzonePreview(c *cursor, st *diag.StickTest, adv bool) {
	if c.remaining() < 4 {
		return
	}
	p := c.p()
	c.gap()
	c.line(i18n.T("Deadzone preview"), p.SectionHeaderStyle())
	var items []choice
	for _, v := range diag.DeadzonePresets {
		v := v
		items = append(items, choice{Kind: kindOption, ID: fmt.Sprintf("dz_preview_%.0f", v), Label: fmt.Sprintf("%.0f%%", v),
			Selected: st.PreviewDeadzone == v, OnSelect: func() { st.PreviewDeadzone = v }})
	}
	if c.s.DiagnosticDeadzoneSlider == nil {
		c.s.DiagnosticDeadzoneSlider = widgets.NewSliderState(int(st.PreviewDeadzone))
	}
	c.s.DiagnosticDeadzoneSlider.Value = int(st.PreviewDeadzone)
	if c.ok() {
		c.f.RenderWidget(widgets.Slider{ID: "diag_deadzone_slider", State: c.s.DiagnosticDeadzoneSlider, Min: 0, Max: 25,
			TrackStyle: p.MutedStyle(), FilledStyle: cell.Style{Fg: p.Warning, Bg: p.BgPanel}, ThumbStyle: p.ValueStyle(),
			OnChange: func(v int) { st.PreviewDeadzone = float64(v) }}, cell.NewRect(c.r.X, c.y, c.r.Width, 1))
		c.y++
	}
	c.pills(i18n.T("Size:"), 6, items)
	c.pills("", 0, []choice{
		{Kind: kindOption, ID: "dz_radial", Label: "Radial", Selected: !st.AxialPreview, OnSelect: func() { st.AxialPreview = false }},
		{Kind: kindOption, ID: "dz_axial", Label: "Axial", Selected: st.AxialPreview, OnSelect: func() { st.AxialPreview = true }},
		{ID: "dz_less", Label: "−1%", OnSelect: func() { st.PreviewDeadzone = math.Max(0, st.PreviewDeadzone-1) }},
		{ID: "dz_more", Label: "+1%", OnSelect: func() { st.PreviewDeadzone = math.Min(25, st.PreviewDeadzone+1) }},
	})
	c.line(fmt.Sprintf("%.0f%% · RAW cyan · FILTERED green · resting center pink", st.PreviewDeadzone), p.MutedStyle())
	c.wrap(i18n.T("Preview only — does not change game input."), cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 2)
	if !adv {
		c.wrap(i18n.T("A deadzone ignores small movements near the center of an analog stick. Increasing it can hide unwanted movement, but also reduces precision."), p.MutedStyle(), 5)
	}
	for i := 0; i < 2 && c.ok(); i++ {
		if !st.Live[i].Valid {
			c.line(i18n.T("Unavailable"), p.MutedStyle())
			continue
		}
		dz := st.Preview(st.Live[i].Norm)
		eff := 100 * math.Hypot(dz.Effective[0], dz.Effective[1])
		lbl := i18n.T(st.Layout.StickLabels[i])
		var text string
		col := p.Success
		switch {
		case dz.RawPct < 0.05:
			text = i18n.Tf("%s: centered → game sees 0%%", lbl)
		case dz.Ignored:
			text, col = i18n.Tf("%s: %.1f%% raw → inside the deadzone, game sees 0%%", lbl, dz.RawPct), p.Warning
		default:
			text = i18n.Tf("%s: %.1f%% raw → game sees %.0f%%", lbl, dz.RawPct, eff)
		}
		c.wrap(text, cell.Style{Fg: col, Bg: p.BgPanel}, 2)
	}
	if adv && c.ok() {
		c.wrap(i18n.T("1 physical movement (your thumb) → 2 raw input (what the controller reports, cyan dot) → 3 deadzone filter (yellow ring) → 4 effective game input (green dot)."), p.MutedStyle(), 3)
	}
}

// renderStickGraphs draws X/Y time series for both sticks (Advanced).
func renderStickGraphs(c *cursor, st *diag.StickTest) {
	if c.remaining() < 4 {
		return
	}
	p := c.p()
	c.gap()
	c.line(i18n.T("Time series (X cyan, Y pink, ±100%)"), p.SectionHeaderStyle())
	colW := c.r.Width / 2
	h := uint16(min(max(c.remaining(), 2), 4))
	top := c.y
	for i := 0; i < 2; i++ {
		x := c.r.X + uint16(i)*colW
		timeSeries(c, fmt.Sprintf("stick_ts_%d", i), cell.NewRect(x, top, colW-1, h),
			[][]float64{st.Live[i].HistX.Values(), st.Live[i].HistY.Values()}, []cell.Color{p.AccentCyan, p.AccentPink}, true)
	}
	c.y = top + h
}

// timeSeries plots series as braille lines. Bipolar series span [-1, 1]
// around a center line; unipolar ones span [0, 1].
func timeSeries(c *cursor, id string, area cell.Rect, series [][]float64, colors []cell.Color, bipolar bool) {
	if area.Width < 4 || area.Height < 1 {
		return
	}
	p := c.p()
	cv := namedCanvas(id)
	cv.Reset(area.Width, area.Height)
	vw, vh := int(area.Width)*2, int(area.Height)*4
	toY := func(v float64) int {
		if bipolar {
			v = (v + 1) / 2
		}
		v = clamp01(v)
		return int(math.Round(float64(vh-1) * (1 - v)))
	}
	axis := cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
	base := vh - 1
	if bipolar {
		base = toY(0)
	}
	for x := 0; x < vw; x += 2 {
		cv.Set(x, base, axis)
	}
	for k, data := range series {
		if len(data) > vw {
			data = data[len(data)-vw:]
		}
		st := cell.Style{Fg: colors[k%len(colors)], Bg: p.BgPanel}
		off := vw - len(data)
		prev := -1
		for x, v := range data {
			y := toY(v)
			if prev >= 0 {
				lo, hi := min(prev, y), max(prev, y)
				for yy := lo; yy <= hi; yy++ {
					cv.Set(off+x, yy, st)
				}
			} else {
				cv.Set(off+x, y, st)
			}
			prev = y
		}
	}
	c.f.RenderWidget(cv, area)
}

// ---- Triggers --------------------------------------------------------------

func triggerStepInfo(tt *diag.TriggerTest) (int, string, string) {
	l, r := tt.Layout.TriggerLabels[0], tt.Layout.TriggerLabels[1]
	n, total := tt.StepIndex(), len(tt.Steps())
	_ = total
	switch tt.Step {
	case diag.TriggerRest:
		return n, i18n.T("Resting position"), i18n.T("Release both triggers and do not touch them.")
	case diag.TriggerPressL:
		if tt.Kind == device.TriggerDigital {
			return n, i18n.T("Press test"), i18n.Tf("Press %s once and release it.", l)
		}
		return n, i18n.T("Full press"), i18n.Tf("Press %s all the way down and hold.", l)
	case diag.TriggerPressR:
		if tt.Kind == device.TriggerDigital {
			return n, i18n.T("Press test"), i18n.Tf("Press %s once and release it.", r)
		}
		return n, i18n.T("Full press"), i18n.Tf("Press %s all the way down and hold.", r)
	case diag.TriggerSlowL:
		return n, i18n.T("Smooth movement"), i18n.Tf("Press %s slowly all the way down, then release it slowly.", l)
	case diag.TriggerSlowR:
		return n, i18n.T("Smooth movement"), i18n.Tf("Press %s slowly all the way down, then release it slowly.", r)
	case diag.TriggerReturn:
		return n, i18n.T("Return to rest"), i18n.T("Release both triggers again.")
	}
	return 0, "", ""
}

func renderTriggerDiag(c *cursor, d *diag.Session, now time.Time) {
	tt := d.Triggers
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	kind := map[device.TriggerKind]string{
		device.TriggerAnalog:     i18n.T("analog axis"),
		device.TriggerDigital:    i18n.T("digital button (on/off)"),
		device.TriggerSharedAxis: i18n.T("shared analog axis (both triggers on one axis)"),
	}[tt.Kind]
	c.line(i18n.T("Reported as: ")+kind, p.MutedStyle())

	switch {
	case tt.Running():
		n, title, text := triggerStepInfo(tt)
		stepHeader(c, n, len(tt.Steps()), title)
		instruction(c, text)
		label := i18n.T("Time left")
		if tt.Settling(now) {
			label = i18n.T("Settling…")
		}
		progressLine(c, label, tt.Progress(now), tt.Cancel)
		if tt.Target() >= 0 {
			c.pills("", 0, []choice{{ID: "trigger_skip", Label: i18n.T("Skip this step ›"), OnSelect: func() { tt.SkipStep(time.Now()) }}})
		}
	case tt.Step == diag.TriggerDone:
		renderTriggerResults(c, d, tt, adv)
	default:
		c.wrap(i18n.T("A guided test: resting position, full press, smooth movement and return to rest for each trigger. Nothing is changed on the controller."), p.LabelStyle(), 3)
		if tt.Kind == device.TriggerDigital {
			c.wrap(i18n.T("In this input mode the triggers are on/off buttons, so a press test replaces the analog range checks."), p.MutedStyle(), 2)
		}
		if tt.Kind == device.TriggerSharedAxis {
			c.wrap(i18n.T("In this input mode both triggers share one axis; pressing both at once cancels out."), p.MutedStyle(), 2)
		}
		c.pills("", 0, []choice{{ID: "triggers_start", Label: i18n.T("▶ Start trigger test"), OnSelect: func() { tt.Start(time.Now()) }}})
	}

	accent := uiwidgets.AccentColor(d.Ctx.Desc)
	target := tt.Target()
	for i := 0; i < 2 && c.ok(); i++ {
		l := tt.Live[i]
		val := fmt.Sprintf("%3.0f%%", 100*l.Norm)
		if !l.Valid {
			val = i18n.T("Unavailable")
		}
		if adv && l.Valid {
			val += i18n.Tf(" raw %.0f", l.Raw)
		}
		col := accent
		if target == i {
			col = p.Warning
		}
		lbl := tt.Layout.TriggerLabels[i]
		if target == i {
			lbl = "▶ " + lbl
		}
		c.gauge(fmt.Sprintf("trigger_bar_%d", i), lbl, l.Norm, val, col)
		if target == i && tt.Kind != device.TriggerDigital && c.ok() {
			// A second bar row makes the active trigger's bar large.
			c.gauge(fmt.Sprintf("trigger_bar_big_%d", i), "", l.Norm, "", col)
		}
	}
	if tt.Kind == device.TriggerSharedAxis && c.ok() {
		c.line(i18n.Tf("Shared axis: %+.0f%%", 100*tt.SharedAxis), p.MutedStyle())
	}
	if (adv || tt.Step == diag.TriggerSlowL || tt.Step == diag.TriggerSlowR) && c.remaining() >= 3 {
		h := uint16(min(c.remaining(), 3))
		timeSeries(c, "trigger_ts", cell.NewRect(c.r.X, c.y, c.r.Width, h),
			[][]float64{tt.Live[0].History.Values(), tt.Live[1].History.Values()}, []cell.Color{p.AccentCyan, p.AccentPink}, false)
		c.y += h
	}
	renderResistance(c, tt, now)
}

func renderTriggerResults(c *cursor, d *diag.Session, tt *diag.TriggerTest, adv bool) {
	p := c.p()
	for i := 0; i < 2 && c.ok(); i++ {
		a := tt.Analysis[i]
		c.line("● "+tt.Layout.TriggerLabels[i]+" — "+a.Verdict.Label(), cellStyle(p.BgPanel, verdictColor(c, a.Verdict)))
		c.wrap(a.Headline, p.LabelStyle(), 5)
		for _, ch := range a.Checks {
			col := p.Success
			switch ch.State {
			case diag.CheckWarn:
				col = p.Warning
			case diag.CheckNotObserved:
				col = p.AccentPurple
			case diag.CheckNotApplicable:
				col = p.FgMuted
			}
			c.kv(fmt.Sprintf("  %s %s", ch.State.Icon(), ch.Label), ch.Text, 20, cell.Style{Fg: col, Bg: p.BgPanel})
		}
		limit := 1
		if adv {
			limit = 3
		}
		for k, e := range a.Explanation {
			if k >= limit {
				break
			}
			c.wrap("· "+e, p.MutedStyle(), 3)
		}
		for _, e := range a.Advice {
			c.wrap("→ "+e, p.MutedStyle(), 2)
		}
	}
	if r, ok := d.Results[diag.CatTriggers]; ok && adv {
		for _, l := range r.Limitations {
			c.wrap("ⓘ "+l, p.MutedStyle(), 2)
		}
	}
	c.pills("", 0, []choice{{ID: "triggers_again", Label: i18n.T("↻ Run the test again"), OnSelect: func() { tt.Start(time.Now()) }}})
}

func renderResistance(c *cursor, tt *diag.TriggerTest, now time.Time) {
	if !tt.ResistanceSupported || !c.ok() || tt.Running() {
		return
	}
	switch {
	case tt.ResistanceActive:
		left := tt.ResistanceUntil.Sub(now).Seconds()
		c.pills(i18n.Tf("Resistance ON · %.0fs", math.Max(0, left)), 22, []choice{{ID: "resistance_stop", Label: i18n.T("■ Stop"), OnSelect: tt.StopResistance}})
	case tt.ResistanceFelt == nil && tt.LastResistance():
		yes, no := true, false
		c.pills(i18n.T("Did you feel it?"), 17, []choice{
			{ID: "resistance_yes", Label: i18n.T("Yes"), OnSelect: func() { tt.ResistanceFelt = &yes }},
			{ID: "resistance_no", Label: i18n.T("No"), OnSelect: func() { tt.ResistanceFelt = &no }},
		})
	default:
		c.pills(i18n.T("Adaptive"), 9, []choice{{ID: "resistance_start", Label: i18n.T("Apply test resistance (5 s)"), OnSelect: func() { tt.StartResistance(time.Now()) }}})
	}
}

// ---- Stick category: deadzone test | snapback test -------------------------

// renderStickCategory shows the two stick sub-tests behind a selector. While
// one of them is measuring, the other cannot be opened.
func renderStickCategory(c *cursor, d *diag.Session, now time.Time) {
	if d.StickSubTest == diag.StickSubCircularity {
		renderCircularity(c, d, now)
		return
	}
	snap := d.StickSubTest == diag.StickSubSnapback && d.Snapback != nil
	busy := d.Sticks.Running() || (d.Snapback != nil && d.Snapback.Running())
	items := []choice{{Kind: kindOption, ID: "stick_sub_deadzone", Label: i18n.T("Deadzone test"), Selected: !snap}}
	if d.Snapback != nil {
		items = append(items, choice{Kind: kindOption, ID: "stick_sub_snapback", Label: i18n.T("Snapback test"), Selected: snap})
	}
	items = append(items, choice{Kind: kindOption, ID: "stick_sub_circularity", Label: "Circularity", OnSelect: func() {
		if !busy {
			d.StickSubTest = diag.StickSubCircularity
		}
	}})
	if !busy {
		items[0].OnSelect = func() { d.StickSubTest = diag.StickSubDeadzone }
		if len(items) > 1 {
			items[1].OnSelect = func() { d.StickSubTest = diag.StickSubSnapback }
		}
	}
	c.pills("", 0, items)
	if snap {
		renderSnapbackDiag(c, d)
		return
	}
	renderStickDiag(c, d, now)
}

// ---- Snapback ----------------------------------------------------------------

func renderSnapbackDiag(c *cursor, d *diag.Session) {
	sb := d.Snapback
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	switch {
	case sb.Running():
		instruction(c, i18n.T("Push a stick all the way to the edge, then let go instantly. Repeat in different directions (both sticks)."))
		progressLine(c, i18n.T("Releases"), sb.Progress(), sb.Finish)
		var parts []string
		for i := 0; i < 2; i++ {
			parts = append(parts, i18n.Tf("%s: %d/%d", i18n.T(sb.Layout.StickLabels[i]), len(sb.Events[i]), diag.SnapbackTarget))
		}
		c.line(strings.Join(parts, "   "), p.LabelStyle())
		c.pills("", 0, []choice{{ID: "snap_finish", Label: i18n.T("■ Finish and show the result"), OnSelect: sb.Finish}})
	case sb.Done():
		c.pills("", 0, []choice{{ID: "snap_again", Label: i18n.T("↻ Run the test again"), OnSelect: func() { sb.Start(time.Now()) }}})
		for i := 0; i < 2 && c.ok(); i++ {
			a := sb.Analyze(i)
			col := p.FgMuted
			if a.Events > 0 {
				col = verdictColor(c, a.Verdict)
			}
			c.line("● "+i18n.T(sb.Layout.StickLabels[i])+" — "+a.Verdict.Label(), cellStyle(p.BgPanel, col))
			c.wrap(a.Headline, p.LabelStyle(), 5)
			if adv && a.Events > 0 {
				c.wrap(i18n.Tf("releases %d · max %.1f%% · avg %.1f%% · settle avg %.0f ms / max %.0f ms · %.0f Hz · slow returns ignored %d",
					a.Events, a.MaxOvershootPct, a.MeanOvershootPct, a.MeanSettleMs, a.MaxSettleMs, a.RateHz, a.SlowReleases), p.ValueStyle(), 3)
			}
			limit := 1
			if adv {
				limit = 3
			}
			for k, e := range a.Explanation {
				if k >= limit {
					break
				}
				c.wrap("· "+e, p.MutedStyle(), 4)
			}
			for _, e := range a.Advice {
				c.wrap("→ "+e, p.MutedStyle(), 3)
			}
		}

	default:
		c.wrap(i18n.T("Snapback: when a stick is released suddenly from the edge, its spring can throw it past the center for a moment, so a game briefly sees the opposite direction."), p.LabelStyle(), 6)
		c.wrap(i18n.Tf("This test measures how far each stick swings past its resting point and how fast it settles. You will release each stick %d times.", diag.SnapbackTarget), p.MutedStyle(), 5)
		c.pills("", 0, []choice{{ID: "snap_start", Label: i18n.T("▶ Start snapback test"), OnSelect: func() { sb.Start(time.Now()) }}})
	}
	renderSnapTraces(c, sb)
	if !sb.Running() && c.remaining() >= 4 {
		c.gap()
		c.line(i18n.T("Deadzone preview"), p.SectionHeaderStyle())
		var items []choice
		for _, v := range diag.DeadzonePresets {
			v := v
			items = append(items, choice{Kind: kindOption, ID: fmt.Sprintf("snap_dz_%.0f", v), Label: fmt.Sprintf("%.0f%%", v),
				Selected: d.Sticks.PreviewDeadzone == v, OnSelect: func() { d.Sticks.PreviewDeadzone, sb.PreviewDeadzone = v, v }})
		}
		c.pills(i18n.T("Size:"), 6, items)
		c.wrap(i18n.T("Preview only — does not change game input."), cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 2)
	}
}

// renderSnapTraces plots the most recent release of each stick along its push
// direction: the line falls from the edge and dips below the axis when the
// stick overshoots (the deadzone preview is drawn as a dotted level).
func renderSnapTraces(c *cursor, sb *diag.SnapbackTest) {
	if c.remaining() < 5 {
		return
	}
	p := c.p()
	c.gap()
	c.line(i18n.T("Last release (push side up, overshoot below the line)"), p.SectionHeaderStyle())
	colW := c.r.Width / 2
	h := uint16(4)
	top := c.y
	for i := 0; i < 2; i++ {
		x := c.r.X + uint16(i)*colW
		c.f.Buffer.SetStringWithin(x, top, i18n.T(sb.Layout.StickLabels[i]), p.MutedStyle(), colW)
		evs := sb.Events[i]
		if len(evs) == 0 {
			copyValue(c.f, cell.NewRect(x, top+1, colW, 1), c.s, i18n.T("no release yet"), i18n.T("no release yet"), p.MutedStyle())
			continue
		}
		e := evs[len(evs)-1]
		proj := e.Projection()
		dz := make([]float64, len(proj))
		for k := range dz {
			dz[k] = -sb.PreviewDeadzone / 100
		}
		timeSeries(c, fmt.Sprintf("snap_ts_%d", i), cell.NewRect(x, top+1, colW-1, h),
			[][]float64{dz, proj}, []cell.Color{p.Warning, p.AccentCyan}, true)
	}
	c.y = top + 1 + h
}

func renderCircularity(c *cursor, d *diag.Session, now time.Time) {
	st := d.Sticks
	c.pills("", 0, []choice{{ID: "circle_back", Label: "‹ Stick analyzer", OnSelect: func() { st.Cancel(); d.StickSubTest = diag.StickSubDeadzone }}})
	instruction(c, "Rotate both sticks slowly around the full outer edge. Gate shapes differ between controllers.")
	if st.Step == diag.StickRotate {
		progressLine(c, "Coverage", st.Progress(now), st.Cancel)
	} else {
		c.pills("", 0, []choice{{ID: "circle_start", Label: "Start circularity test", OnSelect: func() { st.StartCircularity(time.Now()) }}})
	}
	renderStickScopes(c, st, c.s.Mode == state.ModeAdvanced)
	for i, tr := range st.Travel {
		v := tr.Circularity()
		c.wrap(fmt.Sprintf("%s: coverage %d/16 · radius mean %.1f%% · spread %.1f%% · σ %.1f%% · directional asymmetry %.1f%%", st.Layout.StickLabels[i], v.Covered, v.MeanRadiusPct, v.RadiusSpreadPct, v.RadiusStdPct, v.AsymmetryPct), c.p().ValueStyle(), 3)
	}
	c.wrap("Observed envelope only. Low coverage means incomplete movement; a noncircular path does not prove a defect.", c.p().MutedStyle(), 3)
}
