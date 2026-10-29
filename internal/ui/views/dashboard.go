package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/ui"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
	"github.com/Phobby/TuiSense/internal/update"
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
	if total.Width < 20 || total.Height < 10 {
		return
	}
	s.Copy.Begin(CopyScope(s))
	s.Copy.Disabled = s.ShowHelp || s.ShowDeviceSelector || s.ShowCopyFallback || s.ShowQuitConfirm
	p := s.Theme
	f.SetTheme(p.LimoniTheme())

	bgStyle := p.CanvasStyle()
	for y := total.Y; y < total.Y+total.Height; y++ {
		for x := total.X; x < total.X+total.Width; x++ {
			f.Buffer.SetCell(x, y, cell.Cell{Content: ' ', Style: bgStyle})
		}
	}

	grid := ui.ComputeLayout(total, s.ActivePanel == state.PanelDiagnostics && !s.Gallery.Active)
	pc := PanelContext{S: s, Dispatch: dispatch, CB: cb}

	if s.Gallery.Active {
		renderHeader(f, grid.HeaderArea, s, cb)
		renderGallery(f, grid, s, cb)
	} else {
		renderHeader(f, grid.HeaderArea, s, cb)
		renderPanelTabs(f, grid.TabsArea, s)
		renderBody(f, total, grid, pc)
	}

	uiwidgets.RenderHelpBar(f, grid.FooterArea, s)
	// Under an open dialog only the dialog's own controls take hover.
	firstControl := 0
	if s.ShowDeviceSelector || s.ShowHelp || s.ShowCopyFallback || s.ShowQuitConfirm {
		firstControl = len(f.AccessibilityTree())
	}

	if s.ShowDeviceSelector {
		renderDeviceSelector(f, total, s, cb)
	}
	if s.ShowHelp {
		renderHelpModal(f, total, s)
	}

	if s.ShowCopyFallback {
		renderCopyFallback(f, total, s)
	}
	if s.ShowQuitConfirm {
		renderQuitConfirm(f, total, s)
	}
	renderHover(f, s, firstControl, time.Now())
	if s.Toasts != nil {
		s.Toasts.Update(time.Now())
		s.Toasts.Draw(cell.NewContext(total, p.CanvasStyle()), f.Buffer)
	}
}

