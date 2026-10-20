package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// renderAudioPanel renders speaker, headset, microphone gain, and mute LED controls,
// each shown only when the active device declares the matching audio feature.
func renderAudioPanel(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	inner := panelFrame(f, area, s, i18n.T("Audio & Microphone"))
	y := supportBanner(f, inner, inner.Y, s, device.CapAudio)
	y++

	labelW := uint16(19)
	sliderW := inner.Width - labelW - 2
	if sliderW < 8 {
		sliderW = 8
	}
	sendAudio := func() {
		pc.dispatch(device.Command{Type: device.CmdSetSpeakerVol, Config: s.Config})
	}
	volume := func(id, label string, val *int, st *widgets.SliderState, col cell.Color) {
		if y >= inner.Y+inner.Height {
			return
		}
		f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("%-14s%3d%%", label, *val), p.LabelStyle(), labelW)
		f.RenderWidget(widgets.Slider{
			ID:           id,
			State:        st,
			Min:          0,
			Max:          100,
			TrackStyle:   cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
			FilledStyle:  cell.Style{Fg: col, Bg: p.BgPanel},
			ThumbStyle:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold},
			FocusedStyle: cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel},
			OnChange: func(v int) {
				*val = v
				sendAudio()
			},
		}, cell.NewRect(inner.X+labelW, y, sliderW, 1))
		y++
	}

	if s.Caps.HasFeature(device.CapAudio, device.FeatSpeaker) {
		volume("slider_speaker_vol", i18n.T("Speaker:"), &s.Config.Audio.SpeakerVolume, s.SpeakerVolSlider, p.Success)
	}
	if s.Caps.HasFeature(device.CapAudio, device.FeatHeadset) {
		volume("slider_headset_vol", i18n.T("Headset:"), &s.Config.Audio.HeadphoneVolume, s.HeadsetVolSlider, p.AccentBlue)
	}
	if s.Caps.HasFeature(device.CapAudio, device.FeatMicrophone) {
		volume("slider_mic_gain", i18n.T("Mic Gain:"), &s.Config.Audio.MicGain, s.MicGainSlider, p.AccentPurple)
	}

	if s.Caps.HasFeature(device.CapLighting, device.FeatMicLED) && y+1 < inner.Y+inner.Height {
		y++
		var items []choice
		for _, mm := range []device.MicLEDMode{device.MicLEDOff, device.MicLEDSolid, device.MicLEDPulse} {
			mode := mm
			items = append(items, choice{
				ID:       "mic_led_" + mode.String(),
				Label:    i18n.T(mode.String()),
				Selected: s.Config.Lighting.MicLED == mode,
				OnSelect: func() {
					s.Config.Lighting.MicLED = mode
					l := s.Config.Lighting
					pc.dispatch(device.Command{Type: device.CmdSetLighting, Config: s.Config, Lighting: &l})
				},
			})
		}
		pillRow(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Mute LED:"), 11, items)
	}
}

// renderFirmwarePanel shows firmware identity and the (simulated) update check.
// It is only reachable for devices declaring CapFirmwareUpdate.
func renderFirmwarePanel(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	inner := panelFrame(f, area, s, i18n.T("Firmware"))
	y := supportBanner(f, inner, inner.Y, s, device.CapFirmwareUpdate)
	y++

	rows := [][2]string{
		{i18n.T("Installed"), nonEmptyStr(s.DeviceInfo.FirmwareVersion, "unknown")},
		{i18n.T("Hardware"), nonEmptyStr(s.DeviceInfo.HardwareVersion, "unknown")},
		{i18n.T("Model"), s.Descriptor.DisplayName},
	}
	for _, r := range rows {
		if y >= inner.Y+inner.Height {
			return
		}
		f.Buffer.SetStringWithin(inner.X, y, r[0], p.MutedStyle(), 12)
		f.Buffer.SetStringWithin(inner.X+12, y, r[1], p.ValueStyle(), inner.Width-12)
		y++
	}
	y++
	if y < inner.Y+inner.Height {
		pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{{
			ID:    "btn_firmware_check",
			Label: i18n.T("⟳ Check for updates"),
			OnSelect: func() {
				if s.Toasts != nil {
					s.Toasts.Info(i18n.T("Firmware"), i18n.Tf("%s firmware %s is up to date (simulated check).",
						s.Descriptor.ShortName, s.DeviceInfo.FirmwareVersion))
				}
			},
		}})
	}
}
