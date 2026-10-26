package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
)

// renderHome exposes only telemetry and intentional entry points.
func renderHome(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	if area.Width < 20 || area.Height < 4 {
		return
	}
	status := i18n.T(string(s.DeviceInfo.Connection))
	if !s.DeviceInfo.Connected {
		status = i18n.T("Disconnected")
	} else if s.Caps.Supports(device.CapBattery) && s.Telemetry.BatteryPercent >= 0 {
		status += fmt.Sprintf(" · %s %d%%", i18n.T("Battery"), s.Telemetry.BatteryPercent)
		if s.Telemetry.Charging {
			status += " · " + i18n.T("Charging")
		} else if s.Telemetry.FullyCharged {
			status += " · " + i18n.T("Full")
		} else {
			status += " · " + i18n.T("On battery")
		}
	}
	if s.Caps.Supports(device.CapProfiles) {
		status += " · " + i18n.T("Profile") + ": " + s.ActiveProfile
	}
	w := min(area.Width, uint16(cell.StringWidth(status)))
	f.Buffer.SetStringWithin(area.X+(area.Width-w)/2, area.Y, status, s.Theme.MutedStyle(), w)
	var items []choice
	for _, id := range []state.PanelID{state.PanelDiagnostics, state.PanelCustomize, state.PanelFirmware} {
		if !state.PanelVisible(s.VisiblePanels(), id) {
			continue
		}
		title := map[state.PanelID]string{state.PanelDiagnostics: "Test Controller", state.PanelCustomize: "Customize", state.PanelFirmware: "Device Details"}[id]
		target := id
		items = append(items, choice{ID: "home_" + string(id), Label: i18n.T(title), OnSelect: func() { s.ActivePanel = target }})
	}
	width := uint16(0)
	for _, it := range items {
		width += uint16(cell.StringWidth(it.Label) + 3)
	}
	x := area.X
	if width < area.Width {
		x += (area.Width - width) / 2
	}
	pillRow(f, x, area.Y+2, area.X+area.Width, s, "", 0, items)
	if s.RegionHint != "" && area.Height > 4 {
		f.Buffer.SetStringWithin(area.X, area.Y+4, s.RegionHint, s.Theme.MutedStyle(), area.Width)
	}
}

// renderSubnav wraps secondary destinations instead of truncating access.
func renderSubnav(f *terminal.Frame, area cell.Rect, s *state.AppState) cell.Rect {
	group := state.PrimaryPanel(s.ActivePanel)
	if group != state.PanelCustomize && group != state.PanelSettings {
		return area
	}
	x, y := area.X+2, area.Y
	for _, e := range s.VisiblePanels() {
		if state.PrimaryPanel(e.Spec.ID) != group || e.Spec.ID == state.PanelCustomize {
			continue
		}
		label := e.DisplayTitle(s.Mode)
		w := uint16(cell.StringWidth(label) + 3)
		if x+w > area.X+area.Width-2 {
			x = area.X + 2
			y++
		}
		if y >= area.Y+area.Height {
			break
		}
		target := e.Spec.ID
		x = pillRow(f, x, y, area.X+area.Width-2, s, "", 0, []choice{{ID: "nav_" + string(target), Label: label, Selected: target == s.ActivePanel, OnSelect: func() { s.ActivePanel = target }}})
	}
	used := min(area.Height, y-area.Y+2)
	return cell.NewRect(area.X, area.Y+used, area.Width, area.Height-used)
}

func renderCustomize(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	inner := panelFrame(f, area, s, i18n.T("Customize"))
	y := inner.Y
	for _, r := range s.Descriptor.Regions {
		panel, ok := state.PanelForCapability(r.Capability)
		if !ok || !state.PanelVisible(s.VisiblePanels(), panel) || state.PrimaryPanel(panel) != state.PanelCustomize {
			continue
		}
		if y >= inner.Y+inner.Height {
			break
		}
		region := r.ID
		pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{{ID: "region_" + string(r.ID), Label: i18n.T(r.Label), OnSelect: func() { s.SelectRegion(region) }}})
		y++
	}
}