// renderBody draws the controller stage and the active panel.
func renderBody(f *terminal.Frame, total cell.Rect, grid ui.DashboardLayout, pc PanelContext) {
	s := pc.S
	stageCB := uiwidgets.StageCallbacks{OnSelectComponent: s.SelectComponent}
	if s.ActivePanel == state.PanelDiagnostics && total.Width >= 140 {
		// Wide terminals: controller beside the results, as in Customize.
		body := cell.NewRect(total.X, grid.Stage3DArea.Y, total.Width, grid.FooterArea.Y-grid.Stage3DArea.Y)
		stage := cell.NewRect(body.X, body.Y, body.Width-96, body.Height)
		uiwidgets.RenderControllerStage(f, stage, s, stageCB)
		RenderPanel(f, cell.NewRect(stage.X+stage.Width, body.Y, 96, body.Height), pc)
		return
	}
	if s.ActivePanel == state.PanelOverview || s.ActivePanel == state.PanelDiagnostics {
		uiwidgets.RenderControllerStage(f, grid.Stage3DArea, s, stageCB)
		if s.ActivePanel == state.PanelDiagnostics {
			RenderPanel(f, grid.InspectorArea, pc)
		} else if s.ActiveDeviceKey != "" {
			renderHome(f, grid.InspectorArea, pc)
		}
		return
	}
	area := cell.NewRect(total.X, grid.Stage3DArea.Y, total.Width, grid.FooterArea.Y-grid.Stage3DArea.Y)
	area = renderSubnav(f, area, s)
	if total.Width >= 140 && state.PrimaryPanel(s.ActivePanel) == state.PanelCustomize && s.ActivePanel != state.PanelCustomize {
		stage := cell.NewRect(area.X, area.Y, area.Width-72, area.Height)
		uiwidgets.RenderControllerStage(f, stage, s, stageCB)
		area = cell.NewRect(stage.X+stage.Width, area.Y, 72, area.Height)
	}
	RenderPanel(f, area, pc)
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
	x := area.X + 1
	x += f.Buffer.SetStringWithin(x, area.Y, i18n.T(" TUISENSE "), cell.Style{Fg: p.FgPrimary, Bg: p.BgCanvas, Modifier: cell.ModifierBold}, area.Width-2)
	x++

	// Device selector pill
	accent := p.FgSecondary
	pill := " " + s.Descriptor.ShortName + " "
	if s.ActiveDeviceKey == "" {
		pill = ""
	}
	if s.IsPreviewDevice() {
		pill += i18n.T(" PREVIEW ")
		accent = p.AccentBlue
	} else if s.IsSimulatedDevice() {
		pill += i18n.T(" MOCK ")
	}
	if len(s.Devices) > 1 {
		pill += " ▾"
	}
	if s.Gallery.Active {
		// The connected device stays active underneath; the header says so.
		pill = " ◇ " + i18n.T("Controller Gallery") + " "
		accent = p.AccentBlue
	}
	pw := uint16(cell.StringWidth(pill))
	if x+pw < area.X+area.Width {
		rect := cell.NewRect(x, area.Y, pw, 1)
		f.Buffer.SetStringWithin(x, area.Y, pill, cell.Style{Fg: accent, Bg: p.BgCanvas, Modifier: cell.ModifierBold}, pw)
		if len(s.Devices) > 1 && !s.Gallery.Active {
			f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { openSelector(s) })
		}
		f.RegisterAccessibility(accessibility.AccessibilityNode{
			ID: "device_selector", Role: accessibility.RoleButton, Label: i18n.T("Select controller"),
			Value: s.Descriptor.DisplayName, Bounds: rect,
		})
		x += pw + 1
	}
	// A settings preview says so and offers the way back.
	if s.IsPreviewDevice() && !s.Gallery.Active && cb.OnExitPreview != nil {
		exit := " ✕ " + i18n.T("Exit preview") + " "
		ew := uint16(cell.StringWidth(exit))
		if x+ew < area.X+area.Width {
			rect := cell.NewRect(x, area.Y, ew, 1)
			f.Buffer.SetStringWithin(x, area.Y, exit, cell.Style{Fg: p.FgPrimary, Bg: p.BgSubtle}, ew)
			f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { cb.OnExitPreview() })
			f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "exit_preview", Role: accessibility.RoleButton, Label: i18n.T("Exit preview"), Bounds: rect})
			x += ew + 1
		}
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
	// The installed version sits in the top-right corner, small and muted,
	// with the application updater's state beside it when there is room.
	// Clicking it opens Settings, where the update details live.
	ver := displayVersion(s.AppVersion)
	label := ver
	if s.WebDemo {
		// The browser demo says so quietly where the update state would be.
		for _, l := range []string{i18n.T("WEB DEMO — SIMULATED CONTROLLERS") + " · " + ver, i18n.T("WEB DEMO") + " · " + ver} {
			label = l
			if area.X+area.Width >= x+width+uint16(cell.StringWidth(label))+6 {
				break
			}
		}
	} else if badge := updateBadge(s.Update); badge != "" {
		label = badge + "  " + ver
		if area.X+area.Width < x+width+uint16(cell.StringWidth(label))+6 {
			label = ver // too narrow: keep the mode switch visible
		}
	}
	vw := uint16(cell.StringWidth(label))
	if area.Width > vw+2 {
		vx := area.X + area.Width - vw - 1
		st := cell.Style{Fg: p.FgMuted, Bg: p.BgCanvas}
		if s.Update.Status == update.StatusAvailable || s.Update.Status == update.StatusReady {
			st.Fg = p.AccentBlue
		}
		f.Buffer.SetStringWithin(vx, area.Y, label, st, vw)
		rect := cell.NewRect(vx, area.Y, vw, 1)
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { s.ActivePanel = state.PanelSettings })
		f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "app_version", Role: accessibility.RoleButton, Label: "TuiSense " + ver,
			Value: updateStatusText(s.Update), Bounds: rect})
	}
	mx := area.X + area.Width - width - vw - 3
	if mx <= x+1 || mx > area.X+area.Width {
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

// renderPanelTabs draws the capability-filtered pages as folder tabs: the
// selected page is framed, its top edge on the row above area, and opens into
// a line drawn under all of them. It returns the x after the last tab.
func renderPanelTabs(f *terminal.Frame, area cell.Rect, s *state.AppState) uint16 {
	if area.Width == 0 || area.Y == 0 {
		return area.X
	}
	p := s.Theme
	panels := s.PrimaryPanels()
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
			width += cell.StringWidth(t) + 4 // padding + the frame's two edges
		}
		return titles, width
	}
	titles, width := build(true)
	if width > int(area.Width)-2 {
		titles, _ = build(false) // drop "1 " prefixes before truncating titles
	}
	base := area.Y + 1
	line := cell.Style{Fg: p.BgSubtle, Bg: p.BgCanvas}
	edge := cell.Style{Fg: p.AccentCyan, Bg: p.BgCanvas}
	f.Buffer.SetStringWithin(area.X, base, strings.Repeat("─", int(area.Width)), line, area.Width)
	x := area.X + 1
	for i, t := range titles {
		lbl := " " + t + " "
		w := uint16(cell.StringWidth(lbl))
		if x+w+2 > area.X+area.Width {
			break
		}
		target := panels[i].Spec.ID
		selected := target == state.PrimaryPanel(s.ActivePanel)
		bar := strings.Repeat("─", int(w))
		if selected {
			f.Buffer.SetStringWithin(x, area.Y-1, "╭"+bar+"╮", edge, w+2)
			f.Buffer.SetStringWithin(x, area.Y, "│", edge, 1)
			f.Buffer.SetStringWithin(x+1, area.Y, lbl, cell.Style{Fg: p.FgPrimary, Bg: p.BgCanvas, Modifier: cell.ModifierBold}, w)
			f.Buffer.SetStringWithin(x+w+1, area.Y, "│", edge, 1)
			f.Buffer.SetStringWithin(x, base, "╯"+strings.Repeat(" ", int(w))+"╰", edge, w+2)
		} else {
			f.Buffer.SetStringWithin(x+1, area.Y, lbl, cell.Style{Fg: p.FgMuted, Bg: p.BgCanvas}, w)
		}
		rect := cell.NewRect(x, area.Y, w+2, 1)
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { s.ActivePanel = target })
		st := accessibility.NodeState(0)
		if selected {
			st |= accessibility.StateSelected
		}
		f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "page_" + string(target), Role: accessibility.RoleTab, Label: t, State: st, Bounds: rect})
		x += w + 2
	}
	return x
}

