package app

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/clipboard"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/buffer"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/uitest"
)

func waitForCopy(t *testing.T, a *Application) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for a.copying && time.Now().Before(deadline) {
		a.drainCopies()
		time.Sleep(time.Millisecond)
	}
	if a.copying {
		t.Fatal("copy did not finish")
	}
}

type stubClipboard struct {
	confirmed bool
	err       error
}

func (s stubClipboard) Copy(context.Context, string) (clipboard.Outcome, error) {
	return clipboard.Outcome{Backend: "test", Confirmed: s.confirmed}, s.err
}
func TestCopyFeedbackFailureAndAccessibleFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		service stubClipboard
		want    string
	}{{"success", stubClipboard{confirmed: true}, "Copied to clipboard"}, {"terminal", stubClipboard{}, "paste to verify"}, {"failure", stubClipboard{err: errors.New("permission denied")}, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			a := newMockApp(t, state.ModeEZ)
			a.Opts.Clipboard = tc.service
			a.Opts.CopyText = nil
			text := "Exact multiline diagnostic\n0x00000413 — 3.200%"
			a.CopyText(text)
			waitForCopy(t, a)
			if tc.want != "" {
				if !strings.Contains(a.State.CopyStatus, tc.want) {
					t.Fatalf("feedback: %s", a.State.CopyStatus)
				}
			} else {
				if a.State.CopyStatus != "" || !a.State.ShowCopyFallback {
					t.Fatal("failure claimed success or omitted fallback")
				}
				path := a.State.CopyFallbackPath
				t.Cleanup(func() { _ = os.Remove(path) })
				data, err := os.ReadFile(path)
				if err != nil || string(data) != text {
					t.Fatalf("fallback: %q %v", data, err)
				}
				// Windows has no Unix permission bits; the file lives in the
				// user's own temp directory, which its ACL keeps private.
				stat, err := os.Stat(path)
				if err != nil || runtime.GOOS != "windows" && stat.Mode().Perm() != 0600 {
					t.Fatal("fallback file not private")
				}
				page := uitest.Run(t, 120, 40, a.Draw)
				page.Expect(page.GetByID("copy_fallback")).ToBeVisible()
				a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyEsc})
				if a.State.ShowCopyFallback {
					t.Fatal("keyboard cannot dismiss fallback")
				}
			}
		})
	}
}
func TestCopyActionsKeyboardAndControllerInteraction(t *testing.T) {
	a, page := openDiagnostics(t, state.ModeAdvanced)
	var copied []string
	a.Opts.CopyText = func(s string) error { copied = append(copied, s); return nil }
	page.GetByID("copy_all_diagnostics").Click()
	waitForCopy(t, a)
	if len(copied) != 1 || !strings.Contains(copied[0], "TuiSense — Kontrolcü Sağlık Raporu") {
		t.Fatal("bulk copy failed")
	}
	d := a.State.Diag
	d.Active = diag.CatTriggers
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyRune, Ch: 'y'})
	waitForCopy(t, a)
	if !strings.Contains(copied[1], "Test: Tetikler") || strings.Contains(copied[1], "Analog çubuklar") {
		t.Fatal("current test shortcut scope incorrect")
	}
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyRune, Ch: 'Y'})
	waitForCopy(t, a)
	if !strings.Contains(copied[2], "Analog çubuklar") {
		t.Fatal("full report shortcut failed")
	}
	// Existing region, navigation and mode interactions retain their meanings.
	a.State.SelectRegion(device.RegionLeftTrigger)
	page.Press("tab")
	if d.Active != diag.CatTriggers {
		t.Fatal("3D trigger region stopped opening diagnostics")
	}
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyRune, Ch: 'h'})
	if d.Active == diag.CatTriggers {
		t.Fatal("diagnostics navigation consumed")
	}
	a.HandleKeyEvent(nil, driver.KeyEvent{Type: driver.KeyRune, Ch: 'e'})
	if a.State.Mode != state.ModeEZ {
		t.Fatal("mode shortcut consumed")
	}
	f := terminal.NewFrame(buffer.NewBuffer(cell.NewRect(0, 0, 120, 40)), terminal.NewFocusManager())
	a.Draw(f, nil)
	f.FocusManager.SetFocused("copy_all_diagnostics")
	a.HandleKeyEvent(f, driver.KeyEvent{Type: driver.KeyEnter})
	waitForCopy(t, a)
	if len(copied) != 4 {
		t.Fatal("focused Enter did not copy")
	}
}
