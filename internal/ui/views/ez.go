package views

import (
	"fmt"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/theme"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// EZ mode views: a handful of plain-language choices with generous spacing.
// They share the action helpers below with Advanced mode, so both modes
// change the same underlying config.

// vibrationLevels are the EZ intensity steps mapped to master haptic gain.
var vibrationLevels = []struct {
	Name  string
	Value int
}{
	{"Off", 0}, {"Low", 35}, {"Medium", 70}, {"High", 100},
}

// triggerFeels are the EZ adaptive trigger presets applied to both triggers.
var triggerFeels = []struct {
	Name, Description string
	Config            device.TriggerConfig
}{
	{"Off", "No resistance — smooth and free.", device.BuiltinTriggerPresets[0].Config},
	{"Soft", "Light, even resistance.", device.TriggerConfig{Mode: device.TriggerModeResistance, StartPosition: 40, EndPosition: 255, Strength: 110}},
	{"Firm", "Heavy bow-draw resistance.", device.BuiltinTriggerPresets[1].Config},
	{"Weapon", "Crisp click like a trigger break.", device.BuiltinTriggerPresets[2].Config},
	{"Brake", "Rumbling ABS brake pedal.", device.BuiltinTriggerPresets[3].Config},
}

func nearestVibration(v int) int {
	best, bestD := 0, 1<<30
	for i, l := range vibrationLevels {
		d := l.Value - v
		if d < 0 {
			d = -d
		}
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func setVibration(pc PanelContext, v int) {
	pc.S.Config.Haptics.MasterIntensity = v
	pc.S.MasterHapSlider.Set(v, 0, 100)
	pc.dispatch(device.Command{Type: device.CmdSetHaptics, Config: pc.S.Config})
}

func testPulse(pc PanelContext) {
	s := pc.S
	for _, a := range s.Descriptor.Actions {
		if a.ID == "pulse_test" && s.Caps.Supports(a.Capability) {
			pc.dispatch(a.Command)
			if s.Toasts != nil {
				s.Toasts.Info(a.Label, s.Descriptor.ShortName+" pulsed")
			}
			return
		}
	}
}

func setLightColor(pc PanelContext, r, g, b uint8) {
	s := pc.S
	s.Config.Lighting.Enabled = true
	s.Config.Lighting.Animation = device.AnimSolid
	s.Config.Lighting.Red, s.Config.Lighting.Green, s.Config.Lighting.Blue = r, g, b
	s.ColorPickerState.SetRGB(r, g, b)
	l := s.Config.Lighting
	pc.dispatch(device.Command{Type: device.CmdSetLighting, Config: s.Config, Lighting: &l})
}

func setBrightness(pc PanelContext, b device.BrightnessLevel) {
	s := pc.S
	s.Config.Lighting.Enabled = true
	s.Config.Lighting.Brightness = b
	l := s.Config.Lighting
	pc.dispatch(device.Command{Type: device.CmdSetLighting, Config: s.Config, Lighting: &l})
}

func applyTriggerFeel(pc PanelContext, tc device.TriggerConfig) {
	s := pc.S
	s.Config.LeftTrigger = tc
	s.Config.RightTrigger = tc
	s.SyncWidgetsFromConfig()
	pc.dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
}

func currentTriggerFeel(s *state.AppState) int {
	for i, tf := range triggerFeels {
		if s.Config.LeftTrigger == tf.Config && s.Config.RightTrigger == tf.Config {
			return i
		}
	}
	return -1
}

func applyProfile(pc PanelContext, prof state.Profile) {
	s := pc.S
	s.Config = prof.Config
	s.ActiveProfile = prof.Name
	s.SyncWidgetsFromConfig()
	pc.dispatch(device.Command{Type: device.CmdApplyFullConfig, Config: s.Config})
	pc.prefsChanged()
	if s.Toasts != nil {
		s.Toasts.Success("Profile", "Applied "+prof.Name)
	}
}

func cycleProfile(pc PanelContext, delta int) {
	list := pc.S.ProfileMgr.ListFor(pc.S.Descriptor.Family)
	if len(list) == 0 {
		return
	}
	idx := 0
	for i, p := range list {
		if p.Name == pc.S.ActiveProfile {
			idx = i
		}
	}
	applyProfile(pc, list[((idx+delta)%len(list)+len(list))%len(list)])
}

func lightChoices(pc PanelContext) []choice {
	s := pc.S
	cfg := s.Config.Lighting
	var items []choice
	if s.Caps.HasFeature(device.CapLighting, device.FeatLightbarRGB) {
		for i, sw := range colorSwatches {
			sw := sw
			items = append(items, choice{
				ID:       fmt.Sprintf("ez_light_%d", i),
				Label:    "● " + sw.Short,
				Selected: cfg.Enabled && cfg.Red == sw.R && cfg.Green == sw.G && cfg.Blue == sw.B,
				OnSelect: func() { setLightColor(pc, sw.R, sw.G, sw.B) },
			})
		}
		return items
	}
	for _, lvl := range []device.BrightnessLevel{device.BrightnessLow, device.BrightnessMedium, device.BrightnessHigh} {
		lvl := lvl
		name := map[device.BrightnessLevel]string{device.BrightnessLow: "Dim", device.BrightnessMedium: "Medium", device.BrightnessHigh: "Bright"}[lvl]
		items = append(items, choice{
			ID:       "ez_bright_" + name,
			Label:    name,
			Selected: cfg.Enabled && cfg.Brightness == lvl,
			OnSelect: func() { setBrightness(pc, lvl) },
		})
	}
	return items
}

func vibrationChoices(pc PanelContext) []choice {
	cur := nearestVibration(pc.S.Config.Haptics.MasterIntensity)
	items := make([]choice, 0, len(vibrationLevels))
	for i, l := range vibrationLevels {
		v := l.Value
		items = append(items, choice{
			ID:       "ez_vib_" + l.Name,
			Label:    l.Name,
			Selected: i == cur,
			OnSelect: func() { setVibration(pc, v) },
		})
	}
	return items
}

// renderEZHome is the EZ overview: a short status sentence and one row per
// supported quick setting.
func renderEZHome(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	inner := panelFrame(f, area, s, "Quick Settings")
	y := inner.Y
	limit := inner.X + inner.Width
	bottom := inner.Y + inner.Height

	if !s.DeviceInfo.Connected {
		f.Buffer.SetStringWithin(inner.X, y, "No controller is connected yet.", cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold}, inner.Width)
		y += 2
		if y < bottom {
			pillRow(f, inner.X, y, limit, s, "", 0, []choice{
				{ID: "btn_overview_reconnect", Label: "Use a simulated controller [D]", OnSelect: pc.CB.OnToggleConnect},
			})
		}
		return
	}

	status := fmt.Sprintf("%s is ready", s.Descriptor.ShortName)
	if s.Caps.Supports(device.CapBattery) {
		status += fmt.Sprintf(" · battery %d%%", s.Telemetry.BatteryPercent)
		if s.Telemetry.Charging {
			status += " (charging)"
		}
	}
	f.Buffer.SetStringWithin(inner.X, y, status, cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold}, inner.Width)
	y++
	if s.RegionHint != "" && y < bottom {
		f.Buffer.SetStringWithin(inner.X, y, "ℹ "+s.RegionHint, cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel}, inner.Width)
		y++
	}
	y++

	const labelW = 12
	row := func(label string, items []choice) {
		if y >= bottom {
			return
		}
		pillRow(f, inner.X, y, limit, s, label, labelW, items)
		y += 2
	}

	if s.Caps.Supports(device.CapLighting) {
		row("Lights", lightChoices(pc))
	}
	if s.Caps.Supports(device.CapHaptics) {
		items := vibrationChoices(pc)
		items = append(items, choice{ID: "ez_test", Label: "▶ Test", OnSelect: func() { testPulse(pc) }})
		row("Vibration", items)
	}
	if s.Caps.Supports(device.CapAdaptiveTriggers) {
		cur := currentTriggerFeel(s)
		var items []choice
		for i, tf := range triggerFeels[:4] {
			tc := tf.Config
			items = append(items, choice{ID: "ez_trig_" + tf.Name, Label: tf.Name, Selected: i == cur, OnSelect: func() { applyTriggerFeel(pc, tc) }})
		}
		row("Triggers", items)
	}
	if s.Caps.Supports(device.CapProfiles) {
		row("Profile", []choice{
			{ID: "ez_profile_prev", Label: "‹", OnSelect: func() { cycleProfile(pc, -1) }},
			{ID: "ez_profile_name", Label: s.ActiveProfile, Selected: true},
			{ID: "ez_profile_next", Label: "›", OnSelect: func() { cycleProfile(pc, 1) }},
		})
	}

	if bottom > inner.Y+1 && bottom-1 > y {
		f.Buffer.SetStringWithin(inner.X, bottom-1, "Tip: click a part of the controller · [E] shows every setting", p.MutedStyle(), inner.Width)
	}
}

