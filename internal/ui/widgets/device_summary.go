package widgets

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/ui/copytext"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
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
		Title:         i18n.T(" Device "),
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

	lastSeenStr := i18n.T("Never")
	if !info.LastSeen.IsZero() {
		ago := time.Since(info.LastSeen)
		if ago < time.Second {
			lastSeenStr = i18n.Tf("%dms ago (Live)", ago.Milliseconds())
		} else {
			lastSeenStr = i18n.Tf("%s (%ds ago)", info.LastSeen.Format("15:04:05"), int(ago.Seconds()))
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
		nameVal = i18n.T("No Controller Attached")
	}

	desc := s.Descriptor
	firmware := nonEmpty(info.FirmwareVersion, "—")
	var rows []kvRow
	if s.Mode == state.ModeEZ {
		rows = []kvRow{
			{key: i18n.T("Model"), val: nameVal, col: p.FgPrimary},
			{key: i18n.T("Brand"), val: nonEmpty(desc.Brand, "—"), col: p.FgSecondary},
			{key: i18n.T("Link"), val: i18n.T(string(info.Connection)), col: p.FgSecondary},
		}
		if s.Caps.Supports(device.CapFirmwareInfo) {
			rows = append(rows, kvRow{key: i18n.T("Firmware"), val: firmware, col: p.FgSecondary})
		}
	} else {
		rows = []kvRow{
			{key: i18n.T("Model"), val: nameVal, col: p.FgPrimary},
			{key: i18n.T("Brand"), val: nonEmpty(desc.Brand, "—"), col: p.FgSecondary},
			{key: i18n.T("Link"), val: i18n.T(string(info.Connection)), col: p.FgSecondary},
		}
		if s.Caps.Supports(device.CapFirmwareInfo) {
			rows = append(rows, kvRow{key: i18n.T("Firmware"), val: firmware, col: p.FgSecondary})
			if info.FirmwareBuild != "" {
				rows = append(rows, kvRow{key: i18n.T("FW build"), val: info.FirmwareBuild, col: p.FgMuted})
			}
		}
		rows = append(rows, kvRow{key: i18n.T("USB ID"), val: fmt.Sprintf("%04X:%04X", info.VendorID, info.ProductID), col: p.FgMuted})
		if info.MACAddress != "" {
			rows = append(rows, kvRow{key: i18n.T("MAC"), val: info.MACAddress, col: p.FgSecondary})
		}
		if info.KernelDriver != "" {
			rows = append(rows, kvRow{key: i18n.T("Driver"), val: info.KernelDriver, col: p.FgSecondary})
		}
		rows = append(rows,
			kvRow{key: i18n.T("Path"), val: nonEmpty(info.HIDRawPath, nonEmpty(info.InputDevicePath, "—")), col: p.FgSecondary},
			kvRow{key: i18n.T("Seen"), val: lastSeenStr, col: p.FgMuted},
			kvRow{key: i18n.T("Caps"), val: capabilityCounts(s.Caps), col: p.FgSecondary},
		)
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
			copytext.CopyableText{Text: r.val, Value: r.val, Style: valStyle}.Render(f, cell.NewRect(inner.X+labelW, y, inner.Width-labelW, 1), &s.Copy)
		}
	}

	f.RegisterAccessibility(accessibility.AccessibilityNode{
		ID:     "device_summary_widget",
		Role:   accessibility.RoleGeneric,
		Label:  i18n.T("Device Information"),
		Value:  i18n.Tf("%s MAC=%s Path=%s", nameVal, info.MACAddress, nonEmpty(info.HIDRawPath, info.InputDevicePath)),
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
	return i18n.Tf("%d full · %d limited · %d n/a", full, limited, len(device.AllCapabilities)-full-limited)
}