func renderDeviceSelector(f *terminal.Frame, total cell.Rect, s *state.AppState, cb Callbacks) {
	if total.Width < 36 || total.Height < 11 {
		return
	}
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
	start := max(0, s.SelectorIndex-int((mh-4)/2)+1)
	for i, d := range s.Devices {
		if i < start {
			continue
		}
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
		name := i18n.T(d.Descriptor.ShortName)
		ordinal, count := 0, 0
		for j, other := range s.Devices {
			if other.Descriptor.ModelID == d.Descriptor.ModelID {
				count++
				if j <= i {
					ordinal++
				}
			}
		}
		if count > 1 {
			name += fmt.Sprintf(" #%d", ordinal)
		}
		source := ""
		if d.Source != device.SourceHardware {
			source = " · " + i18n.T(string(d.Source))
		}
		line := fmt.Sprintf("%s%s%s · %s", mark, name, source, status)
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

// renderQuitConfirm draws the small "Quit TuiSense?" dialog that Esc opens on
// the main screen.
func renderQuitConfirm(f *terminal.Frame, total cell.Rect, s *state.AppState) {
	p := s.Theme
	yes, no := i18n.T("Yes"), i18n.T("No")
	title := i18n.T("Quit TuiSense?")
	hint := i18n.T("Enter: quit · Esc: stay")
	w := uint16(max(cell.StringWidth(title), cell.StringWidth(hint), int(pillWidth(choice{Label: yes})+1+pillWidth(choice{Label: no}))) + 6)
	if total.Width < w+2 || total.Height < 9 {
		return
	}
	area := cell.NewRect(total.X+(total.Width-w)/2, total.Y+(total.Height-7)/2, w, 7)
	f.RegisterModal("quit_confirm", area, func() { s.ShowQuitConfirm = false })
	f.BeginLayer("quit_confirm")
	defer f.EndLayer()

	widgets.DrawShadow(f.Buffer, area, 2, 1)
	// Same frame as the controller selector, with a clearly visible border.
	f.RenderWidget(widgets.Block{
		Title: " TuiSense ", TitleStyle: cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated, Modifier: cell.ModifierBold},
		Borders: widgets.BorderAll, BorderSymbols: widgets.SymbolsRounded,
		BorderStyle: cell.Style{Fg: p.AccentBlue, Bg: p.BgElevated, Modifier: cell.ModifierBold},
		Style:       cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated},
	}, area)
	center := func(text string) uint16 { return area.X + (w-uint16(cell.StringWidth(text)))/2 }
	f.Buffer.SetStringWithin(center(title), area.Y+1, title, cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated, Modifier: cell.ModifierBold}, w-2)

	bw := pillWidth(choice{Label: yes}) + 1 + pillWidth(choice{Label: no})
	pillRow(f, area.X+(w-bw)/2, area.Y+3, area.X+w, s, "", 0, []choice{
		{ID: "quit_yes", Label: yes, Selected: true, OnSelect: func() { s.QuitRequested = true }},
		{ID: "quit_no", Label: no, OnSelect: func() { s.ShowQuitConfirm = false }},
	})
	f.Buffer.SetStringWithin(center(hint), area.Y+5, hint, cell.Style{Fg: p.FgMuted, Bg: p.BgElevated}, w-2)
	f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "quit_confirm", Role: accessibility.RoleGeneric, Label: title, Bounds: area})
}
