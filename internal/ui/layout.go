package ui

import "github.com/thebanri/limoni/core/cell"

// LayoutKind names the responsive arrangement chosen for a terminal size.
type LayoutKind int

const (
	// LayoutWide: three symmetric columns — info | controller | inspector.
	LayoutWide LayoutKind = iota
	// LayoutStacked: status | controller | info on top, inspector full-width below.
	LayoutStacked
	// LayoutCompact: controller left, status + inspector right (small terminals).
	LayoutCompact
)

// DashboardLayout holds the computed rectangles for all regions of the main screen.
// In Wide and Stacked layouts the side columns have equal width, so the
// controller stage is horizontally centered on screen.
type DashboardLayout struct {
	Kind              LayoutKind
	HeaderArea        cell.Rect // brand, device selector, mode switch
	TabsArea          cell.Rect // capability-driven panel tabs
	Stage3DArea       cell.Rect
	BatteryStatusArea cell.Rect
	DeviceSummaryArea cell.Rect
	TelemetryArea     cell.Rect // optional live input / tips card (zero if absent)
	InspectorArea     cell.Rect
	FooterArea        cell.Rect
	Compact           bool
}

func clampU16(v, lo, hi uint16) uint16 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ComputeDashboardLayout calculates responsive regions for any terminal size
// from compact 80x24 up to ultrawide 200x60+ displays.
func ComputeDashboardLayout(total cell.Rect) DashboardLayout {
	return ComputeLayout(total, false)
}

// ComputeLayout is ComputeDashboardLayout with an option to give the inspector
// more height (used by the Diagnostics tab); the stage stays centered.
func ComputeLayout(total cell.Rect, tallInspector bool) DashboardLayout {
	if total.Width < 20 || total.Height < 10 {
		return DashboardLayout{Kind: LayoutCompact, InspectorArea: total, Compact: true}
	}

	footerH := uint16(2)
	if total.Height < 30 {
		footerH = 1
	}
	header := cell.NewRect(total.X, total.Y, total.Width, 1)
	tabs := cell.NewRect(total.X, total.Y+1, total.Width, 1)
	footer := cell.NewRect(total.X, total.Y+total.Height-footerH, total.Width, footerH)

	bodyY := total.Y + 3 // header + tabs + one spacer row
	bodyH := total.Height - 3 - footerH
	l := DashboardLayout{HeaderArea: header, TabsArea: tabs, FooterArea: footer}

	switch {
	case total.Width >= 150 && bodyH >= 22:
		l.Kind = LayoutWide
		side := clampU16(total.Width*30/100, 46, 64)
		center := total.Width - 2*side
		leftX := total.X
		centerX := total.X + side
		rightX := centerX + center

		l.Stage3DArea = cell.NewRect(centerX, bodyY, center, bodyH)

		statusH := uint16(5)
		l.BatteryStatusArea = cell.NewRect(rightX, bodyY, side, statusH)
		l.InspectorArea = cell.NewRect(rightX, bodyY+statusH, side, bodyH-statusH)

		infoH := clampU16(bodyH/2, 9, 13)
		l.DeviceSummaryArea = cell.NewRect(leftX, bodyY, side, infoH)
		l.TelemetryArea = cell.NewRect(leftX, bodyY+infoH, side, bodyH-infoH)

	case total.Width >= 96 && bodyH >= 30:
		l.Kind = LayoutStacked
		side := clampU16(total.Width*24/100, 28, 40)
		center := total.Width - 2*side
		topH := bodyH * 58 / 100
		if tallInspector {
			topH = bodyH * 42 / 100
		}
		if topH < 13 {
			topH = 13
		}
		l.BatteryStatusArea = cell.NewRect(total.X, bodyY, side, 5)
		l.TelemetryArea = cell.NewRect(total.X, bodyY+5, side, topH-5)
		l.Stage3DArea = cell.NewRect(total.X+side, bodyY, center, topH)
		l.DeviceSummaryArea = cell.NewRect(total.X+side+center, bodyY, side, topH)
		l.InspectorArea = cell.NewRect(total.X, bodyY+topH, total.Width, bodyH-topH)

	default:
		l.Kind = LayoutCompact
		l.Compact = true
		leftW := total.Width * 46 / 100
		if leftW < 34 && total.Width >= 72 {
			leftW = 34
		}
		rightW := total.Width - leftW
		rightX := total.X + leftW
		l.Stage3DArea = cell.NewRect(total.X, bodyY, leftW, bodyH)
		statusH := uint16(5)
		if bodyH < 24 {
			statusH = 4
		}
		l.BatteryStatusArea = cell.NewRect(rightX, bodyY, rightW, statusH)
		l.InspectorArea = cell.NewRect(rightX, bodyY+statusH, rightW, bodyH-statusH)
	}
	return l
}
