package app

import (
	"context"
	"fmt"
	"github.com/Phobby/TuiSense/internal/clipboard"
	"github.com/Phobby/TuiSense/internal/compat"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/update"
	"github.com/Phobby/TuiSense/internal/version"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/audio"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/logging"
	"github.com/Phobby/TuiSense/internal/session"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/system"
	"github.com/Phobby/TuiSense/internal/theme"
	"github.com/Phobby/TuiSense/internal/ui/views"
	"github.com/thebanri/limoni"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// Options configures the TuiSense Application instance.
type Options struct {
	TestDevices   bool // explicitly expose architecture test devices
	ForceMock     bool
	AllowMock     bool
	NoTestDevices bool   // hide UI/architecture test devices (simulated Xbox and DualSense)
	ProfileDir    string // default: $XDG_CONFIG_HOME/tuisense/profiles
	ConfigDir     string // settings.json location; default: $XDG_CONFIG_HOME/tuisense
	TargetFPS     int
	// Registry overrides the built-in driver registry (used by tests to run
	// without touching real hardware).
	Registry *device.Registry
	// AudioBackend overrides the platform audio backend for real devices
	// (nil = ALSA on Linux, unsupported elsewhere).
	AudioBackend audio.Backend
	ReportDir    string // diagnostics reports; default $XDG_DATA_HOME/tuisense/reports
	Version      string
	// Language forces the UI language ("en"/"tr"); empty = saved choice, then $LANG.
	Language string
	// FirmwareProviders overrides the read-only firmware checkers (tests use
	// mock metadata; nil = Sony's published metadata over HTTPS).
	FirmwareProviders []firmware.Provider
	// OpenURL and CopyText override the browser / clipboard adapters (tests).
	OpenURL   func(url string) error
	CopyText  func(text string) error
	Clipboard clipboard.ClipboardService
	// CompatEnv and CompatRunner override the Driver & Compatibility system
	// access (tests); nil = the real, read-only system view and exec runner.
	CompatEnv    compat.Env
	CompatRunner compat.Runner
	// UpdateSource overrides the application update server (tests use a
	// local mock); Updater replaces the whole manager.
	UpdateSource *update.Source
	Updater      *update.Manager
}

// Application coordinates AppState, the device session, the dispatcher, and the Limoni render loop.
type Application struct {
	State             *state.AppState
	Logger            *logging.Logger
	Manager           *session.Manager
	Dispatcher        *session.Dispatcher
	Prefs             *state.PrefsStore
	Opts              Options
	lastFrame         time.Time
	copyResults       chan copyCompletion
	copying           bool
	diagnosticBusy    bool
	diagnosticResults chan diagnosticCompletion
	compatResults     chan string
	// Updater is the application (not firmware) update manager.
	Updater       *update.Manager
	updateSeen    update.Status
	updateChecked time.Time
	updateLatest  string
	// escAt is the last Esc press (see escHeld); escFix and escFixAt track a
	// mouse report broken by a held Esc (see repairKey).
	escAt    time.Time
	escFix   escRepair
	escFixAt time.Time

	eventsMu sync.Mutex
	events   []session.DeviceEvent

	diagSessions map[string]*diag.Session
	simAudio     map[string]*audio.Simulated
	prevPanel    state.PanelID
	audioProbeAt time.Time
	// confirmed holds the user's controller identifications (hardware key → model ID).
	confirmed map[string]string

	// Firmware runs read-only firmware update checks off the UI goroutine.
	Firmware    *firmware.Service
	fwResults   map[string]firmware.Result // latest result per device key
	fwQueue     []firmware.Result          // finished checks (guarded by eventsMu)
	fwAutoCheck bool
	ctx         context.Context
	started     bool // Run is active (automatic network checks allowed)
}

// appActuator lets diagnostics drive hardware through the dispatcher and read
// (never modify) the user's current configuration for safe restoration.
type appActuator struct{ a *Application }

func (x appActuator) Dispatch(cmd device.Command)  { x.a.Dispatcher.Dispatch(cmd) }
func (x appActuator) CurrentConfig() device.Config { return x.a.State.Config }

// New constructs and wires a ready-to-run TuiSense Application.
func New(opts Options) *Application {
	if opts.TargetFPS <= 0 {
		opts.TargetFPS = 30
	}
	if opts.ConfigDir == "" {
		opts.ConfigDir = system.ConfigDir()
	}
	if opts.Version == "" {
		opts.Version = version.String()
	}
	logger := logging.New(500)
	s := state.NewAppState(opts.ProfileDir, logger)

	a := &Application{
		State:        s,
		Logger:       logger,
		Prefs:        state.NewPrefsStore(opts.ConfigDir),
		Opts:         opts,
		lastFrame:    time.Now(),
		diagSessions: map[string]*diag.Session{},
		simAudio:     map[string]*audio.Simulated{},
	}
	if opts.AudioBackend == nil && runtime.GOOS == "linux" {
		a.Opts.AudioBackend = audio.NewALSA()
	}

	prefs, err := a.Prefs.Load()
	if err != nil {
		logger.Warn("PREFS", "Ignoring unreadable %s: %v", a.Prefs.Path(), err)
	}
	s.Mode = state.ParseMode(prefs.Mode)
	lang, ok := i18n.Parse(opts.Language)
	if !ok {
		if lang, ok = i18n.Parse(prefs.Language); !ok {
			lang = i18n.Detect()
		}
	}
	s.Language = lang
	i18n.Set(lang)
	if prefs.HighContrast {
		s.HighContrast = true
		s.Theme = theme.HighContrast()
	}
	if prefs.AutoRotate != nil {
		s.AutoRotate = *prefs.AutoRotate
	}
	s.RestoreActiveProfiles(prefs.ActiveProfiles)
	providers := opts.FirmwareProviders
	if providers == nil {
		providers = defaultFirmwareProviders(opts.Version)
	}
	a.Firmware = firmware.NewService(firmware.NewStore(opts.ConfigDir), providers...)
	a.fwResults = map[string]firmware.Result{}
	a.fwAutoCheck = prefs.FirmwareAutoCheck == nil || *prefs.FirmwareAutoCheck
	s.AppVersion = opts.Version
	a.Updater = opts.Updater
	if a.Updater == nil {
		a.Updater = newUpdater(opts, prefs)
	}
	a.Updater.OnChange = limoni.Wakeup
	a.updateChecked, a.updateLatest = prefs.UpdateLastCheck, prefs.UpdateLatest
	s.Update = a.Updater.Snapshot()
	a.updateSeen = s.Update.Status
	a.confirmed = map[string]string{}
	for k, v := range prefs.ConfirmedModels {
		a.confirmed[k] = v
	}
	gamepad.SetOverrides(a.confirmed)

	reg := opts.Registry
	if reg == nil {
		reg = drivers.NewBuiltinRegistry(drivers.RegistryOptions{Hardware: true, Mock: opts.ForceMock || opts.AllowMock, TestDevices: opts.TestDevices && !opts.NoTestDevices})
	}
	// The gallery states real support from the built-in hardware drivers'
	// static descriptors — independent of what is connected or registered.
	s.HardwareModels = map[string]device.Descriptor{}
	for _, drv := range drivers.NewBuiltinRegistry(drivers.RegistryOptions{Hardware: true}).Drivers() {
		for _, d := range drv.Models() {
			s.HardwareModels[d.ModelID] = d
		}
	}

	a.Manager = session.NewManager(reg, session.Options{
		ForceMock:    opts.ForceMock,
		AllowMock:    opts.AllowMock,
		PreferredKey: prefs.LastDevice,
	}, logger, func(ev session.DeviceEvent) {
		// Device events arrive on the session goroutine; they are applied to
		// AppState on the render goroutine at the start of the next frame.
		a.eventsMu.Lock()
		a.events = append(a.events, ev)
		a.eventsMu.Unlock()
		limoni.Wakeup()
	})

	a.Dispatcher = session.NewDispatcher(a.Manager, logger, func(_ device.Command, err error) {
		if err != nil {
			a.eventsMu.Lock()
			a.events = append(a.events, session.DeviceEvent{Kind: session.EventError, Err: err})
			a.eventsMu.Unlock()
		}
		limoni.Wakeup()
	})

	a.copyResults = make(chan copyCompletion, 1)
	a.diagnosticResults = make(chan diagnosticCompletion, 1)
	a.compatResults = make(chan string, 1)
	if a.Opts.CompatEnv == nil {
		a.Opts.CompatEnv = compat.SystemEnv{}
	}
	if a.Opts.CompatRunner == nil {
		a.Opts.CompatRunner = compat.ExecRunner{}
	}
	s.Copy.OnCopy = a.CopyText
	a.syncActiveDevice()
	return a
}

// drainEvents applies queued session events to AppState (render goroutine only).
func (a *Application) drainEvents() {
	a.eventsMu.Lock()
	evs := a.events
	a.events = nil
	a.eventsMu.Unlock()
	for _, ev := range evs {
		a.onDeviceEvent(ev)
	}
}

func (a *Application) onDeviceEvent(ev session.DeviceEvent) {
	s := a.State
	switch ev.Kind {
	case session.EventConnected:
		a.syncActiveDevice()
		s.ClearError()

	case session.EventDisconnected:
		a.syncActiveDevice()
		if s.Toasts != nil {
			s.Toasts.Warning(i18n.T("Controller Disconnected"), i18n.T("Waiting for hot-plug..."))
		}
	case session.EventSelected, session.EventListChanged:
		a.syncActiveDevice()
	case session.EventTelemetry:
		if ev.Key == s.ActiveDeviceKey {
			s.DeviceInfo = ev.Info
			s.Telemetry = ev.Telemetry
			if s.Diag != nil {
				s.Diag.Feed(ev.Telemetry, ev.Info, time.Now())
			}
		}
	case session.EventError:
		if ev.Err != nil {
			s.SetError(i18n.T("Device Write Error"), ev.Err.Error())
		}
	}
}

// syncActiveDevice refreshes the device list and switches the UI context to
// the session's active device (or the placeholder when there is none).
func (a *Application) syncActiveDevice() {
	s := a.State
	summaries := a.Manager.Devices()
	s.Devices = make([]state.DeviceChoice, len(summaries))
	for i, d := range summaries {
		s.Devices[i] = state.DeviceChoice{Key: d.Key, Descriptor: d.Descriptor, Source: d.Source, Connected: d.Connected, Active: d.Active}
	}

	key := a.Manager.ActiveKey()
	dev := a.Manager.Active()
	if key != s.ActiveDeviceKey && s.Diag != nil {
		// Leaving a device: stop every diagnostic effect on it first.
		s.Diag.Suspend()
		if dev == nil {
			s.Diag.Conn.MarkDisconnected()
		}
	}
	defer a.refreshFirmware()
	defer a.refreshCompat()
	if dev == nil {
		s.Diag = nil
		s.ActivateDevice("", device.PlaceholderDescriptor(), device.NewCapabilitySet(),
			device.Info{Name: i18n.T("No Controller Detected"), Connection: device.ConnDisconnected}, device.Telemetry{}, s.Config)
		s.ActiveDeviceKey = ""
		return
	}

	if key == s.ActiveDeviceKey {
		s.DeviceInfo = dev.Info()
		s.Telemetry = dev.Telemetry()
		s.Caps = dev.Capabilities()
		s.EnsureActivePanelVisible()
		return
	}
	defer a.maybeAutoCheck()

	firstUse := !s.HasDeviceConfig(key)
	desc := dev.Descriptor()
	cfg, profile := a.initialConfig(dev)
	s.ActivateDevice(key, desc, dev.Capabilities(), dev.Info(), dev.Telemetry(), cfg)
	if firstUse {
		s.ActiveProfile = profile
		_ = dev.ApplyConfig(context.Background(), s.Config)
	}
	s.Diag = a.diagSession(key, dev)
}

// isSimulated reports whether the device under key is a mock or test device.
func (a *Application) isSimulated(key string) bool {
	for _, d := range a.State.Devices {
		if d.Key == key {
			return d.Source != device.SourceHardware
		}
	}
	return false
}

// diagContext describes the device for diagnostics, including a fresh probe
// for the controller's own audio endpoint.
func (a *Application) diagContext(key string, dev device.Device) diag.DeviceContext {
	desc := dev.Descriptor()
	caps := dev.Capabilities()
	dc := diag.DeviceContext{Key: key, Desc: desc, Caps: caps, Info: dev.Info(), Simulated: a.isSimulated(key)}
	backend := a.audioBackendFor(key, dc.Simulated, caps, desc)
	if backend == nil {
		dc.Audio.Err = fmt.Errorf("controller audio diagnostics are not implemented on %s yet", runtime.GOOS)
		return dc
	}
	dc.Audio.Endpoint, dc.Audio.Found, dc.Audio.Err = audio.FindControllerEndpoint(backend, desc.Inputs.Audio)
	return dc
}

// audioBackendFor returns the platform backend for hardware, or an in-memory
// simulated backend for mock/test devices (clearly flagged as simulated).
func (a *Application) audioBackendFor(key string, simulated bool, caps device.CapabilitySet, desc device.Descriptor) audio.Backend {
	if !simulated {
		return a.Opts.AudioBackend
	}
	if b, ok := a.simAudio[key]; ok {
		return b
	}
	capture, playback := 0, 0
	if caps.HasFeature(device.CapAudio, device.FeatMicrophone) {
		capture = 1
	}
	if caps.HasFeature(device.CapAudio, device.FeatSpeaker) || caps.HasFeature(device.CapAudio, device.FeatHeadset) {
		playback = 2
	}
	b := audio.NewSimulated(desc.ShortName+i18n.T(" audio"), capture, playback)
	a.simAudio[key] = b
	return b
}

// diagSession returns (creating on first use) the diagnostics session of a device.
func (a *Application) diagSession(key string, dev device.Device) *diag.Session {
	if sess, ok := a.diagSessions[key]; ok {
		sess.SetContext(a.diagContext(key, dev))
		return sess
	}
	dc := a.diagContext(key, dev)
	sim, _ := dev.(device.ScenarioSimulator)
	bat, _ := dev.(device.BatteryDetailsProvider)
	sess := diag.NewSession(dc, appActuator{a}, sim, bat, a.audioBackendFor(key, dc.Simulated, dc.Caps, dc.Desc), time.Now())
	sess.OnChange = limoni.Wakeup
	sess.SoftwareVersion = a.Opts.Version
	a.diagSessions[key] = sess
	return sess
}

// tickDiagnostics advances diagnostics timers, stops effects when the user
// leaves the Diagnostics tab, and re-probes audio while the tab is open.
func (a *Application) tickDiagnostics(now time.Time) {
	a.drainDiagnosticStorage()
	s := a.State
	if a.prevPanel == state.PanelDiagnostics && s.ActivePanel != state.PanelDiagnostics && s.Diag != nil {
		s.Diag.Suspend()
	}
	a.prevPanel = s.ActivePanel
	if s.Diag == nil {
		return
	}
	s.Diag.Tick(now)
	if s.ActivePanel == state.PanelDiagnostics && !s.Diag.Ctx.Simulated && !s.Diag.Ctx.Audio.Found && now.Sub(a.audioProbeAt) > 3*time.Second {
		a.audioProbeAt = now
		if dev := a.Manager.Active(); dev != nil {
			s.Diag.SetContext(a.diagContext(s.ActiveDeviceKey, dev))
		}
	}
}

// suspendAllDiagnostics stops effects on every device (quit / shutdown).
func (a *Application) suspendAllDiagnostics() {
	for _, sess := range a.diagSessions {
		sess.Suspend()
	}
}

// ExportDiagnosticsReport writes JSON + Markdown reports for the active device.
func (a *Application) ExportDiagnosticsReport() {
	a.diagnosticStorage("export")
}

// initialConfig resolves the first config for a device: MAC-bound profile →
// remembered profile for its family → descriptor default profile → factory config.
func (a *Application) initialConfig(dev device.Device) (device.Config, string) {
	s := a.State
	desc := dev.Descriptor()
	if mac := dev.Info().MACAddress; mac != "" {
		if bound, ok := s.ProfileMgr.BoundProfile(mac); ok {
			a.Logger.Info("PROFILES", "Auto-loaded bound profile %q for MAC %s", bound.Name, mac)
			return bound.Config, bound.Name
		}
	}
	for _, name := range []string{s.ActiveProfiles()[desc.Family], desc.DefaultProfile} {
		if name == "" {
			continue
		}
		if p, ok := s.ProfileMgr.Get(name); ok && (p.Family == "" || p.Family == desc.Family) {
			return p.Config, p.Name
		}
	}
	return desc.NewDefaultConfig(), "Factory Defaults"
}

// savePrefs persists mode, selected device, and per-family active profiles.
func (a *Application) savePrefs() {
	s := a.State
	auto := s.AutoRotate
	p := state.Preferences{
		Mode:           string(s.Mode),
		Language:       string(s.Language),
		LastDevice:     s.ActiveDeviceKey,
		ActiveProfiles: s.ActiveProfiles(),
		HighContrast:   s.HighContrast,
		AutoRotate:     &auto,
	}
	if !a.fwAutoCheck {
		off := false
		p.FirmwareAutoCheck = &off
	}
	if len(a.confirmed) > 0 {
		p.ConfirmedModels = a.confirmed
	}
	us := a.Updater.Snapshot().Settings
	if !us.Check {
		off := false
		p.UpdateChecks = &off
	}
	p.UpdateAutoDownload = us.AutoDownload
	if us.Channel == update.Prerelease {
		p.UpdateChannel = string(update.Prerelease)
	}
	p.UpdateLastCheck = a.updateChecked
	p.UpdateLatest = a.updateLatest
	if err := a.Prefs.Save(p); err != nil {
		a.Logger.Warn("PREFS", "Could not save %s: %v", a.Prefs.Path(), err)
	}
}

// SelectDevice makes key the active device and switches the whole UI to it.
func (a *Application) SelectDevice(key string) {
	if key == a.State.ActiveDeviceKey {
		return
	}
	if a.State.Diag != nil {
		a.State.Diag.Suspend() // cleanup commands still target the outgoing device
	}
	if err := a.Manager.Select(key); err != nil {
		a.State.SetError(i18n.T("Device Selection"), err.Error())
		return
	}
	a.syncActiveDevice()
	a.savePrefs()
}

// CycleDevice selects the previous/next attached device.
func (a *Application) CycleDevice(delta int) {
	if a.State.Diag != nil {
		a.State.Diag.Suspend()
	}
	if err := a.Manager.SelectNext(delta); err != nil {
		return
	}
	a.syncActiveDevice()
	a.savePrefs()
}

// SetLanguage switches the UI language and persists the choice.
func (a *Application) SetLanguage(l i18n.Lang) {
	if a.State.Language == l {
		return
	}
	a.State.Language = l
	i18n.Set(l)
	a.savePrefs()
	a.refreshFirmware()
	if a.State.Toasts != nil {
		a.State.Toasts.Info(i18n.T("Language"), l.Name())
	}
}

// ConfirmIdentity remembers the user's model choice for a hardware key
// (modelID "" forgets it) and asks the session to re-detect devices, which
// re-opens the controller with the chosen model's descriptor.
func (a *Application) ConfirmIdentity(hwKey, modelID string) {
	if hwKey == "" {
		return
	}
	if modelID == "" {
		delete(a.confirmed, hwKey)
	} else {
		a.confirmed[hwKey] = modelID
	}
	gamepad.SetOverride(hwKey, modelID)
	a.savePrefs()
	a.Logger.Info("IDENTITY", "Identification for %s set to %q", hwKey, modelID)
	if a.State.Toasts != nil {
		a.State.Toasts.Info(i18n.T("Identification saved"), i18n.T("Re-detecting the controller…"))
	}
	a.Manager.Rescan()
}

// SetMode switches EZ/Advanced and persists the choice.
func (a *Application) SetMode(m state.UXMode) {
	if a.State.Mode == m {
		return
	}
	a.State.SetMode(m)
	a.savePrefs()
}

// simulator returns the active device's simulator, enabling the mock DualSense
// when no device is active. ok=false means the active device is real hardware.
func (a *Application) simulator() (device.Simulator, bool) {
	if sim := a.Manager.ActiveSimulator(); sim != nil {
		return sim, true
	}
	if a.Manager.Active() == nil {
		a.Manager.EnableMock()
		a.drainEvents()
		a.syncActiveDevice()
		return a.Manager.ActiveSimulator(), true
	}
	if a.State.Toasts != nil {
		a.State.Toasts.Warning(i18n.T("Simulation"), i18n.T("Simulation controls only apply to mock and test devices"))
	}
	return nil, false
}

func (a *Application) refreshActive() {
	defer a.refreshFirmware()
	defer a.refreshCompat()
	if dev := a.Manager.Active(); dev != nil {
		a.State.DeviceInfo = dev.Info()
		a.State.Telemetry = dev.Telemetry()
		a.State.Caps = dev.Capabilities()
		a.State.EnsureActivePanelVisible()
		if a.State.Diag != nil {
			a.State.Diag.Feed(a.State.Telemetry, a.State.DeviceInfo, time.Now())
			if a.State.Diag.Ctx.Info.Connection != a.State.DeviceInfo.Connection || !a.State.Diag.Ctx.Simulated {
				a.State.Diag.SetContext(a.diagContext(a.State.ActiveDeviceKey, dev))
			}
		}
	}
}

// pumpTelemetry polls the active device once and feeds the result through the
// normal event path (used by tests; the session goroutine does this at 25 Hz).
func (a *Application) pumpTelemetry() {
	dev := a.Manager.Active()
	if dev == nil {
		return
	}
	tel, err := dev.Poll(context.Background())
	if err == nil {
		a.onDeviceEvent(session.DeviceEvent{Kind: session.EventTelemetry, Key: a.Manager.ActiveKey(), Info: dev.Info(), Telemetry: tel})
	}
}

// ToggleSimulatedConnection toggles the active simulated device's connection (or activates the mock).
func (a *Application) ToggleSimulatedConnection() {
	wasEmpty := a.Manager.Active() == nil
	sim, ok := a.simulator()
	if !ok || sim == nil {
		return
	}
	if !wasEmpty {
		sim.ToggleConnection()
	}
	a.refreshActive()
	if a.State.Toasts != nil {
		if a.State.DeviceInfo.Connected {
			a.State.Toasts.Success(i18n.T("Hot-Plug Connected"), i18n.Tf("Attached %s (%s)", i18n.T(a.State.Descriptor.ShortName), i18n.T(string(a.State.DeviceInfo.Connection))))
		} else {
			a.State.Toasts.Warning(i18n.T("Hot-Plug Disconnected"), i18n.T("Controller unplugged (empty state active)"))
		}
	}
}

// CycleSimulatedTransport switches the simulated device's transport.
func (a *Application) CycleSimulatedTransport() {
	sim, ok := a.simulator()
	if !ok || sim == nil {
		return
	}
	conn := sim.CycleTransport()
	a.refreshActive()
	if a.State.Toasts != nil {
		a.State.Toasts.Info(i18n.T("Transport Changed"), i18n.Tf("Switched to %s", i18n.T(string(conn))))
	}
}

// ToggleSimulatedPermission toggles a simulated udev permission error.
func (a *Application) ToggleSimulatedPermission() {
	sim, ok := a.simulator()
	if !ok || sim == nil {
		return
	}
	denied := sim.TogglePermissionError()
	a.refreshActive()
	if denied {
		a.State.SetError(i18n.T("Permission Denied"), a.State.DeviceInfo.PermissionHint)
	} else {
		a.State.ClearError()
	}
}

// TestPulse fires the active device's test-vibration action, if it has one.
func (a *Application) TestPulse() {
	s := a.State
	for _, act := range s.Descriptor.Actions {
		if act.ID == "pulse_test" && s.Caps.Supports(act.Capability) {
			a.Dispatcher.Dispatch(act.Command)
			if s.Toasts != nil {
				s.Toasts.Info(i18n.T(act.Label), i18n.Tf("%s pulsed", i18n.T(s.Descriptor.ShortName)))
			}
			return
		}
	}
}

func (a *Application) callbacks() views.Callbacks {
	return views.Callbacks{
		OnToggleConnect:            a.ToggleSimulatedConnection,
		OnCycleTransport:           a.CycleSimulatedTransport,
		OnTogglePermission:         a.ToggleSimulatedPermission,
		OnSelectDevice:             a.SelectDevice,
		OnCycleDevice:              a.CycleDevice,
		OnSetMode:                  a.SetMode,
		OnPrefsChanged:             a.savePrefs,
		OnExportReport:             a.ExportDiagnosticsReport,
		OnSaveDiagnosticHistory:    func() { a.diagnosticStorage("save") },
		OnCompareDiagnosticHistory: func() { a.diagnosticStorage("compare") },
		OnClearDiagnosticHistory:   func() { a.diagnosticStorage("clear") },
		OnRetestMapping:            a.RetestInputMapping,
		OnReviewDiagnosticHistory:  func() { a.diagnosticStorage("review") },
		OnOlderDiagnosticHistory:   func() { a.diagnosticStorage("older") },
		OnNewerDiagnosticHistory:   func() { a.diagnosticStorage("newer") },
		OnSetLanguage:              a.SetLanguage,
		OnConfirmIdentity:          a.ConfirmIdentity,

		OnCheckFirmware:        a.CheckFirmware,
		OnCheckDrivers:         a.refreshCompat,
		OnCheckUpdates:         a.CheckForUpdates,
		OnDownloadUpdate:       a.DownloadUpdate,
		OnSetUpdateSettings:    a.SetUpdateSettings,
		OnRunCompatAction:      a.RunCompatAction,
		OnSetFirmwareAutoCheck: a.SetFirmwareAutoCheck,
		OnSetFirmwareSection:   a.SetFirmwareSection,
		OnSetDetailGroup:       a.SetDetailGroup,
		OnSelectDetail:         a.SelectDetail,
		OnOpenLink:             a.OpenOfficialLink,
		OnCopyLink:             a.CopyOfficialLink,
	}
}

// Draw is the immediate-mode Limoni frame callback compatible with both
// limoni.RunWithContext and headless uitest.Run.
func (a *Application) Draw(f *terminal.Frame, ev *driver.Event) bool {
	a.drainEvents()
	a.drainFirmware()
	a.drainCompat()
	a.syncUpdater()
	a.drainCopies()

	now := time.Now()
	dt := now.Sub(a.lastFrame).Seconds()
	if dt <= 0 || dt > 0.25 {
		dt = 1.0 / 30.0
	}
	a.lastFrame = now

	if ev != nil && ev.Type == driver.EventKey {
		if key, ok := a.repairKey(ev.Key, now); ok && !a.HandleKeyEvent(f, key) {
			return false
		}
	}
	if a.State.QuitRequested {
		return false
	}

	a.tickDiagnostics(now)

	s := a.State
	s.StepCamera(dt)
	if s.AutoRotate && (s.DeviceInfo.Connected || s.Gallery.Active) {
		// Sway around whichever side (front or rear) is being inspected.
		base := s.StageSpec().Front
		if s.ViewingRear() {
			base = s.StageSpec().Rear
		}
		s.ShowcasePhase += s.AutoRotateSpeed * dt
		s.RotY = base.RotY + 22.0*math.Sin(s.ShowcasePhase)
		s.RotX = base.RotX + 5.0*math.Cos(s.ShowcasePhase*0.7)
	}

	views.RenderDashboard(f, a.State, a.Dispatcher.Dispatch, a.callbacks())
	a.tickDiagnostics(now) // catch panel changes made by clicks during this frame
	return true
}

// Run starts the background session and dispatcher workers and launches the
// full-screen Limoni terminal UI.
func (a *Application) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.ctx, a.started = runCtx, true
	a.maybeAutoCheck()
	// The application update check runs in the background after the UI is
	// up; offline or slow networks never delay start-up.
	a.Updater.CheckAsync(false)

	go a.Manager.Start(runCtx)
	go a.Dispatcher.Start(runCtx)

	err := limoni.RunWithContext(
		runCtx,
		a.Draw,
		limoni.WithTitle("TuiSense — Controller Control Center"),
		limoni.WithFPS(a.Opts.TargetFPS),
		limoni.WithSuspend(),
	)
	// Never leave vibration, trigger resistance, or audio running on exit.
	a.suspendAllDiagnostics()
	a.Dispatcher.FlushNow()
	return err
}

