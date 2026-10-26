package views

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// The Device Details panel shows what the controller reported about itself
// and runs the read-only firmware update check: it compares the installed
// version with the vendor's published metadata and explains how to update
// with the vendor's official tools. Nothing here can install firmware.

func renderEZFirmware(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	renderFirmware(f, area, pc, false)
}

func renderFirmwarePanel(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	renderFirmware(f, area, pc, true)
}

func renderFirmware(f *terminal.Frame, area cell.Rect, pc PanelContext, advanced bool) {
	s := pc.S
	fv := s.Firmware
	inner := panelFrame(f, area, s, i18n.T("Device Details"))
	c := newCursor(f, inner, s)
	renderCopyActions(c, true)
	if s.Diag != nil && s.Diag.Ctx.Caps.Supports(device.CapFirmwareInfo) {
		report := diag.BuildReport(s.Diag, "", false, time.Now()).Scope(diag.CatFirmware)
		c.pills("", 0, []choice{{ID: "firmware_copy_test_report", Label: i18n.T("Copy Test Report"), OnSelect: func() { reportCopy(c, report.TestText()) }}, {ID: "firmware_copy_test_summary", Label: i18n.T("Copy Test Summary"), OnSelect: func() { reportCopy(c, report.SummaryText()) }}})
	}
	if !fv.Connected {
		c.wrap(i18n.T("No controller is connected. Connect one to see its details."), s.Theme.MutedStyle(), 3)
		return
	}

	type section struct {
		id    state.FirmwareSection
		label string
	}
	sections := []section{{state.FirmwareStatus, i18n.T("Overview")}}
	if fv.Supported {
		sections = append(sections, section{state.FirmwareGuide, i18n.T("How to update")}, section{state.FirmwareLinks, i18n.T("Official links")})
	}
	if advanced {
		sections = append(sections, section{state.FirmwareDetails, i18n.T("Advanced details")})
	}
	active := fv.Section
	if !slices.ContainsFunc(sections, func(sec section) bool { return sec.id == active }) {
		active = state.FirmwareStatus
	}
	var items []choice
	for _, sec := range sections {
		id := sec.id
		items = append(items, choice{
			ID: "fw_section_" + string(id), Label: sec.label, Selected: active == id,
			OnSelect: func() {
				if pc.CB.OnSetFirmwareSection != nil {
					pc.CB.OnSetFirmwareSection(id)
				}
			},
		})
	}
	c.pills("", 0, items)
	c.gap()

	switch active {
	case state.FirmwareGuide:
		renderFirmwareGuide(c, pc)
	case state.FirmwareLinks:
		renderFirmwareLinks(c, pc)
	case state.FirmwareDetails:
		renderFirmwareDetails(c, pc)
	default:
		renderFirmwareStatus(c, pc, advanced)
	}
}

// firmwareStatusLabel maps a status to its icon, label and color.
func firmwareStatusLabel(c *cursor, st firmware.Status) (string, cell.Color) {
	p := c.p()
	switch st {
	case firmware.StatusUpdateAvailable:
		return i18n.T("⬆ Update available"), p.Warning
	case firmware.StatusUpToDate:
		return i18n.T("✓ Up to date"), p.Success
	case firmware.StatusUnsupported:
		return i18n.T("✕ Unsupported device revision"), p.FgSecondary
	case firmware.StatusNetworkUnavailable:
		return i18n.T("⚠ Network error"), p.Warning
	case firmware.StatusSourceUnavailable:
		return i18n.T("⚠ Metadata unavailable"), p.Warning
	default:
		return i18n.T("? Unable to verify"), p.FgSecondary
	}
}

func formatCheckTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// detailValue renders a reading for display.
func detailValue(d device.Detail) string {
	if d.Unavailable {
		return i18n.T("unavailable")
	}
	return i18n.T(d.Value)
}

func detailStyle(c *cursor, d device.Detail) cell.Style {
	p := c.p()
	if d.Unavailable {
		return p.MutedStyle()
	}
	return p.ValueStyle()
}

// renderDetailRows prints the readings with the given keys that fit the mode.
func renderDetailRows(c *cursor, fv state.FirmwareView, advanced bool, kw uint16, keys ...string) {
	for _, k := range keys {
		d, ok := fv.Detail(k)
		if !ok || (d.Advanced && !advanced) {
			continue
		}
		c.kvCopy(i18n.T(d.Label), detailValue(d)+rawSuffix(d), detailValue(d), kw, detailStyle(c, d))
	}
}

