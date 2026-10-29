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

	labelW := sliderColumn(4, i18n.T("Speaker:"), i18n.T("Headset:"), i18n.T("Mic Gain:"))
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
		sliderLabel(f, cell.NewRect(inner.X, y, labelW, 1), s, label, fmt.Sprintf("%d%%", *val))
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
				Kind:     kindOption,
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
