package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"math"

	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// RenderMotionPanel renders the 6-Axis IMU Gyroscope and Accelerometer settings and live telemetry gauges.
func RenderMotionPanel(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, i18n.T("Motion Sensors"))
	y := inner.Y

	f.Buffer.SetStringWithin(inner.X, y, i18n.T("IMU Sensor Configuration"), p.SectionHeaderStyle(), inner.Width)
	y++

	// Checkboxes
	f.RenderWidget(widgets.Checkbox{
		ID:           "cb_gyro_enabled",
		Checked:      &s.Config.Motion.GyroEnabled,
		Label:        i18n.T("Enable 3-Axis Gyroscope Telemetry"),
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}, cell.NewRect(inner.X, y, inner.Width, 1))
	y++

	f.RenderWidget(widgets.Checkbox{
		ID:           "cb_accel_enabled",
		Checked:      &s.Config.Motion.AccelEnabled,
		Label:        i18n.T("Enable 3-Axis Linear Accelerometer"),
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}, cell.NewRect(inner.X, y, inner.Width, 1))
	y++

	f.RenderWidget(widgets.Checkbox{
		ID:           "cb_sync_3d_tilt",
		Checked:      &s.Config.Motion.Sync3DViewTilt,
		Label:        i18n.T("Link Live Gyro Tilt to the 3D Viewport"),
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}, cell.NewRect(inner.X, y, inner.Width, 1))
	y++

	f.RenderWidget(widgets.Checkbox{
		ID:           "cb_invert_pitch",
		Checked:      &s.Config.Motion.InvertPitch,
		Label:        i18n.T("Invert Gyro Pitch Axis"),
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}, cell.NewRect(inner.X, y, inner.Width, 1))
	y += 2

	labelW := uint16(19)
	sliderW := inner.Width - labelW - 2
	if sliderW < 8 {
		sliderW = 8
	}

	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("Gyro Sensitivity:%3d%%", s.Config.Motion.GyroSensitivity), p.LabelStyle(), labelW)
		f.RenderWidget(widgets.Slider{
			ID:           "slider_gyro_sens",
			State:        s.GyroSensSlider,
			Min:          10,
			Max:          200,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel},
			OnChange: func(v int) {
				s.Config.Motion.GyroSensitivity = v
			},
		}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y += 2
	}

	// Live IMU Telemetry Bars
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.T("Live 6-Axis IMU Telemetry"), p.SectionHeaderStyle(), inner.Width)
		y++
	}

	tel := s.Telemetry
	drawAxisBar := func(id, name string, val, maxAbs float64, unit string, col cell.Color) {
		if y >= inner.Y+inner.Height {
			return
		}
		norm := math.Min(1.0, math.Max(0.0, (val+maxAbs)/(2.0*maxAbs)))
		lbl := fmt.Sprintf("%-11s %+6.1f%s", name, val, unit)
		f.Buffer.SetStringWithin(inner.X, y, lbl, p.LabelStyle(), labelW)
		lg := widgets.LineGauge{
			ID:            id,
			Ratio:         norm,
			HideLabel:     true,
			FilledStyle:   cell.Style{Fg: col, Bg: p.BgPanel},
			UnfilledStyle: cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
		}
		f.RenderWidget(lg, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y++
	}

	drawAxisBar("gauge_gyro_pitch", i18n.T("Gyro Pitch:"), tel.GyroPitch, 45.0, "°", p.AccentCyan)
	drawAxisBar("gauge_gyro_yaw", i18n.T("Gyro Yaw:"), tel.GyroYaw, 45.0, "°", p.AccentBlue)
	drawAxisBar("gauge_gyro_roll", i18n.T("Gyro Roll:"), tel.GyroRoll, 45.0, "°", p.AccentPurple)
	drawAxisBar("gauge_accel_x", i18n.T("Accel X:"), tel.AccelX, 2.0, "g", p.Success)
	drawAxisBar("gauge_accel_y", i18n.T("Accel Y:"), tel.AccelY, 2.0, "g", p.Warning)
	drawAxisBar("gauge_accel_z", i18n.T("Accel Z:"), tel.AccelZ, 2.0, "g", p.AccentPink)
}
