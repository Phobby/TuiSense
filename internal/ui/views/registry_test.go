package views

import (
	"testing"

	"github.com/Phobby/TuiSense/internal/state"
)

func TestEveryCatalogPanelHasAView(t *testing.T) {
	for _, spec := range state.PanelCatalog {
		for _, m := range []state.UXMode{state.ModeEZ, state.ModeAdvanced} {
			if spec.Modes.Includes(m) && !HasView(spec.ID, m) {
				t.Errorf("panel %s is listed for %s mode but has no renderer", spec.ID, m)
			}
		}
	}
}

func TestEZPanelsHaveDedicatedRenderers(t *testing.T) {
	for _, spec := range state.PanelCatalog {
		if spec.Modes.Includes(state.ModeEZ) && panelViews[spec.ID].EZ == nil {
			t.Errorf("EZ panel %s reuses the Advanced renderer; EZ must be genuinely simpler", spec.ID)
		}
	}
}
