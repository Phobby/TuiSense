package widgets

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// RenderHelpBar keeps a single quiet entry point to the complete help screen.
func RenderHelpBar(f *terminal.Frame, area cell.Rect, s *state.AppState) {
	if area.Width == 0 || area.Height == 0 {
		return
	}
	text := "? " + i18n.T("Help")
	f.Buffer.SetStringWithin(area.X, area.Y, text, s.Theme.MutedStyle(), area.Width)
	f.RegisterClickHandler(cell.NewRect(area.X, area.Y, min(area.Width, uint16(cell.StringWidth(text))), 1), func(_ driver.MouseEvent) { s.ShowHelp = true })
}