// refreshCompat re-runs the read-only Driver & Compatibility analysis for
// the active controller (on connect, transport change, or on request).
func (a *Application) refreshCompat() {
	s := a.State
	dev := a.Manager.Active()
	if dev == nil {
		s.Compat, s.CompatReview = compat.Report{}, ""
		return
	}
	s.Compat = compat.Analyze(a.Opts.CompatEnv, compat.Input{Desc: dev.Descriptor(), Caps: dev.Capabilities(), Info: dev.Info(), Simulated: a.isSimulated(s.ActiveDeviceKey)})
	if _, ok := s.Compat.Action(s.CompatReview); !ok {
		s.CompatReview = ""
	}
}

// RunCompatAction executes an allowlisted remedy. It runs only for the action
// the user is currently reviewing (the explicit approval step), never at
// startup, and off the UI goroutine because polkit asks for a password.
func (a *Application) RunCompatAction(id string) {
	s := a.State
	if s.CompatBusy || s.CompatReview != id {
		return
	}
	act, ok := s.Compat.Action(id)
	if !ok || act.Manual {
		return
	}
	if id == compat.ActionUdevDualSense {
		if err := compat.StageRule(); err != nil {
			s.CompatStatus = err.Error()
			return
		}
	}
	s.CompatBusy = true
	a.Logger.Info("COMPAT", "Running approved action %s", id)
	env, run := a.Opts.CompatEnv, a.Opts.CompatRunner
	go func() {
		err := compat.Execute(env, run, act, true, func() bool { return env.Exists(compat.UdevRulePath) })
		msg := "Done. Reconnect the controller to apply the change."
		if err != nil {
			msg = "The action did not complete: " + err.Error()
		}
		a.compatResults <- msg
		limoni.Wakeup()
	}()
}

