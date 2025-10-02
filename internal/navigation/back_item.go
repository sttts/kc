package navigation

import "github.com/charmbracelet/lipgloss/v2"

// BackItem renders the ".." entry for WithBack wrappers.
type BackItem struct{}

func (BackItem) Columns() (string, []string, []*lipgloss.Style, bool) {
	style := GreenStyle()
	return "__back__", []string{".."}, []*lipgloss.Style{style}, true
}

func (BackItem) Details() string { return "Back" }
func (BackItem) Path() []string  { return nil }
