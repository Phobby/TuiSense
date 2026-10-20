package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"math"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/state"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/widgets"
)

// Reusable canvases for the diagnostics graphics (single UI instance).
var diagCanvas = struct {
	touch *widgets.Canvas
	named map[string]*widgets.Canvas
}{named: map[string]*widgets.Canvas{}}

func canvasFor(slot **widgets.Canvas) *widgets.Canvas {
	if *slot == nil {
		*slot = widgets.NewCanvas(20, 6)
	}
	return *slot
}

// namedCanvas returns a reusable canvas for id.
func namedCanvas(id string) *widgets.Canvas {
	cv, ok := diagCanvas.named[id]
	if !ok {
		cv = widgets.NewCanvas(20, 6)
		diagCanvas.named[id] = cv
	}
	return cv
}

func progressLine(c *cursor, label string, frac float64, onCancel func()) {
	if !c.ok() {
		return
	}
	p := c.p()
	lw := uint16(cell.StringWidth(label) + 1)
	cancelW := uint16(11)
	if c.r.Width < lw+cancelW+12 {
		// Narrow panel: label on its own line, the bar below it.
		c.line(label, cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold})
		if !c.ok() {
			return
		}
		lw = 0
	}
	c.f.Buffer.SetStringWithin(c.r.X, c.y, label, cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}, lw)
	if c.r.Width > lw+cancelW+6 {
		c.f.RenderWidget(widgets.LineGauge{ID: "diag_progress", Ratio: clamp01(frac), HideLabel: true,
			FilledStyle: cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel}, UnfilledStyle: cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}},
			cell.NewRect(c.r.X+lw, c.y, c.r.Width-lw-cancelW-1, 1))
		pillRow(c.f, c.r.X+c.r.Width-cancelW, c.y, c.limit(), c.s, "", 0, []choice{{ID: "diag_cancel", Label: i18n.T("✕ Cancel"), OnSelect: onCancel}})
	}
	c.y++
}

// ---- Buttons ---------------------------------------------------------------

func renderButtonDiag(c *cursor, d *diag.Session) {
	bt := d.Buttons
	p := c.p()
	accent := uiwidgets.AccentColor(d.Ctx.Desc)
	if target, ok := bt.Prompt(); ok {
		c.line(i18n.Tf("Press:  %s   (%d of %d)", i18n.T(target.Label), bt.GuidedIdx+1, len(bt.Layout.Buttons)), cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold})
		c.pills("", 0, []choice{{ID: "buttons_skip", Label: i18n.T("Skip (not working)"), OnSelect: bt.SkipCurrent}})
	} else {
		c.wrap(i18n.T("Press any button — it lights up here and on the controller."), p.LabelStyle(), 2)
		c.pills("", 0, []choice{
			{ID: "buttons_guided", Label: i18n.T("▶ Guided: press each button"), OnSelect: bt.StartGuided},
			{ID: "buttons_reset", Label: i18n.T("↻ Reset counts"), OnSelect: bt.Reset},
		})
	}
	x := c.r.X
	for _, b := range bt.Layout.Buttons {
		if !c.ok() {
			break
		}
		label := " " + i18n.T(b.Label)
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
			Label: i18n.T(b.Label), Value: i18n.Tf("count=%d", bt.Counts[b.ID]), State: nodeState, Bounds: rect})
		x += w + 1
	}
	c.y++
	if len(bt.Stuck) > 0 {
		c.line(i18n.T("! Some buttons were held for a long time — stuck, or simply held?"), cell.Style{Fg: p.Warning, Bg: p.BgPanel})
	}
	if r, ok := d.Results[diag.CatButtons]; ok {
		renderResult(c, r, c.s.Mode == state.ModeAdvanced)
	}
}

// ---- Touchpad --------------------------------------------------------------