func (a *Application) drainCompat() {
	select {
	case msg := <-a.compatResults:
		a.State.CompatBusy = false
		a.State.CompatStatus = msg
		a.State.CompatReview = ""
		a.Logger.Info("COMPAT", "%s", msg)
		a.refreshCompat()
	default:
	}
}

func newUpdater(opts Options, prefs state.Preferences) *update.Manager {
	settings := update.Settings{
		Check:        prefs.UpdateChecks == nil || *prefs.UpdateChecks,
		AutoDownload: prefs.UpdateAutoDownload,
		Channel:      update.Stable,
	}
	if prefs.UpdateChannel == string(update.Prerelease) {
		settings.Channel = update.Prerelease
	}
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
	}
	m := update.NewManager(opts.Version, settings, prefs.UpdateLastCheck, filepath.Join(system.DataDir(), "updates"), exe)
	m.RestoreLatest(prefs.UpdateLatest)
	if opts.UpdateSource != nil {
		m.Source = *opts.UpdateSource
	}
	return m
}

// syncUpdater mirrors the updater into AppState and announces real state
// changes only: a release found, an update downloaded and verified, or a
// discarded download. It never claims an update is installed.
func (a *Application) syncUpdater() {
	s := a.State
	snap := a.Updater.Snapshot()
	s.Update = snap
	if snap.CheckedAt.After(a.updateChecked) {
		a.updateChecked = snap.CheckedAt
		if snap.Latest != "" {
			a.updateLatest = snap.Latest
		}
		a.savePrefs()
	}
	if snap.Status == a.updateSeen {
		return
	}
	a.updateSeen = snap.Status
	if s.Toasts == nil {
		return
	}
	switch snap.Status {
	case update.StatusAvailable:
		s.Toasts.Info(i18n.T("Update"), i18n.T("A new TuiSense version is available."))
	case update.StatusReady:
		s.Toasts.Success(i18n.T("Update"), i18n.T("A new TuiSense version is ready. It will be applied when you restart the app."))
	case update.StatusFailed:
		s.Toasts.Warning(i18n.T("Update"), i18n.T("The update could not be verified and was discarded. The installed version is unchanged."))
	}
}

// CheckForUpdates runs a manual application update check.
func (a *Application) CheckForUpdates() { a.Updater.CheckAsync(true) }

// DownloadUpdate stages the update found by the last check.
func (a *Application) DownloadUpdate() { a.Updater.DownloadAsync() }

// SetUpdateSettings changes and persists the update preferences.
func (a *Application) SetUpdateSettings(us update.Settings) {
	a.Updater.SetSettings(us)
	a.State.Update = a.Updater.Snapshot()
	a.savePrefs()
}

// ApplyStagedUpdate installs a verified, staged update after the UI exited
// normally. It returns a message for the terminal ("" = nothing to do).
func (a *Application) ApplyStagedUpdate() string {
	installed, err := a.Updater.ApplyPending(update.RunVersion)
	switch {
	case err != nil:
		return "TuiSense update was not applied: " + err.Error() + "\nThe installed version is unchanged."
	case installed != "":
		return "TuiSense was updated to v" + installed + ". The previous version is kept for recovery (tuisense update --rollback)."
	}
	return ""
}
