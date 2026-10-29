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

	footer := cell.NewRect(total.X, total.Y+total.Height-1, total.Width, 1)
	// Header row, the page tabs' roof, labels and base line, then the body.
	bodyY, bodyH := total.Y+4, total.Height-5
	kind := LayoutStacked
	if total.Width < 100 {
		kind = LayoutCompact
	}
	inspectorH := uint16(6)
	if tallInspector {
		inspectorH = bodyH - min(uint16(8), bodyH/3)
	}
	stageH := bodyH - inspectorH
	return DashboardLayout{Kind: kind, Compact: kind == LayoutCompact,
		HeaderArea:    cell.NewRect(total.X, total.Y, total.Width, 1),
		TabsArea:      cell.NewRect(total.X, total.Y+2, total.Width, 1),
		Stage3DArea:   cell.NewRect(total.X, bodyY, total.Width, stageH),
		InspectorArea: cell.NewRect(total.X, bodyY+stageH, total.Width, inspectorH), FooterArea: footer}
}
