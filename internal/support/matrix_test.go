package support

import (
	"os"
	"strings"
	"testing"
)

// The checked-in matrix must be regenerated whenever a descriptor changes:
//
//	go run ./cmd/tuisense debug support-matrix > docs/SUPPORT_MATRIX.md
func TestSupportMatrixDocumentIsCurrent(t *testing.T) {
	want := Markdown(Matrix())
	got, err := os.ReadFile("../../docs/SUPPORT_MATRIX.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatal("docs/SUPPORT_MATRIX.md is stale; regenerate it with `go run ./cmd/tuisense debug support-matrix > docs/SUPPORT_MATRIX.md`")
	}
}

func TestMatrixCoversTheTenControllersHonestly(t *testing.T) {
	rows := Matrix()
	if len(rows) != 10 {
		t.Fatalf("expected the 10 target controllers, got %d", len(rows))
	}
	for _, r := range rows {
		if strings.Contains(r.Verification, "Verified on hardware") {
			t.Errorf("%s: hardware verification must never be claimed from automated tests", r.Model)
		}
		if r.Visual == "" || r.Detection == "" || r.Drivers == "" {
			t.Errorf("%s: incomplete row %+v", r.Model, r)
		}
	}
	byModel := map[string]Row{}
	for _, r := range rows {
		byModel[r.ModelID] = r
	}
	if r := byModel["msi.force_gc20_v2"]; r.Battery != "—" || r.Lighting != "—" || r.Triggers != "—" {
		t.Fatalf("MSI has no battery, lighting or adaptive triggers: %+v", r)
	}
	if r := byModel["microsoft.xbox_elite_series2"]; r.Rear != "Implemented (read/test)" || r.Remapping == "Implemented (configures device)" {
		t.Fatalf("Elite paddles are readable when the driver reports them, never remappable by TuiSense: %+v", r)
	}
}