func renderEZLighting(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	inner := panelFrame(f, area, s, "Lights")
	y := supportBanner(f, inner, inner.Y, s, device.CapLighting)
	limit := inner.X + inner.Width
	enabled := s.Config.Lighting.Enabled
	pillRow(f, inner.X, y, limit, s, "Power", 12, []choice{
		{ID: "ez_light_on", Label: "On", Selected: enabled, OnSelect: func() {
			s.Config.Lighting.Enabled = true
			if s.Config.Lighting.Animation == device.AnimOff {
				s.Config.Lighting.Animation = device.AnimSolid
			}
			l := s.Config.Lighting
			pc.dispatch(device.Command{Type: device.CmdSetLighting, Config: s.Config, Lighting: &l})
		}},
		{ID: "ez_light_off", Label: "Off", Selected: !enabled, OnSelect: func() {
			s.Config.Lighting.Enabled = false
			l := s.Config.Lighting
			pc.dispatch(device.Command{Type: device.CmdSetLighting, Config: s.Config, Lighting: &l})
		}},
	})
	y += 2
	items := lightChoices(pc)
	label := "Color"
	if !s.Caps.HasFeature(device.CapLighting, device.FeatLightbarRGB) {
		label = "Brightness"
	}
	// Wrap the choices across rows so each stays easy to hit.
	for len(items) > 0 && y < inner.Y+inner.Height {
		n := min(len(items), 4)
		pillRow(f, inner.X, y, limit, s, label, 12, items[:n])
		items = items[n:]
		label = ""
		y += 2
	}
}

