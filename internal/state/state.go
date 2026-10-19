package state

import (
	"sync"
	"time"

	"github.com/thebanri/limoni/widgets"
	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/diag"
	"github.com/ygzztrk/tuisense/internal/logging"
	"github.com/ygzztrk/tuisense/internal/theme"
)

// DeviceChoice is a selector entry for an attached device.
type DeviceChoice struct {
	Key        string
	Descriptor device.Descriptor
	Source     device.Source
	Connected  bool
	Active     bool
}

// AppState holds the complete UI state, active controller snapshot, and reusable
// Limoni widget states so drawing remains allocation-free and responsive.
type AppState struct {
	mu sync.RWMutex

	// Theme, Mode & Layout
	Theme        theme.Palette
	HighContrast bool
	Mode         UXMode
	ActivePanel  PanelID
	ShowHelp     bool

	// Active Device Context (drives visual, info card, panels, and interaction)
	ActiveDeviceKey    string
	Descriptor         device.Descriptor
	Caps               device.CapabilitySet
	Devices            []DeviceChoice
	ShowDeviceSelector bool
	SelectorIndex      int
	RegionHint         string        // shown when a region's panel is hidden in the current mode
	Diag               *diag.Session // diagnostics session of the active device (nil without one)
	deviceConfigs      map[string]device.Config
	activeProfiles     map[string]string // device family -> active profile name

	// 3D Stage & Region Selection
	SelectedRegion  device.RegionID
	HoveredRegion   device.RegionID
	RotX            float64
	RotY            float64
	RotZ            float64
	Zoom            float64
	AutoRotate      bool
	AutoRotateSpeed float64
	ShowcasePhase   float64
	ShadingMode     string // "Gouraud", "Lambert", "Flat", "Wireframe"
	RenderMarker    int    // 0: Braille, 1: HalfBlock, 2: Quadrant

	// Controller State Snapshot
	DeviceInfo    device.Info
	Telemetry     device.Telemetry
	Config        device.Config
	ActiveProfile string
	LastError     string
	LastErrorAt   time.Time

	// Trigger Panel Widget States
	EditingRightTrigger bool // false = L2, true = R2
	L2StartSlider       *widgets.SliderState
	L2EndSlider         *widgets.SliderState
	L2StrengthSlider    *widgets.SliderState
	L2FreqSlider        *widgets.SliderState
	R2StartSlider       *widgets.SliderState
	R2EndSlider         *widgets.SliderState
	R2StrengthSlider    *widgets.SliderState
	R2FreqSlider        *widgets.SliderState

	// Haptics Panel Widget States
	LeftMotorSlider  *widgets.SliderState
	RightMotorSlider *widgets.SliderState
	MasterHapSlider  *widgets.SliderState
	LTMotorSlider    *widgets.SliderState
	RTMotorSlider    *widgets.SliderState
	SpeakerVolSlider *widgets.SliderState
	HeadsetVolSlider *widgets.SliderState
	MicGainSlider    *widgets.SliderState

	// Lighting Panel Widget States
	ColorPickerState *widgets.ColorPickerState

	// Stick Tuning Panel Widget States
	EditingRightStick bool
	LInnerDZSlider    *widgets.SliderState
	LOuterDZSlider    *widgets.SliderState
	LSensSlider       *widgets.SliderState
	RInnerDZSlider    *widgets.SliderState
	ROuterDZSlider    *widgets.SliderState
	RSensSlider       *widgets.SliderState

	// Motion Panel Widget States
	GyroSensSlider *widgets.SliderState

	// Profiles Panel Widget States
	ProfileMgr       *ProfileManager
	ProfileListState *widgets.ListState
	ProfileNameInput *widgets.TextInputState
	BindToMAC        bool

	// Top Tabs, Logs, and Toasts
	NavTabsState *widgets.TabsState
	LogViewState *widgets.LogViewState
	Logger       *logging.Logger
	Toasts       *widgets.ToastManager

	// Reusable 3D Viewer & Canvases
	Viewer3D    *widgets.Viewer3D
	StageCanvas *widgets.Canvas
	CurveCanvas *widgets.Canvas
	ScopeCanvas *widgets.Canvas
}

