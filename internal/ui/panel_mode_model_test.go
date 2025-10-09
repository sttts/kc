package ui

import "testing"

func TestModeLabel(t *testing.T) {
	cases := map[PanelViewMode]string{
		PanelModeList:     "List",
		PanelModeDescribe: "Describe",
		PanelModeManifest: "Manifest",
		PanelModeFile:     "File",
	}
	for mode, expect := range cases {
		if got := modeLabel(mode); got != expect {
			t.Fatalf("mode %v label = %q, want %q", mode, got, expect)
		}
	}
}
