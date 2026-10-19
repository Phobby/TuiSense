package views

import (
	"fmt"
	"math"
	"time"

	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/widgets"
	"github.com/ygzztrk/tuisense/internal/diag"
	"github.com/ygzztrk/tuisense/internal/state"
	uiwidgets "github.com/ygzztrk/tuisense/internal/ui/widgets"
)

// Reusable canvases for the diagnostics graphics (single UI instance).
var diagCanvas = struct {
	stick [2]*widgets.Canvas
	touch *widgets.Canvas
}{}

func canvasFor(slot **widgets.Canvas) *widgets.Canvas {
	if *slot == nil {
		*slot = widgets.NewCanvas(20, 6)
	}
	return *slot
}

func progressLine(c *cursor, label string, frac float64, onCancel func()) {
	if !c.ok() {
		return
	}
	p := c.p()
	lw := uint16(cell.StringWidth(label) + 1)
	c.f.Buffer.SetStringWithin(c.r.X, c.y, label, cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}, lw)
	cancelW := uint16(11)
	if c.r.Width > lw+cancelW+6 {
		c.f.RenderWidget(widgets.LineGauge{ID: "diag_progress", Ratio: clamp01(frac), HideLabel: true,
			FilledStyle: cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel}, UnfilledStyle: cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}},
			cell.NewRect(c.r.X+lw, c.y, c.r.Width-lw-cancelW-1, 1))
		pillRow(c.f, c.r.X+c.r.Width-cancelW, c.y, c.limit(), c.s, "", 0, []choice{{ID: "diag_cancel", Label: "✕ Cancel", OnSelect: onCancel}})
	}
	c.y++
}

// ---- Sticks ----------------------------------------------------------------

func renderStickDiag(c *cursor, d *diag.Session, now time.Time) {
	st := d.Sticks
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	switch st.Phase {
	case diag.StickStationary:
		progressLine(c, "Keep BOTH sticks released…", st.Progress(now), st.Cancel)
	case diag.StickRange:
		progressLine(c, "Rotate both sticks slowly along the rim…", st.Progress(now), st.Cancel)
	case diag.StickRecenter:
		progressLine(c, "Release both sticks again…", st.Progress(now), st.Cancel)
	default:
		c.wrap("Step 1: let go of both sticks and keep the controller still, then start.", p.LabelStyle(), 2)
		var items []choice
		if adv {
			for _, dur := range diag.StickDurations {
				dur := dur
				items = append(items, choice{ID: fmt.Sprintf("stick_dur_%d", int(dur.Seconds())), Label: fmt.Sprintf("%ds", int(dur.Seconds())),
					Selected: st.Duration == dur, OnSelect: func() { st.Duration = dur }})
			}
		}
		items = append(items, choice{ID: "stick_start", Label: "▶ Start drift test", OnSelect: func() { st.StartStationary(time.Now()) }})
		if st.Phase == diag.StickDone {
			items = append(items, choice{ID: "stick_range", Label: "↻ Step 2: movement & recenter", OnSelect: func() { st.StartRange(time.Now()) }})
		}
		if adv {
			items = append(items, choice{ID: "stick_reset_live", Label: "Reset min/max", OnSelect: st.ResetLive})
		}
		c.pills("", 0, items)
	}

	// Two scopes side by side
	colW := c.r.Width / 2
	reserve := 1
	if st.Phase == diag.StickDone {
		reserve = 4
		if adv {
			reserve = 6
		}
	}
	scopeH := uint16(min(max(c.remaining()-reserve-1, 3), 10))
	if c.remaining() >= int(scopeH)+1 {
		top := c.y
		for i := 0; i < 2; i++ {
			x := c.r.X + uint16(i)*colW
			lbl := st.Layout.StickLabels[i]
			c.f.Buffer.SetStringWithin(x, top, lbl, p.SectionHeaderStyle(), colW)
			// Braille dots are square (2×4 per 1×2-ratio cell): a circle needs width = 2×height.
			sw := min(colW-2, scopeH*2+1)
			drawStickScope(c, cell.NewRect(x, top+1, sw, scopeH), st, i)
			info := cell.NewRect(x+sw+1, top+1, colW-sw-2, scopeH)
			if info.Width > 8 {
				ic := newCursor(c.f, info, c.s)
				l := st.Live[i]
				if adv {
					j := l.Jitter()
					ic.line(fmt.Sprintf("raw %4.0f %4.0f", l.Raw[0], l.Raw[1]), p.ValueStyle())
					ic.line(fmt.Sprintf("norm %+.3f %+.3f", l.Norm[0], l.Norm[1]), p.LabelStyle())
					ic.line(fmt.Sprintf("X %.0f…%.0f", l.Min[0], l.Max[0]), p.MutedStyle())
					ic.line(fmt.Sprintf("Y %.0f…%.0f", l.Min[1], l.Max[1]), p.MutedStyle())
					ic.line(fmt.Sprintf("jitter σ %.1f/%.1f", j[0], j[1]), p.MutedStyle())
					ic.line(fmt.Sprintf("deadzone %.0f%%", st.Deadzone[i]), p.MutedStyle())
				} else {
					mag := math.Hypot(l.Norm[0], l.Norm[1])
					state := "● at rest"
					if mag > 0.1 {
						state = "● moving"
					}
					ic.line(state, p.LabelStyle())
					ic.line(fmt.Sprintf("%.0f%% from center", 100*mag), p.MutedStyle())
				}
			}
		}
		c.y = top + 1 + scopeH
	}
	if adv && st.Phase == diag.StickIdle {
		c.sparkline("stick_hist", st.History[0].Values(), p.AccentBlue, 1)
	}
	if st.Phase == diag.StickDone {
		for i := 0; i < 2; i++ {
			a := st.Analysis[i]
			if a.Summary == "" {
				continue
			}
			c.wrap(a.Status.Icon()+" "+a.Summary, cellStyle(p.BgPanel, statusColor(p, a.Status)), 2)
			if adv {
				c.line(fmt.Sprintf("   offset %.1f counts (%.2f%%) · σ %.2f · p-p %.0f · max dev %.1f%% · n=%d",
					a.OffsetRaw, a.OffsetPct, a.NoiseRaw, a.PeakToPeak, a.MaxDevPct, a.Samples), p.MutedStyle())
				for _, e := range a.Explanation {
					c.wrap("   "+e, p.MutedStyle(), 2)
				}
			}
		}
		if r, ok := d.Results[diag.CatSticks]; ok && adv {
			for _, l := range r.Limitations {
				c.wrap("ⓘ "+l, p.MutedStyle(), 2)
			}
		}
	}
}

