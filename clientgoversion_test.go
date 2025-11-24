package kc

import "testing"

func TestClientGoVersionFromEmbeddedGoMod(t *testing.T) {
	expected := normalizeClientGoVersion(parseClientGoVersion(string(goModData)))
	if expected == "" {
		expected = "(unknown)"
	}
	if got := ClientGoVersion(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestParseClientGoVersionInBlock(t *testing.T) {
	data := `
require (
	k8s.io/client-go v1.2.3
	k8s.io/api v1.2.3
)
`
	if got := parseClientGoVersion(data); got != "v1.2.3" {
		t.Fatalf("expected v1.2.3, got %q", got)
	}
}

func TestParseClientGoVersionSingleLine(t *testing.T) {
	data := `require k8s.io/client-go v4.5.6 // comment`
	if got := parseClientGoVersion(data); got != "v4.5.6" {
		t.Fatalf("expected v4.5.6, got %q", got)
	}
}

func TestParseClientGoVersionMissing(t *testing.T) {
	data := `module example.com/test`
	if got := parseClientGoVersion(data); got != "" {
		t.Fatalf("expected empty version, got %q", got)
	}
}

func TestNormalizeClientGoVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"v0.34.1", "v1.34.1"},
		{"v0.0.0-20250101", "v1.0.0-20250101"},
		{"v1.2.3", "v1.2.3"},
		{"", ""},
	}
	for _, tt := range cases {
		if got := normalizeClientGoVersion(tt.in); got != tt.want {
			t.Fatalf("normalizeClientGoVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
