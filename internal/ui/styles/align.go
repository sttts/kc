package styles

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// AlignCenter pads the rendered content to the given width using the provided
// style for the filler columns (typically to ensure the background color
// matches the modal). When width <= content width, content is returned
// unchanged.
func AlignCenter(width int, content string, style lipgloss.Style) string {
	contentWidth := lipgloss.Width(content)
	if width <= contentWidth {
		return content
	}
	padding := width - contentWidth
	left := padding / 2
	right := padding - left
	filler := func(n int) string {
		if n <= 0 {
			return ""
		}
		return style.Render(strings.Repeat(" ", n))
	}
	return filler(left) + content + filler(right)
}
