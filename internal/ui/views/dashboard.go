package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/ui"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// RenderDashboard renders the complete TuiSense frame. Every region is driven
// by the active device (descriptor + capabilities) and the UX mode.
func RenderDashboard(f *terminal.Frame, s *state.AppState, dispatch func(device.Command), cb Callbacks) {
	total := f.Area()
	if total.Width == 0 || total.Height == 0 {
		return
	}
	p := s.Theme
	f.SetTheme(p.LimoniTheme())

	bgStyle := p.CanvasStyle()
	for y := total.Y; y < total.Y+total.Height; y++ {
		for x := total.X; x < total.X+total.Width; x++ {
			f.Buffer.SetCell(x, y, cell.Cell{Content: ' ', Style: bgStyle})
		}
	}

	grid := ui.ComputeLayout(total, s.ActivePanel == state.PanelDiagnostics)
	pc := PanelContext{S: s, Dispatch: dispatch, CB: cb}

	renderHeader(f, grid.HeaderArea, s, cb)
	renderPanelTabs(f, grid.TabsArea, s)

	uiwidgets.RenderControllerStage(f, grid.Stage3DArea, s, uiwidgets.StageCallbacks{
		OnSelectRegion: s.SelectRegion,
		OnSelectDevice: cb.OnSelectDevice,
		OnCycleDevice:  cb.OnCycleDevice,
		OnOpenSelector: func() { openSelector(s) },
	})

	uiwidgets.RenderBatteryStatusWidget(f, grid.BatteryStatusArea, s, cb.OnToggleConnect, cb.OnCycleTransport)
	if grid.DeviceSummaryArea.Height > 0 {
		uiwidgets.RenderDeviceSummaryWidget(f, grid.DeviceSummaryArea, s)
	}
	if grid.TelemetryArea.Height > 0 {
		if s.ActivePanel == state.PanelDiagnostics {
			renderHealthCard(f, grid.TelemetryArea, s)
		} else {
			renderSideCard(f, grid.TelemetryArea, s)
		}
	}

	RenderPanel(f, grid.InspectorArea, pc)
	uiwidgets.RenderHelpBar(f, grid.FooterArea, s)

	if s.ShowDeviceSelector {
		renderDeviceSelector(f, total, s, cb)
	}
	if s.ShowHelp {
		renderHelpModal(f, total, s)
	}

	if s.Toasts != nil {
		s.Toasts.Update(time.Now())
		s.Toasts.Draw(cell.NewContext(total, p.CanvasStyle()), f.Buffer)
	}
}

func openSelector(s *state.AppState) {
	s.ShowDeviceSelector = true
	s.SelectorIndex = 0
	for i, d := range s.Devices {
		if d.Key == s.ActiveDeviceKey {
			s.SelectorIndex = i
		}
	}
}