func cellStyle(bg, fg cell.Color) cell.Style {
	return cell.Style{Fg: fg, Bg: bg, Modifier: cell.ModifierBold}
}

// drawStickScope plots the stick: rim, deadzone ring, center cross, trail, dot.
func drawStickScope(c *cursor, area cell.Rect, st *diag.StickTest, i int) {
	if area.Width < 6 || area.Height < 3 {
		return
	}
	p := c.p()
	cv := canvasFor(&diagCanvas.stick[i])
	cv.Reset(area.Width, area.Height)
	vw, vh := int(area.Width)*2, int(area.Height)*4
	cx, cy := vw/2, vh/2
	rx, ry := float64(vw/2-1), float64(vh/2-1)
	plot := func(nx, ny float64, s cell.Style) { cv.Set(cx+int(nx*rx), cy+int(ny*ry), s) }
	rim := cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
	dz := cell.Style{Fg: p.Warning, Bg: p.BgPanel}
	for a := 0.0; a < 2*math.Pi; a += 0.06 {
		plot(math.Cos(a), math.Sin(a), rim)
		if r := st.Deadzone[i] / 100; r > 0 {
			plot(r*math.Cos(a), r*math.Sin(a), dz)
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
	dot := cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}
	n := st.Live[i].Norm
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			cv.Set(cx+int(n[0]*rx)+dx, cy+int(n[1]*ry)+dy, dot)
		}
	}
	c.f.RenderWidget(cv, area)
}

// ---- Buttons ---------------------------------------------------------------

