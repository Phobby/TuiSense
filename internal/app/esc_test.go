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
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/uitest"
)

// keysFrom decodes raw terminal input with Limoni's own parser, as the event
// loop does, and returns the key events that survive repairKey.
func keysFrom(t *testing.T, a *Application, raw string) []driver.KeyEvent {
	t.Helper()
	var keys []driver.KeyEvent
	buf := []byte(raw)
	for len(buf) > 0 {
		ev, n := driver.ParseEvent(buf)
		if n == 0 {
			t.Fatalf("parser stalled at %q", buf)
		}
		buf = buf[n:]
		if ev.Type != driver.EventKey {
			continue
		}
		if key, ok := a.repairKey(ev.Key, time.Now()); ok {
			keys = append(keys, key)
		}
	}
	return keys
}

// Holding Esc while moving the mouse interleaves ESC bytes with SGR mouse
// reports. When "ESC ESC" swallows a report's own ESC, the rest of the report
// must not reach TuiSense as key presses ('3' switches tabs, 'M' changes the
// shading, '[' cycles controller parts).
func TestHeldEscWithMouseMotionIsNotTypedAsKeys(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	raw := strings.Repeat("\x1b\x1b[<35;40;10M\x1b\x1b[<35;41;10m", 3) + "\x1b\x1b[A"
	keys := keysFrom(t, a, raw)
	if len(keys) == 0 {
		t.Fatal("the Esc presses themselves must still arrive")
	}
	for _, k := range keys {
		if k.Type != driver.KeyEsc {
			t.Fatalf("a broken mouse report leaked as a key: %+v", k)
		}
	}
	// Without a preceding Alt+Esc, keys are untouched.
	if got := keysFrom(t, a, "3M["); len(got) != 3 || got[0].Ch != '3' {
		t.Fatalf("ordinary typing must pass through: %+v", got)
	}
	// Alt+Esc followed by a key that does not start a sequence keeps that key.
	if got := keysFrom(t, a, "\x1b\x1bg"); len(got) != 2 || got[0].Type != driver.KeyEsc || got[1].Ch != 'g' {
		t.Fatalf("Alt+Esc then 'g': %+v", got)
	}
}

func TestEscAsksBeforeQuitting(t *testing.T) {
	a := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 120, 40, a.Draw)
	fresh := func() { a.escAt = time.Time{} } // the test presses faster than a hand

	page.Press("2")
	page.Press("esc")
	page.Expect(page.GetByID("quit_confirm")).ToBeVisible()
	if a.State.ActivePanel != state.PanelDiagnostics {
		t.Fatal("the dialog must not change the page behind it")
	}
	page.Press("3") // keys behind the dialog are ignored
	if a.State.ActivePanel != state.PanelDiagnostics {
		t.Fatal("keys leaked past the quit dialog")
	}
	page.Press("esc") // still held: an auto-repeat must not close it
	page.Expect(page.GetByID("quit_confirm")).ToBeVisible()
	fresh()
	page.Press("esc")
	page.Expect(page.GetByID("quit_confirm")).Not().ToBeVisible()

	fresh()
	page.Press("esc")
	page.GetByID("quit_no").Click()
	page.Expect(page.GetByID("quit_confirm")).Not().ToBeVisible()
	if page.Exited() {
		t.Fatal("No must keep TuiSense open")
	}

	fresh()
	page.Press("esc")
	page.Press("enter")
	page.ExpectExit()
}

func TestQuitDialogYesButtonQuits(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	page := uitest.Run(t, 120, 40, a.Draw)
	page.Press("esc")
	page.GetByID("quit_yes").Click()
	page.ExpectExit()
}

// Esc first closes what is open; only then does it ask to quit.
func TestEscClosesHelpBeforeAskingToQuit(t *testing.T) {
	a := newMockApp(t, state.ModeEZ)
	page := uitest.Run(t, 120, 40, a.Draw)
	page.Press("?")
	page.Press("esc")
	if a.State.ShowHelp || a.State.ShowQuitConfirm {
		t.Fatalf("help %v, quit dialog %v", a.State.ShowHelp, a.State.ShowQuitConfirm)
	}
}

// releaseServer serves a GitHub-style release list with one release.
func releaseServer(t *testing.T, tag string) *update.Source {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"` + tag + `","draft":false,"prerelease":false,"assets":[]}]`))
	}))
	t.Cleanup(srv.Close)
	return &update.Source{API: srv.URL, AllowHTTP: true, Hosts: []string{"127.0.0.1"}}
}

// A finished background check is visible beside the version, and clicking
// it opens Settings where the details are.
func TestHeaderShowsUpdateCheckResult(t *testing.T) {
	for _, tc := range []struct{ tag, want string }{{"v0.8.0", "✓  v0.8.0"}, {"v0.9.0", "v0.9.0 available  v0.8.0"}} {
		a := newTestApp(t, newDirs(t), Options{Version: "0.8.0", UpdateSource: releaseServer(t, tc.tag),
			Registry: drivers.NewBuiltinRegistry(drivers.RegistryOptions{})})
		a.Updater.CheckAsync(true)
		deadline := time.Now().Add(5 * time.Second)
		for st := a.Updater.Snapshot().Status; st == update.StatusChecking || st == update.StatusIdle; st = a.Updater.Snapshot().Status {
			if time.Now().After(deadline) {
				t.Fatal("the check did not finish")
			}
			time.Sleep(10 * time.Millisecond)
		}
		first := strings.Split(snapshotOf(a, 110, 30), "\n")[0]
		if !strings.HasSuffix(strings.TrimRight(first, " "), tc.want) {
			t.Fatalf("%s: header %q, want suffix %q", tc.tag, first, tc.want)
		}
		page := uitest.Run(t, 110, 30, a.Draw)
		page.GetByID("app_version").Click()
		if a.State.ActivePanel != state.PanelSettings {
			t.Fatalf("%s: clicking the version opens %s", tc.tag, a.State.ActivePanel)
		}
	}
}
