package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
)

// RenderDeviceInfoPanel renders the hardware, transport, identification and
// capability matrix view. Rows only appear for facts the device reports.
func RenderDeviceInfoPanel(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	s := pc.S
	p := s.Theme
	info := s.DeviceInfo
	tel := s.Telemetry

	inner := panelFrame(f, area, s, i18n.T("Device & Capabilities"))
	c := newCursor(f, inner, s)
	renderCopyActions(c, false)
	left, right := splitColumns(c, s, 110)

	lastSeen := i18n.T("Offline")
	if !info.LastSeen.IsZero() {
		lastSeen = i18n.Tf("%s (%dms ago)", info.LastSeen.Format("15:04:05.000"), time.Since(info.LastSeen).Milliseconds())
	}
	type row struct{ k, v string }
	identity := []row{
		{i18n.T("Device Name:"), info.Name},
		{i18n.T("Model ID:"), s.Descriptor.ModelID + " (" + s.Descriptor.Brand + ")"},
		{i18n.T("Model / SKU:"), i18n.T(info.Model)},
		{i18n.T("USB VID / PID:"), fmt.Sprintf("0x%04X : 0x%04X", info.VendorID, info.ProductID)},
	}
	if s.Caps.Supports(device.CapFirmwareInfo) {
		identity = append(identity, row{i18n.T("Firmware:"), nonEmptyStr(info.FirmwareVersion, i18n.T("unknown"))})
		if info.FirmwareBuild != "" {
			identity = append(identity, row{i18n.T("FW build:"), info.FirmwareBuild})
		}
		if info.HardwareVersion != "" {
			identity = append(identity, row{i18n.T("HW version:"), info.HardwareVersion + " " + i18n.T("(raw)")})
		}
	}
	if info.MACAddress != "" {
		identity = append(identity, row{i18n.T("Bluetooth MAC:"), info.MACAddress})
	}
	var link []row
	if info.HIDRawPath != "" {
		link = append(link, row{i18n.T("HIDRaw Node:"), info.HIDRawPath})
	}
	if info.InputDevicePath != "" {
		link = append(link, row{i18n.T("Evdev Input:"), info.InputDevicePath})
	}
	link = append(link,
		row{i18n.T("Transport:"), i18n.T(string(info.Connection))},
		row{i18n.T("Input Report:"), i18n.T(info.InputMode)},
		row{i18n.T("Output Report:"), i18n.T(info.OutputMode)},
		row{i18n.T("Packets Rx/Tx:"), i18n.Tf("%d Rx  /  %d Tx", tel.PacketsRx, tel.PacketsTx)},
	)
	if s.Descriptor.Inputs.TouchpadContacts > 0 {
		link = append(link, row{i18n.T("Touchpad Pt 1:"), i18n.Tf("Active=%v  X=%d  Y=%d", tel.TouchActive, tel.TouchX, tel.TouchY)})
	}
	link = append(link, row{i18n.T("Last Seen:"), lastSeen})
	for _, d := range info.Details {
		link = append(link, row{i18n.T(d.Label) + ":", i18n.T(d.Value)})
	}
	section := func(c *cursor, title string, rows []row) {
		if !c.ok() {
			return
		}
		c.y = sectionHeader(f, s, c.r, c.y, title)
		for _, d := range rows {
			if !c.ok() {
				return
			}
			c.kv(d.k, d.v, 16, p.LabelStyle())
		}
		c.gap()
	}
	section(left, i18n.T("Identity"), identity)
	section(left, i18n.T("Connection"), link)
	renderIdentity(left, s, pc.CB)

	// Capabilities: declared support × live availability, one line each with
	// the reason underneath when there is one.
	if right == left {
		right.gap()
	}
	if !right.ok() {
		return
	}
	right.y = sectionHeader(f, s, right.r, right.y, i18n.T("Capabilities"))
	for _, capability := range device.AllCapabilities {
		if !right.ok() {
			break
		}
		level, reason := device.Availability(info, s.Caps, capability)
		icon, col, text := "✓", p.Success, p.FgPrimary
		switch level {
		case device.Limited:
			icon, col = "◐", p.Warning
		case device.Unsupported:
			icon, col, text = "✕", p.FgMuted, p.FgMuted
		}
		f.Buffer.SetStringWithin(right.r.X, right.y, icon, cell.Style{Fg: col, Bg: p.BgPanel, Modifier: cell.ModifierBold}, 2)
		label := capability.Label()
		copyText := label
		if reason != "" {
			copyText += " — " + reason
		}
		copyValue(f, cell.NewRect(right.r.X+2, right.y, right.r.Width-2, 1), s, label, copyText, cell.Style{Fg: text, Bg: p.BgPanel})
		right.y++
		if reason != "" {
			note := newCursor(f, cell.NewRect(right.r.X+2, right.y, right.r.Width-2, uint16(max(0, right.remaining()))), s)
			note.wrap(reason, p.MutedStyle(), 3)
			right.y = note.y
		}
	}
}
