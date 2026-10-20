package views

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
)

// Callbacks connects views to application-level actions without importing
// the session layer.
type Callbacks struct {
	// Simulation (mock/test devices)
	OnToggleConnect    func()
	OnCycleTransport   func()
	OnTogglePermission func()

	// Device & mode
	OnSelectDevice func(key string)
	OnCycleDevice  func(delta int)
	OnSetMode      func(state.UXMode)

	// OnPrefsChanged persists preferences (e.g. after a profile is applied).
	OnPrefsChanged func()

	// OnExportReport writes a diagnostics report (JSON + text).
	OnExportReport func()

	// OnSetLanguage switches and persists the UI language.
	OnSetLanguage func(i18n.Lang)

	// OnConfirmIdentity remembers the user's model choice for a hardware key
	// (modelID "" forgets it) and re-opens the device with that model.
	OnConfirmIdentity func(hwKey, modelID string)
}

// PanelContext is passed to every panel renderer.
type PanelContext struct {
	S        *state.AppState
	Dispatch func(device.Command)
	CB       Callbacks
}

func (pc PanelContext) dispatch(cmd device.Command) {
	if pc.Dispatch != nil {
		pc.Dispatch(cmd)
	}
}

func (pc PanelContext) prefsChanged() {
	if pc.CB.OnPrefsChanged != nil {
		pc.CB.OnPrefsChanged()
	}
}

// PanelRenderer draws one inspector panel.
type PanelRenderer func(f *terminal.Frame, area cell.Rect, pc PanelContext)

// PanelView binds a catalog panel to its renderers. EZ is optional: panels
// without an EZ renderer fall back to Advanced (only reachable if the catalog
// lists them for EZ).
type PanelView struct {
	EZ       PanelRenderer
	Advanced PanelRenderer
}

// panelViews maps each state.PanelCatalog entry to its renderers. Which panels
// are reachable is decided by the catalog (capabilities × mode), not here.
var panelViews = map[state.PanelID]PanelView{
	state.PanelOverview: {EZ: renderEZHome, Advanced: renderOverviewInspector},
	// Diagnostics uses one renderer that adapts every section to the mode
	// (guided + plain language in EZ; raw values, graphs, statistics in Advanced).
	state.PanelDiagnostics: {EZ: renderDiagnosticsPanel, Advanced: renderDiagnosticsPanel},
	state.PanelLighting: {EZ: renderEZLighting, Advanced: func(f *terminal.Frame, a cell.Rect, pc PanelContext) {
		RenderLightingPanel(f, a, pc.S, pc.Dispatch)
	}},
	state.PanelHaptics: {EZ: renderEZHaptics, Advanced: func(f *terminal.Frame, a cell.Rect, pc PanelContext) {
		RenderHapticsPanel(f, a, pc.S, pc.Dispatch)
	}},
	state.PanelTriggers: {EZ: renderEZTriggers, Advanced: func(f *terminal.Frame, a cell.Rect, pc PanelContext) {
		RenderTriggersPanel(f, a, pc.S, pc.Dispatch)
	}},
	state.PanelSticks: {Advanced: func(f *terminal.Frame, a cell.Rect, pc PanelContext) {
		RenderSticksPanel(f, a, pc.S)
	}},
	state.PanelMotion: {Advanced: func(f *terminal.Frame, a cell.Rect, pc PanelContext) {
		RenderMotionPanel(f, a, pc.S)
	}},
	state.PanelAudio:    {Advanced: renderAudioPanel},
	state.PanelProfiles: {EZ: renderEZProfiles, Advanced: renderProfilesPanel},
	state.PanelDevice:   {Advanced: RenderDeviceInfoPanel},
	state.PanelFirmware: {Advanced: renderFirmwarePanel},
	state.PanelSettings: {EZ: renderEZSettings, Advanced: func(f *terminal.Frame, a cell.Rect, pc PanelContext) {
		RenderSettingsPanel(f, a, pc.S, pc.CB)
	}},
	state.PanelLogs: {Advanced: func(f *terminal.Frame, a cell.Rect, pc PanelContext) {
		RenderLogsPanel(f, a, pc.S)
	}},
}

// HasView reports whether a renderer exists for id in mode (used by tests to
// guarantee every catalog panel is renderable).
func HasView(id state.PanelID, mode state.UXMode) bool {
	v, ok := panelViews[id]
	if !ok {
		return false
	}
	if mode == state.ModeEZ && v.EZ != nil {
		return true
	}
	return v.Advanced != nil
}

// RenderPanel draws the active panel using the mode-appropriate renderer.
func RenderPanel(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	if area.Width < 24 || area.Height < 6 {
		return
	}
	v, ok := panelViews[pc.S.ActivePanel]
	if !ok {
		v = panelViews[state.PanelOverview]
	}
	r := v.Advanced
	if pc.S.Mode == state.ModeEZ && v.EZ != nil {
		r = v.EZ
	}
	r(f, area, pc)
}