// renderHeader draws: brand · device selector pill (left) · EZ/Advanced switch (right).
func renderHeader(f *terminal.Frame, area cell.Rect, s *state.AppState, cb Callbacks) {
	if area.Width == 0 {
		return
	}
	p := s.Theme
	bar := cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}
	for x := area.X; x < area.X+area.Width; x++ {
		f.Buffer.SetCell(x, area.Y, cell.Cell{Content: ' ', Style: bar})
	}
	x := area.X + 1
	x += f.Buffer.SetStringWithin(x, area.Y, i18n.T(" TUISENSE "), cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated, Modifier: cell.ModifierBold}, area.Width-2)
	x++

	// Device selector pill
	accent := uiwidgets.AccentColor(s.Descriptor)
	pill := fmt.Sprintf(" ◆ %s ▾ ", s.Descriptor.ShortName)
	pw := uint16(cell.StringWidth(pill))
	if x+pw < area.X+area.Width {
		rect := cell.NewRect(x, area.Y, pw, 1)
		f.Buffer.SetStringWithin(x, area.Y, pill, cell.Style{Fg: p.BgCanvas, Bg: accent, Modifier: cell.ModifierBold}, pw)
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { openSelector(s) })
		f.RegisterAccessibility(accessibility.AccessibilityNode{
			ID: "device_selector", Role: accessibility.RoleButton, Label: i18n.T("Select controller"),
			Value: s.Descriptor.DisplayName, Bounds: rect,
		})
		x += pw + 1
		hint := i18n.Tf("%d device(s) · [C]", len(s.Devices))
		f.Buffer.SetStringWithin(x, area.Y, hint, cell.Style{Fg: p.FgMuted, Bg: p.BgElevated}, area.X+area.Width-x)
	}

	// Mode switch (right aligned)
	segs := []struct {
		id    string
		label string
		mode  state.UXMode
	}{
		{"mode_ez", i18n.T(" EZ "), state.ModeEZ},
		{"mode_advanced", i18n.T(" ADVANCED "), state.ModeAdvanced},
	}
	width := uint16(0)
	for _, sg := range segs {
		width += uint16(cell.StringWidth(sg.label))
	}
	mx := area.X + area.Width - width - 2
	if mx <= x+12 {
		return
	}
	for _, sg := range segs {
		w := uint16(cell.StringWidth(sg.label))
		st := cell.Style{Fg: p.FgMuted, Bg: p.BgSubtle}
		active := s.Mode == sg.mode
		if active {
			st = cell.Style{Fg: p.BgCanvas, Bg: p.FgSecondary, Modifier: cell.ModifierBold}
		}
		rect := cell.NewRect(mx, area.Y, w, 1)
		f.Buffer.SetStringWithin(mx, area.Y, sg.label, st, w)
		mode := sg.mode
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) {
			if cb.OnSetMode != nil {
				cb.OnSetMode(mode)
			} else {
				s.SetMode(mode)
			}
		})
		nodeState := accessibility.NodeState(0)
		if active {
			nodeState |= accessibility.StateSelected
		}
		f.RegisterAccessibility(accessibility.AccessibilityNode{
			ID: sg.id, Role: accessibility.RoleButton, Label: mode.Label() + i18n.T(" Mode"), State: nodeState, Bounds: rect,
		})
		mx += w
	}
}

// renderPanelTabs shows the capability-filtered panels for the active device and mode.
func renderPanelTabs(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width == 0 {
		return
	}
	p := s.Theme
	panels := s.VisiblePanels()
	build := func(withHotkeys bool) ([]string, int) {
		titles := make([]string, len(panels))
		width := 0
		for i, e := range panels {
			t := e.DisplayTitle(s.Mode)
			if withHotkeys && e.Hotkey != 0 {
				t = string(e.Hotkey) + " " + t
			}
			if e.Support == device.Limited {
				t += " ◐"
			}
			titles[i] = t
			width += cell.StringWidth(t) + 3 // padding + divider
		}
		return titles, width
	}
	titles, width := build(true)
	if width > int(area.Width)-2 {
		titles, _ = build(false) // drop "1 " prefixes before truncating titles
	}
	selected := -1
	for i, e := range panels {
		if e.Spec.ID == s.ActivePanel {
			selected = i
		}
	}
	divider := " "
	f.RenderWidget(widgets.Tabs{
		ID:            "top_nav_tabs",
		Titles:        titles,
		Selected:      selected,
		State:         s.NavTabsState,
		Divider:       &divider,
		Style:         cell.Style{Fg: p.FgMuted, Bg: p.BgCanvas},
		SelectedStyle: cell.Style{Fg: p.FgPrimary, Bg: p.BgSubtle, Modifier: cell.ModifierBold},
		DividerStyle:  cell.Style{Fg: p.BgSubtle, Bg: p.BgCanvas},
		OnSelect: func(idx int) {
			if idx >= 0 && idx < len(panels) {
				s.ActivePanel = panels[idx].Spec.ID
			}
		},
	}, cell.NewRect(area.X+1, area.Y, area.Width-2, 1))
}

