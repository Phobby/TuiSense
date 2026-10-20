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
	c.line(i18n.T("Identity & Transport"), p.SectionHeaderStyle())

	lastSeen := i18n.T("Offline")
	if !info.LastSeen.IsZero() {
		lastSeen = i18n.Tf("%s (%dms ago)", info.LastSeen.Format("15:04:05.000"), time.Since(info.LastSeen).Milliseconds())
	}
	type row struct{ k, v string }
	rows := []row{
		{i18n.T("Device Name:"), info.Name},
		{i18n.T("Model ID:"), s.Descriptor.ModelID + " (" + s.Descriptor.Brand + ")"},
		{i18n.T("Model / SKU:"), i18n.T(info.Model)},
		{i18n.T("USB VID / PID:"), fmt.Sprintf("0x%04X : 0x%04X", info.VendorID, info.ProductID)},
	}
	if s.Caps.Supports(device.CapFirmwareInfo) {
		rows = append(rows, row{i18n.T("Firmware / HW:"), i18n.Tf("FW %s / HW %s", info.FirmwareVersion, info.HardwareVersion)})
	}
	if info.MACAddress != "" {
		rows = append(rows, row{i18n.T("Bluetooth MAC:"), info.MACAddress})
	}
	if info.HIDRawPath != "" {
		rows = append(rows, row{i18n.T("HIDRaw Node:"), info.HIDRawPath})
	}
	if info.InputDevicePath != "" {
		rows = append(rows, row{i18n.T("Evdev Input:"), info.InputDevicePath})
	}
	rows = append(rows,
		row{i18n.T("Transport:"), i18n.T(string(info.Connection))},
		row{i18n.T("Input Report:"), i18n.T(info.InputMode)},
		row{i18n.T("Output Report:"), i18n.T(info.OutputMode)},
		row{i18n.T("Packets Rx/Tx:"), i18n.Tf("%d Rx  /  %d Tx", tel.PacketsRx, tel.PacketsTx)},
	)
	if s.Descriptor.Inputs.TouchpadContacts > 0 {
		rows = append(rows, row{i18n.T("Touchpad Pt 1:"), i18n.Tf("Active=%v  X=%d  Y=%d", tel.TouchActive, tel.TouchX, tel.TouchY)})
	}
	rows = append(rows, row{i18n.T("Last Seen:"), lastSeen})
	for _, d := range info.Details {
		rows = append(rows, row{i18n.T(d.Label) + ":", i18n.T(d.Value)})
	}
	for _, d := range rows {
		if !c.ok() {
			break
		}
		c.kv(fmt.Sprintf("%-16s", d.k), d.v, 16, p.LabelStyle())
	}
	renderIdentity(c, s, pc.CB)
	c.gap()
	y := c.y

	// Capability Matrix (declared support × live availability)
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.T("Capability Matrix"), p.SectionHeaderStyle(), inner.Width)
		y++
	}
	for _, c := range device.AllCapabilities {
		if y >= inner.Y+inner.Height {
			break
		}
		level, reason := device.Availability(info, s.Caps, c)
		badge := i18n.T(" ✓ FULL    ")
		bStyle := p.BadgeStyle(p.BgCanvas, p.Success)
		switch level {
		case device.Limited:
			badge = i18n.T(" ◐ LIMITED ")
			bStyle = p.BadgeStyle(p.BgCanvas, p.Warning)
		case device.Unsupported:
			badge = " ✕ N/A     "
			bStyle = p.BadgeStyle(p.FgMuted, p.BgSubtle)
		}
		bw := uint16(cell.StringWidth(badge))
		f.Buffer.SetStringWithin(inner.X, y, badge, bStyle, min(bw, inner.Width))
		text := c.Label()
		if reason != "" {
			text += " — " + reason
		}
		if inner.Width > bw+1 {
			f.Buffer.SetStringWithin(inner.X+bw+1, y, text, p.LabelStyle(), inner.Width-bw-1)
		}
		y++
	}
}
