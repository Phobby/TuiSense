package views

import "testing"

func TestButtonLabelShowsToggleState(t *testing.T) {
	for _, tc := range []struct {
		it   choice
		want string
	}{
		{choice{Label: "Raw Logs"}, "Raw Logs"},
		{choice{Label: "Advanced", Kind: kindOption, Selected: true}, "Advanced"},
		{choice{Label: "Pre-releases", Kind: kindToggle, Selected: true}, "[x] Pre-releases"},
		{choice{Label: "Pre-releases", Kind: kindToggle}, "[ ] Pre-releases"},
	} {
		if got := buttonLabel(tc.it); got != tc.want {
			t.Errorf("%+v labelled %q, want %q", tc.it, got, tc.want)
		}
		if w := pillWidth(tc.it); int(w) != len([]rune(tc.want))+4 {
			t.Errorf("pillWidth(%+v) = %d, want the label plus the frame", tc.it, w)
		}
	}
}
