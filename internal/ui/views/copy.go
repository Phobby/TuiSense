package views

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/ui/copytext"
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// CopyScope prevents an old focus target from copying a value from another panel.
func CopyScope(s *state.AppState) string {
	scope := string(s.ActivePanel) + ":" + s.ActiveDeviceKey + ":" + string(s.Mode)
	if s.Diag != nil {
		scope += ":" + string(s.Diag.Active) + ":" + s.Diag.StickSubTest + fmt.Sprintf(":report=%v:%s:%d:%d", s.Diag.ShowTestReport, s.Diag.ExpandedReport, s.Diag.HistoryIndex, s.Diag.ReportGeneration) + fmt.Sprintf(":technical=%v", s.Diag.ShowTechnicalReport)
	}
	return scope + ":" + string(s.Firmware.Section) + ":" + string(s.Firmware.DetailGroup)
}

func copyValue(f *terminal.Frame, r cell.Rect, s *state.AppState, display, value string, st cell.Style) {
	copytext.CopyableText{Text: display, Value: value, Style: st}.Render(f, r, &s.Copy)
}

func renderCopyActions(c *cursor, current bool) {
	type action struct {
		id, label string
		run       func()
	}
	items := []action{{"copy_all_diagnostics", i18n.T("Copy All Diagnostics"), func() {
		if c.s.Copy.OnCopy != nil {
			if c.s.ActivePanel == state.PanelDiagnostics {
				c.s.Copy.OnCopy(BuildHealthClipboard(c.s, false, time.Now()))
			} else {
				c.s.Copy.OnCopy(BuildClipboardReport(c.s, false, "", time.Now()))
			}
		}
	}}}
	if current {
		items = append(items, action{"copy_test_results", i18n.T("Copy Current Test"), func() {
			if c.s.Copy.OnCopy != nil {
				if c.s.ActivePanel == state.PanelDiagnostics {
					c.s.Copy.OnCopy(BuildHealthClipboard(c.s, true, time.Now()))
				} else {
					c.s.Copy.OnCopy(BuildClipboardReport(c.s, true, "", time.Now()))
				}
			}
		}})
	}
	label := i18n.T("Identifiers: Off")
	if c.s.IncludeCopyIdentifiers {
		label = i18n.T("Identifiers: On")
	}
	if c.s.ActivePanel != state.PanelDiagnostics {
		items = append(items, action{"copy_identifiers", label, func() { c.s.IncludeCopyIdentifiers = !c.s.IncludeCopyIdentifiers }})
	}
	x := c.r.X
	for _, a := range items {
		w := min(c.r.Width, uint16(cell.StringWidth(a.label)))
		if x+w > c.limit() {
			c.y++
			x = c.r.X
		}
		if !c.ok() {
			return
		}
		copytext.CopyableText{ID: a.id, Text: a.label, Style: c.p().ButtonStyle(false), OnActivate: a.run}.Render(c.f, cell.NewRect(x, c.y, w, 1), &c.s.Copy)
		x += w + 2
	}
	c.y++
}

var macPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(?:[:-][0-9a-f]{2}){5}\b`)
var identifierPattern = regexp.MustCompile(`(?i)\b(serial(?: number)?|mac(?: address)?|unique(?: identifier)?|bluetooth address)\s*[:=]\s*\S+`)

func sensitiveDetail(d device.Detail) bool {
	key := strings.ToLower(d.Key + " " + d.Label)
	for _, token := range []string{"serial", "mac", "unique", "sysfs", "bluetooth address", "device id", "address", "path"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

// BuildClipboardReport formats a snapshot of actual diagnostic results and
// device facts. Missing measurements stay unmeasured; raw audio/HID dumps and
// unrelated logs are deliberately outside this technical report.
func BuildClipboardReport(s *state.AppState, current bool, version string, now time.Time) string {
	var b strings.Builder
	fmt.Fprintln(&b, "TuiSense Diagnostic Report")
	if version != "" {
		fmt.Fprintln(&b, "Version:", version)
	}
	fmt.Fprintln(&b, "Generated:", now.Format(time.RFC3339Nano))
	fmt.Fprintln(&b, "\nDevice:")
	field := func(k, v string) {
		if v == "" {
			v = "Unavailable / not reported"
		}
		fmt.Fprintf(&b, "  %s: %s\n", k, v)
	}
	info := s.DeviceInfo
	field("Model", s.Descriptor.DisplayName)
	field("Manufacturer", s.Descriptor.Brand)
	field("Model ID", s.Descriptor.ModelID)
	field("Connection", string(info.Connection))
	field("Firmware", info.FirmwareVersion)
	field("Firmware build", info.FirmwareBuild)
	field("Hardware revision", info.HardwareVersion)
	if info.VendorID != 0 || info.ProductID != 0 {
		field("USB VID:PID", fmt.Sprintf("0x%04X:0x%04X", info.VendorID, info.ProductID))
	}
	field("Input device", info.InputDevicePath)
	field("HID device", info.HIDRawPath)
	field("Kernel driver", info.KernelDriver)
	field("Input report", info.InputMode)
	field("Identification", info.Identification)
	if s.IncludeCopyIdentifiers {
		field("Bluetooth MAC", info.MACAddress)
		field("Device identifier", info.ID)
	}
	fmt.Fprintln(&b, "\nDevice Details & Firmware:")
	// Arbitrary provider details require a known nonunique key, or explicit opt-in.
	safeKeys := map[string]bool{"model": true, "manufacturer": true, "connection": true, "usb_id": true, "hidraw": true, "driver": true, "fw_version": true, "fw_build": true, "fw_date": true, "hw_version": true, "revision": true, "fw_target": true, "fw_build_sysfs": true, "fw_type": true, "sw_series": true, "fw_components": true, "update_image": true, "hw_fields": true, "hid_name": true, "sysfs_attrs": true, "kernel": true, "notice_usb": true, "notice_permission": true, "notice_simulated": true}
	for _, d := range s.Firmware.Details {
		if !s.IncludeCopyIdentifiers && (sensitiveDetail(d) || !safeKeys[d.Key]) {
			continue
		}
		value := d.Value
		if d.Unavailable {
			value = "Unavailable: " + value
		}
		field(d.Label, value)
		if d.Note != "" {
			field("Explanation", d.Note)
		}
	}
	if r := s.Firmware.Result; r != nil {
		field("Update status", string(r.Status))
		field("Explanation", r.Summary)
		field("Basis", r.Basis)
		field("Checked at", r.CheckedAt.Format(time.RFC3339Nano))
		for _, msg := range r.Diagnostics {
			field("Warning", msg)
		}
	} else {
		field("Update status", "Not checked / unavailable")
	}
	if d := s.Diag; d != nil {
		if d.Ctx.Simulated {
			fmt.Fprintln(&b, "\nSIMULATED DEVICE: values are mock data, not hardware measurements.")
		}
		r := diag.BuildReport(d, version, s.IncludeCopyIdentifiers, now)
		for _, e := range r.Entries {
			if current && s.ActivePanel != state.PanelFirmware && d.Active != "" && e.Category != d.Active {
				continue
			}
			if current && s.ActivePanel == state.PanelFirmware {
				continue
			}
			fmt.Fprintf(&b, "\n%s:\n", e.Label)
			field("Status", e.Status.String())
			if e.Reason != "" {
				field("Availability", e.Reason)
			}
			if e.Result == nil {
				field("Measurements", "Not measured")
				continue
			}
			result := e.Result
			field("Summary", result.Summary)
			if !result.At.IsZero() {
				field("Test timestamp", result.At.Format(time.RFC3339Nano))
			}
			for _, detail := range result.Details {
				field("Detail", detail)
			}
			for _, m := range result.Measurements {
				field(m.Name, fmt.Sprintf("%.2f%s", m.Value, unitSuffix(m.Unit)))
			}
			for _, l := range result.Limitations {
				field("Limitation", l)
			}
			if result.UserConfirmed {
				field("Confirmation", "User confirmed; not an instrument measurement")
			}
		}
		if d.Sticks != nil && (!current || s.ActivePanel != state.PanelFirmware && d.Active == diag.CatSticks) {
			fmt.Fprintln(&b, "\nDeadzone Preview (software illustration):")
			field("Selected deadzone", fmt.Sprintf("%.1f%%", d.Sticks.PreviewDeadzone))
			if s.Telemetry.Raw.Valid {
				for i, l := range d.Sticks.Live {
					field(fmt.Sprintf("Stick %d raw position", i+1), fmt.Sprintf("%+.4f, %+.4f", l.Norm[0], l.Norm[1]))
				}
			} else {
				field("Live raw input", "Unavailable / no raw sample")
			}
		}
	} else {
		fmt.Fprintln(&b, "\nDiagnostics: no active session; no measurements available.")
	}
	if !current || s.ActivePanel == state.PanelDiagnostics {
		fmt.Fprintln(&b, "\nWarnings & Errors:")
		if s.Logger != nil {
			for i := 0; i < s.Logger.Len(); i++ {
				if s.Logger.Level(i) >= widgets.LevelWarn {
					field("Log", s.Logger.Line(i))
				}
			}
		}
		if s.LastError != "" {
			field("Error", s.LastError)
			field("Timestamp", s.LastErrorAt.Format(time.RFC3339Nano))
		}
		if info.PermissionDenied {
			field("Permission", info.PermissionHint)
		}
	}
	report := b.String()
	if !s.IncludeCopyIdentifiers {
		// Also scrub identifiers embedded inside provider explanations and errors.
		secrets := []string{info.MACAddress, info.ID, info.SysfsPath}
		for _, d := range s.Firmware.Details {
			if sensitiveDetail(d) {
				secrets = append(secrets, d.Value)
			}
		}
		sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
		for _, secret := range secrets {
			if len(secret) >= 4 {
				report = strings.ReplaceAll(report, secret, "[redacted]")
			}
		}
		report = macPattern.ReplaceAllString(report, "[redacted]")
		report = identifierPattern.ReplaceAllString(report, "identifier: [redacted]")
	}
	return report
}

// cleanCopyText strips presentation prefixes while preserving technical payloads.
func cleanCopyText(text string) string {
	for _, prefix := range []string{"· ", "• ", "ⓘ ", "▸ ", "⚠ ", "✓ ", "✕ "} {
		text = strings.TrimPrefix(text, prefix)
	}
	return text
}

func wrapCopyText(text string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if line != "" && cell.StringWidth(line)+1+cell.StringWidth(word) > width {
				lines = append(lines, line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			// Long tokens (hex dumps, paths) still fit the visible cell width.
			for _, r := range word {
				if cell.StringWidth(line+string(r)) > width && line != "" {
					lines = append(lines, line)
					line = ""
				}
				line += string(r)
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func renderCopyFallback(f *terminal.Frame, total cell.Rect, s *state.AppState) {
	if total.Width < 12 || total.Height < 6 {
		return
	}
	w := min(total.Width-4, uint16(90))
	h := min(total.Height-2, uint16(14))
	r := cell.NewRect(total.X+(total.Width-w)/2, total.Y+(total.Height-h)/2, w, h)
	f.RegisterModal("copy_fallback", r, func() { s.ShowCopyFallback = false })
	f.BeginLayer("copy_fallback")
	defer f.EndLayer()
	inner := panelFrame(f, r, s, i18n.T("Clipboard unavailable"))
	text := i18n.T("Exact text saved to a private UTF-8 file:") + "\n" + s.CopyFallbackPath + "\n\n" + i18n.T("Open this file to copy or share the text. Delete it after use.") + "\n" + i18n.T("Install wl-clipboard (Wayland), xclip/xsel (X11), or enable OSC 52 with TUISENSE_OSC52=1. Restart TuiSense after changing the environment.") + "\n\n" + i18n.T("Enter / Esc: close")
	lines := wrapCopyText(text, int(inner.Width))
	for i, line := range lines {
		if i >= int(inner.Height) {
			break
		}
		f.Buffer.SetStringWithin(inner.X, inner.Y+uint16(i), line, s.Theme.LabelStyle(), inner.Width)
	}
	f.RegisterAccessibility(accessibility.AccessibilityNode{ID: "copy_fallback", Role: accessibility.RoleGeneric, Label: i18n.T("Clipboard unavailable"), Value: text, Bounds: r})
}
