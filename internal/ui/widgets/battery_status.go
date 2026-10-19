package widgets

import (
	"fmt"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// RenderBatteryStatusWidget renders the top-right Battery, Charging state,
// Transport (USB-C / Bluetooth), and Connected status widget.
func RenderBatteryStatusWidget(f *terminal.Frame, area cell.Rect, s *state.AppState, onToggleConnect func(), onCycleTransport func()) {
	if area.Width < 20 || area.Height < 4 {
		return
	}
	p := s.Theme
	info := s.DeviceInfo
	tel := s.Telemetry

	borderStyle := p.CardBorder()
	if !info.Connected {
		borderStyle = cell.Style{Fg: p.Danger, Bg: p.BgPanel}
	} else if info.PermissionDenied {
		borderStyle = cell.Style{Fg: p.Warning, Bg: p.BgPanel}
	}

	block := widgets.Block{
		Title:         " Status ",
		TitleStyle:    p.CardTitle(),
		Borders:       widgets.BorderAll,
		BorderSymbols: widgets.SymbolsRounded,
		BorderStyle:   borderStyle,
		Style:         p.PanelStyle(),
	}
	f.RenderWidget(block, area)

	inner := cell.NewRect(area.X+2, area.Y+1, area.Width-4, area.Height-2)
	if inner.Width < 12 || inner.Height < 2 {
		return
	}

	// Row 0: Connection Status Badge + Transport Badge
	connText := " ● CONNECTED "
	connStyle := p.BadgeStyle(p.BgCanvas, p.Success)
	if !info.Connected {
		connText = " ○ DISCONNECTED "
		connStyle = p.BadgeStyle(p.FgPrimary, p.Danger)
	}
	connW := uint16(cell.StringWidth(connText))
	connRect := cell.NewRect(inner.X, inner.Y, min(connW, inner.Width), 1)
	f.Buffer.SetStringWithin(connRect.X, connRect.Y, connText, connStyle, connRect.Width)
	if onToggleConnect != nil {
		f.RegisterClickHandler(connRect, func(_ driver.MouseEvent) {
			onToggleConnect()
		})
	}

	transIcon := "⌁"
	if info.Connection == device.ConnBluetooth {
		transIcon = "ᛒ"
	}
	transText := fmt.Sprintf(" %s %s ", transIcon, info.Connection)
	if !info.Connected {
		transText = " ── Offline ── "
	}
	transW := uint16(cell.StringWidth(transText))
	if connRect.X+connRect.Width+1+transW <= inner.X+inner.Width {
		tx := connRect.X + connRect.Width + 1
		tRect := cell.NewRect(tx, inner.Y, transW, 1)
		f.Buffer.SetStringWithin(tx, inner.Y, transText, cell.Style{Fg: p.FgSecondary, Bg: p.BgElevated}, transW)
		if onCycleTransport != nil {
			f.RegisterClickHandler(tRect, func(_ driver.MouseEvent) {
				onCycleTransport()
			})
		}
	}

	if inner.Height < 2 {
		return
	}

	// Row 1: Battery Gauge + Charging State
	batY := inner.Y + 1
	hasBattery := s.Caps.Supports(device.CapBattery)
	batRatio := float64(tel.BatteryPercent) / 100.0
	if !info.Connected || !hasBattery {
		batRatio = 0
	}
	batColor := p.Success
	if tel.BatteryPercent <= 20 {
		batColor = p.Danger
	} else if tel.BatteryPercent <= 45 {
		batColor = p.Warning
	}

	chargeTag := "On battery"
	chargeColor := p.FgSecondary
	if !info.Connected || !hasBattery {
		chargeTag = "No battery telemetry"
		chargeColor = p.FgMuted
	} else if tel.FullyCharged {
		chargeTag = "↯ Full"
		chargeColor = p.Success
	} else if tel.Charging {
		chargeTag = "↯ Charging"
		chargeColor = p.Warning
	}

	gaugeW := uint16(18)
	if gaugeW > inner.Width/2 {
		gaugeW = inner.Width / 2
	}
	if inner.Width < 30 {
		gaugeW = 10
	}
	batLabel := fmt.Sprintf("%d%%", tel.BatteryPercent)
	if !info.Connected || !hasBattery {
		batLabel = "--%"
	}
	gauge := widgets.Gauge{
		ID:         "battery_gauge",
		Ratio:      batRatio,
		Label:      batLabel,
		Style:      cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated},
		GaugeStyle: cell.Style{Fg: batColor, Bg: p.BgElevated},
	}
	f.RenderWidget(gauge, cell.NewRect(inner.X, batY, gaugeW, 1))

	statusX := inner.X + gaugeW + 2
	if statusX < inner.X+inner.Width {
		f.Buffer.SetStringWithin(statusX, batY, chargeTag, cell.Style{Fg: chargeColor, Bg: p.BgPanel, Modifier: cell.ModifierBold}, inner.X+inner.Width-statusX)
	}

	// Row 2 (if height permits): Active Profile & Permission indicator
	if inner.Height >= 3 {
		row2Y := inner.Y + 2
		if info.PermissionDenied {
			f.Buffer.SetStringWithin(inner.X, row2Y, "⚠ Read-Only (udev rule needed)", cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold}, inner.Width)
		} else {
			profLine := fmt.Sprintf("Profile: %s", s.ActiveProfile)
			f.Buffer.SetStringWithin(inner.X, row2Y, profLine, p.LabelStyle(), inner.Width)
		}
	}

	f.RegisterAccessibility(accessibility.AccessibilityNode{
		ID:     "status_widget",
		Role:   accessibility.RoleGeneric,
		Label:  "Controller Status",
		Value:  fmt.Sprintf("Connected=%v Battery=%d%% Charging=%v Transport=%s", info.Connected, tel.BatteryPercent, tel.Charging, info.Connection),
		Bounds: area,
	})
}
