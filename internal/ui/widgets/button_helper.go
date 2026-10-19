package widgets

import (
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// RenderButton renders a Limoni widgets.Button and ensures mouse clicks
// both focus the button and invoke OnPress (working around widgets.Button
// registering ClickAction{Focus} after RegisterClick in Limoni v0.9.4).
func RenderButton(f *terminal.Frame, area cell.Rect, btn widgets.Button) {
	f.RenderWidget(btn, area)
	if !btn.Disabled && btn.OnPress != nil {
		id := btn.ID
		onPress := btn.OnPress
		f.RegisterClickHandler(area, func(_ driver.MouseEvent) {
			if id != "" && f.FocusManager != nil {
				f.FocusManager.SetFocused(id)
			}
			onPress()
		})
	}
}
