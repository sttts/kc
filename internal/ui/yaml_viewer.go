package ui

import (
    tea "github.com/charmbracelet/bubbletea/v2"
    "strings"
)

// YAMLViewer is a simple scrollable text viewer for YAML content.
// Note: Syntax highlighting to be integrated with a library (e.g., chroma) in a follow-up.
type YAMLViewer struct {
    title   string
    content []string
    width   int
    height  int
    offset  int
    onEdit  func() tea.Cmd // invoked on F4
}

func NewYAMLViewer(title, text string, onEdit func() tea.Cmd) *YAMLViewer {
    lines := strings.Split(text, "\n")
    return &YAMLViewer{title: title, content: lines, onEdit: onEdit}
}

func (v *YAMLViewer) Init() tea.Cmd { return nil }

func (v *YAMLViewer) SetDimensions(w, h int) { v.width, v.height = w, h }

func (v *YAMLViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch m := msg.(type) {
    case tea.KeyMsg:
        switch m.String() {
        case "up":
            if v.offset > 0 { v.offset-- }
        case "down":
            if v.offset < max(0, len(v.content)-v.height) { v.offset++ }
        case "pgup":
            v.offset = max(0, v.offset-(v.height-1))
        case "pgdown":
            v.offset = min(max(0, len(v.content)-v.height), v.offset+(v.height-1))
        case "home":
            v.offset = 0
        case "end":
            v.offset = max(0, len(v.content)-v.height)
        case "f4":
            if v.onEdit != nil { return v, v.onEdit() }
        }
    }
    return v, nil
}

func (v *YAMLViewer) View() string {
    if v.height <= 0 || v.width <= 0 { return "" }
    end := min(len(v.content), v.offset+v.height)
    lines := v.content[v.offset:end]
    // Trim each line to width
    trimmed := make([]string, len(lines))
    for i, ln := range lines {
        if len(ln) > v.width { ln = ln[:v.width] }
        trimmed[i] = ln
    }
    // Render using panel content style for consistent look
    return PanelContentStyle.Width(v.width).Height(v.height).Render(strings.Join(trimmed, "\n"))
}