func renderButtonDiag(c *cursor, d *diag.Session) {
	bt := d.Buttons
	p := c.p()
	accent := uiwidgets.AccentColor(d.Ctx.Desc)
	if target, ok := bt.Prompt(); ok {
		c.line(fmt.Sprintf("Press:  %s   (%d of %d)", target.Label, bt.GuidedIdx+1, len(bt.Layout.Buttons)), cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold})
		c.pills("", 0, []choice{{ID: "buttons_skip", Label: "Skip (not working)", OnSelect: bt.SkipCurrent}})
	} else {
		c.wrap("Press any button — it lights up here and on the controller.", p.LabelStyle(), 2)
		c.pills("", 0, []choice{
			{ID: "buttons_guided", Label: "▶ Guided: press each button", OnSelect: bt.StartGuided},
			{ID: "buttons_reset", Label: "↻ Reset counts", OnSelect: bt.Reset},
		})
	}
	x := c.r.X
	for _, b := range bt.Layout.Buttons {
		if !c.ok() {
			break
		}
		label := " " + b.Label
		if n := bt.Counts[b.ID]; n > 0 {
			label += fmt.Sprintf(" ·%d", n)
		}
		label += " "
		w := uint16(cell.StringWidth(label))
		if x+w > c.limit() {
			c.y++
			x = c.r.X
			if !c.ok() {
				break
			}
		}
		style := cell.Style{Fg: p.FgMuted, Bg: p.BgElevated}
		switch {
		case bt.Pressed.Has(b.ID):
			style = cell.Style{Fg: p.BgCanvas, Bg: accent, Modifier: cell.ModifierBold}
		case bt.Guided && bt.Outcomes[b.ID] == diag.OutcomeSkipped:
			style = cell.Style{Fg: p.Danger, Bg: p.BgElevated}
		case bt.Seen.Has(b.ID):
			style = cell.Style{Fg: p.Success, Bg: p.BgElevated}
		}
		if t, ok := bt.Prompt(); ok && t.ID == b.ID {
			style = cell.Style{Fg: p.BgCanvas, Bg: p.Warning, Modifier: cell.ModifierBold}
		}
		rect := cell.NewRect(x, c.y, w, 1)
		c.f.Buffer.SetStringWithin(x, c.y, label, style, w)
		nodeState := accessibility.NodeState(0)
		if bt.Pressed.Has(b.ID) {
			nodeState |= accessibility.StateSelected
		}
		c.f.RegisterAccessibility(accessibility.AccessibilityNode{ID: fmt.Sprintf("btn_diag_%d", b.ID), Role: accessibility.RoleGeneric,
			Label: b.Label, Value: fmt.Sprintf("count=%d", bt.Counts[b.ID]), State: nodeState, Bounds: rect})
		x += w + 1
	}
	c.y++
	if len(bt.Stuck) > 0 {
		c.line("! Some buttons were held for a long time — stuck, or simply held?", cell.Style{Fg: p.Warning, Bg: p.BgPanel})
	}
	if r, ok := d.Results[diag.CatButtons]; ok {
		renderResult(c, r, c.s.Mode == state.ModeAdvanced)
	}
}

// ---- Triggers --------------------------------------------------------------

func renderTriggerDiag(c *cursor, d *diag.Session, now time.Time) {
	tt := d.Triggers
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	switch tt.Phase {
	case diag.TriggerRest:
		progressLine(c, "Release both triggers…", tt.Progress(now), tt.Cancel)
	case diag.TriggerSweep:
		progressLine(c, "Press both fully, then release slowly…", tt.Progress(now), tt.Cancel)
	default:
		c.wrap("Tests resting position, full travel, and smoothness of both triggers.", p.LabelStyle(), 2)
		c.pills("", 0, []choice{{ID: "triggers_start", Label: "▶ Start trigger test", OnSelect: func() { tt.Start(time.Now()) }}})
	}
	accent := uiwidgets.AccentColor(d.Ctx.Desc)
	for i := 0; i < 2; i++ {
		l := tt.Live[i]
		val := fmt.Sprintf("%3.0f%%", 100*l.Norm)
		if adv {
			val += fmt.Sprintf(" raw %4.0f", l.Raw)
		}
		c.gauge(fmt.Sprintf("trigger_bar_%d", i), tt.Layout.TriggerLabels[i], l.Norm, val, accent)
		if adv && !math.IsInf(l.Min, 1) {
			c.line(fmt.Sprintf("          seen %.0f … %.0f (raw)", l.Min, l.Max), p.MutedStyle())
		}
	}
	if adv {
		c.sparkline("trigger_hist", tt.Live[0].History.Values(), accent, 1)
	}
	if tt.ResistanceSupported && c.ok() {
		switch {
		case tt.ResistanceActive:
			left := tt.ResistanceUntil.Sub(now).Seconds()
			c.pills(fmt.Sprintf("Resistance ON · %.0fs", math.Max(0, left)), 22, []choice{{ID: "resistance_stop", Label: "■ Stop", OnSelect: tt.StopResistance}})
		case tt.ResistanceFelt == nil && tt.LastResistance():
			yes, no := true, false
			c.pills("Did you feel it?", 17, []choice{
				{ID: "resistance_yes", Label: "Yes", OnSelect: func() { tt.ResistanceFelt = &yes }},
				{ID: "resistance_no", Label: "No", OnSelect: func() { tt.ResistanceFelt = &no }},
			})
		default:
			c.pills("Adaptive", 9, []choice{{ID: "resistance_start", Label: "Apply test resistance (5 s)", OnSelect: func() { tt.StartResistance(time.Now()) }}})
		}
	}
	if r, ok := d.Results[diag.CatTriggers]; ok {
		renderResult(c, r, adv)
	}
}