// NewAppState constructs an initialized AppState with all Limoni widget states attached.
func NewAppState(profileDir string, logger *logging.Logger) *AppState {
	pm := NewProfileManager(profileDir)
	defaultProf, ok := pm.Get("Astro Standard")
	cfg := device.DefaultConfig()
	if ok {
		cfg = defaultProf.Config
	}

	nameInput := widgets.NewTextInputState()
	nameInput.SetValue("My Custom DualSense")

	s := &AppState{
		Theme:            theme.Default(),
		Mode:             ModeEZ,
		ActivePanel:      PanelOverview,
		Descriptor:       device.PlaceholderDescriptor(),
		Caps:             device.NewCapabilitySet(),
		deviceConfigs:    map[string]device.Config{},
		activeProfiles:   map[string]string{},
		SelectedRegion:   device.RegionLight,
		RotX:             -20.0,
		RotY:             14.0,
		RotZ:             0.0,
		Zoom:             1.0,
		AutoRotate:       true,
		AutoRotateSpeed:  1.1,
		ShadingMode:      "Lambert",
		Config:           cfg,
		ActiveProfile:    "Astro Standard",
		L2StartSlider:    widgets.NewSliderState(int(cfg.LeftTrigger.StartPosition)),
		L2EndSlider:      widgets.NewSliderState(int(cfg.LeftTrigger.EndPosition)),
		L2StrengthSlider: widgets.NewSliderState(int(cfg.LeftTrigger.Strength)),
		L2FreqSlider:     widgets.NewSliderState(int(cfg.LeftTrigger.Frequency)),
		R2StartSlider:    widgets.NewSliderState(int(cfg.RightTrigger.StartPosition)),
		R2EndSlider:      widgets.NewSliderState(int(cfg.RightTrigger.EndPosition)),
		R2StrengthSlider: widgets.NewSliderState(int(cfg.RightTrigger.Strength)),
		R2FreqSlider:     widgets.NewSliderState(int(cfg.RightTrigger.Frequency)),
		LeftMotorSlider:  widgets.NewSliderState(int(cfg.Haptics.LeftMotor)),
		RightMotorSlider: widgets.NewSliderState(int(cfg.Haptics.RightMotor)),
		MasterHapSlider:  widgets.NewSliderState(cfg.Haptics.MasterIntensity),
		LTMotorSlider:    widgets.NewSliderState(int(cfg.Haptics.LeftTriggerMotor)),
		RTMotorSlider:    widgets.NewSliderState(int(cfg.Haptics.RightTriggerMotor)),
		SpeakerVolSlider: widgets.NewSliderState(cfg.Audio.SpeakerVolume),
		HeadsetVolSlider: widgets.NewSliderState(cfg.Audio.HeadphoneVolume),
		MicGainSlider:    widgets.NewSliderState(cfg.Audio.MicGain),
		ColorPickerState: widgets.NewColorPickerState(cfg.Lighting.Red, cfg.Lighting.Green, cfg.Lighting.Blue),
		LInnerDZSlider:   widgets.NewSliderState(cfg.Sticks.Left.InnerDeadzone),
		LOuterDZSlider:   widgets.NewSliderState(cfg.Sticks.Left.OuterDeadzone),
		LSensSlider:      widgets.NewSliderState(cfg.Sticks.Left.Sensitivity),
		RInnerDZSlider:   widgets.NewSliderState(cfg.Sticks.Right.InnerDeadzone),
		ROuterDZSlider:   widgets.NewSliderState(cfg.Sticks.Right.OuterDeadzone),
		RSensSlider:      widgets.NewSliderState(cfg.Sticks.Right.Sensitivity),
		GyroSensSlider:   widgets.NewSliderState(cfg.Motion.GyroSensitivity),
		ProfileMgr:       pm,
		ProfileListState: widgets.NewListState(),
		ProfileNameInput: nameInput,
		BindToMAC:        true,
		NavTabsState:     &widgets.TabsState{},
		LogViewState:     widgets.NewLogViewState(),
		Logger:           logger,
		Toasts:           widgets.NewToastManager(widgets.ToastBottomRight),
		Viewer3D:         &widgets.Viewer3D{ID: "stage_3d"},
		StageCanvas:      widgets.NewCanvas(60, 24),
		CurveCanvas:      widgets.NewCanvas(40, 8),
		ScopeCanvas:      widgets.NewCanvas(28, 8),
	}
	return s
}