// splitColumns returns two cursors side by side below c when the panel is
// wide enough, or c twice otherwise.
func splitColumns(c *cursor, s *state.AppState, minWidth uint16) (left, right *cursor) {
	rest := cell.NewRect(c.r.X, c.y, c.r.Width, uint16(max(0, c.remaining())))
	if rest.Width < minWidth || rest.Height == 0 {
		return c, c
	}
	lw := rest.Width / 2
	return newCursor(c.f, cell.NewRect(rest.X, rest.Y, lw-2, rest.Height), s),
		newCursor(c.f, cell.NewRect(rest.X+lw, rest.Y, rest.Width-lw, rest.Height), s)
}

func renderFirmwareStatus(c *cursor, pc PanelContext, advanced bool) {
	s := pc.S
	fv := s.Firmware
	p := c.p()
	const kw = 23
	left, right := splitColumns(c, s, 96)

	// Left: what the controller is, as read from it.
	left.line(i18n.T("DEVICE INFORMATION"), p.SectionHeaderStyle())
	renderDetailRows(left, fv, advanced, kw, "model", "manufacturer", "connection", "usb_id")
	if advanced {
		renderDetailRows(left, fv, advanced, kw, "hidraw", "driver")
	}
	left.gap()
	left.line(i18n.T("HARDWARE & BUILD"), p.SectionHeaderStyle())
	if advanced {
		renderDetailRows(left, fv, advanced, kw, "fw_build", "fw_date")
	}
	renderDetailRows(left, fv, advanced, kw, "hw_version", "revision")
	for _, d := range fv.Details {
		if d.Label == "Note" {
			left.gap()
			left.wrap(i18n.T(d.Value), cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 3)
		}
	}
	if left == right {
		c.gap()
	}

	// Right: the firmware version and the read-only update check.
	right.line(i18n.T("FIRMWARE"), p.SectionHeaderStyle())
	installed := i18n.T("unknown")
	instStyle := p.MutedStyle()
	if d, ok := fv.Detail("fw_version"); ok && !d.Unavailable {
		installed, instStyle = d.Value, p.ValueStyle()
	} else if fv.Installed != "" {
		installed, instStyle = fv.Installed, p.ValueStyle()
	}
	right.kv(i18n.T("Installed version"), installed, kw, instStyle)
	if !fv.Supported {
		right.gap()
		right.wrap(i18n.T("Firmware update checks are not available for this controller."), p.MutedStyle(), 2)
		return
	}

	r := fv.Result
	if r != nil && fv.Checking {
		r = nil
	}
	target, targetStyle := i18n.T("Unknown"), p.MutedStyle()
	if d, ok := fv.Detail("fw_target"); ok && d.Value != "Unknown" {
		target, targetStyle = d.Value, p.ValueStyle()
	}
	if r != nil && r.Target != "" {
		target, targetStyle = r.Target+" "+i18n.T("(only target known for this model)"), p.ValueStyle()
	}
	right.kv(i18n.T("Firmware target"), target, kw, targetStyle)
	available, availStyle := i18n.T("Not checked"), p.MutedStyle()
	if r != nil {
		available = nonEmptyStr(r.Available, i18n.T("not verified"))
		if r.Available != "" {
			availStyle = p.ValueStyle()
		}
	}
	right.kv(i18n.T("Available version"), available, kw, availStyle)
	switch {
	case fv.Checking:
		right.kv(i18n.T("Update status"), i18n.T("… Checking Sony's firmware metadata"), kw, cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel})
	case r != nil:
		lbl, col := firmwareStatusLabel(right, r.Status)
		right.kv(i18n.T("Update status"), lbl, kw, cell.Style{Fg: col, Bg: p.BgPanel, Modifier: cell.ModifierBold})
	case fv.Last != nil:
		lbl, col := firmwareStatusLabel(right, fv.Last.Status)
		right.kv(i18n.T("Update status"), lbl+"  "+i18n.Tf("(last check %s)", formatCheckTime(fv.Last.CheckedAt)), kw, cell.Style{Fg: col, Bg: p.BgPanel})
	default:
		right.kv(i18n.T("Update status"), i18n.T("Not checked"), kw, p.MutedStyle())
	}
	right.f.RegisterAccessibility(accessibility.AccessibilityNode{
		ID: "firmware_status", Role: accessibility.RoleGeneric, Label: i18n.T("Firmware status"),
		Value: firmwareStatusValue(fv), Bounds: cell.NewRect(right.r.X, right.y-1, right.r.Width, 1),
	})
	lastAt := time.Time{}
	if fv.Last != nil {
		lastAt = fv.Last.CheckedAt
	}
	right.kv(i18n.T("Last successful check"), formatCheckTime(lastAt), kw, p.MutedStyle())

	right.gap()
	label := i18n.T("⟳ Check for updates")
	if fv.Checking {
		label = i18n.T("… Checking")
	}
	right.pills("", 0, []choice{
		{ID: "btn_firmware_check", Label: label, OnSelect: func() {
			if !fv.Checking && pc.CB.OnCheckFirmware != nil {
				pc.CB.OnCheckFirmware()
			}
		}},
		{ID: "btn_firmware_howto", Label: i18n.T("Official update instructions"), OnSelect: func() {
			if pc.CB.OnSetFirmwareSection != nil {
				pc.CB.OnSetFirmwareSection(state.FirmwareGuide)
			}
		}},
	})

	if r == nil {
		right.wrap(i18n.T("Checking only reads Sony's public version list. Nothing is installed or written to the controller."), p.MutedStyle(), 2)
		return
	}
	if r.Simulated {
		right.line(i18n.T("Simulated controller — this result is only a demonstration."), cell.Style{Fg: p.Warning, Bg: p.BgPanel})
	}
	right.wrap(r.Summary, p.LabelStyle(), 3)
	if advanced {
		for _, cand := range r.Candidates {
			right.line(firmwareCandidateLine(r, cand), p.MutedStyle())
		}
	}
	switch r.Status {
	case firmware.StatusUpdateAvailable:
		right.wrap(i18n.T("Installing firmware requires Sony's official updater. TuiSense does not start or perform updates — open \"How to update\"."), cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 3)
	case firmware.StatusUndetermined:
		if r.Basis != "" {
			right.wrap(r.Basis, p.MutedStyle(), 5)
		}
		right.wrap(i18n.T("To be sure, check with Sony's official updater on a PS5 or Windows PC — see \"How to update\"."), p.MutedStyle(), 2)
	default:
		if advanced && r.Basis != "" {
			right.wrap(r.Basis, p.MutedStyle(), 4)
		}
	}
	if advanced {
		right.kv(i18n.T("Metadata source"), nonEmptyStr(r.Source, "—"), kw, p.MutedStyle())
		for _, d := range r.Diagnostics {
			right.wrap("• "+d, cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 2)
		}
	}
}

