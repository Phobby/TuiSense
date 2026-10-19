package views

import (
	"fmt"

	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/state"
	uiwidgets "github.com/ygzztrk/tuisense/internal/ui/widgets"
)

// RenderTriggersPanel renders the L2 / R2 Adaptive Trigger configuration view,
// including mode selection, quick presets, parameter sliders, and a live Braille force-curve graph.
func RenderTriggersPanel(f *terminal.Frame, area cell.Rect, s *state.AppState, dispatch func(device.Command)) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, "Adaptive Triggers")
	y := inner.Y

	y = supportBanner(f, inner, y, s, device.CapAdaptiveTriggers)

	// Row 1: L2 vs R2 Selector + Link Triggers Checkbox
	l2Btn := widgets.Button{
		ID:           "btn_trig_l2",
		Label:        "L2 Left Trigger",
		Style:        p.ButtonStyle(!s.EditingRightTrigger),
		FocusedStyle: p.ButtonFocusedStyle(),
		OnPress: func() {
			s.EditingRightTrigger = false
			s.SelectedRegion = device.RegionLeftTrigger
		},
	}
	r2Btn := widgets.Button{
		ID:           "btn_trig_r2",
		Label:        "R2 Right Trigger",
		Style:        p.ButtonStyle(s.EditingRightTrigger),
		FocusedStyle: p.ButtonFocusedStyle(),
		OnPress: func() {
			s.EditingRightTrigger = true
			s.SelectedRegion = device.RegionRightTrigger
		},
	}
	wL2, _ := l2Btn.SizeHint(inner)
	wR2, _ := r2Btn.SizeHint(inner)
	uiwidgets.RenderButton(f, cell.NewRect(inner.X, y, wL2, 1), l2Btn)
	uiwidgets.RenderButton(f, cell.NewRect(inner.X+wL2+1, y, wR2, 1), r2Btn)

	linkX := inner.X + wL2 + wR2 + 3
	if linkX+18 <= inner.X+inner.Width {
		cb := widgets.Checkbox{
			ID:           "cb_link_triggers",
			Checked:      &s.Config.LinkTriggers,
			Label:        "Mirror L2=R2",
			Style:        p.LabelStyle(),
			FocusedStyle: p.ButtonFocusedStyle(),
		}
		f.RenderWidget(cb, cell.NewRect(linkX, y, inner.X+inner.Width-linkX, 1))
	}
	y += 2

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

	// Row 2: Mode Selector Pills
	f.Buffer.SetStringWithin(inner.X, y, "Mode:", p.MutedStyle(), 6)
	mx := inner.X + 6
	for _, mode := range device.AllTriggerModes {
		short := shortTriggerModeName(mode)
		lbl := " " + short + " "
		mw := uint16(cell.StringWidth(lbl))
		if mx+mw > inner.X+inner.Width {
			break
		}
		mRect := cell.NewRect(mx, y, mw, 1)
		st := p.ButtonStyle(activeTrig.Mode == mode)
		f.Buffer.SetStringWithin(mx, y, lbl, st, mw)
		targetMode := mode
		f.RegisterClickHandler(mRect, func(_ driver.MouseEvent) {
			activeTrig.Mode = targetMode
			syncAndDispatch()
		})
		mx += mw + 1
	}
	y++

	// Row 3: Curated Presets Row
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, "Presets:", p.MutedStyle(), 9)
		px := inner.X + 9
		for i, preset := range device.BuiltinTriggerPresets {
			tag := fmt.Sprintf(" [%s] ", preset.Name)
			tw := uint16(cell.StringWidth(tag))
			if px+tw > inner.X+inner.Width {
				break
			}
			pRect := cell.NewRect(px, y, tw, 1)
			f.Buffer.SetStringWithin(px, y, tag, cell.Style{Fg: p.AccentCyan, Bg: p.BgElevated}, tw)
			idx := i
			f.RegisterClickHandler(pRect, func(_ driver.MouseEvent) {
				*activeTrig = device.BuiltinTriggerPresets[idx].Config
				s.SyncWidgetsFromConfig()
				syncAndDispatch()
			})
			px += tw + 1
		}
		y += 2
	}

	// Parameter Sliders
	sliderLabelW := uint16(18)
	sliderW := inner.Width - sliderLabelW - 2
	if sliderW < 8 {
		sliderW = 8
	}

	drawParamSlider := func(id, label string, val uint8, minV, maxV int, st *widgets.SliderState, onVal func(uint8)) {
		if y >= inner.Y+inner.Height {
			return
		}
		txt := fmt.Sprintf("%-13s %3d", label, val)
		f.Buffer.SetStringWithin(inner.X, y, txt, p.LabelStyle(), sliderLabelW)
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

	drawParamSlider("slider_"+prefix+"_start", "Start Zone:", activeTrig.StartPosition, 0, 255, startSlider, func(v uint8) {
		activeTrig.StartPosition = v
	})
	drawParamSlider("slider_"+prefix+"_end", "End / Snap:", activeTrig.EndPosition, 0, 255, endSlider, func(v uint8) {
		activeTrig.EndPosition = v
	})
	drawParamSlider("slider_"+prefix+"_strength", "Force Strength:", activeTrig.Strength, 0, 255, strSlider, func(v uint8) {
		activeTrig.Strength = v
	})
	drawParamSlider("slider_"+prefix+"_freq", "Pulse Freq Hz:", activeTrig.Frequency, 0, 100, freqSlider, func(v uint8) {
		activeTrig.Frequency = v
	})
	y++

	// Live Force-Curve Braille Graph
	remainH := int(inner.Y+inner.Height) - int(y)
	if remainH >= 4 && s.CurveCanvas != nil {
		f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("Live Force Curve (%s) │ Pull: %3.0f%% │ Resistance: %3.0f%%",
			activeTrig.Mode, livePull*100, activeTrig.ForceAt(livePull)*100), p.SectionHeaderStyle(), inner.Width)
		y++
		remainH--

		graphW := inner.Width
		graphH := uint16(min(remainH, 6))
		s.CurveCanvas.Reset(graphW, graphH)
		virtW := int(graphW) * 2
		virtH := int(graphH) * 4

		// Baseline grid
		gridStyle := cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
		for gx := 0; gx < virtW; gx += 4 {
			s.CurveCanvas.Set(gx, virtH-1, gridStyle)
		}

		// Force curve line & fill
		curveStyle := cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel}
		fillStyle := cell.Style{Fg: p.AccentBlue, Bg: p.BgPanel}
		for px := 0; px < virtW; px++ {
			t := float64(px) / float64(max(1, virtW-1))
			force := activeTrig.ForceAt(t)
			py := virtH - 1 - int(force*float64(virtH-2))
			s.CurveCanvas.Set(px, py, curveStyle)
			for fy := py + 2; fy < virtH; fy += 2 {
				s.CurveCanvas.Set(px, fy, fillStyle)
			}
		}

		// Live trigger pull marker vertical cursor
		pullX := int(livePull * float64(virtW-1))
		markerStyle := cell.Style{Fg: p.AccentPink, Bg: p.BgPanel, Modifier: cell.ModifierBold}
		for py := 0; py < virtH; py++ {
			s.CurveCanvas.Set(pullX, py, markerStyle)
		}

		f.RenderWidget(s.CurveCanvas, cell.NewRect(inner.X, y, graphW, graphH))
	}
}

func shortTriggerModeName(m device.TriggerMode) string {
	switch m {
	case device.TriggerModeOff:
		return "Off"
	case device.TriggerModeResistance:
		return "Resistance"
	case device.TriggerModeWeapon:
		return "Weapon"
	case device.TriggerModeFeedback:
		return "Feedback"
	case device.TriggerModeVibration:
		return "Pulse"
	case device.TriggerModeCustom:
		return "Custom"
	default:
		return string(m)
	}
}
