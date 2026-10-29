package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"math"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// RenderTriggersPanel renders the L2 / R2 Adaptive Trigger configuration view,
// including mode selection, quick presets, parameter sliders, and a live Braille force-curve graph.
func RenderTriggersPanel(f *terminal.Frame, area cell.Rect, s *state.AppState, dispatch func(device.Command)) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, i18n.T("Adaptive Triggers"))
	y := inner.Y + 1 // the first button's frame stays clear of the title

	y = supportBanner(f, inner, y, s, device.CapAdaptiveTriggers)

	// Row 1: L2 vs R2 Selector + Link Triggers Checkbox
	linkX := pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{
		{Kind: kindOption, ID: "btn_trig_l2", Label: i18n.T("L2 Left Trigger"), Selected: !s.EditingRightTrigger, OnSelect: func() {
			s.EditingRightTrigger = false
			s.SelectedRegion = device.RegionLeftTrigger
		}},
		{Kind: kindOption, ID: "btn_trig_r2", Label: i18n.T("R2 Right Trigger"), Selected: s.EditingRightTrigger, OnSelect: func() {
			s.EditingRightTrigger = true
			s.SelectedRegion = device.RegionRightTrigger
		}},
	}) + 1
	if linkX+18 <= inner.X+inner.Width {
		cb := widgets.Checkbox{
			ID:           "cb_link_triggers",
			Checked:      &s.Config.LinkTriggers,
			Label:        i18n.T("Mirror L2=R2"),
			Style:        p.LabelStyle(),
			FocusedStyle: p.ButtonFocusedStyle(),
		}
		f.RenderWidget(cb, cell.NewRect(linkX, y, inner.X+inner.Width-linkX, 1))
	}
	y += pillRowH

	activeTrig := &s.Config.LeftTrigger
	startSlider := s.L2StartSlider
	endSlider := s.L2EndSlider
	strSlider := s.L2StrengthSlider
	freqSlider := s.L2FreqSlider
	prefix := "l2"
	livePull := s.Telemetry.L2Pressure
	if s.EditingRightTrigger {
		activeTrig = &s.Config.RightTrigger
		startSlider = s.R2StartSlider
		endSlider = s.R2EndSlider
		strSlider = s.R2StrengthSlider
		freqSlider = s.R2FreqSlider
		prefix = "r2"
		livePull = s.Telemetry.R2Pressure
	}

	syncAndDispatch := func() {
		if s.Config.LinkTriggers {
			if s.EditingRightTrigger {
				s.Config.LeftTrigger = s.Config.RightTrigger
			} else {
				s.Config.RightTrigger = s.Config.LeftTrigger
			}
			s.SyncWidgetsFromConfig()
		}
		if dispatch != nil {
			dispatch(device.Command{
				Type:   device.CmdSetTrigger,
				Config: s.Config,
			})
		}
	}

	// Row 2: mode, then curated presets, sharing one label column.
	lw := uint16(max(cell.StringWidth(i18n.T("Mode:")), cell.StringWidth(i18n.T("Presets:"))) + 1)
	var modes, presets []choice
	for _, mode := range device.AllTriggerModes {
		target := mode
		modes = append(modes, choice{Kind: kindOption, ID: "trigger_mode_" + string(mode), Label: shortTriggerModeName(mode), Selected: activeTrig.Mode == mode, OnSelect: func() {
			activeTrig.Mode = target
			syncAndDispatch()
		}})
	}
	for i, preset := range device.BuiltinTriggerPresets {
		idx := i
		presets = append(presets, choice{ID: fmt.Sprintf("trigger_preset_%d", i), Label: i18n.T(preset.Name), OnSelect: func() {
			*activeTrig = device.BuiltinTriggerPresets[idx].Config
			s.SyncWidgetsFromConfig()
			syncAndDispatch()
		}})
	}
	y = pillRows(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Mode:"), lw, modes)
	y = pillRows(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Presets:"), lw, presets)

	// Parameter Sliders
	sliderLabelW := sliderColumn(3, i18n.T("Start Zone:"), i18n.T("End / Snap:"), i18n.T("Force Strength:"), i18n.T("Pulse Freq Hz:"))
	sliderW := inner.Width - sliderLabelW - 2
	if sliderW < 8 {
		sliderW = 8
	}

	drawParamSlider := func(id, label string, val uint8, minV, maxV int, st *widgets.SliderState, onVal func(uint8)) {
		if y >= inner.Y+inner.Height {
			return
		}
		sliderLabel(f, cell.NewRect(inner.X, y, sliderLabelW, 1), s, label, fmt.Sprint(val))
		sl := widgets.Slider{
			ID:           id,
			State:        st,
			Min:          minV,
			Max:          maxV,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel},
			OnChange: func(v int) {
				onVal(uint8(v))
				syncAndDispatch()
			},
		}
		f.RenderWidget(sl, cell.NewRect(inner.X+sliderLabelW, y, sliderW, 1))
		y++
	}

	drawParamSlider("slider_"+prefix+"_start", i18n.T("Start Zone:"), activeTrig.StartPosition, 0, 255, startSlider, func(v uint8) {
		activeTrig.StartPosition = v
	})
	drawParamSlider("slider_"+prefix+"_end", i18n.T("End / Snap:"), activeTrig.EndPosition, 0, 255, endSlider, func(v uint8) {
		activeTrig.EndPosition = v
	})
	drawParamSlider("slider_"+prefix+"_strength", i18n.T("Force Strength:"), activeTrig.Strength, 0, 255, strSlider, func(v uint8) {
		activeTrig.Strength = v
	})
	drawParamSlider("slider_"+prefix+"_freq", i18n.T("Pulse Freq Hz:"), activeTrig.Frequency, 0, 100, freqSlider, func(v uint8) {
		activeTrig.Frequency = v
	})
	y++

	// Live force curve: an area chart of resistance across the trigger's travel.
	remainH := int(inner.Y+inner.Height) - int(y)
	if remainH >= 6 && inner.Width >= 24 {
		title := i18n.Tf("Live force curve · %s", i18n.T(string(activeTrig.Mode)))
		stats := i18n.Tf("Pull %.0f%% · Resistance %.0f%%", livePull*100, activeTrig.ForceAt(livePull)*100)
		f.Buffer.SetStringWithin(inner.X, y, title, p.SectionHeaderStyle(), inner.Width)
		tw, sw := uint16(cell.StringWidth(title)), uint16(cell.StringWidth(stats))
		if tw+2+sw > inner.Width { // no room beside the title: its own line
			y++
			remainH--
			tw = 0
		}
		copyValue(f, cell.NewRect(inner.X+inner.Width-min(sw, inner.Width-tw), y, min(sw, inner.Width-tw), 1), s, stats, stats, p.ValueStyle())
		y += 2
		remainH -= 2
		renderForceCurve(f, cell.NewRect(inner.X, y, inner.Width, uint16(min(remainH, 10))), s, *activeTrig, livePull)
	}
}

