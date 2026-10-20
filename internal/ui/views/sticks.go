package views

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"math"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
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
	y := inner.Y
	// TuiSense has no input-processing backend: these values are stored in
	// the profile and visualized, but games never receive them.
	f.Buffer.SetStringWithin(inner.X, y, i18n.T("Preview only — does not change game input."), cell.Style{Fg: p.Warning, Bg: p.BgPanel}, inner.Width)
	y += 2

	// Row 0: Left vs Right Stick buttons + Link Sticks checkbox
	btnL := widgets.Button{
		ID:           "btn_stick_left",
		Label:        i18n.T("Left Stick (L3)"),
		Style:        p.ButtonStyle(!s.EditingRightStick),
		FocusedStyle: p.ButtonFocusedStyle(),
		OnPress: func() {
			s.EditingRightStick = false
			s.SelectedRegion = device.RegionLeftStick
		},
	}
	btnR := widgets.Button{
		ID:           "btn_stick_right",
		Label:        i18n.T("Right Stick (R3)"),
		Style:        p.ButtonStyle(s.EditingRightStick),
		FocusedStyle: p.ButtonFocusedStyle(),
		OnPress: func() {
			s.EditingRightStick = true
			s.SelectedRegion = device.RegionRightStick
		},
	}
	wL, _ := btnL.SizeHint(inner)
	wR, _ := btnR.SizeHint(inner)
	uiwidgets.RenderButton(f, cell.NewRect(inner.X, y, wL, 1), btnL)
	uiwidgets.RenderButton(f, cell.NewRect(inner.X+wL+1, y, wR, 1), btnR)

	if inner.X+wL+wR+3+16 <= inner.X+inner.Width {
		cb := widgets.Checkbox{
			ID:           "cb_link_sticks",
			Checked:      &s.Config.Sticks.LinkSticks,
			Label:        i18n.T("Link L3=R3"),
			Style:        p.LabelStyle(),
			FocusedStyle: p.ButtonFocusedStyle(),
		}
		f.RenderWidget(cb, cell.NewRect(inner.X+wL+wR+3, y, inner.Width-wL-wR-3, 1))
	}
	y += 2

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
	f.Buffer.SetStringWithin(inner.X, y, i18n.T("Curve:"), p.MutedStyle(), 7)
	cx := inner.X + 7
	for _, curve := range device.AllStickCurves {
		lbl := " " + i18n.T(string(curve)) + " "
		cw := uint16(cell.StringWidth(lbl))
		if cx+cw > inner.X+inner.Width {
			break
		}
		cRect := cell.NewRect(cx, y, cw, 1)
		f.Buffer.SetStringWithin(cx, y, lbl, p.ButtonStyle(activeStick.Curve == curve), cw)
		targetCurve := curve
		f.RegisterClickHandler(cRect, func(_ driver.MouseEvent) {
			activeStick.Curve = targetCurve
			syncSticks()
		})
		cx += cw + 1
	}
	y += 2

	labelW := uint16(20)
	sliderW := inner.Width - labelW - 2
	if sliderW < 8 {
		sliderW = 8
	}

	// Inner Deadzone (0..40%)
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("Inner Deadzone: %2d%%", activeStick.InnerDeadzone), p.LabelStyle(), labelW)
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
		f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("Outer Max Zone:%3d%%", activeStick.OuterDeadzone), p.LabelStyle(), labelW)
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
		f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("Sensitivity:   %3d%%", activeStick.Sensitivity), p.LabelStyle(), labelW)
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

	// Live 2D XY Stick Drift & Deadzone Scope
	remainH := int(inner.Y+inner.Height) - int(y)
	if remainH >= 4 && s.ScopeCanvas != nil {
		mappedX := device.ApplyStickCurve(rawX, *activeStick)
		mappedY := device.ApplyStickCurve(rawY, *activeStick)
		driftMag := math.Hypot(rawX, rawY) * 100.0

		f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("2D Stick Scope │ Raw:(%+.2f,%+.2f) → Out:(%+.2f,%+.2f) │ Deflection:%4.1f%%",
			rawX, rawY, mappedX, mappedY, driftMag), p.SectionHeaderStyle(), inner.Width)
		y++
		remainH--

		scopeW := min(inner.Width, uint16(36))
		scopeH := uint16(min(remainH, 7))
		s.ScopeCanvas.Reset(scopeW, scopeH)

		virtW := int(scopeW) * 2
		virtH := int(scopeH) * 4
		midX := virtW / 2
		midY := virtH / 2
		rad := float64(min(midX, midY) - 2)

		gridStyle := cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
		for gx := 0; gx < virtW; gx += 2 {
			s.ScopeCanvas.Set(gx, midY, gridStyle)
		}
		for gy := 0; gy < virtH; gy += 2 {
			s.ScopeCanvas.Set(midX, gy, gridStyle)
		}

		// Outer boundary circle & inner deadzone ring
		dzRad := rad * (float64(activeStick.InnerDeadzone) / 100.0)
		dzStyle := cell.Style{Fg: p.Warning, Bg: p.BgPanel}
		outerStyle := cell.Style{Fg: p.AccentBlue, Bg: p.BgPanel}
		for a := 0.0; a < 2*math.Pi; a += 0.08 {
			ox := midX + int(math.Cos(a)*rad*1.4)
			oy := midY + int(math.Sin(a)*rad)
			s.ScopeCanvas.Set(ox, oy, outerStyle)
			if dzRad >= 1.0 {
				dx := midX + int(math.Cos(a)*dzRad*1.4)
				dy := midY + int(math.Sin(a)*dzRad)
				s.ScopeCanvas.Set(dx, dy, dzStyle)
			}
		}

		// Live raw position dot & mapped position crosshair
		px := midX + int(rawX*rad*1.4)
		py := midY + int(rawY*rad)
		dotStyle := cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				s.ScopeCanvas.Set(px+dx, py+dy, dotStyle)
			}
		}

		f.RenderWidget(s.ScopeCanvas, cell.NewRect(inner.X, y, scopeW, scopeH))
	}
}