// ---- Touchpad --------------------------------------------------------------

func renderTouchDiag(c *cursor, d *diag.Session) {
	t := d.Touch
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	c.wrap("Slide over the whole pad, try two fingers, and click it.", p.LabelStyle(), 2)
	c.pills("", 0, []choice{{ID: "touch_reset", Label: "↻ Reset", OnSelect: t.Reset}})
	h := uint16(min(max(c.remaining()-3, 3), 7))
	w := min(c.r.Width/2, h*4)
	if c.remaining() >= int(h) {
		top := c.y
		cv := canvasFor(&diagCanvas.touch)
		cv.Reset(w, h)
		vw, vh := int(w)*2, int(h)*4
		cols, rows := len(t.Visited[0]), len(t.Visited)
		visited := cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
		for ry := 0; ry < rows; ry++ {
			for rx := 0; rx < cols; rx++ {
				if !t.Visited[ry][rx] {
					continue
				}
				for py := ry * vh / rows; py < (ry+1)*vh/rows; py += 2 {
					for px := rx * vw / cols; px < (rx+1)*vw/cols; px += 2 {
						cv.Set(px, py, visited)
					}
				}
			}
		}
		border := cell.Style{Fg: p.FgMuted, Bg: p.BgPanel}
		for px := 0; px < vw; px++ {
			cv.Set(px, 0, border)
			cv.Set(px, vh-1, border)
		}
		for py := 0; py < vh; py++ {
			cv.Set(0, py, border)
			cv.Set(vw-1, py, border)
		}
		cols2 := [2]cell.Color{p.AccentCyan, p.AccentPink}
		for i := 0; i < 2; i++ {
			for _, pt := range t.Trails[i] {
				cv.Set(int(pt[0]*float64(vw-1)), int(pt[1]*float64(vh-1)), cell.Style{Fg: cols2[i], Bg: p.BgPanel})
			}
		}
		c.f.RenderWidget(cv, cell.NewRect(c.r.X, top, w, h))
		ic := newCursor(c.f, cell.NewRect(c.r.X+w+2, top, c.r.Width-w-2, h), c.s)
		for i := 0; i < t.Layout.TouchpadContacts && i < 2; i++ {
			ct := t.Contacts[i]
			if ct.Active {
				txt := fmt.Sprintf("Finger %d: touching", i+1)
				if adv {
					txt = fmt.Sprintf("Finger %d: x=%4d y=%4d id=%d", i+1, ct.X, ct.Y, ct.ID)
				}
				ic.line(txt, cell.Style{Fg: cols2[i], Bg: p.BgPanel})
			} else {
				ic.line(fmt.Sprintf("Finger %d: —", i+1), p.MutedStyle())
			}
		}
		click := "released"
		if t.ClickHeld {
			click = "PRESSED"
		}
		ic.line("Click switch: "+click, p.LabelStyle())
		v, total := t.Coverage()
		ic.line(fmt.Sprintf("Covered %d/%d zones", v, total), p.MutedStyle())
		ic.line(fmt.Sprintf("Touches %d · clicks %d · max fingers %d", t.Touches, t.Clicks, t.MaxSimul), p.MutedStyle())
		c.y = top + h
	}
	if r, ok := d.Results[diag.CatTouchpad]; ok {
		renderResult(c, r, adv)
	}
}

