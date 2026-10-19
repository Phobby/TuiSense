package views

import (
	"fmt"

	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/state"
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

	inner := panelFrame(f, area, s, "Lighting")
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
		Label:        "Enable LEDs",
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
	y++

	// Row 1: Color Swatches
	if hasRGB && y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, "Swatches:", p.MutedStyle(), 10)
		sx := inner.X + 10
		for _, sw := range colorSwatches {
			pill := fmt.Sprintf(" ● %s ", sw.Name)
			pw := uint16(cell.StringWidth(pill))
			if sx+pw > inner.X+inner.Width {
				break
			}
			sRect := cell.NewRect(sx, y, pw, 1)
			col := cell.NewColorRGB(sw.R, sw.G, sw.B)
			f.Buffer.SetStringWithin(sx, y, pill, cell.Style{Fg: col, Bg: p.BgElevated, Modifier: cell.ModifierBold}, pw)
			r, g, b := sw.R, sw.G, sw.B
			f.RegisterClickHandler(sRect, func(_ driver.MouseEvent) {
				s.Config.Lighting.Enabled = true
				s.Config.Lighting.Animation = device.AnimSolid
				s.Config.Lighting.Red, s.Config.Lighting.Green, s.Config.Lighting.Blue = r, g, b
				s.ColorPickerState.SetRGB(r, g, b)
				sendLighting()
			})
			sx += pw + 1
		}
		y++
	}

	// Row 2: Brightness Presets
	if hasBrightness && y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, "Brightness:", p.MutedStyle(), 12)
		bx := inner.X + 12
		levels := []device.BrightnessLevel{device.BrightnessHigh, device.BrightnessMedium, device.BrightnessLow}
		for _, lvl := range levels {
			lbl := " " + lvl.String() + " "
			lw := uint16(cell.StringWidth(lbl))
			if bx+lw > inner.X+inner.Width {
				break
			}
			bRect := cell.NewRect(bx, y, lw, 1)
			f.Buffer.SetStringWithin(bx, y, lbl, p.ButtonStyle(s.Config.Lighting.Brightness == lvl), lw)
			targetLvl := lvl
			f.RegisterClickHandler(bRect, func(_ driver.MouseEvent) {
				s.Config.Lighting.Brightness = targetLvl
				sendLighting()
			})
			bx += lw + 1
		}
		y++
	}

	// Row 3: Animation Mode Selector
	if hasAnim && y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, "Animation:", p.MutedStyle(), 12)
		ax := inner.X + 12
		for _, anim := range device.AllLightbarAnimations {
			lbl := " " + string(anim) + " "
			aw := uint16(cell.StringWidth(lbl))
			if ax+aw > inner.X+inner.Width {
				break
			}
			aRect := cell.NewRect(ax, y, aw, 1)
			f.Buffer.SetStringWithin(ax, y, lbl, p.ButtonStyle(s.Config.Lighting.Animation == anim), aw)
			targetAnim := anim
			f.RegisterClickHandler(aRect, func(_ driver.MouseEvent) {
				s.Config.Lighting.Animation = targetAnim
				s.Config.Lighting.Enabled = targetAnim != device.AnimOff
				sendLighting()
			})
			ax += aw + 1
		}
		y++
	}

	// Row 4: Player Indicator LEDs (P1..P4)
	if hasPlayer && y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, "Player LED:", p.MutedStyle(), 12)
		px := inner.X + 12
		masks := []struct {
			num  int
			mask uint8
			str  string
		}{
			{1, 0x04, " P1 (••█••) "},
			{2, 0x0A, " P2 (•█•█•) "},
			{3, 0x15, " P3 (█•█•█) "},
			{4, 0x1B, " P4 (██•██) "},
			{0, 0x00, " Off "},
		}
		for _, m := range masks {
			mw := uint16(cell.StringWidth(m.str))
			if px+mw > inner.X+inner.Width {
				break
			}
			pRect := cell.NewRect(px, y, mw, 1)
			f.Buffer.SetStringWithin(px, y, m.str, p.ButtonStyle(s.Config.Lighting.PlayerMask == m.mask), mw)
			targetNum, targetMask := m.num, m.mask
			f.RegisterClickHandler(pRect, func(_ driver.MouseEvent) {
				s.Config.Lighting.PlayerNumber = targetNum
				s.Config.Lighting.PlayerMask = targetMask
				sendLighting()
			})
			px += mw + 1
		}
		y++
	}

	y++

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