func renderTouchDiag(c *cursor, d *diag.Session) {
	t := d.Touch
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	c.wrap(i18n.T("Slide over the whole pad, try two fingers, and click it."), p.LabelStyle(), 2)
	c.pills("", 0, []choice{{ID: "touch_reset", Label: i18n.T("↻ Reset"), OnSelect: t.Reset}})
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
				txt := i18n.Tf("Finger %d: touching", i+1)
				if adv {
					txt = i18n.Tf("Finger %d: x=%4d y=%4d id=%d", i+1, ct.X, ct.Y, ct.ID)
				}
				ic.line(txt, cell.Style{Fg: cols2[i], Bg: p.BgPanel})
			} else {
				ic.line(i18n.Tf("Finger %d: —", i+1), p.MutedStyle())
			}
		}
		click := "released"
		if t.ClickHeld {
			click = i18n.T("PRESSED")
		}
		ic.line(i18n.T("Click switch: ")+click, p.LabelStyle())
		v, total := t.Coverage()
		ic.line(i18n.Tf("Covered %d/%d zones", v, total), p.MutedStyle())
		ic.line(i18n.Tf("Touches %d · clicks %d · max fingers %d", t.Touches, t.Clicks, t.MaxSimul), p.MutedStyle())
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
		progressLine(c, i18n.T("Put the controller down flat and don't touch it…"), m.Progress(now), m.Cancel)
	case diag.MotionMove:
		progressLine(c, i18n.T("Tilt forward/back, turn left/right, roll side to side…"), m.Progress(now), m.Cancel)
	default:
		c.wrap(i18n.T("Checks sensor noise at rest and that every axis reacts to movement."), p.LabelStyle(), 2)
		c.pills("", 0, []choice{{ID: "motion_start", Label: i18n.T("▶ Start motion test"), OnSelect: func() { m.Start(time.Now()) }}})
	}
	names := [3]string{i18n.T("Gyro X"), i18n.T("Gyro Y"), i18n.T("Gyro Z")}
	for i := 0; i < 3; i++ {
		val := ""
		if adv {
			val = fmt.Sprintf("%6.0f", m.Gyro[i])
		}
		c.gauge(fmt.Sprintf("motion_gyro_%d", i), names[i], 0.5+m.Gyro[i]/8000, val, p.AccentBlue)
	}
	if adv {
		c.line(i18n.Tf("Accel raw  X %6.0f  Y %6.0f  Z %6.0f   (uncalibrated counts)", m.Accel[0], m.Accel[1], m.Accel[2]), p.MutedStyle())
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
	c.wrap(i18n.T("Play a short pattern and tell TuiSense whether you felt it."), p.LabelStyle(), 2)
	var levels []choice
	for i, lv := range diag.HapticIntensities {
		i := i
		levels = append(levels, choice{ID: "haptic_level_" + lv.Name, Label: i18n.T(lv.Name), Selected: h.Intensity == i, OnSelect: func() { h.Intensity = i }})
	}
	c.pills(i18n.T("Strength"), 10, levels)
	var pats []choice
	for _, pt := range h.Patterns {
		id := pt.ID
		pats = append(pats, choice{ID: "haptic_play_" + id, Label: "▶ " + i18n.T(pt.Label), Selected: h.Running != nil && h.Running.ID == id,
			OnSelect: func() { h.Play(id, time.Now()) }})
	}
	pats = append(pats, choice{ID: "haptic_stop", Label: i18n.T("■ Stop"), OnSelect: h.Stop})
	c.pills(i18n.T("Pattern"), 10, pats)
	if h.LastPlayed != "" && h.Running == nil {
		label := h.LastPlayed
		for _, pt := range h.Patterns {
			if pt.ID == h.LastPlayed {
				label = pt.Label
			}
		}
		id := h.LastPlayed
		c.pills(i18n.Tf("Felt \"%s\"?", i18n.T(label)), uint16(cell.StringWidth(i18n.T(label))+12), []choice{
			{ID: "haptic_felt_yes", Label: i18n.T("Yes"), Selected: feedbackIs(h.Feedback, id, true), OnSelect: func() { h.Feedback[id] = true }},
			{ID: "haptic_felt_no", Label: i18n.T("No"), Selected: feedbackIs(h.Feedback, id, false), OnSelect: func() { h.Feedback[id] = false }},
		})
	}
	if !h.Separate {
		c.line(i18n.T("This controller's motors cannot be driven separately."), p.MutedStyle())
	}
	if d.Ctx.Simulated {
		c.wrap(i18n.T("Simulated device: commands are recorded, nothing physically vibrates."), cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 2)
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
		c.line(i18n.T("Microphone endpoint not available."), p.MutedStyle())
		return
	}
	c.kv(i18n.T("Testing"), m.In.Label(), 10, p.ValueStyle())
	c.wrap(i18n.T("Audio is only captured while you run this test and is never saved."), p.MutedStyle(), 2)
	running, level, peak, chunks, err, rec, hist := m.Snapshot()
	if running {
		c.pills("", 0, []choice{{ID: "mic_stop", Label: i18n.T("■ Stop listening"), OnSelect: m.Stop}})
		c.gauge("mic_level", i18n.T("Level"), (level+60)/60, i18n.Tf("%5.0f dBFS", level), p.Success)
		if adv {
			c.line(i18n.Tf("Peak %.0f dBFS · %d chunks received", peak, chunks), p.MutedStyle())
			c.sparkline("mic_hist", shift(hist, 60), p.Success, 1)
		} else {
			c.wrap(i18n.T("Say something near the controller — the bar should move."), p.LabelStyle(), 2)
		}
		var recItems []choice
		switch rec {
		case diag.RecNone:
			recItems = append(recItems, choice{ID: "mic_record", Label: i18n.T("● Record 3 s"), OnSelect: func() { m.Record() }})
		case diag.RecRecording:
			recItems = append(recItems, choice{ID: "mic_recording", Label: i18n.T("● Recording…"), Selected: true})
		case diag.RecReady:
			if m.HasOut {
				recItems = append(recItems, choice{ID: "mic_playback", Label: i18n.T("▶ Play back on controller"), OnSelect: func() { _ = m.Playback() }})
			}
			recItems = append(recItems, choice{ID: "mic_discard", Label: i18n.T("Discard"), OnSelect: m.Discard})
		case diag.RecPlaying:
			recItems = append(recItems, choice{ID: "mic_playing", Label: i18n.T("▶ Playing…"), Selected: true})
		}
		c.pills(i18n.T("Optional"), 10, recItems)
	} else {
		c.pills("", 0, []choice{{ID: "mic_start", Label: i18n.T("▶ Start microphone test"), OnSelect: func() { m.Start(time.Now()) }}})
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
		c.line(i18n.T("Audio output endpoint not available."), p.MutedStyle())
		return
	}
	c.kv(i18n.T("Plays on"), sp.Out.Label(), 10, p.ValueStyle())
	c.wrap(i18n.T("A short, moderate chime is sent only to this device — never to your system speakers."), p.MutedStyle(), 2)
	if sp.Playing() {
		c.pills("", 0, []choice{{ID: "speaker_stop", Label: i18n.T("■ Stop"), OnSelect: sp.Stop}})
	} else {
		c.pills("", 0, []choice{{ID: "speaker_play", Label: i18n.T("▶ Play test sound"), OnSelect: sp.Play}})
	}
	if err := sp.LastError(); err != nil {
		c.wrap("✕ "+err.Error(), cell.Style{Fg: p.Danger, Bg: p.BgPanel}, 3)
	}
	if sp.PlayCount() > 0 && !sp.Playing() {
		yes, no := true, false
		c.pills(i18n.T("Heard it?"), 11, []choice{
			{ID: "speaker_heard_yes", Label: i18n.T("Yes"), Selected: sp.Heard != nil && *sp.Heard, OnSelect: func() { sp.Heard = &yes }},
			{ID: "speaker_heard_no", Label: i18n.T("No sound"), Selected: sp.Heard != nil && !*sp.Heard, OnSelect: func() { sp.Heard = &no }},
		})
	}
	if n := d.SpeakerVolumeNote(); n != "" {
		c.line("! "+n, cell.Style{Fg: p.Warning, Bg: p.BgPanel})
	}
	c.wrap(i18n.T("This is the controller's audio output — not its vibration motors."), p.MutedStyle(), 2)
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
	c.gauge("diag_battery", i18n.T("Level"), float64(b.Percent)/100, fmt.Sprintf("%3d%%", b.Percent), col)
	state := i18n.T("discharging")
	switch {
	case b.Full:
		state = i18n.T("full")
	case b.Charging:
		state = i18n.T("charging")
	}
	c.kv(i18n.T("Status"), state, 10, p.ValueStyle())
	c.kv(i18n.T("Link"), i18n.T(string(d.Ctx.Info.Connection)), 10, p.LabelStyle())
	if h, ok := b.Health(); ok {
		c.kv(i18n.T("Health"), h, 10, p.ValueStyle())
	} else {
		c.line(diag.HealthUnavailableText(), cell.Style{Fg: p.FgSecondary, Bg: p.BgPanel, Modifier: cell.ModifierBold})
	}
	if adv {
		for _, a := range b.Details {
			c.kv(a.Name, a.Value+"  ("+i18n.T(a.Source)+")", 16, p.LabelStyle())
		}
	}
	c.wrap(i18n.T("Observed: ")+b.ObservationSummary(now), p.MutedStyle(), 2)
	if b.Step > 1 {
		c.line(i18n.Tf("The controller reports its level in %d%% steps.", b.Step), p.MutedStyle())
	}
	if adv {
		for i := len(b.Observations) - 1; i >= 0 && i >= len(b.Observations)-4 && c.ok(); i-- {
			o := b.Observations[i]
			c.line(i18n.Tf("  %s  %3d%%  charging=%v", o.At.Format("15:04:05"), o.Percent, o.Charging), p.MutedStyle())
		}
	}
}

