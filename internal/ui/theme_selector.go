package ui

import (
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"sort"
	"strings"
)

// ThemeSelector is a simple list to choose a chroma style.
type ThemeSelector struct {
	names    []string
	selected int
	width    int
	height   int
	onApply  func(name string) tea.Cmd
	onChange func(name string) tea.Cmd
}

func NewThemeSelector(onApply func(name string) tea.Cmd) *ThemeSelector {
	// Ensure custom styles (e.g., turbo-pascal) are registered before fetching names
	registerCustomStylesOnce()
	// Curated list to keep selection compact while useful.
	curated := []string{
		"turbo-pascal", // custom style resembling Turbo Pascal colors
		"dracula", "monokai", "github-dark", "nord", "solarized-dark",
		"solarized-light", "gruvbox-dark", "friendly", "borland", "native",
	}
	// Intersect curated with available; fall back to all if intersection is small.
	avail := styles.Names()
	set := map[string]bool{}
	for _, n := range avail {
		set[n] = true
	}
	var names []string
	for _, n := range curated {
		if set[n] {
			names = append(names, n)
		}
	}
	if len(names) < 5 {
		names = avail
	}
	sort.Strings(names)
	return &ThemeSelector{names: names, selected: 0, onApply: onApply}
}

// SetSelectedByName moves the selection to the first occurrence of name
// if present; otherwise it leaves the current selection unchanged.
func (s *ThemeSelector) SetSelectedByName(name string) {
	if name == "" {
		return
	}
	for i, n := range s.names {
		if n == name {
			s.selected = i
			return
		}
	}
}

func (s *ThemeSelector) Init() tea.Cmd { return nil }

func (s *ThemeSelector) SetDimensions(w, h int) { s.width, s.height = w, h }

func (s *ThemeSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.String() {
		case "up":
			if s.selected > 0 {
				s.selected--
			}
			if s.onChange != nil {
				return s, s.onChange(s.names[s.selected])
			}
		case "down":
			if s.selected < len(s.names)-1 {
				s.selected++
			}
			if s.onChange != nil {
				return s, s.onChange(s.names[s.selected])
			}
		case "enter":
			if s.onApply != nil {
				return s, s.onApply(s.names[s.selected])
			}
		case "esc":
			// Close via modal onClose
			return s, nil
		}
	}
	return s, nil
}

func (s *ThemeSelector) View() string {
	// Dialog-specific styling: whole area cyan background with black text;
	// selected row contrasted as white background with black text.
	base := lipgloss.NewStyle().
		Background(lipgloss.White).
		Foreground(lipgloss.Black)
	sel := lipgloss.NewStyle().
		Background(lipgloss.Cyan).
		Foreground(lipgloss.Black)

	var b strings.Builder
	start := 0
	end := len(s.names)
	if s.height > 0 && len(s.names) > s.height {
		// Scroll window to keep selection visible
		if s.selected < s.height {
			end = s.height
		} else {
			start = s.selected - s.height + 1
			end = start + s.height
		}
	}
	for i := start; i < end && i < len(s.names); i++ {
		name := s.names[i]
		line := name
		if i == s.selected {
			line = sel.Render(line)
		} else {
			line = base.Render(line)
		}
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return base.Width(s.width).Height(s.height).Render(b.String())
}

// SetOnChange sets a callback invoked when selection changes via navigation.
func (s *ThemeSelector) SetOnChange(fn func(name string) tea.Cmd) { s.onChange = fn }