func rawSuffix(d device.Detail) string {
	if d.Raw && !d.Unavailable && !strings.Contains(d.Label, "(raw)") {
		return " " + i18n.T("(raw)")
	}
	return ""
}

func firmwareStatusValue(fv state.FirmwareView) string {
	switch {
	case fv.Checking:
		return "checking"
	case fv.Result != nil:
		return string(fv.Result.Status)
	case fv.Last != nil:
		return string(fv.Last.Status)
	}
	return "not_checked"
}

func firmwareCandidateLine(r *firmware.Result, cand firmware.Candidate) string {
	if cand.Latest == "" {
		return i18n.Tf("  target %s: not listed", cand.Target)
	}
	rel := ""
	if r.Installed != "" {
		if strings.Compare(padVersion(r.Installed), padVersion(cand.Latest)) >= 0 {
			rel = i18n.T("installed is current")
		} else {
			rel = i18n.T("newer than installed")
		}
	}
	return i18n.Tf("  target %s: latest %s — %s", cand.Target, cand.Latest, rel)
}

// padVersion makes "0xNNNN" strings comparable as text.
func padVersion(v string) string {
	v = strings.ToUpper(strings.TrimPrefix(strings.TrimPrefix(v, "0x"), "0X"))
	return fmt.Sprintf("%8s", v)
}

func renderFirmwareGuide(c *cursor, pc PanelContext) {
	g := pc.S.Firmware.Guide
	p := c.p()
	if g.PlatformNote != "" {
		c.wrap(g.PlatformNote, cell.Style{Fg: p.Warning, Bg: p.BgPanel, Modifier: cell.ModifierBold}, 4)
	}
	for _, sec := range g.Sections {
		c.gap()
		c.line(sec.Title, p.SectionHeaderStyle())
		for i, step := range sec.Steps {
			c.wrap(fmt.Sprintf("%d. %s", i+1, step), p.LabelStyle(), 3)
		}
	}
	if g.Caution != "" {
		c.gap()
		c.wrap(g.Caution, cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 3)
	}
}

func renderFirmwareLinks(c *cursor, pc PanelContext) {
	g := pc.S.Firmware.Guide
	p := c.p()
	if g.Summary != "" {
		c.wrap(g.Summary, p.LabelStyle(), 3)
		c.gap()
	}
	for i, l := range g.Links {
		url := l.URL
		c.line(l.Label, p.SectionHeaderStyle())
		c.line(url, p.MutedStyle())
		c.pills("", 0, []choice{
			{ID: fmt.Sprintf("btn_firmware_open_%d", i), Label: i18n.T("Open web page"), OnSelect: func() {
				if pc.CB.OnOpenLink != nil {
					pc.CB.OnOpenLink(url)
				}
			}},
			{ID: fmt.Sprintf("btn_firmware_copy_%d", i), Label: i18n.T("Copy link"), OnSelect: func() {
				if pc.CB.OnCopyLink != nil {
					pc.CB.OnCopyLink(url)
				}
			}},
		})
		c.gap()
	}
	c.wrap(i18n.T("Opening a page only shows Sony's instructions in your browser. It does not update the controller."), cell.Style{Fg: p.Warning, Bg: p.BgPanel}, 2)
}