func renderConnectionDiag(c *cursor, d *diag.Session, pc PanelContext, now time.Time) {
	cm := d.Conn
	p := c.p()
	adv := c.s.Mode == state.ModeAdvanced
	info := d.Ctx.Info
	status := i18n.T("connected")
	if !info.Connected {
		status = i18n.T("DISCONNECTED")
	}
	kw := uint16(18)
	c.kv(i18n.T("Transport"), i18n.T(string(info.Connection)), kw, p.ValueStyle())
	c.kv(i18n.T("Status"), status, kw, p.ValueStyle())
	c.kv(i18n.T("Device"), fmt.Sprintf("%s (%04X:%04X)", i18n.T(d.Ctx.Desc.DisplayName), info.VendorID, info.ProductID), kw, p.LabelStyle())
	if d.Ctx.Caps.Supports(device.CapFirmwareInfo) {
		c.kv(i18n.T("Firmware"), nonEmptyStr(info.FirmwareVersion, i18n.T("not reported")), kw, p.LabelStyle())
	}
	if info.InputMode != "" {
		c.kv(i18n.T("Input mode"), i18n.T(info.InputMode), kw, p.LabelStyle())
	}
	if info.EventDriven {
		c.kv(i18n.T("Input events"), i18n.Tf("%.0f /s (only sent on change)", cm.RateHz), kw, p.ValueStyle())
	} else {
		c.kv(i18n.T("Input reports"), i18n.Tf("%.0f Hz", cm.RateHz), kw, p.ValueStyle())
	}
	if adv {
		c.kv(i18n.T("Missing reports"), i18n.Tf("%d (sequence gaps)", cm.Dropped), kw, p.LabelStyle())
		c.kv(i18n.T("Read errors"), fmt.Sprintf("%d", cm.ReadErrors), kw, p.LabelStyle())
		if !info.EventDriven {
			c.kv(i18n.T("Stalls > 0.5 s"), fmt.Sprintf("%d", cm.Stalls), kw, p.LabelStyle())
		}
		for _, f := range info.Details {
			c.kv(i18n.T(f.Label), i18n.T(f.Value), kw, p.LabelStyle())
		}
	} else if len(info.Details) > 0 {
		for _, f := range info.Details {
			if f.Label == "USB ID" || f.Label == "Kernel driver" {
				c.kv(i18n.T(f.Label), f.Value, kw, p.LabelStyle())
			}
		}
	}
	c.kv(i18n.T("Interruptions"), i18n.Tf("%d · reconnections %d", cm.Interruptions, cm.Reconnections), kw, p.LabelStyle())
	c.kv(i18n.T("Latency"), i18n.T("not measured (needs external timing hardware)"), kw, p.MutedStyle())
	c.kv(i18n.T("Observed for"), cm.Observed(now).Round(time.Second).String(), kw, p.MutedStyle())
	if r, ok := d.Results[diag.CatConnection]; ok {
		renderResult(c, r, adv)
	}
	renderIdentity(c, c.s, pc.CB)
	renderModeGuide(c, d.Ctx.Desc)
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
		st := d.Sticks
		switch {
		case st.Step == diag.StickExplain:
			st.Continue(now)
		case st.Moving():
			st.SkipStep(now)
		case !st.Running():
			st.Start(now)
		}
	case diag.CatTriggers:
		tt := d.Triggers
		switch {
		case tt.Target() >= 0:
			tt.SkipStep(now)
		case !tt.Running():
			tt.Start(now)
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
