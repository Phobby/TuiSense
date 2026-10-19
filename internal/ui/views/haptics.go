package views

import (
	"fmt"
	"time"

	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/state"
	uiwidgets "github.com/ygzztrk/tuisense/internal/ui/widgets"
)

// RenderHapticsPanel renders the Dual-Actuator Vibration, Test Pulse, and Speaker/Mic Audio controls.
func RenderHapticsPanel(f *terminal.Frame, area cell.Rect, s *state.AppState, dispatch func(device.Command)) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, "Haptics")
	y := inner.Y

	y = supportBanner(f, inner, y, s, device.CapHaptics)
	hasDual := s.Caps.HasFeature(device.CapHaptics, device.FeatDualMotors)
	hasTrigMotors := s.Caps.HasFeature(device.CapHaptics, device.FeatTriggerMotors)

	f.Buffer.SetStringWithin(inner.X, y, "Rumble Motors", p.SectionHeaderStyle(), inner.Width)
	y++

	// Link Motors Checkbox
	cbLink := widgets.Checkbox{
		ID:           "cb_link_motors",
		Checked:      &s.Config.Haptics.LinkMotors,
		Label:        "Link Left (Low-Freq) & Right (High-Freq) Motors",
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}
	if hasDual {
		f.RenderWidget(cbLink, cell.NewRect(inner.X, y, inner.Width, 1))
		y++
	}
	y++

	labelW := uint16(19)
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
		f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("Master Gain:   %3d%%", s.Config.Haptics.MasterIntensity), p.LabelStyle(), labelW)
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
		f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("Left Motor:     %3d", s.Config.Haptics.LeftMotor), p.LabelStyle(), labelW)
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
		f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("Right Motor:    %3d", s.Config.Haptics.RightMotor), p.LabelStyle(), labelW)
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
			f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("%-14s %3d", label, *val), p.LabelStyle(), labelW)
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
		trigSlider("slider_hap_lt", "LT Impulse:", &s.Config.Haptics.LeftTriggerMotor, s.LTMotorSlider)
		trigSlider("slider_hap_rt", "RT Impulse:", &s.Config.Haptics.RightTriggerMotor, s.RTMotorSlider)
	}
	y++

	// Live Test Vibration Buttons
	if y < inner.Y+inner.Height {
		btnLight := widgets.Button{
			ID:           "btn_test_light_pulse",
			Label:        "⚡ Light Pulse",
			Disabled:     !s.DeviceInfo.Connected,
			Style:        p.ButtonStyle(false),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress: func() {
				if dispatch != nil {
					dispatch(device.Command{Type: device.CmdPulseHaptics, PulseLeft: 95, PulseRight: 120, PulseDur: 240 * time.Millisecond})
				}
				if s.Toasts != nil {
					s.Toasts.Info("Haptic Test", "Fired light dual-actuator pulse (240ms)")
				}
			},
		}
		btnHeavy := widgets.Button{
			ID:           "btn_test_heavy_rumble",
			Label:        "💥 Heavy Impact",
			Disabled:     !s.DeviceInfo.Connected,
			Style:        p.ButtonStyle(true),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress: func() {
				if dispatch != nil {
					dispatch(device.Command{Type: device.CmdPulseHaptics, PulseLeft: 250, PulseRight: 250, PulseDur: 450 * time.Millisecond})
				}
				if s.Toasts != nil {
					s.Toasts.Success("Haptic Test", "Fired heavy impact vibration (450ms)")
				}
			},
		}
		btnStop := widgets.Button{
			ID:           "btn_stop_rumble",
			Label:        "⏹ Stop Motors",
			Disabled:     !s.DeviceInfo.Connected,
			Style:        p.ButtonStyle(false),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress: func() {
				s.Config.Haptics.LeftMotor = 0
				s.Config.Haptics.RightMotor = 0
				s.SyncWidgetsFromConfig()
				sendHaptics()
			},
		}

		w1, _ := btnLight.SizeHint(inner)
		w2, _ := btnHeavy.SizeHint(inner)
		w3, _ := btnStop.SizeHint(inner)
		uiwidgets.RenderButton(f, cell.NewRect(inner.X, y, w1, 1), btnLight)
		if inner.X+w1+1+w2 <= inner.X+inner.Width {
			uiwidgets.RenderButton(f, cell.NewRect(inner.X+w1+1, y, w2, 1), btnHeavy)
		}
		if inner.X+w1+w2+2+w3 <= inner.X+inner.Width {
			uiwidgets.RenderButton(f, cell.NewRect(inner.X+w1+w2+2, y, w3, 1), btnStop)
		}
	}
}
