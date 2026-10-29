package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// RenderHapticsPanel renders the Dual-Actuator Vibration, Test Pulse, and Speaker/Mic Audio controls.
func RenderHapticsPanel(f *terminal.Frame, area cell.Rect, s *state.AppState, dispatch func(device.Command)) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, i18n.T("Haptics"))
	y := inner.Y

	y = supportBanner(f, inner, y, s, device.CapHaptics)
	hasDual := s.Caps.HasFeature(device.CapHaptics, device.FeatDualMotors)
	hasTrigMotors := s.Caps.HasFeature(device.CapHaptics, device.FeatTriggerMotors)

	f.Buffer.SetStringWithin(inner.X, y, i18n.T("Rumble Motors"), p.SectionHeaderStyle(), inner.Width)
	y++

	// Link Motors Checkbox
	cbLink := widgets.Checkbox{
		ID:           "cb_link_motors",
		Checked:      &s.Config.Haptics.LinkMotors,
		Label:        i18n.T("Link Left (Low-Freq) & Right (High-Freq) Motors"),
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}
	if hasDual {
		f.RenderWidget(cbLink, cell.NewRect(inner.X, y, inner.Width, 1))
		y++
	}
	y++

	labelW := sliderColumn(4, i18n.T("Master Gain:"), i18n.T("Left Motor:"), i18n.T("Right Motor:"), i18n.T("LT Impulse:"), i18n.T("RT Impulse:"))
	sliderW := inner.Width - labelW - 2
	if sliderW < 8 {
		sliderW = 8
	}

	sendHaptics := func() {
		if dispatch != nil {
			dispatch(device.Command{
				Type:   device.CmdSetHaptics,
				Config: s.Config,
			})
		}
	}

	// Master Haptic Intensity (0..100%)
	if y < inner.Y+inner.Height {
		sliderLabel(f, cell.NewRect(inner.X, y, labelW, 1), s, i18n.T("Master Gain:"), fmt.Sprintf("%d%%", s.Config.Haptics.MasterIntensity))
		f.RenderWidget(widgets.Slider{
			ID:           "slider_hap_master",
			State:        s.MasterHapSlider,
			Min:          0,
			Max:          100,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			OnChange: func(v int) {
				s.Config.Haptics.MasterIntensity = v
				sendHaptics()
			},
		}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y++
	}

	// Left Motor (0..255)
	if hasDual && y < inner.Y+inner.Height {
		sliderLabel(f, cell.NewRect(inner.X, y, labelW, 1), s, i18n.T("Left Motor:"), fmt.Sprint(s.Config.Haptics.LeftMotor))
		f.RenderWidget(widgets.Slider{
			ID:           "slider_hap_left",
			State:        s.LeftMotorSlider,
			Min:          0,
			Max:          255,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel},
			OnChange: func(v int) {
				s.Config.Haptics.LeftMotor = uint8(v)
				if s.Config.Haptics.LinkMotors {
					s.Config.Haptics.RightMotor = uint8(v)
					s.RightMotorSlider.Set(v, 0, 255)
				}
				sendHaptics()
			},
		}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y++
	}

	// Right Motor (0..255)
	if hasDual && y < inner.Y+inner.Height {
		sliderLabel(f, cell.NewRect(inner.X, y, labelW, 1), s, i18n.T("Right Motor:"), fmt.Sprint(s.Config.Haptics.RightMotor))
		f.RenderWidget(widgets.Slider{
			ID:           "slider_hap_right",
			State:        s.RightMotorSlider,
			Min:          0,
			Max:          255,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel},
			OnChange: func(v int) {
				s.Config.Haptics.RightMotor = uint8(v)
				if s.Config.Haptics.LinkMotors {
					s.Config.Haptics.LeftMotor = uint8(v)
					s.LeftMotorSlider.Set(v, 0, 255)
				}
				sendHaptics()
			},
		}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y++
	}

	// Impulse trigger motors (Xbox-style)
	if hasTrigMotors {
		trigSlider := func(id, label string, val *uint8, st *widgets.SliderState) {
			if y >= inner.Y+inner.Height {
				return
			}
			sliderLabel(f, cell.NewRect(inner.X, y, labelW, 1), s, label, fmt.Sprint(*val))
			f.RenderWidget(widgets.Slider{
				ID:           id,
				State:        st,
				Min:          0,
				Max:          255,
				TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
				FilledStyle:  cell.Style{Fg: p.AccentBlue, Bg: p.BgPanel},
				ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
				FocusedStyle: cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
				OnChange: func(v int) {
					*val = uint8(v)
					sendHaptics()
				},
			}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
			y++
		}
		trigSlider("slider_hap_lt", i18n.T("LT Impulse:"), &s.Config.Haptics.LeftTriggerMotor, s.LTMotorSlider)
		trigSlider("slider_hap_rt", i18n.T("RT Impulse:"), &s.Config.Haptics.RightTriggerMotor, s.RTMotorSlider)
	}
	y += 2 // a blank row, then the buttons' top edge

	// Live Test Vibration Buttons
	if y < inner.Y+inner.Height {
		off := !s.DeviceInfo.Connected
		pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{
			{ID: "btn_test_light_pulse", Label: i18n.T("Light Pulse"), Disabled: off, OnSelect: func() {
				if dispatch != nil {
					dispatch(device.Command{Type: device.CmdPulseHaptics, PulseLeft: 95, PulseRight: 120, PulseDur: 240 * time.Millisecond})
				}
				if s.Toasts != nil {
					s.Toasts.Info(i18n.T("Haptic Test"), i18n.T("Fired light dual-actuator pulse (240ms)"))
				}
			}},
			{ID: "btn_test_heavy_rumble", Label: i18n.T("Heavy Impact"), Disabled: off, OnSelect: func() {
				if dispatch != nil {
					dispatch(device.Command{Type: device.CmdPulseHaptics, PulseLeft: 250, PulseRight: 250, PulseDur: 450 * time.Millisecond})
				}
				if s.Toasts != nil {
					s.Toasts.Success(i18n.T("Haptic Test"), i18n.T("Fired heavy impact vibration (450ms)"))
				}
			}},
			{ID: "btn_stop_rumble", Label: i18n.T("Stop Motors"), Disabled: off, OnSelect: func() {
				s.Config.Haptics.LeftMotor = 0
				s.Config.Haptics.RightMotor = 0
				s.SyncWidgetsFromConfig()
				sendHaptics()
			}},
		})
	}
}