// ---- Motion ----------------------------------------------------------------

func renderMotionDiag(c *cursor, d *diag.Session, now time.Time) {
	m := d.Motion
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	switch m.Phase {
	case diag.MotionStill:
		progressLine(c, "Put the controller down flat and don't touch it…", m.Progress(now), m.Cancel)
	case diag.MotionMove:
		progressLine(c, "Tilt forward/back, turn left/right, roll side to side…", m.Progress(now), m.Cancel)
	default:
		c.wrap("Checks sensor noise at rest and that every axis reacts to movement.", p.LabelStyle(), 2)
		c.pills("", 0, []choice{{ID: "motion_start", Label: "▶ Start motion test", OnSelect: func() { m.Start(time.Now()) }}})
	}
	names := [3]string{"Gyro X", "Gyro Y", "Gyro Z"}
	for i := 0; i < 3; i++ {
		val := ""
		if adv {
			val = fmt.Sprintf("%6.0f", m.Gyro[i])
		}
		c.gauge(fmt.Sprintf("motion_gyro_%d", i), names[i], 0.5+m.Gyro[i]/8000, val, p.AccentBlue)
	}
	if adv {
		c.line(fmt.Sprintf("Accel raw  X %6.0f  Y %6.0f  Z %6.0f   (uncalibrated counts)", m.Accel[0], m.Accel[1], m.Accel[2]), p.MutedStyle())
		c.sparkline("motion_hist", m.History[0].Values(), p.AccentBlue, 1)
	}
	if r, ok := d.Results[diag.CatMotion]; ok {
		renderResult(c, r, adv)
	}
}

// ---- Haptics ---------------------------------------------------------------

func renderHapticDiag(c *cursor, d *diag.Session, now time.Time) {
	h := d.Haptics
	p := c.p()
	c.wrap("Play a short pattern and tell TuiSense whether you felt it.", p.LabelStyle(), 2)
	var levels []choice
	for i, lv := range diag.HapticIntensities {
		i := i
		levels = append(levels, choice{ID: "haptic_level_" + lv.Name, Label: lv.Name, Selected: h.Intensity == i, OnSelect: func() { h.Intensity = i }})
	}
	c.pills("Strength", 10, levels)
	var pats []choice
	for _, pt := range h.Patterns {
		id := pt.ID
		pats = append(pats, choice{ID: "haptic_play_" + id, Label: "▶ " + pt.Label, Selected: h.Running != nil && h.Running.ID == id,
			OnSelect: func() { h.Play(id, time.Now()) }})
	}
	pats = append(pats, choice{ID: "haptic_stop", Label: "■ Stop", OnSelect: h.Stop})
	c.pills("Pattern", 10, pats)
	if h.LastPlayed != "" && h.Running == nil {
		label := h.LastPlayed
		for _, pt := range h.Patterns {
			if pt.ID == h.LastPlayed {
				label = pt.Label
			}
		}
		id := h.LastPlayed
		c.pills("Felt \""+label+"\"?", uint16(cell.StringWidth(label)+10), []choice{
			{ID: "haptic_felt_yes", Label: "Yes", Selected: feedbackIs(h.Feedback, id, true), OnSelect: func() { h.Feedback[id] = true }},
			{ID: "haptic_felt_no", Label: "No", Selected: feedbackIs(h.Feedback, id, false), OnSelect: func() { h.Feedback[id] = false }},
		})
	}
	if !h.Separate {
		c.line("This controller's motors cannot be driven separately.", p.MutedStyle())
	}
	if d.Ctx.Simulated {
		c.wrap("Simulated device: commands are recorded, nothing physically vibrates.", cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 2)
	}
	if r, ok := d.Results[diag.CatHaptics]; ok {
		renderResult(c, r, c.s.Mode == state.ModeAdvanced)
	}
}

func feedbackIs(m map[string]bool, id string, v bool) bool {
	f, ok := m[id]
	return ok && f == v
}

// ---- Audio -----------------------------------------------------------------