// SyncWidgetsFromConfig updates all Limoni slider & color-picker states from s.Config.
func (s *AppState) SyncWidgetsFromConfig() {
	cfg := s.Config
	s.L2StartSlider.Set(int(cfg.LeftTrigger.StartPosition), 0, 255)
	s.L2EndSlider.Set(int(cfg.LeftTrigger.EndPosition), 0, 255)
	s.L2StrengthSlider.Set(int(cfg.LeftTrigger.Strength), 0, 255)
	s.L2FreqSlider.Set(int(cfg.LeftTrigger.Frequency), 0, 100)

	s.R2StartSlider.Set(int(cfg.RightTrigger.StartPosition), 0, 255)
	s.R2EndSlider.Set(int(cfg.RightTrigger.EndPosition), 0, 255)
	s.R2StrengthSlider.Set(int(cfg.RightTrigger.Strength), 0, 255)
	s.R2FreqSlider.Set(int(cfg.RightTrigger.Frequency), 0, 100)

	s.LeftMotorSlider.Set(int(cfg.Haptics.LeftMotor), 0, 255)
	s.RightMotorSlider.Set(int(cfg.Haptics.RightMotor), 0, 255)
	s.MasterHapSlider.Set(cfg.Haptics.MasterIntensity, 0, 100)
	s.LTMotorSlider.Set(int(cfg.Haptics.LeftTriggerMotor), 0, 255)
	s.RTMotorSlider.Set(int(cfg.Haptics.RightTriggerMotor), 0, 255)
	s.SpeakerVolSlider.Set(cfg.Audio.SpeakerVolume, 0, 100)
	s.HeadsetVolSlider.Set(cfg.Audio.HeadphoneVolume, 0, 100)
	s.MicGainSlider.Set(cfg.Audio.MicGain, 0, 100)

	s.ColorPickerState.SetRGB(cfg.Lighting.Red, cfg.Lighting.Green, cfg.Lighting.Blue)

	s.LInnerDZSlider.Set(cfg.Sticks.Left.InnerDeadzone, 0, 40)
	s.LOuterDZSlider.Set(cfg.Sticks.Left.OuterDeadzone, 70, 100)
	s.LSensSlider.Set(cfg.Sticks.Left.Sensitivity, 50, 150)

	s.RInnerDZSlider.Set(cfg.Sticks.Right.InnerDeadzone, 0, 40)
	s.ROuterDZSlider.Set(cfg.Sticks.Right.OuterDeadzone, 70, 100)
	s.RSensSlider.Set(cfg.Sticks.Right.Sensitivity, 50, 150)

	s.GyroSensSlider.Set(cfg.Motion.GyroSensitivity, 10, 200)
}

// VisiblePanels returns the panels available for the active device and mode.
func (s *AppState) VisiblePanels() []PanelEntry {
	return VisiblePanels(s.Caps, s.Mode)
}

// EnsureActivePanelVisible falls back to the overview when the active panel
// is not offered by the current device/mode combination.
func (s *AppState) EnsureActivePanelVisible() {
	if !PanelVisible(s.VisiblePanels(), s.ActivePanel) {
		s.ActivePanel = PanelOverview
	}
}

// SetMode switches the UX mode and re-validates the active panel.
func (s *AppState) SetMode(m UXMode) {
	s.Mode = m
	s.RegionHint = ""
	s.EnsureActivePanelVisible()
}

