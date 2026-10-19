package views

import (
	"fmt"

	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// RenderLogsPanel renders the live ring-buffer diagnostic log viewer (widgets.LogView)
// alongside a real-time HID Input/Output report hex inspector.
func RenderLogsPanel(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width < 24 || area.Height < 8 {
		return
	}
	p := s.Theme

	inner := panelFrame(f, area, s, "Diagnostics & Logs")
	y := inner.Y

	// Live HID Report Hex Inspector Header
	tel := s.Telemetry
	f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("HID Bus Inspector (Rx:%d  Tx:%d)", tel.PacketsRx, tel.PacketsTx), p.SectionHeaderStyle(), inner.Width)
	y++

	if y < inner.Y+inner.Height {
		inLine := fmt.Sprintf("IN  [%s]: %s", s.DeviceInfo.InputMode, nonEmptyStr(tel.LastInputHex, "--"))
		f.Buffer.SetStringWithin(inner.X, y, inLine, cell.Style{Fg: p.AccentCyan, Bg: p.BgElevated}, inner.Width)
		y++
	}
	if y < inner.Y+inner.Height {
		outLine := fmt.Sprintf("OUT [%s]: %s", s.DeviceInfo.OutputMode, nonEmptyStr(tel.LastOutputHex, "--"))
		f.Buffer.SetStringWithin(inner.X, y, outLine, cell.Style{Fg: p.AccentPurple, Bg: p.BgElevated}, inner.Width)
		y += 2
	}

	remainH := int(inner.Y+inner.Height) - int(y)
	if remainH >= 3 {
		lv := &widgets.LogView{
			ID:          "diagnostic_log_view",
			Label:       "System Logs",
			Source:      s.Logger,
			State:       s.LogViewState,
			LineNumbers: true,
			Style:       p.PanelStyle(),
		}
		f.RenderWidget(lv, cell.NewRect(inner.X, y, inner.Width, uint16(remainH)))
	}
}