func renderEZHaptics(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	inner := panelFrame(f, area, s, "Vibration")
	y := supportBanner(f, inner, inner.Y, s, device.CapHaptics)
	limit := inner.X + inner.Width
	f.Buffer.SetStringWithin(inner.X, y, "How strong should vibration feel in games?", p.LabelStyle(), inner.Width)
	y += 2
	pillRow(f, inner.X, y, limit, s, "Strength", 12, vibrationChoices(pc))
	y += 2
	if y < inner.Y+inner.Height {
		pillRow(f, inner.X, y, limit, s, "Try it", 12, []choice{
			{ID: "ez_test", Label: "▶ Test vibration", OnSelect: func() { testPulse(pc) }},
		})
		y += 2
	}
	if s.Caps.HasFeature(device.CapHaptics, device.FeatTriggerMotors) && y < inner.Y+inner.Height {
		level := s.Config.Haptics.LeftTriggerMotor
		set := func(v uint8) func() {
			return func() {
				s.Config.Haptics.LeftTriggerMotor, s.Config.Haptics.RightTriggerMotor = v, v
				s.SyncWidgetsFromConfig()
				pc.dispatch(device.Command{Type: device.CmdSetHaptics, Config: s.Config})
			}
		}
		pillRow(f, inner.X, y, limit, s, "Triggers", 12, []choice{
			{ID: "ez_trigmotor_off", Label: "Off", Selected: level == 0, OnSelect: set(0)},
			{ID: "ez_trigmotor_low", Label: "Low", Selected: level > 0 && level <= 130, OnSelect: set(120)},
			{ID: "ez_trigmotor_high", Label: "High", Selected: level > 130, OnSelect: set(230)},
		})
	}
}

func renderEZTriggers(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	inner := panelFrame(f, area, s, "Trigger Feel")
	y := supportBanner(f, inner, inner.Y, s, device.CapAdaptiveTriggers)
	f.Buffer.SetStringWithin(inner.X, y, "Pick how the triggers push back. Applies to both triggers.", p.LabelStyle(), inner.Width)
	y += 2
	cur := currentTriggerFeel(s)
	for i, tf := range triggerFeels {
		if y >= inner.Y+inner.Height {
			break
		}
		tc := tf.Config
		x := pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{
			{ID: "ez_trig_" + tf.Name, Label: fmt.Sprintf("%-7s", tf.Name), Selected: i == cur, OnSelect: func() { applyTriggerFeel(pc, tc) }},
		})
		if x < inner.X+inner.Width {
			f.Buffer.SetStringWithin(x+1, y, tf.Description, p.MutedStyle(), inner.X+inner.Width-x-1)
		}
		y += 2
	}
}

func renderEZProfiles(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	inner := panelFrame(f, area, s, "Profiles")
	y := inner.Y
	f.Buffer.SetStringWithin(inner.X, y, "Choose a ready-made setup:", p.LabelStyle(), inner.Width)
	y += 2
	for i, prof := range s.ProfileMgr.ListFor(s.Descriptor.Family) {
		if y >= inner.Y+inner.Height {
			break
		}
		prof := prof
		x := pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{
			{ID: fmt.Sprintf("ez_profile_%d", i), Label: fmt.Sprintf("%-20s", prof.Name), Selected: prof.Name == s.ActiveProfile, OnSelect: func() { applyProfile(pc, prof) }},
		})
		if x < inner.X+inner.Width {
			f.Buffer.SetStringWithin(x+1, y, prof.Description, p.MutedStyle(), inner.X+inner.Width-x-1)
		}
		y++
	}
}

func renderEZSettings(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	inner := panelFrame(f, area, s, "Settings")
	y := renderModeChooser(f, inner, inner.Y, s, pc.CB)
	y++
	if y < inner.Y+inner.Height {
		f.RenderWidget(widgets.Checkbox{
			ID: "cb_settings_autorot", Checked: &s.AutoRotate, Label: "Slowly rotate the controller",
			Style: p.LabelStyle(), FocusedStyle: p.ButtonFocusedStyle(),
		}, cell.NewRect(inner.X, y, inner.Width, 1))
		y += 2
	}
	if y < inner.Y+inner.Height {
		prevHC := s.HighContrast
		f.RenderWidget(widgets.Checkbox{
			ID: "cb_settings_high_contrast", Checked: &s.HighContrast, Label: "High-contrast colors",
			Style: p.LabelStyle(), FocusedStyle: p.ButtonFocusedStyle(),
		}, cell.NewRect(inner.X, y, inner.Width, 1))
		if s.HighContrast != prevHC {
			if s.HighContrast {
				s.Theme = theme.HighContrast()
			} else {
				s.Theme = theme.Default()
			}
			pc.prefsChanged()
		}
	}
}
