package ui

import (
    "sort"
    "strings"
    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/alecthomas/chroma/v2/styles"
)

// ThemeSelector is a simple list to choose a chroma style.
type ThemeSelector struct {
    names    []string
    selected int
    width    int
    height   int
    onApply  func(name string) tea.Cmd
}

func NewThemeSelector(onApply func(name string) tea.Cmd) *ThemeSelector {
    // Curated list to keep selection compact while useful.
    curated := []string{
        "turbo-pascal", // custom style resembling Turbo Pascal colors
        "dracula", "monokai", "github-dark", "nord", "solarized-dark",
        "solarized-light", "gruvbox-dark", "friendly", "borland", "native",
    }
    // Intersect curated with available; fall back to all if intersection is small.
    avail := styles.Names()
    set := map[string]bool{}
    for _, n := range avail { set[n] = true }
    var names []string
    for _, n := range curated { if set[n] { names = append(names, n) } }
    if len(names) < 5 {
        names = avail
    }
    sort.Strings(names)
    return &ThemeSelector{names: names, selected: 0, onApply: onApply}
}

func (s *ThemeSelector) Init() tea.Cmd { return nil }

func (s *ThemeSelector) SetDimensions(w, h int) { s.width, s.height = w, h }

func (s *ThemeSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch m := msg.(type) {
    case tea.KeyMsg:
        switch m.String() {
        case "up":
            if s.selected > 0 { s.selected-- }
        case "down":
            if s.selected < len(s.names)-1 { s.selected++ }
        case "enter":
            if s.onApply != nil { return s, s.onApply(s.names[s.selected]) }
        case "esc":
            // Close via modal onClose
            return s, nil
        }
    }
    return s, nil
}

func (s *ThemeSelector) View() string {
    // Render a simple list with current selection highlighted
    var b strings.Builder
    start := 0
    end := len(s.names)
    if s.height > 0 && len(s.names) > s.height {
        // Scroll window to keep selection visible
        if s.selected < s.height { end = s.height } else {
            start = s.selected - s.height + 1
            end = start + s.height
        }
    }
    for i := start; i < end && i < len(s.names); i++ {
        name := s.names[i]
        line := name
        if i == s.selected {
            line = PanelItemSelectedStyle.Render(line)
        } else {
            line = PanelItemStyle.Render(line)
        }
        b.WriteString(line)
        if i < end-1 { b.WriteString("\n") }
    }
    return PanelContentStyle.Width(s.width).Height(s.height).Render(b.String())
}