func renderMicDiag(c *cursor, d *diag.Session, now time.Time) {
	m := d.Mic
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	if m == nil {
		c.line("Microphone endpoint not available.", p.MutedStyle())
		return
	}
	c.kv("Testing", m.In.Label(), 10, p.ValueStyle())
	c.wrap("Audio is only captured while you run this test and is never saved.", p.MutedStyle(), 2)
	running, level, peak, chunks, err, rec, hist := m.Snapshot()
	if running {
		c.pills("", 0, []choice{{ID: "mic_stop", Label: "■ Stop listening", OnSelect: m.Stop}})
		c.gauge("mic_level", "Level", (level+60)/60, fmt.Sprintf("%5.0f dBFS", level), p.Success)
		if adv {
			c.line(fmt.Sprintf("Peak %.0f dBFS · %d chunks received", peak, chunks), p.MutedStyle())
			c.sparkline("mic_hist", shift(hist, 60), p.Success, 1)
		} else {
			c.wrap("Say something near the controller — the bar should move.", p.LabelStyle(), 2)
		}
		var recItems []choice
		switch rec {
		case diag.RecNone:
			recItems = append(recItems, choice{ID: "mic_record", Label: "● Record 3 s", OnSelect: func() { m.Record() }})
		case diag.RecRecording:
			recItems = append(recItems, choice{ID: "mic_recording", Label: "● Recording…", Selected: true})
		case diag.RecReady:
			if m.HasOut {
				recItems = append(recItems, choice{ID: "mic_playback", Label: "▶ Play back on controller", OnSelect: func() { _ = m.Playback() }})
			}
			recItems = append(recItems, choice{ID: "mic_discard", Label: "Discard", OnSelect: m.Discard})
		case diag.RecPlaying:
			recItems = append(recItems, choice{ID: "mic_playing", Label: "▶ Playing…", Selected: true})
		}
		c.pills("Optional", 10, recItems)
	} else {
		c.pills("", 0, []choice{{ID: "mic_start", Label: "▶ Start microphone test", OnSelect: func() { m.Start(time.Now()) }}})
	}
	if w := d.MicMuteWarning(); w != "" {
		c.wrap("! "+w, cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 2)
	}
	if err != nil {
		c.wrap("✕ "+err.Error(), cell.Style{Fg: p.Danger, Bg: p.BgPanel}, 3)
	}
	if r, ok := d.Results[diag.CatAudioIn]; ok {
		renderResult(c, r, adv)
	}
}

func shift(db []float64, by float64) []float64 {
	out := make([]float64, len(db))
	for i, v := range db {
		out[i] = math.Max(0, v+by)
	}
	return out
}

func renderSpeakerDiag(c *cursor, d *diag.Session) {
	sp := d.Speaker
	p := c.p()
	if sp == nil {
		c.line("Audio output endpoint not available.", p.MutedStyle())
		return
	}
	c.kv("Plays on", sp.Out.Label(), 10, p.ValueStyle())
	c.wrap("A short, moderate chime is sent only to this device — never to your system speakers.", p.MutedStyle(), 2)
	if sp.Playing() {
		c.pills("", 0, []choice{{ID: "speaker_stop", Label: "■ Stop", OnSelect: sp.Stop}})
	} else {
		c.pills("", 0, []choice{{ID: "speaker_play", Label: "▶ Play test sound", OnSelect: sp.Play}})
	}
	if err := sp.LastError(); err != nil {
		c.wrap("✕ "+err.Error(), cell.Style{Fg: p.Danger, Bg: p.BgPanel}, 3)
	}
	if sp.PlayCount() > 0 && !sp.Playing() {
		yes, no := true, false
		c.pills("Heard it?", 11, []choice{
			{ID: "speaker_heard_yes", Label: "Yes", Selected: sp.Heard != nil && *sp.Heard, OnSelect: func() { sp.Heard = &yes }},
			{ID: "speaker_heard_no", Label: "No sound", Selected: sp.Heard != nil && !*sp.Heard, OnSelect: func() { sp.Heard = &no }},
		})
	}
	if n := d.SpeakerVolumeNote(); n != "" {
		c.line("! "+n, cell.Style{Fg: p.Warning, Bg: p.BgPanel})
	}
	c.wrap("This is the controller's audio output — not its vibration motors.", p.MutedStyle(), 2)
	if r, ok := d.Results[diag.CatAudioOut]; ok {
		renderResult(c, r, c.s.Mode == state.ModeAdvanced)
	}
}

// ---- Battery & connection --------------------------------------------------

