package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/update"
)

func TestVersionIsShownTopRight(t *testing.T) {
	a := newTestApp(t, newDirs(t), Options{Version: "0.8.0", Registry: drivers.NewBuiltinRegistry(drivers.RegistryOptions{})})
	snap := snapshotOf(a, 100, 30)
	first := strings.Split(snap, "\n")[0]
	if !strings.HasSuffix(strings.TrimRight(first, " "), "v0.8.0") {
		t.Fatalf("version must end the header line: %q", first)
	}
}

// Offline start-up is immediate and silent: no false update notification.
func TestOfflineStartupIsNonBlocking(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	src := update.Source{API: closed.URL, AllowHTTP: true, Hosts: []string{"127.0.0.1"}}
	closed.Close()
	a := newTestApp(t, newDirs(t), Options{Version: "0.8.0", UpdateSource: &src, Registry: drivers.NewBuiltinRegistry(drivers.RegistryOptions{})})
	start := time.Now()
	if !a.Updater.CheckAsync(false) {
		t.Fatal("the first check must start in the background")
	}
	snapshotOf(a, 100, 30)
	if time.Since(start) > time.Second {
		t.Fatal("start-up must not wait for the network")
	}
	deadline := time.Now().Add(5 * time.Second)
	for a.Updater.Snapshot().Status != update.StatusOffline && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	a.syncUpdater()
	if a.State.Update.Status != update.StatusOffline {
		t.Fatalf("status %s", a.State.Update.Status)
	}
	if len(a.State.Toasts.Toasts) != 0 {
		t.Fatal("being offline is not worth a notification")
	}
	prefs, _ := a.Prefs.Load()
	if prefs.UpdateLastCheck.IsZero() {
		t.Fatal("the check time is persisted to avoid repeated checks")
	}
}

func TestUpdateSettingsPersistAndShowInSettings(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	a.SetUpdateSettings(update.Settings{Check: false, AutoDownload: true, Channel: update.Prerelease})
	prefs, _ := a.Prefs.Load()
	if prefs.UpdateChecks == nil || *prefs.UpdateChecks || !prefs.UpdateAutoDownload || prefs.UpdateChannel != "prerelease" {
		t.Fatalf("settings not persisted: %+v", prefs)
	}
	a.State.ActivePanel = state.PanelSettings
	snap := snapshotOf(a, 160, 45)
	for _, want := range []string{"Application updates", "Check now", "[ ] Check automatically", "[x] Download automatically", "[x] Pre-releases"} {
		if !strings.Contains(snap, want) {
			t.Fatalf("settings missing %q:\n%s", want, snap)
		}
	}
}