// ActivateDevice switches the whole UI context to a device. The outgoing
// device's config is remembered so switching back restores its settings.
// cfg is used the first time a key is seen; a nil descriptor key ("") resets
// to the placeholder (no-controller) context.
func (s *AppState) ActivateDevice(key string, desc device.Descriptor, caps device.CapabilitySet, info device.Info, tel device.Telemetry, cfg device.Config) {
	if s.ActiveDeviceKey != "" {
		s.deviceConfigs[s.ActiveDeviceKey] = s.Config
		s.activeProfiles[s.Descriptor.Family] = s.ActiveProfile
	}
	s.ActiveDeviceKey = key
	s.Descriptor = desc
	s.Caps = caps
	s.DeviceInfo = info
	s.Telemetry = tel
	if key != "" {
		if saved, ok := s.deviceConfigs[key]; ok {
			cfg = saved
		}
		s.Config = cfg
		if name, ok := s.activeProfiles[desc.Family]; ok {
			s.ActiveProfile = name
		}
		s.SyncWidgetsFromConfig()
	}
	if _, ok := desc.Region(s.SelectedRegion); !ok {
		s.SelectedRegion = device.RegionNone
		if len(desc.Regions) > 0 {
			s.SelectedRegion = desc.Regions[0].ID
		}
	}
	s.RegionHint = ""
	s.EnsureActivePanelVisible()
}

// HasDeviceConfig reports whether a config has been remembered for key.
func (s *AppState) HasDeviceConfig(key string) bool {
	_, ok := s.deviceConfigs[key]
	return ok || (key != "" && key == s.ActiveDeviceKey)
}

// ActiveProfiles returns the per-family active profile names for persistence.
func (s *AppState) ActiveProfiles() map[string]string {
	out := make(map[string]string, len(s.activeProfiles)+1)
	for k, v := range s.activeProfiles {
		out[k] = v
	}
	if s.ActiveDeviceKey != "" && s.ActiveProfile != "" {
		out[s.Descriptor.Family] = s.ActiveProfile
	}
	return out
}

// RestoreActiveProfiles seeds per-family profile names loaded from preferences.
func (s *AppState) RestoreActiveProfiles(m map[string]string) {
	for k, v := range m {
		s.activeProfiles[k] = v
	}
}

// SelectRegion highlights a controller region and opens the panel for the
// capability it is bound to. If that panel is hidden in the current mode the
// overview stays active and RegionHint explains how to reach it.
func (s *AppState) SelectRegion(r device.RegionID) {
	s.SelectedRegion = r
	s.RegionHint = ""
	region, ok := s.Descriptor.Region(r)
	if !ok {
		return
	}
	// In the diagnostics view a region opens its diagnostic test instead.
	if s.ActivePanel == PanelDiagnostics && s.Diag != nil {
		if c, ok := diag.CategoryForRegion(string(r), s.Diag.Avail); ok {
			s.Diag.Active = c
		}
		return
	}
	switch r {
	case device.RegionLeftTrigger:
		s.EditingRightTrigger = false
	case device.RegionRightTrigger:
		s.EditingRightTrigger = true
	case device.RegionLeftStick:
		s.EditingRightStick = false
	case device.RegionRightStick:
		s.EditingRightStick = true
	}
	panel, ok := PanelForCapability(region.Capability)
	if !ok {
		return
	}
	if PanelVisible(s.VisiblePanels(), panel) {
		s.ActivePanel = panel
		return
	}
	s.ActivePanel = PanelOverview
	switch {
	case !s.Caps.Supports(region.Capability):
		s.RegionHint = region.Label + ": " + s.Caps.Spec(region.Capability).Note
	case s.Mode == ModeEZ:
		s.RegionHint = region.Label + " settings are available in Advanced mode — press [E]."
	}
}

// CycleRegion advances the selected region through the active descriptor's regions.
func (s *AppState) CycleRegion(delta int) {
	ids := s.Descriptor.RegionIDs()
	if len(ids) == 0 {
		return
	}
	idx := 0
	for i, r := range ids {
		if r == s.SelectedRegion {
			idx = i
			break
		}
	}
	next := ((idx+delta)%len(ids) + len(ids)) % len(ids)
	s.SelectRegion(ids[next])
}

// ResetCamera restores the default 3D controller orientation and zoom.
func (s *AppState) ResetCamera() {
	s.RotX = -20.0
	s.RotY = 14.0
	s.RotZ = 0.0
	s.Zoom = 1.0
	s.ShowcasePhase = 0.0
}

// SetError records a recoverable inline error and displays a toast notification.
func (s *AppState) SetError(title, msg string) {
	s.LastError = msg
	s.LastErrorAt = time.Now()
	if s.Toasts != nil {
		s.Toasts.Error(title, msg)
	}
}

// ClearError clears any active inline error banner.
func (s *AppState) ClearError() {
	s.LastError = ""
}
