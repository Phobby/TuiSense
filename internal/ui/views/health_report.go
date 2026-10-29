package views

import (
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
)

// BuildHealthClipboard shares exactly the report displayed by both UX modes.
func BuildHealthClipboard(s *state.AppState, current bool, now time.Time) string {
	if s.Diag == nil {
		return "Tanılama oturumu yok; ölçüm bulunmuyor."
	}
	report := s.Diag.ReportSnapshot(now)
	if current {
		return report.Scope(s.Diag.Active).TestText()
	}
	return report.Text()
}

func reportCopy(c *cursor, text string) {
	if c.s.Copy.OnCopy != nil {
		c.s.Copy.OnCopy(text)
	}
}

func renderTestReportControls(c *cursor, d *diag.Session) {
	report := d.ReportSnapshot(time.Now()).Scope(d.Active)
	c.pills("", 0, []choice{
		{ID: "copy_test_report", Label: i18n.T("Copy This Test"), OnSelect: func() { reportCopy(c, report.TestText()) }},
		{Kind: kindToggle, ID: "diag_show_test_report", Label: i18n.T("Test Report"), Selected: d.ShowTestReport, OnSelect: func() { d.ShowTestReport = !d.ShowTestReport }},
		copyAllChoice(c),
	})
}

// copyAllChoice copies every test's results; it sits beside the page's own
// actions rather than taking a row of its own.
func copyAllChoice(c *cursor) choice {
	return choice{ID: "copy_all_diagnostics", Label: i18n.T("Copy All Diagnostics"), OnSelect: func() { reportCopy(c, BuildHealthClipboard(c.s, false, time.Now())) }}
}

func renderHealthReport(c *cursor, d *diag.Session, pc PanelContext, report diag.Report) {
	p := c.p()
	individual := d.Active != diag.CatHealthReport
	var controls []choice
	if individual {
		controls = append(controls, choice{ID: "copy_detailed_test_report", Label: i18n.T("Copy This Test"), OnSelect: func() { reportCopy(c, report.TestText()) }})
	} else {
		controls = append(controls, choice{ID: "copy_full_report", Label: i18n.T("Copy Report"), OnSelect: func() { reportCopy(c, report.Text()) }}, choice{ID: "copy_warnings_only", Label: i18n.T("Copy Warnings Only"), OnSelect: func() { reportCopy(c, report.WarningsText()) }})
	}
	// Three groups: copying, viewing/exporting, then history below.
	controls = append(controls, choice{ID: "copy_technical_report", Label: i18n.T("Copy Technical Report"), OnSelect: func() { reportCopy(c, report.TechnicalText()) }})
	c.pills("", 0, controls)
	view := []choice{
		{Kind: kindToggle, ID: "show_technical_report", Label: i18n.T("Technical Report"), Selected: d.ShowTechnicalReport, OnSelect: func() { d.ShowTechnicalReport = !d.ShowTechnicalReport }},
		{ID: "report_raw_logs", Label: i18n.T("Raw Logs"), OnSelect: func() { d.Suspend(); c.s.Mode = state.ModeAdvanced; c.s.ActivePanel = state.PanelLogs }},
	}
	if pc.CB.OnExportReport != nil {
		view = append(view, choice{ID: "health_report_export", Label: i18n.T("Export: TXT / Markdown / JSON"), OnSelect: pc.CB.OnExportReport})
	}
	if !individual {
		view = append(view, choice{ID: "health_report_refresh", Label: i18n.T("Generate Current Report"), OnSelect: func() { d.Select(diag.CatHealthReport); d.GenerateHealthReport(time.Now()) }})
	}
	c.pills("", 0, view)
	if !individual {
		renderHistoryControls(c, d, pc)
	}
	if d.PreviousReport != nil && !individual {
		c.line("Kaydedilmiş rapor — güncel ölçüm değildir.", p.SectionHeaderStyle())
	}
	text := report.Text()
	if individual {
		text = report.TestText()
	}
	if d.ShowTechnicalReport {
		text = report.TechnicalText()
	}
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			c.gap()
		} else {
			c.wrap(line, p.LabelStyle(), 12)
		}
	}
}

func renderHistoryControls(c *cursor, d *diag.Session, pc PanelContext) {
	var actions []choice
	if pc.CB.OnSaveDiagnosticHistory != nil {
		actions = append(actions, choice{ID: "report_history_save", Label: i18n.T("Save snapshot"), OnSelect: pc.CB.OnSaveDiagnosticHistory})
	}
	if pc.CB.OnReviewDiagnosticHistory != nil {
		actions = append(actions, choice{ID: "report_history_review", Label: i18n.T("Review Previous Report"), OnSelect: pc.CB.OnReviewDiagnosticHistory})
	}
	if d.PreviousReport != nil {
		if pc.CB.OnOlderDiagnosticHistory != nil {
			actions = append(actions, choice{ID: "report_history_older", Label: i18n.T("Older report"), OnSelect: pc.CB.OnOlderDiagnosticHistory})
		}
		if pc.CB.OnNewerDiagnosticHistory != nil {
			actions = append(actions, choice{ID: "report_history_newer", Label: i18n.T("Newer report"), OnSelect: pc.CB.OnNewerDiagnosticHistory})
		}
		actions = append(actions, choice{ID: "copy_previous_report", Label: i18n.T("Copy Previous Report"), OnSelect: func() { reportCopy(c, d.PreviousReport.Text()) }})
	}
	if pc.CB.OnCompareDiagnosticHistory != nil {
		actions = append(actions, choice{ID: "report_history_compare", Label: i18n.T("Compare history"), OnSelect: pc.CB.OnCompareDiagnosticHistory})
	}
	if pc.CB.OnClearDiagnosticHistory != nil {
		actions = append(actions, choice{ID: "report_history_clear", Label: i18n.T("Clear history"), OnSelect: pc.CB.OnClearDiagnosticHistory})
	}
	c.pills("", 0, actions)
	for _, line := range d.HistorySummary {
		c.wrap(strings.TrimSpace(line), c.p().LabelStyle(), 3)
	}
}
