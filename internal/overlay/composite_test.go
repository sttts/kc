package overlay

import (
	"strings"
	"testing"
)

func Test_clamp(t *testing.T) {
	tests := []struct {
		name                    string
		val, min, max, expected int
	}{
		{"val 0, min 0, max 100", 0, 0, 100, 0},
		{"val 100, min 0, max 100", 100, 0, 100, 100},
		{"val -1, min 0, max 100", -1, 0, 100, 0},
		{"val 101, min 0, max 100", 101, 0, 100, 100},
		{"val -1, min 0, max -100", -1, 0, -100, -1},
		{"val 0, min 100, max 0", 0, 100, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clamp(tt.val, tt.min, tt.max); got != tt.expected {
				t.Fatalf("clamp got=%d want=%d", got, tt.expected)
			}
		})
	}
}

func Test_lines(t *testing.T) {
	tests := []struct {
		name, val string
		expected  int
	}{
		{"3 lines, no unexpected line endings", "aaa\nbbb\nccc", 3},
		{"3 lines, one unexpected line ending", "aaa\r\nbbb\nccc", 3},
		{"1 line, no line ending", "aaabbbccc", 1},
		{"empty string", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := lines(tt.val)
			if len(res) != tt.expected {
				t.Fatalf("lines len=%d want=%d", len(res), tt.expected)
			}
		})
	}
}

// Note: centering pushes left/up when dimensions are odd (from original tests).
func Test_offsets(t *testing.T) {
	cases := []struct {
		name                 string
		fg, bg               string
		xPos, yPos           Position
		xOff, yOff           int
		expectedX, expectedY int
	}{
		{"centered, odd fg height and width, no offset",
			strings.Repeat("abcde\n", 5), strings.Repeat("123456789\n", 9), Center, Center, 0, 0, 2, 2},
		{"centered, even fg height and width, no offset",
			strings.Repeat("abcd\n", 4), strings.Repeat("123456789\n", 9), Center, Center, 0, 0, 2, 3},
		{"centered, odd fg height and width, with offset",
			strings.Repeat("abcde\n", 5), strings.Repeat("123456789\n", 9), Center, Center, 1, 1, 3, 3},
		{"top left, odd fg height and width, no offset",
			strings.Repeat("abcde\n", 5), strings.Repeat("123456789\n", 9), Left, Top, 0, 0, 0, 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			x, y := offsets(tt.fg, tt.bg, tt.xPos, tt.yPos, tt.xOff, tt.yOff)
			if x != tt.expectedX || y != tt.expectedY {
				t.Fatalf("offsets got=(%d,%d) want=(%d,%d)", x, y, tt.expectedX, tt.expectedY)
			}
		})
	}
}

func Test_composite(t *testing.T) {
	fg := strings.Repeat("abc\n", 2) + "abc"
	bg := strings.Repeat("1234567\n", 6) + "1234567"
	cases := []struct {
		name       string
		xPos, yPos Position
		xOff, yOff int
		expected   string
	}{
		{"centered, no offset", Center, Center, 0, 0,
			strings.Repeat("1234567\n", 2) + strings.Repeat("12abc67\n", 3) + "1234567\n1234567"},
		{"centered, with offset", Center, Center, 1, 1,
			strings.Repeat("1234567\n", 3) + strings.Repeat("123abc7\n", 3) + "1234567"},
		{"top left, no offset", Left, Top, 0, 0,
			strings.Repeat("abc4567\n", 3) + strings.Repeat("1234567\n", 3) + "1234567"},
		{"top center, no offset", Center, Top, 0, 0,
			strings.Repeat("12abc67\n", 3) + strings.Repeat("1234567\n", 3) + "1234567"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := Composite(fg, bg, tt.xPos, tt.yPos, tt.xOff, tt.yOff)
			if got != tt.expected {
				t.Fatalf("composite mismatch\n got:\n%q\nwant:\n%q", got, tt.expected)
			}
		})
	}
}