// renderSideCard shows live input in Advanced mode and gentle guidance in EZ mode.
func renderSideCard(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width < 20 || area.Height < 4 {
		return
	}
	p := s.Theme
	title := i18n.T("Live Input")
	if s.Mode == state.ModeEZ {
		title = i18n.T("Getting Started")
	}
	f.RenderWidget(widgets.Block{
		Title: " " + title + " ", TitleStyle: p.CardTitle(),
		Borders: widgets.BorderAll, BorderSymbols: widgets.SymbolsRounded,
		BorderStyle: p.CardBorder(), Style: p.PanelStyle(),
	}, area)
	inner := cell.NewRect(area.X+2, area.Y+1, area.Width-4, area.Height-2)
	y := inner.Y

	if s.Mode == state.ModeEZ {
		tips := []string{
			i18n.T("Click a part of the"),
			i18n.T("controller to adjust it."),
			"",
			i18n.T("C  switch controller"),
			i18n.T("E  all settings"),
			i18n.T("?  keyboard help"),
		}
		for _, t := range tips {
			if y >= inner.Y+inner.Height {
				break
			}
			f.Buffer.SetStringWithin(inner.X, y, t, p.LabelStyle(), inner.Width)
			y++
		}
		return
	}

	tel := s.Telemetry
	labelW := uint16(9)
	gaugeW := inner.Width - labelW
	gauge := func(id, label string, ratio float64, col cell.Color) {
		if y >= inner.Y+inner.Height || gaugeW < 4 {
			return
		}
		f.Buffer.SetStringWithin(inner.X, y, label, p.MutedStyle(), labelW)
		f.RenderWidget(widgets.LineGauge{
			ID: id, Ratio: ratio, HideLabel: true,
			FilledStyle:   cell.Style{Fg: col, Bg: p.BgPanel},
			UnfilledStyle: cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
		}, cell.NewRect(inner.X+labelW, y, gaugeW, 1))
		y++
	}
	axis := func(v float64) float64 { return (v + 1) / 2 }
	accent := uiwidgets.AccentColor(s.Descriptor)
	gauge("live_lt", i18n.T("L Trig"), tel.L2Pressure, accent)
	gauge("live_rt", i18n.T("R Trig"), tel.R2Pressure, accent)
	if s.Caps.Supports(device.CapSticks) {
		gauge("live_lx", i18n.T("L Stick"), axis(tel.LeftStickX), p.AccentBlue)
		gauge("live_rx", i18n.T("R Stick"), axis(tel.RightStickX), p.AccentBlue)
	}
	if s.Caps.Supports(device.CapMotion) && y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("Gyro  %+5.1f° %+5.1f° %+5.1f°", tel.GyroPitch, tel.GyroYaw, tel.GyroRoll), p.MutedStyle(), inner.Width)
		y++
	}
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("Packets  Rx %d · Tx %d", tel.PacketsRx, tel.PacketsTx), p.MutedStyle(), inner.Width)
	}
}