// renderForceCurve draws resistance (0..100%, up) against pull (0..100%,
// right) as solid eighth-block columns: the travel already pulled in the
// accent, the rest dimmed, with a percent axis on the left and the pull
// axis underneath.
func renderForceCurve(f *terminal.Frame, area cell.Rect, s *state.AppState, trig device.TriggerConfig, pull float64) {
	p := s.Theme
	const axisW = 5 // "100% "
	if area.Height < 3 || area.Width <= axisW+8 {
		return
	}
	graphH := area.Height - 1 // the bottom row holds the pull axis
	graphW := area.Width - axisW
	gx := area.X + axisW
	blocks := []rune(" ▁▂▃▄▅▆▇█")
	axis := p.MutedStyle()
	f.Buffer.SetStringWithin(area.X, area.Y, "100%", axis, axisW)
	f.Buffer.SetStringWithin(area.X+2, area.Y+graphH-1, "0%", axis, axisW)
	for col := uint16(0); col < graphW; col++ {
		t := float64(col) / float64(max(1, graphW-1))
		eighths := int(math.Round(math.Min(1, math.Max(0, trig.ForceAt(t))) * float64(graphH) * 8))
		colour := p.BgSubtle
		if t <= pull {
			colour = p.AccentCyan
		}
		for row := uint16(0); row < graphH; row++ {
			level := min(8, max(0, eighths-int(row)*8))
			ch := blocks[level]
			st := cell.Style{Fg: colour, Bg: p.BgPanel}
			if level == 0 && row == 0 {
				ch, st = '▁', cell.Style{Fg: p.BgElevated, Bg: p.BgPanel} // a faint floor where there is no force
			}
			f.Buffer.SetCell(gx+col, area.Y+graphH-1-row, cell.Cell{Content: ch, Style: st})
		}
	}
	// Pull axis: 0% · a marker under the live pull · 100%.
	ay := area.Y + graphH
	f.Buffer.SetStringWithin(gx, ay, "0%", axis, graphW)
	f.Buffer.SetStringWithin(gx+graphW-4, ay, "100%", axis, 4)
	mark := gx + uint16(math.Round(math.Min(1, math.Max(0, pull))*float64(graphW-1)))
	f.Buffer.SetStringWithin(mark, ay, "▲", cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}, 1)
}

func shortTriggerModeName(m device.TriggerMode) string {
	switch m {
	case device.TriggerModeOff:
		return i18n.T("Off")
	case device.TriggerModeResistance:
		return i18n.T("Resistance")
	case device.TriggerModeWeapon:
		return i18n.T("Weapon")
	case device.TriggerModeFeedback:
		return i18n.T("Feedback")
	case device.TriggerModeVibration:
		return i18n.T("Pulse")
	case device.TriggerModeCustom:
		return i18n.T("Custom")
	default:
		return string(m)
	}
}
