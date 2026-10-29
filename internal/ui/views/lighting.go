package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
	"strings"
)

type colorPreset struct {
	Name    string
	Short   string
	R, G, B uint8
}

var colorSwatches = []colorPreset{
	{"Astro Blue", "Blue", 0, 114, 255},
	{"Cosmic Violet", "Violet", 147, 51, 234},
	{"Cyber Cyan", "Cyan", 0, 229, 255},
	{"Crimson", "Red", 244, 63, 94},
	{"Emerald", "Green", 16, 185, 129},
	{"Solar Gold", "Gold", 245, 158, 11},
	{"Starlight White", "White", 235, 242, 255},
}

// RenderLightingPanel renders the Lightbar RGB HSV ColorPicker, preset swatches,
// brightness, animation modes, Player Indicator LEDs, and Microphone Mute LED controls.
func RenderLightingPanel(f *terminal.Frame, area cell.Rect, s *state.AppState, dispatch func(device.Command)) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, i18n.T("Lighting"))
	y := inner.Y
	y = supportBanner(f, inner, y, s, device.CapLighting)
	hasRGB := s.Caps.HasFeature(device.CapLighting, device.FeatLightbarRGB)
	hasBrightness := s.Caps.HasFeature(device.CapLighting, device.FeatLEDBrightness)
	hasAnim := s.Caps.HasFeature(device.CapLighting, device.FeatLEDAnimation)
	hasPlayer := s.Caps.HasFeature(device.CapLighting, device.FeatPlayerLEDs)

	sendLighting := func() {
		l := s.Config.Lighting
		if dispatch != nil {
			dispatch(device.Command{
				Type:     device.CmdSetLighting,
				Config:   s.Config,
				Lighting: &l,
			})
		}
	}

	// Row 0: Enable Lightbar Checkbox + Live Hex Preview Swatch
	cbEnable := widgets.Checkbox{
		ID:           "cb_lightbar_enabled",
		Checked:      &s.Config.Lighting.Enabled,
		Label:        i18n.T("Enable LEDs"),
		Style:        p.LabelStyle(),
		FocusedStyle: p.ButtonFocusedStyle(),
	}
	f.RenderWidget(cbEnable, cell.NewRect(inner.X, y, 26, 1))

	swatchCol := cell.NewColorRGB(s.Config.Lighting.Red, s.Config.Lighting.Green, s.Config.Lighting.Blue)
	hexTxt := fmt.Sprintf("  ██ #%02X%02X%02X (%d,%d,%d) ",
		s.Config.Lighting.Red, s.Config.Lighting.Green, s.Config.Lighting.Blue,
		s.Config.Lighting.Red, s.Config.Lighting.Green, s.Config.Lighting.Blue)
	if hasRGB && inner.Width > 26 {
		f.Buffer.SetStringWithin(inner.X+26, y, hexTxt, cell.Style{Fg: swatchCol, Bg: p.BgElevated, Modifier: cell.ModifierBold}, inner.Width-26)
	}
	y += 3 // a blank row, then the first button row's top edge

	// Color swatches, brightness, animation and player LEDs: one button group each.
	limit := inner.X + inner.Width
	if hasRGB && y < inner.Y+inner.Height {
		var items []choice
		for _, sw := range colorSwatches {
			r, g, b := sw.R, sw.G, sw.B
			items = append(items, choice{ID: "swatch_" + sw.Name, Label: "● " + i18n.T(sw.Name), Fg: cell.NewColorRGB(r, g, b), OnSelect: func() {
				s.Config.Lighting.Enabled = true
				s.Config.Lighting.Animation = device.AnimSolid
				s.Config.Lighting.Red, s.Config.Lighting.Green, s.Config.Lighting.Blue = r, g, b
				s.ColorPickerState.SetRGB(r, g, b)
				sendLighting()
			}})
		}
		y = pillRows(f, inner.X, y, limit, s, i18n.T("Swatches:"), 12, items)
	}
	if hasBrightness && y < inner.Y+inner.Height {
		var items []choice
		for _, lvl := range []device.BrightnessLevel{device.BrightnessHigh, device.BrightnessMedium, device.BrightnessLow} {
			target := lvl
			items = append(items, choice{Kind: kindOption, ID: fmt.Sprintf("brightness_%d", lvl), Label: i18n.T(lvl.String()), Selected: s.Config.Lighting.Brightness == lvl, OnSelect: func() {
				s.Config.Lighting.Brightness = target
				sendLighting()
			}})
		}
		y = pillRows(f, inner.X, y, limit, s, i18n.T("Brightness:"), 12, items)
	}
	if hasAnim && y < inner.Y+inner.Height {
		var items []choice
		for _, anim := range device.AllLightbarAnimations {
			target := anim
			items = append(items, choice{Kind: kindOption, ID: "animation_" + string(anim), Label: i18n.T(string(anim)), Selected: s.Config.Lighting.Animation == anim, OnSelect: func() {
				s.Config.Lighting.Animation = target
				s.Config.Lighting.Enabled = target != device.AnimOff
				sendLighting()
			}})
		}
		y = pillRows(f, inner.X, y, limit, s, i18n.T("Animation:"), 12, items)
	}
	if hasPlayer && y < inner.Y+inner.Height {
		masks := []struct {
			num  int
			mask uint8
			str  string
		}{
			{1, 0x04, "P1 (••█••)"},
			{2, 0x0A, "P2 (•█•█•)"},
			{3, 0x15, "P3 (█•█•█)"},
			{4, 0x1B, "P4 (██•██)"},
			{0, 0x00, strings.TrimSpace(i18n.T(" Off "))},
		}
		var items []choice
		for _, m := range masks {
			num, mask := m.num, m.mask
			items = append(items, choice{Kind: kindOption, ID: fmt.Sprintf("player_%d", m.num), Label: m.str, Selected: s.Config.Lighting.PlayerMask == m.mask, OnSelect: func() {
				s.Config.Lighting.PlayerNumber = num
				s.Config.Lighting.PlayerMask = mask
				sendLighting()
			}})
		}
		y = pillRows(f, inner.X, y, limit, s, i18n.T("Player LED:"), 12, items)
	}

	// Interactive 2D HSV ColorPicker (uses Limoni's built-in widgets.ColorPicker)
	remainH := int(inner.Y+inner.Height) - int(y)
	if hasRGB && remainH >= 7 && inner.Width >= 28 {
		cpArea := cell.NewRect(inner.X, y, inner.Width, uint16(min(remainH, 9)))
		prevR, prevG, prevB := s.ColorPickerState.Red, s.ColorPickerState.Green, s.ColorPickerState.Blue
		cp := widgets.ColorPicker{
			ID:          "lightbar_color_picker",
			State:       s.ColorPickerState,
			ShowPreview: true,
			Style:       p.PanelStyle(),
		}
		f.RenderWidget(cp, cpArea)

		// Sync back if interactive mouse click changed ColorPickerState
		if s.ColorPickerState.Red != prevR || s.ColorPickerState.Green != prevG || s.ColorPickerState.Blue != prevB {
			s.Config.Lighting.Red = s.ColorPickerState.Red
			s.Config.Lighting.Green = s.ColorPickerState.Green
			s.Config.Lighting.Blue = s.ColorPickerState.Blue
			s.Config.Lighting.Enabled = true
			sendLighting()
		}
	}
}
