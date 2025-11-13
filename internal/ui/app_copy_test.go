package ui

import "testing"

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"pod.log":         "pod.log",
		"/tmp/foo.yaml":   "foo.yaml",
		"../etc/passwd":   "passwd",
		"config*map?.txt": "config_map_.txt",
		"":                "",
	}
	for input, expect := range cases {
		if got := sanitizeFilename(input); got != expect {
			t.Fatalf("sanitizeFilename(%q) = %q, want %q", input, got, expect)
		}
	}
}
