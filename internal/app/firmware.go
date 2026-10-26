package app

import (
	"context"
	"runtime"
	"slices"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/firmware/sony"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/system"
	"github.com/thebanri/limoni"
	"github.com/thebanri/limoni/widgets"
)

// autoCheckInterval is how often the read-only firmware check runs by itself.
const autoCheckInterval = 24 * time.Hour

// defaultFirmwareProviders returns the built-in read-only firmware checkers.
func defaultFirmwareProviders(version string) []firmware.Provider {
	ua := "TuiSense"
	if version != "" {
		ua += "/" + version
	}
	return []firmware.Provider{sony.New(sony.NewHTTPFetcher(ua + " (read-only firmware check)"))}
}

// firmwareSubject snapshots the active device for the firmware service.
func (a *Application) firmwareSubject() firmware.Subject {
	key := a.Manager.ActiveKey()
	return firmware.NewSubject(key, a.Manager.Active(), a.isSimulated(key))
}

// refreshFirmware rebuilds the Device Details snapshot for the active device
// (render goroutine only).
func (a *Application) refreshFirmware() {
	s := a.State
	sub := a.firmwareSubject()
	fv := state.FirmwareView{Section: s.Firmware.Section, DetailGroup: s.Firmware.DetailGroup, SelectedDetail: s.Firmware.SelectedDetail, AutoCheck: a.fwAutoCheck}
	if fv.Section == "" {
		fv.Section = state.FirmwareStatus
	}
	dev := a.Manager.Active()
	fv.Connected = dev != nil && sub.Info.Connected
	fv.Details = device.DetailsOf(dev)
	if p := a.Firmware.ProviderFor(sub); p != nil {
		fv.Supported = true
		fv.Checking = a.Firmware.Running(sub.Key)
		fv.Guide = p.Guide(runtime.GOOS)
		fv.Installed = p.InstalledVersion(sub)
		if r, ok := a.fwResults[sub.Key]; ok {
			// A result describes the firmware read at check time. When the
			// controller reconnects over another transport or reports another
			// version, the old comparison no longer applies.
			if r.Connection == sub.Info.Connection && r.Installed == fv.Installed {
				fv.Result = &r
			} else {
				delete(a.fwResults, sub.Key)
			}
		}
		if rec, ok := a.Firmware.LastSuccess(sub); ok {
			fv.Last = &rec
		}
	}
	s.Firmware = fv
}

// CheckFirmware starts a read-only check of the active device in the
// background. The UI keeps running; the result arrives through the event queue.
func (a *Application) CheckFirmware() {
	sub := a.firmwareSubject()
	if !a.Firmware.Supported(sub) {
		return
	}
	a.startFirmwareCheck(sub)
}

func (a *Application) startFirmwareCheck(sub firmware.Subject) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if a.Firmware.Start(ctx, sub, a.queueFirmwareResult) {
		a.Logger.Info("FIRMWARE", "Checking %s firmware against %s", sub.Descriptor.ShortName, a.Firmware.ProviderFor(sub).Name())
	}
	a.refreshFirmware()
}

// queueFirmwareResult is called on the worker goroutine.
func (a *Application) queueFirmwareResult(r firmware.Result) {
	a.eventsMu.Lock()
	a.fwQueue = append(a.fwQueue, r)
	a.eventsMu.Unlock()
	limoni.Wakeup()
}

// drainFirmware applies finished checks (render goroutine only).
func (a *Application) drainFirmware() {
	a.eventsMu.Lock()
	rs := a.fwQueue
	a.fwQueue = nil
	a.eventsMu.Unlock()
	if len(rs) == 0 {
		return
	}
	for _, r := range rs {
		a.fwResults[r.Key] = r
		a.Logger.Info("FIRMWARE", "%s: %s (installed %s, available %s)", r.Key, r.Status, nonEmpty(r.Installed, "?"), nonEmpty(r.Available, "?"))
		for _, d := range r.Diagnostics {
			a.Logger.Debug("FIRMWARE", "%s", d)
		}
		if r.Notify && a.State.Toasts != nil {
			a.State.Toasts.Show(i18n.T("Firmware update available"),
				i18n.Tf("Installed %s, available %s. Official installation is required: press F → Official update instructions.", r.Installed, r.Available),
				widgets.ToastInfo, 10*time.Second)
		}
	}
	a.refreshFirmware()
}

// maybeAutoCheck runs the daily check for real hardware only.
func (a *Application) maybeAutoCheck() {
	if !a.fwAutoCheck || !a.started {
		return
	}
	sub := a.firmwareSubject()
	if sub.Simulated || !a.Firmware.Supported(sub) || !sub.Info.Connected || !a.Firmware.Due(sub, autoCheckInterval) {
		return
	}
	a.startFirmwareCheck(sub)
}

// SetFirmwareAutoCheck enables or disables the daily check and persists it.
func (a *Application) SetFirmwareAutoCheck(on bool) {
	a.fwAutoCheck = on
	a.savePrefs()
	a.refreshFirmware()
}

// SetFirmwareSection switches the Device Details panel section.
func (a *Application) SetFirmwareSection(sec state.FirmwareSection) {
	a.State.Firmware.Section = sec
}

// SetDetailGroup switches the group shown in Advanced details.
func (a *Application) SetDetailGroup(g device.DetailGroup) {
	a.State.Firmware.DetailGroup = g
	a.State.Firmware.SelectedDetail = ""
}

// SelectDetail chooses the reading whose origin and meaning are explained.
func (a *Application) SelectDetail(key string) {
	a.State.Firmware.SelectedDetail = key
}

// isOfficialLink reports whether url is one of the provider's official pages.
func (a *Application) isOfficialLink(url string) bool {
	return slices.ContainsFunc(a.State.Firmware.Guide.Links, func(l firmware.Link) bool { return l.URL == url })
}

// OpenOfficialLink opens an official update page in the browser. This only
// shows a web page; it does not update the controller.
func (a *Application) OpenOfficialLink(url string) {
	if !a.isOfficialLink(url) {
		return
	}
	open := a.Opts.OpenURL
	if open == nil {
		open = system.OpenURL
	}
	if err := open(url); err != nil {
		a.State.SetError(i18n.T("Could not open the browser"), i18n.Tf("%v — the link is shown in the panel so you can open it manually.", err))
		return
	}
	if a.State.Toasts != nil {
		a.State.Toasts.Info(i18n.T("Web page opened"), i18n.T("This only opens Sony's instructions; it does not update the controller."))
	}
}

// CopyOfficialLink copies an official update page URL to the clipboard.
func (a *Application) CopyOfficialLink(url string) {
	if !a.isOfficialLink(url) {
		return
	}
	a.CopyText(url)
}

func nonEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
