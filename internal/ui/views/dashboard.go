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

	renderHeader(f, grid.HeaderArea, s, cb)
	if s.Gallery.Active {
		renderGallery(f, grid, s, cb)
	} else {
		renderPanelTabs(f, grid.TabsArea, s)
		renderBody(f, total, grid, pc)
	}

	uiwidgets.RenderHelpBar(f, grid.FooterArea, s)

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
	if s.Toasts != nil {
		s.Toasts.Update(time.Now())
		s.Toasts.Draw(cell.NewContext(total, p.CanvasStyle()), f.Buffer)
	}
}

// renderBody draws the controller stage and the active panel.
func renderBody(f *terminal.Frame, total cell.Rect, grid ui.DashboardLayout, pc PanelContext) {
	s := pc.S
	stageCB := uiwidgets.StageCallbacks{OnSelectComponent: s.SelectComponent}
	if s.ActivePanel == state.PanelOverview || s.ActivePanel == state.PanelDiagnostics {
		uiwidgets.RenderControllerStage(f, grid.Stage3DArea, s, stageCB)
		if s.ActivePanel == state.PanelDiagnostics {
			RenderPanel(f, grid.InspectorArea, pc)
		} else if s.ActiveDeviceKey != "" {
			renderHome(f, grid.InspectorArea, pc)
		}
		return
	}
	area := cell.NewRect(total.X, total.Y+3, total.Width, total.Height-4)
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
	bar := cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}
	for x := area.X; x < area.X+area.Width; x++ {
		f.Buffer.SetCell(x, area.Y, cell.Cell{Content: ' ', Style: bar})
	}
	x := area.X + 1
	x += f.Buffer.SetStringWithin(x, area.Y, i18n.T(" TUISENSE "), cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated, Modifier: cell.ModifierBold}, area.Width-2)
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
		f.Buffer.SetStringWithin(x, area.Y, pill, cell.Style{Fg: accent, Bg: p.BgElevated, Modifier: cell.ModifierBold}, pw)
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
	if badge := updateBadge(s.Update); badge != "" {
		label = badge + "  " + ver
		if area.X+area.Width < x+width+uint16(cell.StringWidth(label))+6 {
			label = ver // too narrow: keep the mode switch visible
		}
	}
	vw := uint16(cell.StringWidth(label))
	if area.Width > vw+2 {
		vx := area.X + area.Width - vw - 1
		st := cell.Style{Fg: p.FgMuted, Bg: p.BgElevated}
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

// renderPanelTabs shows the capability-filtered panels for the active device and mode.
func renderPanelTabs(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width == 0 {
		return
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
		if e.Spec.ID == state.PrimaryPanel(s.ActivePanel) {
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
	yes, no := " "+i18n.T("Yes")+" ", " "+i18n.T("No")+" "
	title := i18n.T("Quit TuiSense?")
	hint := i18n.T("Enter: quit · Esc: stay")
	w := uint16(max(cell.StringWidth(title), cell.StringWidth(hint), cell.StringWidth(yes+"  "+no)) + 6)
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

	bx := center(yes + "  " + no)
	buttons := []struct {
		id, label string
		x         uint16
		selected  bool
		onClick   func()
	}{
		{"quit_yes", yes, bx, true, func() { s.QuitRequested = true }},
		{"quit_no", no, bx + uint16(cell.StringWidth(yes)) + 2, false, func() { s.ShowQuitConfirm = false }},
	}
	for _, b := range buttons {
		bw := uint16(cell.StringWidth(b.label))
		rect := cell.NewRect(b.x, area.Y+3, bw, 1)
		f.Buffer.SetStringWithin(b.x, area.Y+3, b.label, p.ButtonStyle(b.selected), bw)
		onClick := b.onClick
		f.RegisterClickHandler(rect, func(_ driver.MouseEvent) { onClick() })
		f.RegisterAccessibility(accessibility.AccessibilityNode{ID: b.id, Role: accessibility.RoleButton, Label: strings.TrimSpace(b.label), Bounds: rect})
	}
	f.Buffer.SetStringWithin(center(hint), area.Y+5, hint, cell.Style{Fg: p.FgMuted, Bg: p.BgElevated}, w-2)
	f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "quit_confirm", Role: accessibility.RoleGeneric, Label: title, Bounds: area})
}
