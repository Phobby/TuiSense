package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/ui/copytext"
	"github.com/thebanri/limoni/core/driver"

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

	inner := panelFrame(f, area, s, i18n.T("Diagnostics & Logs"))
	y := inner.Y

	// Live HID Report Hex Inspector Header
	tel := s.Telemetry
	f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("HID Bus Inspector (Rx:%d  Tx:%d)", tel.PacketsRx, tel.PacketsTx), p.SectionHeaderStyle(), inner.Width)
	y++

	if y < inner.Y+inner.Height {
		inLine := i18n.Tf("IN  [%s]: %s", s.DeviceInfo.InputMode, nonEmptyStr(tel.LastInputHex, "--"))
		copyValue(f, cell.NewRect(inner.X, y, inner.Width, 1), s, inLine, inLine, cell.Style{Fg: p.AccentCyan, Bg: p.BgElevated})
		y++
	}
	if y < inner.Y+inner.Height {
		outLine := i18n.Tf("OUT [%s]: %s", s.DeviceInfo.OutputMode, nonEmptyStr(tel.LastOutputHex, "--"))
		copyValue(f, cell.NewRect(inner.X, y, inner.Width, 1), s, outLine, outLine, cell.Style{Fg: p.AccentPurple, Bg: p.BgElevated})
		y += 2
	}

	remainH := int(inner.Y+inner.Height) - int(y)
	if remainH >= 3 {
		lv := &widgets.LogView{
			ID:          "diagnostic_log_view",
			Label:       i18n.T("System Logs"),
			Source:      s.Logger,
			State:       s.LogViewState,
			LineNumbers: true,
			Style:       p.PanelStyle(),
		}
		f.RenderWidget(lv, cell.NewRect(inner.X, y, inner.Width, uint16(remainH)))
		widest := s.Logger.Len()
		if widest > 0 {
			widest = s.Logger.LineNumber(widest - 1)
		}
		gutter := uint16(len(fmt.Sprint(widest)) + 1)
		if gutter >= inner.Width {
			gutter = 0
		}
		if inner.Width > gutter {
			for row := 0; row < remainH; row++ {
				index := s.LogViewState.Offset + row
				if index >= s.Logger.Len() {
					break
				}
				full := s.Logger.Line(index)
				display := full
				offset := s.LogViewState.HOffset
				for len(display) > 0 && offset > 0 {
					r := []rune(display)
					display = string(r[1:])
					offset -= cell.StringWidth(string(r[0]))
				}
				copytext.CopyableText{Text: display, Value: full, Style: p.LabelStyle(), Overlay: true, OnScroll: func(ev driver.MouseEvent) {
					if ev.Button == driver.MouseScrollUp {
						s.LogViewState.Offset = max(0, s.LogViewState.Offset-1)
					} else {
						s.LogViewState.Offset = min(max(0, s.Logger.Len()-remainH), s.LogViewState.Offset+1)
					}
					s.LogViewState.Follow = false
				}}.Render(f, cell.NewRect(inner.X+gutter, y+uint16(row), inner.Width-gutter, 1), &s.Copy)
			}
		}
	}
}
