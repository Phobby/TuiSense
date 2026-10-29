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

// RenderSticksPanel renders the Left/Right Analog Stick deadzones, sensitivity,
// response curves, and a live 2D Braille XY drift & deadzone visualizer.
func RenderSticksPanel(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, i18n.T("Sticks & Drift"))
	// TuiSense has no input-processing backend: these values are stored in
	// the profile and visualized, but games never receive them; the panel's
	// title already says "preview only".
	y := inner.Y + 1 // the first button's frame stays clear of the title

	// Row 0: Left vs Right Stick buttons + Link Sticks checkbox
	linkX := pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{
		{Kind: kindOption, ID: "btn_stick_left", Label: i18n.T("Left Stick (L3)"), Selected: !s.EditingRightStick, OnSelect: func() {
			s.EditingRightStick = false
			s.SelectedRegion = device.RegionLeftStick
		}},
		{Kind: kindOption, ID: "btn_stick_right", Label: i18n.T("Right Stick (R3)"), Selected: s.EditingRightStick, OnSelect: func() {
			s.EditingRightStick = true
			s.SelectedRegion = device.RegionRightStick
		}},
	}) + 1
	if linkX+16 <= inner.X+inner.Width {
		cb := widgets.Checkbox{
			ID:           "cb_link_sticks",
			Checked:      &s.Config.Sticks.LinkSticks,
			Label:        i18n.T("Link L3=R3"),
			Style:        p.LabelStyle(),
			FocusedStyle: p.ButtonFocusedStyle(),
		}
		f.RenderWidget(cb, cell.NewRect(linkX, y, inner.X+inner.Width-linkX, 1))
	}
	y += pillRowH

	activeStick := &s.Config.Sticks.Left
	inSlider := s.LInnerDZSlider
	outSlider := s.LOuterDZSlider
	sensSlider := s.LSensSlider
	prefix := "lstick"
	rawX, rawY := s.Telemetry.LeftStickX, s.Telemetry.LeftStickY
	if s.EditingRightStick {
		activeStick = &s.Config.Sticks.Right
		inSlider = s.RInnerDZSlider
		outSlider = s.ROuterDZSlider
		sensSlider = s.RSensSlider
		prefix = "rstick"
		rawX, rawY = s.Telemetry.RightStickX, s.Telemetry.RightStickY
	}

	syncSticks := func() {
		if s.Config.Sticks.LinkSticks {
			if s.EditingRightStick {
				s.Config.Sticks.Left = s.Config.Sticks.Right
			} else {
				s.Config.Sticks.Right = s.Config.Sticks.Left
			}
			s.SyncWidgetsFromConfig()
		}
	}

	// Response Curve Selector
	var curves []choice
	for _, curve := range device.AllStickCurves {
		target := curve
		curves = append(curves, choice{Kind: kindOption, ID: "curve_" + string(curve), Label: i18n.T(string(curve)), Selected: activeStick.Curve == curve, OnSelect: func() {
			activeStick.Curve = target
			syncSticks()
		}})
	}
	y = pillRows(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Curve:"), 7, curves)

	labelW := sliderColumn(4, i18n.T("Inner Deadzone:"), i18n.T("Outer Max Zone:"), i18n.T("Sensitivity:"))
	sliderW := inner.Width - labelW - 2
	if sliderW < 8 {
		sliderW = 8
	}

	// Inner Deadzone (0..40%)
	if y < inner.Y+inner.Height {
		sliderLabel(f, cell.NewRect(inner.X, y, labelW, 1), s, i18n.T("Inner Deadzone:"), fmt.Sprintf("%d%%", activeStick.InnerDeadzone))
		f.RenderWidget(widgets.Slider{
			ID:           "slider_" + prefix + "_inner_dz",
			State:        inSlider,
			Min:          0,
			Max:          40,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: p.Warning, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			OnChange: func(v int) {
				activeStick.InnerDeadzone = v
				syncSticks()
			},
		}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y++
	}

	// Outer Threshold (70..100%)
	if y < inner.Y+inner.Height {
		sliderLabel(f, cell.NewRect(inner.X, y, labelW, 1), s, i18n.T("Outer Max Zone:"), fmt.Sprintf("%d%%", activeStick.OuterDeadzone))
		f.RenderWidget(widgets.Slider{
			ID:           "slider_" + prefix + "_outer_dz",
			State:        outSlider,
			Min:          70,
			Max:          100,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: p.AccentBlue, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			OnChange: func(v int) {
				activeStick.OuterDeadzone = v
				syncSticks()
			},
		}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y++
	}

	// Sensitivity (50..150%)
	if y < inner.Y+inner.Height {
		sliderLabel(f, cell.NewRect(inner.X, y, labelW, 1), s, i18n.T("Sensitivity:"), fmt.Sprintf("%d%%", activeStick.Sensitivity))
		f.RenderWidget(widgets.Slider{
			ID:           "slider_" + prefix + "_sens",
			State:        sensSlider,
			Min:          50,
			Max:          150,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel},
			OnChange: func(v int) {
				activeStick.Sensitivity = v
				syncSticks()
			},
		}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y += 2
	}

	// Live 2D stick scope: the full travel, the max-zone and deadzone rings,
	// the raw position and what a game would receive, with the values beside it.
	remainH := int(inner.Y+inner.Height) - int(y)
	if remainH >= 6 && s.ScopeCanvas != nil {
		f.Buffer.SetStringWithin(inner.X, y, i18n.T("2D stick scope"), p.SectionHeaderStyle(), inner.Width)
		y += 2
		remainH -= 2
		// Braille dots are square: two per column and four per row.
		scopeH := uint16(min(remainH, 9))
		scopeW := min(scopeH*2, inner.Width)
		s.ScopeCanvas.Reset(scopeW, scopeH)
		midX, midY := int(scopeW), int(scopeH)*2
		rad := float64(min(midX, midY) - 1)
		ring := func(r float64, st cell.Style) {
			if r < 1 {
				return
			}
			for a := 0.0; a < 2*math.Pi; a += 0.7 / r {
				s.ScopeCanvas.Set(midX+int(math.Round(math.Cos(a)*r)), midY+int(math.Round(math.Sin(a)*r)), st)
			}
		}
		grid := cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
		for d := -int(rad); d <= int(rad); d += 2 {
			s.ScopeCanvas.Set(midX+d, midY, grid)
			s.ScopeCanvas.Set(midX, midY+d, grid)
		}
		ring(rad, grid)
		ring(rad*float64(activeStick.OuterDeadzone)/100, cell.Style{Fg: p.AccentBlue, Bg: p.BgPanel})
		ring(rad*float64(activeStick.InnerDeadzone)/100, cell.Style{Fg: p.Warning, Bg: p.BgPanel})

		mappedX := device.ApplyStickCurve(rawX, *activeStick)
		mappedY := device.ApplyStickCurve(rawY, *activeStick)
		dot := func(x, y float64, col cell.Color) {
			cx, cy := midX+int(math.Round(x*rad)), midY+int(math.Round(y*rad))
			st := cell.Style{Fg: col, Bg: p.BgPanel, Modifier: cell.ModifierBold}
			for d := range 4 {
				s.ScopeCanvas.Set(cx+d%2, cy+d/2, st)
			}
		}
		dot(rawX, rawY, p.AccentCyan)
		dot(mappedX, mappedY, p.Success)
		f.RenderWidget(s.ScopeCanvas, cell.NewRect(inner.X, y, scopeW, scopeH))

		// Values and legend to the right of the scope.
		lx := inner.X + scopeW + 3
		if lx+24 <= inner.X+inner.Width {
			lw := inner.X + inner.Width - lx
			row := func(dy uint16, mark string, col cell.Color, text, value string) {
				if dy >= scopeH {
					return // a short scope keeps only the first lines
				}
				f.Buffer.SetStringWithin(lx, y+dy, mark, cell.Style{Fg: col, Bg: p.BgPanel, Modifier: cell.ModifierBold}, 2)
				copyValue(f, cell.NewRect(lx+2, y+dy, lw-2, 1), s, text, value, p.LabelStyle())
			}
			raw := fmt.Sprintf("%+.2f  %+.2f", rawX, rawY)
			out := fmt.Sprintf("%+.2f  %+.2f", mappedX, mappedY)
			deflection := fmt.Sprintf("%.1f%%", math.Hypot(rawX, rawY)*100)
			row(1, "●", p.AccentCyan, i18n.T("Raw")+"     "+raw, raw)
			row(2, "●", p.Success, i18n.T("Output")+"  "+out, out)
			row(3, " ", p.FgMuted, i18n.Tf("Deflection %s", deflection), deflection)
			row(5, "○", p.Warning, i18n.Tf("Deadzone %d%%", activeStick.InnerDeadzone), fmt.Sprintf("%d%%", activeStick.InnerDeadzone))
			row(6, "○", p.AccentBlue, i18n.Tf("Max zone %d%%", activeStick.OuterDeadzone), fmt.Sprintf("%d%%", activeStick.OuterDeadzone))
		}
	}
}
