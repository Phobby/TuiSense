package app

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/audio"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/drivers"
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
	ForceMock     bool
	AllowMock     bool
	NoTestDevices bool   // hide UI/architecture test devices (mock Xbox)
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
}

// Application coordinates AppState, the device session, the dispatcher, and the Limoni render loop.
type Application struct {
	State      *state.AppState
	Logger     *logging.Logger
	Manager    *session.Manager
	Dispatcher *session.Dispatcher
	Prefs      *state.PrefsStore
	Opts       Options
	lastFrame  time.Time

	eventsMu sync.Mutex
	events   []session.DeviceEvent

	diagSessions map[string]*diag.Session
	simAudio     map[string]*audio.Simulated
	prevPanel    state.PanelID
	audioProbeAt time.Time
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
	if prefs.HighContrast {
		s.HighContrast = true
		s.Theme = theme.HighContrast()
	}
	if prefs.AutoRotate != nil {
		s.AutoRotate = *prefs.AutoRotate
	}
	s.RestoreActiveProfiles(prefs.ActiveProfiles)

	reg := opts.Registry
	if reg == nil {
		reg = drivers.NewBuiltinRegistry(drivers.RegistryOptions{Hardware: true, TestDevices: !opts.NoTestDevices})
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
		if s.Toasts != nil && ev.Key == s.ActiveDeviceKey {
			s.Toasts.Success("Controller Connected", fmt.Sprintf("%s (%s)", s.Descriptor.ShortName, ev.Info.Connection))
		}
	case session.EventDisconnected:
		a.syncActiveDevice()
		if s.Toasts != nil {
			s.Toasts.Warning("Controller Disconnected", "Waiting for hot-plug...")
		}
	case session.EventSelected:
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
			s.SetError("Device Write Error", ev.Err.Error())
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
	if dev == nil {
		s.Diag = nil
		s.ActivateDevice("", device.PlaceholderDescriptor(), device.NewCapabilitySet(),
			device.Info{Name: "No Controller Detected", Connection: device.ConnDisconnected}, device.Telemetry{}, s.Config)
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
	b := audio.NewSimulated(desc.ShortName+" audio", capture, playback)
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
	a.diagSessions[key] = sess
	return sess
}

// tickDiagnostics advances diagnostics timers, stops effects when the user
// leaves the Diagnostics tab, and re-probes audio while the tab is open.
func (a *Application) tickDiagnostics(now time.Time) {
	s := a.State
	if a.prevPanel == state.PanelDiagnostics && s.ActivePanel != state.PanelDiagnostics && s.Diag != nil {
		s.Diag.Suspend()
	}
	a.prevPanel = s.ActivePanel
	if s.Diag == nil {
		return
	}
	if st := s.Diag.Sticks; st != nil {
		// Read-only context for interpreting drift; diagnostics never change it.
		st.Deadzone = [2]float64{float64(s.Config.Sticks.Left.InnerDeadzone), float64(s.Config.Sticks.Right.InnerDeadzone)}
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

// ExportDiagnosticsReport writes JSON + text reports for the active device.
func (a *Application) ExportDiagnosticsReport() {
	s := a.State
	if s.Diag == nil {
		return
	}
	dir := a.Opts.ReportDir
	if dir == "" {
		dir = filepath.Join(system.DataDir(), "reports")
	}
	r := diag.BuildReport(s.Diag, a.Opts.Version, false, time.Now())
	jp, _, err := r.Export(dir)
	if err != nil {
		s.SetError("Report Export", err.Error())
		return
	}
	a.Logger.Info("DIAG", "Report written to %s (+ .txt)", jp)
	if s.Toasts != nil {
		s.Toasts.Success("Report saved", jp)
	}
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
		LastDevice:     s.ActiveDeviceKey,
		ActiveProfiles: s.ActiveProfiles(),
		HighContrast:   s.HighContrast,
		AutoRotate:     &auto,
	}
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
		a.State.SetError("Device Selection", err.Error())
		return
	}
	a.syncActiveDevice()
	a.savePrefs()
	if a.State.Toasts != nil {
		a.State.Toasts.Info("Controller", "Switched to "+a.State.Descriptor.DisplayName)
	}
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

// SetMode switches EZ/Advanced and persists the choice.
func (a *Application) SetMode(m state.UXMode) {
	if a.State.Mode == m {
		return
	}
	a.State.SetMode(m)
	a.savePrefs()
	if a.State.Toasts != nil {
		a.State.Toasts.Info("Mode", m.Label()+" mode")
	}
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
		a.State.Toasts.Warning("Simulation", "Simulation controls only apply to mock and test devices")
	}
	return nil, false
}

func (a *Application) refreshActive() {
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
			a.State.Toasts.Success("Hot-Plug Connected", fmt.Sprintf("Attached %s (%s)", a.State.Descriptor.ShortName, a.State.DeviceInfo.Connection))
		} else {
			a.State.Toasts.Warning("Hot-Plug Disconnected", "Controller unplugged (empty state active)")
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
		a.State.Toasts.Info("Transport Changed", fmt.Sprintf("Switched to %s", conn))
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
		a.State.SetError("Permission Denied", a.State.DeviceInfo.PermissionHint)
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
				s.Toasts.Info(act.Label, s.Descriptor.ShortName+" pulsed")
			}
			return
		}
	}
}

func (a *Application) callbacks() views.Callbacks {
	return views.Callbacks{
		OnToggleConnect:    a.ToggleSimulatedConnection,
		OnCycleTransport:   a.CycleSimulatedTransport,
		OnTogglePermission: a.ToggleSimulatedPermission,
		OnSelectDevice:     a.SelectDevice,
		OnCycleDevice:      a.CycleDevice,
		OnSetMode:          a.SetMode,
		OnPrefsChanged:     a.savePrefs,
		OnExportReport:     a.ExportDiagnosticsReport,
	}
}

// Draw is the immediate-mode Limoni frame callback compatible with both
// limoni.RunWithContext and headless uitest.Run.
func (a *Application) Draw(f *terminal.Frame, ev *driver.Event) bool {
	a.drainEvents()

	now := time.Now()
	dt := now.Sub(a.lastFrame).Seconds()
	if dt <= 0 || dt > 0.25 {
		dt = 1.0 / 30.0
	}
	a.lastFrame = now

	if ev != nil && ev.Type == driver.EventKey {
		if !a.HandleKeyEvent(f, ev.Key) {
			return false
		}
	}

	a.tickDiagnostics(now)

	if a.State.AutoRotate && a.State.DeviceInfo.Connected {
		a.State.ShowcasePhase += a.State.AutoRotateSpeed * dt
		a.State.RotY = 22.0 * math.Sin(a.State.ShowcasePhase)
		a.State.RotX = -20.0 + 5.0*math.Cos(a.State.ShowcasePhase*0.7)
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
