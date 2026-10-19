package views

import (
	"fmt"
	"time"

	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/state"
)

// RenderDeviceInfoPanel renders the full hardware, firmware, transport, and capability matrix view.
func RenderDeviceInfoPanel(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme
	info := s.DeviceInfo
	tel := s.Telemetry

	inner := panelFrame(f, area, s, "Device & Capabilities")
	y := inner.Y

	f.Buffer.SetStringWithin(inner.X, y, "Identity & Transport", p.SectionHeaderStyle(), inner.Width)
	y++

	lastSeen := "Offline"
	if !info.LastSeen.IsZero() {
		lastSeen = fmt.Sprintf("%s (%dms ago)", info.LastSeen.Format("15:04:05.000"), time.Since(info.LastSeen).Milliseconds())
	}

	details := []struct {
		k string
		v string
	}{
		{"Device Name:", info.Name},
		{"Model ID:", s.Descriptor.ModelID + " (" + s.Descriptor.Brand + ")"},
		{"Model / SKU:", info.Model},
		{"USB VID / PID:", fmt.Sprintf("0x%04X : 0x%04X", info.VendorID, info.ProductID)},
		{"Firmware / HW:", fmt.Sprintf("FW %s / HW %s", info.FirmwareVersion, info.HardwareVersion)},
		{"Bluetooth MAC:", nonEmptyStr(info.MACAddress, "N/A (Wired USB)")},
		{"HIDRaw Node:", nonEmptyStr(info.HIDRawPath, "None")},
		{"Evdev Input:", nonEmptyStr(info.InputDevicePath, "Managed by hid-playstation")},
		{"Transport:", string(info.Connection)},
		{"Input Report:", info.InputMode},
		{"Output Report:", info.OutputMode},
		{"Packets Rx/Tx:", fmt.Sprintf("%d Rx  /  %d Tx", tel.PacketsRx, tel.PacketsTx)},
		{"Touchpad Pt 1:", fmt.Sprintf("Active=%v  X=%d  Y=%d", tel.TouchActive, tel.TouchX, tel.TouchY)},
		{"Last Seen:", lastSeen},
	}

	for _, d := range details {
		if y >= inner.Y+inner.Height {
			break
		}
		f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("%-16s", d.k), p.MutedStyle(), 16)
		if inner.Width > 16 {
			f.Buffer.SetStringWithin(inner.X+16, y, d.v, p.LabelStyle(), inner.Width-16)
		}
		y++
	}
	y++

	// Capability Matrix (declared support × live availability)
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, "Capability Matrix", p.SectionHeaderStyle(), inner.Width)
		y++
	}
	for _, c := range device.AllCapabilities {
		if y >= inner.Y+inner.Height {
			break
		}
		level, reason := device.Availability(info, s.Caps, c)
		badge := " ✓ FULL    "
		bStyle := p.BadgeStyle(p.BgCanvas, p.Success)
		switch level {
		case device.Limited:
			badge = " ◐ LIMITED "
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
