package visual

import (
	"testing"

	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/drivers/dualsense"
	"github.com/ygzztrk/tuisense/internal/drivers/xboxmock"
	"github.com/ygzztrk/tuisense/internal/theme"
)

// Every declared button of every built-in model must light up on its visual.
func TestDeclaredButtonsHighlightOnVisual(t *testing.T) {
	for _, d := range []device.Descriptor{dualsense.Descriptor(), xboxmock.Descriptor()} {
		v := Lookup(d.VisualID)
		base := Context{Palette: theme.Default(), Connected: true, ShowPressed: true}
		_, idle := v.Build(base)
		for _, b := range d.Inputs.Buttons {
			ctx := base
			ctx.Pressed = device.ButtonSet(0).With(b.ID)
			_, pressed := v.Build(ctx)
			changed := 0
			for i := range idle {
				if idle[i] != pressed[i] {
					changed++
				}
			}
			if changed == 0 {
				t.Errorf("%s: button %q has no highlighted geometry", d.ShortName, b.Label)
			}
		}
	}
}
