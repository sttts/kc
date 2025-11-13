package podfs

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestQuoteArg(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "''"},
		{"simple", "'simple'"},
		{"path with space", "'path with space'"},
		{"has'single", "'has'\"'\"'single'"},
	}
	for _, tc := range cases {
		if got := quoteArg(tc.in); got != tc.want {
			t.Fatalf("quoteArg(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDecodeEntry(t *testing.T) {
	row := "373f|42|1700000000|file|etc/passwd|"
	encoded := encodeRowForTest(row)
	entry, err := decodeEntry(encoded)
	if err != nil {
		t.Fatalf("decodeEntry error: %v", err)
	}
	if entry.Name != "etc/passwd" || entry.Type != EntryTypeFile || entry.Size != 42 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.Mode != 0x373f {
		t.Fatalf("mode mismatch: %x", entry.Mode)
	}
	if entry.UpdatedAt.Unix() != 1700000000 {
		t.Fatalf("mtime mismatch: %v", entry.UpdatedAt)
	}
}

func encodeRowForTest(row string) string {
	var b strings.Builder
	encoder := base64.StdEncoding.WithPadding(base64.StdPadding)
	b.Grow(encoder.EncodedLen(len(row)))
	b.WriteString(encoder.EncodeToString([]byte(row)))
	return b.String()
}
