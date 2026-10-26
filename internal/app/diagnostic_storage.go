package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/system"
	"github.com/thebanri/limoni"
)

type diagnosticCompletion struct {
	session      *diag.Session
	previous     *diag.Report
	historyIndex int
	contextKey   string
	message      string
	comparison   []string
	err          error
}

// diagnosticStorage snapshots UI-owned state before doing file I/O in a worker.
// History collection is always initiated by the user, never by polling.
func (a *Application) diagnosticStorage(action string) {
	if a.diagnosticBusy || a.State.Diag == nil {
		return
	}
	a.diagnosticBusy = true
	s := a.State.Diag
	report := s.ReportSnapshot(time.Now())
	if action == "review" || action == "older" || action == "newer" || action == "compare" {
		report = diag.BuildReport(s, a.Opts.Version, false, time.Now())
	}
	if s.Active != diag.CatHealthReport && s.ShowTestReport {
		report = report.Scope(s.Active)
	}
	historyIndex := s.HistoryIndex
	if action == "review" {
		historyIndex = 0
	}
	if action == "older" {
		historyIndex++
	}
	if action == "newer" {
		historyIndex = max(0, historyIndex-1)
	}
	contextKey := s.Ctx.Key
	key := s.Ctx.Info.MACAddress
	stable := key != ""
	if key == "" {
		key = s.Ctx.Info.ID
	}
	if key == "" {
		key = s.Ctx.Key
	}
	dataDir := system.DataDir()
	// Custom config roots isolate test and portable installations from user data.
	if a.Opts.ConfigDir != system.ConfigDir() {
		dataDir = a.Opts.ConfigDir
	}
	store := diag.HistoryStore{Dir: filepath.Join(dataDir, "diagnostics")}
	dir := a.Opts.ReportDir
	if dir == "" {
		dir = filepath.Join(dataDir, "reports")
	}
	go func() {
		result := diagnosticCompletion{session: s, contextKey: contextKey}
		switch action {
		case "export":
			jp, mp, err := report.Export(dir)
			result.err = err
			if err == nil {
				result.message = jp + "\n" + mp + "\n" + strings.TrimSuffix(mp, ".md") + ".txt"
			}
		case "clear":
			result.err = store.Clear()
			result.message = "Diagnostic history cleared."
		case "save", "compare", "review", "older", "newer":
			var current diag.HistoryEntry
			current, result.err = store.Snapshot(report, key)
			if result.err == nil {
				if action == "save" {
					result.err = store.Save(current)
					result.message = "Diagnostic snapshot saved locally. No data was uploaded."
				} else if action != "compare" {
					var entries []diag.HistoryEntry
					entries, result.err = store.Load()
					var matches []diag.HistoryEntry
					for i := len(entries) - 1; i >= 0; i-- {
						if entries[i].Identity == current.Identity {
							matches = append(matches, entries[i])
						}
					}
					if result.err == nil {
						if len(matches) == 0 {
							result.err = fmt.Errorf("no saved report for this local device")
						} else {
							result.historyIndex = min(historyIndex, len(matches)-1)
							previous := matches[result.historyIndex].StoredReport()
							result.previous = &previous
							result.message = fmt.Sprintf("Saved report %d/%d loaded.", result.historyIndex+1, len(matches))
						}
					}
				} else {
					var entries []diag.HistoryEntry
					entries, result.err = store.Load()
					result.comparison = []string{"No previous snapshot for this local device. Save a snapshot first."}
					for i := len(entries) - 1; i >= 0; i-- {
						if entries[i].Identity == current.Identity {
							result.comparison = diag.CompareHistory(entries[i], current)
							break
						}
					}
					if !stable {
						result.comparison = append([]string{"Stable device identity unavailable; this comparison applies only to the current device path. Reconnected identical devices may be indistinguishable."}, result.comparison...)
					}
					result.message = "History comparison ready."
				}
			}
		default:
			result.err = fmt.Errorf("unknown diagnostic action %q", action)
		}
		a.diagnosticResults <- result
		limoni.Wakeup()
	}()
}
func (a *Application) drainDiagnosticStorage() {
	select {
	case result := <-a.diagnosticResults:
		a.diagnosticBusy = false
		if result.err != nil {
			a.State.SetError(i18n.T("Diagnostics"), result.err.Error())
			return
		}
		if a.State.Diag == result.session && a.State.Diag.Ctx.Key == result.contextKey {
			if result.previous != nil {
				result.session.Select(diag.CatHealthReport)
				result.session.PreviousReport = result.previous
				result.session.ReportGeneration++
				result.session.HistoryIndex = result.historyIndex
				a.State.DiagnosticsScroll = 0
			}
			if result.comparison != nil {
				result.session.HistorySummary = result.comparison
			}
			if result.message == "Diagnostic history cleared." {
				result.session.HistorySummary = nil
				result.session.PreviousReport = nil
				result.session.HistoryIndex = 0
			}
		}
		a.Logger.Info("DIAG", "%s", result.message)
		if a.State.Toasts != nil {
			a.State.Toasts.Info(i18n.T("Diagnostics"), i18n.T(result.message))
		}
	default:
	}
}

// RetestInputMapping reopens hardware through the manager's polling worker.
func (a *Application) RetestInputMapping() {
	if a.State.Diag != nil {
		a.State.Diag.Reset(time.Now())
	}
	a.Manager.RetestMapping()
}