var detailGroupTitles = []struct {
	g     device.DetailGroup
	title string
}{
	{device.DetailDevice, "Device"},
	{device.DetailFirmware, "Firmware"},
	{device.DetailHardware, "Hardware"},
	{device.DetailDriver, "Linux driver"},
}

// renderFirmwareDetails lists every reading of one group; the selected one
// is explained below with its origin and meaning (Advanced mode).
func renderFirmwareDetails(c *cursor, pc PanelContext) {
	s := pc.S
	fv := s.Firmware
	p := c.p()
	const kw = 30
	group := fv.DetailGroup
	if group == "" {
		group = device.DetailFirmware
	}
	var items []choice
	for _, grp := range detailGroupTitles {
		g := grp.g
		items = append(items, choice{ID: "fw_group_" + string(g), Label: i18n.T(grp.title), Selected: g == group, OnSelect: func() {
			if pc.CB.OnSetDetailGroup != nil {
				pc.CB.OnSetDetailGroup(g)
			}
		}})
	}
	c.pills(i18n.T("Show:"), 0, items)
	if fv.Supported && group == device.DetailFirmware {
		setAuto := func(on bool) func() {
			return func() {
				if pc.CB.OnSetFirmwareAutoCheck != nil {
					pc.CB.OnSetFirmwareAutoCheck(on)
				}
			}
		}
		c.pills(i18n.T("Daily automatic check:"), 0, []choice{
			{ID: "fw_autocheck_on", Label: i18n.T("On"), Selected: fv.AutoCheck, OnSelect: setAuto(true)},
			{ID: "fw_autocheck_off", Label: i18n.T("Off"), Selected: !fv.AutoCheck, OnSelect: setAuto(false)},
		})
	}

	var rows []device.Detail
	for _, d := range fv.Details {
		if d.Group == group && d.Label != "Note" {
			rows = append(rows, d)
		}
	}
	c.gap()
	if len(rows) == 0 {
		c.wrap(i18n.T("The driver reports nothing in this group."), p.MutedStyle(), 2)
		return
	}
	sel := rows[0]
	for _, d := range rows {
		if d.Key == fv.SelectedDetail {
			sel = d
		}
	}
	for _, d := range rows {
		if !c.ok() {
			break
		}
		key := d.Key
		selected := key == sel.Key
		marker, lst := "  ", p.MutedStyle()
		if selected {
			marker, lst = "› ", cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}
		}
		y := c.y
		c.f.Buffer.SetStringWithin(c.r.X, y, marker+i18n.T(d.Label), lst, kw)
		if c.r.Width > kw {
			copyValue(c.f, cell.NewRect(c.r.X+kw, y, c.r.Width-kw, 1), s, detailValue(d), detailValue(d), detailStyle(c, d))
		}
		rect := cell.NewRect(c.r.X, y, min(kw, c.r.Width), 1)
		c.f.RegisterClickHandler(rect, func(_ driver.MouseEvent) {
			if pc.CB.OnSelectDetail != nil {
				pc.CB.OnSelectDetail(key)
			}
		})
		st := accessibility.NodeState(0)
		if selected {
			st = accessibility.StateSelected
		}
		c.f.RegisterAccessibility(accessibility.AccessibilityNode{
			ID: "fw_detail_" + key, Role: accessibility.RoleListItem, Label: i18n.T(d.Label), Value: detailValue(d), State: st, Bounds: rect,
		})
		c.y++
	}

	if c.remaining() > 4 {
		c.gap()
	}
	c.line(i18n.Tf("About %s", i18n.T(sel.Label)), p.SectionHeaderStyle())
	var about []string
	if sel.Origin != "" {
		about = append(about, i18n.Tf("Read from %s.", i18n.T(sel.Origin)))
	}
	if sel.Note != "" {
		about = append(about, i18n.T(sel.Note))
	}
	if sel.Raw {
		about = append(about, i18n.T("(raw): shown exactly as read; its meaning is not documented."))
	}
	st := p.LabelStyle()
	if sel.Unavailable {
		st = cell.Style{Fg: p.Warning, Bg: p.BgPanel}
	}
	c.wrap(strings.Join(about, " "), st, 5)
}
