package ui

import (
	"testing"

	"github.com/thebanri/limoni/core/cell"
)

func TestStageIsHorizontallyCentered(t *testing.T) {
	for _, sz := range [][2]uint16{{120, 40}, {150, 40}, {170, 46}, {220, 60}} {
		total := cell.NewRect(0, 0, sz[0], sz[1])
		l := ComputeDashboardLayout(total)
		if l.Kind == LayoutCompact {
			t.Fatalf("%dx%d should not use the compact layout", sz[0], sz[1])
		}
		left := l.Stage3DArea.X - total.X
		right := total.X + total.Width - (l.Stage3DArea.X + l.Stage3DArea.Width)
		if left != right {
			t.Errorf("%dx%d: stage not centered (left margin %d, right margin %d)", sz[0], sz[1], left, right)
		}
		if l.InspectorArea.Height > 6 || l.BatteryStatusArea.Height != 0 || l.DeviceSummaryArea.Height != 0 {
			t.Errorf("%dx%d: unexpected permanent side panels: %+v", sz[0], sz[1], l)
		}
	}
}

func TestCompactLayoutFitsSmallTerminals(t *testing.T) {
	total := cell.NewRect(0, 0, 80, 24)
	l := ComputeDashboardLayout(total)
	if l.Kind != LayoutCompact || !l.Compact {
		t.Fatalf("80x24 should be compact")
	}
	if l.Stage3DArea.Width == 0 || l.InspectorArea.Width == 0 {
		t.Fatalf("compact layout must still show stage and inspector")
	}
	if end := l.InspectorArea.Y + l.InspectorArea.Height; end > l.FooterArea.Y {
		t.Fatalf("inspector overlaps footer")
	}
}

func TestDiagnosticsStageStaysCenteredOnCompactScreens(t *testing.T) {
	for _, size := range [][2]uint16{{80, 24}, {100, 28}, {120, 40}, {170, 46}} {
		area := cell.NewRect(2, 3, size[0], size[1])
		layout := ComputeLayout(area, true)
		left := layout.Stage3DArea.X - area.X
		right := area.X + area.Width - layout.Stage3DArea.X - layout.Stage3DArea.Width
		if left != right {
			t.Fatalf("diagnostics stage must remain centered at %v", size)
		}
		if layout.InspectorArea.Y+layout.InspectorArea.Height > layout.FooterArea.Y {
			t.Fatal("inspector overlaps footer")
		}
	}
}
