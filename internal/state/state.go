package state

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/ui/copytext"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/compat"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/theme"
	"github.com/Phobby/TuiSense/internal/update"
	"github.com/thebanri/limoni/widgets"
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
	DiagnosticDeadzoneSlider *widgets.SliderState
	DiagnosticsScroll        int
	DiagnosticsScrollMax     int
	DiagnosticsScope         string
	mu                       sync.RWMutex
	Copy                     copytext.Model
	IncludeCopyIdentifiers   bool
	CopyFallbackPath         string
	ShowCopyFallback         bool
	CopyStatus               string
	CopyStatusUntil          time.Time

	// AppVersion is the running TuiSense version ("0.8.0" or "dev").
	AppVersion string
	// Update mirrors the application updater for the Settings view.
	Update update.Snapshot

	// Theme, Mode & Layout
	Theme            theme.Palette
	HighContrast     bool
	Mode             UXMode
	Language         i18n.Lang
	ActivePanel      PanelID
	ShowHelp         bool
	HelpScroll       int
	HelpScrollMax    int
	PanelScroll      int
	PanelScrollMax   int
	PanelScrollScope string

	// Active Device Context (drives visual, info card, panels, and interaction)
	ActiveDeviceKey    string
	Descriptor         device.Descriptor
	Caps               device.CapabilitySet
	Devices            []DeviceChoice
	ShowDeviceSelector bool
	SelectorIndex      int
	// ShowQuitConfirm is the "Quit TuiSense?" dialog that Esc opens on the
	// main screen; QuitRequested is set by its Yes button.
	ShowQuitConfirm bool
	QuitRequested   bool
	RegionHint      string        // shown when a region's panel is hidden in the current mode
	Diag            *diag.Session // diagnostics session of the active device (nil without one)
	Firmware        FirmwareView  // read-only firmware update check of the active device
	// Compat is the Driver & Compatibility analysis of the active device;
	// CompatReview names the action awaiting explicit approval ("" = none)
	// and CompatStatus reports the last action's outcome.
	Compat         compat.Report
	CompatReview   string
	CompatBusy     bool
	CompatStatus   string
	deviceConfigs  map[string]device.Config
	activeProfiles map[string]string // device family -> active profile name

	// 3D Stage, Component & Region Selection
	SelectedRegion    device.RegionID
	HoveredRegion     device.RegionID
	SelectedComponent string // visual.Component ID on the stage model
	HoveredComponent  string
	Gallery           GalleryState
	// HardwareModels are the static descriptors of real hardware drivers,
	// keyed by ModelID (the gallery uses them to state actual support).
	HardwareModels map[string]device.Descriptor
	RotX           float64
	RotY           float64
	RotZ           float64
	// TargetRotX/Y and CameraTween drive smooth front/rear transitions.
	TargetRotX, TargetRotY float64
	CameraTween            bool
	// StageClick remembers the last stage click for double-click detection.
	StageClick struct {
		At   time.Time
		X, Y uint16
	}
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
		SelectedRegion:   device.RegionNone,
		Zoom:             1.0,
		AutoRotate:       false,
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
	s.ResetCamera()
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
	if key != s.ActiveDeviceKey {
		s.IncludeCopyIdentifiers = false
	}
	modelChanged := desc.VisualID != s.Descriptor.VisualID
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
	s.SelectedRegion = device.RegionNone
	s.HoveredRegion = device.RegionNone
	s.RegionHint = ""
	if !s.Gallery.Active {
		s.SelectedComponent, s.HoveredComponent = "", ""
		if modelChanged {
			s.ResetCamera()
		}
	}
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
			s.Diag.Select(c)
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

// IsSimulatedDevice reports the source of the active selection, never its model.
func (s *AppState) IsSimulatedDevice() bool {
	for _, d := range s.Devices {
		if d.Key == s.ActiveDeviceKey {
			return d.Source != device.SourceHardware
		}
	}
	return false
}