func renderBatteryDiag(c *cursor, d *diag.Session, now time.Time) {
	b := d.Battery
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	col := p.Success
	if b.Percent <= 20 {
		col = p.Danger
	}
	c.gauge("diag_battery", "Level", float64(b.Percent)/100, fmt.Sprintf("%3d%%", b.Percent), col)
	state := "discharging"
	switch {
	case b.Full:
		state = "full"
	case b.Charging:
		state = "charging"
	}
	c.kv("Status", state, 10, p.ValueStyle())
	c.kv("Link", string(d.Ctx.Info.Connection), 10, p.LabelStyle())
	if h, ok := b.Health(); ok {
		c.kv("Health", h, 10, p.ValueStyle())
	} else {
		c.line(diag.HealthUnavailable, cell.Style{Fg: p.FgSecondary, Bg: p.BgPanel, Modifier: cell.ModifierBold})
	}
	if adv {
		for _, a := range b.Details {
			c.kv(a.Name, a.Value+"  ("+a.Source+")", 16, p.LabelStyle())
		}
	}
	c.wrap("Observed: "+b.ObservationSummary(now), p.MutedStyle(), 2)
	if b.Step > 1 {
		c.line(fmt.Sprintf("The controller reports its level in %d%% steps.", b.Step), p.MutedStyle())
	}
	if adv {
		for i := len(b.Observations) - 1; i >= 0 && i >= len(b.Observations)-4 && c.ok(); i-- {
			o := b.Observations[i]
			c.line(fmt.Sprintf("  %s  %3d%%  charging=%v", o.At.Format("15:04:05"), o.Percent, o.Charging), p.MutedStyle())
		}
	}
}

func renderConnectionDiag(c *cursor, d *diag.Session, now time.Time) {
	cm := d.Conn
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	info := d.Ctx.Info
	status := "connected"
	if !info.Connected {
		status = "DISCONNECTED"
	}
	kw := uint16(18)
	c.kv("Transport", string(info.Connection), kw, p.ValueStyle())
	c.kv("Status", status, kw, p.ValueStyle())
	c.kv("Device", fmt.Sprintf("%s (%04X:%04X)", d.Ctx.Desc.DisplayName, info.VendorID, info.ProductID), kw, p.LabelStyle())
	c.kv("Firmware", nonEmptyStr(info.FirmwareVersion, "not reported"), kw, p.LabelStyle())
	c.kv("Input reports", fmt.Sprintf("%.0f Hz", cm.RateHz), kw, p.ValueStyle())
	if adv {
		c.kv("Missing reports", fmt.Sprintf("%d (sequence gaps)", cm.Dropped), kw, p.LabelStyle())
		c.kv("Read errors", fmt.Sprintf("%d", cm.ReadErrors), kw, p.LabelStyle())
		c.kv("Stalls > 0.5 s", fmt.Sprintf("%d", cm.Stalls), kw, p.LabelStyle())
	}
	c.kv("Interruptions", fmt.Sprintf("%d · reconnections %d", cm.Interruptions, cm.Reconnections), kw, p.LabelStyle())
	c.kv("Latency", "not measured (needs external timing hardware)", kw, p.MutedStyle())
	c.kv("Observed for", cm.Observed(now).Round(time.Second).String(), kw, p.MutedStyle())
	if r, ok := d.Results[diag.CatConnection]; ok {
		renderResult(c, r, adv)
	}
}

// DiagPrimaryAction runs the main action of the active diagnostics view
// (keyboard shortcut). It never starts audio capture implicitly — that
// still requires the explicit mic button/shortcut on the mic view.
func DiagPrimaryAction(d *diag.Session, now time.Time) {
	if d == nil || !d.Avail[d.Active].Available() {
		return
	}
	switch d.Active {
	case diag.CatSticks:
		if !d.Sticks.Running() {
			d.Sticks.StartStationary(now)
		}
	case diag.CatTriggers:
		if !d.Triggers.Running() {
			d.Triggers.Start(now)
		}
	case diag.CatMotion:
		if !d.Motion.Running() {
			d.Motion.Start(now)
		}
	case diag.CatButtons:
		d.Buttons.StartGuided()
	case diag.CatAudioIn:
		if d.Mic != nil {
			d.Mic.Start(now)
		}
	case diag.CatAudioOut:
		if d.Speaker != nil {
			d.Speaker.Play()
		}
	}
}
