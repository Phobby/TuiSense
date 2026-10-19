package widgets

import (
	"fmt"
	"time"

	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/state"
)

// RenderDeviceSummaryWidget renders the compact controller hardware & protocol
// information panel positioned directly below the battery/status widget.
func RenderDeviceSummaryWidget(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width < 20 || area.Height < 5 {
		return
	}
	p := s.Theme
	info := s.DeviceInfo

	block := widgets.Block{
		Title:         " Device ",
		TitleStyle:    p.CardTitle(),
		Borders:       widgets.BorderAll,
		BorderSymbols: widgets.SymbolsRounded,
		BorderStyle:   p.CardBorder(),
		Style:         p.PanelStyle(),
	}
	f.RenderWidget(block, area)

	inner := cell.NewRect(area.X+2, area.Y+1, area.Width-4, area.Height-2)
	if inner.Width < 14 || inner.Height < 3 {
		return
	}

	lastSeenStr := "Never"
	if !info.LastSeen.IsZero() {
		ago := time.Since(info.LastSeen)
		if ago < time.Second {
			lastSeenStr = fmt.Sprintf("%dms ago (Live)", ago.Milliseconds())
		} else {
			lastSeenStr = fmt.Sprintf("%s (%ds ago)", info.LastSeen.Format("15:04:05"), int(ago.Seconds()))
		}
	}

	type kvRow struct {
		key string
		val string
		col cell.Color
	}

	nameVal := s.Descriptor.ShortName
	if nameVal == "" {
		nameVal = info.Model
	}
	if !info.Connected {
		nameVal = "No Controller Attached"
	}

	desc := s.Descriptor
	firmware := "—"
	if info.FirmwareVersion != "" {
		firmware = "v" + info.FirmwareVersion
	}
	var rows []kvRow
	if s.Mode == state.ModeEZ {
		rows = []kvRow{
			{key: "Model", val: nameVal, col: p.FgPrimary},
			{key: "Brand", val: nonEmpty(desc.Brand, "—"), col: p.FgSecondary},
			{key: "Link", val: string(info.Connection), col: p.FgSecondary},
			{key: "Firmware", val: firmware, col: p.FgSecondary},
		}
	} else {
		rows = []kvRow{
			{key: "Model", val: nameVal, col: p.FgPrimary},
			{key: "Brand", val: nonEmpty(desc.Brand, "—"), col: p.FgSecondary},
			{key: "Link", val: string(info.Connection), col: p.FgSecondary},
			{key: "Firmware", val: fmt.Sprintf("%s · HW %s", nonEmpty(info.FirmwareVersion, "—"), nonEmpty(info.HardwareVersion, "—")), col: p.FgSecondary},
			{key: "USB ID", val: fmt.Sprintf("%04X:%04X", info.VendorID, info.ProductID), col: p.FgMuted},
			{key: "MAC", val: nonEmpty(info.MACAddress, "N/A"), col: p.FgSecondary},
			{key: "Path", val: nonEmpty(info.HIDRawPath, "—"), col: p.FgSecondary},
			{key: "Seen", val: lastSeenStr, col: p.FgMuted},
			{key: "Caps", val: capabilityCounts(s.Caps), col: p.FgSecondary},
		}
	}

	labelW := uint16(10)
	for i, r := range rows {
		if uint16(i) >= inner.Height {
			break
		}
		y := inner.Y + uint16(i)
		f.Buffer.SetStringWithin(inner.X, y, r.key, p.MutedStyle(), labelW)
		if inner.Width > labelW {
			valStyle := cell.Style{Fg: r.col, Bg: p.BgPanel}
			if i == 0 {
				valStyle.Modifier |= cell.ModifierBold
			}
			f.Buffer.SetStringWithin(inner.X+labelW, y, r.val, valStyle, inner.Width-labelW)
		}
	}

	f.RegisterAccessibility(accessibility.AccessibilityNode{
		ID:     "device_summary_widget",
		Role:   accessibility.RoleGeneric,
		Label:  "Device Information",
		Value:  fmt.Sprintf("%s MAC=%s Path=%s", nameVal, info.MACAddress, info.HIDRawPath),
		Bounds: area,
	})
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func capabilityCounts(caps device.CapabilitySet) string {
	full, limited := 0, 0
	for _, c := range device.AllCapabilities {
		switch caps.Level(c) {
		case device.Full:
			full++
		case device.Limited:
			limited++
		}
	}
	return fmt.Sprintf("%d full · %d limited · %d n/a", full, limited, len(device.AllCapabilities)-full-limited)
}