// renderOverviewInspector (Advanced) summarizes every supported capability as
// a clickable card row, generated from the capability set.
func renderOverviewInspector(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	p := s.Theme
	inner := panelFrame(f, area, s, i18n.T("Overview"))
	y := inner.Y

	if !s.DeviceInfo.Connected {
		f.Buffer.SetStringWithin(inner.X, y, i18n.T("⚠ No controller connected"), cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold}, inner.Width)
		y += 2
		pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{
			{ID: "btn_overview_reconnect", Label: i18n.T("Use simulated controller [D]"), OnSelect: pc.CB.OnToggleConnect},
		})
		return
	}
	if s.RegionHint != "" {
		f.Buffer.SetStringWithin(inner.X, y, "ℹ "+s.RegionHint, cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel}, inner.Width)
		y++
	}

	cfg := s.Config
	type card struct {
		capability device.Capability
		value      string
	}
	cards := []card{
		{device.CapAdaptiveTriggers, fmt.Sprintf("L: %s (%d) · R: %s (%d)", shortTriggerModeName(cfg.LeftTrigger.Mode), cfg.LeftTrigger.Strength, shortTriggerModeName(cfg.RightTrigger.Mode), cfg.RightTrigger.Strength)},
		{device.CapLighting, lightingSummary(s)},
		{device.CapHaptics, i18n.Tf("Master %d%% · L/R %d/%d", cfg.Haptics.MasterIntensity, cfg.Haptics.LeftMotor, cfg.Haptics.RightMotor)},
		{device.CapSticks, i18n.Tf("L DZ %d%% (%s) · R DZ %d%%", cfg.Sticks.Left.InnerDeadzone, i18n.T(string(cfg.Sticks.Left.Curve)), cfg.Sticks.Right.InnerDeadzone)},
		{device.CapMotion, i18n.Tf("Gyro %s · %d%%", onOffStr(cfg.Motion.GyroEnabled), cfg.Motion.GyroSensitivity)},
		{device.CapAudio, i18n.Tf("Speaker %d%% · Headset %d%% · Mic %d%%", cfg.Audio.SpeakerVolume, cfg.Audio.HeadphoneVolume, cfg.Audio.MicGain)},
		{device.CapProfiles, i18n.Tf("Active: %s", s.ActiveProfile)},
	}
	titleW := uint16(20)
	visible := s.VisiblePanels()
	for _, c := range cards {
		if y >= inner.Y+inner.Height {
			break
		}
		level := s.Caps.Level(c.capability)
		if level == device.Unsupported {
			continue
		}
		label := "▸ " + c.capability.Label()
		if level == device.Limited {
			label += " ◐"
		}
		f.Buffer.SetStringWithin(inner.X, y, label, cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold}, min(titleW, inner.Width))
		if inner.Width > titleW+1 {
			f.Buffer.SetStringWithin(inner.X+titleW+1, y, c.value, p.LabelStyle(), inner.Width-titleW-1)
		}
		if panel, ok := state.PanelForCapability(c.capability); ok && state.PanelVisible(visible, panel) {
			f.RegisterClickHandler(cell.NewRect(inner.X, y, inner.Width, 1), func(_ driver.MouseEvent) {
				s.ActivePanel = panel
			})
		}
		y++
	}
	y++

	if y < inner.Y+inner.Height && len(s.Descriptor.Actions) > 0 {
		var items []choice
		for _, a := range s.Descriptor.Actions {
			if !s.Caps.Supports(a.Capability) {
				continue
			}
			cmd, label := a.Command, a.Label
			items = append(items, choice{ID: "action_" + a.ID, Label: "⚡ " + i18n.T(label), OnSelect: func() {
				pc.dispatch(cmd)
				if s.Toasts != nil {
					s.Toasts.Info(label, s.Descriptor.ShortName)
				}
			}})
		}
		pillRow(f, inner.X, y, inner.X+inner.Width, s, i18n.T("Actions:"), 10, items)
	}
}

func lightingSummary(s *state.AppState) string {
	l := s.Config.Lighting
	if !s.Caps.HasFeature(device.CapLighting, device.FeatLightbarRGB) {
		return i18n.Tf("LED %s · %s", onOffStr(l.Enabled), i18n.T(l.Brightness.String()))
	}
	return fmt.Sprintf("#%02X%02X%02X · %s · %s", l.Red, l.Green, l.Blue, i18n.T(l.Brightness.String()), i18n.T(string(l.Animation)))
}

func onOffStr(b bool) string {
	if b {
		return i18n.T("on")
	}
	return i18n.T("off")
}

// renderDeviceSelector is the modal listing every attached device.
func renderDeviceSelector(f *terminal.Frame, total cell.Rect, s *state.AppState, cb Callbacks) {
	p := s.Theme
	mw := min(total.Width-6, uint16(64))
	mh := min(total.Height-4, uint16(len(s.Devices)*2+6))
	if mw < 30 || mh < 7 {
		return
	}
	mx := total.X + (total.Width-mw)/2
	my := total.Y + (total.Height-mh)/2
	area := cell.NewRect(mx, my, mw, mh)

	f.RegisterModal("device_selector_modal", area, func() { s.ShowDeviceSelector = false })
	f.BeginLayer("device_selector_modal")
	defer f.EndLayer()

	widgets.DrawShadow(f.Buffer, area, 2, 1)
	f.RenderWidget(widgets.Block{
		Title: i18n.T(" Choose a controller "), TitleStyle: cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated, Modifier: cell.ModifierBold},
		Borders: widgets.BorderAll, BorderSymbols: widgets.SymbolsRounded,
		BorderStyle: cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated},
		Style:       cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated},
	}, area)

	y := my + 2
	for i, d := range s.Devices {
		if y >= my+mh-2 {
			break
		}
		rect := cell.NewRect(mx+2, y, mw-4, 1)
		st := cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated}
		if i == s.SelectorIndex {
			st = cell.Style{Fg: p.BgCanvas, Bg: uiwidgets.AccentColor(d.Descriptor), Modifier: cell.ModifierBold}
		}
		for x := rect.X; x < rect.X+rect.Width; x++ {
			f.Buffer.SetCell(x, y, cell.Cell{Content: ' ', Style: st})
		}
		mark := "  "
		if d.Key == s.ActiveDeviceKey {
			mark = "◆ "
		}
		status := i18n.T("connected")
		if !d.Connected {
			status = i18n.T("offline")
		}
		line := fmt.Sprintf(" %s%-34s %-9s %s", mark, i18n.T(d.Descriptor.DisplayName), i18n.T(string(d.Source)), status)
		f.Buffer.SetStringWithin(rect.X, y, line, st, rect.Width)
		key := d.Key
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) {
			s.ShowDeviceSelector = false
			if cb.OnSelectDevice != nil {
				cb.OnSelectDevice(key)
			}
		})
		f.RegisterAccessibility(accessibility.AccessibilityNode{
			ID: fmt.Sprintf("selector_item_%d", i), Role: accessibility.RoleButton,
			Label: d.Descriptor.DisplayName, Value: string(d.Source), Bounds: rect,
		})
		y += 2
	}
	f.Buffer.SetStringWithin(mx+3, my+mh-2, i18n.T("↑/↓ choose · Enter select · Esc close"), cell.Style{Fg: p.FgMuted, Bg: p.BgElevated}, mw-6)
}

