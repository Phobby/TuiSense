package views

import (
	"fmt"

	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/update"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
)

// updateStatusText describes the application updater state. It updates
// TuiSense itself, never controller firmware.
func updateStatusText(u update.Snapshot) string {
	switch u.Status {
	case update.StatusDisabled:
		return i18n.T("Automatic update checks are off.")
	case update.StatusDev:
		return i18n.T("Development build: updates are not checked.")
	case update.StatusChecking:
		return i18n.T("Checking for updates…")
	case update.StatusUpToDate:
		return i18n.T("TuiSense is up to date.")
	case update.StatusAvailable:
		return i18n.T("A new TuiSense version is available.")
	case update.StatusDownloading:
		return i18n.Tf("Downloading and verifying the update… %d%%", int(u.Progress*100))
	case update.StatusReady:
		return i18n.T("A new TuiSense version is ready. It will be applied when you restart the app.")
	case update.StatusOffline:
		return i18n.T("Could not reach the update server; TuiSense works normally offline.")
	case update.StatusFailed:
		return i18n.T("The update could not be verified and was discarded. The installed version is unchanged.")
	case update.StatusManaged:
		return i18n.T("Installed by a package manager: update TuiSense with that package manager.")
	}
	return i18n.T("Updates are checked in the background.")
}

// renderUpdates draws the compact Application updates section of Settings.
func renderUpdates(f *terminal.Frame, inner cell.Rect, y uint16, s *state.AppState, cb Callbacks, advanced bool) uint16 {
	p := s.Theme
	limit := inner.Y + inner.Height
	line := func(text string, st cell.Style) {
		if y < limit {
			f.Buffer.SetStringWithin(inner.X, y, text, st, inner.Width)
			y++
		}
	}
	u := s.Update
	line(i18n.T("Application updates"), p.SectionHeaderStyle())
	info := "TuiSense " + displayVersion(s.AppVersion)
	if u.Latest != "" {
		info += " · " + i18n.T("Latest") + " v" + u.Latest
	}
	channel := i18n.T("Stable")
	if u.Settings.Channel == update.Prerelease {
		channel = i18n.T("Pre-release")
	}
	info += " · " + i18n.T("Channel") + ": " + channel
	if !u.CheckedAt.IsZero() {
		info += " · " + i18n.T("Last checked") + ": " + u.CheckedAt.Local().Format("2006-01-02 15:04")
	}
	line(info, p.ValueStyle())
	line(updateStatusText(u), p.MutedStyle())
	if !u.CanStage && u.Status != update.StatusDev && u.Status != update.StatusManaged {
		line(i18n.T("This build cannot verify downloads (no pinned signing key), so it only checks for new versions."), p.MutedStyle())
	}
	if y >= limit {
		return y
	}
	items := []choice{{ID: "update_check_now", Label: i18n.T("Check now"), OnSelect: func() {
		if cb.OnCheckUpdates != nil {
			cb.OnCheckUpdates()
		}
	}}}
	if u.Status == update.StatusAvailable && u.CanStage {
		items = append(items, choice{ID: "update_download", Label: i18n.T("Download update"), OnSelect: func() {
			if cb.OnDownloadUpdate != nil {
				cb.OnDownloadUpdate()
			}
		}})
	}
	pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, items)
	y++
	set := func(fn func(*update.Settings)) func() {
		return func() {
			us := u.Settings
			fn(&us)
			if cb.OnSetUpdateSettings != nil {
				cb.OnSetUpdateSettings(us)
			}
		}
	}
	toggles := []choice{
		{ID: "update_auto_check", Label: checkbox(u.Settings.Check) + i18n.T("Check automatically"), OnSelect: set(func(us *update.Settings) { us.Check = !us.Check })},
		{ID: "update_auto_download", Label: checkbox(u.Settings.AutoDownload) + i18n.T("Download automatically (verified, applied on restart)"), OnSelect: set(func(us *update.Settings) { us.AutoDownload = !us.AutoDownload })},
	}
	if advanced {
		toggles = append(toggles, choice{ID: "update_prerelease", Label: checkbox(u.Settings.Channel == update.Prerelease) + i18n.T("Pre-releases"), OnSelect: set(func(us *update.Settings) {
			if us.Channel == update.Prerelease {
				us.Channel = update.Stable
			} else {
				us.Channel = update.Prerelease
			}
		})})
	}
	for _, t := range toggles {
		if y >= limit {
			break
		}
		pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{t})
		y++
	}
	if advanced && u.Notes != "" && (u.Status == update.StatusAvailable || u.Status == update.StatusReady || u.Status == update.StatusDownloading) {
		for _, n := range update.NotesExcerpt(u.Notes, 3) {
			line("  "+n, p.MutedStyle())
		}
	}
	return y
}

func checkbox(on bool) string {
	if on {
		return "[x] "
	}
	return "[ ] "
}

func displayVersion(v string) string {
	if v == "" || v == "dev" {
		return "dev"
	}
	return fmt.Sprintf("v%s", v)
}