func renderHelpModal(f *terminal.Frame, total cell.Rect, s *state.AppState) {
	p := s.Theme
	mw := min(total.Width-6, uint16(68))
	mh := min(total.Height-4, uint16(21))
	if mw < 30 || mh < 10 {
		return
	}
	mx := total.X + (total.Width-mw)/2
	my := total.Y + (total.Height-mh)/2
	modalArea := cell.NewRect(mx, my, mw, mh)

	f.RegisterModal("help_modal", modalArea, func() { s.ShowHelp = false })
	f.BeginLayer("help_modal")
	defer f.EndLayer()

	widgets.DrawShadow(f.Buffer, modalArea, 2, 1)
	f.RenderWidget(widgets.Block{
		Title: i18n.T(" Keyboard & Mouse "), TitleStyle: cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated, Modifier: cell.ModifierBold},
		Borders: widgets.BorderAll, BorderSymbols: widgets.SymbolsRounded,
		BorderStyle: cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated},
		Style:       cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated},
	}, modalArea)

	lines := []struct{ k, v string }{
		{i18n.T("Click region / badge"), i18n.T("Select a controller part & open its settings")},
		{i18n.T("Drag / Scroll on stage"), i18n.T("Orbit / zoom the controller")},
		{"C", i18n.T("Open the controller selector")},
		{", / .", i18n.T("Previous / next controller")},
		{"E", i18n.T("Toggle EZ ↔ Advanced mode")},
		{"1..9, 0", i18n.T("Jump to a panel (numbers follow the tabs)")},
		{"[ / ]", i18n.T("Cycle controller regions")},
		{i18n.T("Tab / Shift+Tab"), i18n.T("Move focus between controls")},
		{i18n.T("Arrows / Enter"), i18n.T("Adjust sliders, lists, color picker")},
		{"W A S D", i18n.T("Orbit with the keyboard")},
		{i18n.T("Space / M / R"), i18n.T("Auto-orbit / shading / reset view")},
		{"T", i18n.T("Test vibration")},
		{i18n.T("Diagnostics: h / l"), i18n.T("Previous / next test category")},
		{i18n.T("Diagnostics: o / g / n / x"), i18n.T("Start test / guided check / next step / stop all")},
		{"D / B / U", i18n.T("Simulate hot-plug / transport / permission")},
		{i18n.T("Esc / ? / Q"), i18n.T("Close · Help · Quit")},
	}
	y := my + 2
	for _, ln := range lines {
		if y >= my+mh-1 {
			break
		}
		f.Buffer.SetStringWithin(mx+3, y, fmt.Sprintf("%-23s", ln.k), cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated, Modifier: cell.ModifierBold}, 23)
		f.Buffer.SetStringWithin(mx+27, y, ln.v, cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}, mw-29)
		y++
	}
}
